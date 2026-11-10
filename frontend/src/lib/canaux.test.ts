import { beforeEach, describe, expect, it, vi } from 'vitest';
import { créerCanaux, type Canal, type RéponseCanaux } from './canaux.svelte';

/** Fabrique d'une entrée canaux conforme au contrat Go (issue #11). */
function canal(partial: Partial<Canal>): Canal {
  return {
    numero: null,
    nom: '',
    logo: null,
    groupe: '',
    tvg_id: null,
    source_m3u: '',
    source_xmltv: null,
    ...partial,
  };
}

/** Page de 4 chaînes : numero/logo/tvg_id/source_xmltv tantôt null, groupe
    mixte — assez pour tester pagination, filtre et recherche. */
const page: RéponseCanaux = {
  total: 120,
  offset: 0,
  limit: 50,
  canaux: [
    canal({ numero: '4', nom: 'Arte', logo: 'https://img/arte.png', groupe: 'Sports', tvg_id: 'arte.fr', source_m3u: 'arte', source_xmltv: 'arte' }),
    canal({ numero: '5', nom: 'BFM', groupe: 'News', tvg_id: 'bfm.fr', source_m3u: 'arte' }),
    canal({ numero: '7', nom: 'CNN', groupe: 'News', tvg_id: 'cnn.fr', source_m3u: 'miroir', source_xmltv: 'arte' }),
    canal({ nom: 'Sans rien', source_m3u: 'miroir' }),
  ],
};

/** Clé canonique d'une requête : offset|limit|groupe — la requête mockée
    parse L'URL réellement demandée et refuse toute clé inattendue : le
    test voit exactement quels paramètres le store a envoyés, sans copier
    la construction d'URL du store (assertion du contrat, pas du format). */
function clé(url: string): string {
  if (!url.startsWith('/api/canaux?')) {
    throw new Error(`URL inattendue : ${url}`);
  }
  const paramètres = new URLSearchParams(url.slice('/api/canaux?'.length));
  const offset = paramètres.get('offset');
  const limit = paramètres.get('limit');
  const groupe = paramètres.get('groupe') ?? '';
  if (offset === null || limit === null) {
    throw new Error(`paramètres offset/limit absents : ${url}`);
  }
  return `${offset}|${limit}|${groupe}`;
}

/** Dépôt clé canonique → corps. */
function dépôt(corps: Record<string, RéponseCanaux>) {
  return vi.fn(async (url: string) => {
    const trouvé = corps[clé(url)];
    if (!trouvé) {
      throw new Error(`requête inattendue : ${url}`);
    }
    return { ok: true, status: 200, json: async () => trouvé } as unknown as Response;
  });
}

