// Package server : la surface HTTP de zapette — /m3u, /xmltv,
// /m3u?group= (prévisualisation), /api/status et la page d'accueil.
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
// l'adresse d'écoute est fixée au démarrage par le câblage (main) depuis
// server_addr, le reste (intervalle, sources, groupes) est relu à chaque
// /api/status — la dernière bonne config du loader masque les erreurs de
// relecture, cohérent avec l'ordonnanceur.
package server

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/m3u"
	"github.com/pierrepinon/zapette/internal/store"
)

// plafondErreurs : le ring /api/status conserve les 20 dernières erreurs.
const plafondErreurs = 20

// Serveur porte les handlers HTTP. Il ne détient AUCUN artefact en
// mémoire : tout ce qui est servi est sur disque (ADR-0001), tout ce qui
// est vivant est relu de meta.json et de la config à chaque requête. Seule
// dérogation : les compteurs par groupe sont memoïsés PAR VERSION du
// snapshot (taille + horodatage du fichier) — un dump de 200 000 entrées
// ne doit pas être balayé à chaque poll de 5 s du dashboard.
type Serveur struct {
	boutique    *store.Store
	lireConfig  func() config.Config
	horloge     func() time.Time
	signalerFct func(string)

	verrou    sync.Mutex
	clefCache string // "taille:modtime" du snapshot.m3u déjà compté
	compteurs compteursSnapshot
	erreurs   anneauErreurs
}

type compteursSnapshot struct {
	Total     int
	ParGroupe map[string]int
}

