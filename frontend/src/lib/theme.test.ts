import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import {
  CLÉ_THÈME,
  créerContrôleurThème,
  écrirePréférence,
  élanSombre,
  lirePréférence,
} from './theme';

/** matchMedia simulé, avec callback de changement contrôlable. */
function matchMediaSimulé(darkInitial: boolean) {
  const auditeurs = new Set<() => void>();
  return {
    matches: darkInitial,
    addEventListener: (_: string, cb: () => void) => auditeurs.add(cb),
    removeEventListener: (_: string, cb: () => void) => auditeurs.delete(cb),
    déclencher() {
      for (const cb of auditeurs) cb();
    },
  };
}

/** Une fenêtre minimale pour les tests (vrai document DOM fourni par jsdom).
    Retourne aussi la carte de stockage pour inspecter ce qui est persisté. */
function fenêtreSimuléeVers(
  stockage: Record<string, string> | 'indisponible',
  media: ReturnType<typeof matchMediaSimulé> | null,
): { fenêtre: Window; carte: Map<string, string> | null } {
  const carte =
    stockage === 'indisponible' ? null : (new Map(Object.entries(stockage)) as Map<string, string>);
  const fenêtre = {
    document,
    matchMedia: media ? (_q: string) => media : undefined,
    // Imite window.localStorage : soit fonctionnel, soit en échec.
    get localStorage(): Storage | null {
      if (carte == null) return null;
      return {
        getItem: (clé: string) => carte.get(clé) ?? null,
        setItem: (clé: string, valeur: string) => void carte.set(clé, valeur),
        removeItem: (clé: string) => void carte.delete(clé),
        clear: () => carte.clear(),
        key: (i: number) => [...carte.keys()][i] ?? null,
        get length() {
          return carte.size;
        },
      };
    },
  } as unknown as Window;
  return { fenêtre, carte };
}

const fenêtreSimulée = (
  stockage: Record<string, string> | 'indisponible',
  media: ReturnType<typeof matchMediaSimulé> | null,
): Window => fenêtreSimuléeVers(stockage, media).fenêtre;

beforeEach(() => {
  document.documentElement.classList.remove('dark');
  localStorage.removeItem(CLÉ_THÈME);
});
afterEach(() => {
  document.documentElement.classList.remove('dark');
  localStorage.removeItem(CLÉ_THÈME);
  vi.unstubAllGlobals();
});

describe('lirePréférence / écrirePréférence', () => {
  it('défaut « system » sans préférence stockée', () => {
    expect(lirePréférence(localStorage)).toBe('system');
  });

  it('restaure dark | light depuis localStorage', () => {
    écrirePréférence(localStorage, 'dark');
    expect(lirePréférence(localStorage)).toBe('dark');
    écrirePréférence(localStorage, 'light');
    expect(lirePréférence(localStorage)).toBe('light');
  });

  it('une valeur corrompue retombe sur « system »', () => {
    localStorage.setItem(CLÉ_THÈME, 'hacker-mode');
    expect(lirePréférence(localStorage)).toBe('system');
  });

  it('un stockage indisponible ne casse rien', () => {
    expect(lirePréférence(null)).toBe('system');
    expect(() => écrirePréférence(null, 'dark')).not.toThrow();
  });
});

describe('élanSombre', () => {
  it('system suit le système', () => {
    expect(élanSombre('system', false)).toBe(false);
    expect(élanSombre('system', true)).toBe(true);
  });

  it('dark et light tranchent', () => {
    expect(élanSombre('dark', false)).toBe(true);
    expect(élanSombre('light', true)).toBe(false);
  });
});

describe('créerContrôleurThème — vraie page (jsdom)', () => {
  it('défaut system : suit prefers-color-scheme light → pas de .dark', () => {
    const contrôleur = créerContrôleurThème(fenêtreSimulée({}, matchMediaSimulé(false)));
    expect(contrôleur.préférence).toBe('system');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
    contrôleur.détruire();
  });

  it('défaut system : prefers-color-scheme dark → .dark posé sur <html>', () => {
    const contrôleur = créerContrôleurThème(fenêtreSimulée({}, matchMediaSimulé(true)));
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    contrôleur.détruire();
  });

  it('préférence stockée dark prime sur le système clair', () => {
    localStorage.setItem(CLÉ_THÈME, 'dark');
    const contrôleur = créerContrôleurThème(fenêtreSimulée({ [CLÉ_THÈME]: 'dark' }, matchMediaSimulé(false)));
    expect(document.documentElement.classList.contains('dark')).toBe(true);
    contrôleur.détruire();
  });

  it('définir light alors que le système est sombre gagne', () => {
    const { fenêtre, carte } = fenêtreSimuléeVers({}, matchMediaSimulé(true));
    const contrôleur = créerContrôleurThème(fenêtre);
    contrôleur.définir('light');
    expect(document.documentElement.classList.contains('dark')).toBe(false);
    expect(carte?.get(CLÉ_THÈME)).toBe('light');
    contrôleur.détruire();
  });

  it('le basculement du système est suivi tant que la préférence est system', () => {
    const media = matchMediaSimulé(false);
    const contrôleur = créerContrôleurThème(fenêtreSimulée({}, media));
    expect(document.documentElement.classList.contains('dark')).toBe(false);

    media.matches = true;
    media.déclencher();
    expect(document.documentElement.classList.contains('dark')).toBe(true);

    // Préférence manuelle : la bascule du système n'a plus d'effet.
    contrôleur.définir('light');
    media.déclencher();
    expect(document.documentElement.classList.contains('dark')).toBe(false);
    contrôleur.détruire();
  });
});
