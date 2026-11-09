package config

import (
	"os"
	"testing"
)

// ecrire remplace tout le contenu du fichier existant.
func ecrire(t *testing.T, chemin, contenu string) {
	t.Helper()
	if err := os.WriteFile(chemin, []byte(contenu), 0o600); err != nil {
		t.Fatalf("écriture du fichier de config temporaire : %v", err)
	}
}
