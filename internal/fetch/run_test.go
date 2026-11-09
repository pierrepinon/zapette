package fetch

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/m3u"
	"github.com/pierrepinon/zapette/internal/store"
)

// ecrireFichierDepot écrit un fichier de config minimal pour le loader :
// intervalle court, une seule source (arte, avec guide), data_dir isolé.
func ecrireFichierDepot(t *testing.T, dir string, intervalle string, urlM3U string) string {
	t.Helper()
	chemin := filepath.Join(t.TempDir(), "config.yaml")
	contenu := "refresh_interval: " + intervalle + "\n" +
		"data_dir: " + dir + "\n" +
		"sources:\n" +
		"  - name: arte\n" +
		"    m3u_url: " + urlM3U + "\n" +
		"    xmltv_url: " + serveurTexte(t, arteXMLTVCorps) + "\n"
	if err := os.WriteFile(chemin, []byte(contenu), 0o644); err != nil {
		t.Fatalf("écriture de la config de test : %v", err)
	}
	return chemin
}

// TestRunFetchAuDemarragePuisAuxTicks : passe au démarrage, passe à chaque
// tick (intervalle court de test), arrêt propre à l'annulation — plus
// aucun fetch lancé après l'arrêt.
func TestRunFetchAuDemarragePuisAuxTicks(t *testing.T) {
	var appels atomic.Int64
	urlM3U := serveurTexte(t, arteM3UCorps)
	b, dir := nouvelleBoutique(t)
	chemin := ecrireFichierDepot(t, dir, "30ms", urlM3U)
	// Compter les fetch : intercepter via une fenêtre réseau comptée.
	o := Nouveau(b, func(timeout time.Duration) Fetcher {
		interne := NouveauFetcher(timeout)
		return func(ctx context.Context, url string) (io.ReadCloser, error) {
			appels.Add(1)
			return interne(ctx, url)
		}
	}, 0, 1*time.Millisecond)
	loader := config.NewLoader(chemin)

	ctx, annule := context.WithCancel(context.Background())
	termine := make(chan struct{})
	go func() { o.Run(ctx, loader); close(termine) }()

	// Le démarrage fetch sans attendre le premier tick.
	échéance := time.Now().Add(3 * time.Second)
	for time.Now().Before(échéance) {
		if appels.Load() >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if appels.Load() < 3 {
		t.Fatalf("appels : %d, attendu au moins 3 (démarrage + ticks)", appels.Load())
	}

	annule()
	select {
	case <-termine:
	case <-time.After(2 * time.Second):
		t.Fatal("Run ne s'est pas arrêté après l'annulation")
	}
	aprèsArret := appels.Load()
	time.Sleep(100 * time.Millisecond)
	if appels.Load() != aprèsArret {
		t.Errorf("fetchs après l'arrêt : %d → %d", aprèsArret, appels.Load())
	}
}

// TestBouclerSauteLesTicksTombesPendantLaPasse : un tick qui tombe pendant
// une passe est VIDÉ (drain non bloquant) — il ne déclenche pas de passe
// immédiate à la fin de la passe ; le fetch suivant attend un tick neuf.
// Jamais de passes dos à dos : le fournisseur n'est pas martelé. Test
// déterministe : le canal de ticks est injecté, la passe est bloquée par
// un capot (gate).
func TestBouclerSauteLesTicksTombesPendantLaPasse(t *testing.T) {
	capot := make(chan struct{})
	ouvrirCapot := sync.OnceFunc(func() { close(capot) })
	entrées := make(chan struct{}, 8)
	serveurM3U := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entrées <- struct{}{}
		<-capot // la passe reste en cours tant que le capot est fermé
		_, _ = w.Write([]byte(arteM3UCorps))
	}))
	t.Cleanup(serveurM3U.Close)
	t.Cleanup(ouvrirCapot)
	b, dir := nouvelleBoutique(t)
	chemin := ecrireFichierDepot(t, dir, "1h", serveurM3U.URL)
	o := Nouveau(b, NouveauFetcher, 0, 1*time.Millisecond)
	loader := config.NewLoader(chemin)

	ticks := make(chan time.Time, 1) // tampon 1 : comme un Ticker
	resets := atomic.Int64{}

	ctx, annule := context.WithCancel(context.Background())
	termine := make(chan struct{})
	go func() { o.boucler(ctx, loader, ticks, func(time.Duration) { resets.Add(1) }); close(termine) }()

	// Tick 1 : la passe démarre et attend sur le capot.
	ticks <- time.Now()
	select {
	case <-entrées:
	case <-time.After(2 * time.Second):
		t.Fatal("la passe n'a pas démarré au premier tick")
	}
	// Tick 2 tombe PENDANT la passe : retenu par le tampon (comme un
	// vrai Ticker de tampon 1).
	ticks <- time.Now()

	// Fin de la passe : meta.json, écrit EN DERNIER, la signe.
	ouvrirCapot()
	attendre(t, "meta.json après la passe", func() bool {
		_, erreur := os.Stat(filepath.Join(dir, "meta.json"))
		return erreur == nil
	})

	// Le tick 2 devait être VIDÉ : aucune seconde passe immédiate.
	select {
	case <-entrées:
		t.Fatal("seconde passe déclenchée à la fin de la passe : le tick tombé pendant la passe n'a pas été vidé")
	case <-time.After(150 * time.Millisecond):
	}

	// Tick NEUF : la passe suivante démarre normalement.
	ticks <- time.Now()
	attendre(t, "passe déclenchée par le tick neuf", func() bool {
		select {
		case <-entrées:
			return true
		default:
			return false
		}
	})

	annule()
	select {
	case <-termine:
	case <-time.After(2 * time.Second):
		t.Fatal("boucler ne s'est pas arrêté à l'annulation")
	}
	if resets.Load() != 2 {
		t.Errorf("resets du ticker : %d, attendu 2 (deux passes, intervalle relu à chaque tick)", resets.Load())
	}
}

