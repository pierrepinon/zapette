package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/fetch"
	"github.com/pierrepinon/zapette/internal/store"
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

	// Le store : copies de source, snapshot fusionné et meta.json sur
	// disque — publié par renommages atomiques, jamais de fichier partiel.
	boutique, err := store.Ouvrir(cfg.DataDir)
	if err != nil {
		slog.Error("ouverture du répertoire de données impossible", "répertoire", cfg.DataDir, "erreur", err)
		os.Exit(1)
	}

	// L'ordonnanceur : fetch séquentiel au démarrage puis à chaque tick,
	// config relue à chaque tick (dernière bonne conservée), fetch au
	// timeout de la config avec 1+reprises et backoff fixe. SIGINT/SIGTERM
	// coupent le fetch en cours ; Run termine proprement.
	ordonnanceur := fetch.Nouveau(boutique, fetch.NouveauFetcher, fetch.RetourArriereDefaut)
	ordonnanceur.Run(ctx, loader)
	slog.Info("service arrêté")
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
