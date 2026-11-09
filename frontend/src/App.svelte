<script lang="ts">
  // Page unique du dashboard. Le store (créerStatut) poll /api/status
  // toutes les 5 s ; la page n'appelle jamais fetch, ne connaît ni fetch
  // ni temporisateurs. Responsive mobile-first (une colonne → 2 → 3),
  // accessible : role="status" aria-live sur le badge du poll et sur la
  // bannière d'erreur, étiquettes visibles, focus visible (app.css).
  import { onMount } from 'svelte';
  import { créerStatut, type ÉtatArtefact, type VueSource } from './lib/statut.svelte';
  import { formatDurée, formatHorodatage, formatOctets } from './lib/format';
  import Carte from './lib/composantes/Carte.svelte';
  import BasculeTheme from './lib/composantes/BasculeTheme.svelte';

  const statut = créerStatut();

  /** Les deux artefacts d'une source, sous forme plate pour la boucle
      {#each} imbriquée du tableau (typage explicite de la paire). */
  function artefactsDe(source: VueSource): Array<{ artefact: 'm3u' | 'xmltv'; état: ÉtatArtefact }> {
    return [
      { artefact: 'm3u', état: source.m3u },
      { artefact: 'xmltv', état: source.xmltv },
    ];
  }

  onMount(() => {
    statut.démarrer();
    return () => statut.arrêter();
  });

  const classeBadge = (état: string) =>
    état === 'ok'
      ? 'bg-green-700 dark:bg-green-600'
      : état === 'erreur'
        ? 'bg-red-700 dark:bg-red-500'
        : 'bg-amber-700 dark:bg-amber-500';

  const libelléÉtat = (état: string) =>
    état === 'ok' ? 'Connecté' : état === 'erreur' ? 'Rafraîchissement en échec' : 'Chargement';
</script>

