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
)

// Filtrer lit la playlist depuis entree et écrit dans sortie la version
// filtrée : l'en-tête #EXTM3U, puis pour chaque entrée retenue toutes ses
// lignes depuis #EXTINF jusqu'à l'URL de flux incluse, à l'octet près (les
// fins de ligne d'origine sont conservées). Si la source ne comporte pas de
// ligne #EXTM3U, un en-tête nu est émis.
// groupesAutorises contient les valeurs exactes et case-sensitive de
// group-title à conserver ; une liste vide conserve tout. Une entrée sans
// group-title n'est jamais retenue hors liste vide. Les lignes orphelines
// (directives ou URL hors d'une entrée) sont ignorées.
func Filtrer(entree io.Reader, sortie io.Writer, groupesAutorises []string) error {
	crit := bufio.NewWriterSize(sortie, 64*1024)

	autorises := make(map[string]struct{}, len(groupesAutorises))
	for _, groupe := range groupesAutorises {
		autorises[groupe] = struct{}{}
	}
	tout := len(autorises) == 0

	enteteEcrite := false

	err := balayer(entree,
		func(enTete string) error {
			// Premier en-tête : émis tel quel, les attributs comme url-tvg
			// sont préservés, jamais réécrits.
			enteteEcrite = true
			_, err := crit.WriteString(enTete)
			return err
		},
		func(c Chaine, lignes []string) error {
			garder := tout
			if !garder && len(c.Groupe) > 0 {
				_, garder = autorises[c.Groupe]
			}
			if !garder {
				return nil
			}
			if !enteteEcrite {
				enteteEcrite = true
				if _, err := crit.WriteString("#EXTM3U\n"); err != nil {
					return err
				}
			}
			for _, ligne := range lignes {
				if _, err := crit.WriteString(ligne); err != nil {
					return err
				}
			}
			return nil
		},
	)
	if err != nil {
		return err
	}
	return crit.Flush()
}
