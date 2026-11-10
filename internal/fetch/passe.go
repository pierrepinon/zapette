package fetch

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/m3u"
	"github.com/pierrepinon/zapette/internal/store"
	"github.com/pierrepinon/zapette/internal/xmltv"
)

// Passe réalise une passe de fetch complète, strictement séquentielle :
//  1. M3U de chaque source : fetch réseau (timeout de config, 1+essais
//     reprises, backoff fixe), filtre par liste de groupes autorisés,
//     copie sur disque (copie figée en cas d'échec, sans toucher aux
//     autres sources).
//  2. XMLTV de chaque source : fetch, filtre par l'ENSEMBLE des identités
//     retenues par le merge M3U des copies filtrées sur disque.
//     allowed_groups vide ⇒ tout est retenu côté M3U ⇒ l'ensemble porte
//     toutes les identités disponibles : c'est une conséquence du
//     filtrage déjà appliqué, câblée ici, sans cas spécial dans le
//     store. Aucun ensemble vide n'est jamais transmis (un ensemble vide
//     ne retient RIEN, par design) : sans chaîne retenue, le fetch XMLTV
//     est sauté et la passe le trace.
//  3. Reconstruction : fusion streaming des copies, publication par
//     renommages atomiques.
//  4. meta.json, écrit EN DERNIER.
//
// Les états de la passe sont amorcés par l'état persisté (store.Charger) :
// last_ok d'une source qui échoue conserve sa dernière réussite.
func (o *Ordonnanceur) Passe(ctx context.Context, cfg config.Config) error {
	fetch := o.fenetre(cfg.FetchTimeout)

	meta, err := o.boutique.Charger()
	if err != nil {
		return fmt.Errorf("lecture de l'état : %w", err)
	}
	if meta.Sources == nil {
		meta.Sources = make(map[string]store.EtatSource, len(cfg.Sources))
	}
	suivant := o.horloge().Add(cfg.RefreshInterval)

	// 1) M3U : fetch, filtre, copie — source après source.
	for _, src := range cfg.Sources {
		es := meta.Sources[src.Name]
		es.M3U = amorcer(es.M3U, suivant)
		corps, erreur := o.recuperer(ctx, fetch, src.M3UURL, cfg.FetchRetries)
		if erreur != nil {
			o.echecCopieFigee(meta.Sources, src.Name, es, es.M3U, "fetch m3u", erreur)
			continue
		}
		erreur = o.boutique.EcrireCopie(src.Name, "m3u", func(w io.Writer) error {
			return m3u.Filtrer(corps, w, cfg.AllowedGroups)
		})
		corps.Close()
		if erreur != nil {
			o.echecCopieFigee(meta.Sources, src.Name, es, es.M3U, "copie m3u", erreur)
			continue
		}
		maintenant := o.horloge()
		es.M3U.DernierOK = &maintenant
		meta.Sources[src.Name] = es
	}

	// Ensemble des identités retenues par le merge M3U : balayage des
	// copies filtrées sur disque dans l'ordre des sources. La dédup ne
	// change rien à l'ensemble (union des identités, ordre indifférent).
	retenues := map[string]struct{}{}
	for _, src := range cfg.Sources {
		f, erreur := os.Open(o.boutique.CheminCopie(src.Name, "m3u"))
		if erreur != nil {
			continue // copie absente : jamais réussie, rien à en retenir
		}
		erreur = m3u.Balayer(f, nil, func(c m3u.Chaine, _ []string) error {
			retenues[c.Identite()] = struct{}{}
			return nil
		})
		f.Close()
		if erreur != nil {
			return fmt.Errorf("balayage de la copie %s : %w", src.Name, erreur)
		}
	}

	// 2) XMLTV : fetch, filtre par l'ensemble des identités retenues.
	for _, src := range cfg.Sources {
		if src.XMLTVURL == "" {
			continue // artefact hors du plan de la source
		}
		es := meta.Sources[src.Name]
		es.XMLTV = amorcer(es.XMLTV, suivant)
		if len(retenues) == 0 {
			es.XMLTV.DerniereErreur = "aucune chaîne m3u retenue, guide ignoré"
			meta.Sources[src.Name] = es
			continue
		}
		corps, erreur := o.recuperer(ctx, fetch, src.XMLTVURL, cfg.FetchRetries)
		if erreur != nil {
			o.echecCopieFigee(meta.Sources, src.Name, es, es.XMLTV, "fetch xmltv", erreur)
			continue
		}
		erreur = o.boutique.EcrireCopie(src.Name, "xmltv", func(w io.Writer) error {
			return xmltv.Filtrer(corps, w, retenues)
		})
		corps.Close()
		if erreur != nil {
			o.echecCopieFigee(meta.Sources, src.Name, es, es.XMLTV, "copie xmltv", erreur)
			continue
		}
		maintenant := o.horloge()
		es.XMLTV.DernierOK = &maintenant
		meta.Sources[src.Name] = es
	}

	// 3) Reconstruction : fusion streaming des copies dans l'ordre des
	// sources de la config. Reconstruire ignore les copies absentes et ne
	// publie rien sans aucune copie — le disque reste celui d'avant.
	copiesM3U := cheminsCopies(o.boutique, cfg, "m3u")
	copiesXMLTV := cheminsCopies(o.boutique, cfg, "xmltv")
	if erreur := o.boutique.Reconstruire(copiesM3U, copiesXMLTV); erreur != nil {
		return erreur
	}

	// 4) meta.json EN DERNIER.
	return o.boutique.EcrireMeta(meta)
}

// echecCopieFigee enregistre l'échec d'un artefact d'une source (les quatre
// sites de la passe : fetch/copie × m3u/xmltv) : last_error posé dans l'état
// (persisté par EcrireMeta en toute fin de passe), journalisation et
// signalement au ring /api/status ; les autres sources continuent et la
// dernière bonne copie de celle-ci reste servie — « copie figée ».
func (o *Ordonnanceur) echecCopieFigee(meta map[string]store.EtatSource, source string, etatSource store.EtatSource, artefact *store.EtatArtefact, site string, err error) {
	artefact.DerniereErreur = err.Error()
	meta[source] = etatSource
	slog.Warn(site+" en échec, copie figée", "source", source, "erreur", err)
	o.signaler(fmt.Sprintf("source %s : %s en échec, copie figée : %v", source, site, err))
}

// amorcer amorce l'état d'un artefact pour la passe : last_error repart à
// vide, next_at est posé, et last_ok EST CONSERVÉ de l'état précédent — une
// source qui a déjà réussi garde sa dernière réussite même si cette passe
// échoue (ensuite, seule une nouvelle réussite la réavance).
func amorcer(ancien *store.EtatArtefact, suivant time.Time) *store.EtatArtefact {
	etat := &store.EtatArtefact{SuivantA: &suivant}
	if ancien != nil {
		etat.DernierOK = ancien.DernierOK
	}
	return etat
}

func cheminsCopies(boutique *store.Store, cfg config.Config, artefact string) []string {
	chemins := make([]string, 0, len(cfg.Sources))
	for _, src := range cfg.Sources {
		chemins = append(chemins, boutique.CheminCopie(src.Name, artefact))
	}
	return chemins
}
