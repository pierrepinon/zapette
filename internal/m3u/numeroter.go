package m3u

import "strings"

// cleChno est le préfixe d'occurrence de l'attribut tvg-chno d'une ligne
// #EXTINF : les attributs sont précédés d'un espace (« tvg-chno="…" »),
// même convention que attribut.
const cleChno = " tvg-chno="

// Numeroter réécrit la valeur de l'attribut tvg-chno d'une ligne #EXTINF
// BRUTE : le numéro calculé du store (plan Arcom ou suite de repli) y
// remplace celui du fournisseur ; attribut absent, il est INSÉRÉ à la fin
// de la section des attributs, juste avant la virgule du nom affiché.
// C'est la SEULE réécriture admise du M3U servi (ADR-0002) : tout le reste
// de la ligne — autres attributs, nom affiché, fins de ligne \n ou \r\n
// d'origine — traverse à l'octet près. Numéroter une ligne qui n'en est
// pas une est une erreur de programmation : la ligne sort inchangée
// (jamais de corruption silencieuse du snapshot).
func Numeroter(extinfBrute, numero string) string {
	if !strings.HasPrefix(strings.TrimRight(extinfBrute, "\r\n"), "#EXTINF") {
		return extinfBrute
	}
	corps := strings.TrimRight(extinfBrute, "\r\n")
	fins := extinfBrute[len(corps):]
	if i := strings.Index(corps, cleChno); i >= 0 {
		return remplacerValeurChno(corps, i, numero) + fins
	}
	return insererChno(corps, numero) + fins
}

// remplacerValeurChno substitue la valeur de l'attribut tvg-chno repéré à
// l'octet i : la valeur quotée est remplacée entre ses guillemets ; non
// quotée, jusqu'au premier espace ou à la virgule du nom affiché. Une
// valeur quotée jamais fermée (ligne dégénérée) est remplacée telle
// quelle : le reste de la ligne, syntaxiquement irrécupérable, est PERDU
// — jamais de #EXTINF faussement refermée dans le snapshot servi.
func remplacerValeurChno(extinf string, i int, numero string) string {
	jusquacle := extinf[:i+len(cleChno)]
	reste := extinf[i+len(cleChno):]
	if strings.HasPrefix(reste, `"`) {
		if fin := strings.IndexByte(reste[1:], '"'); fin >= 0 {
			return jusquacle + `"` + numero + `"` + reste[fin+2:]
		}
		return jusquacle + `"` + numero + `"`
	}
	if fin := strings.IndexAny(reste, " \t,"); fin >= 0 {
		return jusquacle + numero + reste[fin:]
	}
	return jusquacle + numero
}

// insererChno ajoute l'attribut à la fin de la section des attributs :
// juste avant la première virgule non quotée — celle qui introduit le nom
// affiché —, ou en fin de ligne si la ligne dégénérée n'en a aucune (les
// virgules des attributs quotés ne comptent pas, même règle que nomAffiche).
func insererChno(extinf, numero string) string {
	dansQuote := false
	for i, r := range extinf {
		switch r {
		case '"':
			dansQuote = !dansQuote
		case ',':
			if !dansQuote {
				return extinf[:i] + ` tvg-chno="` + numero + `"` + extinf[i:]
			}
		}
	}
	return extinf + ` tvg-chno="` + numero + `"`
}
