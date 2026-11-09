package fetch

import (
	"context"
	"io"
	"time"

	"github.com/pierrepinon/zapette/internal/store"
)

// RetourArriereDefaut est le backoff fixe entre deux essais de fetch : il
// est fixé par la passe de conception, non configurable (5 s).
const RetourArriereDefaut = 5 * time.Second

// Ordonnanceur orchestre les passes de fetch : fabrique de fenêtre réseau,
// backoff fixe, horloge. Le nombre d'essais et le timeout ne sont PAS des
// champs : relus de la config à chaque passe (story 14). Il ne porte aucun
// autre état de passe : tout ce qui persiste va au store.
type Ordonnanceur struct {
	boutique      *store.Store
	fenetre       func(timeout time.Duration) Fetcher
	retourArriere time.Duration
	horloge       func() time.Time
}

// Nouveau prépare l'ordonnanceur. retourArriere est le backoff fixe entre
// deux essais (RetourArriereDefaut en prod, court en test). fenetre sert à
// injecter la fenêtre réseau ; timeout et reprises sont lus de la config à
// chaque passe.
func Nouveau(boutique *store.Store, fenetre func(timeout time.Duration) Fetcher, retourArriere time.Duration) *Ordonnanceur {
	return &Ordonnanceur{
		boutique:      boutique,
		fenetre:       fenetre,
		retourArriere: retourArriere,
		horloge:       time.Now,
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
			case <-time.After(o.retourArriere):
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
