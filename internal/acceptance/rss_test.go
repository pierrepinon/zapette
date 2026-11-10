package acceptance

import (
	"bufio"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/fetch"
	"github.com/pierrepinon/zapette/internal/m3u"
	"github.com/pierrepinon/zapette/internal/server"
	"github.com/pierrepinon/zapette/internal/store"
)

// budgetRSS : le critère de la spec — rester sous 100 Mio de RSS VmRSS
// (/proc/self/status) pour une playlist de 216 000 entrées, pics de fetch
// et requêtes simultanées comprises. Le harnais est volontairement strict :
// c'est le PIC échantillonné qui est jugé, pas une moyenne.
const budgetRSS = 100 << 20

// groupeFiltre : le seul groupe autorisé par le harnais — un quart des
// entrées générées est retenu ; la diff binaire porte donc sur un
// sous-ensemble FILTRÉ, preuve plus forte qu'une copie intégrale.
const groupeFiltre = "Sports"

// lectureRSSBrute lit le VmRSS de /proc/self/status en octets ; 0 si
// indisponible (hors Linux). C'est le même champ que celui qu'expose
// /api/status (rss_bytes) en production.
func lectureRSSBrute() int64 {
	brut, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, ligne := range strings.Split(string(brut), "\n") {
		if valeur, ok := strings.CutPrefix(ligne, "VmRSS:"); ok {
			champs := strings.Fields(valeur)
			if len(champs) >= 2 {
				if kio, err := strconv.ParseInt(champs[0], 10, 64); err == nil {
					return kio << 10
				}
			}
		}
	}
	return 0
}

// exigerRSS saute le test si VmRSS est illisible : le critère mesuré exige
// Linux ; hors Linux, le diff et le kill/restart restent couverts ailleurs.
func exigerRSS(t *testing.T) {
	t.Helper()
	if lectureRSSBrute() == 0 {
		t.Skip("VmRSS indisponible (/proc/self/status) : la mesure RSS exige Linux")
	}
}

// demarrerEchantillonnage lance un échantillonneur de VmRSS toutes les 5 ms
// et retourne la fonction d'arrêt, qui retourne le PIC mesuré. Le critère
// couvre les pics : le harnais juge le pic, pas une moyenne.
func demarrerEchantillonnage() (arreter func() int64) {
	var pic atomic.Int64
	stop := make(chan struct{})
	fini := make(chan struct{})
	go func() {
		defer close(fini)
		tick := time.NewTicker(5 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				if r := lectureRSSBrute(); r > pic.Load() {
					pic.Store(r)
				}
			}
		}
	}()
	return func() int64 {
		close(stop)
		<-fini
		return pic.Load()
	}
}

// generer écrit sur disque les playlists SYNTHÉTIQUES M3U + XMLTV de n
// entrées, en streaming (jamais la playlist entière en mémoire, même côté
// harnais — la mesure RSS ne doit pas payer le jeu d'essai).
func generer(t *testing.T, cheminM3U, cheminXMLTV string, n int) {
	t.Helper()
	fM3U, err := os.Create(cheminM3U)
	if err != nil {
		t.Fatalf("création de %s : %v", cheminM3U, err)
	}
	if err := genererM3U(fM3U, n); err != nil {
		t.Fatalf("génération M3U : %v", err)
	}
	if err := fM3U.Close(); err != nil {
		t.Fatalf("fermeture %s : %v", cheminM3U, err)
	}
	fXMLTV, err := os.Create(cheminXMLTV)
	if err != nil {
		t.Fatalf("création de %s : %v", cheminXMLTV, err)
	}
	if err := genererXMLTV(fXMLTV, n, 1); err != nil {
		t.Fatalf("génération XMLTV : %v", err)
	}
	if err := fXMLTV.Close(); err != nil {
		t.Fatalf("fermeture %s : %v", cheminXMLTV, err)
	}
}

