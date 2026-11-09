package xmltv

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"testing"
)

// Le guide d'essai : déclaration XML, doctype, racine <tv>, deux chaînes
// (c1.fr retenue, c2.fr hors liste) et des programmes dont un doublon exact
// du premier (même chaîne, même start).
const guide = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE tv SYSTEM "xmltv.dtd">
<tv generator-info-name="zapette">
  <channel id="c1.fr"><display-name>Une</display-name></channel>
  <channel id="c2.fr"><display-name>Deux</display-name></channel>
  <programme start="20240101120000 +0100" channel="c1.fr"><title>Film</title></programme>
  <programme start="20240101130000 +0100" channel="c1.fr"><title>Série</title></programme>
  <programme start="20240101120000 +0100" channel="c1.fr"><title>Film</title></programme>
  <programme start="20240101120000 +0100" channel="c2.fr"><title>Téléchat</title></programme>
  <programme start="20240101130000 +0100" channel="c2.fr"><title>Journal</title></programme>
</tv>`

// filtreEtValide filtre le guide avec les chaînes retenues et vérifie que la
// sortie reste du XML bien formé.
func filtreEtValide(t *testing.T, guide string, retenus map[string]struct{}) string {
	t.Helper()
	sortie := &strings.Builder{}
	if err := Filtrer(strings.NewReader(guide), sortie, retenus); err != nil {
		t.Fatalf("Filtrer : %v", err)
	}
	obtenu := sortie.String()
	if err := xml.NewDecoder(strings.NewReader(obtenu)).Decode(new(any)); err != nil {
		t.Fatalf("sortie mal formée : %v\nsortie :\n%s", err, obtenu)
	}
	return obtenu
}

// TestFiltrerConserveLesChaînesRetenues vérifie le critère d'acceptation :
// la sortie ne contient que les chaînes retenues, avec leurs programmes.
func TestFiltrerConserveLesChaînesRetenues(t *testing.T) {
	obtenu := filtreEtValide(t, guide, map[string]struct{}{"c1.fr": {}})

	if !strings.Contains(obtenu, "<channel id=\"c1.fr\">") {
		t.Errorf("la chaîne retenue doit passer : %s", obtenu)
	}
	for _, retiré := range []string{"c2.fr", "Téléchat"} {
		if strings.Contains(obtenu, retiré) {
			t.Errorf("%s ne devait pas passer : %s", retiré, obtenu)
		}
	}
}

// TestFiltrerDéclaréASousEnsembleVérifieLaSortie.ValideLaSortie vérifie les critères d'acceptation
// « dédup programmes par (id chaîne, start), premier arrivé conservé » et
// « déclaration XML et doctype conservés ».
func TestFiltrerDédupProgrammes(t *testing.T) {
	obtenu := filtreEtValide(t, guide, map[string]struct{}{"c1.fr": {}})

	if !strings.HasPrefix(obtenu, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("la déclaration XML et le doctype doivent être conservés : %s", obtenu)
	}
	if n := strings.Count(obtenu, "<programme"); n != 2 {
		t.Errorf("%d programmes dans la sortie, attendu 3 (le doublon exact jeté) :\n%s", n, obtenu)
	}
	if n := strings.Count(obtenu, "<title>Série</title>"); n != 1 {
		t.Errorf("le premier arrivé doit être conservé une seule fois, obtenu %d :\n%s", n, obtenu)
	}
}

// TestFiltrerTolèreUnGuideRompé vérifie le critère d'acceptation « le
// lisible continue d'être filtré, pas de crash » : après une rupture au
// milieu du document, la partie lisible a bien été filtrée et transmise.
func TestFiltrerTolèreUnGuideRompé(t *testing.T) {
	rupture := `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="c1.fr"><display-name>Une</display-name></channel>
  <channel id="c2.fr"><display-name>Deux</display-name>
  < programme rompu…`
	sortie := &strings.Builder{}
	if err := Filtrer(strings.NewReader(rupture), sortie, map[string]struct{}{"c1.fr": {}}); err != nil {
		t.Fatalf("un guide rompu ne doit pas être une erreur : %v", err)
	}
	if !strings.Contains(sortie.String(), "c1.fr") {
		t.Errorf("la partie lisible doit être filtrée puis transmise : %s", sortie.String())
	}
}

// generateurGuide est un io.Reader qui synthétise les programmes à la
// volée : jamais tous en mémoire en même temps, pour que la mesure ne
// contienne que le filtre et pas le document d'essai (ADR-0001).
type generateurGuide struct {
	restantes int
}

func (g *generateurGuide) Read(p []byte) (int, error) {
	if g.restantes == 0 {
		return 0, io.EOF
	}
	texte := fmt.Sprintf(`<programme start="202401%06d" channel="c1.fr"><title>Film</title></programme>`+"\n", g.restantes)
	g.restantes--
	return copy(p, texte), nil
}

// TestFiltrerMemoireConstante vérifie le budget RSS du filtre sur un gros
// guide : la mémoire ne doit pas croître avec le nombre de programmes. La
// carte de dédup (une entrée par programme, métadonnée admise par ADR-0001)
// est le terme dominant de la mesure ; le budget complet est arbitré au
// ticket #9.
func TestFiltrerMemoireConstante(t *testing.T) {
	avant := lectureRSS(t)
	if err := Filtrer(&generateurGuide{restantes: 100_000}, io.Discard, map[string]struct{}{"c1.fr": {}}); err != nil {
		t.Fatalf("Filtrer : %v", err)
	}
	if après := lectureRSS(t); après-avant > margeRSS {
		t.Errorf("RSS : %d octets de plus après le filtrage d'un guide de %d programmes", après-avant, 100_000)
	}
}

// TestFiltrerSansChaineRetenue épingle le comportement d'un ensemble de
// chaînes retenues vide : aucune entrée de guide ne passe, seul le
// squelette du document (déclaration, doctype, racine <tv>) est transmis.
func TestFiltrerSansChaineRetenue(t *testing.T) {
	squelette := &strings.Builder{}
	if err := Filtrer(strings.NewReader(guide), squelette, map[string]struct{}{}); err != nil {
		t.Fatalf("Filtrer : %v", err)
	}
	for _, interdit := range []string{"<channel", "<programme", "bein1.fr"} {
		if strings.Contains(squelette.String(), interdit) {
			t.Errorf("un ensemble vide de retenus ne doit rien garder : %s passé", interdit)
		}
	}
	if !strings.Contains(squelette.String(), "<tv") {
		t.Errorf("le squelette <tv> doit rester : %s", squelette.String())
	}
}
