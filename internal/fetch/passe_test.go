package fetch

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/store"
)

// Matériaux de test : deux sources — arte (Sports/Arte + News/BFM) avec
// guide, miroir (Sports/Arte miroir + News/CNews) avec guide. arte.fr
// figure aux deux : le merge doit le dédupliquer (first-wins).
const (
	arteM3UCorps = `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" group-title="Sports",Arte
https://flux.exemple.tld/arte.m3u8
#EXTINF:-1 tvg-id="bfm.fr" group-title="News",BFM
https://flux.exemple.tld/bfm.m3u8
`
	miroirM3UCorps = `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" group-title="Sports",Arte
https://miroir.tld/arte.m3u8
#EXTINF:-1 tvg-id="cnews.fr" group-title="News",CNews
https://miroir.tld/cnews.m3u8
`
	arteXMLTVCorps = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="arte.fr"><display-name>Arte</display-name></channel>
  <channel id="bfm.fr"><display-name>BFM</display-name></channel>
  <programme start="20250101120000 +0100" channel="arte.fr"><title>Journal</title></programme>
  <programme start="20250101200000 +0100" channel="bfm.fr"><title>Direct</title></programme>
</tv>
`
	miroirXMLTVCorps = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="arte.fr"><display-name>Arte miroir</display-name></channel>
  <channel id="cnews.fr"><display-name>CNews</display-name></channel>
  <programme start="20250101120000 +0100" channel="arte.fr"><title>Doublon</title></programme>
  <programme start="20250101130000 +0100" channel="cnews.fr"><title>Info</title></programme>
</tv>
`
)

func serveurTexte(t *testing.T, corps string) string {
	t.Helper()
	serveur := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(corps))
	}))
	t.Cleanup(serveur.Close)
	return serveur.URL
}

func serveursDeTest(t *testing.T) (arte, miroir config.Source) {
	t.Helper()
	return config.Source{
		Name:     "arte",
		M3UURL:   serveurTexte(t, arteM3UCorps),
		XMLTVURL: serveurTexte(t, arteXMLTVCorps),
	}, config.Source{
		Name:     "miroir",
		M3UURL:   serveurTexte(t, miroirM3UCorps),
		XMLTVURL: serveurTexte(t, miroirXMLTVCorps),
	}
}

func cfgDeTest(dir string, groupes []string, sources ...config.Source) config.Config {
	return config.Config{
		RefreshInterval: time.Minute,
		AllowedGroups:   groupes,
		Sources:         sources,
		FetchTimeout:    2 * time.Second,
		FetchRetries:    1,
		DataDir:         dir,
	}
}

func nouvelleBoutique(t *testing.T) (*store.Store, string) {
	t.Helper()
	dir := t.TempDir()
	b, err := store.Ouvrir(dir)
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	return b, dir
}

func nouvelOrdonnanceur(b *store.Store) *Ordonnanceur {
	return Nouveau(b, NouveauFetcher, 1*time.Millisecond)
}

// TestPasseFiltreCopieEtPublie : la passe complète — m3u filtrées par liste
// de groupes autorisés et copiées, xmltv filtrées par l'ensemble des
// identités retenues par le merge M3U (artef miroir dédupliqué), snapshot
// publié, meta.json écrit en dernier.
func TestPasseFiltreCopieEtPublie(t *testing.T) {
	arte, miroir := serveursDeTest(t)
	b, dir := nouvelleBoutique(t)
	o := nouvelOrdonnanceur(b)

	if err := o.Passe(context.Background(), cfgDeTest(dir, []string{"News"}, arte, miroir)); err != nil {
		t.Fatalf("Passe : %v", err)
	}

	// Copies de source : filtrées par la liste de groupes autorisés.
	if lu, err := os.ReadFile(b.CheminCopie("arte", "m3u")); err != nil ||
		!strings.Contains(string(lu), "bfm.fr") || strings.Contains(string(lu), "arte.fr") {
		t.Errorf("copie arte.m3u : %q (%v)", lu, err)
	}
	// Snapshot m3u : en-tête du premier arrivé, arte.fr dédupliqué ;
	// tvg-chno réécrits aux numéros calculés (ADR-0002) : bfm.fr hors plan
	// → 27, cnews.fr → 14 (plan Arcom).
	attendu := `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="bfm.fr" group-title="News" tvg-chno="27",BFM
https://flux.exemple.tld/bfm.m3u8
#EXTINF:-1 tvg-id="cnews.fr" group-title="News" tvg-chno="14",CNews
https://miroir.tld/cnews.m3u8
`
	if lu, err := os.ReadFile(b.CheminSnapshot("m3u")); err != nil || string(lu) != attendu {
		t.Errorf("snapshot.m3u :\n%s\nattendu :\n%s (%v)", lu, attendu, err)
	}
	// Snapshot xmltv : une seule racine, seulement les identités retenues.
	lu, err := os.ReadFile(b.CheminSnapshot("xmltv"))
	if err != nil {
		t.Fatalf("lecture du snapshot xmltv : %v", err)
	}
	var guide tvDeTeste
	if err := xml.Unmarshal(lu, &guide); err != nil {
		t.Fatalf("snapshot.xmltv illisible : %v\n%s", err, lu)
	}
	if noms := idsDeTeste(guide.Canaux); len(noms) != 2 || noms["bfm.fr"] != "BFM" || noms["cnews.fr"] != "CNews" {
		t.Errorf("chaînes : %+v, attendu bfm+cnews uniquement", guide.Canaux)
	}
	if len(guide.Programmes) != 2 {
		t.Errorf("programmes : %+v, attendu ceux de bfm et cnews uniquement", guide.Programmes)
	}
	// meta.json écrit en dernier : état par source/artefact.
	meta, err := b.Charger()
	if err != nil {
		t.Fatalf("Charger : %v", err)
	}
	if len(meta.Sources) != 2 {
		t.Fatalf("meta : %d sources, attendu 2", len(meta.Sources))
	}
	for nom, es := range meta.Sources {
		if es.M3U == nil || es.M3U.DernierOK == nil || es.M3U.DerniereErreur != "" || es.M3U.SuivantA == nil {
			t.Errorf("%s.m3u : %+v", nom, es.M3U)
		}
		if es.XMLTV == nil || es.XMLTV.DernierOK == nil || es.XMLTV.DerniereErreur != "" {
			t.Errorf("%s.xmltv : %+v", nom, es.XMLTV)
		}
	}
	// Publication atomique : aucun temporaire résiduel.
	for _, chemin := range []string{dir, filepath.Join(dir, "sources")} {
		entrées, err := os.ReadDir(chemin)
		if err != nil {
			t.Fatalf("lecture de %s : %v", chemin, err)
		}
		for _, e := range entrées {
			if strings.HasPrefix(e.Name(), ".tmp-") {
				t.Errorf("temporaire résiduel : %s/%s", chemin, e.Name())
			}
		}
	}
}

