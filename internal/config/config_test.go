package config

import (
	"testing"
	"time"
)

func TestLoadYAMLValide(t *testing.T) {
	fichier := ecrireTemp(t, `
refresh_interval: 6h
allowed_groups: [Sports, News]
sources:
  - name: provider-a
    m3u_url: https://exemple-a.tld/m3u
    xmltv_url: https://exemple-a.tld/xmltv
  - name: provider-a-vod
    m3u_url: https://exemple-a-vod.tld/m3u
fetch_timeout: 60s
fetch_retries: 3
server_addr: "[::]:9999"
data_dir: ./donnees
public_ipv4_url: https://ipv4.exemple.tld
public_ipv6_url: https://ipv6.exemple.tld
`)

	cfg, err := Load(fichier)
	if err != nil {
		t.Fatalf("Load(%q) erreur inattendue : %v", fichier, err)
	}

	if cfg.RefreshInterval != 6*time.Hour {
		t.Errorf("RefreshInterval = %s, attendu 6h", cfg.RefreshInterval)
	}

	if len(cfg.AllowedGroups) != 2 || cfg.AllowedGroups[0] != "Sports" || cfg.AllowedGroups[1] != "News" {
		t.Errorf("AllowedGroups = %v, attendu [Sports News]", cfg.AllowedGroups)
	}

	if len(cfg.Sources) != 2 {
		t.Fatalf("Nombre de sources = %d, attendu 2", len(cfg.Sources))
	}
	if cfg.Sources[0].Name != "provider-a" ||
		cfg.Sources[0].M3UURL != "https://exemple-a.tld/m3u" ||
		cfg.Sources[0].XMLTVURL != "https://exemple-a.tld/xmltv" {
		t.Errorf("Source[0] = %+v, inattendue", cfg.Sources[0])
	}
	if cfg.Sources[1].Name != "provider-a-vod" || cfg.Sources[1].M3UURL != "https://exemple-a-vod.tld/m3u" {
		t.Errorf("Source[1] = %+v, inattendue", cfg.Sources[1])
	}
	if cfg.Sources[1].XMLTVURL != "" {
		t.Errorf("Source[1].XMLTVURL = %q, attendu vide", cfg.Sources[1].XMLTVURL)
	}

	if cfg.FetchTimeout != 60*time.Second {
		t.Errorf("FetchTimeout = %s, attendu 60s", cfg.FetchTimeout)
	}
	if cfg.FetchRetries != 3 {
		t.Errorf("FetchRetries = %d, attendu 3", cfg.FetchRetries)
	}
	if cfg.ServerAddr != "[::]:9999" {
		t.Errorf("ServerAddr = %q, attendu [::]:9999", cfg.ServerAddr)
	}
	if cfg.DataDir != "./donnees" {
		t.Errorf("DataDir = %q, attendu ./donnees", cfg.DataDir)
	}
	if cfg.PublicIPv4URL != "https://ipv4.exemple.tld" {
		t.Errorf("PublicIPv4URL = %q, attendu https://ipv4.exemple.tld", cfg.PublicIPv4URL)
	}
	if cfg.PublicIPv6URL != "https://ipv6.exemple.tld" {
		t.Errorf("PublicIPv6URL = %q, attendu https://ipv6.exemple.tld", cfg.PublicIPv6URL)
	}
}
