package store

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"

	"github.com/pierrepinon/zapette/internal/m3u"
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
	return nil
}

// fusionnerM3U concatène les copies comme une seule playlist : en-tête du
// premier arrivé (redondants ignorés par Balayer), dédup first-wins par
// identité de chaîne (première source de la config gagne, puis premier
// ordre de fichier), émission octet par octet des lignes brutes d'origine,
// fins de ligne comprises.
func fusionnerM3U(copies []string, sortie io.Writer) error {
	vues := map[string]struct{}{}
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

// cleProgramme est la clé de dédup d'un programme de guide : le couple
// (chaîne, start) — même convention que xmltv.Filtrer au sein d'une copie.
type cleProgramme struct {
	chaine string
	start  string
}

// fusionnerXMLTV réassemble les guides sous une seule racine <tv> : la
// déclaration XML et la racine du premier arrivé, puis les enfants de
// chaque copie, chaînes et programmes dédupliqués first-wins. Une copie
// rompue au milieu n'est pas une erreur : la partie lisible est fusionnée
// et on passe à la copie suivante (même tolérance que xmltv.Filtrer).
func fusionnerXMLTV(copies []string, sortie io.Writer) error {
	var (
		encodeur    = xml.NewEncoder(sortie)
		racine      xml.Name
		declaration bool
		vuesChaine  = map[string]struct{}{}
		vusProg     = map[cleProgramme]struct{}{}
	)

	for _, copie := range copies {
		f, err := os.Open(copie)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		decodeur := xml.NewDecoder(f)

		// Jusqu'à la racine <tv> de cette copie : émettre la déclaration
		// XML du premier arrivé, puis solder la racine à la fin.
		var racineCopie xml.StartElement
		for racineCopie.Name.Local == "" {
			token, erreur := decodeur.Token()
			if erreur != nil {
				break // copie rompue avant sa racine : passer à la suivante
			}
			switch t := token.(type) {
			case xml.ProcInst:
				if t.Target == "xml" && !declaration {
					declaration = true
					erreur = encodeur.EncodeToken(t)
				}
			case xml.StartElement:
				racineCopie = t
			}
			if erreur != nil {
				break
			}
		}
		if racineCopie.Name.Local == "" {
			f.Close()
			continue
		}
		if racine.Local == "" {
			racine = racineCopie.Name
			if err := encodeur.EncodeToken(racineCopie); err != nil {
				f.Close()
				return err
			}
		}

		// Enfants de cette copie : dédup des entrées, réémission token par
		// token (le réencodage du parseur est la normalisation attendue, le
		// contenu sémantique est préservé).
		var (
			profondeur int   // sous-éléments courants entre racine et fin d'entrée
			jeter      int   // profondeur restante au-dessous d'une entrée éjectée
			fin        error // première erreur non tolérée
		)
		for {
			token, erreur := decodeur.Token()
			if erreur != nil {
				break
			}
			switch t := token.(type) {
			case xml.EndElement:
				if jeter > 0 {
					jeter--
					continue
				}
				if profondeur == 0 {
					// Fin de la racine de cette copie : copie suivante.
					goto copieSuivante
				}
				if profondeur > 0 {
					if err := encodeur.EncodeToken(t); err != nil {
						fin = err
						goto copieSuivante
					}
					profondeur--
				}
			case xml.StartElement:
				if jeter > 0 {
					jeter++
					continue
				}
				if profondeur == 0 && !retenuXMLTV(t, vuesChaine, vusProg) {
					jeter = 1
					continue
				}
				if err := encodeur.EncodeToken(t); err != nil {
					fin = err
					goto copieSuivante
				}
				profondeur++
			case xml.ProcInst:
				// Déclaration imbriquée d'une copie : jamais réémise.
				continue
			case xml.CharData:
				if jeter == 0 {
					if err := encodeur.EncodeToken(t); err != nil {
						fin = err
						goto copieSuivante
					}
				}
			case xml.Comment:
				if jeter == 0 {
					if err := encodeur.EncodeToken(t); err != nil {
						fin = err
						goto copieSuivante
					}
				}
			}
		}
	copieSuivante:
		f.Close()
		if fin != nil {
			return fin
		}
	}
	if racine.Local == "" {
		return nil
	}
	if err := encodeur.EncodeToken(xml.EndElement{Name: racine}); err != nil {
		return err
	}
	return encodeur.Flush()
}

// retenuXMLTV dit si une entrée de niveau enfant (chaîne, programme) doit
// être émise : dédup first-wins par identité de chaîne, puis par clé
// (chaîne, start) — première copie et première occurrence gagnent. Une
// profondeur > 0 dans les sous-éléments ne re-décide jamais : l'entrée
// entière passe.
func retenuXMLTV(element xml.StartElement, vuesChaine map[string]struct{}, vusProg map[cleProgramme]struct{}) bool {
	switch element.Name.Local {
	case "channel":
		id := attributXMLTV(element, "id")
		if id == "" {
			return true // sans identifiant, impossible de dédupliquer
		}
		if _, vu := vuesChaine[id]; vu {
			return false
		}
		vuesChaine[id] = struct{}{}
		return true
	case "programme":
		cle := cleProgramme{chaine: attributXMLTV(element, "channel"), start: attributXMLTV(element, "start")}
		if _, vu := vusProg[cle]; vu {
			return false
		}
		vusProg[cle] = struct{}{}
		return true
	}
	return true
}

func attributXMLTV(element xml.StartElement, nom string) string {
	for _, attr := range element.Attr {
		if attr.Name.Local == nom {
			return attr.Value
		}
	}
	return ""
}
