package m3u

import (
	"strings"
	"testing"
)

// TestBalayerFusionPremierGagne simule l'usage du ticket fetch/store : fusion
// et déduplication first-wins de deux copies de source en un seul passage, en
// réémontant les lignes brutes à l'octet près (en-tête du premier arrivé, les
// suivants ignorés).
func TestBalayerFusionPremierGagne(t *testing.T) {
	sourceA := `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" group-title="Sports",Arte
https://flux.exemple.tld/arte.m3u8
#EXTINF:-1 tvg-id="bfm.fr" group-title="News",BFM
https://flux.exemple.tld/bfm.m3u8
`
	sourceB := `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" group-title="Sports",Arte
https://miroir.tld/arte.m3u8
#EXTINF:-1 tvg-id="cnews.fr" group-title="News",CNews
https://miroir.tld/cnews.m3u8
`

	vues := map[string]bool{}
	sortie := &strings.Builder{}
	enteteEcrit := false

	recevoirEntete := func(brute string) error {
		if enteteEcrit {
			return nil
		}
		enteteEcrit = true
		_, err := sortie.WriteString(brute)
		return err
	}
	recevoirEntree := func(c Chaine, lignes []string) error {
		if vues[c.Identite()] {
			return nil
		}
		vues[c.Identite()] = true
		for _, ligne := range lignes {
			if _, err := sortie.WriteString(ligne); err != nil {
				return err
			}
		}
		return nil
	}

	// Un seul passage par source, dans l'ordre des sources d'origine.
	for _, source := range []string{sourceA, sourceB} {
		if err := Balayer(strings.NewReader(source), recevoirEntete, recevoirEntree); err != nil {
			t.Fatalf("Balayer : %v", err)
		}
	}

	attendu := `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" group-title="Sports",Arte
https://flux.exemple.tld/arte.m3u8
#EXTINF:-1 tvg-id="bfm.fr" group-title="News",BFM
https://flux.exemple.tld/bfm.m3u8
#EXTINF:-1 tvg-id="cnews.fr" group-title="News",CNews
https://miroir.tld/cnews.m3u8
`
	if sortie.String() != attendu {
		t.Errorf("fusion dédup, sortie :\n%s\nattendu :\n%s", sortie.String(), attendu)
	}
}

// TestBalayerAttributsPublication épingle l'exposition des attributs du
// ticket « page Chaînes » (#11) : tvg-chno et tvg-logo sont analysés comme
// les attributs existants (tvg-id, group-title), sans que le consommateur
// (le store) ne repare la ligne #EXTINF à la main.
func TestBalayerAttributsPublication(t *testing.T) {
	source := `#EXTM3U
#EXTINF:-1 tvg-id="arte.fr" tvg-chno="4" tvg-logo="https://img.tld/arte.png" group-title="Généralistes",Arte
https://flux.tld/arte.m3u8
#EXTINF:-1 tvg-chno="" tvg-logo="" group-title="Local",Sans
https://flux.tld/sans.m3u8
#EXTINF:-1 tvg-id="nue.fr",Nue
https://flux.tld/nue.m3u8
`

	var reçues []Chaine
	if err := Balayer(strings.NewReader(source), nil, func(c Chaine, _ []string) error {
		reçues = append(reçues, c)
		return nil
	}); err != nil {
		t.Fatalf("Balayer : %v", err)
	}

	if len(reçues) != 3 {
		t.Fatalf("3 chaînes attendues, obtenu %d", len(reçues))
	}

	cas := []struct {
		nom     string
		tvgChno string
		tvgLogo string
		tvgID   string
		groupe  string
		nomAff2 string
	}{
		{"Arte", "4", "https://img.tld/arte.png", "arte.fr", "Généralistes", "Arte"},
		{"Sans", "", "", "", "Local", "Sans"},
		{"Nue", "", "", "nue.fr", "", "Nue"},
	}
	for i, c := range cas {
		if reçues[i].TvgChno != c.tvgChno {
			t.Errorf("%s : TvgChno %q, attendu %q", c.nom, reçues[i].TvgChno, c.tvgChno)
		}
		if reçues[i].TvgLogo != c.tvgLogo {
			t.Errorf("%s : TvgLogo %q, attendu %q", c.nom, reçues[i].TvgLogo, c.tvgLogo)
		}
		if reçues[i].TvgID != c.tvgID {
			t.Errorf("%s : TvgID %q, attendu %q", c.nom, reçues[i].TvgID, c.tvgID)
		}
		if reçues[i].Groupe != c.groupe {
			t.Errorf("%s : Groupe %q, attendu %q", c.nom, reçues[i].Groupe, c.groupe)
		}
		if reçues[i].Nom != c.nomAff2 {
			t.Errorf("%s : Nom %q, attendu %q", c.nom, reçues[i].Nom, c.nomAff2)
		}
	}
}
