/* Formatage pur des valeurs /api/status, testé par Vitest : octets (unités
   binaires IEC), âges relatifs, horodatages RFC3339 → heure locale fr-FR. */

/** formatOctets — 4 432 512 → « 4,2 Mio » (unités binaires IEC).
    0 ou invalide → « — » : /api/status expose 0 quand VmRSS est illisible,
    le dashboard n'invente pas une valeur. */
export function formatOctets(octets: number | null | undefined): string {
  if (octets == null || !Number.isFinite(octets) || octets <= 0) {
    return '—';
  }
  if (octets < 1024) {
    return `${octets} o`;
  }
  const unités: Array<[number, string]> = [
    [1024 ** 3, 'Gio'],
    [1024 ** 2, 'Mio'],
    [1024, 'Kio'],
  ];
  for (const [facteur, unité] of unités) {
    if (octets >= facteur) {
      const valeur = octets / facteur;
      return `${valeur.toFixed(valeur >= 100 ? 0 : 1).replace('.', ',')} ${unité}`;
    }
  }
  return `${octets} o`;
}

/** formatDurée — un âge en secondes, français compact :
    0 → « 0 s », 59 → « 59 s », 120 → « 2 min », 3660 → « 1 h 1 min »,
    93 600 → « 1 j 2 h ». L'âge du snapshot est la première valeur que
    l'opérateur regarde : elle doit se lire d'un coup d'œil. */
export function formatDurée(secondes: number | null | undefined): string {
  if (secondes == null || !Number.isFinite(secondes) || secondes < 0) {
    return '—';
  }
  const ent = Math.floor(secondes);
  if (ent < 60) {
    return `${ent} s`;
  }
  if (ent < 3600) {
    return `${Math.floor(ent / 60)} min`;
  }
  if (ent < 86_400) {
    const heures = Math.floor(ent / 3600);
    const minutes = Math.floor((ent % 3600) / 60);
    return `${heures} h ${minutes} min`;
  }
  const jours = Math.floor(ent / 86_400);
  const heuresRestantes = Math.floor((ent % 86_400) / 3600);
  return hoursJours(jours, heuresRestantes);
}

function hoursJours(jours: number, heures: number): string {
  return heures > 0 ? `${jours} j ${heures} h` : `${jours} j`;
}

/** formatDuréeGo — une durée Go (ex. « 6h0m0s », « 90s », « 1h30m45.5s »)
    en français compact pour le résumé de config : 6h0m0s → « 6 h »,
    1h30m45s → « 1 h 30 min 45 s ». Contrairement à l'âge du snapshot, un
    intervalle n'a pas besoin de traîner ses composantes nulles (« 6 h »
    plutôt que « 6 h 0 min »). Hors format Go → valeur brute ; vide/absent → « — ». */
export function formatDuréeGo(duréeGo: string | null | undefined): string {
  if (duréeGo == null || duréeGo === '') {
    return '—';
  }
  const facteurs: Record<string, number> = { h: 3600, m: 60, s: 1, ms: 0.001, µs: 0.000001, ns: 1e-9 };
  let total = 0;
  let consommé = 0;
  for (const match of duréeGo.matchAll(/(\d+(?:\.\d+)?)(ms|µs|ns|h|m|s)/g)) {
    total += Number.parseFloat(match[1]) * facteurs[match[2]];
    consommé += match[0].length;
  }
  if (total < 0 || consommé !== duréeGo.length) {
    return duréeGo;
  }
  const parties: Array<string> = [];
  const jours = Math.floor(total / 86_400);
  const heures = Math.floor((total % 86_400) / 3600);
  const minutes = Math.floor((total % 3600) / 60);
  const secondes = total % 60;
  if (jours > 0) {
    parties.push(`${jours} j`);
  }
  if (heures > 0) {
    parties.push(`${heures} h`);
  }
  if (minutes > 0) {
    parties.push(`${minutes} min`);
  }
  if (parties.length === 0 && secondes < 1 && secondes > 0) {
    return `${secondes.toFixed(2).replace('.', ',')} s`;
  }
  if (secondes >= 1) {
    parties.push(`${Math.floor(secondes)} s`);
  }
  return parties.length > 0 ? parties.join(' ') : '0 s';
}

/** formatHorodatage — RFC3339 UTC → « 10/11/2025 08:42:05 » en Europe/Paris
    par défaut (fuseau injectable pour les tests). null/absent → « — » :
    last_ok=null et next_at=null sont des cas normaux du contrat. */
export function formatHorodatage(
  iso: string | null | undefined,
  timeZone = 'Europe/Paris',
): string {
  if (iso == null || iso === '') {
    return '—';
  }
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) {
    return '—';
  }
  return new Intl.DateTimeFormat('fr-FR', {
    dateStyle: 'short',
    timeStyle: 'medium',
    timeZone,
  }).format(date);
}
