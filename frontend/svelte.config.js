// Configuration Svelte : préprocesseur TS pour les composants
// `<script lang="ts">`, plus rien d'autre — l'application est une seule
// page, pas un projet SvelteKit.
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';

export default {
  preprocess: vitePreprocess(),
};
