/* Store de statut du dashboard : poll de GET /api/status toutes les 5 s
   (story 16), isolé de la mise en page, testé par Vitest avec /api/status
   simulé. États chargement/ok/erreur, âge du snapshot ré-estimé en temps
   réel, projections prêtes à l'affichage (compteurs par groupe, liens de
   prévisualisation /m3u?group=, ring d'erreurs, résumé config, IP).

   Svelte 5 runes dans un .svelte.ts : les $state sont des champs privés,
   exposés par des getters — le poll ne connaît ni Svelte ni le DOM, la
   page ne connaît ni fetch ni timing. Les deux passerelles (implémentation
   de fetch, horloge) sont injectables : les tests pilotent la boucle avec
   des temporisateurs simulés. */

export type ÉtatPoll = 'chargement' | 'ok' | 'erreur';

/** Réponse GET /api/status — le contrat Go de internal/server, sans null
    ambigu : sources, per_group et errors sont présents même à froid. */
export interface ÉtatArtefact {
  last_ok: string | null;
  last_error: string;
  next_at: string | null;
}

export interface ÉtatSource {
  m3u: ÉtatArtefact;
  xmltv: ÉtatArtefact;
}

export interface RéponseStatut {
  generated_at: string;
  sources: Record<string, ÉtatSource>;
  snapshot: {
    age_seconds: number | null;
    total_channels: number;
    per_group: Record<string, number>;
  };
  rss_bytes: number;
  errors: Array<{ at: string; message: string }>;
  config: { refresh_interval: string; nb_sources: number; nb_groupes: number };
  public_ipv4: string;
  public_ipv6: string;
  public_ip_last_check: string | null;
}

/** uneSource / unGroupe / uneErreur : projections à plat, triées, prêtes
    pour la boucle {#each} des composants. */
export interface VueSource {
  nom: string;
  m3u: ÉtatArtefact;
  xmltv: ÉtatArtefact;
}

export interface VueGroupe {
  nom: string;
  nombre: number;
  /** Lien de prévisualisation brute du groupe (story 17). encodeURIComponent
      : un nom de groupe contient souvent des caractères spéciaux (&, %). */
  prévisu: string;
}

export interface VueErreur {
  at: string;
  message: string;
}

export interface OptionsStatut {
  /** Cycle de rafraîchissement, 5 000 ms par défaut (story 16). */
  intervalleMs?: number;
  /** fetch injectable (tests) ; défaut : le fetch global. */
  requête?: typeof fetch;
  /** Horloge injectable (tests) ; défaut : Date.now. */
  horloge?: () => number;
}

const DÉFAUT_INTERVALLE = 5_000;

