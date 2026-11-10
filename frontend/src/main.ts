// Point d'entrée : monte la page unique. lang="fr" et la classe .dark sont
// déjà posés par index.html (prévention du flash de thème).
import { mount } from 'svelte';
import App from './App.svelte';
import './app.css';

const racine = document.querySelector('#app');
if (!racine) {
  throw new Error('#app introuvable dans index.html');
}

mount(App, { target: racine });