<div class="mx-auto max-w-6xl px-4 py-6 sm:px-6 lg:py-10">
  <header class="mb-6 flex flex-wrap items-start justify-between gap-3">
    <div>
      <h1 class="text-2xl font-bold tracking-tight text-slate-900 dark:text-slate-50">
        zapette
      </h1>
      <p class="mt-1 flex items-center gap-2 text-sm text-slate-600 dark:text-slate-400">
        <span
          role="status"
          aria-live="polite"
          class="inline-block size-2.5 rounded-full {classeBadge(statut.état)}"
        ></span>
        {libelléÉtat(statut.état)}
        {#if statut.dernierReçu > 0}
          — dernière réponse {formatHorodatage(new Date(statut.dernierReçu).toISOString())}
        {/if}
      </p>
    </div>
    <BasculeTheme />
  </header>

  {#if statut.état === 'erreur'}
    <div
      role="alert"
      class="mb-6 rounded-xl border border-red-300 bg-red-50 p-4 text-sm text-red-800 dark:border-red-500/40 dark:bg-red-950/40 dark:text-red-300"
    >
      <p class="font-semibold">Rafraîchissement du statut en échec</p>
      <p class="mt-1">
        {statut.messageErreur}
        {#if statut.données}
          — affichage du dernier statut connu
          ({formatHorodatage(new Date(statut.dernierReçu).toISOString())}).
        {/if}
      </p>
    </div>
  {/if}

  {#if statut.données}
    <div class="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
      <Carte titre="Snapshot">
        <dl class="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
          <div>
            <dt class="text-slate-500 dark:text-slate-400">Total chaînes</dt>
            <dd class="mt-0.5 text-lg font-semibold tabular-nums">
              {statut.totalChaînes ?? '—'}
            </dd>
          </div>
          <div>
            <dt class="text-slate-500 dark:text-slate-400">Âge du snapshot</dt>
            <dd class="mt-0.5 text-lg font-semibold tabular-nums">
              {formatDurée(statut.âgeSnapshot)}
            </dd>
          </div>
        </dl>
      </Carte>

      <Carte titre="Mémoire (RSS)">
        <p class="text-lg font-semibold tabular-nums">{formatOctets(statut.rssOctets)}</p>
        <p class="mt-1 text-xs text-slate-500 dark:text-slate-400">
          VmRSS du binaire, pics de fetch inclus.
        </p>
      </Carte>

      <Carte titre="Configuration">
        {#if statut.résuméConfig}
          <dl class="grid grid-cols-3 gap-x-4 gap-y-3 text-sm">
            <div>
              <dt class="text-slate-500 dark:text-slate-400">Intervalle</dt>
              <dd class="mt-0.5 font-semibold">{statut.résuméConfig.intervalle}</dd>
            </div>
            <div>
              <dt class="text-slate-500 dark:text-slate-400">Sources</dt>
              <dd class="mt-0.5 font-semibold tabular-nums">{statut.résuméConfig.nbSources}</dd>
            </div>
            <div>
              <dt class="text-slate-500 dark:text-slate-400">Groupes autorisés</dt>
              <dd class="mt-0.5 font-semibold tabular-nums">{statut.résuméConfig.nbGroupes}</dd>
            </div>
          </dl>
        {/if}
      </Carte>

      <Carte titre="IP publiques">
        <dl class="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">
          <div class="min-w-0">
            <dt class="text-slate-500 dark:text-slate-400">IPv4</dt>
            <dd class="mt-0.5 truncate font-mono">{statut.ipv4 ?? '—'}</dd>
          </div>
          <div class="min-w-0">
            <dt class="text-slate-500 dark:text-slate-400">IPv6</dt>
            <dd class="mt-0.5 truncate font-mono">{statut.ipv6 ?? '—'}</dd>
          </div>
          <div class="col-span-2">
            <dt class="text-slate-500 dark:text-slate-400">Dernière vérification</dt>
            <dd class="mt-0.5">{formatHorodatage(statut.dernièreVérificationIP)}</dd>
          </div>
        </dl>
      </Carte>

      <Carte titre="Sources" pleineLargeur>
        {#if statut.sources.length === 0}
          <p class="text-sm text-slate-500 dark:text-slate-400">Aucune source connue.</p>
        {:else}
          <div class="overflow-x-auto">
            <table class="w-full text-sm">
              <thead>
                <tr class="text-left text-slate-500 dark:text-slate-400">
                  <th scope="col" class="py-2 pr-4 font-medium">Source</th>
                  <th scope="col" class="py-2 pr-4 font-medium">Artefact</th>
                  <th scope="col" class="py-2 pr-4 font-medium">Dernier succès</th>
                  <th scope="col" class="py-2 pr-4 font-medium">Dernière erreur</th>
                  <th scope="col" class="py-2 font-medium">Prochain fetch</th>
                </tr>
              </thead>
              <tbody class="divide-y divide-slate-100 dark:divide-slate-800">
                {#each statut.sources as source (source.nom)}
                {#each artefactsDe(source) as ligne (source.nom + ligne.artefact)}
                  <tr>
                    <th scope="row" class="py-2 pr-4 text-left font-medium">{source.nom}</th>
                    <td class="py-2 pr-4 uppercase">{ligne.artefact}</td>
                    <td class="py-2 pr-4 {ligne.état.last_ok == null ? 'text-slate-400 dark:text-slate-500' : ''}">
                      {formatHorodatage(ligne.état.last_ok)}
                    </td>
                    <td class="py-2 pr-4 {ligne.état.last_error === '' ? '' : 'text-red-700 dark:text-red-400'}">
                      {ligne.état.last_error === '' ? '—' : ligne.état.last_error}
                    </td>
                    <td class="py-2">
                      {formatHorodatage(ligne.état.next_at)}
                    </td>
                  </tr>
                {/each}
                {/each}
              </tbody>
            </table>
          </div>
        {/if}
      </Carte>

      <Carte titre="Groupes autorisés" pleineLargeur>
        <p class="mb-3 text-sm text-slate-600 dark:text-slate-400">
          {statut.groupes.length}
          {statut.groupes.length === 1 ? 'groupe' : 'groupes'} dans le snapshot.
        </p>
        <ul class="flex flex-wrap gap-2">
          {#each statut.groupes as groupe (groupe.nom)}
            <li>
              <a
                href={groupe.prévisu}
                class="inline-flex items-center gap-2 rounded-lg border border-slate-300 bg-slate-50 px-3 py-1.5 text-sm font-medium text-slate-800 hover:border-indigo-500 hover:text-indigo-700 dark:border-slate-700 dark:bg-slate-800 dark:text-slate-200 dark:hover:border-indigo-400 dark:hover:text-indigo-300"
                title="Voir le M3U brut de ce groupe (prévisualisation)"
              >
                <span class="max-w-60 truncate">{groupe.nom}</span>
                <span class="rounded bg-slate-200 px-1.5 py-0.5 text-xs tabular-nums text-slate-700 dark:bg-slate-700 dark:text-slate-300">
                  {groupe.nombre}
                </span>
              </a>
            </li>
          {:else}
            <li class="text-sm text-slate-500 dark:text-slate-400">
              Aucun groupe : liste vide (tout conserver) ou snapshot absent.
            </li>
          {/each}
        </ul>
        <p class="mt-3 text-sm">
          <a
            href="/m3u"
            class="font-medium text-indigo-700 underline decoration-indigo-400 underline-offset-4 hover:text-indigo-800 dark:text-indigo-400 dark:hover:text-indigo-300"
          >
            Voir le M3U complet
          </a>
        </p>
      </Carte>

      <Carte titre="Dernières erreurs" pleineLargeur>
        {#if statut.erreurs.length === 0}
          <p class="text-sm text-slate-500 dark:text-slate-400">Aucune erreur signalée.</p>
        {:else}
          <ul class="space-y-2">
            {#each statut.erreurs as erreur, i (i + erreur.at + erreur.message)}
              <li class="flex flex-col gap-0.5 text-sm sm:flex-row sm:gap-3">
                <time class="shrink-0 text-slate-500 tabular-nums dark:text-slate-400">
                  {formatHorodatage(erreur.at)}
                </time>
                <span class="text-red-700 dark:text-red-400">{erreur.message}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </Carte>
    </div>
  {:else if statut.état === 'chargement'}
    <p role="status" aria-live="polite" class="text-sm text-slate-500 dark:text-slate-400">
      Chargement du statut…
    </p>
  {/if}
</div>
