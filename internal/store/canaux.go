package store

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pierrepinon/zapette/internal/m3u"
	"github.com/pierrepinon/zapette/internal/xmltv"
)

// pointeurOuNil retourne nil pour une valeur vide : un attribut absent ou
// vide produit le même null (le fournisseur ne distingue pas les deux).
func pointeurOuNil(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

// Canal est une entrée de canaux.json : une chaîne du snapshot (filtrée et
// dédoublonnée), avec la provenance déduite PAR LE MERGE du store (issue #11).
// Numero est la POSITION de la chaîne dans l'ordre du merge — les sources
// de la config dans leur ordre déclaré, puis l'ordre du fichier — : une
// suite 1, 2, 3…, TOUJOURS définie, jamais le tvg-chno du fournisseur
// (numérotations hors ordre et trous du fournisseur ignorées).
// Les champs pointeur (logo, tvg_id, source_xmltv) sont null quand
// la valeur n'existe pas — nulls EXPLICITES, jamais omitempty : le contrat
// lu par le dashboard est toujours de la même forme (ADR-0001).
type Canal struct {
	Numero      string  `json:"numero"`
	Nom         string  `json:"nom"`
	Logo        *string `json:"logo"`
	Groupe      string  `json:"groupe"`
	TvgID       *string `json:"tvg_id"`
	SourceM3U   string  `json:"source_m3u"`
	SourceXMLTV *string `json:"source_xmltv"`
}

// CheminCanaux retourne le chemin de canaux.json.
func (s *Store) CheminCanaux() string {
	return filepath.Join(s.racine, "canaux.json")
}

// EcrireCanaux publie canaux.json : le tableau des chaînes retenues par le
// merge M3U (une entrée par chaîne, ordre des sources de la config puis
// ordre du fichier), numérotées SÉQUENTIELLEMENT dans cet ordre (1, 2, 3…),
// avec provenance source_m3u (source du merge premier-arrivé) et
// source_xmltv (premier guide qui publie l'id). Publication par
// temporaire voisin + rename atomique, via le même publier que les snapshots
// (ADR-0001). Appelée par Reconstruire : meta.json reste écrit EN DERNIER
// par la passe de fetch.
func (s *Store) EcrireCanaux(copiesM3U, copiesXMLTV []string) error {
	if err := publier(s.CheminCanaux(), func(w io.Writer) error {
		return ecrireCanaux(copiesM3U, copiesXMLTV, w)
	}); err != nil {
		return fmt.Errorf("écriture de canaux.json : %w", err)
	}
	return nil
}

// ecrireCanaux produit le tableau JSON des chaînes, UNE ENTRÉE À LA FOIS :
// la liste n'est jamais chargée en RAM (ADR-0001), seuls résident les
// ensembles de dédup (mêmes cartes que la fusion des snapshots) et la carte
// id → source du guide. Deux passes streaming sur disque, dans l'ordre de
// la config (position connue des sources) :
//
//  1. copies XMLTV : le MÊME dédup first-wins que fusionnerXMLTV décide de
//     la retention ; chaque <channel id> publié par une copie enregistre
//     id → nom de la source POUR LA PREMIÈRE fois (source_xmltv) ;
//  2. copies M3U : le MÊME dédup first-wins que fusionnerM3U retient
//     l'entrée gagnante de chaque identité, qui est numérotée par sa
//     position dans la suite et encodée au fil de l'eau entre crochets.
func ecrireCanaux(copiesM3U, copiesXMLTV []string, sortie io.Writer) error {
	// Passe 1 : provenance XMLTV, déduite pendant la fusion du guide (le
	// même balayage qui décide de la retention). Les copies filtraient déjà
	// leurs guides par l'ensemble des identités retenues côté M3U : tout id
	// publié ici EST un id de chaîne du snapshot.
	publiées := map[string]struct{}{}
	origine := map[string]string{}
	for _, copie := range copiesXMLTV {
		f, err := os.Open(copie)
		if os.IsNotExist(err) {
			continue // copie absente : jamais réussie, ignorée comme le merge
		}
		if err != nil {
			return err
		}
		_, erreur := xmltv.Balayer(f,
			// Balayer n'accepte pas de callback nil : no-op sur les sites
			// sans décision (racine observée, jetons traversés).
			func(xml.StartElement) error { return nil },
			func(debut xml.StartElement) (bool, error) {
				switch debut.Name.Local {
				case "channel":
					// Mêmes décisions que la fusion : retenuChaine marque
					// ET dédoublonne. Un id déjà retenu par une copie
					// antérieure conserve sa PREMIÈRE source (first-wins).
					if retenuChaine(debut, publiées) {
						if id := xmltv.Attribut(debut, "id"); id != "" {
							if _, vu := origine[id]; !vu {
								origine[id] = nomCopie(copie)
							}
						}
					}
					return true, nil
				default:
					return true, nil
				}
			},
			func(xml.Token) error { return nil })
		f.Close()
		if erreur != nil {
			return erreur
		}
	}

	// Passe 2 : les chaînes M3U retenues, encodées au fil de l'eau.
	if _, err := sortie.Write([]byte("[")); err != nil {
		return err
	}
	vues := map[string]struct{}{}
	// Suite séquentielle : la position de chaque chaîne retenue, dans
	// l'ordre déclaré de la config puis l'ordre du fichier — le tvg-chno
	// du fournisseur ne numérote RIEN (numérotation hors ordre, absente
	// ou trouée : jamais réinterprétée).
	numero := 0
	première := true
	for _, copie := range copiesM3U {
		f, err := os.Open(copie)
		if os.IsNotExist(err) {
			continue // copie absente : jamais réussie, ignorée comme le merge
		}
		if err != nil {
			return err
		}
		nom := nomCopie(copie)
		erreur := m3u.Balayer(f, nil, func(c m3u.Chaine, _ []string) error {
			if _, vue := vues[c.Identite()]; vue {
				return nil // le merge a déjà retenu cette identité : dédoublon
			}
			vues[c.Identite()] = struct{}{}
			numero++
			// tvg_id n'est publié que si l'ATTRIBUT existe : quand
			// l'identité est l'URL de flux, il reste null (contrat #11) —
			// la source XMLTV n'est alors jamais recherchée.
			var sourceXMLTV *string
			if c.TvgID != "" {
				if source, vu := origine[c.TvgID]; vu {
					sourceXMLTV = &source
				}
			}
			entrée := Canal{
				Numero:      strconv.Itoa(numero),
				Nom:         c.Nom,
				Logo:        pointeurOuNil(c.TvgLogo),
				Groupe:      c.Groupe,
				TvgID:       pointeurOuNil(c.TvgID),
				SourceM3U:   nom,
				SourceXMLTV: sourceXMLTV,
			}
			marqué, erreur := json.Marshal(entrée)
			if erreur != nil {
				return erreur
			}
			if !première {
				if _, erreur := sortie.Write([]byte(",")); erreur != nil {
					return erreur
				}
			}
			première = false
			_, erreur = sortie.Write(marqué)
			return erreur
		})
		f.Close()
		if erreur != nil {
			return erreur
		}
	}
	_, err := sortie.Write([]byte("]"))
	return err
}

// nomCopie retourne le nom de la source d'une copie : le nom de fichier
// sans l'extension d'artefact (sources/<nom>.m3u|.xmltv, nom posé par
// CheminCopie). L'extension d'artefact est toujours la DERNIÈRE du nom de
// fichier — filepath.Ext("prov.m3u.m3u") = ".m3u" — donc le nom retourne
// tel quel, fût-il terminé par « .m3u » ou « .xmltv » ; les séparateurs de
// chemin (/, \) n'y figurent jamais : la config les refuse en amont
// (config.valider, le name ne peut pas sortir de sources/).
func nomCopie(copie string) string {
	return strings.TrimSuffix(filepath.Base(copie), filepath.Ext(copie))
}
