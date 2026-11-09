package server

import (
	"sync"
	"time"
)

// anneauErreurs garde les plafondErreurs dernières erreurs signalées pour
// le ring « errors » de /api/status : les plus récentes d'abord, avec leur
// horodatage. En production, le signaleur de l'ordonnanceur de fetch est
// branché dessus par le câblage (main) via AvecSignaleur — la dépendance
// est inversée par la callback : fetch ne connaît jamais server.
type anneauErreurs struct {
	verrou  sync.Mutex
	entrees []erreurRing
}

// SignalerErreur enregistre une erreur dans le ring : elle apparaîtra dans
// /api/status jusqu'à plafondErreurs erreurs plus récentes. Sûr en
// concurrence (ring partagé entre goroutines).
func (s *Serveur) SignalerErreur(message string) {
	s.erreurs.ajouter(erreurRing{
		At:      s.horloge().UTC().Format(time.RFC3339),
		Message: message,
	})
}

// ajouter inscrit l'erreur en tête du ring, en plafonnant la longueur.
func (a *anneauErreurs) ajouter(e erreurRing) {
	a.verrou.Lock()
	defer a.verrou.Unlock()
	a.entrees = append([]erreurRing{e}, a.entrees...)
	if len(a.entrees) > plafondErreurs {
		a.entrees = a.entrees[:plafondErreurs]
	}
}

// dernieres retourne une copie du ring pour l'exposition JSON : toujours
// une liste (vide si rien), jamais null.
func (s *Serveur) dernieresErreurs() []erreurRing {
	s.erreurs.verrou.Lock()
	defer s.erreurs.verrou.Unlock()
	sortie := make([]erreurRing, len(s.erreurs.entrees))
	copy(sortie, s.erreurs.entrees)
	return sortie
}
