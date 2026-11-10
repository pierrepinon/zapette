// Tests à budget RSS (ADR-0001) : exclus sous le runtime race (-race),
// dont l'instrumentation mémoire gonfle le RSS et rend la mesure non
// significative. Sous -race, ce fichier est remplacé par memoire_race_test.go
// qui skip le test ; le budget réel reste certifié par le harnais
// d'acceptance sur binaire de production.
//go:build !race

package xmltv

import (
	"fmt"
	"io"
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
