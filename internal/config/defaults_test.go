package config

import (
	"testing"
	"time"
)

func TestLoadValeursParDefaut(t *testing.T) {
	fichier := ecrireTemp(t, `
refresh_interval: 6h
sources:
  - name: provider-a
    m3u_url: https://exemple-a.tld/m3u
`)

	cfg, err := Load(fichier)
	if err != nil {
		t.Fatalf("Load(%q) erreur inattendue : %v", fichier, err)
	}
	if cfg.ServerAddr != "[::]:8080" {
		t.Errorf("ServerAddr = %q, attendu [::]:8080", cfg.ServerAddr)
	}
	if cfg.DataDir != "./data" {
		t.Errorf("DataDir = %q, attendu ./data", cfg.DataDir)
	}
	if cfg.PublicIPv4URL != "https://api4.ipify.org" {
		t.Errorf("PublicIPv4URL = %q, attendu https://api4.ipify.org", cfg.PublicIPv4URL)
	}
	if cfg.PublicIPv6URL != "https://api6.ipify.org" {
		t.Errorf("PublicIPv6URL = %q, attendu https://api6.ipify.org", cfg.PublicIPv6URL)
	}
	if cfg.FetchTimeout != 60*time.Second {
		t.Errorf("FetchTimeout = %s, attendu 60s", cfg.FetchTimeout)
	}
	if cfg.FetchRetries != 3 {
		t.Errorf("FetchRetries = %d, attendu 3", cfg.FetchRetries)
	}
	if len(cfg.AllowedGroups) != 0 {
		t.Errorf("AllowedGroups = %v, attendu vide (= tout retenir)", cfg.AllowedGroups)
	}
}
