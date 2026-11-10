// Doublon sous le runtime race (//go:build race) du test à budget RSS de
// memoire_test.go (//go:build !race) : l'instrumentation mémoire du
// détecteur de course gonfle le RSS et rend la mesure non significative.
// Ce test saute donc sous -race pour que la suite reste verte — c'est elle
// qui a attrapé la data race du Loader, on la garde dans la CI. Le budget
// réel reste certifié par le harnais d'acceptance sur binaire de
// production (adjudication revue round 15 : option (a), tags de build).
//go:build race

package xmltv

import "testing"

// TestFiltrerMemoireConstante est la version « skip » du test à budget RSS
// de memoire_test.go : voir l'en-tête de fichier.
func TestFiltrerMemoireConstante(t *testing.T) {
	t.Skip("budget RSS non significatif sous -race (instrumentation mémoire du détecteur) : certifié par le harnais d'acceptance sur binaire de production")
}