export function créerStatut(options: OptionsStatut = {}) {
  const intervalleMs = options.intervalleMs ?? DÉFAUT_INTERVALLE;
  const requête = options.requête ?? ((url: string) => fetch(url));
  const horloge = options.horloge ?? Date.now;

  let état = $state<ÉtatPoll>('chargement');
  let données = $state<RéponseStatut | null>(null);
  let messageErreur = $state<string | null>(null);
  /** reçuÀ : horodatage local (horloge()) de la dernière réponse OK —
      sert à la ré-estimation de l'âge du snapshot entre deux polls. */
  let reçuÀ = $state<number>(0);

  let minuterie: ReturnType<typeof setTimeout> | undefined;
  let enCours = false;
  let abandonné = false;
  /** génération : un numéro d'ordre ignore les réponses périmées si un
      rafraîchissement manuel croise une réponse en vol. */
  let génération = 0;

  async function rafraîchir(): Promise<void> {
    if (enCours) {
      return;
    }
    enCours = true;
    const mienne = ++génération;
    try {
      const réponse = await requête('/api/status');
      if (abandonné || mienne !== génération) {
        return;
      }
      if (!réponse.ok) {
        throw new Error(`/api/status répond ${réponse.status}`);
      }
      const corps = (await réponse.json()) as RéponseStatut;
      if (abandonné || mienne !== génération) {
        return;
      }
      données = corps;
      état = 'ok';
      messageErreur = null;
      reçuÀ = horloge();
    } catch (erreur) {
      if (abandonné || mienne !== génération) {
        return;
      }
      // Le poll en échec ne jette PAS les données précédentes : le
      // dashboard continue d'afficher le dernier bon statut, une bannière
      // dit depuis quand il ne bouge plus.
      état = 'erreur';
      messageErreur = erreur instanceof Error ? erreur.message : String(erreur);
    } finally {
      enCours = false;
    }
  }

  function programmeLaProchaine(): void {
    if (abandonné) {
      return;
    }
    minuterie = setTimeout(async () => {
      await rafraîchir();
      programmeLaProchaine();
    }, intervalleMs);
  }

  return {
    /** État du poll : 'chargement' avant la première réponse, puis 'ok'
        ou 'erreur' (mais les données précédentes survivent à 'erreur'). */
    get état(): ÉtatPoll {
      return état;
    },
    get données(): RéponseStatut | null {
      return données;
    },
    get messageErreur(): string | null {
      return messageErreur;
    },
    /** Âge du snapshot ré-estimé : age_seconds du serveur + temps écoulé
        localement depuis la réponse, borné à 0 (horloges légèrement
        désaccordées ne doivent pas afficher -12 s). null sans snapshot. */
    get âgeSnapshot(): number | null {
      const brut = données?.snapshot.age_seconds;
      if (brut == null) {
        return null;
      }
      const delta = évaporation(reçuÀ, horloge()) / 1000;
      return Math.max(0, brut + Math.floor(delta));
    },
    /** Total de chaînes du snapshot : null si pas de statut. */
    get totalChaînes(): number | null {
      return données ? données.snapshot.total_channels : null;
    },
    /** Groupes triés alphabétiquement, avec lien /m3u?group=. */
    get groupes(): VueGroupe[] {
      if (!données) {
        return [];
      }
      return Object.entries(données.snapshot.per_group)
        .map(([nom, nombre]) => ({ nom, nombre, prévisu: `/m3u?group=${encodeURIComponent(nom)}` }))
        .sort((a, b) => a.nom.localeCompare(b.nom, 'fr'));
    },
    /** Sources triées alphabétiquement. */
    get sources(): VueSource[] {
      if (!données) {
        return [];
      }
      return Object.entries(données.sources)
        .map(([nom, états]) => ({ nom, m3u: états.m3u, xmltv: états.xmltv }))
        .sort((a, b) => a.nom.localeCompare(b.nom, 'fr'));
    },
    /** Ring des dernières erreurs (ordre du serveur : récent d'abord). */
    get erreurs(): VueErreur[] {
      return données ? données.errors.map((e) => ({ at: e.at, message: e.message })) : [];
    },
    get résuméConfig(): { intervalle: string; nbSources: number; nbGroupes: number } | null {
      if (!données) {
        return null;
      }
      return {
        intervalle: données.config.refresh_interval,
        nbSources: données.config.nb_sources,
        nbGroupes: données.config.nb_groupes,
      };
    },
    get rssOctets(): number | null {
      return données ? données.rss_bytes : null;
    },
    /** IP publiques : « indisponible » du contrat est exposé tel quel —
        la page affiche « — » dans ce cas, pas une fausse IP. */
    get ipv4(): string | null {
      const valeur = données?.public_ipv4 ?? null;
      return valeur && valeur !== 'indisponible' ? valeur : null;
    },
    get ipv6(): string | null {
      const valeur = données?.public_ipv6 ?? null;
      return valeur && valeur !== 'indisponible' ? valeur : null;
    },
    get dernièreVérificationIP(): string | null {
      return données?.public_ip_last_check ?? null;
    },
    /** Horodatage local (reçuÀ) de la dernière réponse acceptée. */
    get dernierReçu(): number {
      return reçuÀ;
    },

    /** démarrer — premier fetch immédiat, puis un fetch chaque intervalle.
        Ré-entrant : un démarrage en cours abandonne la boucle précédente. */
    démarrer(): void {
      abandonné = false;
      if (minuterie !== undefined) {
        clearTimeout(minuterie);
        minuterie = undefined;
      }
      void rafraîchir().then(programmeLaProchaine);
    },
    /** arrêter — plus de poll ; une réponse en vol garde son résultat côté
        demandeur, seuls le cycle et l'attribution sont stoppés. */
    arrêter(): void {
      abandonné = true;
      génération++;
      if (minuterie !== undefined) {
        clearTimeout(minuterie);
        minuterie = undefined;
      }
    },
    /** rafraîchir — un fetch manuel sans toucher au cycle. Retourne la
        promesse pour les tests. */
    forcerRafraîchir(): Promise<void> {
      return rafraîchir();
    },
  };
}

function évaporation(avant: number, maintenant: number): number {
  return Math.max(0, maintenant - avant);
}
