package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/fetch"
	"github.com/pierrepinon/zapette/internal/store"
)

// Matériaux e2e : un fournisseur fake (httptest) qui publie une playlist et
// un guide réduits mais réels. allowed_groups vide ⇒ tout est conservé :
// le snapshot M3U servi est alors la copie du fournisseur, octet pour octet.
const (
	m3uPourE2E = `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" group-title="General",Arte
https://flux.exemple.tld/arte.m3u8
#EXTINF:-1 tvg-id="bfm.fr" group-title="News",BFM
https://flux.exemple.tld/bfm.m3u8
`
	xmltvPourE2E = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="arte.fr"><display-name>Arte</display-name></channel>
  <programme start="20250101120000 +0100" channel="arte.fr"><title>Journal</title></programme>
</tv>
`
)

// fournir mocke le fournisseur : un upstream httptest par artefact, URLs
// injectées dans la source de config retournée.
func fournir(t *testing.T, m3u, xmltv string) config.Source {
	t.Helper()
	publier := func(corps string) string {
		serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(corps))
		}))
		t.Cleanup(serveur.Close)
		return serveur.URL
	}
	return config.Source{
		Name:     "arte",
		M3UURL:   publier(m3u),
		XMLTVURL: publier(xmltv),
	}
}

// configE2E : config pleine pour les sources fournies, groupes vides (tout
// conserver), data_dir isolé dans un répertoire temporaire.
func configE2E(t *testing.T, sources ...config.Source) config.Config {
	t.Helper()
	return config.Config{
		RefreshInterval: time.Hour,
		AllowedGroups:   nil,
		Sources:         sources,
		FetchTimeout:    2 * time.Second,
		FetchRetries:    0,
		ServerAddr:      "[::]:8080",
		DataDir:         t.TempDir(),
	}
}

// passerUneFois lance une passe de fetch réelle contre les fournisseurs de
// la config : le snapshot et meta.json finissent sur disque exactement
// comme après un tick de production.
func passerUneFois(t *testing.T, cfg config.Config) {
	t.Helper()
	boutique, err := store.Ouvrir(cfg.DataDir)
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	o := fetch.Nouveau(boutique, fetch.NouveauFetcher, fetch.RetourArriereDefaut)
	if err := o.Passe(context.Background(), cfg); err != nil {
		t.Fatalf("passe de fetch : %v", err)
	}
}

// E2E démarre le serveur (handlers réels) in-process sur le data_dir de la
// config : la couture de test du ticket — serveur entier derrière httptest,
// upstream compris, aucune dépendance au réseau réel.
func E2E(t *testing.T, cfg config.Config) *httptest.Server {
	t.Helper()
	boutique, err := store.Ouvrir(cfg.DataDir)
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	recette := httptest.NewServer(Nouveau(boutique).Handler())
	t.Cleanup(recette.Close)
	return recette
}

// obtenir fait un GET réel et retourne (code, en-têtes, corps).
func obtenir(t *testing.T, url string) (int, http.Header, []byte) {
	t.Helper()
	reponse, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s : %v", url, err)
	}
	defer reponse.Body.Close()
	corps, err := io.ReadAll(reponse.Body)
	if err != nil {
		t.Fatalf("lecture de %s : %v", url, err)
	}
	return reponse.StatusCode, reponse.Header, corps
}

// TestFroidSansSnapshot : au démarrage, avant la première passe réussie,
// aucun snapshot n'existe sur disque. /m3u et /xmltv répondent 503 avec un
// corps JSON explicite — l'échec doit être visible, jamais silencieux
// (story 10) — et GET / reste accessible.
func TestFroidSansSnapshot(t *testing.T) {
	recette := E2E(t, configE2E(t))

	for _, artefact := range []string{"/m3u", "/xmltv"} {
		code, entetes, corps := obtenir(t, recette.URL+artefact)
		if code != http.StatusServiceUnavailable {
			t.Errorf("%s : code %d, voulu 503", artefact, code)
		}
		if type_ := entetes.Get("Content-Type"); type_ != "application/json; charset=utf-8" {
			t.Errorf("%s : Content-Type %q, voulu application/json; charset=utf-8", artefact, type_)
		}
		var corps503 struct {
			Erreur string `json:"erreur"`
		}
		if err := json.Unmarshal(corps, &corps503); err != nil {
			t.Errorf("%s : corps non JSON : %v", artefact, err)
		} else if corps503.Erreur == "" {
			t.Errorf("%s : 503 sans message d'erreur", artefact)
		}
	}

	if code, _, _ := obtenir(t, recette.URL+"/"); code != http.StatusOK {
		t.Errorf("/ : code %d, voulu 200", code)
	}
}

// TestPrevisuGroupe : GET /m3u?group=<nom> est une prévisualisation brute
// d'UN groupe du snapshot, filtrée à la volée en streaming (story 17) —
// elle ne modifie jamais le snapshot, et un groupe absent donne une
// playlist vide valide (#EXTM3U seul).
func TestPrevisuGroupe(t *testing.T) {
	cfg := configE2E(t, fournir(t, m3uPourE2E, xmltvPourE2E))
	passerUneFois(t, cfg)
	recette := E2E(t, cfg)

	boutique, err := store.Ouvrir(cfg.DataDir)
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	avant, err := os.ReadFile(boutique.CheminSnapshot("m3u"))
	if err != nil {
		t.Fatalf("lecture du snapshot : %v", err)
	}

	attenduNews := `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="bfm.fr" group-title="News",BFM
https://flux.exemple.tld/bfm.m3u8
`
	tests := []struct {
		groupe string
		corps  string
	}{
		{"News", attenduNews},
		{"General", m3uPourE2E[:len(m3uPourE2E)-len(`#EXTINF:-1 tvg-id="bfm.fr" group-title="News",BFM
https://flux.exemple.tld/bfm.m3u8
`)]},
		{"Inconnu", "#EXTM3U url-tvg=\"https://exemple.tld/guide.xml\"\n"},
	}
	for _, cas := range tests {
		code, entetes, corps := obtenir(t, recette.URL+"/m3u?group="+url.QueryEscape(cas.groupe))
		if code != http.StatusOK {
			t.Errorf("group=%s : code %d, voulu 200", cas.groupe, code)
		}
		if type_ := entetes.Get("Content-Type"); type_ != "audio/x-mpegurl; charset=utf-8" {
			t.Errorf("group=%s : Content-Type %q, voulu audio/x-mpegurl; charset=utf-8", cas.groupe, type_)
		}
		if cache := entetes.Get("Cache-Control"); cache != "no-cache" {
			t.Errorf("group=%s : Cache-Control %q, voulu no-cache", cas.groupe, cache)
		}
		if string(corps) != cas.corps {
			t.Errorf("group=%s : contenu inattendu\nservi :\n%q\nvoulu :\n%q", cas.groupe, corps, cas.corps)
		}
	}

	après, err := os.ReadFile(boutique.CheminSnapshot("m3u"))
	if err != nil {
		t.Fatalf("relecture du snapshot : %v", err)
	}
	if string(avant) != string(après) {
		t.Error("la prévisu ?group= a modifié le snapshot")
	}
}

// TestSnapshotServiOctetPourOctet : fournisseur OK, une passe, puis /m3u
// et /xmltv servent le snapshot du disque — les octets servies sont
// identiques au fichier snapshot, en-têtes de contenu et no-cache corrects
// (story 22), Content-Length connu (taille du fichier, ADR-0001). Le M3U
// est en plus identique au corps du fournisseur (aucune réécriture).
func TestSnapshotServiOctetPourOctet(t *testing.T) {
	cfg := configE2E(t, fournir(t, m3uPourE2E, xmltvPourE2E))
	passerUneFois(t, cfg)
	recette := E2E(t, cfg)

	boutique, err := store.Ouvrir(cfg.DataDir)
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}

	tests := []struct {
		chemin      string
		typeContenu string
		corpsFourni string // corps attendu quand copie = corps du fournisseur, vide sinon
	}{
		{"/m3u", "audio/x-mpegurl; charset=utf-8", m3uPourE2E},
		{"/xmltv", "application/xml; charset=utf-8", ""},
	}
	for _, cas := range tests {
		artefact := strings.TrimPrefix(cas.chemin, "/")
		snapshot, err := os.ReadFile(boutique.CheminSnapshot(artefact))
		if err != nil {
			t.Fatalf("lecture du snapshot %s : %v", artefact, err)
		}
		voulu := snapshot
		if cas.corpsFourni != "" {
			voulu = []byte(cas.corpsFourni)
		}

		code, entetes, corps := obtenir(t, recette.URL+cas.chemin)
		if code != http.StatusOK {
			t.Errorf("%s : code %d, voulu 200", cas.chemin, code)
		}
		if type_ := entetes.Get("Content-Type"); type_ != cas.typeContenu {
			t.Errorf("%s : Content-Type %q, voulu %q", cas.chemin, type_, cas.typeContenu)
		}
		if cache := entetes.Get("Cache-Control"); cache != "no-cache" {
			t.Errorf("%s : Cache-Control %q, voulu no-cache", cas.chemin, cache)
		}
		if longueur := entetes.Get("Content-Length"); longueur != strconv.Itoa(len(voulu)) {
			t.Errorf("%s : Content-Length %q, voulu %d", cas.chemin, longueur, len(voulu))
		}
		if string(corps) != string(voulu) {
			t.Errorf("%s : octets servies différentes du snapshot attendu\nservi :\n%q\nvoulu :\n%q", cas.chemin, corps, voulu)
		}
	}
}
