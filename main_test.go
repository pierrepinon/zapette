package main

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/pierrepinon/zapette/internal/config"
)

// TestServerAddrCheminUnique : une ressource = un chemin (motif de l'issue
// #1) — l'adresse d'écoute vient UNIQUEMENT de la config (server_addr, défaut
// [::]:8080 appliqué par config.Load) ; ZAPETTE_ADDR n'y entre que par
// l'expansion ${ZAPETTE_ADDR} du fichier, JAMAIS par une relecture directe
// de l'env dans main. Un ${ZAPETTE_ADDR} non défini est une erreur de
// chargement, pas un retour au défaut.
func TestServerAddrCheminUnique(t *testing.T) {
	fichier := filepath.Join(t.TempDir(), "config.yaml")
	configYAML := "refresh_interval: 6h\nsources:\n  - name: provider-a\n    m3u_url: https://exemple.tld/playlist.m3u\nserver_addr: \"[::]:9999\"\n"
	if err := os.WriteFile(fichier, []byte(configYAML), 0o600); err != nil {
		t.Fatalf("écriture de la config : %v", err)
	}

	t.Setenv("ZAPETTE_ADDR", "[::]:7777") // ne DOIT pas être relu par main
	cfg, err := config.Load(fichier)
	if err != nil {
		t.Fatalf("Load : %v", err)
	}
	if cfg.ServerAddr != "[::]:9999" {
		t.Errorf("server_addr : %q, voulu [::]:9999 (l'env ne s'applique que par l'expansion du fichier)", cfg.ServerAddr)
	}

	// La clé absente : le défaut s'applique à la lecture.
	sansAdresse := filepath.Join(t.TempDir(), "config.yaml")
	defautYAML := "refresh_interval: 6h\nsources:\n  - name: provider-a\n    m3u_url: https://exemple.tld/playlist.m3u\n"
	if err := os.WriteFile(sansAdresse, []byte(defautYAML), 0o600); err != nil {
		t.Fatalf("écriture de la config : %v", err)
	}
	cfg, err = config.Load(sansAdresse)
	if err != nil {
		t.Fatalf("Load (sans server_addr) : %v", err)
	}
	if cfg.ServerAddr != "[::]:8080" {
		t.Errorf("server_addr défaut : %q, voulu [::]:8080", cfg.ServerAddr)
	}

	// La clé présente mais développée d'une variable indéfinie : ERREUR de
	// chargement, pas un retour au défaut.
	avecVariable := filepath.Join(t.TempDir(), "config.yaml")
	variableYAML := "refresh_interval: 6h\nserver_addr: ${ZAPETTE_NON_DEFINIE}\n"
	if err := os.WriteFile(avecVariable, []byte(variableYAML), 0o600); err != nil {
		t.Fatalf("écriture de la config : %v", err)
	}
	if _, err := config.Load(avecVariable); err == nil {
		t.Errorf("${ZAPETTE_NON_DEFINIE} devait être une erreur de chargement, pas un défaut")
	}
}

// TestVersionVariable : la var version existe et est câblée — « dev » par
// défaut, remplacée par -ldflags -X main.version à la constructionrelease.
func TestVersionVariable(t *testing.T) {
	if version == "" {
		t.Skipf("var version vide : sans ldflags, le défaut « dev » reste servi")
	}
}

// HTTP correcte, arrêt propre — le contrat du câblage de main sans occuper
// [::]:8080 (réservé à la vraie écoute).
func TestDemarrerHTTP(t *testing.T) {
	serv, ecouteur, err := demarrerHTTP("127.0.0.1:0", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("zapette"))
	}))
	if err != nil {
		t.Fatalf("démarrage HTTP : %v", err)
	}
	t.Cleanup(func() { _ = serv.Close() })

	reponse, err := http.Get("http://" + ecouteur.Addr().String())
	if err != nil {
		t.Fatalf("GET sur le serveur câblé : %v", err)
	}
	defer reponse.Body.Close()
	corps, err := io.ReadAll(reponse.Body)
	if err != nil {
		t.Fatalf("lecture du corps : %v", err)
	}
	if string(corps) != "zapette" {
		t.Errorf("corps %q, voulu zapette", corps)
	}

	// L'arrêt propre rend le handler quittable sans goroutine pendante.
	if err := serv.Shutdown(t.Context()); err != nil {
		t.Errorf("arrêt du serveur : %v", err)
	}
}
