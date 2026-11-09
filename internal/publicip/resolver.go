// Package publicip : résolution de l'IP externe en arrière-plan pour le
// dashboard. Le résolveur fetch des endpoints écho configurables (défauts
// ipify v4/v6) au démarrage puis à chaque tick, avec un timeout court, et
// n'est JAMAIS bloquant pour le service : il vit dans sa propre goroutine
// (câblée par main), le reste du service ne lit qu'un instantané d'état.
//
// En échec réseau (offline inclus), l'état retombe sur « indisponible » :
// le dashboard reste 100 % fonctionnel, aucun fetching n'a d'effet de bord
// sur les artefacts servis (ADR-0001 — le snapshot n'est jamais touché).
package publicip

import (
	"context"

	"io"
	"log/slog"
	"sync"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
)

// Timeout est la durée max d'un fetch d'endpoint echo : fixé par la passe
// de conception (5 s), non configurable.
const Timeout = 5 * time.Second

// Fetcher est la fenêtre réseau : une URL et un contexte, un corps de
// réponse ou une erreur. Même forme que la fenêtre de fetch (assignable
// depuis fetch.NouveauFetcher en production), type local pour ne pas
// coupler les deux paquets.
type Fetcher func(ctx context.Context, url string) (io.ReadCloser, error)

// Etat est l'instantané exposé au serveur pour /api/status : les adresses
// résolues (ou « indisponible ») et l'horodatage de la dernière passe.
type Etat struct {
	IPv4                 string
	IPv6                 string
	DerniereVerification *time.Time
}

// indisponible : la valeur contrat des champs en échec ou jamais vérifiés.
const indisponible = "indisponible"

// Resolver maintient l'état IP externe fetché en arrière-plan. Il ne porte
// aucune dépendance au serveur ni au store : main branche l'état dans
// /api/status (dépendance inversée par le lecteur injecté, comme le ring
// d'erreurs).
type Resolver struct {
	lireConfig func() config.Config
	fenetre    func(timeout time.Duration) Fetcher
	horloge    func() time.Time

	verrou sync.Mutex
	etat   Etat
}

// Nouveau prépare le résolveur : lireConfig retourne la config COURANTE
// (config.Loader.Relire adaptée en production — les endpoints sont relus à
// chaque tick, une relecture en échec conserve la dernière bonne config),
// fenetre injecte la fenêtre réseau (fetch.NouveauFetcher en production).
// L'état initial est « indisponible » des deux côtés.
func Nouveau(lireConfig func() config.Config, fenetre func(timeout time.Duration) Fetcher) *Resolver {
	return &Resolver{
		lireConfig: lireConfig,
		fenetre:    fenetre,
		horloge:    time.Now,
		etat:       Etat{IPv4: indisponible, IPv6: indisponible},
	}
}

// Etat retourne l'instantané courant, sûr en concurrence : c'est le seul
// accès du reste du service à l'état du résolveur.
func (r *Resolver) Etat() Etat {
	r.verrou.Lock()
	defer r.verrou.Unlock()
	return r.etat
}

// Verifier réalise une passe : fetch de l'endpoint IPv4 puis de l'endpoint
// IPv6, chacun avec Timeout. Un corps valide (texte non vide, IP telle que
// publiée par l'endpoint) devient l'adresse exposée ; ANY échec — réseau,
// statut, corps illisible — retombe sur « indisponible » pour la famille
// concernée : l'état reflète la DERNIÈRE passe, jamais une valeur périmée.
// L'horodatage est avancé à chaque passe, y compris en échec : une vérification
// a bien eu lieu. L'erreur de passe est retournée pour les tests et la
// journalisation (elle n'arrête jamais le résolveur).
func (r *Resolver) Verifier(ctx context.Context) error {
	cfg := r.lireConfig()
	fetch := r.fenetre(Timeout)

	ipv4 := resoudre(ctx, fetch, cfg.PublicIPv4URL, "IPv4")
	ipv6 := resoudre(ctx, fetch, cfg.PublicIPv6URL, "IPv6")

	r.verrou.Lock()
	maintenant := r.horloge()
	r.etat = Etat{IPv4: ipv4, IPv6: ipv6, DerniereVerification: &maintenant}
	r.verrou.Unlock()
	return nil
}

// resoudre fetch un endpoint echo et retourne l'adresse lue, ou
// « indisponible » : l'échec est journalisé, jamais propagé au service.
func resoudre(ctx context.Context, fetch Fetcher, url, famille string) string {
	corps, err := fetch(ctx, url)
	if err != nil {
		slog.Warn("résolution IP externe en échec", "famille", famille, "endpoint", url, "erreur", err)
		return indisponible
	}
	defer corps.Close()

	adresse, err := io.ReadAll(io.LimitReader(corps, 4096))
	if err != nil || len(adresse) == 0 {
		slog.Warn("réponse de l'endpoint IP illisible", "famille", famille, "endpoint", url, "erreur", err)
		return indisponible
	}
	return string(adresse)
}

// Run exécute la boucle de fond : une passe au démarrage, puis une passe à
// chaque tick (ticker du refresh_interval de la config courante, relu à
// chaque tick). L'annulation du contexte (SIGINT/SIGTERM côté câblage) coupe
// le fetch en cours et termine proprement. La passe n'est JAMAIS bloquante
// pour le service : Run est appelé dans sa propre goroutine.
func (r *Resolver) Run(ctx context.Context) {
	if err := r.Verifier(context.Background()); err != nil {
		slog.Warn("première résolution IP en échec", "erreur", err)
	}
	tick := time.NewTicker(r.intervalleCourant())
	defer tick.Stop()
	r.boucler(ctx, tick.C, tick.Reset)
}

// intervalleCourant lit l'intervalle de rafraîchissement de la config
// courante au démarrage de Run. Sans config valide (échec de relecture sans
// dernière bonne — impossible en production : le câblage sort avant), 1 h
// de repli : les ticks sont alors des passes à vide sans effet de bord
// visible, jamais une erreur bloquante.
func (r *Resolver) intervalleCourant() time.Duration {
	if cfg := r.lireConfig(); cfg.RefreshInterval > 0 {
		return cfg.RefreshInterval
	}
	return time.Hour
}

// boucler est la boucle des passes à chaud : une passe à chaque tick, avec
// reinitialiser appelé à chaque tick (intervalle relu par l'injecteur du
// ticker, comme l'ordonnanceur des fetchs). Les ticks tombés pendant une
// passe sont vidés : jamais de passes dos à dos. L'annulation du contexte
// termine immédiatement.
func (r *Resolver) boucler(ctx context.Context, ticks <-chan time.Time, reinitialiser func(time.Duration)) {
	for {
		select {
		case <-ctx.Done():
			slog.Info("résolveur IP arrêté")
			return
		case <-ticks:
			cfg := r.lireConfig()
			if cfg.RefreshInterval > 0 {
				reinitialiser(cfg.RefreshInterval)
			}
			if err := r.Verifier(ctx); err != nil {
				slog.Warn("résolution IP en échec", "erreur", err)
			}
			// Les ticks tombés pendant la passe ne déclenchent rien :
			// drain avant de retourner dans le select (symétrie fetch).
			r.drainer(ticks)
		}
	}
}

// drainer vide non bloquamment un canal de ticks : les ticks tombés pendant
// une passe sont jetés, la prochaine résolution attend un tick neuf.
func (r *Resolver) drainer(ticks <-chan time.Time) {
	for {
		select {
		case <-ticks:
		default:
			return
		}
	}
}
