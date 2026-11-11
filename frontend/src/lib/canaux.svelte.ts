/* Store de la vue Chaînes (issue #11) : pagination SERVEUR de
   GET /api/canaux (50/page, offset/limit), filtre groupe (exact, retourné
   au serveur — c'est lui qui scanne canaux.json en streaming) et recherche
   locale (nom / id XMLTV) de la page courante. Isolé de la mise en page,
   testé par Vitest avec l'API simulée.

   Même discipline que statut.svelte.ts : Svelte 5 runes dans un .svelte.ts,
   $state privés exposés par getters, fetch injectable — le store ne connaît
   ni Svelte ni le DOM, la page ne connaît ni fetch ni URL. */

export type ÉtatChargement = 'chargement' | 'ok' | 'erreur';

/** Une chaîne du snapshot — le contrat Go de internal/store.Canal : numero
    est TOUJOURS défini (numéro officiel TNT/Arcom quand le tvg_id y
    figure, sinon suite séquentielle après le plan), nulls explicites pour
    logo/tvg_id/source_xmltv. */
export interface Canal {
  numero: string;
  nom: string;
  logo: string | null;
  groupe: string;
  tvg_id: string | null;
  source_m3u: string;
  source_xmltv: string | null;
}

/** Réponse GET /api/canaux : la fenêtre demandée, total au-delà d'elle. */
export interface RéponseCanaux {
  total: number;
  offset: number;
  limit: number;
  canaux: Canal[];
}

export interface OptionsCanaux {
  /** Taille de page côté serveur, 50 par défaut (issue #11). */
  taillePage?: number;
  /** fetch injectable (tests) ; défaut : le fetch global. */
  requête?: typeof fetch;
}

export function créerCanaux(options: OptionsCanaux = {}) {
  const taillePage = options.taillePage ?? 50;
  const requête = options.requête ?? ((url: string) => fetch(url));

  let état = $state<ÉtatChargement>('chargement');
  let données = $state<RéponseCanaux | null>(null);
  let messageErreur = $state<string | null>(null);
  /** Front de pagination/filtre : page 0-based, groupe actuel ('' = tous),
      recherche locale — elle NE retourne JAMAIS au serveur. */
  let page = $state(0);
  let groupe = $state('');
  let recherche = $state('');

  async function charger(): Promise<void> {
    const paramètres = new URLSearchParams({
      offset: String(page * taillePage),
      limit: String(taillePage),
    });
    if (groupe !== '') {
      // Un nom de groupe porte espaces, accents, esperluettes : encoder.
      paramètres.set('groupe', groupe);
    }
    const url = `/api/canaux?${paramètres.toString()}`;
    try {
      const réponse = await requête(url);
      if (!réponse.ok) {
        throw new Error(`/api/canaux répond ${réponse.status}`);
      }
      const corps = (await réponse.json()) as RéponseCanaux;
      données = corps;
      état = 'ok';
      messageErreur = null;
    } catch (erreur) {
      // Un raté de chargement ne jette PAS la page précédente : comme le
      // poll de statut, la vue continue d'afficher le dernier bon contenu.
      état = 'erreur';
      messageErreur = erreur instanceof Error ? erreur.message : String(erreur);
    }
  }

  /** Total côté serveur du filtre courant : la pagination s'y fonde. */
  function totalFiltré(): number {
    return données?.total ?? 0;
  }

  return {
    /** État du dernier chargement : 'chargement' avant la première
        réponse, puis 'ok' ou 'erreur' (données précédentes survivent). */
    get état(): ÉtatChargement {
      return état;
    },
    get messageErreur(): string | null {
      return messageErreur;
    },
    /** Total d'entrées du filtre courant (toutes pages). */
    get total(): number {
      return totalFiltré();
    },
    /** Nombre de pages 50, minimum 1 (une liste vide est une page vide). */
    get totalPages(): number {
      return Math.max(1, Math.ceil(totalFiltré() / taillePage));
    },
    /** Page courante, 0-based. */
    get pageActuelle(): number {
      return page;
    },
    get aPagePrécédente(): boolean {
      return page > 0;
    },
    get aPageSuivante(): boolean {
      return (page + 1) * taillePage < totalFiltré();
    },
    get groupeActuel(): string {
      return groupe;
    },
    /** Entrées de la page courante, filtrées LOCALEMENT par la recherche
        (nom affiché OU id XMLTV, insensible à la casse) : la recherche ne
        concerne que la page servie, jamais un refetch. */
    get lignes(): Canal[] {
      const terme = recherche.trim().toLowerCase();
      const entrées = données?.canaux ?? [];
      if (terme === '') {
        return entrées;
      }
      return entrées.filter(
        (c) =>
          c.nom.toLowerCase().includes(terme) ||
          (c.tvg_id ?? '').toLowerCase().includes(terme),
      );
    },

    /** charger — la page courante depuis le serveur (premier chargement,
        reprise après une navigation). */
    charger,

    /** pageSuivante / pagePrécédente : un refetch de la fenêtre adjacente ;
        au-delà de la dernière page (ou avant la première), inerte. */
    async pageSuivante(): Promise<void> {
      if (!this.aPageSuivante) {
        return;
      }
      page++;
      await charger();
    },
    async pagePrécédente(): Promise<void> {
      if (!this.aPagePrécédente) {
        return;
      }
      page--;
      await charger();
    },
    /** choisirGroupe — changement de filtre, refetch REPARTI de la page 1
        (le total change, l'ancienne fenêtre ne veut plus rien dire).
        Le groupe courant ne déclenche pas de refetch. */
    async choisirGroupe(nouveau: string): Promise<void> {
      if (nouveau === groupe) {
        return;
      }
      groupe = nouveau;
      page = 0;
      await charger();
    },
    /** définirRecherche — filtre local, sans requête (page courante). */
    définirRecherche(terme: string): void {
      recherche = terme;
    },
  };
}
