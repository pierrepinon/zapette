package config

import "fmt"

// Loader relit le fichier de config au fil des ticks et garde la dernière
// bonne configuration : une relecture en échec retourne l'erreur accompagnée
// de la précédente configuration valide, si elle existe.
type Loader struct {
	chemin        string
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
