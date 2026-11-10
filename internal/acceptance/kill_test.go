package acceptance

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/fetch"
	"github.com/pierrepinon/zapette/internal/store"
)

// detecteurPrete absorbe la sortie du binaire (logs slog JSON sur stdout) et
// signale la ligne « serveur HTTP en écoute » : le harnais n'attend jamais
// aveuglément, il attend le journal de PRODUCTION.
type detecteurPrete struct {
	vu    bool
	prete chan struct{}
}

func nouveauDetecteurPrete() *detecteurPrete {
	return &detecteurPrete{prete: make(chan struct{})}
}

func (d *detecteurPrete) Write(p []byte) (int, error) {
	if !d.vu && strings.Contains(string(p), "serveur HTTP en écoute") {
		d.vu = true
		close(d.prete)
	}
	return len(p), nil
}

// ecrireConfigYAML écrit le fichier de config versionné d'une phase. La même
// source (name fournisseur) est réadressée de phase en phase : c'est le
// scénario d'exploitation réel — même data_dir, sources qui changent. Avec
// xmltvURL vide, la ligne xmltv_url est omise : l'artefact sort du plan de la
// source (fetch et état non renouvelés, cf. internal/fetch).
func ecrireConfigYAML(t *testing.T, chemin string, port int, dataDir, m3uURL, xmltvURL string) {
	t.Helper()
	var contenu strings.Builder
	fmt.Fprintf(&contenu, `refresh_interval: 1h
allowed_groups: [%s]
fetch_timeout: 10s
fetch_retries: 3
server_addr: 127.0.0.1:%d
data_dir: %s
sources:
  - name: fournisseur
    m3u_url: %s
`, groupeFiltre, port, dataDir, m3uURL)
	if xmltvURL != "" {
		fmt.Fprintf(&contenu, "    xmltv_url: %s\n", xmltvURL)
	}
	if err := os.WriteFile(chemin, []byte(contenu.String()), 0o644); err != nil {
		t.Fatalf("écriture de %s : %v", chemin, err)
	}
}

// portLibre réserve puis libère un port : le binaire du harnais écoutera là.
func portLibre(t *testing.T) int {
	t.Helper()
	ecouteur, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("réservation d'un port : %v", err)
	}
	port := ecouteur.Addr().(*net.TCPAddr).Port
	ecouteur.Close()
	return port
}

// lancerBinaire compile le binaire de production, le lance avec la config de
// la phase, et le retourne une fois la ligne « serveur HTTP en écoute » vue
// du journal (ou cloue le test : une anomalie de démarrage doit être
// visible, pas attendue sans fin). Le journal capturé est également retourné
// : le harnais le logue en cas d'échec de la phase.
func lancerBinaire(t *testing.T, repertoire, cheminConfig string) (*exec.Cmd, *strings.Builder) {
	t.Helper()

	// Le binaire : compilation du paquet main depuis la racine du dépôt.
	cheminBinaire := filepath.Join(repertoire, "zapette-binaire")
	commande := exec.Command("go", "build", "-o", cheminBinaire, ".")
	commande.Dir = "../.."
	if sortie, err := commande.CombinedOutput(); err != nil {
		t.Fatalf("compilation du binaire : %v\n%s", err, sortie)
	}

	cmd := exec.Command(cheminBinaire)
	cmd.Env = append(os.Environ(), "ZAPETTE_CONFIG="+cheminConfig)
	var emballee strings.Builder
	detecteur := nouveauDetecteurPrete()
	cmd.Stdout = io.MultiWriter(detecteur, &emballee)
	cmd.Stderr = &emballee
	if err := cmd.Start(); err != nil {
		t.Fatalf("démarrage du binaire : %v", err)
	}
	// Quoi qu'il arrive au déroulé du test : pas de binaire orphelin.
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	select {
	case <-detecteur.prete:
		return cmd, &emballee
	case <-time.After(15 * time.Second):
		t.Fatalf("binaire jamais prêt (log :\n%s)", emballee.String())
		return nil, &emballee
	}
}

// tuerParSIGKILL : le kill du scénario, vrai signal, vrai processus.
func tuerParSIGKILL(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("SIGKILL du binaire : %v", err)
	}
	_, _ = cmd.Process.Wait()
	// Terminé : le cleanup ne doit plus le toucher.
	cmd.Process = nil
}

