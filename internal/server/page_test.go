package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/store"
)

// recetteDashboard : serveur complet sur un data_dir vierge — le dashboard
// n'exige aucun snapshot, GET / se sert quoi qu'il arrive.
func recetteDashboard(t *testing.T) *httptest.Server {
	t.Helper()
	boutique, err := store.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	recette := httptest.NewServer(
		Nouveau(boutique, func() config.Config { return config.Config{} }).Handler(),
	)
	t.Cleanup(recette.Close)
	return recette
}

// TestTableauBordEmbarqué : GET / sert le dashboard EMBARQUÉ dans le
// binaire (frontend.Dist, embed.FS) — la page entière se sert sans le
// moindre asset externe (story 18, critère d'acceptation « binary alone,
// offline OK »). Le test tourne sur le FS intégré réel : ce qui passe ici
// passe dans le binaire.
func TestTableauBordEmbarqué(t *testing.T) {
	recette := recetteDashboard(t)

	code, entêtes, corps := obtenir(t, recette.URL+"/")
	if code != http.StatusOK {
		t.Fatalf("/ : code %d, voulu 200", code)
	}
	if type_ := entêtes.Get("Content-Type"); !strings.HasPrefix(type_, "text/html") {
		t.Errorf("/ : Content-Type %q, voulu du text/html", type_)
	}
	page := string(corps)
	for _, attendu := range []string{`lang="fr"`, "zapette", `rel="icon"`} {
		if !strings.Contains(page, attendu) {
			t.Errorf("/ : %q absent de la page embarquée (lang fr, titre, favicon)", attendu)
		}
	}

	// La favicon embarquée se sert sur le binaire seul (offline OK,
	// story 18) avec son type SVG.
	codeFavicon, entêtesFavicon, _ := obtenir(t, recette.URL+"/favicon.svg")
	if codeFavicon != http.StatusOK {
		t.Errorf("/favicon.svg : code %d, voulu 200 (embarquée via l'embed)", codeFavicon)
	}
	if typeFavicon := entêtesFavicon.Get("Content-Type"); !strings.Contains(typeFavicon, "image/svg+xml") {
		t.Errorf("/favicon.svg : Content-Type %q, voulu image/svg+xml", typeFavicon)
	}

	// Les assets hachés du build Vite sont résolus par le même embed :
	// chaque /assets/Header référencé dans index.html doit se servir avec
	// son type. Sans build encore produit, la page de repli n'en référence
	// pas — on saute (le binaire reste fonctionnel, page text/html seule).
	assetPaths := assetPathsDe(page)
	if assetPaths == nil {
		t.Skip("aucun asset référencé sur la page : build Vite absent du dépôt")
	}
	for chemin := range assetPaths {
		code, entêtes, _ := obtenir(t, recette.URL+chemin)
		if code != http.StatusOK {
			t.Errorf("%s : code %d, voulu 200", chemin, code)
		}
		type_ := entêtes.Get("Content-Type")
		switch {
		case strings.HasSuffix(chemin, ".js"):
			if !strings.HasPrefix(type_, "text/javascript") && !strings.HasPrefix(type_, "application/javascript") {
				t.Errorf("%s : Content-Type %q, voulu du JS", chemin, type_)
			}
		case strings.HasSuffix(chemin, ".css"):
			if !strings.HasPrefix(type_, "text/css") {
				t.Errorf("%s : Content-Type %q, voulu du CSS", chemin, type_)
			}
		}
	}
}

// TestTableauBordCheminInconnu : une route inconnue sous « / » reste un
// 404 — le fourre-tout du dashboard ne masque pas les frappes.
func TestTableauBordCheminInconnu(t *testing.T) {
	recette := recetteDashboard(t)
	if code, _, _ := obtenir(t, recette.URL+"/jamais-ce-chemin"); code != http.StatusNotFound {
		t.Errorf("/jamais-ce-chemin : code %d, voulu 404", code)
	}
}

// assetPathsDe extrait les /assets/… référencés par l'index (src et href
// du build Vite). Retourne nil si la page n'en référence aucun.
func assetPathsDe(page string) map[string]bool {
	assets := map[string]bool{}
	for _, motif := range []string{`src="`, `href="`} {
		morceaux := strings.Split(page, motif)
		for _, bout := range morceaux[1:] {
			chemin, _, trouvé := strings.Cut(bout, `"`)
			if trouvé && strings.HasPrefix(chemin, "/assets/") {
				assets[chemin] = true
			}
		}
	}
	if len(assets) == 0 {
		return nil
	}
	return assets
}
