// Tests à budget RSS (ADR-0001) : exclus sous le runtime race (-race),
// dont l'instrumentation mémoire gonfle le RSS et rend la mesure non
// significative. Sous -race, ce fichier est remplacé par memoire_race_test.go
// qui skip le test ; le budget réel reste certifié par le harnais
// d'acceptance sur binaire de production.
//go:build !race

package xmltv

import (
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// margeRSS est l'augmentation de RSS tolérée pour le filtrage d'un gros
// guide : le filtre doit rester streaming, quelle que soit la taille du
// guide (ADR-0001). La carte de dédup (une entrée par programme, ~110 octets
// pour 100 000 programmes, soit ~12 Mio) et les travaux du parseur (buffers
// de décodage/encodage, ~8 Mio) composent la marge : ce qui NE doit PAS
// croître, c'est le document lui-même — jamais tout le guide en RAM.
const margeRSS = 32 << 20

// lectureRSS lit la RAM effectivement occupée par le processus de test
// (VmRSS de /proc/self/status), en octets.
func lectureRSS(t *testing.T) int64 {
	t.Helper()
	brut, err := os.ReadFile("/proc/self/status")
	if err != nil {
		t.Skipf("VmRSS indisponible : %v", err)
	}
	for _, ligne := range strings.Split(string(brut), "\n") {
		if !strings.HasPrefix(ligne, "VmRSS:") {
			continue
		}
		champs := strings.Fields(ligne)
		if len(champs) < 2 {
			break
		}
		kio, err := strconv.ParseInt(champs[1], 10, 64)
		if err != nil {
			t.Fatalf("VmRSS illisible : %v", err)
		}
		return kio << 10
	}
	t.Skip("VmRSS absent de /proc/self/status")
	return 0
}

// forceLeGC rend la mesure de RSS avant/après significative : sans lui, le
// ramasse-miettes peut laisser traîner les déchets du filtre et faire
// échouer la mesure alors que la mémoire vive est en réalité stable.
func forceLeGC() {
	runtime.GC()
}

// generateurGuide est un io.Reader qui synthétise à la volée un guide
// XMLTV COMPLET : d'abord la racine <tv>, puis les n programmes de premier
// niveau, puis la fermeture de la racine — même forme que genererXMLTV du
// harnais d'acceptance (internal/acceptance), le guide canonique d'un
// fournisseur. JAMAIS tout en mémoire en même temps : un fragment prêt à
// être lu à la fois, la mesure ne contient que le filtre (ADR-0001).
//
// Revue round 19 : la fixture était une suite de <programme> SANS racine
// (XML mal formé, aucun cas réel) ; sous la primitive partagée Balayer
// (cf6ef2c), le premier programme devenait pseudoracine, son EndElement
// déclenchait l'arrêt « fin de racine » et le filtre ne traitait plus
// QU'UN programme — le test restait vert en ne mesurant plus rien. La
// sondePourProgrammes ci-dessous aurait attrapé ce silence.
type generateurGuide struct {
	restantes int
	enTete    bool
	enAttente string // fragment prêt à être lu (tête, programme, ou pied)
}

func (g *generateurGuide) Read(p []byte) (int, error) {
	if g.enAttente == "" {
		switch {
		case !g.enTete:
			g.enTete, g.enAttente = true, "<tv>\n"
		case g.restantes > 0:
			g.enAttente = fmt.Sprintf(`<programme start="202401%06d" channel="c1.fr"><title>Film</title></programme>`+"\n", g.restantes)
			g.restantes--
		default:
			g.enAttente = "</tv>\n"
		}
	}
	n := copy(p, g.enAttente)
	g.enAttente = g.enAttente[n:]
	return n, nil
}

// sondePourProgrammes est un io.Writer qui COMPTE les balises <programme
// émises par le filtre sans retenir le contenu (recouvrement d'un fragment
// à l'autre pour les balises à cheval sur deux Write) : sonde de traversal
// — la mesure RSS doit rester celle du filtre, jamais une accumulation.
type sondePourProgrammes struct {
	emis  int
	queue string // fin de l'écriture précédente (balise à cheval)
}

func (s *sondePourProgrammes) Write(p []byte) (int, error) {
	recu := s.queue + string(p)
	s.emis += strings.Count(recu, "<programme")
	if surplus := len(recu) - (len("<programme") - 1); surplus > 0 {
		s.queue = recu[surplus:]
	}
	return len(p), nil
}

// TestFiltrerMemoireConstante vérifie le budget RSS du filtre sur un gros
// guide ENTIER (racine <tv> + 100 000 programmes de premier niveau) : la
// mémoire ne doit pas croître avec le nombre de programmes — ET le filtre
// doit avoir réellement traversé les 100 000 (sonde de traversal, revue
// round 19 : un test vert qui ne traite plus qu'un programme n'en est pas
// un ; toute perte de traversal apparaît ici en ÉCHEC, pas en silence).
// La carte de dédup (une entrée par programme, métadonnée admise par
// ADR-0001) est le terme dominant de la mesure ; le budget complet est
// arbitré au ticket #9.
func TestFiltrerMemoireConstante(t *testing.T) {
	avant := lectureRSS(t)
	sonde := &sondePourProgrammes{}
	if err := Filtrer(&generateurGuide{restantes: 100_000}, sonde, map[string]struct{}{"c1.fr": {}}); err != nil {
		t.Fatalf("Filtrer : %v", err)
	}
	apres := lectureRSS(t)
	if apres-avant > margeRSS {
		t.Errorf("RSS : %d octets de plus après le filtrage d'un guide de %d programmes", apres-avant, 100_000)
	}
	if sonde.emis != 100_000 {
		t.Errorf("%d programmes émis sur %d : le filtre n'a pas traversé tout le guide", sonde.emis, 100_000)
	}
}