// monterFournisseur monte un faux fournisseur HTTP qui sert en STREAMING le
// fichier généré sur disque (ReadSeeker : jamais le fichier entier en RAM
// côté amont non plus) et retourne son URL.
func monterFournisseur(t *testing.T, chemin, typeContenu string) string {
	t.Helper()
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, requete *http.Request) {
		f, err := os.Open(chemin)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		w.Header().Set("Content-Type", typeContenu)
		http.ServeContent(w, requete, "", time.Time{}, f)
	}))
	t.Cleanup(serveur.Close)
	return serveur.URL
}

// empreinteEntrees calcule l'empreinte SHA-256 des OCTETS BRUTS des entrées
// (depuis #EXTINF jusqu'à l'URL de flux, fins de ligne d'origine) retenues du
// fichier indiqué, filtrées par groupe exact si groupe != "". C'est la
// fonction de diff binaire du critère « URL de flux strictement
// inchangées ». Mémoire constante : le harnais ne charge jamais la playlist
// (ADR-0001).
func empreinteEntrees(t *testing.T, chemin, groupe string) (string, int) {
	t.Helper()
	f, err := os.Open(chemin)
	if err != nil {
		t.Fatalf("ouverture de %s : %v", chemin, err)
	}
	defer f.Close()
	h := sha256.New()
	compte := 0
	err = m3u.Balayer(bufio.NewReaderSize(f, 1<<20), nil, func(c m3u.Chaine, lignes []string) error {
		if groupe != "" && c.Groupe != groupe {
			return nil
		}
		compte++
		for _, ligne := range lignes {
			if _, err := io.WriteString(h, ligne); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("balayage de %s : %v", chemin, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil)), compte
}

// empreinteFichier : le SHA-256 d'un fichier entier, lu en streaming.
func empreinteFichier(t *testing.T, chemin string) string {
	t.Helper()
	f, err := os.Open(chemin)
	if err != nil {
		t.Fatalf("ouverture de %s : %v", chemin, err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		t.Fatalf("lecture de %s : %v", chemin, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// telechargerEnFichier fait un GET réel et écrit le corps en streaming dans
// le fichier (empreinte calculée au vol) : vérifie le code HTTP.
func telechargerEnFichier(t *testing.T, client *http.Client, url, fichier string) string {
	t.Helper()
	reponse, err := client.Get(url)
	if err != nil {
		t.Fatalf("GET %s : %v", url, err)
	}
	defer reponse.Body.Close()
	if reponse.StatusCode != http.StatusOK {
		t.Fatalf("GET %s : code %d, attendu 200", url, reponse.StatusCode)
	}
	sortie, err := os.Create(fichier)
	if err != nil {
		t.Fatalf("création de %s : %v", fichier, err)
	}
	defer sortie.Close()
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(sortie, h), reponse.Body); err != nil {
		t.Fatalf("téléchargement %s : %v", url, err)
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// premiereLigne retourne la première ligne du fichier (l'en-tête #EXTM3U).
func premiereLigne(t *testing.T, chemin string) string {
	t.Helper()
	f, err := os.Open(chemin)
	if err != nil {
		t.Fatalf("ouverture de %s : %v", chemin, err)
	}
	defer f.Close()
	lu := bufio.NewReaderSize(f, 1<<20)
	ligne, _ := lu.ReadString('\n')
	return ligne
}

// TestAcceptationRSS216k : LA case d'acceptation volumétrique de la spec.
//
//  1. Playlists SYNTHÉTIQUES de 216 000 entrées générées à la volée sur
//     disque, servies par un faux fournisseur HTTP local (aucune donnée
//     réelle du fournisseur, aucun secret).
//  2. Une passe de fetch RÉELLE (fetch + filtre + copie de source +
//     reconstruction du snapshot + meta.json), pic VmRSS échantillonné.
//  3. Requêtes SIMULTANÉES /m3u + /xmltv (goroutines clientes HTTP réelles)
//     et téléchargements dédiés des deux artefacts PENDANT la charge, pic
//     VmRSS échantillonné, corps lus en streaming.
//  4. Diff binaire : les lignes #EXTINF/URL retenues côté source doivent se
//     retrouver servies octet pour octet, et l'en-tête #EXTM3U préservé.
//  5. /api/status doit rendre compte du même compte de chaînes et d'un RSS.
func TestAcceptationRSS216k(t *testing.T) {
	if testing.Short() {
		t.Skip("harnais volumétrique 216k : sauté en mode court (lancé par scripts/acceptance.sh)")
	}
	exigerRSS(t)

	repertoire := t.TempDir()
	cheminM3U := filepath.Join(repertoire, "source.m3u")
	cheminXMLTV := filepath.Join(repertoire, "source.xmltv")
	generer(t, cheminM3U, cheminXMLTV, entreesReference)
	urlM3U := monterFournisseur(t, cheminM3U, "audio/x-mpegurl")
	urlXMLTV := monterFournisseur(t, cheminXMLTV, "application/xml")

	cfg := config.Config{
		RefreshInterval: time.Hour,
		AllowedGroups:   []string{groupeFiltre},
		Sources: []config.Source{{
			Name:     "synthetique",
			M3UURL:   urlM3U,
			XMLTVURL: urlXMLTV,
		}},
		FetchTimeout: 60 * time.Second,
		FetchRetries: 0,
		ServerAddr:   "127.0.0.1:0",
		DataDir:      filepath.Join(repertoire, "data"),
	}
	boutique, err := store.Ouvrir(cfg.DataDir)
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}

	// ——— Mesure 1 : la passe de FETCH. ———
	mesurerFetch := demarrerEchantillonnage()
	ordonnanceur := fetch.Nouveau(boutique, fetch.NouveauFetcher, fetch.RetourArriereDefaut)
	if err := ordonnanceur.Passe(t.Context(), cfg); err != nil {
		t.Fatalf("passe de fetch 216k : %v", err)
	}
	picFetch := mesurerFetch()

	for _, artefact := range []string{"m3u", "xmltv"} {
		if _, err := os.Stat(boutique.CheminSnapshot(artefact)); err != nil {
			t.Fatalf("snapshot %s absent après la passe : %v", artefact, err)
		}
	}

	// ——— Mesure 2 : requêtes simultanées /m3u + /xmltv + /api/status. ———
	serveur := httptest.NewServer(server.Nouveau(boutique, func() config.Config { return cfg }).Handler())
	t.Cleanup(serveur.Close)

	var (
		requetes   atomic.Int64
		octets     atomic.Int64
		echecs     atomic.Int64
		stopCharge = make(chan struct{})
		selection  sync.WaitGroup
	)
	for _, chemin := range []string{"/m3u", "/xmltv", "/m3u", "/xmltv", "/m3u", "/xmltv", "/m3u", "/xmltv"} {
		selection.Add(1)
		go func(chemin string) {
			defer selection.Done()
			client := serveur.Client()
			client.Timeout = 30 * time.Second
			for {
				select {
				case <-stopCharge:
					return
				default:
				}
				reponse, err := client.Get(serveur.URL + chemin)
				if err != nil {
					echecs.Add(1)
					continue
				}
				taille, _ := io.Copy(io.Discard, reponse.Body)
				reponse.Body.Close()
				octets.Add(taille)
				if reponse.StatusCode == http.StatusOK {
					requetes.Add(1)
				} else {
					echecs.Add(1)
				}
			}
		}(chemin)
	}

	// L'échantillonnage couvre AUSSI les téléchargements dédiés ci-dessous :
	// les octets jugés par le diff sont servis PENDANT la charge.
	mesurerService := demarrerEchantillonnage()

	// Pendant la charge : téléchargement DÉDIÉ de chaque artefact — c'est
	// son corps exact, servi sous requêtes simultanées, que le diff binaire
	// juge — et un poll de /api/status, comme le dashboard.
	serveXMLTV := filepath.Join(repertoire, "servi.xmltv")
	telechargerEnFichier(t, serveur.Client(), serveur.URL+"/xmltv", serveXMLTV)
	serveM3U := filepath.Join(repertoire, "servi.m3u")
	telechargerEnFichier(t, serveur.Client(), serveur.URL+"/m3u", serveM3U)
	reponseStatut, err := serveur.Client().Get(serveur.URL + "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status : %v", err)
	}
	corpsStatut, err := io.ReadAll(reponseStatut.Body)
	reponseStatut.Body.Close()
	if err != nil {
		t.Fatalf("lecture /api/status : %v", err)
	}
	if reponseStatut.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/status : code %d, attendu 200", reponseStatut.StatusCode)
	}

	// La charge martèle sous échantillonnage pendant la fenêtre.
	time.Sleep(2 * time.Second)
	close(stopCharge)
	selection.Wait()
	picService := mesurerService()

	// ——— Verdict RSS. ———
	if picFetch >= budgetRSS {
		t.Errorf("pic VmRSS pendant le fetch : %d Mio (>= %d Mio, budget de la spec)",
			picFetch>>20, budgetRSS>>20)
	}
	if picService >= budgetRSS {
		t.Errorf("pic VmRSS sous requêtes simultanées : %d Mio (>= %d Mio, budget de la spec)",
			picService>>20, budgetRSS>>20)
	}

	// ——— Diff binaire : URLs de flux strictement inchangées. ———
	// Les entrées RETENUES de la source (octets bruts du fournisseur) se
	// retrouvent à l'identique dans ce qui est servi sous charge.
	srcEmpreinte, srcCompte := empreinteEntrees(t, cheminM3U, groupeFiltre)
	serviEmpreinte, serviCompte := empreinteEntrees(t, serveM3U, "")
	if srcEmpreinte != serviEmpreinte || srcCompte != serviCompte {
		t.Errorf("diff binaire des lignes #EXTINF/URL : source %d entrées (%s…), servi %d entrées (%s…) — des octets diffèrent",
			srcCompte, srcEmpreinte[:16], serviCompte, serviEmpreinte[:16])
	}
	if serviCompte != entreesReference/4 {
		t.Errorf("%d entrées retenues, attendu %d (un quart de %d, groupe %s)",
			serviCompte, entreesReference/4, entreesReference, groupeFiltre)
	}
	// L'en-tête #EXTM3U d'origine est conservé tel quel (attributs compris).
	if enteteSrc, enteteServi := premiereLigne(t, cheminM3U), premiereLigne(t, serveM3U); enteteSrc != enteteServi {
		t.Errorf("en-tête réécrit : source %q, servi %q", enteteSrc, enteteServi)
	}
	// Et les octets servis restent exactement ceux du snapshot sur disque.
	if empreinteFichier(t, serveM3U) != empreinteFichier(t, boutique.CheminSnapshot("m3u")) {
		t.Error("les octets servis /m3u ne sont pas ceux du snapshot sur disque")
	}

	// ——— /api/status : compteurs et RSS rendus. ———
	var statut struct {
		RSSBytes int64 `json:"rss_bytes"`
		Snapshot struct {
			TotalChannels int            `json:"total_channels"`
			PerGroup      map[string]int `json:"per_group"`
		} `json:"snapshot"`
	}
	if err := json.Unmarshal(corpsStatut, &statut); err != nil {
		t.Fatalf("/api/status illisible : %v", err)
	}
	if statut.Snapshot.TotalChannels != entreesReference/4 {
		t.Errorf("/api/status total_channels = %d, attendu %d",
			statut.Snapshot.TotalChannels, entreesReference/4)
	}
	if statut.Snapshot.PerGroup[groupeFiltre] != entreesReference/4 {
		t.Errorf("/api/status per_group[%s] = %d, attendu %d",
			groupeFiltre, statut.Snapshot.PerGroup[groupeFiltre], entreesReference/4)
	}
	if statut.RSSBytes <= 0 {
		t.Errorf("/api/status rss_bytes = %d, attendu > 0 (lecture /proc/self/status)", statut.RSSBytes)
	}

	t.Logf("MESURES RSS : pic fetch = %d Mio, pic service (%d requêtes OK, %d échecs, %d Mio servis) = %d Mio, budget = %d Mio ; snapshot m3u = %d entrées retenues ; /api/status rss_bytes = %d o",
		picFetch>>20, requetes.Load(), echecs.Load(), octets.Load()>>20, picService>>20, budgetRSS>>20, serviCompte, statut.RSSBytes)
}
