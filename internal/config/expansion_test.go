package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoadExpansionEnvironnement(t *testing.T) {
	t.Setenv("ZAPETTE_Test_M3U_URL", "https://secret.tld/m3u")
	t.Setenv("ZAPETTE_Test_XMLTV_URL", "https://secret.tld/xmltv")
	t.Setenv("ZAPETTE_Test_GROUPE", "Sports")
	t.Setenv("ZAPETTE_Test_INTERVALLE", "6h")
	t.Setenv("ZAPETTE_Test_RETRIES", "3")
	t.Setenv("ZAPETTE_Test_VALEUR_NUMERIQUE", "8080")

	fichier := ecrireTemp(t, `
refresh_interval: ${ZAPETTE_Test_INTERVALLE}
allowed_groups: ["${ZAPETTE_Test_GROUPE}"]
sources:
  - name: provider-a
    m3u_url: ${ZAPETTE_Test_M3U_URL}
    xmltv_url: ${ZAPETTE_Test_XMLTV_URL}
fetch_retries: ${ZAPETTE_Test_RETRIES}
server_addr: "${ZAPETTE_Test_VALEUR_NUMERIQUE}"
`)

	cfg, err := Load(fichier)
	if err != nil {
		t.Fatalf("Load(%q) erreur inattendue : %v", fichier, err)
	}
	if cfg.RefreshInterval != 6*time.Hour {
		t.Errorf("RefreshInterval = %s, attendu 6h", cfg.RefreshInterval)
	}
	if cfg.FetchRetries != 3 {
		t.Errorf("FetchRetries = %d, attendu 3 (type réinféré)", cfg.FetchRetries)
	}
	if cfg.ServerAddr != "8080" {
		t.Errorf("ServerAddr = %q, attendu \"8080\" comme chaîne (référence quotée)", cfg.ServerAddr)
	}
	if len(cfg.AllowedGroups) != 1 || cfg.AllowedGroups[0] != "Sports" {
		t.Errorf("AllowedGroups = %v, attendu [Sports]", cfg.AllowedGroups)
	}
	if cfg.Sources[0].M3UURL != "https://secret.tld/m3u" {
		t.Errorf("Source[0].M3UURL = %q, attendu https://secret.tld/m3u", cfg.Sources[0].M3UURL)
	}
	if cfg.Sources[0].XMLTVURL != "https://secret.tld/xmltv" {
		t.Errorf("Source[0].XMLTVURL = %q, attendu https://secret.tld/xmltv", cfg.Sources[0].XMLTVURL)
	}
}

func TestLoadVariableIndefinie(t *testing.T) {
	fichier := ecrireTemp(t, `
refresh_interval: 6h
sources:
  - name: provider-a
    m3u_url: ${ZAPETTE_Test_URL_MANQUANTE}
`)

	_, err := Load(fichier)
	if err == nil {
		t.Fatal("Load a réussi alors que la variable est indéfinie, erreur attendue")
	}
	if !strings.Contains(err.Error(), "ZAPETTE_Test_URL_MANQUANTE") {
		t.Errorf("l'erreur %q doit nommer la variable indéfinie", err.Error())
	}
}