// monterFournisseurLent : un amont qui se fait tirer l'oreille — le corps
// part par blocs de 8 Kio avec 25 ms de pause, et le canal retourné est
// noté dès le premier bloc servi : c'est CE point qui garantit que le
// SIGKILL tombe PENDANT un fetch, pas avant.
func monterFournisseurLent(t *testing.T, chemin, typeContenu string) (url string, enVol <-chan struct{}) {
	t.Helper()
	vol := make(chan struct{})
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, requete *http.Request) {
		f, err := os.Open(chemin)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", typeContenu)
		w.WriteHeader(http.StatusOK)
		lu := bufio.NewReaderSize(f, 8192)
		tampon := make([]byte, 8192)
		premier := true
		for {
			n, err := lu.Read(tampon)
			if n > 0 {
				if _, err := w.Write(tampon[:n]); err != nil {
					return // le lecteur est mort (SIGKILL) : fin de séance
				}
				if fl, ok := w.(http.Flusher); ok {
					fl.Flush()
				}
				if premier {
					premier = false
					close(vol)
				}
				select {
				case <-requete.Context().Done():
					return
				case <-time.After(25 * time.Millisecond):
				}
			}
			if err != nil {
				return
			}
		}
	}))
	t.Cleanup(serveur.Close)
	return serveur.URL, vol
}