// Nouveau prépare le serveur HTTP. lireConfig est invoqué à chaque
// /api/status (config.Loader.Relire en production) ; le serveur ignore son
// erreur : le loader retourne déjà la dernière bonne configuration.
func Nouveau(boutique *store.Store, lireConfig func() config.Config) *Serveur {
	return &Serveur{
		boutique:   boutique,
		lireConfig: lireConfig,
		horloge:    time.Now,
	}
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

// servirM3U sert le snapshot M3U fusionné ; avec ?group=<nom>, la
// prévisualisation brute de ce seul groupe : filtrage à la volée en
// streaming depuis le snapshot stocké (m3u.Filtrer), qui ne MODIFIE jamais
// le snapshot (story 17). Le match est exact et case-sensitive, comme le
// filtrage à la réception.
func (s *Serveur) servirM3U(w http.ResponseWriter, r *http.Request) {
	groupe := r.URL.Query().Get("group")
	if groupe == "" {
		servirArtefact(w, s.boutique.CheminSnapshot("m3u"), "audio/x-mpegurl; charset=utf-8")
		return
	}
	servirPrevisu(w, s.boutique.CheminSnapshot("m3u"), groupe)
}

// servirPrevisu sert le M3U snapshot filtré à la volée sur un groupe :
// le fichier est ouvert ici et la sortie du filtre est streamée vers la
// réponse — le snapshot n'est pas réécrit, la mémoire reste constante
// (ADR-0001).
func servirPrevisu(w http.ResponseWriter, chemin, groupe string) {
	snapshot, err := os.Open(chemin)
	if err != nil {
		if os.IsNotExist(err) {
			reponse503(w, "aucun snapshot n'existe encore : la première passe de fetch n'a pas abouti")
			return
		}
		slog.Error("ouverture du snapshot impossible", "fichier", chemin, "erreur", err)
		http.Error(w, "erreur interne", http.StatusInternalServerError)
		return
	}
	defer snapshot.Close()

	w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if err := m3u.Filtrer(snapshot, w, []string{groupe}); err != nil {
		slog.Warn("prévisu de groupe interrompue", "groupe", groupe, "erreur", err)
	}
}

// servirXMLTV sert le snapshot XMLTV fusionné.
func (s *Serveur) servirXMLTV(w http.ResponseWriter, _ *http.Request) {
	servirArtefact(w, s.boutique.CheminSnapshot("xmltv"), "application/xml; charset=utf-8")
}

// servirArtefact sert un fichier du disque en streaming : io.Copy depuis le
// fichier ouvert, jamais tout l'artefact en RAM (ADR-0001). Content-Length
// est connu d'avance (taille du fichier) ; le no-cache garantit que le
// lecteur reçoit toujours le snapshot frais. Sans fichier (démarrage à
// froid, avant la première passe réussie) : 503 JSON explicite (story 10).
func servirArtefact(w http.ResponseWriter, chemin, typeContenu string) {
	snapshot, err := os.Open(chemin)
	if err != nil {
		if os.IsNotExist(err) {
			reponse503(w, "aucun snapshot n'existe encore : la première passe de fetch n'a pas abouti")
			return
		}
		slog.Error("ouverture du snapshot impossible", "fichier", chemin, "erreur", err)
		http.Error(w, "erreur interne", http.StatusInternalServerError)
		return
	}
	defer snapshot.Close()

	info, err := snapshot.Stat()
	if err != nil {
		slog.Error("stat du snapshot impossible", "fichier", chemin, "erreur", err)
		http.Error(w, "erreur interne", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", typeContenu)
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Length", strconv.FormatInt(info.Size(), 10))
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, snapshot)
}

// ——— contrat /api/status ————————————————————————————————

// etatArtefactStatus est l'état exposé d'un artefact d'une source : les
// mêmes champs que meta.json (last_ok, last_error, next_at), avec null
// explicite pour les absents — le contrat ne dépend pas d'omitempty.
type etatArtefactStatus struct {
	LastOk    *string `json:"last_ok"`
	LastError string  `json:"last_error"`
	NextAt    *string `json:"next_at"`
}

type etatSourceStatus struct {
	M3U   etatArtefactStatus `json:"m3u"`
	XMLTV etatArtefactStatus `json:"xmltv"`
}

type snapshotStatus struct {
	AgeSeconds    *int64         `json:"age_seconds"`
	TotalChannels int            `json:"total_channels"`
	PerGroup      map[string]int `json:"per_group"`
}

type erreurRing struct {
	At      string `json:"at"`
	Message string `json:"message"`
}

type resumeConfig struct {
	RefreshInterval string `json:"refresh_interval"`
	NbSources       int    `json:"nb_sources"`
	NbGroupes       int    `json:"nb_groupes"`
}

type statutStatus struct {
	GeneratedAt       string                      `json:"generated_at"`
	Sources           map[string]etatSourceStatus `json:"sources"`
	Snapshot          snapshotStatus              `json:"snapshot"`
	RssBytes          int64                       `json:"rss_bytes"`
	Errors            []erreurRing                `json:"errors"`
	Config            resumeConfig                `json:"config"`
	PublicIPv4        string                      `json:"public_ipv4"`
	PublicIPv6        string                      `json:"public_ipv6"`
	PublicIPLastCheck *string                     `json:"public_ip_last_check"`
}

// statut répond GET /api/status (story 15) : l'état par source/artefact
// vient de meta.json relu à chaque requête (source unique de vérité, à jour
// à chaque passe), les compteurs du snapshot du balayage streamé memoïsé
// par version de fichier, rss_bytes de /proc/self/status VmRSS, le ring
// des erreurs signalées (SignalerErreur), le résumé de la config courante.
// public_ipv4/ipv6/public_ip_last_check sont exposés « indisponible » :
// le ticket publicip viendra les remplir, le format ne changera pas.
func (s *Serveur) statut(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")

	reponse := statutStatus{
		GeneratedAt:       s.horloge().UTC().Format(time.RFC3339),
		Sources:           map[string]etatSourceStatus{},
		Config:            resumeConfig{RefreshInterval: "0s"},
		PublicIPv4:        "indisponible",
		PublicIPv6:        "indisponible",
		PublicIPLastCheck: nil,
		Errors:            []erreurRing{},
	}

	if meta, err := s.boutique.Charger(); err != nil {
		slog.Warn("meta.json illisible pour /api/status", "erreur", err)
	} else {
		for nom, etat := range meta.Sources {
			reponse.Sources[nom] = etatSourceStatus{
				M3U:   etatStatus(etat.M3U),
				XMLTV: etatStatus(etat.XMLTV),
			}
		}
	}

	if cfg := s.lireConfig(); cfg.RefreshInterval > 0 || len(cfg.Sources) > 0 {
		reponse.Config = resumeConfig{
			RefreshInterval: cfg.RefreshInterval.String(),
			NbSources:       len(cfg.Sources),
			NbGroupes:       len(cfg.AllowedGroups),
		}
	}

	reponse.Snapshot = s.compteSnapshot()
	reponse.RssBytes = lireRSS()
	reponse.Errors = s.dernieresErreurs()

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(reponse)
}

// etatStatus convertit l'état persisté du store en état exposé : les
// horodatages partent en RFC3339 UTC, l'erreur vide reste vide.
func etatStatus(etat *store.EtatArtefact) etatArtefactStatus {
	if etat == nil {
		return etatArtefactStatus{}
	}
	rfc3339 := func(t *time.Time) *string {
		if t == nil {
			return nil
		}
		utc := t.UTC().Format(time.RFC3339)
		return &utc
	}
	return etatArtefactStatus{
		LastOk:    rfc3339(etat.DernierOK),
		LastError: etat.DerniereErreur,
		NextAt:    rfc3339(etat.SuivantA),
	}
}

// compteSnapshot retourne âge, total et compteurs par groupe du snapshot
// M3U courant. Sans snapshot : âge nil, compteurs à zéro. Le balayage est
// en streaming (m3u.Parcourir, ADR-0001) et memoïsé par version de fichier
// (taille + mtime) : le poll de 5 s du dashboard ne re-balaye pas un
// snapshot inchangé, une passe nouvelle l'invalide.
func (s *Serveur) compteSnapshot() snapshotStatus {
	chemin := s.boutique.CheminSnapshot("m3u")
	info, err := os.Stat(chemin)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("stat du snapshot impossible pour /api/status", "fichier", chemin, "erreur", err)
		}
		return snapshotStatus{}
	}

	clef := fmt.Sprintf("%d:%d", info.Size(), info.ModTime().UnixNano())
	s.verrou.Lock()
	detecte := s.clefCache == clef
	compte := s.compteurs
	s.verrou.Unlock()

	if !detecte {
		compte = compterGroupes(chemin)
		s.verrou.Lock()
		// Deux écritures peuvent se croiser (recompte d'une version plus
		// ancienne) : on ne retient que la version demandée TARDIVEMENT.
		s.clefCache = clef
		s.compteurs = compte
		s.verrou.Unlock()
	}

	age := int64(s.horloge().Sub(info.ModTime()).Seconds())
	return snapshotStatus{
		AgeSeconds:    &age,
		TotalChannels: compte.Total,
		PerGroup:      compte.ParGroupe,
	}
}

