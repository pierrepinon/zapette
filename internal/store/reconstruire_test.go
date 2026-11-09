package store

import (
	"encoding/xml"
	"io"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/pierrepinon/zapette/internal/m3u"
)

// arteM3U et miroirM3U reprennent le scénario de TestBalayerFusionPremierGagne
// (internal/m3u) : la fusion du snapshot doit s'y plier octet par octet.
const (
	arteM3U = `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" group-title="Sports",Arte
https://flux.exemple.tld/arte.m3u8
#EXTINF:-1 tvg-id="bfm.fr" group-title="News",BFM
https://flux.exemple.tld/bfm.m3u8
`
	miroirM3U = `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" group-title="Sports",Arte
https://miroir.tld/arte.m3u8
#EXTINF:-1 tvg-id="cnews.fr" group-title="News",CNews
https://miroir.tld/cnews.m3u8
`
)

func TestReconstruireM3UFusionPremierGagne(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	if err := s.EcrireCopie("arte", "m3u", ecrire(arteM3U)); err != nil {
		t.Fatalf("EcrireCopie arte : %v", err)
	}
	if err := s.EcrireCopie("miroir", "m3u", ecrire(miroirM3U)); err != nil {
		t.Fatalf("EcrireCopie miroir : %v", err)
	}

	if err := s.Reconstruire([]string{s.CheminCopie("arte", "m3u"), s.CheminCopie("miroir", "m3u")}, nil); err != nil {
		t.Fatalf("Reconstruire : %v", err)
	}

	lu, err := os.ReadFile(s.CheminSnapshot("m3u"))
	if err != nil {
		t.Fatalf("lecture du snapshot : %v", err)
	}
	// Arte : première source gagne ; bfm retenu ; cnews ajouté par la
	// seconde ; en-tête du premier arrivé.
	attendu := `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" group-title="Sports",Arte
https://flux.exemple.tld/arte.m3u8
#EXTINF:-1 tvg-id="bfm.fr" group-title="News",BFM
https://flux.exemple.tld/bfm.m3u8
#EXTINF:-1 tvg-id="cnews.fr" group-title="News",CNews
https://miroir.tld/cnews.m3u8
`
	if string(lu) != attendu {
		t.Errorf("snapshot.m3u :\n%s\nattendu :\n%s", string(lu), attendu)
	}
	// Publication atomique : aucun temporaire résiduel à la racine.
	entrées, err := os.ReadDir(s.racine)
	if err != nil {
		t.Fatalf("lecture du répertoire de données : %v", err)
	}
	for _, e := range entrées {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temporaire résiduel : %s", e.Name())
		}
	}
}

func TestReconstruireIgnoreLesCopiesAbsentes(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	if err := s.EcrireCopie("arte", "m3u", ecrire(arteM3U)); err != nil {
		t.Fatalf("EcrireCopie arte : %v", err)
	}

	// La copie fantôme n'a jamais été récupérée : elle est ignorée, le
	// snapshot fusionne ce qui existe dans l'ordre donné.
	if err := s.Reconstruire(
		[]string{s.CheminCopie("arte", "m3u"), s.CheminCopie("fantome", "m3u")},
		nil); err != nil {
		t.Fatalf("Reconstruire : %v", err)
	}

	lu, err := os.ReadFile(s.CheminSnapshot("m3u"))
	if err != nil {
		t.Fatalf("lecture du snapshot : %v", err)
	}
	if string(lu) == "" || !strings.Contains(string(lu), "arte.m3u8") {
		t.Errorf("snapshot.m3u devait fusionner la copie existante : %q", lu)
	}
	if strings.Contains(string(lu), "fantome") {
		t.Errorf("copie absente devait être ignorée : %q", lu)
	}
}

func TestReconstruireSansCopieNePubliePas(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	// Aucune copie (échec total dès le premier fetch) : rien à fusionner,
	// aucun snapshot publié — celui du disque reste celui d'avant, jamais
	// un fichier partiel.
	if err := s.Reconstruire(nil, nil); err != nil {
		t.Fatalf("Reconstruire (sans copies) : %v", err)
	}
	if _, err := os.Stat(s.CheminSnapshot("m3u")); !os.IsNotExist(err) {
		t.Errorf("aucun snapshot devait être publié, stat : %v", err)
	}
	if _, err := os.Stat(s.CheminSnapshot("xmltv")); !os.IsNotExist(err) {
		t.Errorf("aucun snapshot devait être publié, stat : %v", err)
	}
}