describe('créerCanaux — store de la vue Chaînes', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('démarre en « chargement », charge la première page et plie le contrat', async () => {
    const requête = dépôt({ '0|50|': page });
    const canaux = créerCanaux({ requête });
    expect(canaux.état).toBe('chargement');

    await canaux.charger();

    expect(canaux.état).toBe('ok');
    expect(canaux.total).toBe(120);
    expect(canaux.lignes.length).toBe(4);
    expect(canaux.aPageSuivante).toBe(true);
    expect(canaux.aPagePrécédente).toBe(false);
    expect(canaux.pageActuelle).toBe(0);
  });

  it('pagination 50/page : page 2 demande offset=50, pas de page fantôme au-delà de la dernière', async () => {
    const page2: RéponseCanaux = { total: 120, offset: 50, limit: 50, canaux: page.canaux };
    const page3: RéponseCanaux = { total: 120, offset: 100, limit: 50, canaux: page.canaux.slice(0, 3) };
    const requête = dépôt({
      '0|50|': page,
      '50|50|': page2,
      '100|50|': page3,
    });
    const canaux = créerCanaux({ requête });
    await canaux.charger();

    await canaux.pageSuivante();
    expect(canaux.pageActuelle).toBe(1);
    expect(canaux.aPagePrécédente).toBe(true);

    await canaux.pageSuivante();
    expect(canaux.pageActuelle).toBe(2);
    expect(canaux.aPageSuivante).toBe(false); // ceil(120/50) = 3 pages

    // Pas de page fantôme au-delà de la dernière.
    requête.mockClear();
    await canaux.pageSuivante();
    expect(requête).not.toHaveBeenCalled();
    expect(canaux.pageActuelle).toBe(2);

    await canaux.pagePrécédente();
    expect(canaux.pageActuelle).toBe(1);
  });

  it('sélecteur de groupe : refetch filtré (?groupe encodé) et retour page 0', async () => {
    const news: RéponseCanaux = { total: 47, offset: 0, limit: 50, canaux: [page.canaux[1], page.canaux[2]] };
    const requête = dépôt({
      '0|50|': page, // premier chargement PUIS retour « tous les groupes »
      '50|50|': page,
      // Groupe avec accent et espace : l'URL doit l'encoder.
      '0|50|Chaînes info': news,
    });
    const canaux = créerCanaux({ requête });
    await canaux.charger();
    await canaux.pageSuivante();
    expect(canaux.pageActuelle).toBe(1);

    await canaux.choisirGroupe('Chaînes info');
    expect(canaux.groupeActuel).toBe('Chaînes info');
    expect(canaux.pageActuelle).toBe(0); // le filtre ramène à la page 1
    expect(canaux.total).toBe(47);

    // Le paramètre transmis au serveur est bien le groupe décodé.
    const dernière = requête.mock.calls.at(-1)![0] as string;
    expect(new URLSearchParams(dernière.split('?')[1]).get('groupe')).toBe('Chaînes info');

    // Retour « tous » : ré-interroge le serveur sans paramètre groupe.
    await canaux.choisirGroupe('');
    const appel = requête.mock.calls.at(-1)![0] as string;
    expect(appel).not.toContain('groupe=');
    expect(canaux.groupeActuel).toBe('');
  });

  it('recherche locale (nom / id XMLTV) filtre la PAGE COURANTE, insensible à la casse', async () => {
    const requête = dépôt({ '0|50|': page });
    const canaux = créerCanaux({ requête });
    await canaux.charger();

    canaux.définirRecherche('cnn');
    expect(canaux.lignes.length).toBe(1);
    expect(canaux.lignes[0].tvg_id).toBe('cnn.fr');

    // La recherche porte AUSSI sur l'id XMLTV, pas seulement le nom.
    canaux.définirRecherche('BFM.FR');
    expect(canaux.lignes.length).toBe(1);
    expect(canaux.lignes[0].nom).toBe('BFM');

    // Aucune correspondance : liste vide, pas d'erreur.
    canaux.définirRecherche('autrichien');
    expect(canaux.lignes).toEqual([]);
    expect(canaux.état).toBe('ok');

    // Effacement : la page entière revient.
    canaux.définirRecherche('');
    expect(canaux.lignes.length).toBe(4);
  });

  it('erreur : état « erreur », message explicite, les données précédentes restent affichées', async () => {
    let échoue = false;
    const requête = vi.fn(async (url: string) => {
      if (échoue) {
        return {
          ok: false,
          status: 503,
          json: async () => ({ erreur: 'aucun snapshot' }),
        } as unknown as Response;
      }
      return dépôt({ [clé(url)]: page })(url);
    });
    const canaux = créerCanaux({ requête });
    await canaux.charger();
    expect(canaux.état).toBe('ok');

    échoue = true;
    await canaux.pageSuivante();
    expect(canaux.état).toBe('erreur');
    expect(canaux.messageErreur).toContain('503');
    // Le dernier bon contenu reste servi.
    expect(canaux.total).toBe(120);
    expect(canaux.lignes.length).toBe(4);
  });

  it('réponse non ok : état erreur avec le code HTTP dans le message', async () => {
    const requête = vi.fn(async () => ({ ok: false, status: 503, json: async () => ({}) }) as unknown as Response);
    const canaux = créerCanaux({ requête });
    await canaux.charger();
    expect(canaux.état).toBe('erreur');
    expect(canaux.messageErreur).toContain('503');
  });
});