// compterGroupes balaye le snapshot M3U en streaming et compte les chaînes
// par groupe. L'entrée sans group-title (possible quand allowed_groups est
// vide — tout conserver) est comptée au total mais absente de per_group :
// une clé vide n'est pas un groupe.
func compterGroupes(chemin string) compteursSnapshot {
	compte := compteursSnapshot{ParGroupe: map[string]int{}}
	f, err := os.Open(chemin)
	if err != nil {
		slog.Warn("balayage du snapshot impossible pour /api/status", "fichier", chemin, "erreur", err)
		return compte
	}
	defer f.Close()

	_ = m3u.Parcourir(bufio.NewReaderSize(f, 64*1024), func(c m3u.Chaine) error {
		compte.Total++
		if c.Groupe != "" {
			compte.ParGroupe[c.Groupe]++
		}
		return nil
	})
	return compte
}

// lireRSS lit VmRSS dans /proc/self/status (kio → octets) : le MemoryStream
// RÉEL du processus, lu comme le dashboard s'attend à le voir. Hors Linux
// ou parse impossible : 0, le dashboard l'affiche comme indisponible.
func lireRSS() int64 {
	status, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, ligne := range strings.Split(string(status), "\n") {
		if valeur, ok := strings.CutPrefix(ligne, "VmRSS:"); ok {
			champs := strings.Fields(valeur)
			if len(champs) >= 1 {
				if kio, err := strconv.ParseInt(champs[0], 10, 64); err == nil {
					return kio * 1024
				}
			}
		}
	}
	return 0
}
