// Tests à budget RSS (ADR-0001) : exclus sous le runtime race (-race),
// dont l'instrumentation mémoire gonfle le RSS et rend la mesure non
// significative. Sous -race, ce fichier est remplacé par memoire_race_test.go
// qui skip ces tests ; le budget réel reste certifié par le harnais
// d'acceptance sur binaire de production.
//go:build !race

package m3u

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// margeRSS est l'augmentation de RSS tolérée pour le filtrage d'une playlist
// de 200 000 entrées : le filtrage doit être streaming (mémoire constante,
// ADR-0001). Le filter streaming mesuré dépasse rarement quelques Mio ;
// une accumulation des entrées en mémoire dépasserait largement cette marge
// (une playlist réelle fait ~1 Gio pour 216 000 entrées).
const margeRSS = 16 << 20

// nombreEntrees est le volume de référence du ticket : 200 000 entrées,
// soit l'ordre de grandeur réel (216 000 entrées).
const nombreEntrees = 200_000

// lectureRSS lit le VmRSS du processus de test depuis /proc/self/status.
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

// generateurPlaylist est un io.Reader qui synthétise la playlist à la volée :
// les entrées n'existent jamais toutes en mémoire en même temps, ce qui
// mesure bien la mémoire du filtre et pas celle du jeu d'essai.
type generateurPlaylist struct {
	restantes int
	tampon    []byte
}

func (g *generateurPlaylist) Read(p []byte) (int, error) {
	for len(g.tampon) == 0 {
		if g.restantes == 0 {
			return 0, io.EOF
		}
		n := g.restantes
		g.restantes--
		// Groupe alterné : la moitié des entrées est retenue.
		groupe := "Films"
		if n%2 == 0 {
			groupe = "Sports"
		}
		g.tampon = append(g.tampon,
			[]byte(fmt.Sprintf(
				"#EXTINF:-1 tvg-id=\"ch%d.fr\" tvg-name=\"Chaîne %d\" group-title=\"%s\",Chaîne %d\n",
				n, n, groupe, n))...)
		g.tampon = append(g.tampon,
			[]byte(fmt.Sprintf("https://flux.exemple.tld/ch%d.m3u8\n", n))...)
		if g.restantes == nombreEntrees-1 {
			// En-tête, émis une fois, à la première entrée générée.
			g.tampon = append([]byte("#EXTM3U url-tvg=\"https://exemple.tld/guide.xml\"\n"), g.tampon...)
		}
	}
	n := copy(p, g.tampon)
	g.tampon = g.tampon[n:]
	return n, nil
}

// TestFiltrerMemoireConstante vérifie le critère streaming : filtrer une
// playlist de 200 000 entrées (la moitié retenue) ne doit pas faire grimper
// la mémoire au fil des entrées — la playlist complète n'est jamais chargée.
func TestFiltrerMemoireConstante(t *testing.T) {
	runtime.GC()
	avant := lectureRSS(t)

	generateur := &generateurPlaylist{restantes: nombreEntrees}
	if err := Filtrer(generateur, io.Discard, []string{"Sports"}); err != nil {
		t.Fatalf("Filtrer : %v", err)
	}

	runtime.GC()
	apres := lectureRSS(t)
	if croissance := apres - avant; croissance > margeRSS {
		t.Errorf("RSS a grandi de %d Mio (> %d Mio) : le filtrage accumule la playlist en mémoire",
			croissance>>20, margeRSS>>20)
	}
}

// TestParcourirMemoireConstante garantit le même budget mémoire pour
// Parcourir, point d'entrée de la fusion/dédup des sources.
func TestParcourirMemoireConstante(t *testing.T) {
	runtime.GC()
	deja := lectureRSS(t)

	generateur := &generateurPlaylist{restantes: nombreEntrees}
	compteur := 0
	if err := Parcourir(generateur, func(Chaine) error {
		compteur++
		return nil
	}); err != nil {
		t.Fatalf("Parcourir : %v", err)
	}
	if compteur != nombreEntrees {
		t.Fatalf("%d entrées visitées, attendu %d", compteur, nombreEntrees)
	}

	runtime.GC()
	maintenant := lectureRSS(t)
	if croissance := maintenant - deja; croissance > margeRSS {
		t.Errorf("RSS a grandi de %d Mio (> %d Mio) : Parcourir accumule la playlist en mémoire",
			croissance>>20, margeRSS>>20)
	}
}
