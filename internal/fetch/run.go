package fetch

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
)

// Run enchaîne les passes : une passe au démarrage, puis les passes à
// chaud via boucler sur un ticker du refresh_interval de la config. Une
// annulation du contexte (SIGINT/SIGTERM côté câblage) coupe la fenêtre
// réseau en cours et termine proprement.
func (o *Ordonnanceur) Run(ctx context.Context, loader *config.Loader) {
	cfg, erreur := loader.Relire()
	if erreur != nil {
		// Sans config au démarrage, il n'y a ni intervalle ni sources : le
		// câblage (main) sort en échec avant d'appeler Run.
		slog.Error("chargement initial de la config impossible", "erreur", erreur)
		return
	}
	if erreur := o.passerEnSignalant(ctx, cfg); erreur != nil {
		// Une passe peut avorter sur une erreur LOCALISÉE (balayage,
		// publication) ; les erreurs par source sont tracées dans
		// meta.json et la passe continue. Loguer ici suffit.
		slog.Error("passe de fetch", "erreur", erreur)
	}
	tick := time.NewTicker(cfg.RefreshInterval)
	defer tick.Stop()
	o.boucler(ctx, loader, tick.C, tick.Reset)
}

// boucler est la boucle des passes à chaud : une passe à chaque tick. La
// config est relue à chaque tick (loader.Relire) : une relecture en échec
// conserve la dernière bonne configuration et la passe continue avec elle ;
// une relecture réussie suit l'intervalle relu via reinitialiser. À la fin
// de CHAQUE passe, les ticks tombés pendant la passe sont vidés (drain non
// bloquant) : ils ne déclenchent jamais une passe immédiate — le fetch
// suivant attend un tick neuf, pas de passes dos à dos, le fournisseur
// n'est pas martelé. Une annulation du contexte coupe la fenêtre réseau en
// cours et termine proprement.
func (o *Ordonnanceur) boucler(ctx context.Context, loader *config.Loader, ticks <-chan time.Time, reinitialiser func(time.Duration)) {
	cfg, _ := loader.Relire() // démarrage garanti, sert quand la relecture échoue sans dernière bonne
	for {
		select {
		case <-ctx.Done():
			slog.Info("arrêt demandé")
			return
		case <-ticks:
			cfgRelue, erreurRelue := loader.Relire()
			switch {
			case erreurRelue == nil:
				cfg = cfgRelue
				reinitialiser(cfg.RefreshInterval)
			case cfgRelue.RefreshInterval > 0:
				// Dernière bonne conservée par le loader : on continue avec.
				cfg = cfgRelue
				slog.Warn("config invalide, dernière bonne conservée", "erreur", erreurRelue)
			default:
				// Démarrage déjà réussi, donc une dernière bonne existe ;
				// garde-fou : la passe est sautée.
				slog.Error("config illisible et aucune dernière bonne", "erreur", erreurRelue)
				continue
			}
			if erreur := o.passerEnSignalant(ctx, cfg); erreur != nil {
				slog.Error("passe de fetch", "erreur", erreur)
			}
			// Les ticks tombés pendant la passe ne déclenchent rien : drain
			// avant de retourner dans le select, sinon le tick retenu par le
			// tampon partirait immédiatement (dérive d'un cycle) et, en
			// régime passe ≥ intervalle, les passes s'enchaîneraient sans
			// répit.
			drainer(ticks)
		}
	}
}

// drainer vide non bloquamment un canal de ticks : les ticks tombés
// pendant une passe sont jetés, le prochain fetch attend un tick neuf.
// Dupliqué assumé (revue #12) avec le drainer de publicip/resolver.go :
// même sémantique de drain de ticker, deux paquets sans dépendance
// partagée — les deux commentaires se renvoient l'un à l'autre.
func drainer(ticks <-chan time.Time) {
	for {
		select {
		case <-ticks:
		default:
			return
		}
	}
}

// passerEnSignalant enrobe Passe : l'erreur locale d'une passe (balayage,
// reconstruction, lecture de l'état) est journalisée ICI — un seul site —
// et signalée au signaleur injecté, comme les échecs par source le sont
// depuis Passe. Le ring de /api/status reflète ainsi tout échec visible
// dans les logs (story 15).
func (o *Ordonnanceur) passerEnSignalant(ctx context.Context, cfg config.Config) error {
	erreur := o.Passe(ctx, cfg)
	if erreur != nil {
		o.signaler(fmt.Sprintf("passe de fetch : %v", erreur))
	}
	return erreur
}