// TestArretProprePendantUnFetch : l'annulation pendant un fetch bloquant
// termine Run promptement — le contexte coupe la fenêtre réseau.
func TestArretProprePendantUnFetch(t *testing.T) {
	gate := make(chan struct{})
	entrée := make(chan struct{}, 1)
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entrée <- struct{}{}
		<-gate // fetch long : l'annulation du contexte le coupe
	}))
	t.Cleanup(serveur.Close)
	t.Cleanup(func() { close(gate) })

	b, dir := nouvelleBoutique(t)
	chemin := ecrireFichierDepot(t, dir, time.Hour.String(), serveur.URL)
	o := Nouveau(b, NouveauFetcher, 3, 1*time.Millisecond) // essais larges
	loader := config.NewLoader(chemin)

	ctx, annule := context.WithCancel(context.Background())
	termine := make(chan struct{})
	go func() { o.Run(ctx, loader); close(termine) }()

	select {
	case <-entrée:
	case <-time.After(2 * time.Second):
		t.Fatal("le fetch bloquant n'a pas commencé")
	}
	départ := time.Now()
	annule()
	select {
	case <-termine:
	case <-time.After(2 * time.Second):
		t.Fatal("Run ne s'est pas arrêté pendant le fetch bloquant")
	}
	if écoulé := time.Since(départ); écoulé > 2*time.Second {
		t.Errorf("arrêt lent : %v", écoulé)
	}
}

