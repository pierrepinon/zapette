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

// Ordonnanceur orchestre les passes de fetch : fenêtre réseau, nombre
// d'essais, backoff fixe, horloge. Il ne porte aucun état de passe : tout
// ce qui persiste va au store.
type Ordonnanceur struct {
	boutique      *store.Store
	fenetre       func(timeout time.Duration) Fetcher
	essais        int
	retourArriere time.Duration
	horloge       func() time.Time
}

// Nouveau prépare l'ordonnanceur. essais est le nombre de reprises après
// le premier essai (défaut de config 3, donc 4 tentatives) ; retourArriere
// est le backoff fixe entre deux essais (RetourArriereDefaut en prod, court
// en test). fenetre sert à injecter la fenêtre réseau ; timeout est lu
// de la config à chaque passe.
func Nouveau(boutique *store.Store, fenetre func(timeout time.Duration) Fetcher, essais int, retourArriere time.Duration) *Ordonnanceur {
	return &Ordonnanceur{
		boutique:      boutique,
		fenetre:       fenetre,
		essais:        essais,
		retourArriere: retourArriere,
		horloge:       time.Now,
	}
}

// recuperer applique la série d'essais : un premier essai puis essais
// reprises avec un backoff fixe, arrêt au premier succès ou à l'annulation
// du contexte (pendant un essai comme pendant un backoff).
func (o *Ordonnanceur) recuperer(ctx context.Context, fetch Fetcher, url string) (io.ReadCloser, error) {
	var dernierErreur error
	for essai := 0; essai <= o.essais; essai++ {
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