// TestPasseSansListeToutRetenir : allowed_groups vide ⇒ les copies m3u
// comptent toutes les entrées, l'ensemble des identités transmises au
// xmltv porte tout le guide disponible (conséquence câblée côté passe
// fetch, sans cas spécial dans le store).
func TestPasseSansListeToutRetenir(t *testing.T) {
	arte, miroir := serveursDeTest(t)
	b, dir := nouvelleBoutique(t)
	o := nouvelOrdonnanceur(b)

	if err := o.Passe(context.Background(), cfgDeTest(dir, nil, arte, miroir)); err != nil {
		t.Fatalf("Passe : %v", err)
	}

	attendu := `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" group-title="Sports" tvg-chno="7",Arte
https://flux.exemple.tld/arte.m3u8
#EXTINF:-1 tvg-id="bfm.fr" group-title="News" tvg-chno="27",BFM
https://flux.exemple.tld/bfm.m3u8
#EXTINF:-1 tvg-id="cnews.fr" group-title="News" tvg-chno="14",CNews
https://miroir.tld/cnews.m3u8
`
	if lu, err := os.ReadFile(b.CheminSnapshot("m3u")); err != nil || string(lu) != attendu {
		t.Errorf("snapshot.m3u :\n%s\nattendu :\n%s (%v)", lu, attendu, err)
	}
	lu, err := os.ReadFile(b.CheminSnapshot("xmltv"))
	if err != nil {
		t.Fatalf("lecture du snapshot xmltv : %v", err)
	}
	var guide tvDeTeste
	if err := xml.Unmarshal(lu, &guide); err != nil {
		t.Fatalf("snapshot.xmltv illisible : %v\n%s", err, lu)
	}
	// arte.fr dédupliqué : première source gagne (« Arte », pas miroir).
	if noms := idsDeTeste(guide.Canaux); len(noms) != 3 || noms["arte.fr"] != "Arte" {
		t.Errorf("chaînes : %+v, attendu arte (première source gagne)+bfm+cnews", guide.Canaux)
	}
	doublons := 0
	for _, p := range guide.Programmes {
		if p.Channel == "arte.fr" && p.Start == "20250101120000 +0100" {
			doublons++ // le Doublon du miroir éjecté, le Journal conservé
		}
	}
	if doublons != 1 {
		t.Errorf("programmes arte@12h : %d, attendu 1 (dédup first-wins)", doublons)
	}
}

