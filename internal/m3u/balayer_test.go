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
