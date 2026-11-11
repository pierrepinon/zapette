package tnt

import (
	"strconv"
	"testing"
)

// TestNumeroTrouve : les tvg-id publiés par le fournisseur (casse
// variable) correspondent tous au même numéro officiel.
func TestNumeroTrouve(t *testing.T) {
	cas := []struct {
		tvgID  string
		numero string
	}{
		{"TF1.fr", "1"},
		{"tf1.fr", "1"},
		{"france2.fr", "2"},
		{"arte.fr", "7"},
		{"CNews.fr", "14"},
		{"CStar.fr", "17"},
		{"tf1seriesfilms.fr", "20"},
		{"lequipe21.fr", "21"},
		{"6ter.fr", "22"},
		{"rmcstory.fr", "23"},
	}
	for _, c := range cas {
		numero, ok := Numero(c.tvgID)
		if !ok {
			t.Errorf("%s : attendu au plan, le tvg-id est inconnu", c.tvgID)
			continue
		}
		if numero != c.numero {
			t.Errorf("%s : numéro %q, attendu %q", c.tvgID, numero, c.numero)
		}
	}
}

// TestNumeroInconnu : un tvg-id hors plan ne reçoit JAMAIS de numéro
// implicite — le repli appartient à l'appelant, pas au plan.
func TestNumeroInconnu(t *testing.T) {
	for _, tvgID := range []string{"", "bfm.fr", "rmcsport1.fr", "TFX.fr ", "viaVosges.ch"} {
		if _, ok := Numero(tvgID); ok {
			t.Errorf("%s : tvg-id inconnu, un numéro a été retourné", tvgID)
		}
	}
}

// TestPlanSansDoublonNiSurnombre : des numéros uniques, tous dans la
// ressource officielle 1..26 — la table ne doit jamais dériver (27+ serait
// un bug de maintenance, une collision casserait la lisibilité du
// dashboard).
func TestPlanSansDoublonNiSurnombre(t *testing.T) {
	vus := map[string]string{}
	for tvgID, numero := range plan {
		if ancien, vu := vus[numero]; vu {
			t.Errorf("numéro %s attribué deux fois (%s et %s)", numero, ancien, tvgID)
			continue
		}
		vus[numero] = tvgID
		n, err := strconv.Atoi(numero)
		if err != nil {
			t.Errorf("%s : numéro %s non entier", tvgID, numero)
			continue
		}
		if n < 1 || n > 26 {
			t.Errorf("%s : numéro %s hors de la plage TNT 1..26", tvgID, numero)
		}
	}
	if PremierHorsPlan() != 27 {
		t.Errorf("PremierHorsPlan = %d, attendu 27 (après le canal 26)", PremierHorsPlan())
	}
}
