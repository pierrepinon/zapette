package m3u

import (
	"bufio"
	"io"
	"strings"
)

// Balayer expose la primitive complète de balayage : la première ligne
// #EXTM3U brute passe par surEnTete, chaque entrée complète passe par
// surChaine avec toutes ses lignes brutes, depuis #EXTINF jusqu'à l'URL de
// flux incluse, fins de ligne d'origine comprises et à l'octet près. Ce sont
// les lignes d'origine du fournisseur, jamais réécrites. Les lignes
// orphelines (directives ou URL hors d'une entrée) et les en-têtes redondants
// sont ignorés. Aucune ligne n'est retenue en mémoire au-delà de l'entrée
// courante : la playlist complète n'est jamais chargée (ADR-0001).
func Balayer(
	entree io.Reader,
	surEnTete func(brute string) error,
	surChaine func(c Chaine, lignes []string) error,
) error {
	lu := bufio.NewReaderSize(entree, 64*1024)

	enteteTransmise := false
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
			case normale == "":
				// Ligne vide : ignorée, elle ne clôt pas l'entrée courante.
			case strings.HasPrefix(normale, "#EXTINF"):
				tvgID, _ := attribut(normale, "tvg-id")
				groupe, _ := attribut(normale, "group-title")
				tvgChno, _ := attribut(normale, "tvg-chno")
				tvgLogo, _ := attribut(normale, "tvg-logo")
				courante = Chaine{
					TvgID:   tvgID,
					Groupe:  groupe,
					TvgChno: tvgChno,
					TvgLogo: tvgLogo,
					Nom:     nomAffiche(normale),
				}
				lignesCourantes = append(lignesCourantes[:0], brute)
			case strings.HasPrefix(normale, "#EXTM3U"):
				// Seul le premier en-tête est transmis : les redondants et
				// ceux qui couperaient une entrée en cours sont ignorés.
				if !enteteTransmise && len(lignesCourantes) == 0 {
					enteteTransmise = true
					if surEnTete != nil {
						if err := surEnTete(brute); err != nil {
							return err
						}
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
