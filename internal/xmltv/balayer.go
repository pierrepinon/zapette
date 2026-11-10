package xmltv

import (
	"encoding/xml"
	"io"
)

// Balayer est la primitive complète de balayage d'un document XMLTV : le
// miroir de m3u.Balayer côté guide. Filtrer (filtrage à la réception) et la
// fusion du store (Reconstruire) se reposent TOUS DEUX dessus — une seule
// machine à tokens au lieu de deux :
//
//   - le document est lu token par token depuis entree, JUSQU'À LA FIN DE SA
//     RACINE (la racine est le premier StartElement) ; le nom de la racine
//     est retourné (vide si le document s'est rompu avant) ;
//   - l'ouverture de la racine passe par surRacine (observation) : elle est
//     TOUJOURS suivie du balayage de sa descendance, le visiteur ne décide
//     pas d'abandonner une copie ici ;
//   - chaque StartElement de PREMIER NIVEAU sous la racine, nommé
//     <channel> ou <programme>, passe d'abord par surEntree : garder=true et
//     l'entrée entière passe token par token par surToken — toute sa
//     descendance, jamais redécidée à l'intérieur ; garder=false et elle est
//     jetée avec toute sa descendance — le balayeur compte la profondeur et
//     n'appelle plus aucun callback jusqu'à la fin de l'entrée ;
//   - tout le reste (déclaration XML xml.ProcInst, doctype xml.Directive,
//     commentaires, texte, éléments imbriqués inconnus, et la FIN de la
//     racine) passe par surToken tel quel, dans l'ordre du document ;
//   - le décodage est tolérant : un document rompu au milieu
//     (xml.SyntaxError) n'est pas une erreur — la partie lisible a été
//     balayée, le balayage s'arrête proprement (nom de racine retourné) ;
//   - une erreur renvoyée par un callback arrête le balayage et est
//     propagée telle quelle.
//
// La mémoire reste constante quelle que soit la taille du document : jamais
// le guide tout entier en RAM (ADR-0001).
func Balayer(
	entree io.Reader,
	surRacine func(racine xml.StartElement) error,
	surEntree func(debut xml.StartElement) (garder bool, err error),
	surToken func(token xml.Token) error,
) (xml.Name, error) {
	var (
		decodeur = xml.NewDecoder(entree)
		racine   xml.Name // nom de la racine du document (premier StartElement)
		jeter    int      // profondeur restante au-dessous d'une entrée rejetée
		ouverts  int      // éléments ouverts sous la racine (entrées comprises)
	)
	for {
		token, erreur := decodeur.Token()
		if erreur == io.EOF {
			return racine, nil
		}
		if erreur != nil {
			// Document rompu au milieu : la partie lisible a déjà été
			// balayée, on s'arrête proprement — poursuite tolérée par les
			// deux clients (filtre, fusion).
			if _, malForme := erreur.(*xml.SyntaxError); malForme {
				return racine, nil
			}
			return racine, erreur
		}

		if jeter > 0 {
			// Au-dessous d'une entrée rejetée : compter pour retrouver la fin
			// de cette entrée, sans rien transmettre.
			switch token.(type) {
			case xml.StartElement:
				jeter++
			case xml.EndElement:
				jeter--
			}
			continue
		}

		if debut, ouvert := token.(xml.StartElement); ouvert {
			if racine.Local == "" {
				// La racine du document : observée, jamais décision d'entrée.
				racine = debut.Name
				if erreur := surRacine(debut); erreur != nil {
					return racine, erreur
				}
				continue
			}
			if ouverts == 0 {
				switch debut.Name.Local {
				case "channel", "programme":
					garder, erreur := surEntree(debut)
					if erreur != nil {
						return racine, erreur
					}
					if !garder {
						// L'entrée entière est jetée, sa racine comprise.
						jeter = 1
						continue
					}
				}
			}
			ouverts++
			if erreur := surToken(debut); erreur != nil {
				return racine, erreur
			}
			continue
		}

		if fin, fermee := token.(xml.EndElement); fermee {
			if ouverts > 0 {
				ouverts--
				if erreur := surToken(fin); erreur != nil {
					return racine, erreur
				}
				continue
			}
			// Fin de la racine du document (ou fermeture pathologique avant
			// toute racine) : transmise puis le balayage s'arrête là — fin
			// du document.
			if erreur := surToken(fin); erreur != nil {
				return racine, erreur
			}
			return racine, nil
		}

		if erreur := surToken(token); erreur != nil {
			return racine, erreur
		}
	}
}
