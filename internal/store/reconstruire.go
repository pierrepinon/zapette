package store

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"

	"github.com/pierrepinon/zapette/internal/m3u"
	"github.com/pierrepinon/zapette/internal/xmltv"
)

// Reconstruire réassemble le snapshot fusionné par fusion streaming des
// copies de source, dans l'ordre donné (l'ordre des sources de la config) :
// publication par temporaire + rename atomique, m3u et xmltv indépendants.
// Les copies absentes (jamais récupérées) sont ignorées ; sans aucune copie
// d'un artefact, aucun snapshot n'est publié — celui du disque reste celui
// de la passe précédente, jamais un fichier partiel. La mémoire reste
// constante quelle que soit la taille des copies (ADR-0001) : seuls les
// ensembles de dédup résident en RAM.
func (s *Store) Reconstruire(copiesM3U, copiesXMLTV []string) error {
	if len(copiesM3U) > 0 {
		if err := publier(s.CheminSnapshot("m3u"), func(w io.Writer) error {
			return fusionnerM3U(copiesM3U, w)
		}); err != nil {
			return fmt.Errorf("publication du snapshot.m3u : %w", err)
		}
	}
	if len(copiesXMLTV) > 0 {
		if err := publier(s.CheminSnapshot("xmltv"), func(w io.Writer) error {
			return fusionnerXMLTV(copiesXMLTV, w)
		}); err != nil {
			return fmt.Errorf("publication du snapshot.xmltv : %w", err)
		}
	}
	// canaux.json : une entrée par chaîne retenue, avec la provenance
	// déduite PAR LE MERGE (issue #11). Comme les snapshots ci-dessus, il
	// n'est publié que si des copies M3U existent ; les copies XMLTV
	// absentes sont simplement ignorées. Écrit ICI, donc AVANT meta.json :
	// meta reste le DERNIER artefact de la passe (étape 4 de fetch.Passe).
	if len(copiesM3U) > 0 {
		if err := s.EcrireCanaux(copiesM3U, copiesXMLTV); err != nil {
			return err
		}
	}
	return nil
}

