package xmltv

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// margeRSS est l'augmentation de RSS tolérée pour le filtrage d'un gros
// guide : le filtre doit rester streaming, quelle que soit la taille du
// guide (ADR-0001). Le filtre mesuré dépasse rarement quelques Mio ; une
// accumulation des programmes dépasserait largement cette marge.
const margeRSS = 16 << 20

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
