<script lang="ts">
  // Carte générique : titre + emplacement optionnel à droite du titre
  // (badge, compteur), corps passé en snippet enfants.
  import type { Snippet } from 'svelte';

  interface CarteProps {
    titre: string;
    àCôté?: Snippet | null;
    children: Snippet;
    /** Pleine largeur sur la grille (tables, listes). */
    pleineLargeur?: boolean;
  }

  const { titre, àCôté = null, children, pleineLargeur = false }: CarteProps = $props();
</script>

<section
  class="rounded-xl border border-slate-200 bg-white p-4 shadow-sm sm:p-5 dark:border-slate-800 dark:bg-slate-900
    {pleineLargeur ? 'sm:col-span-2 lg:col-span-3' : ''}"
>
  <header class="mb-3 flex items-center justify-between gap-2">
    <h2 class="text-sm font-semibold tracking-wide text-slate-500 uppercase dark:text-slate-400">
      {titre}
    </h2>
    {@render àCôté?.()}
  </header>
  {@render children()}
</section>
