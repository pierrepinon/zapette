package main

import (
	"io"
	"net/http"
	"testing"

	"github.com/pierrepinon/zapette/internal/config"
)

// TestAdresseEcoute : l'adresse d'écoute suit la config (server_addr,
// défaut [::]:8080) et ZAPETTE_ADDR la priorise — la variable
// d'environnement est le moyen de surcharger sans éditer le fichier.
func TestAdresseEcoute(t *testing.T) {
	t.Setenv("ZAPETTE_ADDR", "")

	if adresse := adresseEcoute(config.Config{}); adresse != "[::]:8080" {
		t.Errorf("défaut : %q, voulu [::]:8080", adresse)
	}
	if adresse := adresseEcoute(config.Config{ServerAddr: "[::]:9999"}); adresse != "[::]:9999" {
		t.Errorf("adresse de config : %q, voulu [::]:9999", adresse)
	}

	t.Setenv("ZAPETTE_ADDR", "[::]:7777")
	if adresse := adresseEcoute(config.Config{ServerAddr: "[::]:9999"}); adresse != "[::]:7777" {
		t.Errorf("env prioritaire : %q, voulu [::]:7777", adresse)
	}
}

// TestDemarrerHTTP : démarrage sur un port attribué par le noyau, réponse
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