// TestKillPendantFetchRedemarrageDepuisDisque :
//
//	passe 1 réussie ;
//	« kill » = contexte annulé pendant le fetch bloquant de la passe 2 ;
//	zéro fichier partiel : copies, snapshots et meta.json restent ceux de
//	la passe 1 ;
//	redémarrage : un store neuf sur le même répertoire recharge l'état et
//	reconstruit un snapshot servable SANS AUCUN RÉSEAU.
func TestKillPendantFetchRedemarrageDepuisDisque(t2 *testing.T) {
	urlM3U := serveurTexte(t2, arteM3UCorps)
	urlXMLTV := serveurTexte(t2, arteXMLTVCorps)
	b, dir := nouvelleBoutique(t2)
	o := nouvelOrdonnanceur(b)
	cfg := cfgDeTest(dir, nil, config.Source{Name: "arte", M3UURL: urlM3U, XMLTVURL: urlXMLTV})

	// Passe 1 : réussie, tout est publié.
	if err := o.Passe(context.Background(), cfg); err != nil {
		t2.Fatalf("Passe 1 : %v", err)
	}
	snapshotAvant, err := os.ReadFile(b.CheminSnapshot("m3u"))
	if err != nil {
		t2.Fatalf("lecture du snapshot : %v", err)
	}
	metaAvant, err := b.Charger()
	if err != nil {
		t2.Fatalf("Charger avant kill : %v", err)
	}

	// Passe 2 : fetch bloquant puis « kill » (annulation en pleine passe).
	gate := make(chan struct{})
	entrée := make(chan struct{}, 1)
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entrée <- struct{}{}
		<-gate
	}))
	t2.Cleanup(serveur.Close)
	t2.Cleanup(func() { close(gate) })
	cfgKill := cfgDeTest(dir, nil, config.Source{Name: "arte", M3UURL: serveur.URL, XMLTVURL: urlXMLTV})

	ctx, annule := context.WithCancel(context.Background())
	fin := make(chan error, 1)
	go func() { fin <- o.Passe(ctx, cfgKill) }()
	select {
	case <-entrée:
	case <-time.After(2 * time.Second):
		t2.Fatal("le fetch bloquant n'a pas commencé")
	}
	départ := time.Now()
	annule()
	select {
	case <-fin:
	case <-time.After(2 * time.Second):
		t2.Fatal("la passe ne s'est pas terminée après le kill")
	}
	if écoulé := time.Since(départ); écoulé > 2*time.Second {
		t2.Errorf("kill lent : %v", écoulé)
	}

	// Aucun fichier partiel : snapshots/copies octet pour octet, aucun
	// temporaire résiduel, meta.json lisible et last_ok conservé.
	snapshotAprès, err := os.ReadFile(b.CheminSnapshot("m3u"))
	if err != nil || string(snapshotAprès) != string(snapshotAvant) {
		t2.Errorf("snapshot altéré par le kill : %d vs %d octets (%v)",
			len(snapshotAprès), len(snapshotAvant), err)
	}
	if err := aucunTemporaire(dir); err != nil {
		t2.Error(err)
	}
	metaAprès, err := b.Charger()
	if err != nil {
		t2.Fatalf("meta.json illisible après kill : %v", err)
	}
	if metaAprès.Sources["arte"].M3U == nil || metaAprès.Sources["arte"].M3U.DernierOK == nil {
		t2.Errorf("last_ok perdu après kill : %+v", metaAprès.Sources["arte"].M3U)
	}
	if metaAprès.Sources["arte"].M3U != nil &&
		metaAprès.Sources["arte"].M3U.DerniereErreur == "" {
		t2.Errorf("erreur du kill non tracée : %+v", metaAprès.Sources["arte"].M3U)
	}
	if metaAprès.Sources["arte"].M3U != nil &&
		!metaAprès.Sources["arte"].M3U.DernierOK.Equal(*metaAvant.Sources["arte"].M3U.DernierOK) {
		t2.Errorf("last_ok changé sans réussite : %v", metaAprès.Sources["arte"].M3U.DernierOK)
	}

	// Redémarrage : store neuf, état relu, snapshot reconstruit depuis le
	// disque seul — la source est opérationnelle sans réseau.
	bouteille, err := store.Ouvrir(dir)
	if err != nil {
		t2.Fatalf("Ouvrir (redémarrage) : %v", err)
	}
	metaRedemarre, err := bouteille.Charger()
	if err != nil {
		t2.Fatalf("Charger (redémarrage) : %v", err)
	}
	if len(metaRedemarre.Sources) == 0 {
		t2.Fatalf("état vide au redémarrage : %+v", metaRedemarre)
	}
	if err := bouteille.Reconstruire(
		[]string{bouteille.CheminCopie("arte", "m3u")},
		[]string{bouteille.CheminCopie("arte", "xmltv")}); err != nil {
		t2.Fatalf("Reconstruire (redémarrage, sans réseau) : %v", err)
	}
	redemarre, err := os.ReadFile(bouteille.CheminSnapshot("m3u"))
	if err != nil || string(redemarre) != string(snapshotAvant) {
		t2.Errorf("snapshot au redémarrage : %q (%v), attendu %q", redemarre, err, snapshotAvant)
	}
	// Servable : balayable, entrées complètes et dédupliquées.
	f, err := os.Open(bouteille.CheminSnapshot("m3u"))
	if err != nil {
		t2.Fatalf("ouverture du snapshot : %v", err)
	}
	defer f.Close()
	var identites []string
	if err := m3u.Balayer(f, nil, func(c m3u.Chaine, _ []string) error {
		identites = append(identites, c.Identite())
		return nil
	}); err != nil {
		t2.Fatalf("Balayer du snapshot : %v", err)
	}
	if strings.Join(identites, ",") != "arte.fr,bfm.fr" {
		t2.Errorf("identités servies : %v", identites)
	}
}

// aucunTemporaire signale tout fichier temporaire résiduel sous le
// répertoire de données.
func aucunTemporaire(dir string) error {
	return filepath.Walk(dir, func(chemin string, info os.FileInfo, erreur error) error {
		if erreur != nil || info.IsDir() {
			return erreur
		}
		if strings.HasPrefix(filepath.Base(chemin), ".tmp-") {
			return &residu{nom: chemin}
		}
		return nil
	})
}

type residu struct {
	nom string
}

func (r *residu) Error() string { return "temporaire résiduel : " + r.nom }

// attendre guette jusqu'à ce que condition devienne vraie (poll court),
// avec une borne ferme — les pollings de tests ne doivent jamais pendre.
func attendre(t *testing.T, quoi string, condition func() bool) {
	t.Helper()
	échéance := time.Now().Add(2 * time.Second)
	for time.Now().Before(échéance) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("attendu %s dans le délai", quoi)
}
