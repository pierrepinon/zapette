// Package xmltv : parseur/filtre XMLTV en streaming. Le guide est balayé
// token par token depuis un io.Reader et repassé tel quel vers la sortie ;
// seules les chaînes (<channel>) et les programmes (<programme>) retenus
// sont émis, avec toute leur descendance. La mémoire reste constante quelle
// que soit la taille du guide : jamais tout le guide en RAM (ADR-0001).
package xmltv

import (
	"encoding/xml"
	"io"
)

// CleProgramme est la clé de déduplication d'un programme : le couple
// (identifiant de chaîne, start). Deux programmes de même clé sont des
// doublons ; côté Filtrer comme côté fusion du store (Reconstruire), le
// premier arrivé est conservé. C'est la clé partagée des deux machines —
// l'ancienne copie du store a été éliminée au profit de celle-ci.
type CleProgramme struct {
	Chaine string
	Start  string
}

// Filtrer lit le guide XMLTV depuis entree et écrit dans sortie la version
// filtrée : tout le document passe token par token, sauf les chaînes et les
// programmes dont l'identifiant ne figure pas dans retenus — eux sont jetés
// avec toute leur descendance. La déclaration XML, le doctype et tout ce
// qui n'est pas une entrée de guide passe tel quel — Balayer transmet la
// racine et sa fermeture, et le réencodage du parseur normalise l'écriture.
// Le décodage est tolérant : un guide rompu au milieu n'est pas une erreur,
// la partie lisible est filtrée puis transmise, sans crash.
func Filtrer(entree io.Reader, sortie io.Writer, retenus map[string]struct{}) error {
	encodeur := xml.NewEncoder(sortie)
	vus := make(map[CleProgramme]struct{}, len(retenus))
	_, erreur := Balayer(entree,
		// Racine : émise telle quelle, sa descendance est balayée ensuite.
		func(racine xml.StartElement) error {
			return encodeur.EncodeToken(racine)
		},
		// Chaînes (retenues par l'ensemble) et programmes (dédup par
		// (chaîne, start), premier arrivé conservé).
		func(debut xml.StartElement) (bool, error) {
			switch debut.Name.Local {
			case "channel":
				return retenu(debut, retenus), nil
			case "programme":
				if !retenu(debut, retenus) {
					return false, nil
				}
				cle := CleProgramme{
					Chaine: Attribut(debut, "channel"),
					Start:  Attribut(debut, "start"),
				}
				if _, dejaVu := vus[cle]; dejaVu {
					return false, nil
				}
				vus[cle] = struct{}{}
				return true, nil
			}
			return true, nil
		},
		func(token xml.Token) error {
			return encodeur.EncodeToken(token)
		},
	)
	if erreur != nil {
		return erreur
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
	identifiant := Attribut(element, "id")
	if identifiant == "" {
		identifiant = Attribut(element, "channel")
	}
	_, retenu := retenus[identifiant]
	return retenu
}

// Attribut extrait la valeur d'un attribut d'un élément XML, vide s'il est
// absent.
func Attribut(element xml.StartElement, nom string) string {
	for _, attr := range element.Attr {
		if attr.Name.Local == nom {
			return attr.Value
		}
	}
	return ""
}
