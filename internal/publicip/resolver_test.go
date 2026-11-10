package publicip

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
)

// fenetreReelle : une fenêtre réseau de test identique à la production
// (fetch.HTTP avec le timeout reçu), pour des serveurs httptest réels.
func fenetreReelle(timeout time.Duration) Fetcher {
	client := &http.Client{Timeout: timeout}
	return func(ctx context.Context, url string) (io.ReadCloser, error) {
		requete, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		reponse, err := client.Do(requete)
		if err != nil {
			return nil, err
		}
		return reponse.Body, nil
	}
}

// monterEcho monte un serveur echo qui répond le corps indiqué ; il joue
// le rôle d'un endpoint ipify de test.
func monterEcho(t *testing.T, corps string) string {
	t.Helper()
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(corps))
	}))
	t.Cleanup(serveur.Close)
	return serveur.URL
}

// TestVerifierExposeIPV4EtIPV6 : le résolveur fetch les deux endpoints de
// la config au démarrage (Verifier) et expose les adresses avec
// l'horodatage de la vérification — l'état lu par /api/status.
func TestVerifierExposeIPV4EtIPV6(t *testing.T) {
	urlV4 := monterEcho(t, "203.0.113.7")
	urlV6 := monterEcho(t, "2001:db8::42")

	cfg := config.Config{PublicIPv4URL: urlV4, PublicIPv6URL: urlV6}
	resolver := Nouveau(func() config.Config { return cfg }, fenetreReelle)

	resolver.Verifier(context.Background())

	etat := resolver.Etat()
	if etat.IPv4 != "203.0.113.7" {
		t.Errorf("IPv4 = %q, voulu 203.0.113.7", etat.IPv4)
	}
	if etat.IPv6 != "2001:db8::42" {
		t.Errorf("IPv6 = %q, voulu 2001:db8::42", etat.IPv6)
	}
	if etat.DerniereVerification == nil {
		t.Fatal("DerniereVerification nil, voulu l'horodatage de la passe")
	}
	if age := time.Since(*etat.DerniereVerification); age < 0 || age > 5*time.Second {
		t.Errorf("DerniereVerification = %v, voulu environ maintenant", *etat.DerniereVerification)
	}
}

// TestEtatInitialIndisponible : avant toute vérification, l'état exposé est
// « indisponible » des deux côtés et sans horodatage — le contrat figuré du
// champ /api/status, tenu dès l'instanciation.
func TestEtatInitialIndisponible(t *testing.T) {
	resolver := Nouveau(func() config.Config { return config.Config{} }, fenetreReelle)

	etat := resolver.Etat()
	if etat.IPv4 != "indisponible" {
		t.Errorf("IPv4 initial = %q, voulu \"indisponible\"", etat.IPv4)
	}
	if etat.IPv6 != "indisponible" {
		t.Errorf("IPv6 initial = %q, voulu \"indisponible\"", etat.IPv6)
	}
	if etat.DerniereVerification != nil {
		t.Errorf("DerniereVerification initiale = %v, voulu nil", *etat.DerniereVerification)
	}
}

// TestVerifierEnEchecResteIndisponible : endpoint en panne (offline du
// fournisseur echo) → pas de crash, pas d'erreur bloquante, l'état retombe
// sur « indisponible » et l'horodatage de la passe est quand même avancé —
// une vérification A bien eu lieu, même infirme.
func TestVerifierEnEchecResteIndisponible(t *testing.T) {
	cfg := config.Config{
		PublicIPv4URL: "http://127.0.0.1:1/ipv4", // rien n'écoute sur le port 1
		PublicIPv6URL: "http://127.0.0.1:1/ipv6",
	}
	resolver := Nouveau(func() config.Config { return cfg }, fenetreReelle)

	// L'échec réseau ne bloque ni n'avorte : l'échec est journalisé dans
	// resoudre et visible dans Etat, jamais propagé.
	resolver.Verifier(context.Background())

	etat := resolver.Etat()
	if etat.IPv4 != "indisponible" || etat.IPv6 != "indisponible" {
		t.Errorf("état en échec = (%q, %q), voulu (« indisponible », « indisponible »)", etat.IPv4, etat.IPv6)
	}
	if etat.DerniereVerification == nil {
		t.Error("DerniereVerification nil en échec, voulu l'horodatage de la passe")
	}
}

// TestVerifierTransmetTimeoutCinq : la fenêtre réseau est ouverte avec le
// timeout fixe de 5 s — le contrat de réactivité des endpoints echo.
func TestVerifierTransmetTimeoutCinq(t *testing.T) {
	var recu time.Duration
	fenetre := func(timeout time.Duration) Fetcher {
		recu = timeout
		return func(context.Context, string) (io.ReadCloser, error) {
			return nil, context.Canceled
		}
	}
	resolver := Nouveau(func() config.Config { return config.Config{} }, fenetre)
	resolver.Verifier(context.Background())

	if recu != Timeout {
		t.Errorf("timeout transmis = %v, voulu %v", recu, Timeout)
	}
}

