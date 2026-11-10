import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { créerStatut, type RéponseStatut } from './statut.svelte';

/** Une réponse /api/status réaliste (présent + froid). */
const statutOK: RéponseStatut = {
  generated_at: '2025-11-10T07:42:00Z',
  sources: {
    'provider-a': {
      m3u: {
        last_ok: '2025-11-10T07:40:00Z',
        last_error: '',
        next_at: '2025-11-10T13:40:00Z',
      },
      xmltv: {
        last_ok: null,
        last_error: 'délai dépassé',
        next_at: null,
      },
    },
  },
  snapshot: {
    age_seconds: 30,
    total_channels: 421,
    per_group: { Sports: 300, 'Actualités': 121 },
  },
  rss_bytes: 4_432_512,
  errors: [{ at: '2025-11-10T07:39:00Z', message: 'provider-a xmltv : délai dépassé' }],
  config: { refresh_interval: '6h0m0s', nb_sources: 1, nb_groupes: 2 },
  public_ipv4: '203.0.113.7',
  public_ipv6: 'indisponible',
  public_ip_last_check: '2025-11-10T07:40:30Z',
};

function réponse(corps: unknown, ok = true, status = 200): Response {
  return { ok, status, json: async () => corps } as unknown as Response;
}

describe('créerStatut — poll /api/status', () => {
  let horlogeCourante: number;
  const avancer = (ms: number) => {
    horlogeCourante += ms;
  };
  const horloge = () => horlogeCourante;

  beforeEach(() => {
    vi.useFakeTimers();
    horlogeCourante = 1_000_000;
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('démarre en « chargement », passe en « ok » et expose les données', async () => {
    const requête = vi.fn(async () => réponse(statutOK));
    const statut = créerStatut({ requête, horloge });

    expect(statut.état).toBe('chargement');
    statut.démarrer();
    await vi.advanceTimersByTimeAsync(1);

    expect(statut.état).toBe('ok');
    expect(statut.données).toEqual(statutOK);
    expect(statut.totalChaînes).toBe(421);
    expect(statut.rssOctets).toBe(4_432_512);
    expect(statut.messageErreur).toBeNull();

    statut.arrêter();
  });

  it('poll toutes les 5 s (story 16)', async () => {
    const requête = vi.fn(async () => réponse(statutOK));
    const statut = créerStatut({ requête, horloge });
    statut.démarrer();
    await vi.advanceTimersByTimeAsync(5_000);
    // t=0 (premier fetch) + t=5 s.
    expect(requête).toHaveBeenCalledTimes(2);
    await vi.advanceTimersByTimeAsync(10_000);
    expect(requête).toHaveBeenCalledTimes(4);

    statut.arrêter();
  });

  it('arrêter coupe le cycle', async () => {
    const requête = vi.fn(async () => réponse(statutOK));
    const statut = créerStatut({ requête, horloge });
    statut.démarrer();
    await vi.advanceTimersByTimeAsync(5_000);
    statut.arrêter();
    requête.mockClear();
    await vi.advanceTimersByTimeAsync(60_000);
    expect(requête).not.toHaveBeenCalled();
  });

  it('le tick saute tant qu’une requête est en vol (pas d’empilement)', async () => {
    let débloquer!: () => void;
    const vol = new Promise<Response>((résoudre) => {
      débloquer = () => résoudre(réponse(statutOK));
    });
    const requête = vi.fn(() => vol);
    const statut = créerStatut({ requête, horloge });
    statut.démarrer();

    // Deux cycles passent pendant que le premier fetch reste bloqué.
    await vi.advanceTimersByTimeAsync(10_000);
    expect(requête).toHaveBeenCalledTimes(1);

    débloquer();
    await vol;
    statut.arrêter();
  });
});

describe('créerStatut — états ok / erreur', () => {
  afterEach(() => vi.useRealTimers());

  it('passe en « erreur » sans jeter le dernier bon statut', async () => {
    const requête = vi
      .fn<typeof fetch>()
      .mockResolvedValueOnce(réponse(statutOK))
      .mockRejectedValueOnce(new Error('réseau coupé'));

    const statut = créerStatut({ requête });
    await statut.forcerRafraîchir();
    expect(statut.état).toBe('ok');

    await statut.forcerRafraîchir();
    expect(statut.état).toBe('erreur');
    expect(statut.messageErreur).toBe('réseau coupé');
    // Le dashboard continue d'afficher les données précédentes.
    expect(statut.données).toEqual(statutOK);
    expect(statut.totalChaînes).toBe(421);
  });

  it('réponse HTTP 500 → « erreur » avec le code dans le message', async () => {
    const requête = vi.fn(async () => réponse({ erreur: 'panne' }, false, 500));
    const statut = créerStatut({ requête });
    await statut.forcerRafraîchir();
    expect(statut.état).toBe('erreur');
    expect(statut.messageErreur).toBe('/api/status répond 500');
    expect(statut.données).toBeNull();
  });
});

describe('créerStatut — projections d’affichage', () => {
  afterEach(() => vi.useRealTimers());

  it('groupes triés, avec lien de prévisualisation encodé', async () => {
    const perGroup = { 'Films & Séries': 3, Sports: 5, 'Actualités': 1 };
    const requête = vi.fn(async () =>
      réponse({ ...statutOK, snapshot: { ...statutOK.snapshot, per_group: perGroup } }),
    );
    const statut = créerStatut({ requête });
    await statut.forcerRafraîchir();

    expect(statut.groupes.map((g) => g.nom)).toEqual(['Actualités', 'Films & Séries', 'Sports']);
    expect(statut.groupes.find((g) => g.nom === 'Films & Séries')?.prévisu).toBe(
      '/m3u?group=Films%20%26%20S%C3%A9ries',
    );
    expect(statut.groupes.find((g) => g.nom === 'Sports')?.nombre).toBe(5);
  });

  it('sans données : projections vides, sans exception', () => {
    const statut = créerStatut({ requête: vi.fn() });
    expect(statut.groupes).toEqual([]);
    expect(statut.sources).toEqual([]);
    expect(statut.erreurs).toEqual([]);
    expect(statut.résuméConfig).toBeNull();
    expect(statut.totalChaînes).toBeNull();
    expect(statut.âgeSnapshot).toBeNull();
    expect(statut.ipv4).toBeNull();
    expect(statut.ipv6).toBeNull();
    expect(statut.dernièreVérificationIP).toBeNull();
  });

  it('« indisponible » côté API → null côté store (pas de fausse IP)', async () => {
    const requête = vi.fn(async () => réponse(statutOK)); // v4 renseignée, v6 indisponible
    const statut = créerStatut({ requête });
    await statut.forcerRafraîchir();
    expect(statut.ipv4).toBe('203.0.113.7');
    expect(statut.ipv6).toBeNull();
    expect(statut.dernièreVérificationIP).toBe('2025-11-10T07:40:30Z');
  });

  it('sources triées, projetées du contrat telles quelles', async () => {
    const requête = vi
      .fn<typeof fetch>()
      .mockResolvedValue(réponse(statutOK));
    const statut = créerStatut({ requête });
    await statut.forcerRafraîchir();
    expect(statut.sources.map((s) => s.nom)).toEqual(['provider-a']);
    expect(statut.sources[0].m3u.last_ok).toBe('2025-11-10T07:40:00Z');
    expect(statut.sources[0].xmltv.last_error).toBe('délai dépassé');
  });
});

describe('créerStatut — âge du snapshot ré-estimé', () => {
  let horlogeCourante: number;
  const avance = (ms: number) => {
    horlogeCourante += ms;
  };

  beforeEach(() => vi.useFakeTimers());
  afterEach(() => vi.useRealTimers());

  it('part de age_seconds puis vieillit en temps réel', async () => {
    horlogeCourante = 50_000;
    const requête = vi.fn(async () => réponse(statutOK)); // age_seconds = 30
    const statut = créerStatut({ requête, horloge: () => horlogeCourante });
    await statut.forcerRafraîchir();
    expect(statut.âgeSnapshot).toBe(30);

    avance(7_000);
    expect(statut.âgeSnapshot).toBe(37);
  });

  it('un âge de référence à 0 reste à 0, jamais négatif', async () => {
    horlogeCourante = 50_000;
    const requête = vi.fn(async () =>
      réponse({ ...statutOK, snapshot: { ...statutOK.snapshot, age_seconds: 0 } }),
    );
    const statut = créerStatut({ requête, horloge: () => horlogeCourante });
    await statut.forcerRafraîchir();
    // L'horloge ne recule pas : le delta local est borné à ≥ 0.
    expect(statut.âgeSnapshot).toBe(0);
  });
});
