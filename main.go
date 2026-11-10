package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/fetch"
	"github.com/pierrepinon/zapette/internal/publicip"
	"github.com/pierrepinon/zapette/internal/server"
	"github.com/pierrepinon/zapette/internal/store"
)

// cheminConfig provient de ZAPETTE_CONFIG, avec ./config.yaml par défaut.
func cheminConfig() string {
	if chemin := os.Getenv("ZAPETTE_CONFIG"); chemin != "" {
		return chemin
	}
	return "./config.yaml"
}

// version est le stamp de version du binaire : « dev » par défaut, remplacé
// par -ldflags "-X main.version=vX.Y.Z" à la construction de release. Le
// serveur l'expose tel quel sur /api/status, le dashboard l'affiche au footer.
var version = "dev"

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
	// Le serveur HTTP : handlers de internal/server, config courante pour
	// /api/status. L'ordonnanceur tourne en arrière-plan, Run bloque jusqu'à
	// l'arrêt (SIGINT/SIGTERM), le serveur s'éteint ensuite proprement — le
	// service reste disponible pendant les fetchs, jamais l'inverse.
	// Le résolveur IP externe : fetch écho v4/v6 en arrière-plan, au
	// démarrage puis à chaque tick, timeout 5 s, jamais bloquant. Il lit la
	// config via le MÊME loader que l'ordonnanceur et /api/status
	// (loader.Relire est sûr en concurrence — le mutex du paquet config) et
	// expose son état par callback — publicip ne connaît jamais server, ni
	// fetch. La fenêtre réseau est la MÊME FABRIQUE que fetch (même
	// sémantique de timeout par tentative), adaptée au Fetcher local de
	// publicip. Un échec réseau (offline) le laisse en « indisponible » :
	// le service continue, le processus ne dépend pas de la résolution.
	resolveur := publicip.Nouveau(configCourante(loader),
		func(timeout time.Duration) publicip.Fetcher { return publicip.Fetcher(fetch.NouveauFetcher(timeout)) })
	serveur := server.Nouveau(boutique, configCourante(loader)).AvecPublicIP(resolveur.Etat).AvecVersion(version)
	// L'adresse d'écoute vient UNIQUEMENT de la config (server_addr, défaut
	// [::]:8080 appliqué à la lecture) : une ressource = un chemin. L'env
	// n'y entre que par l'expansion ${ZAPETTE_ADDR} du fichier de config.
	httpServ, ecouteur, err := demarrerHTTP(cfg.ServerAddr, serveur.Handler())
	if err != nil {
		slog.Error("écoute impossible", "adresse", cfg.ServerAddr, "erreur", err)
		os.Exit(1)
	}
	slog.Info("serveur HTTP en écoute", "adresse", ecouteur.Addr().String())

	// Le signaleur du fetch nourrit le ring d'erreurs de /api/status :
	// dépendance inversée par la callback, fetch ne connaît jamais server
	// (story 15 — recent-error ring).
	ordonnanceur := fetch.Nouveau(boutique, fetch.NouveauFetcher, fetch.DefautPauseEntreEssais).
		AvecSignaleur(serveur.SignalerErreur)
	go ordonnanceur.Run(ctx, loader)
	go resolveur.Run(ctx)
	<-ctx.Done()
	slog.Info("arrêt demandé, extinctions en cours")

	arret, arretAnnule := context.WithTimeout(context.Background(), 5*time.Second)
	defer arretAnnule()
	if err := httpServ.Shutdown(arret); err != nil {
		slog.Warn("arrêt HTTP incomplet", "erreur", err)
	}
	slog.Info("service arrêté")
}

// configCourante adapte loader.Relire (config + erreur) au lecteur attendu
// par le serveur : la reliture en échec conserve la dernière bonne config
// que le serveur exhibe telle quelle sur /api/status, l'erreur étant déjà
// tracée par l'ordonnanceur à chaque tick.
func configCourante(loader *config.Loader) func() config.Config {
	return func() config.Config {
		cfg, _ := loader.Relire()
		return cfg
	}
}

// demarrerHTTP écoute l'adresse indiquée puis sert le handler en
// arrière-plan ; l'écouteur est retourné pour les tests (port attribué) et
// l'adresse hôte réellement bindée pour le journal. L'arrêt passe par
// http.Server.Shutdown.
func demarrerHTTP(adresse string, handler http.Handler) (*http.Server, net.Listener, error) {
	ecouteur, err := net.Listen("tcp", adresse)
	if err != nil {
		return nil, nil, err
	}
	httpServ := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		if err := httpServ.Serve(ecouteur); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("serveur HTTP arrêté sur erreur", "erreur", err)
		}
	}()
	return httpServ, ecouteur, nil
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