// fusionnerM3U concatène les copies comme une seule playlist : en-tête du
// premier arrivé (redondants ignorés par Balayer), dédup first-wins par
// identité de chaîne (première source de la config gagne, puis premier
// ordre de fichier), émission DES LIGNES BRUTES D'ORIGINE, fins de ligne
// comprises — à LA SEULE EXCEPTION du tvg-chno de chaque #EXTINF,
// réécrit au numéro calculé (ADR-0002 : c'est lui que le lecteur —
// Jellyfin en tête — numérote ; plan Arcom du tvg-id ou suite de repli).
// Numérotation et canaux.json suivent LA MÊME règle dans LE MÊME ordre :
// les numéros publiés des deux artefacts coïncident sans carte partagée.
func fusionnerM3U(copies []string, sortie io.Writer) error {
	vues := map[string]struct{}{}
	numerotation := NouvelleNumerotation()
	enTeteEmise := false

	for _, copie := range copies {
		f, err := os.Open(copie)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		err = m3u.Balayer(f,
			// Seul le premier en-tête rencontré est émis.
			func(brute string) error {
				if enTeteEmise {
					return nil
				}
				enTeteEmise = true
				_, erreur := sortie.Write([]byte(brute))
				return erreur
			},
			// Première occurrence d'une identité : émise octet par octet.
			func(c m3u.Chaine, lignes []string) error {
				if _, vue := vues[c.Identite()]; vue {
					return nil
				}
				vues[c.Identite()] = struct{}{}
				// Ligne #EXTINF réécrite au numéro retenu, reste de
				// l'entrée (URL, directives rattachées) octet par octet.
				lignes[0] = m3u.Numeroter(lignes[0], numerotation.Suivant(c.TvgID))
				for _, ligne := range lignes {
					if _, erreur := sortie.Write([]byte(ligne)); erreur != nil {
						return erreur
					}
				}
				return nil
			})
		f.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// fusionnerXMLTV réassemble les guides sous une seule racine <tv> : déclaration
// XML, DOCTYPE et racine du premier arrivé, puis les enfants de chaque copie,
// chaînes et programmes dédupliqués first-wins — La forme du document est
// préservée : le DOCTYPE de la première copie traverse la fusion (critère de
// la spec « déclaration XML et doctype conservés », consumé par /xmltv). Le
// balayage token par token repose sur la primitive unique xmltv.Balayer (la
// machine commune à xmltv.Filtrer) ; le store n'y apporte que ses décisions
// de dédup. Une copie rompue au milieu n'est pas une erreur : la partie
// lisible est fusionnée et on passe à la copie suivante (même tolérance que
// xmltv.Filtrer).
func fusionnerXMLTV(copies []string, sortie io.Writer) error {
	var (
		encodeur   = xml.NewEncoder(sortie)
		racine     xml.Name // nom de la racine fusionnée : celle du premier arrivé
		ouverte    bool     // la racine fusionnée a été émise (première copie)
		declXML    bool     // déclaration XML déjà émise (une seule par document)
		doctype    bool     // DOCTYPE déjà émis (une seule par document)
		ouverts    int      // éléments reçus de la copie courante et émis, ouverts
		vuesChaine = map[string]struct{}{}
		vusProg    = map[xmltv.CleProgramme]struct{}{}
	)

	for _, copie := range copies {
		f, err := os.Open(copie)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		_, erreur := xmltv.Balayer(f,
			// Racine de la copie : celle du premier arrivé ouvre la racine
			// fusionnée ; celle des copies suivantes (même nom) ne sont
			// pas réémises — leurs enfants se fondent sous la racine unique.
			// Le doctype/déclaration du prologue de chaque copie a déjà
			// traversé surToken.
			func(racineCopie xml.StartElement) error {
				if ouverte {
					return nil
				}
				ouverte = true
				racine = racineCopie.Name
				return encodeur.EncodeToken(racineCopie)
			},
			// Dédup first-wins : première copie et première occurrence
			// gagnent — la dédup vit dans cette callback, la primitive
			// xmltv.Balayer ne décide jamais d'elle-même.
			func(debut xml.StartElement) (bool, error) {
				switch debut.Name.Local {
				case "channel":
					return retenuChaine(debut, vuesChaine), nil
				case "programme":
					cle := xmltv.CleProgramme{
						Chaine: xmltv.Attribut(debut, "channel"),
						Start:  xmltv.Attribut(debut, "start"),
					}
					if _, vu := vusProg[cle]; vu {
						return false, nil
					}
					vusProg[cle] = struct{}{}
					return true, nil
				}
				return true, nil
			},
			func(token xml.Token) error {
				switch t := token.(type) {
				case xml.ProcInst:
					// La déclaration XML du premier arrivé seulement : les
					// suivantes (copies, imbriquées, cibles non-xml) sont
					// jamais réémises.
					if t.Target == "xml" && !declXML && !ouverte {
						declXML = true
						return encodeur.EncodeToken(t)
					}
					return nil
				case xml.Directive:
					// Le DOCTYPE du premier arrivé seulement — c'est lui qui
					// garde le snapshot servi par /xmltv canonique.
					if !doctype && !ouverte {
						doctype = true
						return encodeur.EncodeToken(t)
					}
					return nil
				case xml.CharData, xml.Comment:
					if !ouverte {
						// Texte et commentaires du prologue : jamais réémis.
						return nil
					}
					return encodeur.EncodeToken(t)
				case xml.StartElement:
					ouverts++
					return encodeur.EncodeToken(t)
				case xml.EndElement:
					if ouverts == 0 {
						// Fermeture de la racine de CETTE copie (première ou
						// suivante) : jamais réémise — la racine fusionnée ne
						// se ferme qu'en toute fin, après la dernière copie.
						return nil
					}
					ouverts--
					return encodeur.EncodeToken(t)
				}
				return nil
			})
		f.Close()
		if erreur != nil {
			return erreur
		}
	}
	if !ouverte {
		// Aucune copie lisible : rien publié (aucune racine à fermer).
		return nil
	}
	if err := encodeur.EncodeToken(xml.EndElement{Name: racine}); err != nil {
		return err
	}
	return encodeur.Flush()
}

// retenuChaine dit si l'entrée chaîne vue est A RETENIR : son identifiant
// (l'attribut id) doit être inconnu — dédup first-wins ; une chaîne sans
// identifiant ne peut pas être dédoublonnée et passe toujours.
func retenuChaine(debut xml.StartElement, vues map[string]struct{}) bool {
	id := xmltv.Attribut(debut, "id")
	if id == "" {
		return true // sans identifiant, impossible de dédupliquer
	}
	if _, vu := vues[id]; vu {
		return false
	}
	vues[id] = struct{}{}
	return true
}
