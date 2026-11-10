// Doublon sous le runtime race (//go:build race) de la case d'acceptation
// volumétrique RSS de rss216_test.go (//go:build !race) : le budget de la
// spec (100 Mio pour 216 000 entrées) se mesure SUR BINAIRE DE PRODUCTION —
// l'instrumentation mémoire du détecteur de course gonfle le RSS et rend la
// mesure non significative. Le test saute donc sous -race pour que la suite
// reste verte (adjudication revue round 15, option a : tags de build) ; le
// budget reste certifié par rss216_test.go hors -race et par
// scripts/acceptance.sh.
//go:build race

package acceptance

import "testing"

// TestAcceptationRSS216k est la version « skip » du harnais RSS 216k : voir
// l'en-tête de fichier.
func TestAcceptationRSS216k(t *testing.T) {
	t.Skip("budget RSS non significatif sous -race (instrumentation mémoire du détecteur, binaire != production) : certifié par rss216_test.go hors -race et par scripts/acceptance.sh")
}
