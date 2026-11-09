package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
)

// cheminConfig provient de ZAPETTE_CONFIG, avec ./config.yaml par défaut.
func cheminConfig() string {
	if chemin := os.Getenv("ZAPETTE_CONFIG"); chemin != "" {
		return chemin
	}
	return "./config.yaml"
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	loader := config.NewLoader(cheminConfig())
	cfg, err := loader.Relire()
	if err != nil {
		slog.Error("chargement initial de la config impossible", "erreur", err)
		os.Exit(1)
	}
	consignerResume(cfg)

	// Relecture de la config à chaque tick ; une nouvelle config invalide
	// conserve la dernière bonne et se contente d'un avertissement.
	tick := time.NewTicker(cfg.RefreshInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("arrêt demandé")
			return
		case <-tick.C:
			cfgRelue, err := loader.Relire()
			if err != nil {
				slog.Warn("config invalide, dernière bonne conservée", "erreur", err)
				continue
			}
			cfg = cfgRelue
			tick.Reset(cfg.RefreshInterval)
			consignerResume(cfg)
		}
	}
}

// consignerResume journalise le résumé de la config au format JSON :
// intervalle, nombre de sources, nombre de groupes autorisés.
func consignerResume(cfg config.Config) {
	slog.Info("config chargée",
		"refresh_interval", cfg.RefreshInterval.String(),
		"sources", len(cfg.Sources),
		"allowed_groups", len(cfg.AllowedGroups),
	)
}
