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

	"github.com/pierrepinon/zapette/internal/store"
)

// Bornes du paramètre limit de /api/canaux (issue #11) : défaut 100,
// plancher 1, plafond 500 — une page reste une page, jamais la liste.
const (
	limitDéfaut   = 100
	limitPlancher = 1
	limitPlafond  = 500
)

// servirCanaux répond GET /api/canaux?offset=&limit=[&groupe=] (issue #11) :
// la liste des chaînes du snapshot, depuis le FICHIER canaux.json avec les
// mêmes garanties que les autres artefacts servis (ADR-0001) :
//
//   - streaming : le fichier est décodé entrée par entrée via json.Decoder,
//     la liste n'est JAMAIS chargée en RAM — seules les entrées de la
//     fenêtre demandée (≤ limit, chaque entrée étant un petit objet) forment
//     le corps de réponse, lu volontairement dans le fichier que le store a
//     publié par renommage atomique ;
//   - 503 à froid (aucun canaux.json : aucune passe réussie — même contrat
//     que /m3u et /xmltv, story 10), 500 après journalisation sinon ;
//   - filtre groupe : correspondance EXACTE du group-title (« », groupe
//     absent, sort aussi — c'est une clé comme une autre côté store).
func (s *Serveur) servirCanaux(w http.ResponseWriter, r *http.Request) {
	offset, limit, ok := paramètresCanaux(w, r)
	if !ok {
		return
	}
	groupe := r.URL.Query().Get("groupe")

	fichier, ok := ouvrirSnapshot(w, s.boutique.CheminCanaux())
	if !ok {
		return // 503 à froid ou 500 déjà répondu
	}
	defer fichier.Close()

	// Passe 1 : total des entrées CONSERVÉES par le filtre (le total couvre
	// toute la liste, pas la seule fenêtre demandée).
	inclus := inclusGroupe(groupe)
	total := 0
	if err := balayerCanaux(fichier, func(c store.Canal) error {
		if inclus(c) {
			total++
		}
		return nil
	}); err != nil {
		erreurInterneCanaux(w, "décompte de canaux.json", err)
		return
	}
	if _, err := fichier.Seek(0, io.SeekStart); err != nil {
		erreurInterneCanaux(w, "rebobinage de canaux.json", err)
		return
	}

	// Passe 2 : l'enveloppe d'abord, puis les entrées de la fenêtre,
	// sérialisées au fil du décodage.
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprintf(w, `{"total":%d,"offset":%d,"limit":%d,"canaux":[`, total, offset, limit); err != nil {
		slog.Warn("réponse /api/canaux interrompue", "erreur", err)
		return
	}
	vu := 0
	première := true
	if err := balayerCanaux(fichier, func(c store.Canal) error {
		if !inclus(c) {
			return nil
		}
		fenêtre := vu
		vu++
		if fenêtre < offset || fenêtre >= offset+limit {
			return nil // hors fenêtre : décodé sans être sérialisé
		}
		marqué, erreur := json.Marshal(c)
		if erreur != nil {
			return erreur
		}
		if !première {
			if _, erreur := w.Write([]byte(",")); erreur != nil {
				return erreur
			}
		}
		première = false
		_, erreur = w.Write(marqué)
		return erreur
	}); err != nil {
		slog.Warn("réponse /api/canaux interrompue", "erreur", err)
	}
	_, _ = w.Write([]byte("]}"))
}

// inclusGroupe : le filtre ?groupe est une correspondance exacte du
// group-title ; paramètre absent ou vide = AUCUN filtre (toutes les
// entrées, celles sans groupe comprises).
func inclusGroupe(groupe string) func(store.Canal) bool {
	if groupe == "" {
		return func(store.Canal) bool { return true }
	}
	return func(c store.Canal) bool { return c.Groupe == groupe }
}

// balayerCanaux décode le tableau canaux.json entrée par entrée et passe
// chaque entrée à surCanal. Le fichier doit être positionné sur le début du
// tableau. Mémoire constante : une entrée résidente à la fois (ADR-0001).
func balayerCanaux(fichier *os.File, surCanal func(c store.Canal) error) error {
	décodage := json.NewDecoder(bufio.NewReaderSize(fichier, 64*1024))
	ouverture, err := décodage.Token()
	if err != nil {
		return err
	}
	if délimiteur, estDélimiteur := ouverture.(json.Delim); !estDélimiteur || délimiteur != '[' {
		return fmt.Errorf("canaux.json : tableau attendu, obtenu %v", ouverture)
	}
	for décodage.More() {
		var canal store.Canal
		if err := décodage.Decode(&canal); err != nil {
			return err
		}
		if err := surCanal(canal); err != nil {
			return err
		}
	}
	return nil
}

// paramètresCanaux lit et borne offset/limit de la requête : limit défaut
// 100, plancher 1 (limit=0 → 1), plafond 500 ; offset entier ≥ 0. Une
// valeur non numérique ou un offset négatif est renvoyé en 400 avec un
// corps JSON explicite (le lecteur voit POURQUOI, cf. reponse503).
func paramètresCanaux(w http.ResponseWriter, r *http.Request) (offset, limit int, ok bool) {
	limit = limitDéfaut
	requête := r.URL.Query()

	if valeur := requête.Get("offset"); valeur != "" {
		nombre, erreur := strconv.Atoi(valeur)
		if erreur != nil || nombre < 0 {
			refusCanaux(w, "offset invalide : un entier positif ou nul est attendu")
			return 0, 0, false
		}
		offset = nombre
	}
	if valeur := requête.Get("limit"); valeur != "" {
		nombre, erreur := strconv.Atoi(valeur)
		if erreur != nil {
			refusCanaux(w, "limit invalide : un entier est attendu")
			return 0, 0, false
		}
		limit = nombre
	}
	if limit < limitPlancher {
		limit = limitPlancher
	}
	if limit > limitPlafond {
		limit = limitPlafond
	}
	return offset, limit, true
}

// refusCanaux répond 400 avec un corps JSON explicite, la contrepartie HTTP
// de reponse503 pour une requête malformée.
func refusCanaux(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]string{"erreur": message})
}

// erreurInterneCanaux journalise puis 500 : canaux.json n'est pas censé être
// illisible (publication atomique par le store), mieux vaut le dire franchement.
func erreurInterneCanaux(w http.ResponseWriter, site string, err error) {
	slog.Error(site+" impossible pour /api/canaux", "erreur", err)
	http.Error(w, "erreur interne", http.StatusInternalServerError)
}
