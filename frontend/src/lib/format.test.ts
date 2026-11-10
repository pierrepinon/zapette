import { describe, expect, it } from 'vitest';
import { formatDurée, formatDuréeGo, formatHorodatage, formatOctets } from './format';

describe('formatOctets', () => {
  it('retourne « — » pour null, zéro ou invalide (VmRSS illisible)', () => {
    expect(formatOctets(null)).toBe('—');
    expect(formatOctets(undefined)).toBe('—');
    expect(formatOctets(0)).toBe('—');
    expect(formatOctets(-12)).toBe('—');
    expect(formatOctets(Number.NaN)).toBe('—');
  });

  it('affiche les octets bruts en dessous du Kio', () => {
    expect(formatOctets(1)).toBe('1 o');
    expect(formatOctets(1023)).toBe('1023 o');
  });

  it('formate en unités binaires IEC avec une décimale française', () => {
    expect(formatOctets(1024)).toBe('1,0 Kio');
    expect(formatOctets(4_432_512)).toBe('4,2 Mio');
    expect(formatOctets(1024 ** 3 * 1.5)).toBe('1,5 Gio');
  });

  it('arrondit sans décimale au-delà de 100 (les gros snapshots lisibles)', () => {
    expect(formatOctets(1024 ** 2 * 123.4)).toBe('123 Mio');
  });
});

describe('formatDurée', () => {
  it('retourne « — » pour null ou invalide (pas de snapshot)', () => {
    expect(formatDurée(null)).toBe('—');
    expect(formatDurée(undefined)).toBe('—');
    expect(formatDurée(-1)).toBe('—');
  });

  it('affiche les secondes en dessous d’une minute', () => {
    expect(formatDurée(0)).toBe('0 s');
    expect(formatDurée(59)).toBe('59 s');
  });

  it('affiche les minutes en dessous d’une heure', () => {
    expect(formatDurée(60)).toBe('1 min');
    expect(formatDurée(3599)).toBe('59 min');
  });

  it('affiche les heures + minutes en dessous d’un jour', () => {
    expect(formatDurée(3600)).toBe('1 h 0 min');
    expect(formatDurée(3660)).toBe('1 h 1 min');
    expect(formatDurée(86_399)).toBe('23 h 59 min');
  });

  it('affiche les jours + heures au-delà', () => {
    expect(formatDurée(86_400)).toBe('1 j');
    expect(formatDurée(93_600)).toBe('1 j 2 h');
    expect(formatDurée(86_400 * 3)).toBe('3 j');
  });
});

describe('formatDuréeGo', () => {
  it('retourne « — » pour vide ou absent', () => {
    expect(formatDuréeGo(null)).toBe('—');
    expect(formatDuréeGo(undefined)).toBe('—');
    expect(formatDuréeGo('')).toBe('—');
  });

  it('ébrouille les composantes nulles (« 6h0m0s » → « 6 h »)', () => {
    expect(formatDuréeGo('6h0m0s')).toBe('6 h');
    expect(formatDuréeGo('1h0m30s')).toBe('1 h 30 s');
    expect(formatDuréeGo('90m0s')).toBe('1 h 30 min');
  });

  it('forme le circuit complet j/h/min/s', () => {
    expect(formatDuréeGo('27h0m0s')).toBe('1 j 3 h');
    expect(formatDuréeGo('1h30m45.5s')).toBe('1 h 30 min 45 s');
    expect(formatDuréeGo('90s')).toBe('1 min 30 s');
  });

  it('formate les sous-secondes et zéro', () => {
    expect(formatDuréeGo('0.5s')).toBe('0,50 s');
    expect(formatDuréeGo('0s')).toBe('0 s');
    expect(formatDuréeGo('500ms')).toBe('0,50 s');
  });

  it('renvoie la valeur brute sur un format non Go', () => {
    expect(formatDuréeGo('bientôt')).toBe('bientôt');
    expect(formatDuréeGo('6 h')).toBe('6 h');
  });
});

describe('formatHorodatage', () => {
  it('retourne « — » pour null, vide ou non-date', () => {
    expect(formatHorodatage(null)).toBe('—');
    expect(formatHorodatage(undefined)).toBe('—');
    expect(formatHorodatage('')).toBe('—');
    expect(formatHorodatage('pas une date')).toBe('—');
  });

  it('convertit un RFC3339 UTC en heure locale fr-FR (fuseau injecté)', () => {
    expect(formatHorodatage('2025-11-10T07:42:05Z', 'Europe/Paris')).toBe(
      '10/11/2025 08:42:05',
    );
    expect(formatHorodatage('2025-11-10T07:42:05Z', 'UTC')).toBe('10/11/2025 07:42:05');
  });
});
