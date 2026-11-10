package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pierrepinon/zapette/internal/config"
)

// TestPasseSignaleEchecsDeSource : chaque échec par source déjà journalisé
// dans Passe est aussi SIGNALÉ au signaleur injecté (champ optionnel,
// nil-safe) — c'est la couture que main connecte au ring d'erreurs du
// serveur ; fetch ne connaît jamais server, la dépendance est inversée.
func TestPasseSignaleEchecsDeSource(t *testing.T) {
	var messages []string

	// Upstream : le m3u réussit, le xmltv répond 500 — deux sites d'échec
	// distincts à couvrir (fetch en échec pour le guide, reperage par
	// artefact).
	m3uCorps := "#EXTM3U\n#EXTINF:-1 tvg-id=\"a.fr\" group-title=\"G\",A\nhttps://flux/a\n"
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "xmltv") {
			http.Error(w, "panne guide", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(m3uCorps))
	}))
	t.Cleanup(serveur.Close)

	boutique, dir := nouvelleBoutique(t)
	o := nouvelOrdonnanceur(boutique).AvecSignaleur(func(message string) {
		messages = append(messages, message)
	})

	cfg := cfgDeTest(dir, nil, config.Source{
		Name:     "arte",
		M3UURL:   serveur.URL + "/m3u",
		XMLTVURL: serveur.URL + "/xmltv",
	})
	if err := o.Passe(context.Background(), cfg); err != nil {
		t.Fatalf("Passe : %v", err)
	}

	if len(messages) == 0 {
		t.Fatal("aucune erreur signalée alors que le fetch xmltv a échoué")
	}
	// Le contrat du signaleur : la source et l'artefact sont identifiables
	// (l'opérateur voit dans /api/status ce que montrent les logs) ; le
	// motif amont exact est une concern du fetcher, pas du ring.
	for _, attendu := range []string{"arte", "xmltv", "échec"} {
		trouve := false
		for _, message := range messages {
			if strings.Contains(message, attendu) {
				trouve = true
			}
		}
		if !trouve {
			t.Errorf("ring : aucun message ne contient %q ; reçus %q", attendu, messages)
		}
	}
}

// TestPasseSansSignaleur : le signaleur est OPTIONNEL — une construction
// via Nouveau seul (nil) ne panique pas, la passe se déroule normalement.
func TestPasseSansSignaleur(t *testing.T) {
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "panne", http.StatusInternalServerError)
	}))
	t.Cleanup(serveur.Close)

	boutique, dir := nouvelleBoutique(t)
	o := nouvelOrdonnanceur(boutique) // pas de signaleur

	cfg := cfgDeTest(dir, nil, config.Source{Name: "arte", M3UURL: serveur.URL})
	if err := o.Passe(context.Background(), cfg); err != nil {
		t.Fatalf("Passe sans signaleur : %v", err)
	}
}

// TestPasseSignaleErreurDePasse : une erreur LOCALE de passe (meta.json
// illisible → « lecture de l'état ») sort aussi dans le signaleur — le
// chemin de Run/boucler qui enrobe chaque passe (passerEnSignalant).
func TestPasseSignaleErreurDePasse(t *testing.T) {
	var messages []string

	boutique, dir := nouvelleBoutique(t)
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), []byte("{rompu"), 0o644); err != nil {
		t.Fatalf("écriture meta.json rompu : %v", err)
	}
	o := nouvelOrdonnanceur(boutique).AvecSignaleur(func(message string) {
		messages = append(messages, message)
	})

	cfg := cfgDeTest(dir, nil, config.Source{Name: "arte", M3UURL: "http://injoignable.test/m3u"})
	if err := o.passerEnSignalant(context.Background(), cfg); err == nil {
		t.Fatal("passerEnSignalant : erreur attendue (meta.json illisible)")
	}
	if len(messages) == 0 {
		t.Fatal("erreur de passe non signalée")
	}
	for _, message := range messages {
		if !strings.Contains(message, "lecture de l'état") {
			t.Errorf("message sans la cause : %q", message)
		}
	}
}
