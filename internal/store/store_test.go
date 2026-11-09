package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEcrireCopiePublieEtNeLaissePasDeTemporaire(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}

	err = s.EcrireCopie("arte", "m3u", func(w io.Writer) error {
		_, err := io.WriteString(w, "#EXTM3U\n#EXTINF:-1,Arte\nhttps://flux.exemple.tld/arte.m3u8\n")
		return err
	})
	if err != nil {
		t.Fatalf("EcrireCopie : %v", err)
	}

	lu, err := os.ReadFile(s.CheminCopie("arte", "m3u"))
	if err != nil {
		t.Fatalf("lecture de la copie : %v", err)
	}
	if !strings.Contains(string(lu), "#EXTINF:-1,Arte") {
		t.Errorf("copie publiée : %q", lu)
	}

	entrées, err := os.ReadDir(filepath.Join(s.racine, "sources"))
	if err != nil {
		t.Fatalf("lecture de sources/ : %v", err)
	}
	if len(entrées) != 1 {
		t.Errorf("fichiers résiduels dans sources/ : %d", len(entrées))
	}
	for _, e := range entrées {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temporaire résiduel : %s", e.Name())
		}
	}
}

func TestEcrireCopieErreurFigeLaCopiePrecedente(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	ancien := "#EXTM3U\n#EXTINF:-1,Bonne\nhttps://flux.exemple.tld/bonne.m3u8\n"
	if err := s.EcrireCopie("arte", "m3u", func(w io.Writer) error {
		_, err := io.WriteString(w, ancien)
		return err
	}); err != nil {
		t.Fatalf("EcrireCopie : %v", err)
	}

	echec := fmt.Errorf("fetch interrompu")
	err = s.EcrireCopie("arte", "m3u", func(w io.Writer) error { return echec })
	if err == nil {
		t.Fatal("EcrireCopie devait échouer")
	}
	if !errors.Is(err, echec) {
		t.Errorf("erreur transmise : %v", err)
	}

	lu, err := os.ReadFile(s.CheminCopie("arte", "m3u"))
	if err != nil {
		t.Fatalf("lecture de la copie : %v", err)
	}
	if string(lu) != ancien {
		t.Errorf("copie figée attendue, obtenu %q", lu)
	}
}
