package m3u

import (
	"bufio"
	"io"
	"strings"
)

// balayer lit la playlist ligne à ligne et convoque :
//   - surEnTete avec la ligne #EXTM3U brute (fins de ligne comprises), une
//     seule fois, quand elle est rencontrée ;
//   - surChaine avec la chaîne analysée et toutes ses lignes brutes, depuis
//     #EXTINF jusqu'à l'URL de flux incluse (fins de ligne comprises), dès
//     qu'elle est complète.
//
// Les lignes orphelines (directives ou URL hors d'une entrée) et les en-têtes
// redondants sont ignorés. Aucune ligne n'est retenue en mémoire au-delà de
// l'entrée courante : la playlist complète n'est jamais chargée (ADR-0001).
func balayer(
	entree io.Reader,
	surEnTete func(brute string) error,
	surChaine func(c Chaine, lignes []string) error,
) error {
	lu := bufio.NewReaderSize(entree, 64*1024)

	var lignesCourantes []string
	courante := Chaine{}

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
			case strings.HasPrefix(normale, "#EXTM3U"):
				if surEnTete != nil {
					if err := surEnTete(brute); err != nil {
						return err
					}
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
					// sert d'identité de la chaîne en l'absence de tvg-id.
					courante.URLFlux = normale
					lignesCourantes = append(lignesCourantes, brute)
					if err := surChaine(courante, lignesCourantes); err != nil {
						return err
					}
					lignesCourantes = lignesCourantes[:0]
				}
				// Sinon ligne orpheline : ignorée.
			}
		}
		if err == io.EOF {
			return nil
		}
	}
}
