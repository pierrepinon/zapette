package acceptance

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pierrepinon/zapette/internal/m3u"
)

// budgetRSS : le critère de la spec — rester sous 100 Mio de RSS VmRSS
// (/proc/self/status) pour une playlist de 216 000 entrées, pics de fetch
// et requêtes simultanées comprises. Le harnais est volontairement strict :
// c'est le PIC échantillonné qui est jugé, pas une moyenne. (Constante
// partagée : le test qui la consomme vit dans rss216_test.go.)
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
// fichier indiqué, filtrées par groupe exact si groupe != "". La ligne
// #EXTINF est NORMALISÉE avant empreinte : son tvg-chno est forcé à « 0 »
// par m3u.Numeroter — le tvg-chno est LA SEULE réécriture admise du M3U
// servi (ADR-0002, numéros calculés), tout autre octet doit coïncider.
// C'est la fonction de diff binaire du critère « URL de flux strictement
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
			// Normalisation de la ligne #EXTINF : hors tvg-chno, octets
			// d'origine strictement exigés.
			if strings.HasPrefix(ligne, "#EXTINF") {
				ligne = m3u.Numeroter(ligne, "0")
			}
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
