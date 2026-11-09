package config

import (
	"strings"
	"testing"
)

func TestLoadValidation(t *testing.T) {
	jeux := map[string]struct {
		yaml      string
		mentionne string
	}{
		"refresh_interval absent": {
			yaml: `
sources:
  - name: provider-a
    m3u_url: https://exemple-a.tld/m3u
`,
			mentionne: "refresh_interval",
		},
		"refresh_interval nul": {
			yaml: `
refresh_interval: 0s
sources:
  - name: provider-a
    m3u_url: https://exemple-a.tld/m3u
`,
			mentionne: "refresh_interval",
		},
		"refresh_interval négatif": {
			yaml: `
refresh_interval: -1h
sources:
  - name: provider-a
    m3u_url: https://exemple-a.tld/m3u
`,
			mentionne: "refresh_interval",
		},
		"source sans nom": {
			yaml: `
refresh_interval: 6h
sources:
  - m3u_url: https://exemple-a.tld/m3u
`,
			mentionne: "name",
		},
		"source sans m3u_url": {
			yaml: `
refresh_interval: 6h
sources:
  - name: provider-a
`,
			mentionne: "m3u_url",
		},
		"deux sources homonymes": {
			yaml: `
refresh_interval: 6h
sources:
  - name: provider-a
    m3u_url: https://exemple-a.tld/m3u
  - name: provider-a
    m3u_url: https://exemple-b.tld/m3u
`,
			mentionne: "unicité",
		},
	}

	for nom, jeu := range jeux {
		t.Run(nom, func(t *testing.T) {
			fichier := ecrireTemp(t, jeu.yaml)
			_, err := Load(fichier)
			if err == nil {
				t.Fatalf("%s : Load a réussi alors qu'une erreur de validation était attendue", nom)
			}
			if !strings.Contains(err.Error(), jeu.mentionne) {
				t.Errorf("l'erreur %q doit mentionner %q", err.Error(), jeu.mentionne)
			}
		})
	}
}