// TestPasseSourceEnEchecFigeSaCopie : échec d'une source ⇒ sa copie figée
// (octet pour octet), son last_ok conservé, les autres rafraichies ; le
// snapshot fusionne la copie figée comme les autres.
func TestPasseSourceEnEchecFigeSaCopie(t *testing.T) {
	arte, miroir := serveursDeTest(t)
	b, dir := nouvelleBoutique(t)
	o := nouvelOrdonnanceur(b)

	// Le miroir répond en 500 à partir du flag levé.
	var miroirEnEchec atomic.Int32
	miroirM3U := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if miroirEnEchec.Load() == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(miroirM3UCorps))
	}))
	defer miroirM3U.Close()
	miroir.M3UURL = miroirM3U.URL

	if err := o.Passe(context.Background(), cfgDeTest(dir, nil, arte, miroir)); err != nil {
		t.Fatalf("Passe 1 : %v", err)
	}
	figée, err := os.ReadFile(b.CheminCopie("miroir", "m3u"))
	if err != nil {
		t.Fatalf("lecture de la copie miroir : %v", err)
	}
	meta1, err := b.Charger()
	if err != nil {
		t.Fatalf("Charger après passe 1 : %v", err)
	}

	miroirEnEchec.Store(1)
	if err := o.Passe(context.Background(), cfgDeTest(dir, nil, arte, miroir)); err != nil {
		t.Fatalf("Passe 2 : %v", err)
	}

	// Copie figée octet pour octet malgré l'échec du fetch.
	lu, err := os.ReadFile(b.CheminCopie("miroir", "m3u"))
	if err != nil || string(lu) != string(figée) {
		t.Errorf("copie miroir figée attendue, obtenu %q (%v)", lu, err)
	}
	// Le snapshot fusionne toujours la copie figée.
	if lu, err := os.ReadFile(b.CheminSnapshot("m3u")); err != nil ||
		!strings.Contains(string(lu), "cnews.m3u8") {
		t.Errorf("snapshot.m3u devait fusionner la copie figée : %q (%v)", lu, err)
	}
	// meta.json : l'erreur est tracée POUR le miroir, last_ok conservé ;
	// arte est rafraichi sans erreur.
	meta2, err := b.Charger()
	if err != nil {
		t.Fatalf("Charger après passe 2 : %v", err)
	}
	if meta2.Sources["miroir"].M3U == nil || meta2.Sources["miroir"].M3U.DerniereErreur == "" {
		t.Errorf("miroir.m3u : %+v, attendu une last_error", meta2.Sources["miroir"].M3U)
	}
	if meta2.Sources["miroir"].M3U != nil && meta2.Sources["miroir"].M3U.DernierOK == nil {
		t.Errorf("miroir.m3u.last_ok perdu : %+v", meta2.Sources["miroir"].M3U)
	}
	if meta2.Sources["miroir"].M3U != nil && meta2.Sources["arte"].M3U != nil &&
		meta2.Sources["arte"].M3U.DernierOK.Compare(*meta1.Sources["arte"].M3U.DernierOK) <= 0 {
		t.Errorf("arte.m3u.last_ok devait être rafraichi : %v", meta2.Sources["arte"].M3U.DernierOK)
	}
}

// TestPasseSansChaineRetenueNeFetchPasLeGuide : si aucune chaîne n'a
// survécu au filtrage M3U, aucun fetch XML n'est déclenché — la passe ne
// transmet jamais d'ensemble vide à xmltv.Filtrer (par design).
func TestPasseSansChaineRetenueNeFetchPasLeGuide(t *testing.T) {
	var appelsXMLTV atomic.Int64
	arte, miroir := serveursDeTest(t)
	xmltvCompté := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		appelsXMLTV.Add(1)
		_, _ = w.Write([]byte(arteXMLTVCorps))
	}))
	defer xmltvCompté.Close()
	b, dir := nouvelleBoutique(t)
	o := nouvelOrdonnanceur(b)

	cfg := cfgDeTest(dir, []string{"Zut — aucun groupe réel"}, arte, miroir)
	cfg.Sources[0].XMLTVURL = xmltvCompté.URL
	if err := o.Passe(context.Background(), cfg); err != nil {
		t.Fatalf("Passe : %v", err)
	}

	if appelsXMLTV.Load() != 0 {
		t.Errorf("fetch xmltv déclenché sans chaîne retenue : %d appels", appelsXMLTV.Load())
	}
	meta, err := b.Charger()
	if err != nil {
		t.Fatalf("Charger : %v", err)
	}
	if meta.Sources["arte"].XMLTV == nil || meta.Sources["arte"].XMLTV.DerniereErreur == "" {
		t.Errorf("arte.xmltv : %+v, attendu une last_error explicite", meta.Sources["arte"].XMLTV)
	}
}

func idsDeTeste(canaux []canalDeTeste) map[string]string {
	noms := make(map[string]string, len(canaux))
	for _, c := range canaux {
		noms[c.ID] = c.DisplayName
	}
	return noms
}

// tvDeTeste structure de contrôle du snapshot xmltv (sous-vue utile au test).
type tvDeTeste struct {
	XMLName    xml.Name           `xml:"tv"`
	Canaux     []canalDeTeste     `xml:"channel"`
	Programmes []programmeDeTeste `xml:"programme"`
}

type canalDeTeste struct {
	ID          string `xml:"id,attr"`
	DisplayName string `xml:"display-name"`
}

type programmeDeTeste struct {
	Start   string `xml:"start,attr"`
	Channel string `xml:"channel,attr"`
}
