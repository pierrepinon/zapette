// Package xmltv : parseur/filtre XMLTV en streaming. Le guide est lu token
// par token depuis un io.Reader et repassé tel quel vers la sortie ; seules
// les chaînes (<channel>) et les programmes (<programme>) retenus sont
// émis, avec toute leur descendance. La mémoire reste constante quelle que
// soit la taille du guide : jamais tout le guide en RAM (ADR-0001).
package xmltv

import (
	"encoding/xml"
	"io"
)

// cleProgramme est la clé de déduplication d'un programme : le couple
// (identifiant de chaîne, start). Deux programmes de même clé sont des
// doublons : le premier arrivé est conservé.
type cleProgramme struct {
	identifiant string
	start       string
}

// Filtrer lit le guide XMLTV depuis entree et écrit dans sortie la version
// filtrée : tout le document passe token par token, sauf les chaînes et les
// programmes dont l'identifiant ne figure pas dans retenus — eux sont jetés
// avec toute leur descendance. La déclaration XML, le doctype et tout ce
// qui n'est pas une entrée de guide passe tel quel. Le décodage est
// tolérant : un guide rompu au milieu n'est pas une erreur, la partie
// lisible est filtrée puis transmise, sans crash.
func Filtrer(entree io.Reader, sortie io.Writer, retenus map[string]struct{}) error {
	var (
		decodeur = xml.NewDecoder(entree)
		encodeur = xml.NewEncoder(sortie)
		jeter    int // profondeur restante au-dessous d'une entrée rejetée
		vus      = make(map[cleProgramme]struct{}, len(retenus))
	)
	for {
		token, err := decodeur.Token()
		if err == io.EOF {
			return encodeur.Flush()
		}
		if err != nil {
			// Guide rompu ou malformé : la partie lisible a déjà été
			// filtrée, on la garde et on s'arrête proprement.
			if _, malForme := err.(*xml.SyntaxError); malForme {
				break
			}
			return err
		}

		if jeter > 0 {
			// Au-dessous d'une entrée rejetée : compter pour retrouver la
			// fin de l'entrée rejetée, sans rien émettre.
			switch token.(type) {
			case xml.StartElement:
				jeter++
			case xml.EndElement:
				jeter--
			}
			continue
		}

		if debut, ouvert := token.(xml.StartElement); ouvert {
			switch debut.Name.Local {
			case "channel", "programme":
				if !retenu(debut, retenus) {
					// L'entrée entière est jetée, sa racine comprise.
					jeter = 1
					continue
				}
				if debut.Name.Local == "programme" {
					// Dédup : un programme déjà vu (même chaîne, même
					// start) est jeté, le premier arrivé ayant été
					// conservé.
					cle := cleProgramme{
						identifiant: attribut(debut, "channel"),
						start:       attribut(debut, "start"),
					}
					if _, dejaVu := vus[cle]; dejaVu {
						jeter = 1
						continue
					}
					vus[cle] = struct{}{}
				}
			}
		}
		if err := encodeur.EncodeToken(token); err != nil {
			return err
		}
	}
	return encodeur.Flush()
}

// retenu dit si l'entrée est retenue : son identifiant de chaîne (l'attribut
// id d'un canal, l'attribut channel d'un programme) doit figurer dans
// retenus. Un ensemble vide ne retient rien : côté M3U, vide porte sur la
// LISTE DE GROUPES (entrée du filtre) ; côté XMLTV, l'entrée est déjà
// l'ENSEMBLE DES CHAINES RETENUES ; vide = « aucune chaîne n'a survécu » et
// garder tout violerait la user story 3. La symétrie utile se fait au
// niveau du ticket #5 (allowed_groups vide → toutes les identités retenues →
// ensemble non vide transmis).
func retenu(element xml.StartElement, retenus map[string]struct{}) bool {
	identifiant := attribut(element, "id")
	if identifiant == "" {
		identifiant = attribut(element, "channel")
	}
	_, retenu := retenus[identifiant]
	return retenu
}

// attribut extrait la valeur d'un attribut quoté d'un élément XML, vide s'il
// est absent.
func attribut(element xml.StartElement, nom string) string {
	for _, attr := range element.Attr {
		if attr.Name.Local == nom {
			return attr.Value
		}
	}
	return ""
}
