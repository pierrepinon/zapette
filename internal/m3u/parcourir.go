package m3u

import "io"

// Parcourir lit la playlist depuis entree et convoque visiter pour chaque
// entrée, dans l'ordre du fichier, avec la chaîne analysée : attributs
// (tvg-id, group-title, nom affiché) et URL de flux. C'est le parcours utilisé
// par la fusion/dédup des sources : chaque chaîne doit être identifiée par
// Chaine.Identite (tvg-id sinon URL de flux) sans garder la playlist en
// mémoire (ADR-0001).
//
// API PUBLIQUE ASSUMÉE (revue finale #11) : un wrapper fin sur Balayer (sans
// les lignes brutes) — assumé comme le miroir de lecture simple de Balayer,
// pas un middle man à réinliner : un seul client en production (compteurs
// /api/status) et des tests dédiés (parcourir_test.go, cas_limites_test.go,
// memoire_test.go) l'épinglent comme surface propre, symétrique de la
// primitive complète à laquelle un consommateur peut préférer Balayer.
func Parcourir(entree io.Reader, visiter func(c Chaine) error) error {
	return Balayer(entree, nil, func(c Chaine, _ []string) error {
		return visiter(c)
	})
}
