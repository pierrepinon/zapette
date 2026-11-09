package fetch

import (
	"context"
	"log/slog"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
)

// Run enchaîne les passes : une passe au démarrage, puis une passe à
// chaque tick du refresh_interval de la config. La passe tourne en ligne
// dans la boucle : un tick qui tomberait pendant une passe est sauté — pas
// de fetch en parallèle, jamais. La config est relue à chaque tick
// (loader.Relire) : une relecture en échec conserve la dernière bonne
// configuration et la passe continue avec elle ; le reset du ticker suit
// l'intervalle relu. Une annulation du contexte (SIGINT/SIGTERM côté
// câblage) coupe la fenêtre réseau en cours et termine proprement.
func (o *Ordonnanceur) Run(ctx context.Context, loader *config.Loader) {
	cfg, erreur := loader.Relire()
	if erreur != nil {
		// Sans config au démarrage, il n'y a ni intervalle ni sources : le
		// câblage (main) sort en échec avant d'appeler Run.
		slog.Error("chargement initial de la config impossible", "erreur", erreur)
		return
	}
	if erreur := o.Passe(ctx, cfg); erreur != nil {
		// Une passe peut avorter sur une erreur LOCALISÉE (balayage,
		// publication) ; les erreurs par source sont tracées dans
		// meta.json et la passe continue. Loguer ici suffit.
		slog.Error("passe de fetch", "erreur", erreur)
	}

	tick := time.NewTicker(cfg.RefreshInterval)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("arrêt demandé")
			return
		case <-tick.C:
			cfgRelue, erreurRelue := loader.Relire()
			switch {
			case erreurRelue == nil:
				cfg = cfgRelue
				tick.Reset(cfg.RefreshInterval)
			case cfgRelue.RefreshInterval > 0:
				// Dernière bonne conservée par le loader : on continue avec.
				slog.Warn("config invalide, dernière bonne conservée", "erreur", erreurRelue)
			default:
				// Aucune dernière bonne possible ici (le démarrage a réussi),
				// défense au cas où : la passe est sautée.
				slog.Error("config illisible et aucune dernière bonne", "erreur", erreurRelue)
				continue
			}
			if erreur := o.Passe(ctx, cfg); erreur != nil {
				slog.Error("passe de fetch", "erreur", erreur)
			}
		}
	}
}
