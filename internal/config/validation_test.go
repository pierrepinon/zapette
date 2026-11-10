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
		// Le name sert de clé aux copies sur disque (sources/<name>.m3u|xmltv)
		// ET de provenance affichée au dashboard (issue #11) : un séparateur
		// de chemin ferait atterrir la copie ailleurs que dans sources/
		// (filepath.Join remonte sur ../x), « . »/« .. » n'ont aucun sens
		// comme clé. Trois familles invalides.
		"name avec barre oblique": {
			yaml: `
refresh_interval: 6h
sources:
  - name: ../x
    m3u_url: https://exemple-a.tld/m3u
`,
			mentionne: "invalide",
		},
		"name avec barre oblique inverse": {
			yaml: `
refresh_interval: 6h
sources:
  - name: "fournisseur\\exo"
    m3u_url: https://exemple-a.tld/m3u
`,
			mentionne: "invalide",
		},
		"name point seul": {
			yaml: `
refresh_interval: 6h
sources:
  - name: "."
    m3u_url: https://exemple-a.tld/m3u
`,
			mentionne: "invalide",
		},
		"name point-double": {
			yaml: `
refresh_interval: 6h
sources:
  - name: ".."
    m3u_url: https://exemple-a.tld/m3u
`,
			mentionne: "invalide",
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

// TestLoadNomsSourcesValides : les name SANS séparateur de chemin ni points
// nus sont acceptés, y compris avec des points internes ou une terminaison
// apparentée à un artefact (« prov.m3u » : l'extension d'artefact ajoutée
// par le store est toujours la DERNIÈRE, le nom y retourne tel quel).
func TestLoadNomsSourcesValides(t *testing.T) {
	for nom, name := range map[string]string{
		"simple":           "provider-a",
		"avec points":      "fhd.fr",
		"terminaison arte": "prov.m3u",
		"avec accent":      "chaînes-info",
	} {
		t.Run(nom, func(t *testing.T) {
			fichier := ecrireTemp(t, "refresh_interval: 6h\nsources:\n  - name: "+name+"\n    m3u_url: https://exemple-a.tld/m3u\n")
			cfg, err := Load(fichier)
			if err != nil {
				t.Fatalf("Load a refusé le name %q : %v", name, err)
			}
			if cfg.Sources[0].Name != name {
				t.Errorf("name %q perdu au passage : %q", name, cfg.Sources[0].Name)
			}
		})
	}
}
