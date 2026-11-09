package m3u

import (
	"bytes"
	"embed"
	"strings"
	"testing"
)

//go:embed testdata/playlist.m3u
var fichiers embed.FS

// gerbePlaylist lit la playlist réduite mais réaliste de testdata.
// Cinq groupes (Sports, News, Films, Séries, Local), attribut url-tvg sur
// l'en-tête, noms affichés contenant des virgules et tvg-id de différents formats.
func gerbePlaylist(t *testing.T) string {
	t.Helper()
	brut, err := fichiers.ReadFile("testdata/playlist.m3u")
	if err != nil {
		t.Fatalf("lecture de testdata/playlist.m3u : %v", err)
	}
	return string(brut)
}

// lignesEntree extrait les lignes d'entrées (#EXTINF et URLs de flux) d'un
// contenu M3U.
func lignesEntree(contenu string) []string {
	var lignes []string
	for _, ligne := range strings.Split(contenu, "\n") {
		ligne := strings.TrimRight(ligne, "\r")
		if strings.HasPrefix(ligne, "#EXTINF") ||
			(ligne != "" && !strings.HasPrefix(ligne, "#")) {
			lignes = append(lignes, ligne)
		}
	}
	return lignes
}

// TestFiltrerFixture vérifie le filtre sur le fixture réaliste. Les lignes
// d'entrées émises doivent être un sous-ensemble exact, à l'octet près et
// dans l'ordre d'origine, des lignes de la playlist source : c'est la diff
// binaire exigée par les critères d'acceptation (URL de flux inchangées).
func TestFiltrerFixture(t *testing.T) {
	fixtureBrut := gerbePlaylist(t)
	lignesOrigine := lignesEntree(fixtureBrut)

	tests := []struct {
		nom              string
		groupesAutorises []string
		entriesAttendus  int // chaînes attendues, soit deux lignes (EXTINF + URL) chacune
	}{
		{"liste vide = tout conserver", []string{}, 5},
		{"sports seul", []string{"Sports"}, 1},
		{"news et local", []string{"News", "Local"}, 2},
		{"aucun groupe présent dans la playlist", []string{"Inexistant"}, 0},
	}

	for _, tc := range tests {
		t.Run(tc.nom, func(t *testing.T) {
			sortie := &bytes.Buffer{}
			if err := Filtrer(strings.NewReader(fixtureBrut), sortie, tc.groupesAutorises); err != nil {
				t.Fatalf("Filtrer : %v", err)
			}
			obtenu := sortie.String()

			if !strings.HasPrefix(obtenu, "#EXTM3U") {
				t.Error("l'émission doit commencer par #EXTM3U")
			}
			lignesObtenues := lignesEntree(obtenu)
			entries := len(lignesObtenues) / 2
			if entries != tc.entriesAttendus {
				t.Errorf("%d entrées émises, attendu %d :\n%s", entries, tc.entriesAttendus, obtenu)
			}
			// Sous-ensemble dans l'ordre : chaque ligne émise doit être
			// identique à une ligne d'origine, à l'octet près.
			position := 0
			for _, ligne := range lignesObtenues {
				trouvee := false
				for i := position; i < len(lignesOrigine); i++ {
					if lignesOrigine[i] == ligne {
						position = i + 1
						trouvee = true
						break
					}
				}
				if !trouvee {
					t.Errorf("ligne émise non retrouvée à l'octet près dans l'original : %q", ligne)
				}
			}
		})
	}
}
