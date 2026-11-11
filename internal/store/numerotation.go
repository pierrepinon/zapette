package store

import (
	"strconv"

	"github.com/pierrepinon/zapette/internal/tnt"
)

// Numerotation est la règle qui numérote les chaînes au fil de leur
// RETENTION, dans l'ordre du merge (sources de la config, puis ordre du
// fichier) : numéro officiel TNT — plan Arcom, package tnt — quand le
// tvg-id y figure, PREMIER-ARRIVÉ si le fournisseur étiquette deux chaînes
// du même tvg-id connu ; sinon la suite séquentielle de repli démarrant
// juste après le plan. Le tvg-chno du fournisseur ne numérote RIEN.
//
// La règle est DÉTERMINISTE relativement à l'ordre de balayage : deux
// consommateurs qui parcourent les mêmes copies, dans le même ordre,
// n'appellent Suivant que sur les chaînes retenues par le MÊME dédup
// first-wins et obtiennent des numéros IDENTIQUES — c'est ainsi que
// snapshot.m3u (tvg-chno réécrits) et canaux.json restent d'accord sans
// carte partagée (ADR-0001 : rien de plus résident que l'état du dédoublon).
type Numerotation struct {
	pris     map[string]struct{}
	horsPlan int
}

// NouvelleNumerotation repart d'un état vierge : premiers numéros du plan,
// suite de repli avant le premier appel.
func NouvelleNumerotation() *Numerotation {
	return &Numerotation{
		pris:     map[string]struct{}{},
		horsPlan: tnt.PremierHorsPlan() - 1,
	}
}

// Suivant retourne le numéro de la chaîne retenue, d'après son tvg-id
// (éventuellement vide : la suite de repli, jamais un numéro du plan).
func (n *Numerotation) Suivant(tvgID string) string {
	if officiel, ok := tnt.Numero(tvgID); ok {
		if _, pris := n.pris[officiel]; !pris {
			n.pris[officiel] = struct{}{}
			return officiel
		}
	}
	n.horsPlan++
	return strconv.Itoa(n.horsPlan)
}
