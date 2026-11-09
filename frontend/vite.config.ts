// Configuration Vite : Svelte 5 (runes) + Tailwind CSS v4, un seul point
// d'entrée, aucun réseau au runtime — tout est embarqué dans le binaire Go
// à la fin de `vite build`. Vitest partage la config (environnement jsdom
// pour les tests du thème, store en environnement DOM neutre).
import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  plugins: [svelte(), tailwindcss()],
  build: {
    target: 'es2022',
    sourcemap: false,
    outDir: 'dist',
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts', 'test/**/*.test.ts'],
  },
});
