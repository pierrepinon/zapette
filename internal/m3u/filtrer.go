// Package m3u : parseur/filtre M3U en streaming. La playlist est lue ligne
// ligne depuis un io.Reader ; seules les entrées dont le group-title figure
// dans la liste de groupes autorisés sont émises, octet par octet, sans
// réécriture. La playlist complète n'est jamais chargée en mémoire (ADR-0001 :
// snapshot sur disque, servi en streaming) : la mémoire reste constante quelle
// que soit la taille de la playlist.
package m3u

import (
	"bufio"
	"io"
	"strings"
)

// Filtrer lit la playlist depuis entree et écrit dans sortie la version
// filtrée : l'en-tête #EXTM3U, puis pour chaque entrée retenue toutes ses
// lignes depuis #EXTINF jusqu'à l'URL de flux incluse, à l'octet près (les
// fins de ligne d'origine sont conservées).
// groupesAutorises contient les valeurs exactes et case-sensitive de
// group-title à conserver ; une liste vide conserve tout. Une entrée sans
// group-title n'est jamais retenue hors liste vide. Les lignes orphelines
// (directives ou URL hors d'une entrée) sont ignorées.
func Filtrer(entree io.Reader, sortie io.Writer, groupesAutorises []string) error {
	lu := bufio.NewReaderSize(entree, 64*1024)
	crit := bufio.NewWriterSize(sortie, 64*1024)

	autorises := make(map[string]struct{}, len(groupesAutorises))
	for _, groupe := range groupesAutorises {
		autorises[groupe] = struct{}{}
	}
	tout := len(autorises) == 0

	enteteEcrite := false
	var lignesCourantes []string // lignes brutes (fins de ligne comprises) depuis le dernier #EXTINF
	courante := Chaine{}

	ecrire := func(brute string) error {
		_, err := crit.WriteString(brute)
		return err
	}

	// verser émet l'entrée courante si elle est retenue, suivie de son URL de flux.
	verser := func(urlBrute string) error {
		garder := tout
		if !garder && len(courante.Groupe) > 0 {
			_, garder = autorises[courante.Groupe]
		}
		if !garder {
			return nil
		}
		if !enteteEcrite {
			enteteEcrite = true
			if err := ecrire("#EXTM3U\n"); err != nil {
				return err
			}
		}
		for _, ligne := range lignesCourantes {
			if err := ecrire(ligne); err != nil {
				return err
			}
		}
		return ecrire(urlBrute)
	}

	for {
		brute, err := lu.ReadString('\n')
		if err != nil && err != io.EOF {
			return err
		}
		if len(brute) > 0 {
			normale := strings.TrimRight(brute, "\r\n")
			switch {
			case strings.HasPrefix(normale, "#EXTINF"):
				tvgID, _ := attribut(normale, "tvg-id")
				groupe, _ := attribut(normale, "group-title")
				courante = Chaine{
					TvgID:  tvgID,
					Groupe: groupe,
					Nom:    nomAffiche(normale),
				}
				lignesCourantes = append(lignesCourantes[:0], brute)
			case !enteteEcrite && strings.HasPrefix(normale, "#EXTM3U"):
				// Premier en-tête : émis tel quel (les attributs comme url-tvg
				// sont préservés, jamais réécrits).
				enteteEcrite = true
				if err := ecrire(brute); err != nil {
					return err
				}
			case strings.HasPrefix(normale, "#"):
				if len(lignesCourantes) > 0 {
					// Directive de flux rattachée à l'entrée (#EXTVLCOPT, ...).
					lignesCourantes = append(lignesCourantes, brute)
				}
				// Sinon directive hors entrée : ignorée.
			default:
				if len(lignesCourantes) > 0 {
					// Première ligne non directive après #EXTINF : l'URL du
					// flux clôt l'entrée. Elle est conservée telle quelle et
					// sert l'identité de la chaîne en l'absence de tvg-id.
					courante.URLFlux = normale
					if err := verser(brute); err != nil {
						return err
					}
					lignesCourantes = lignesCourantes[:0]
				}
				// Sinon ligne orpheline : ignorée.
			}
		}
		if err == io.EOF {
			return crit.Flush()
		}
	}
}
