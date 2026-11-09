package m3u

import (
	"strings"
	"testing"
)

// TestFiltrerVergulesQuotées vérifie que les attributs quotés contenant des
// virgules (ex. group-title="Cinéma, Séries") sont extraits correctement et
// que le match exact s'applique à la valeur quotée complète.
func TestFiltrerVirgulesQuotées(t *testing.T) {
	tests := []struct {
		nom              string
		playlist         string
		groupesAutorises []string
		attendu          string
	}{
		{
			nom: "group-title quoté contenant une virgule est retenu en entier",
			playlist: `#EXTM3U
#EXTINF:-1 tvg-id="cinema.fr" group-title="Cinéma, Séries",Films et séries
https://flux.exemple.tld/cinema.m3u8
`,
			groupesAutorises: []string{"Cinéma, Séries"},
			attendu: `#EXTM3U
#EXTINF:-1 tvg-id="cinema.fr" group-title="Cinéma, Séries",Films et séries
https://flux.exemple.tld/cinema.m3u8
`,
		},
		{
			nom: "le match reste exact malgré la virgule",
			playlist: `#EXTM3U
#EXTINF:-1 tvg-id="cinema.fr" group-title="Cinéma, Séries",Films et séries
https://flux.exemple.tld/cinema.m3u8
`,
			groupesAutorises: []string{"Cinéma"},
			attendu:          "#EXTM3U\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.nom, func(t *testing.T) {
			sortie := &strings.Builder{}
			if err := Filtrer(strings.NewReader(tc.playlist), sortie, tc.groupesAutorises); err != nil {
				t.Fatalf("Filtrer : %v", err)
			}
			if sortie.String() != tc.attendu {
				t.Errorf("sortie :\n%s\nattendu :\n%s", sortie.String(), tc.attendu)
			}
		})
	}
}

// TestIdentite vérifie l'identité de chaîne : tvg-id s'il est présent, sinon
// l'URL de flux (règle de dédoublonnage de la fusion des sources).
func TestIdentite(t *testing.T) {
	tests := []struct {
		nom     string
		chaine  Chaine
		attendu string
	}{
		{
			nom:     "tvg-id présent",
			chaine:  Chaine{TvgID: "bein1.fr", URLFlux: "https://flux.exemple.tld/bein1.m3u8"},
			attendu: "bein1.fr",
		},
		{
			nom:     "tvg-id absent = URL de flux",
			chaine:  Chaine{URLFlux: "https://flux.exemple.tld/sans.m3u8"},
			attendu: "https://flux.exemple.tld/sans.m3u8",
		},
	}
	for _, tc := range tests {
		t.Run(tc.nom, func(t *testing.T) {
			if obtenu := tc.chaine.Identite(); obtenu != tc.attendu {
				t.Errorf("Identite() = %q, attendu %q", obtenu, tc.attendu)
			}
		})
	}
}
