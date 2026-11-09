package store

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestChargerSansMeta(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	// Premier démarrage : pas encore de meta.json, un Meta zéro est retourné
	// sans erreur — le service démarre opérationnel depuis un disque vierge.
	meta, err := s.Charger()
	if err != nil {
		t.Fatalf("Charger : %v", err)
	}
	if len(meta.Sources) != 0 {
		t.Errorf("meta zéro attendu, obtenu %d sources", len(meta.Sources))
	}
}

func TestChargerMetaCorrompue(t *testing.T) {
	module := t.TempDir()
	s, err := Ouvrir(module)
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	if err := os.WriteFile(filepath.Join(module, "meta.json"), []byte("{corrompu"), 0o644); err != nil {
		t.Fatalf("écriture du meta corrompu : %v", err)
	}
	if _, err := s.Charger(); err == nil {
		t.Fatal("Charger devait échouer sur un meta.json corrompu")
	}
}

func TestEcrireMetaPuisCharger(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	ok := time.Now().Add(-2 * time.Hour).Truncate(time.Second)
	échec := time.Now().Add(-time.Hour).Truncate(time.Second)
	suivant := time.Now().Add(time.Hour).Truncate(time.Second)

	meta := Meta{Sources: map[string]EtatSource{
		"arte": {
			M3U:   &EtatArtefact{DernierOK: &ok, SuivantA: &suivant},
			XMLTV: &EtatArtefact{DerniereErreur: "statut HTTP 500"},
		},
		"bfm": {
			M3U: &EtatArtefact{DernierOK: &échec, DerniereErreur: "délai dépassé"},
		},
	}}
	if err := s.EcrireMeta(meta); err != nil {
		t.Fatalf("EcrireMeta : %v", err)
	}

	lu, err := s.Charger()
	if err != nil {
		t.Fatalf("Charger : %v", err)
	}
	if len(lu.Sources) != 2 {
		t.Fatalf("meta relu : %d sources, attendu 2", len(lu.Sources))
	}
	if lu.Sources["arte"].M3U == nil || !lu.Sources["arte"].M3U.DernierOK.Equal(ok) {
		t.Errorf("arte.m3u.last_ok : %v", lu.Sources["arte"].M3U)
	}
	if lu.Sources["arte"].XMLTV == nil || lu.Sources["arte"].XMLTV.DerniereErreur != "statut HTTP 500" {
		t.Errorf("arte.xmltv.last_error : %v", lu.Sources["arte"].XMLTV)
	}
	if lu.Sources["arte"].XMLTV != nil && lu.Sources["arte"].XMLTV.DernierOK != nil {
		t.Errorf("arte.xmltv.last_ok devait être absent : %v", lu.Sources["arte"].XMLTV.DernierOK)
	}
	if lu.Sources["bfm"].XMLTV != nil {
		t.Errorf("bfm.xmltv devait être absent : %v", lu.Sources["bfm"].XMLTV)
	}
}