// AttendrePollue attend jusqu'à ce que la condition soit vraie : la boucle
// résolveur vit dans une goroutine, la synchronisation passe par l'état
// exposé (Etat) et les effets de bord observables, avec un plafond.
func AttendrePollue(t *testing.T, quoi string, condition func() bool) {
	t.Helper()
	attendu := time.Now().Add(2 * time.Second)
	for time.Now().Before(attendu) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition jamais remplie : %s", quoi)
}

// TestBouclerRelitLaConfigAChaqueTick : à chaque tick, la passe relit la
// config COURANTE (même famille que loader.Relire côté fetch) — un endpoint
// modifié à chaud est suivi sans redémarrage — et l'intervalle relu est
// réinjecté au ticker (reinitialiser).
func TestBouclerRelitLaConfigAChaqueTick(t *testing.T) {
	urlUn := monterEcho(t, "198.51.100.1")
	urlDeux := monterEcho(t, "198.51.100.2")

	var version atomic.Int64
	lireConfig := func() config.Config {
		if version.Load() == 0 {
			return config.Config{PublicIPv4URL: urlUn, PublicIPv6URL: urlDeux, RefreshInterval: time.Hour}
		}
		return config.Config{PublicIPv4URL: urlDeux, PublicIPv6URL: urlUn, RefreshInterval: 2 * time.Hour}
	}
	resolver := Nouveau(lireConfig, fenetreReelle)

	// Passe de démarrage (comme Run) : la config version 0.
	resolver.Verifier(context.Background())

	ticks := make(chan time.Time)
	var resets []time.Duration
	reinitialiser := func(intervalle time.Duration) { resets = append(resets, intervalle) }

	ctx, annule := context.WithCancel(context.Background())
	termine := make(chan struct{})
	go func() {
		resolver.boucler(ctx, ticks, reinitialiser)
		close(termine)
	}()
	defer func() {
		annule()
		select {
		case <-termine:
		case <-time.After(2 * time.Second):
			t.Fatal("boucler ne s'est pas arrêté à l'annulation")
		}
	}()

	// Tick : la config a basculé en version 1 (endpoint IPv4 différent).
	version.Store(1)
	ticks <- time.Now()
	AttendrePollue(t, "état mis à jour depuis la config relue", func() bool {
		return resolver.Etat().IPv4 == "198.51.100.2"
	})
	if len(resets) != 1 || resets[0] != 2*time.Hour {
		t.Errorf("reinitialiser appelé avec %v, voulu [2h]", resets)
	}
}

// TestBouclerSauteLesTicksTombesPendantLaPasse : un tick qui tombe pendant
// une passe est VIDÉ (drain non bloquant) — les endpoints echo ne sont pas
// martelés de passes dos à dos, symétrique à l'ordonnanceur des fetchs.
func TestBouclerSauteLesTicksTombesPendantLaPasse(t *testing.T) {
	var (
		verrou       sync.Mutex
		appels       int
		débloc       = make(chan struct{})
		ouvrirDébloc = sync.OnceFunc(func() { close(débloc) })
	)
	resolver := Nouveau(
		func() config.Config { return config.Config{} },
		func(time.Duration) Fetcher {
			return func(context.Context, string) (io.ReadCloser, error) {
				verrou.Lock()
				appels++
				verrou.Unlock()
				<-débloc
				return nil, context.Canceled
			}
		},
	)

	ticks := make(chan time.Time, 1) // tampon 1 : comme un Ticker
	ctx, annule := context.WithCancel(context.Background())
	termine := make(chan struct{})
	go func() {
		resolver.boucler(ctx, ticks, func(time.Duration) {})
		close(termine)
	}()
	defer func() {
		ouvrirDébloc()
		annule()
		select {
		case <-termine:
		case <-time.After(2 * time.Second):
			t.Fatal("boucler ne s'est pas arrêté à l'annulation")
		}
	}()

	// Tick 1 : la passe démarre et gèle sur le fetch IPv4 (passe
	// séquentielle v4 puis v6).
	ticks <- time.Now()
	AttendrePollue(t, "première passe démarrée", func() bool {
		verrou.Lock()
		defer verrou.Unlock()
		return appels == 1
	})
	// Tick 2 tombe PENDANT la passe : retenu par le tampon.
	ticks <- time.Now()

	// Fin de la passe : le tick 2 devait être VIDÉ, aucune seconde passe.
	ouvrirDébloc()
	time.Sleep(150 * time.Millisecond)
	verrou.Lock()
	aprèsPasse := appels
	verrou.Unlock()
	if aprèsPasse != 2 {
		t.Errorf("appels = %d, voulu 2 : le tick tombé pendant la passe n'a pas été vidé", aprèsPasse)
	}

	// Tick NEUF : la passe suivante démarre normalement.
	ticks <- time.Now()
	AttendrePollue(t, "passe déclenchée par le tick neuf", func() bool {
		verrou.Lock()
		defer verrou.Unlock()
		return appels == 4
	})
}
