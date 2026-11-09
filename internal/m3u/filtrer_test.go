package m3u

import (
	"strings"
	"testing"
)

// TestFiltrer vérifie le filtre streaming : match exact et case-sensitive sur
// group-title, liste vide = tout conserver, entrée sans group-title jamais
// retenue hors liste vide, en-tête #EXTM3U systématique.
func TestFiltrer(t *testing.T) {
	entete := `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
`
	groupeSports := `#EXTINF:-1 tvg-id="bein1.fr" tvg-name="beIN Sports 1" tvg-logo="https://logo.exemple.tld/bein1.png" group-title="Sports",beIN Sports 1
https://flux.exemple.tld/bein1.m3u8
`
	groupeNews := `#EXTINF:-1 tvg-id="bfm.fr" tvg-name="BFM TV" group-title="News",BFM TV
https://flux.exemple.tld/bfm.m3u8
`
	groupeFilms := `#EXTINF:-1 tvg-id="film1.fr" tvg-name="Films HD" group-title="Films",Films HD
https://flux.exemple.tld/film1.m3u8
`
	// Entrée sans group-title : jamais retenue hors liste vide.
	sansGroupe := `#EXTINF:-1 tvg-name="Sans groupe" tvg-id="sans.fr"
https://flux.exemple.tld/sans.m3u8
`

	tests := []struct {
		nom              string
		playlist         string
		groupesAutorises []string
		attendu          string
	}{
		{
			nom:              "liste vide = tout conserver",
			playlist:         entete + groupeSports + groupeNews + groupeFilms,
			groupesAutorises: []string{},
			attendu:          entete + groupeSports + groupeNews + groupeFilms,
		},
		{
			nom:              "un groupe exact",
			playlist:         entete + groupeSports + groupeNews + groupeFilms,
			groupesAutorises: []string{"News"},
			attendu:          entete + groupeNews,
		},
		{
			nom:              "deux groupes exacts",
			playlist:         entete + groupeSports + groupeNews + groupeFilms,
			groupesAutorises: []string{"Sports", "Films"},
			attendu:          entete + groupeSports + groupeFilms,
		},
		{
			nom:              "match case-sensitive",
			playlist:         entete + groupeSports + groupeNews + groupeFilms,
			groupesAutorises: []string{"sports"},
			attendu:          entete,
		},
		{
			nom:              "entrée sans group-title jamais retenue hors liste vide",
			playlist:         entete + sansGroupe,
			groupesAutorises: []string{"Sports"},
			attendu:          entete,
		},
		{
			nom:              "entrée sans group-title conservée si liste vide",
			playlist:         entete + sansGroupe,
			groupesAutorises: nil,
			attendu:          entete + sansGroupe,
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
