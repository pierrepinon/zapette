// Package tnt : le plan de numérotation officiel des chaînes TNT
// françaises, tel que fixé par décision de l'Arcom (numérotation en vigueur
// depuis le 6 juin 2025, Novo19 en 19 depuis le 1er septembre 2025). Les
// box ADSL/fibre/satellite reprennent cette suite hors TNT, ce qui en fait
// la référence « naturelle » de numérotation d'une ligne IPTV.
//
// Le plan est STATIQUE : il ne change que sur décision de l'Arcom (la
// précédente remontait à 2011), il vit donc ici en table embarquée — aucun
// fetch, aucune API amont (l'Arcom ne publie PAS de service : communiqués
// seulement). Aucune numérotation n'est inventée : pour un tvg-id absent
// du plan, l'appelant replie sur sa propre suite séquentielle (dans
// l'ordre du merge), démarrant à PremierHorsPlan.
//
// La correspondance se fait PAR TVG-ID, pas par nom : les noms varient
// d'un fournisseur à l'autre (suffixes HD/FHD, casse, mentions de groupe).
// La comparaison est INSENSIBLE à la casse — le fournisseur publie tantôt
// « tf1.fr », tantôt « TF1.fr » — mais reste EXACTE sur le reste : un
// fournisseur peut étiqueter une autre chaîne d'un tvg-id connu (pollution
// observée : RMC LIFE FHD publiée sous « TFX.fr »), la décision premier-
// arrivé reste à l'appelant.
//
// T18 et Novo19 (canaux 18 et 19) ne figurent pas dans la table : le
// fournisseur leur publie un tvg-id absent, la clé ne peut pas exister.
package tnt

import (
	"strconv"
	"strings"
)

// plan associe chaque tvg-id TNT connu (MINUSCULES, clé canonique de
// comparaison) à son numéro officiel Arcom, en string : le contrat
// canaux.json est textuel de bout en bout, jamais réinterprété en entier.
var plan = map[string]string{
	"tf1.fr":                   "1",
	"france2.fr":               "2",
	"france3.fr":               "3",
	"france4.fr":               "4",
	"france5.fr":               "5",
	"m6.fr":                    "6",
	"arte.fr":                  "7",
	"lachaineparlementaire.fr": "8",
	"w9.fr":                    "9",
	"tmc.fr":                   "10",
	"tfx.fr":                   "11",
	"gulli.fr":                 "12",
	"bfmtv.fr":                 "13",
	"cnews.fr":                 "14",
	"lci.fr":                   "15",
	"franceinfo.fr":            "16",
	"cstar.fr":                 "17",
	// 18 T18, 19 Novo19 : tvg-id absent — voir le commentaire du package.
	"tf1seriesfilms.fr": "20",
	"lequipe21.fr":      "21",
	"6ter.fr":           "22",
	"rmcstory.fr":       "23",
	"rmcdecouverte.fr":  "24",
	"rmclife.fr":        "25",
	"parispremiere.fr":  "26",
}

// premierHorsPlan est le premier numéro APRÈS le plus grand du plan : le
// point de départ de la suite séquentielle de repli.
var premierHorsPlan = func() int {
	plusGrand := 0
	for _, numero := range plan {
		if n, err := strconv.Atoi(numero); err == nil && n > plusGrand {
			plusGrand = n
		}
	}
	return plusGrand + 1
}()

// Numero retourne le numéro officiel Arcom du tvg-id, s'il figure au plan
// (comparaison insensible à la casse : « TF1.fr » et « tf1.fr » sont la
// même chaîne). Sinon ok=false : l'appelant replie sur sa suite, il n'y a
// PAS de numéro par défaut ici.
func Numero(tvgID string) (numero string, ok bool) {
	numero, ok = plan[strings.ToLower(tvgID)]
	return
}

// PremierHorsPlan retourne le premier numéro libre après le plan Arcom : le
// point de départ d'une suite séquentielle de repli qui ne collide jamais
// avec un numéro officiel.
func PremierHorsPlan() int {
	return premierHorsPlan
}
