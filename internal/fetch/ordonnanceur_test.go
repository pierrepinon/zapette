package fetch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// TestRecupererEssaisEtBackoffFixe : premier essai puis essais reprises,
// backoff fixe entre chaque, succès au premier corps utile — chaque échec
// accusé, le temps d'attente cumulé respecté.
func TestRecupererEssaisEtBackoffFixe(t *testing.T) {
	var appels atomic.Int64
	faux := func(ctx context.Context, url string) (io.ReadCloser, error) {
		n := appels.Add(1)
		if n <= 2 {
			return nil, errors.New("réseau indisponible")
		}
		return io.NopCloser(strings.NewReader("corps")), nil
	}
	o := Nouveau(nil, nil, 2, 1*time.Millisecond)

	départ := time.Now()
	corps, err := o.recuperer(context.Background(), faux, "https://exemple.tld/m3u")
	if err != nil {
		t.Fatalf("recuperer : %v", err)
	}
	defer corps.Close()
	if appels.Load() != 3 {
		t.Errorf("appels : %d, attendu 3 (1+2 essais)", appels.Load())
	}
	if attendu := 2 * 1 * time.Millisecond; time.Since(départ) < attendu {
		t.Errorf("backoff fixe non respecté : %v sans les %v d'attente", time.Since(départ), attendu)
	}
}

// TestRecupererEpuiseLesEssais : échec total → 1+N tentatives, l'erreur du
// dernier essai est celle remontée.
func TestRecupererEpuiseLesEssais(t *testing.T) {
	var appels atomic.Int64
	faux := func(ctx context.Context, url string) (io.ReadCloser, error) {
		appels.Add(1)
		return nil, errors.New("statut HTTP 500")
	}
	o := Nouveau(nil, nil, 3, 1*time.Millisecond)

	corps, err := o.recuperer(context.Background(), faux, "https://exemple.tld/m3u")
	if err == nil {
		t.Fatal("recuperer devait échouer")
	}
	if corps != nil {
		t.Error("corps devait être nil")
	}
	if appels.Load() != 4 {
		t.Errorf("appels : %d, attendu 4 (1+3 essais)", appels.Load())
	}
}

// TestRecupererSArreteALAnnulation : l'interruption du contexte pendant le
// backoff coupe la série d'essais immédiatement.
func TestRecupererSArreteALAnnulation(t *testing.T) {
	var appels atomic.Int64
	faux := func(ctx context.Context, url string) (io.ReadCloser, error) {
		appels.Add(1)
		return nil, errors.New("réseau indisponible")
	}
	o := Nouveau(nil, nil, 3, 5*time.Second)

	ctx, annule := context.WithCancel(context.Background())
	annule()
	if _, err := o.recuperer(ctx, faux, "https://exemple.tld/m3u"); err == nil {
		t.Fatal("recuperer devait échouer, contexte annulé")
	}
	if appels.Load() != 1 {
		t.Errorf("appels : %d, attendu 1 (annulation pendant le backoff)", appels.Load())
	}
}

// TestReseauStatutHTTP : un statut HTTP hors 2xx est une erreur.
func TestReseauStatutHTTP(t *testing.T) {
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer serveur.Close()

	if _, err := NouveauFetcher(2*time.Second)(context.Background(), serveur.URL); err == nil {
		t.Fatal("le statut 500 devait être une erreur")
	}
}

// TestReseauTimeout : le timeout de la fenêtre réseau est celui construit.
func TestReseauTimeout(t *testing.T) {
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer serveur.Close()

	départ := time.Now()
	_, err := NouveauFetcher(50*time.Millisecond)(context.Background(), serveur.URL)
	if err == nil {
		t.Fatal("le délai dépassé devait être une erreur")
	}
	if time.Since(départ) > 2*time.Second {
		t.Errorf("timeout non respecté : %v", time.Since(départ))
	}
}

// TestReseauSertLesLecteurs : un corps de serveur est servi tel quel et
// fermable — le contrat minimal de la fenêtre réseau, le filtrage n'attend
// jamais tout le corps en RAM (ADR-0001).
func TestReseauSertLesLecteurs(t *testing.T) {
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("#EXTM3U\n"))
	}))
	defer serveur.Close()

	corps, err := NouveauFetcher(2*time.Second)(context.Background(), serveur.URL)
	if err != nil {
		t.Fatalf("recuperer : %v", err)
	}
	defer corps.Close()
	lu, err := io.ReadAll(corps)
	if err != nil {
		t.Fatalf("lecture du corps : %v", err)
	}
	if string(lu) != "#EXTM3U\n" {
		t.Errorf("corps : %q", lu)
	}
}

// TestReseauErreurConnexion : une connexion refusée remonte en erreur — le
// fetch réseau n'avale jamais un échec de connexion.
func TestReseauErreurConnexion(t *testing.T) {
	if _, err := NouveauFetcher(time.Second)(context.Background(), "http://127.0.0.1:1/x"); err == nil {
		t.Fatal("connexion refusée devait être une erreur")
	}
}
