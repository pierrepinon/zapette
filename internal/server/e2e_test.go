package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pierrepinon/zapette/internal/store"
)

// dE2E prépare un serveur complet (handlers réels) in-process sur un
// répertoire de données vierge : la couture de test du ticket — le serveur
// entier est derrière httptest, upstream compris, aucune dépendance au
// réseau réel.
func dE2E(t *testing.T) *httptest.Server {
	t.Helper()
	boutique, err := store.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	s := Nouveau(boutique)
	recette := httptest.NewServer(s.Handler())
	t.Cleanup(recette.Close)
	return recette
}

// TestFroidSansSnapshot : au démarrage, avant la première passe réussie,
// aucun snapshot n'existe sur disque. /m3u et /xmltv répondent 503 avec un
// corps JSON explicite — l'échec doit être visible, jamais silencieux
// (story 10) — et GET / reste accessible.
func TestFroidSansSnapshot(t *testing.T) {
	recette := dE2E(t)

	for _, artefact := range []string{"/m3u", "/xmltv"} {
		reponse, err := http.Get(recette.URL + artefact)
		if err != nil {
			t.Fatalf("GET %s : %v", artefact, err)
		}
		defer reponse.Body.Close()
		if reponse.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("%s : code %d, voulu 503", artefact, reponse.StatusCode)
		}
		if type_ := reponse.Header.Get("Content-Type"); type_ != "application/json; charset=utf-8" {
			t.Errorf("%s : Content-Type %q, voulu application/json; charset=utf-8", artefact, type_)
		}
		var corps struct {
			Erreur string `json:"erreur"`
		}
		if err := json.NewDecoder(reponse.Body).Decode(&corps); err != nil {
			t.Errorf("%s : corps non JSON : %v", artefact, err)
		} else if corps.Erreur == "" {
			t.Errorf("%s : 503 sans message d'erreur", artefact)
		}
	}

	page, err := http.Get(recette.URL + "/")
	if err != nil {
		t.Fatalf("GET / : %v", err)
	}
	defer page.Body.Close()
	if page.StatusCode != http.StatusOK {
		t.Errorf("/ : code %d, voulu 200", page.StatusCode)
	}
}
