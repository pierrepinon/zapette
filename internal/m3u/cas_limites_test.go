package m3u

import (
	"strings"
	"testing"
)

// TestFiltrerCasLimites vérifie le comportement du filtre hors cas nominaux.
func TestFiltrerCasLimites(t *testing.T) {
	tests := []struct {
		nom              string
		playlist         string
		groupesAutorises []string
		attendu          string
	}{
		{
			nom: "fins de ligne CRLF émises telles quelles",
			playlist: "#EXTM3U\r\n" +
				"#EXTINF:-1 tvg-id=\"a.fr\" group-title=\"Sports\",A\r\n" +
				"https://flux.exemple.tld/a.m3u8\r\n",
			groupesAutorises: []string{"Sports"},
			attendu: "#EXTM3U\r\n" +
				"#EXTINF:-1 tvg-id=\"a.fr\" group-title=\"Sports\",A\r\n" +
				"https://flux.exemple.tld/a.m3u8\r\n",
		},
		{
			nom: "directives de flux conservées avec l'entrée",
			playlist: `#EXTM3U
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
#EXTVLCOPT:http-user-agent=zapette
#EXTVLCOPT:http-referrer=https://exemple.tld
https://flux.exemple.tld/a.m3u8
`,
			groupesAutorises: []string{"Sports"},
			attendu: `#EXTM3U
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
#EXTVLCOPT:http-user-agent=zapette
#EXTVLCOPT:http-referrer=https://exemple.tld
https://flux.exemple.tld/a.m3u8
`,
		},
		{
			nom: "lignes orphelines ignorées",
			playlist: `#EXTM3U
https://orphelin.exemple.tld/sans-extinf.m3u8
#EXTVIP:mystère
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
https://flux.exemple.tld/a.m3u8
https://orphelin.exemple.tld/derrière.m3u8
`,
			groupesAutorises: []string{"Sports"},
			attendu: `#EXTM3U
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
https://flux.exemple.tld/a.m3u8
`,
		},
		{
			nom: "en-tête #EXTM3U synthétisé s'il est absent",
			playlist: `#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
https://flux.exemple.tld/a.m3u8
`,
			groupesAutorises: []string{"Sports"},
			attendu: `#EXTM3U
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
https://flux.exemple.tld/a.m3u8
`,
		},
		{
			nom: "entrée sans URL en fin de fichier ignorée",
			playlist: `#EXTM3U
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
https://flux.exemple.tld/a.m3u8
#EXTINF:-1 tvg-id="b.fr" group-title="Sports",B`,
			groupesAutorises: []string{"Sports"},
			attendu: `#EXTM3U
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
https://flux.exemple.tld/a.m3u8
`,
		},
		{
			nom: "ligne vide ignorée",
			playlist: `#EXTM3U

#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A

https://flux.exemple.tld/a.m3u8

`,
			groupesAutorises: []string{"Sports"},
			attendu: `#EXTM3U
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
https://flux.exemple.tld/a.m3u8
`,
		},
		{
			nom: "en-tête #EXTM3U redondant ignoré",
			playlist: `#EXTM3U
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
#EXTM3U
https://flux.exemple.tld/a.m3u8
`,
			groupesAutorises: []string{"Sports"},
			attendu: `#EXTM3U
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
https://flux.exemple.tld/a.m3u8
`,
		},
		{
			nom: "dernière entrée sans saut de ligne final émise telle quelle",
			playlist: "#EXTM3U\n" +
				"#EXTINF:-1 tvg-id=\"a.fr\" group-title=\"Sports\",A\n" +
				"https://flux.exemple.tld/a.m3u8",
			groupesAutorises: []string{"Sports"},
			attendu: "#EXTM3U\n" +
				"#EXTINF:-1 tvg-id=\"a.fr\" group-title=\"Sports\",A\n" +
				"https://flux.exemple.tld/a.m3u8",
		},
	}

	for _, tc := range tests {
		t.Run(tc.nom, func(t *testing.T) {
			sortie := &strings.Builder{}
			if err := Filtrer(strings.NewReader(tc.playlist), sortie, tc.groupesAutorises); err != nil {
				t.Fatalf("Filtrer : %v", err)
			}
			if sortie.String() != tc.attendu {
				t.Errorf("sortie :\n%q\nattendu :\n%q", sortie.String(), tc.attendu)
			}
		})
	}
}

// TestParcourirCasLimites vérifie que Parcourir ignore lui aussi les lignes
// orphelines : la déduplication des sources ne doit compter que des chaînes
// complètes.
func TestParcourirCasLimites(t *testing.T) {
	playlist := `#EXTM3U
https://orphelin.exemple.tld/sans-extinf.m3u8
#EXTINF:-1 tvg-id="a.fr" group-title="Sports",A
https://flux.exemple.tld/a.m3u8
`

	var chaines []Chaine
	if err := Parcourir(strings.NewReader(playlist), func(c Chaine) error {
		chaines = append(chaines, c)
		return nil
	}); err != nil {
		t.Fatalf("Parcourir : %v", err)
	}
	if len(chaines) != 1 {
		t.Fatalf("obtenu %d chaînes, attendu 1", len(chaines))
	}
	if idAttendu := "a.fr"; chaines[0].Identite() != idAttendu {
		t.Errorf("identité %q, attendu %q", chaines[0].Identite(), idAttendu)
	}
}
