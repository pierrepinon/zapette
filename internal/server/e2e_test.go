package server

import (
	"context"
	"encoding/json"
	"fmt"
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
// injectées dans la source de config retournée. La fonction retournée met
// le fournisseur en PANNE (fermeture des serveurs fake) pour les scénarios
// de repli.
func fournir(t *testing.T, m3u, xmltv string) (config.Source, func()) {
	t.Helper()
	var montés []*httptest.Server
	publier := func(corps string) string {
		serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(corps))
		}))
		t.Cleanup(serveur.Close)
		montés = append(montés, serveur)
		return serveur.URL
	}
	panne := func() {
		for _, serveur := range montés {
			serveur.Close()
		}
	}
	return config.Source{
		Name:     "arte",
		M3UURL:   publier(m3u),
		XMLTVURL: publier(xmltv),
	}, panne
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
	o := fetch.Nouveau(boutique, fetch.NouveauFetcher, fetch.DefautPauseEntreEssais)
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
	recette := httptest.NewServer(Nouveau(boutique, func() config.Config { return cfg }).Handler())
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

	// Même à froid, /api/status reste 200 et conforme : pas de null dans
	// les listes et maps du contrat (le dashboard ne gère pas deux formes).
	code, _, corps := obtenir(t, recette.URL+"/api/status")
	if code != http.StatusOK {
		t.Fatalf("/api/status à froid : code %d, voulu 200", code)
	}
	for champ, brut := range map[string]string{
		"sources":   `"sources":{}`,
		"per_group": `"per_group":{}`,
		"errors":    `"errors":[]`,
	} {
		if !strings.Contains(string(corps), brut) {
			t.Errorf("/api/status à froid : %s absent ou null (voulu %s)\n%s", champ, brut, corps)
		}
	}
}

