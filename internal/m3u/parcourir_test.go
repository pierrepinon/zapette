package m3u

import (
	"strings"
	"testing"
)

// TestParcourir vérifie le parcours des entrées d'une playlist : chaque
// chaîne (identité, attributs, URL de flux) est visitée dans l'ordre,
// sans charger la playlist en mémoire. C'est le point d'entrée utilisé
// par la fusion/dédup des sources.
func TestParcourir(t *testing.T) {
	playlist := `#EXTM3U
#EXTINF:-1 tvg-id="bein1.fr" group-title="Sports",beIN Sports 1
https://flux.exemple.tld/bein1.m3u8
#EXTINF:-1 tvg-name="Sans identité"
https://flux.exemple.tld/sans.m3u8
#EXTINF:-1 tvg-id="arte.fr",Arte, la chaîne
https://flux.exemple.tld/arte.m3u8
`

	attendues := []Chaine{
		{
			TvgID:   "bein1.fr",
			Groupe:  "Sports",
			Nom:     "beIN Sports 1",
			URLFlux: "https://flux.exemple.tld/bein1.m3u8",
		},
		{
			TvgID:   "",
			URLFlux: "https://flux.exemple.tld/sans.m3u8",
		},
		{
			TvgID:   "arte.fr",
			Nom:     "Arte, la chaîne",
			URLFlux: "https://flux.exemple.tld/arte.m3u8",
		},
	}

	var obtenues []Chaine
	if err := Parcourir(strings.NewReader(playlist), func(c Chaine) error {
		obtenues = append(obtenues, c)
		return nil
	}); err != nil {
		t.Fatalf("Parcourir : %v", err)
	}

	if len(obtenues) != len(attendues) {
		t.Fatalf("obtenu %d chaînes, attendu %d", len(obtenues), len(attendues))
	}
	for i, attendue := range attendues {
		if obtenues[i] != attendue {
			t.Errorf("chaîne %d :\n  obtenu  %+v\n  attendu %+v", i, obtenues[i], attendue)
		}
		if identiteAttendue := attendue.Identite(); obtenues[i].Identite() != identiteAttendue {
			t.Errorf("chaîne %d : identité %q, attendu %q",
				i, obtenues[i].Identite(), identiteAttendue)
		}
	}
}
