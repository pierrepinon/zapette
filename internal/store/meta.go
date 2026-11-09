package store

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Meta est l'état persisté dans meta.json, écrit EN DERNIER à chaque passe :
// pour chaque source, l'état de chaque artefact (last_ok, last_error,
// next_at).
type Meta struct {
	Sources map[string]EtatSource `json:"sources"`
}

// EtatSource est l'état des artefacts d'une source ; un artefact absent du
// plan de config (URL XMLTV non renseignée, passe jamais réussie) est nil.
type EtatSource struct {
	M3U   *EtatArtefact `json:"m3u,omitempty"`
	XMLTV *EtatArtefact `json:"xmltv,omitempty"`
}

// EtatArtefact est l'état du fetch d'un artefact : quand il a réussi pour la
// dernière fois (last_ok), la dernière erreur (last_error), et quand le
// prochain fetch est prévu (next_at).
type EtatArtefact struct {
	DernierOK      *time.Time `json:"last_ok,omitempty"`
	DerniereErreur string     `json:"last_error,omitempty"`
	SuivantA       *time.Time `json:"next_at,omitempty"`
}

// EcrireMeta publie l'état par source/artefact dans meta.json, par
// temporaire + rename atomique. meta.json est le DERNIER artefact écrit de
// la passe : tant qu'il n'est pas renouvelé, les artefacts publiés
// auparavant restent ceux de la passe précédente et servis tels quels.
func (s *Store) EcrireMeta(meta Meta) error {
	if err := publier(filepath.Join(s.racine, "meta.json"), func(w io.Writer) error {
		encodeur := json.NewEncoder(w)
		encodeur.SetIndent("", "  ")
		return encodeur.Encode(&meta)
	}); err != nil {
		return fmt.Errorf("écriture de meta.json : %w", err)
	}
	return nil
}

// Charger lit meta.json ; un fichier absent (premier démarrage) retourne un
// Meta zéro sans erreur. Un fichier présent mais illisible est une erreur —
// l'état perdu vaut mieux qu'un état faux.
func (s *Store) Charger() (Meta, error) {
	brut, err := os.ReadFile(filepath.Join(s.racine, "meta.json"))
	if os.IsNotExist(err) {
		return Meta{}, nil
	}
	if err != nil {
		return Meta{}, err
	}
	var meta Meta
	if err := json.Unmarshal(brut, &meta); err != nil {
		return Meta{}, fmt.Errorf("analyse de meta.json : %w", err)
	}
	return meta, nil
}
