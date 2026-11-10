package config

import (
	"fmt"
	"sync"
)

// Loader relit le fichier de config au fil des ticks et garde la dernière
// bonne configuration : une relecture en échec retourne l'erreur accompagnée
// de la précédente configuration valide, si elle existe.
//
// SÛR EN CONCURRENCE : la MÊME instance est partagée entre la goroutine de
// l'ordonnanceur (Relire à chaque tick) et les handlers HTTP (Relire à
// chaque /api/status, concurrents entre eux) — Relire verrouille les accès
// à la dernière bonne config : config est le propriétaire naturel de la
// garantie, les appelants n'ont rien à synchroniser.
type Loader struct {
	chemin string

	verrou        sync.Mutex
	derniereBonne *Config
}

// NewLoader prépare la relecture du fichier de config indiqué.
func NewLoader(path string) *Loader {
	return &Loader{chemin: path}
}

// Relire charge le fichier ; en cas d'erreur d'analyse, la dernière bonne
// configuration est retournée telle quelle (dernière bonne conservée) avec
// l'erreur correspondante.
func (l *Loader) Relire() (Config, error) {
	l.verrou.Lock()
	defer l.verrou.Unlock()
	cfg, err := Load(l.chemin)
	if err != nil {
		if l.derniereBonne != nil {
			return *l.derniereBonne, fmt.Errorf("config conservée, %w", err)
		}
		return Config{}, err
	}
	l.derniereBonne = &cfg
	return cfg, nil
}
