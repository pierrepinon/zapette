package config

import (
	"os"
	"path/filepath"
	"testing"
)

// ecrireTemp écrit le contenu YAML dans un fichier temporaire et retourne son chemin.
func ecrireTemp(t *testing.T, contenu string) string {
	t.Helper()
	chemin := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(chemin, []byte(contenu), 0o600); err != nil {
		t.Fatalf("écriture du fichier de config temporaire : %v", err)
	}
	return chemin
}
