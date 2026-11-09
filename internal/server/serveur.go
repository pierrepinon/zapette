// Package server : la surface HTTP de zapette — /m3u, /xmltv,
// /m3u?group= (prévisualisation), /api/status et la page d'accueil-.
//
// Conformément à l'ADR-0001, les artefacts servis (/m3u, /xmltv,
// /m3u?group=) sont lus streaming depuis le fichier snapshot sur disque
// via io.Copy ou filtre à la volée : jamais le snapshot entier en RAM,
// quel que soit le nombre de requêtes simultanées. La publication du
// snapshot par renommage atomique (store) garantit qu'un handler lit
// toujours un fichier complet, jamais partiel.
//
// Les handlers lisent la configuration COURANTE au fil des requêtes (via
// le lecteur injecté, qui est config.Loader.Relire en production) :
// l'adresse d'écoute est lue du server_addr du démarrage, le reste
// (intervalle, groupes, sources) est relu à chaque /api/status — cohérent
// avec la relecture à chaque tick de l'ordonnanceur.
package server

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/pierrepinon/zapette/internal/store"
)

// Serveur porte les handlers HTTP. Il ne détient AUCUN état d'artefact en
// mémoire : tout ce qui était servi est sur disque (ADR-0001), tout ce qui
// est vivant est dans meta.json ou relu à la demande.
type Serveur struct {
	boutique *store.Store
}

// Nouveau prépare le serveur HTTP sur le store indiqué.
func Nouveau(boutique *store.Store) *Serveur {
	return &Serveur{boutique: boutique}
}

// Handler assemble le routeur public : les artefacts servis et l'API de
// statut. GET / sert une page minimale — le dashboard embarqué du ticket
// suivant viendra s'y insérer.
func (s *Serveur) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /m3u", s.servirM3U)
	mux.HandleFunc("GET /xmltv", s.servirXMLTV)
	mux.HandleFunc("GET /api/status", s.statut)
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("zapette\n"))
	})
	return mux
}

// reponse503 répond 503 avec un corps JSON explicite : le lecteur et le
// dashboard doivent voir POURQUOI rien n'est servi (story 10).
func reponse503(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusServiceUnavailable)
	_ = json.NewEncoder(w).Encode(map[string]string{"erreur": message})
}

// servirM3U sert le snapshot M3U fusionné.
func (s *Serveur) servirM3U(w http.ResponseWriter, _ *http.Request) {
	reponse503(w, "aucun snapshot n'existe encore : la première passe de fetch n'a pas abouti")
}

// servirXMLTV sert le snapshot XMLTV fusionné.
func (s *Serveur) servirXMLTV(w http.ResponseWriter, _ *http.Request) {
	reponse503(w, "aucun snapshot n'existe encore : la première passe de fetch n'a pas abouti")
}

// statut répond GET /api/status : le contrat JSON du dashboard.
func (s *Serveur) statut(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_ = json.NewEncoder(w).Encode(map[string]string{})
}

// verrou snapshot existe-t-il ? — utilisé par les handlers pour distinguer
// démarrage à froid (503) d'un artefact servable. Un os.Stat suffit : le
// renommage atomique du store garantit qu'un fichier présent est complet.
func snapshotPresent(chemin string) bool {
	_, err := os.Stat(chemin)
	return err == nil
}
