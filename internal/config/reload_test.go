package config

import (
	"testing"
	"time"
)

func TestRelectureConserveDerniereBonne(t *testing.T) {
	original := ecrireTemp(t, `
refresh_interval: 10m
sources:
  - name: provider-a
    m3u_url: https://exemple-a.tld/m3u
`)

	loader := NewLoader(original)

	premiere, err := loader.Relire()
	if err != nil {
		t.Fatalf("première relecture : erreur inattendue : %v", err)
	}
	if premiere.RefreshInterval != 10*time.Minute || len(premiere.Sources) != 1 {
		t.Fatalf("première relecture = %+v, inattendue", premiere)
	}

	ecrire(t, original, `
refresh_interval: [pas une durée
`)

	deuxieme, err := loader.Relire()
	if err == nil {
		t.Fatal("relecture d'une config invalide : erreur attendue")
	}
	if deuxieme.RefreshInterval != 10*time.Minute || len(deuxieme.Sources) != 1 {
		t.Errorf("la relecture invalide doit conserver la dernière bonne config, obtenu %+v", deuxieme)
	}

	ecrire(t, original, `
refresh_interval: 30m
sources:
  - name: provider-a
    m3u_url: https://exemple-a.tld/m3u
`)

	troisieme, err := loader.Relire()
	if err != nil {
		t.Fatalf("relecture après correction : erreur inattendue : %v", err)
	}
	if troisieme.RefreshInterval != 30*time.Minute {
		t.Errorf("relecture après correction = %s, attendu 30m", troisieme.RefreshInterval)
	}
}