// TestReconstruireXmltvFusionPremierGagne : les copies sont des guides
// entiers (racine <tv> dédiée par copie) ; la fusion les réassemble sous une
// seule racine, chaînes et programmes dédupliqués first-wins, première copie
// et première occurrence en tête.
func TestReconstruireXmltvFusionPremierGagne(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	if err := s.EcrireCopie("arte", "xmltv", ecrire(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="arte.fr"><display-name>Arte</display-name></channel>
  <programme start="20250101120000 +0100" channel="arte.fr"><title>Journal</title></programme>
</tv>
`)); err != nil {
		t.Fatalf("EcrireCopie arte : %v", err)
	}
	if err := s.EcrireCopie("miroir", "xmltv", ecrire(`<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="arte.fr"><display-name>Arte miroir</display-name></channel>
  <channel id="cnews.fr"><display-name>CNews</display-name></channel>
  <programme start="20250101120000 +0100" channel="arte.fr"><title>Doublon</title></programme>
  <programme start="20250101130000 +0100" channel="cnews.fr"><title>Direct</title></programme>
</tv>
`)); err != nil {
		t.Fatalf("EcrireCopie miroir : %v", err)
	}

	if err := s.Reconstruire(nil, []string{s.CheminCopie("arte", "xmltv"), s.CheminCopie("miroir", "xmltv")}); err != nil {
		t.Fatalf("Reconstruire : %v", err)
	}

	lu, err := os.ReadFile(s.CheminSnapshot("xmltv"))
	if err != nil {
		t.Fatalf("lecture du snapshot : %v", err)
	}
	var guide tvAttendue
	if err := xml.Unmarshal(lu, &guide); err != nil {
		t.Fatalf("snapshot.xmltv illisible : %v\n%s", err, lu)
	}
	if len(guide.Canaux) != 2 {
		t.Errorf("chaînes : %d, attendu 2 (arte dédupliqué) : %+v", len(guide.Canaux), guide.Canaux)
	}
	if len(guide.Canaux) > 1 && guide.Canaux[0].DisplayName != "Arte" {
		t.Errorf("première chaîne : %+v, attendu Arte (première source gagne)", guide.Canaux[0])
	}
	if len(guide.Programmes) != 2 {
		t.Errorf("programmes : %d, attendu 2 (doublon éjecté) : %+v", len(guide.Programmes), guide.Programmes)
	}
	if len(guide.Programmes) > 1 && guide.Programmes[0].Titre != "Journal" {
		t.Errorf("premier programme : %+v, attendu Journal (première occurrence gagne)", guide.Programmes[0])
	}
	if len(guide.Programmes) > 1 && guide.Programmes[1].Channel != "cnews.fr" {
		t.Errorf("second programme : %+v, attendu cnews.fr", guide.Programmes[1])
	}
}

// TestReconstruireParsebleParLeConsommateur : le snapshot m3u publié doit
// être balayable comme une playlist simple (un seul en-tête, entrées
// complètes) — c'est ce que servira le lecteur.
func TestReconstruireParsebleParLeConsommateur(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	if err := s.EcrireCopie("arte", "m3u", ecrire(arteM3U)); err != nil {
		t.Fatalf("EcrireCopie arte : %v", err)
	}
	if err := s.EcrireCopie("miroir", "m3u", ecrire(miroirM3U)); err != nil {
		t.Fatalf("EcrireCopie miroir : %v", err)
	}
	if err := s.Reconstruire([]string{s.CheminCopie("arte", "m3u"), s.CheminCopie("miroir", "m3u")}, nil); err != nil {
		t.Fatalf("Reconstruire : %v", err)
	}

	f, err := os.Open(s.CheminSnapshot("m3u"))
	if err != nil {
		t.Fatalf("ouverture du snapshot : %v", err)
	}
	defer f.Close()
	var identites []string
	if err := m3u.Balayer(f, nil, func(c m3u.Chaine, _ []string) error {
		identites = append(identites, c.Identite())
		return nil
	}); err != nil {
		t.Fatalf("Balayer du snapshot : %v", err)
	}
	attendus := []string{"arte.fr", "bfm.fr", "cnews.fr"}
	if !slices.Equal(identites, attendus) {
		t.Errorf("identités : %v, attendu %v", identites, attendus)
	}
}

func ecrire(texte string) func(io.Writer) error {
	return func(w io.Writer) error { _, err := io.WriteString(w, texte); return err }
}

// tvAttendue structure de contrôle du snapshot xmltv (sous-vue utile au test).
type tvAttendue struct {
	XMLName    xml.Name           `xml:"tv"`
	Canaux     []canalAttendu     `xml:"channel"`
	Programmes []programmeAttendu `xml:"programme"`
}

type canalAttendu struct {
	ID          string `xml:"id,attr"`
	DisplayName string `xml:"display-name"`
}

type programmeAttendu struct {
	Start   string `xml:"start,attr"`
	Channel string `xml:"channel,attr"`
	Titre   string `xml:"title"`
}
