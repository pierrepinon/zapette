package fetch

import (
	"context"
	"io"
	"time"

	"github.com/pierrepinon/zapette/internal/store"
)

// DefautPauseEntreEssais est le backoff fixe entre deux essais de fetch : il
// est fixé par la passe de conception, non configurable (5 s).
const DefautPauseEntreEssais = 5 * time.Second

// Ordonnanceur orchestre les passes de fetch : fabrique de fenêtre réseau,
// backoff fixe, horloge. Le nombre d'essais et le timeout ne sont PAS des
// champs : relus de la config à chaque passe (story 14). Il ne porte aucun
// autre état de passe : tout ce qui persiste va au store.
type Ordonnanceur struct {
	boutique         *store.Store
	fenetre          func(timeout time.Duration) Fetcher
	pauseEntreEssais time.Duration
	horloge          func() time.Time
	// signaleur est OPTIONNEL (nil-safe, jamais forcé par Nouveau) : appelé
	// aux sites d'échec déjà journalisés. Le câblage (main) y branche le
	// ring d'erreurs du serveur : fetch ne connaît pas server, la
	// dépendance est inversée par la callback (story 15 — recent-error
	// ring).
	signaleur func(string)
}

// Nouveau prépare l'ordonnanceur. pauseEntreEssais est le backoff fixe entre
// deux essais (DefautPauseEntreEssais en prod, court en test). fenetre sert à
// injecter la fenêtre réseau ; timeout et reprises sont lus de la config à
// chaque passe.
func Nouveau(boutique *store.Store, fenetre func(timeout time.Duration) Fetcher, pauseEntreEssais time.Duration) *Ordonnanceur {
	return &Ordonnanceur{
		boutique:         boutique,
		fenetre:          fenetre,
		pauseEntreEssais: pauseEntreEssais,
		horloge:          time.Now,
	}
}

// AvecSignaleur branche une callback d'échec (chaînable après Nouveau) :
// elle sera invoquée à chaque échec journalisé — par source (fetch ou
// copie en échec, copie figée) et pour chaque erreur de passe. f peut être
// nil pour détacher, le champ est nil-safe partout.
func (o *Ordonnanceur) AvecSignaleur(f func(string)) *Ordonnanceur {
	o.signaleur = f
	return o
}

// signaler transmet un message d'échec au signaleur s'il y en a un ; appelé
// exactement aux sites où l'échec est déjà journalisé, le ring de /api/status
// reflète donc ce que l'opérateur voit dans les logs.
func (o *Ordonnanceur) signaler(message string) {
	if o.signaleur != nil {
		o.signaleur(message)
	}
}

// recuperer applique la série d'essais : un premier essai puis essais
// reprises (relu de la config à chaque passe) avec un backoff fixe, arrêt
// au premier succès ou à l'annulation du contexte (pendant un essai comme
// pendant un backoff).
func (o *Ordonnanceur) recuperer(ctx context.Context, fetch Fetcher, url string, essais int) (io.ReadCloser, error) {
	var dernierErreur error
	for essai := 0; essai <= essais; essai++ {
		if essai > 0 {
			select {
			case <-ctx.Done():
				if dernierErreur == nil {
					dernierErreur = ctx.Err()
				}
				return nil, dernierErreur
			case <-time.After(o.pauseEntreEssais):
			}
		}
		corps, err := fetch(ctx, url)
		if err == nil {
			return corps, nil
		}
		dernierErreur = err
	}
	return nil, dernierErreur
}
