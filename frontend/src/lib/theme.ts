/* Thème : « system » par défaut (suit prefers-color-scheme, et reste
   synchronisé si le système bascule), bascule manuelle dark | light
   persistée dans localStorage sous la clé `zapette-theme`. Le mode est
   appliqué par une classe .dark sur <html> (custom-variant Tailwind v4) ;
   un script inline dans index.html la pose déjà avant le CSS. */

export type PréférenceTheme = 'dark' | 'light' | 'system';

export const CLÉ_THÈME = 'zapette-theme';

export interface ContrôleurThème {
  /** Préférence courante : dark | light | system. */
  readonly préférence: PréférenceTheme;
  /** Change la préférence, l'applique et la persiste. */
  définir(préférence: PréférenceTheme): void;
  /** Détache l'écoute de prefers-color-scheme. */
  détruire(): void;
}

/** lirePréférence — la valeur stockée n'est de source sûre que si elle est
    exactement dark | light | system ; sinon (absente ou corrompue) on
    retombe sur « system ». Le stockage peut être nul (navigation privée
    restrictive) : défaut « system », rien ne casse. */
export function lirePréférence(stockage: Storage | null): PréférenceTheme {
  if (stockage == null) {
    return 'system';
  }
  const valeur = stockage.getItem(CLÉ_THÈME);
  return valeur === 'dark' || valeur === 'light' ? valeur : 'system';
}

/** écrirePréférence — persiste la préférence, tolère l'absence de stockage. */
export function écrirePréférence(stockage: Storage | null, préférence: PréférenceTheme): void {
  stockage?.setItem(CLÉ_THÈME, préférence);
}

/** élanSombre — la classe .dark doit-elle être posée ?
    system suit le système, dark et light tranchent. */
export function élanSombre(préférence: PréférenceTheme, systèmeSombre: boolean): boolean {
  if (préférence === 'dark') return true;
  if (préférence === 'light') return false;
  return systèmeSombre;
}

/** appliquerThème — pose ou retire .dark sur <html>. */
export function appliquerThème(racine: HTMLElement, sombre: boolean): void {
  racine.classList.toggle('dark', sombre);
}

/** créerContrôleurThème — colle le tout ensemble pour une vraie page :
    lit la préférence, l'applique, suit les bascules du système tant que la
    préférence est « system », persiste les changements manuels. Les
    fournisseurs (window, matchMedia) sont injectables pour les tests. */
export function créerContrôleurThème(
  fenêtre: (Pick<Window, 'localStorage' | 'document'> & { matchMedia?: typeof window.matchMedia }) | undefined = window,
): ContrôleurThème {
  const stockage = fenêtre.localStorage ?? null;
  const media = fenêtre.matchMedia?.('(prefers-color-scheme: dark)') ?? null;
  let préférence = lirePréférence(stockage);

  const appliquer = (): void => {
    appliquerThème(
      fenêtre.document.documentElement,
      élanSombre(préférence, media?.matches ?? false),
    );
  };
  appliquer();

  const surChangementSystème = (): void => {
    if (préférence === 'system') {
      appliquer();
    }
  };
  media?.addEventListener?.('change', surChangementSystème);

  return {
    get préférence(): PréférenceTheme {
      return préférence;
    },
    définir(nouvelle) {
      préférence = nouvelle;
      écrirePréférence(stockage, nouvelle);
      appliquer();
    },
    détruire() {
      media?.removeEventListener?.('change', surChangementSystème);
    },
  };
}