// attendreHTTPFonctionnel poll /m3u jusqu'à un 200 (le service doit répondre
// dès que le socket est ouvert, fetch en arrière-plan ou pas).
func attendreHTTPFonctionnel(t *testing.T, port int) *http.Client {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/m3u", port)
	dernierStatut := 0
	for essai := 0; essai < 100; essai++ {
		reponse, err := client.Get(url)
		if err == nil {
			reponse.Body.Close()
			if reponse.StatusCode == http.StatusOK {
				return client
			}
			dernierStatut = reponse.StatusCode
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("/m3u jamais fonctionnelle après le redémarrage (dernier code %d)", dernierStatut)
	return nil
}

// TestAcceptationKillPuisRedemarrage : le scénario d'exploitation de la
// story 20 et du critère « kill pendant un fetch → restart → service
// opérationnel depuis le disque », joué sur le VRAI binaire :
//
//  1. Phase A : une passe de fetch complète réussit (playlist synthétique
//     servie par un fournisseur local) → dernière bonne copie sur disque.
//  2. Phase B : le binaire démarre contre un fournisseur LENT et prend un
//     SIGKILL en plein fetch → snapshot au disque octet pour octet identique,
//     temporaires orphelins présents mais sans conséquence.
//  3. Phase C : le binaire redémarre avec un fournisseur MORT → /m3u et
//     /xmltv resservent les octets exacts de la dernière bonne copie, et
//     /api/status rend compte du repli (last_ok conservé, last_error visible).
func TestAcceptationKillPuisRedemarrage(t *testing.T) {
	if testing.Short() {
		t.Skip("compilation et SIGKILL du binaire : sauté en mode court (lancé par scripts/acceptance.sh)")
	}
	exigerRSS(t)

	repertoire := t.TempDir()
	cheminM3U := filepath.Join(repertoire, "source.m3u")
	cheminXMLTV := filepath.Join(repertoire, "source.xmltv")
	generer(t, cheminM3U, cheminXMLTV, entreesKill9)

	// ——— Phase A : la dernière bonne passe réussit. ———
	urlM3U := monterFournisseur(t, cheminM3U, "audio/x-mpegurl")
	urlXMLTV := monterFournisseur(t, cheminXMLTV, "application/xml")
	dataDir := filepath.Join(repertoire, "data")
	boutique, err := store.Ouvrir(dataDir)
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	if err := fetch.Nouveau(boutique, fetch.NouveauFetcher, fetch.RetourArriereDefaut).
		Passe(t.Context(), config.Config{
			RefreshInterval: time.Hour,
			AllowedGroups:   []string{groupeFiltre},
			Sources: []config.Source{{
				Name:     "fournisseur",
				M3UURL:   urlM3U,
				XMLTVURL: urlXMLTV,
			}},
			FetchTimeout: 30 * time.Second,
			FetchRetries: 0,
			ServerAddr:   "127.0.0.1:0",
			DataDir:      dataDir,
		}); err != nil {
		t.Fatalf("passe de fetch de la phase A : %v", err)
	}
	empreinteM3U := empreinteFichier(t, boutique.CheminSnapshot("m3u"))
	empreinteXMLTV := empreinteFichier(t, boutique.CheminSnapshot("xmltv"))

	// ——— Phase B : SIGKILL EN PLEIN FETCH. ———
	urlLenteM3U, enVolM3U := monterFournisseurLent(t, cheminM3U, "audio/x-mpegurl")
	urlLenteXMLTV, _ := monterFournisseurLent(t, cheminXMLTV, "application/xml")

	port := portLibre(t)
	cheminConfig := filepath.Join(repertoire, "config-phase-b.yaml")
	ecrireConfigYAML(t, cheminConfig, port, dataDir, urlLenteM3U, urlLenteXMLTV)

	cmd, journal := lancerBinaire(t, repertoire, cheminConfig)
	select {
	case <-enVolM3U:
	case <-time.After(15 * time.Second):
		t.Fatalf("le fetch m3u n'est jamais parti : la fenêtre de kill n'est pas reproductible (log :\n%s)", journal.String())
	}
	// Le fetch m3u est EN COURS (le filtre alimente déjà la copie temporaire) :
	// une petite marge pour être au cœur de l'écriture, puis le kill.
	time.Sleep(300 * time.Millisecond)
	tuerParSIGKILL(t, cmd)

	// Le snapshot au disque n'a pas bougé d'un octet : renommage atomique
	// (store), jamais de fichier partiel servi ni persisté.
	if nouvelle := empreinteFichier(t, boutique.CheminSnapshot("m3u")); nouvelle != empreinteM3U {
		t.Errorf("snapshot.m3u modifié par le kill : %s (attendu %s)", nouvelle[:16], empreinteM3U[:16])
	}
	if nouvelle := empreinteFichier(t, boutique.CheminSnapshot("xmltv")); nouvelle != empreinteXMLTV {
		t.Errorf("snapshot.xmltv modifié par le kill : %s (attendu %s)", nouvelle[:16], empreinteXMLTV[:16])
	}
	// Le kill a bien laissé des temporaires orphelins de copie en cours —
	// la preuve que le harnais a tué au cœur d'un fetch, et non après coup.
	orphelins := 0
	liste, err := os.ReadDir(filepath.Join(dataDir, "sources"))
	if err != nil {
		t.Fatalf("lecture de sources/ : %v", err)
	}
	for _, entree := range liste {
		if strings.HasPrefix(entree.Name(), ".tmp-") {
			orphelins++
		}
	}
	if orphelins == 0 {
		t.Error("aucun temporaire orphelin après le SIGKILL : le kill n'était probablement pas en plein fetch")
	}

	// ——— Phase C : redémarrage, fournisseur EN PANNE, service depuis le
	// disque. ———
	// Le "mort" du scénario est un fournisseur qui répond 503 instantanément :
	// le fetch échoue au premier essai sans dépendre du délai réseau de l'hôte
	// (un dial refusé peut traîner des dizaines de secondes dans un bac à
	// sable), la passe de fond soutient le contrat : échec rapide tracé,
	// copie figée, service offert depuis le disque.
	panne := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, requete *http.Request) {
		http.Error(w, "panne du fournisseur", http.StatusServiceUnavailable)
	}))
	t.Cleanup(panne.Close)
	cheminConfigC := filepath.Join(repertoire, "config-phase-c.yaml")
	// Sans xmltv_url : un seul artefact en plan pour cette source, la passe
	// de fond échoue en un cycle (1 essai + 3 reprises à 5 s de backoff fixe)
	// et publie meta.json — la reprise ne doit pas durer plus que ça.
	ecrireConfigYAML(t, cheminConfigC, port, dataDir, panne.URL+"/m3u", "")
	_, journalRedemarrage := lancerBinaire(t, repertoire, cheminConfigC)

	// Le service est OPÉRATIONNEL immédiatement, pendant que la passe de
	// fond échoue : c'est le contrat du repli (story 9, story 20).
	client := attendreHTTPFonctionnel(t, port)
	serveM3U := filepath.Join(repertoire, "reprise.m3u")
	serveXMLTV := filepath.Join(repertoire, "reprise.xmltv")
	telechargerEnFichier(t, client, fmt.Sprintf("http://127.0.0.1:%d/m3u", port), serveM3U)
	telechargerEnFichier(t, client, fmt.Sprintf("http://127.0.0.1:%d/xmltv", port), serveXMLTV)
	if reappris := empreinteFichier(t, serveM3U); reappris != empreinteM3U {
		t.Errorf("/m3u après redémarrage : %s (attendu la dernière bonne copie %s)", reappris[:16], empreinteM3U[:16])
	}
	if reappris := empreinteFichier(t, serveXMLTV); reappris != empreinteXMLTV {
		t.Errorf("/xmltv après redémarrage : %s (attendu la dernière bonne copie %s)", reappris[:16], empreinteXMLTV[:16])
	}

	// /api/status rend compte du repli : last_ok de la passe A conservé,
	// last_error de la passe C visible, RSS exposé. Le harnais attend la
	// fin de la passe de fond (meta.json est écrit EN DERNIER) : la
	// dernière erreur n'est visible qu'après sa publication.
	var statut struct {
		RSSBytes int64 `json:"rss_bytes"`
		Sources  map[string]struct {
			M3U struct {
				LastOk    *string `json:"last_ok"`
				LastError string  `json:"last_error"`
			} `json:"m3u"`
			XMLTV struct {
				LastOk    *string `json:"last_ok"`
				LastError string  `json:"last_error"`
			} `json:"xmltv"`
		} `json:"sources"`
		Snapshot struct {
			AgeSeconds *int64 `json:"age_seconds"`
		} `json:"snapshot"`
	}
	urlStatut := fmt.Sprintf("http://127.0.0.1:%d/api/status", port)
	desemble := time.Now()
	for {
		reponse, err := client.Get(urlStatut)
		if err != nil {
			t.Fatalf("GET /api/status après redémarrage : %v", err)
		}
		err = json.NewDecoder(reponse.Body).Decode(&statut)
		reponse.Body.Close()
		if err != nil {
			t.Fatalf("/api/status illisible : %v", err)
		}
		etat, presente := statut.Sources["fournisseur"]
		if presente && etat.M3U.LastError != "" {
			break // la passe de fond a fini et son échec est tracé
		}
		if time.Since(desemble) > 45*time.Second {
			brut, _ := os.ReadFile(filepath.Join(dataDir, "meta.json"))
			t.Fatalf("les échecs de la passe de fond ne sont jamais passés sur /api/status (meta.json sur disque :\n%s\njournal du redémarrage :\n%s)", brut, journalRedemarrage.String())
		}
		time.Sleep(50 * time.Millisecond)
	}
	etat, presente := statut.Sources["fournisseur"]
	if !presente {
		t.Fatal("/api/status n'exhibe pas la source fournisseur après redémarrage")
	}
	if etat.M3U.LastOk == nil {
		t.Error("/api/status : last_ok m3u perdu au redémarrage (la dernière bonne passe doit rester visible)")
	}
	if etat.M3U.LastError == "" {
		t.Error("/api/status : last_error m3u vide alors que le fournisseur est mort (le repli doit être visible)")
	}
	if etat.XMLTV.LastOk == nil {
		t.Error("/api/status : last_ok xmltv perdu au redémarrage")
	}
	if statut.Snapshot.AgeSeconds == nil {
		t.Error("/api/status : age_seconds null alors qu'un snapshot existe sur disque")
	}
	if statut.RSSBytes <= 0 {
		t.Errorf("/api/status rss_bytes = %d, attendu > 0", statut.RSSBytes)
	}

	t.Logf("KILL/RESTART : SIGKILL en plein fetch (%d temporaires orphelins), redémarrage sur disque : /m3u et /xmltv octet pour octet la dernière bonne copie, /api/status last_ok conservé et last_error visible, rss_bytes = %d o",
		orphelins, statut.RSSBytes)
}
