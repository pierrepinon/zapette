package config

import (
	"os"
	"sync"
	"testing"
	"time"
)

const (
	// bonneA / configInvalide : les deux états alternés du fichier pendant le
	// marteau concurrent. Toute relecture DOIT retourner soit une config
	// valide (frais disque), soit la dernière bonne — jamais une config vide.
	bonneA = `
refresh_interval: 10m
sources:
  - name: provider-a
    m3u_url: https://exemple-a.tld/m3u
`
	configInvalide = `
refresh_interval: [pas une durée
`
)

// TestRelireEnConcurrence : Relire est appelé DESTRUCTIONS simultanées sur la
// MÊME instance — la goroutine de l'ordonnanceur (Relire à chaque tick) ET
// les handlers HTTP concurrents (Relire à chaque /api/status) — pendant que
// le fichier bascule entre bon et invalide. derniereBonne est écrit à chaque
// relecture réussie et lu à chaque échec : sans synchronisation, l'écriture/
// l'écriture et la lecture/écriture concurrentes du pointeur sont une DATA
// RACE (comportement indéfini). Le contrat est néanmoins intangible : toute
// relecture retourne une config valide, l'erreur (si le fichier est invalide
// à l'instant) emmenant la dernière bonne — jamais une config vide.
func TestRelireEnConcurrence(t *testing.T) {
	chemin := ecrireTemp(t, bonneA)
	loader := NewLoader(chemin)

	if _, err := loader.Relire(); err != nil {
		t.Fatalf("relecture d'amorce : erreur inattendue : %v", err)
	}

	stop := make(chan struct{})
	var redacteur sync.WaitGroup
	redacteur.Add(1)
	go func() {
		defer redacteur.Done()
		bon := true
		for {
			select {
			case <-stop:
				return
			default:
			}
			// Écriture directe (goroutine impersonnelle : pas de t.Fatalf ici) ;
			// le fichier bascule golfe par golf entre bon et invalide.
			contenu := configInvalide
			if bon {
				contenu = bonneA
			}
			_ = os.WriteFile(chemin, []byte(contenu), 0o600)
			bon = !bon
			time.Sleep(50 * time.Microsecond)
		}
	}()

	var lecteurs sync.WaitGroup
	const nbLecteurs = 8
	const releituresParLecteur = 200
	erreurs := make(chan error, nbLecteurs)
	for i := 0; i < nbLecteurs; i++ {
		lecteurs.Add(1)
		go func() {
			defer lecteurs.Done()
			for j := 0; j < releituresParLecteur; j++ {
				cfg, err := loader.Relire()
				switch {
				case err != nil && (cfg.RefreshInterval != 10*time.Minute || len(cfg.Sources) != 1):
					// La dernière bonne DOIT accompagner l'erreur, jamais vide.
					erreurs <- nil
					return
				case err == nil && (cfg.RefreshInterval != 10*time.Minute || len(cfg.Sources) != 1):
					// Une relecture réussie ne peut vouloir dire autre chose.
					erreurs <- nil
					return
				}
			}
		}()
	}
	lecteurs.Wait()
	close(stop)
	redacteur.Wait()
	select {
	case <-erreurs:
		t.Fatal("contrat violé sous concurrence : une relecture a retourné une config vide ou inattendue")
	default:
	}
}
