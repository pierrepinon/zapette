<script lang="ts">
  // Bascule de thème : segmented control system / light / dark (valeurs du
  // contrat localStorage). Par défaut et à l'ouverture : « system » — le
  // mode suit alors prefers-color-scheme, y compris dans la session.
  import { onMount } from 'svelte';
  import { créerContrôleurThème, type PréférenceTheme } from '../theme';

  const options: Array<{ valeur: PréférenceTheme; libellé: string }> = [
    { valeur: 'system', libellé: 'Système' },
    { valeur: 'light', libellé: 'Clair' },
    { valeur: 'dark', libellé: 'Sombre' },
  ];

  let préférence = $state<PréférenceTheme>('system');
  let contrôleur: ReturnType<typeof créerContrôleurThème> | null = null;

  onMount(() => {
    contrôleur = créerContrôleurThème();
    préférence = contrôleur.préférence;
    return () => {
      contrôleur?.détruire();
      contrôleur = null;
    };
  });

  function choisir(nouvelle: PréférenceTheme): void {
    contrôleur?.définir(nouvelle);
    préférence = nouvelle;
  }
</script>

<div
  role="group"
  aria-label="Choix du thème"
  class="inline-flex overflow-hidden rounded-lg border border-slate-300 dark:border-slate-700"
>
  {#each options as option (option.valeur)}
    <button
      type="button"
      aria-pressed={préférence === option.valeur}
      onclick={() => choisir(option.valeur)}
      class="px-3 py-1.5 text-sm font-medium transition-colors
        {préférence === option.valeur
          ? 'bg-indigo-600 text-white dark:bg-indigo-500'
          : 'bg-white text-slate-700 hover:bg-slate-100 dark:bg-slate-900 dark:text-slate-300 dark:hover:bg-slate-800'}"
    >
      {option.libellé}
    </button>
  {/each}
</div>