// TestPrevisuGroupe : GET /m3u?group=<nom> est une prévisualisation brute
// d'UN groupe du snapshot, filtrée à la volée en streaming (story 17) —
// elle ne modifie jamais le snapshot, et un groupe absent donne une
// playlist vide valide (#EXTM3U seul).
func TestPrevisuGroupe(t *testing.T) {
	arte, _ := fournir(t, m3uPourE2E, xmltvPourE2E)
	cfg := configE2E(t, arte)
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

// TestStatutConforme : /api/status expose tout le champ du contrat — états
// par source/artefact (last_ok/last_error/next_at), âge et compteurs du
// snapshot, rss_bytes, ring des erreurs, résumé de config — plus les
// champs d'IP publique encore vides, réservés au ticket suivant.
func TestStatutConforme(t *testing.T) {
	arte, _ := fournir(t, m3uPourE2E, xmltvPourE2E)
	cfg := configE2E(t, arte)
	passerUneFois(t, cfg)
	recette := E2E(t, cfg)

	code, entetes, corps := obtenir(t, recette.URL+"/api/status")
	if code != http.StatusOK {
		t.Fatalf("/api/status : code %d, voulu 200", code)
	}
	if type_ := entetes.Get("Content-Type"); type_ != "application/json; charset=utf-8" {
		t.Errorf("/api/status : Content-Type %q, voulu application/json; charset=utf-8", type_)
	}

	var statut struct {
		GeneratedAt string `json:"generated_at"`
		Sources     map[string]struct {
			M3U   etatArtefactJSON `json:"m3u"`
			XMLTV etatArtefactJSON `json:"xmltv"`
		} `json:"sources"`
		Snapshot struct {
			AgeSeconds    *int64         `json:"age_seconds"`
			TotalChannels int            `json:"total_channels"`
			PerGroup      map[string]int `json:"per_group"`
		} `json:"snapshot"`
		RssBytes          int64     `json:"rss_bytes"`
		Errors            []erreur  `json:"errors"`
		Config            resumeCfg `json:"config"`
		PublicIPv4        string    `json:"public_ipv4"`
		PublicIPv6        string    `json:"public_ipv6"`
		PublicIPLastCheck *string   `json:"public_ip_last_check"`
	}
	if err := json.Unmarshal(corps, &statut); err != nil {
		t.Fatalf("/api/status : corps non conforme : %v\n%s", err, corps)
	}

	if _, err := time.Parse(time.RFC3339, statut.GeneratedAt); err != nil {
		t.Errorf("generated_at %q non RFC3339 : %v", statut.GeneratedAt, err)
	}
	etatArte := statut.Sources["arte"].M3U
	if etatArte.LastOk == nil {
		t.Error("sources.arte.m3u.last_ok manquant après une passe réussie")
	}
	if etatArte.NextAt == nil {
		t.Error("sources.arte.m3u.next_at manquant après une passe réussie")
	}
	if statut.Sources["arte"].M3U.LastError != "" {
		t.Errorf("sources.arte.m3u.last_error = %q, voulu vide après une passe réussie", statut.Sources["arte"].M3U.LastError)
	}
	if statut.Snapshot.AgeSeconds == nil || *statut.Snapshot.AgeSeconds < 0 {
		t.Errorf("snapshot.age_seconds invalide : %v", statut.Snapshot.AgeSeconds)
	}
	if statut.Snapshot.TotalChannels != 2 {
		t.Errorf("snapshot.total_channels = %d, voulu 2", statut.Snapshot.TotalChannels)
	}
	if statut.Snapshot.PerGroup["General"] != 1 || statut.Snapshot.PerGroup["News"] != 1 {
		t.Errorf("snapshot.per_group = %v, voulu {General:1 News:1}", statut.Snapshot.PerGroup)
	}
	if statut.RssBytes <= 0 {
		t.Errorf("rss_bytes = %d, voulu VmRSS > 0", statut.RssBytes)
	}
	if statut.Config.NbSources != 1 {
		t.Errorf("config.nb_sources = %d, voulu 1", statut.Config.NbSources)
	}
	if statut.Config.RefreshInterval != "1h0m0s" {
		t.Errorf("config.refresh_interval = %q, voulu 1h0m0s", statut.Config.RefreshInterval)
	}
	if statut.PublicIPv4 == "" || statut.PublicIPv6 == "" || statut.PublicIPLastCheck != nil {
		t.Errorf("champs IP publique mal exposés (réservés au ticket publicip, valeurs « indisponible ») : %q %q %v",
			statut.PublicIPv4, statut.PublicIPv6, statut.PublicIPLastCheck)
	}
	if statut.Errors == nil {
		t.Error("errors absent (voulu [])")
	}
}

// TestRingErreurs : les erreurs signalées au serveur sortent dans le ring
// « errors » de /api/status, les plus récentes d'abord, plafonnées à 20.
func TestRingErreurs(t *testing.T) {
	boutique, err := store.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	s := Nouveau(boutique, configVide)
	for i := 0; i < 25; i++ {
		s.SignalerErreur(fmt.Sprintf("échec %d", i))
	}

	recette := httptest.NewServer(s.Handler())
	t.Cleanup(recette.Close)
	_, _, corps := obtenir(t, recette.URL+"/api/status")

	var statut struct {
		Errors []struct {
			At      string `json:"at"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(corps, &statut); err != nil {
		t.Fatalf("/api/status non JSON : %v\n%s", err, corps)
	}
	if len(statut.Errors) != 20 {
		t.Errorf("ring = %d entrées, voulu 20 (plafond)", len(statut.Errors))
	}
	if len(statut.Errors) > 0 && statut.Errors[0].Message != "échec 24" {
		t.Errorf("ring : première entrée %q, voulu la plus récente \"échec 24\"", statut.Errors[0].Message)
	}
	if len(statut.Errors) > 0 && statut.Errors[0].At == "" {
		t.Error("ring : entrée sans horodatage")
	}
}

// configVide sert d'injecteur de config pour les tests sans scénario fetch.
func configVide() config.Config { return config.Config{} }

type etatArtefactJSON struct {
	LastOk    *string `json:"last_ok"`
	LastError string  `json:"last_error"`
	NextAt    *string `json:"next_at"`
}

type erreur struct {
	At      string `json:"at"`
	Message string `json:"message"`
}

type resumeCfg struct {
	RefreshInterval string `json:"refresh_interval"`
	NbSources       int    `json:"nb_sources"`
	NbGroupes       int    `json:"nb_groupes"`
}

// TestSnapshotServiOctetPourOctet : fournisseur OK, une passe, puis /m3u
// et /xmltv servent le snapshot du disque — les octets servies sont
// identiques au fichier snapshot, en-têtes de contenu et no-cache corrects
// (story 22), Content-Length connu (taille du fichier, ADR-0001). Le M3U
// est en plus identique au corps du fournisseur (aucune réécriture).
func TestSnapshotServiOctetPourOctet(t *testing.T) {
	arte, _ := fournir(t, m3uPourE2E, xmltvPourE2E)
	cfg := configE2E(t, arte)
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

// TestRepliSurSnapshotEchecFetch : le fournisseur tombe en panne après une
// passe réussie ; la passe suivante échoue pour ses artefacts, le service
// continue de servir le dernier bon snapshot — octet pour octet — et
// /api/status le montre (last_error renseigné, last_ok figé) ET le ring
// « errors » est alimenté via la callback : main branche le signaleur de
// l'ordonnanceur sur le ring du serveur (story 15).
func TestRepliSurSnapshotEchecFetch(t *testing.T) {
	arte, panne := fournir(t, m3uPourE2E, xmltvPourE2E)
	cfg := configE2E(t, arte)

	boutique, err := store.Ouvrir(cfg.DataDir)
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	// Câblage de production : la callback du fetch alimente le ring du
	// serveur (dépendance inversée, fetch ne connaît jamais server).
	s := Nouveau(boutique, func() config.Config { return cfg })
	ordonnanceur := fetch.Nouveau(boutique, fetch.NouveauFetcher, fetch.DefautPauseEntreEssais).
		AvecSignaleur(s.SignalerErreur)

	if err := ordonnanceur.Passe(context.Background(), cfg); err != nil {
		t.Fatalf("passe réussie : %v", err)
	}

	avant := map[string]string{
		"m3u":   lireFichier(t, boutique.CheminSnapshot("m3u")),
		"xmltv": lireFichier(t, boutique.CheminSnapshot("xmltv")),
	}

	// Panne du fournisseur : les upstreams fake sont fermés, toute URL de
	// la config échoue désormais (connexion refusée).
	panne()

	if err := ordonnanceur.Passe(context.Background(), cfg); err != nil {
		t.Fatalf("passe en échec : %v", err)
	}

	recette := httptest.NewServer(s.Handler())
	t.Cleanup(recette.Close)
	for artefact, avantBytes := range avant {
		code, _, corps := obtenir(t, recette.URL+"/"+artefact)
		if code != http.StatusOK {
			t.Errorf("%s : code %d, voulu 200 (repli sur le dernier bon snapshot)", artefact, code)
		}
		if string(corps) != avantBytes {
			t.Errorf("%s : le repli a changé les octets servies", artefact)
		}
	}

	_, _, corps := obtenir(t, recette.URL+"/api/status")
	var statut struct {
		Sources map[string]struct {
			M3U   etatArtefactJSON `json:"m3u"`
			XMLTV etatArtefactJSON `json:"xmltv"`
		} `json:"sources"`
		Errors []erreur `json:"errors"`
	}
	if err := json.Unmarshal(corps, &statut); err != nil {
		t.Fatalf("/api/status non JSON : %v", err)
	}
	if statut.Sources["arte"].M3U.LastError == "" {
		t.Error("sources.arte.m3u.last_error vide après la panne, voulu renseigné")
	}
	if statut.Sources["arte"].M3U.LastOk == nil {
		t.Error("sources.arte.m3u.last_ok perdu par la panne, voulu la dernière réussite")
	}
	// Le ring de /api/status reflète l'échec de la passe — jamais vide
	// quand un fetch vient de tomber (story 15).
	if len(statut.Errors) == 0 {
		t.Fatal("ring errors VIDE après un fetch en échec : fonctionnalité morte")
	}
	trouve := false
	for _, e := range statut.Errors {
		if strings.Contains(e.Message, "arte") {
			trouve = true
		}
	}
	if !trouve {
		t.Errorf("ring : aucun message ne cite la source \"arte\" ; reçus %v", statut.Errors)
	}
}

func lireFichier(t *testing.T, chemin string) string {
	t.Helper()
	brut, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatalf("lecture de %s : %v", chemin, err)
	}
	return string(brut)
}

// TestFroidSansSnapshot : au démarrage, avant la première passe réussie,
