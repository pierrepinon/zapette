package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/store"
)

// Matériaux /api/canaux (issue #11) : deux sources M3U (arte gagne le
// doublon arte.fr, miroir apporte cnn.fr et une chaîne sans attribut) et un
// guide qui publie arte.fr et cnn.fr — écrits par le store RÉEL
// (Reconstruire), la même fabrique que la production : le handler vit sur
// la même frontière que la passe de fetch.
const (
	canauxJSONArte = `#EXTM3U
#EXTINF:-1 tvg-id="arte.fr" tvg-chno="4" tvg-logo="https://img.tld/arte.png" group-title="Sports",Arte
https://flux.exemple.tld/arte.m3u8
#EXTINF:-1 tvg-id="bfm.fr" tvg-chno="5" group-title="News",BFM
https://flux.exemple.tld/bfm.m3u8
#EXTINF:-1 tvg-id="info.fr" group-title="News",Info
https://flux.exemple.tld/info.m3u8
`
	canauxJSONMiroir = `#EXTM3U
#EXTINF:-1 tvg-id="arte.fr" tvg-chno="9" group-title="Sports",Arte miroir
https://miroir.tld/arte.m3u8
#EXTINF:-1 tvg-id="cnn.fr" tvg-chno="7" group-title="News",CNN
https://miroir.tld/cnn.m3u8
#EXTINF:-1,Sans rien
https://miroir.tld/nue.m3u8
`
	canauxJSONXMLTV = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="arte.fr"><display-name>Arte</display-name></channel>
  <channel id="cnn.fr"><display-name>CNN</display-name></channel>
</tv>
`
)

// recetteCanaux monte un serveur de recette sur un data_dir rempli par
// Reconstruire. Snapshot attendu, dans l'ordre du merge :
//
//	Arte      (arte,   Sports) source_xmltv=arte ; BFM, Info (arte, News) ;
//	CNN       (miroir, News)   source_xmltv=arte (le guide arte publie l'id) ;
//	« Sans rien » (miroir, groupe "") : numero/logo/tvg_id/source_xmltv null.
func recetteCanaux(t *testing.T) (*store.Store, *httptest.Server) {
	t.Helper()
	boutique, err := store.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	for nom, contenu := range map[string]string{
		"arte":   canauxJSONArte,
		"miroir": canauxJSONMiroir,
	} {
		if err := boutique.EcrireCopie(nom, "m3u", remplirDe(contenu)); err != nil {
			t.Fatalf("EcrireCopie %s : %v", nom, err)
		}
	}
	if err := boutique.EcrireCopie("arte", "xmltv", remplirDe(canauxJSONXMLTV)); err != nil {
		t.Fatalf("EcrireCopie xmltv : %v", err)
	}
	recette := httptest.NewServer(
		Nouveau(boutique, func() config.Config { return config.Config{} }).Handler(),
	)
	t.Cleanup(recette.Close)
	return boutique, recette
}

// reconstruireRecette rejoue la fusion sur les copies de la boutique.
func reconstruireRecette(t *testing.T, boutique *store.Store) {
	t.Helper()
	if err := boutique.Reconstruire(
		[]string{boutique.CheminCopie("arte", "m3u"), boutique.CheminCopie("miroir", "m3u")},
		[]string{boutique.CheminCopie("arte", "xmltv")},
	); err != nil {
		t.Fatalf("Reconstruire : %v", err)
	}
}

// remplirDe : copie le texte dans l'écrivain (l'helper ecrire du package
// store est hors de portée).
func remplirDe(texte string) func(io.Writer) error {
	return func(w io.Writer) error {
		_, err := io.WriteString(w, texte)
		return err
	}
}

// réponseCanaux est la forme du contrat {total, offset, limit, canaux}.
type réponseCanaux struct {
	Total  int           `json:"total"`
	Offset int           `json:"offset"`
	Limit  int           `json:"limit"`
	Canaux []store.Canal `json:"canaux"`
	Erreur string        `json:"erreur"`
}

// décoderCanaux lit le corps dans la forme du contrat.
func décoderCanaux(t *testing.T, corps []byte) réponseCanaux {
	t.Helper()
	var réponse réponseCanaux
	if err := json.Unmarshal(corps, &réponse); err != nil {
		t.Fatalf("réponse /api/canaux illisible : %v\n%s", err, corps)
	}
	return réponse
}

func corpsGet(t *testing.T, url string) []byte {
	t.Helper()
	_, _, corps := obtenir(t, url)
	return corps
}

// pointeurÀ : la valeur pointée vaut attendue (nil jamais égal).
func pointeurÀ(v *string, attendu string) bool {
	return v != nil && *v == attendu
}

// TestCanauxPagination : fenêtre offset/limit sur la liste fusionnée, dans
// l'ordre du merge, total EXACT au-delà de la page servie ; provenance
// first-wins lisible dans la réponse.
func TestCanauxPagination(t *testing.T) {
	boutique, recette := recetteCanaux(t)
	reconstruireRecette(t, boutique)

	// Page 1 sur 2 (5 chaînes retenues, limit 3) : Arte, BFM, Info.
	code, entêtes, corps := obtenir(t, recette.URL+"/api/canaux?limit=3&offset=0")
	if code != http.StatusOK {
		t.Fatalf("/api/canaux : code %d, voulu 200", code)
	}
	if type_ := entêtes.Get("Content-Type"); type_ != "application/json; charset=utf-8" {
		t.Errorf("Content-Type %q, voulu application/json charset utf-8", type_)
	}
	réponse := décoderCanaux(t, corps)
	if réponse.Total != 5 {
		t.Errorf("total %d, voulu 5 (5 chaînes retenues, dédoublonnées)", réponse.Total)
	}
	if réponse.Limit != 3 || réponse.Offset != 0 {
		t.Errorf("offset/limit %d/%d, voulus 0/3", réponse.Offset, réponse.Limit)
	}
	if len(réponse.Canaux) != 3 ||
		réponse.Canaux[0].Nom != "Arte" || !pointeurÀ(réponse.Canaux[0].SourceXMLTV, "arte") ||
		!pointeurÀ(réponse.Canaux[0].Numero, "4") ||
		!pointeurÀ(réponse.Canaux[0].Logo, "https://img.tld/arte.png") ||
		!pointeurÀ(réponse.Canaux[0].TvgID, "arte.fr") ||
		réponse.Canaux[0].SourceM3U != "arte" ||
		réponse.Canaux[0].Groupe != "Sports" ||
		réponse.Canaux[1].Nom != "BFM" || réponse.Canaux[1].SourceM3U != "arte" ||
		réponse.Canaux[2].Nom != "Info" || réponse.Canaux[2].SourceM3U != "arte" {
		t.Errorf("page 1 inattendue : %+v", réponse.Canaux)
	}

	// Page 2 : cnn (source miroir, guide arte), puis la chaîne SANS
	// attribut : numero/logo/tvg_id/source_xmltv tous null.
	code, _, corps = obtenir(t, recette.URL+"/api/canaux?limit=3&offset=3")
	if code != http.StatusOK {
		t.Fatalf("/api/canaux?offset=3 : code %d, voulu 200", code)
	}
	réponse = décoderCanaux(t, corps)
	if réponse.Total != 5 || réponse.Offset != 3 {
		t.Errorf("offset %d / total %d, voulus 3/5", réponse.Offset, réponse.Total)
	}
	if len(réponse.Canaux) != 2 {
		t.Fatalf("page 2 : %d entrées, voulues 2 : %+v", len(réponse.Canaux), réponse.Canaux)
	}
	cnn, nue := réponse.Canaux[0], réponse.Canaux[1]
	if cnn.Nom != "CNN" || cnn.SourceM3U != "miroir" || !pointeurÀ(cnn.SourceXMLTV, "arte") {
		t.Errorf("CNN : %+v (source_m3u miroir, source_xmltv arte attendues)", cnn)
	}
	if nue.Nom != "Sans rien" || nue.Numero != nil || nue.Logo != nil ||
		nue.TvgID != nil || nue.SourceXMLTV != nil || nue.Groupe != "" {
		t.Errorf("chaîne sans attribut : %+v (tous champs optionnels null attendus)", nue)
	}
}

// TestCanauxFiltreGroupe : correspondance EXACTE du group-title, seule ou
// combinée avec la pagination.
func TestCanauxFiltreGroupe(t *testing.T) {
	boutique, recette := recetteCanaux(t)
	reconstruireRecette(t, boutique)

	tous := décoderCanaux(t, corpsGet(t, recette.URL+"/api/canaux?limit=500"))
	groupes := map[string]bool{}
	for _, c := range tous.Canaux {
		groupes[c.Groupe] = true
	}
	if !groupes["Sports"] || !groupes["News"] || !groupes[""] {
		t.Fatalf("groupes inattendus : %v", groupes)
	}

	code, _, corps := obtenir(t, recette.URL+"/api/canaux?groupe=News")
	if code != http.StatusOK {
		t.Fatalf("?groupe=News : code %d, voulu 200", code)
	}
	filtrée := décoderCanaux(t, corps)
	// BFM, Info (source arte) et CNN (source miroir) : 3 entrées News.
	if filtrée.Total != 3 || len(filtrée.Canaux) != 3 {
		t.Fatalf("?groupe=News : total %d / %d entrées, voulus 3/3", filtrée.Total, len(filtrée.Canaux))
	}
	for _, c := range filtrée.Canaux {
		if c.Groupe != "News" {
			t.Errorf("entrée hors groupe News : %+v", c)
		}
	}
	if filtrée.Canaux[0].Nom != "BFM" || filtrée.Canaux[1].Nom != "Info" || filtrée.Canaux[2].Nom != "CNN" {
		t.Errorf("ordre News inattendu : %+v", filtrée.Canaux)
	}

	// Filtre + pagination COMBINÉS : deuxième page News seule.
	code, _, corps = obtenir(t, recette.URL+"/api/canaux?groupe=News&limit=1&offset=1")
	if code != http.StatusOK {
		t.Fatalf("?groupe=News&limit=1&offset=1 : code %d, voulu 200", code)
	}
	fenêtre := décoderCanaux(t, corps)
	if fenêtre.Total != 3 || fenêtre.Offset != 1 || len(fenêtre.Canaux) != 1 || fenêtre.Canaux[0].Nom != "Info" {
		t.Fatalf("fenêtre groupée : %+v", fenêtre)
	}
}

// TestCanauxBornesLimit : limit plancher 1, plafond 500, défaut 100 ;
// offset au-delà du total : fenêtre VIDE, code 200 (page hors liste ≠ erreur).
func TestCanauxBornesLimit(t *testing.T) {
	boutique, recette := recetteCanaux(t)
	reconstruireRecette(t, boutique)

	if réponse := décoderCanaux(t, corpsGet(t, recette.URL+"/api/canaux")); réponse.Limit != 100 {
		t.Errorf("limit par défaut %d, voulu 100", réponse.Limit)
	}
	if réponse := décoderCanaux(t, corpsGet(t, recette.URL+"/api/canaux?limit=9999")); réponse.Limit != 500 {
		t.Errorf("limit 9999 → %d, voulu plafonné à 500", réponse.Limit)
	}
	if réponse := décoderCanaux(t, corpsGet(t, recette.URL+"/api/canaux?limit=0")); réponse.Limit != 1 {
		t.Errorf("limit 0 → %d, voulu planché à 1", réponse.Limit)
	}
	if réponse := décoderCanaux(t, corpsGet(t, recette.URL+"/api/canaux?offset=100")); réponse.Total != 5 || len(réponse.Canaux) != 0 {
		t.Errorf("offset=100 : %+v, voulu total 5 sans entrée", réponse)
	}
}

// TestCanauxParamètresInvalides : offset négatif ou non numérique, limit
// non numérique → 400, le lecteur voit POURQUOI (JSON explicite, comme la 503).
func TestCanauxParamètresInvalides(t *testing.T) {
	_, recette := recetteCanaux(t)

	for _, requête := range []string{"?offset=-1", "?offset=trente", "?limit=pasteque"} {
		code, _, corps := obtenir(t, recette.URL+"/api/canaux"+requête)
		if code != http.StatusBadRequest {
			t.Errorf("%s : code %d, voulu 400", requête, code)
		}
		var réponse réponseCanaux
		if err := json.Unmarshal(corps, &réponse); err != nil || réponse.Erreur == "" {
			t.Errorf("%s : corps sans erreur explicite (%s)", requête, corps)
		}
	}
}

// TestCanaux503ÀFroid : sans canaux.json (aucune passe réussie), 503 JSON
// explicite — même contrat que les autres artefacts (story 10).
func TestCanaux503ÀFroid(t *testing.T) {
	boutique, err := store.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	recetteFroide := httptest.NewServer(
		Nouveau(boutique, func() config.Config { return config.Config{} }).Handler(),
	)
	t.Cleanup(recetteFroide.Close)

	code, entêtes, corps := obtenir(t, recetteFroide.URL+"/api/canaux")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("/api/canaux à froid : code %d, voulu 503", code)
	}
	if !strings.Contains(entêtes.Get("Content-Type"), "application/json") {
		t.Errorf("Content-Type %q, voulu du JSON", entêtes.Get("Content-Type"))
	}
	if !strings.Contains(string(corps), "erreur") {
		t.Errorf("corps sans « erreur » : %s", corps)
	}
}

// TestCanauxDèsLaRelectureDuDisque : la réponse est servie depuis le
// FICHIER canaux.json à CHAQUE requête (deux passes streaming, jamais la
// liste en RAM). Publication d'une nouvelle passe (rename atomique) : la
// requête suivante voit immédiatement la nouvelle version — rien n'est
// memoïsé.
func TestCanauxDèsLaRelectureDuDisque(t *testing.T) {
	boutique, recette := recetteCanaux(t)
	reconstruireRecette(t, boutique)

	if réponse := décoderCanaux(t, corpsGet(t, recette.URL+"/api/canaux")); réponse.Total != 5 {
		t.Fatalf("état initial : total %d, voulu 5", réponse.Total)
	}

	// Nouvelle passe : la copie miroir gagne une chaîne ; Reconstruire
	// republie, le handler doit voir le canaux.json neuf sans redémarrage.
	supplément := canauxJSONMiroir + `#EXTINF:-1 tvg-id="rtbf.be" group-title="News",RTBF
https://miroir.tld/rtbf.m3u8
`
	if err := boutique.EcrireCopie("miroir", "m3u", remplirDe(supplément)); err != nil {
		t.Fatalf("EcrireCopie m3u : %v", err)
	}
	reconstruireRecette(t, boutique)

	réponse := décoderCanaux(t, corpsGet(t, recette.URL+"/api/canaux"))
	if réponse.Total != 6 {
		t.Errorf("après nouvelle passe : total %d, voulu 6 (lecture du disque à chaque requête)", réponse.Total)
	}
}
