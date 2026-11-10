<script lang="ts">
  /* Vue « Chaînes » (issue #11) : la liste des chaînes du snapshot avec
     numéro, logo, nom, source playlist, groupe, source XMLTV, id XMLTV.
     Tout l'état vit dans le store canaux.svelte (pagination SERVEUR 50,
     filtre groupe, recherche locale de la page courante) : la vue ne
     connaît ni fetch ni URL. Les logos sont servis depuis leur URL
     d'ORIGINE du fournisseur — Zapette ne proxifie pas les logos ;
     images décoratives (le nom est déjà dans la table) : alt="" et
     loading="lazy". Champs absents → « — ». Responsive : la table défile
     horizontalement (overflow-x-auto, même motif que la table Sources).
     Accessibilité : th scope, labels explicites, aria-live sur le pied de
     pagination, focus visible (app.css), thèmes sombre/clair via dark:. */
  import { onMount } from 'svelte';
  import { créerCanaux } from '../canaux.svelte';
  import Carte from './Carte.svelte';

  interface CanauxProps {
    /** Groupes pour le sélecteur : ceux connus du /api/status (per_group,
        triés par la vue appelante). Valeur '' = tous les groupes. */
    groupes?: string[];
  }
  const { groupes = [] }: CanauxProps = $props();

  const canaux = créerCanaux();

  onMount(() => {
    void canaux.charger();
  });

  /** Valeur affichée : null/absent → « — » (contrat #11). '' du groupe
      est traité comme un groupe absent côté affichage. */
  const afficher = (valeur: string | null): string =>
    valeur === null || valeur === '' ? '—' : valeur;

  function choisirGroupe(évènement: Event): void {
    const sélecteur = évènement.currentTarget as HTMLSelectElement;
    void canaux.choisirGroupe(sélecteur.value);
  }

  function saisirRecherche(évènement: Event): void {
    const champ = évènement.currentTarget as HTMLInputElement;
    canaux.définirRecherche(champ.value);
  }
</script>

<Carte titre="Chaînes" pleineLargeur>
  {#snippet àCôté()}
    <p class="text-xs tabular-nums text-slate-500 dark:text-slate-400">
      {canaux.total}
      {canaux.total === 1 ? 'chaîne' : 'chaînes'}
    </p>
  {/snippet}

  {#if canaux.état === 'erreur'}
    <div
      role="alert"
      class="mb-4 rounded-lg border border-red-300 bg-red-50 p-3 text-sm text-red-800 dark:border-red-500/40 dark:bg-red-950/40 dark:text-red-300"
    >
      Chargement des chaînes en échec ({canaux.messageErreur}).
      {#if canaux.total > 0}
        Affichage du dernier contenu connu.
      {/if}
    </div>
  {/if}

  <div class="mb-4 flex flex-wrap items-end gap-3">
    <div class="flex flex-col gap-1">
      <label for="filtre-groupe" class="text-sm font-medium text-slate-600 dark:text-slate-400">
        Groupe
      </label>
      <select
        id="filtre-groupe"
        onchange={choisirGroupe}
        value={canaux.groupeActuel}
        class="max-w-60 rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm text-slate-800 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200"
      >
        <option value="">Tous les groupes</option>
        {#each groupes as nom (nom)}
          <option value={nom}>{nom}</option>
        {/each}
      </select>
    </div>
    <div class="flex flex-col gap-1">
      <label for="recherche-canaux" class="text-sm font-medium text-slate-600 dark:text-slate-400">
        Recherche (nom ou id XMLTV, page courante)
      </label>
      <input
        id="recherche-canaux"
        type="search"
        oninput={saisirRecherche}
        placeholder="filtre local…"
        class="w-56 rounded-lg border border-slate-300 bg-white px-3 py-1.5 text-sm text-slate-800 placeholder:text-slate-400 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200"
      />
    </div>
  </div>

  {#if canaux.état === 'chargement' && canaux.total === 0}
    <p role="status" aria-live="polite" class="text-sm text-slate-500 dark:text-slate-400">
      Chargement des chaînes…
    </p>
  {:else if canaux.total === 0}
    <p class="text-sm text-slate-500 dark:text-slate-400">
      Aucune chaîne dans le snapshot (liste vide ou snapshot absent).
    </p>
  {:else if canaux.lignes.length === 0}
    <p class="text-sm text-slate-500 dark:text-slate-400">
      Aucune chaîne de la page courante ne correspond à la recherche.
    </p>
  {:else}
    <div class="overflow-x-auto">
      <table class="w-full text-sm">
        <thead>
          <tr class="text-left text-slate-500 dark:text-slate-400">
            <th scope="col" class="py-2 pr-3 font-medium">N°</th>
            <th scope="col" class="py-2 pr-3 font-medium">Logo</th>
            <th scope="col" class="py-2 pr-3 font-medium">Nom</th>
            <th scope="col" class="py-2 pr-3 font-medium">Source playlist</th>
            <th scope="col" class="py-2 pr-3 font-medium">Groupe</th>
            <th scope="col" class="py-2 pr-3 font-medium">Source XMLTV</th>
            <th scope="col" class="py-2 font-medium">id XMLTV</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-slate-100 dark:divide-slate-800">
          {#each canaux.lignes as chaîne, i (chaîne.nom + chaîne.tvg_id + i)}
            <tr>
              <td class="py-2 pr-3 tabular-nums">{afficher(chaîne.numero)}</td>
              <td class="py-2 pr-3">
                {#if chaîne.logo}
                  <img
                    src={chaîne.logo}
                    alt=""
                    loading="lazy"
                    class="inline-block size-6 rounded object-contain"
                  />
                {:else}
                  <span class="text-slate-400 dark:text-slate-500">—</span>
                {/if}
              </td>
              <th scope="row" class="py-2 pr-3 text-left font-medium">{chaîne.nom}</th>
              <td class="py-2 pr-3">{chaîne.source_m3u}</td>
              <td class="py-2 pr-3">{afficher(chaîne.groupe)}</td>
              <td class="py-2 pr-3">{afficher(chaîne.source_xmltv)}</td>
              <td class="py-2 font-mono text-xs">{afficher(chaîne.tvg_id)}</td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}

  <nav aria-label="Pagination des chaînes" class="mt-4 flex items-center justify-between gap-3">
    <p class="text-sm tabular-nums text-slate-500 dark:text-slate-400">
      Page {canaux.pageActuelle + 1} / {canaux.totalPages}
    </p>
    <div class="flex gap-2">
      <button
        type="button"
        disabled={!canaux.aPagePrécédente}
        onclick={() => void canaux.pagePrécédente()}
        class="rounded-lg border border-slate-300 bg-slate-50 px-3 py-1.5 text-sm font-medium text-slate-800
          hover:border-indigo-500 hover:text-indigo-700 disabled:cursor-not-allowed disabled:opacity-50
          dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200 dark:hover:border-indigo-400 dark:hover:text-indigo-300"
      >
        Précédent
      </button>
      <button
        type="button"
        disabled={!canaux.aPageSuivante}
        onclick={() => void canaux.pageSuivante()}
        class="rounded-lg border border-slate-300 bg-slate-50 px-3 py-1.5 text-sm font-medium text-slate-800
          hover:border-indigo-500 hover:text-indigo-700 disabled:cursor-not-allowed disabled:opacity-50
          dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200 dark:hover:border-indigo-400 dark:hover:text-indigo-300"
      >
        Suivant
      </button>
    </div>
  </nav>
</Carte>
