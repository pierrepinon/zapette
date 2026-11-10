// Package acceptance : harnais d'acceptation MESURÉ du ticket 09 (issue #10).
//
// Il matérialise les critères d'acceptation de la spec sous forme de tests
// reproductibles à la demande (jamais de payload 216 000 entrées en CI
// courte — les tests sautent sous -short) :
//
//   - TestAcceptationRSS216k            : RSS (VmRSS, /proc/self/status)
//     mesurée pendant le fetch ET sous requêtes simultanées /m3u + /xmltv
//     (goroutines clientes), avec diff binaire prouvant que les lignes
//     #EXTINF/URL retenues sont servies octet pour octet.
//   - TestAcceptationKillPuisRedemarrage : kill -9 du VRAI binaire pendant
//     un fetch, puis redémarrage → service opérationnel depuis le disque.
//
// La playlist de 216 000 entrées est générée À LA VOLÉE par le générateur
// de ce package : SYNTHÉTIQUE et DÉTERMINISTE — aucune donnée réelle du
// fournisseur, aucun secret. Elle est écrite en streaming sur le disque et
// servie par des serveurs faux fournisseurs, comme le ferait l'amont.
//
// Point d'entrée committé : scripts/acceptance.sh (go test ./internal/acceptance -v).
// Le harnais orchestre les briques RÉELLES du service (internal/store,
// internal/fetch, internal/server) au niveau du package, pas dans main :
// le binaire de production n'importe jamais ce code, et les mêmes tests ne
// dépendent d'aucun conteneur ni port réservé.
package acceptance

import (
	"bufio"
	"fmt"
	"io"
)

// entreesReference : le volume de référence de la spec (issue #1, story 19) :
// 216 000 entrées de playlist réelle.
const entreesReference = 216_000

// entreesReferenceK9 : volume réduit pour le scénario kill -9 (le critère y
// est comportemental, pas volumétrique : le RSS est mesuré par le test 216k).
const entreesKill9 = 20_000

// groupesReference : les groupes synthétiques distribués cycliquement dans
// la playlist, Sports en premier (le harnais filtre sur [Sports]).
var groupesReference = [4]string{"Sports", "Documentaires", "Films", "Jeunesse"}

// genererM3U écrit une playlist M3U SYNTHÉTIQUE de n entrées dans sortie, en
// streaming (jamais la playlist entière en mémoire). Déterministe : la même n
// donne les mêmes octets. Réalisme : attributs quotés (tvg-id, tvg-name,
// tvg-logo, group-title), noms affichés avec accents et virgules, directives
// #EXTVLCOPT rattachées à une entrée, un treizième des entrées SANS tvg-id
// (identité = URL de flux, cf. GLOSSARY.md), groupes de la table ci-dessus —
// exactement 1/4 d'entrées Sports. Rien n'est réécrit ensuite par le service.
func genererM3U(sortie io.Writer, n int) error {
	b := bufio.NewWriterSize(sortie, 1<<20)
	if _, err := fmt.Fprint(b, "#EXTM3U url-tvg=\"https://guide.synthetique.invalid/guide.xml\" x-tvg-url=\"https://guide.synthetique.invalid/\"\n"); err != nil {
		return err
	}
	for i := range n {
		groupe := groupesReference[i%len(groupesReference)]
		identite := fmt.Sprintf("ch%06d.%s.exemple.invalid", i, groupe)
		if i%13 == 0 {
			// Entrée sans tvg-id : son identité est son URL de flux.
			identite = ""
		}
		nom := fmt.Sprintf("Chaîne %06d — %s, montage", i, groupe)
		if _, err := fmt.Fprintf(b,
			"#EXTINF:-1 tvg-id=%q tvg-name=%q tvg-logo=\"https://logos.exemple.invalid/%06d.png\" group-title=%q,%s\n",
			identite, "Chaîne "+fmt.Sprintf("%06d", i), i, groupe, nom); err != nil {
			return err
		}
		if i%11 == 0 {
			// Directive de flux rattachée à l'entrée (jamais réécrite).
			if _, err := fmt.Fprint(b, "#EXTVLCOPT:http-user-agent=zapette-acceptance/1.0\n"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(b, "https://flux.exemple.invalid/live/ch%06d.m3u8?token=synthese&qualite=auto\n", i); err != nil {
			return err
		}
	}
	return b.Flush()
}

// genererXMLTV écrit un guide XMLTV SYNTHÉTIQUE couvrant les n chaînes de la
// playlist générée (mêmes tvg-id, une entrée <channel> et programmes
// programmesParChaine par chaîne), en streaming. Déterministe.
func genererXMLTV(sortie io.Writer, n, programmesParChaine int) error {
	b := bufio.NewWriterSize(sortie, 1<<20)
	if _, err := fmt.Fprint(b, "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<tv generator-info-name=\"zapette-acceptance\" source-data-url=\"https://guide.synthetique.invalid/\">\n"); err != nil {
		return err
	}
	// Les <channel> d'abord, puis les <programme> : même ordre qu'un guide réel.
	for i := range n {
		groupe := groupesReference[i%len(groupesReference)]
		if i%13 == 0 {
			continue // cohérent avec le M3U : pas de tvg-id, pas de chaîne guide
		}
		identite := fmt.Sprintf("ch%06d.%s.exemple.invalid", i, groupe)
		if _, err := fmt.Fprintf(b,
			"<channel id=%q><display-name lang=\"fr\">Chaîne %06d</display-name><icon src=\"https://logos.exemple.invalid/%06d.png\"/></channel>\n",
			identite, i, i); err != nil {
			return err
		}
	}
	for i := range n {
		groupe := groupesReference[i%len(groupesReference)]
		if i%13 == 0 {
			continue
		}
		identite := fmt.Sprintf("ch%06d.%s.exemple.invalid", i, groupe)
		for j := range programmesParChaine {
			debut := fmt.Sprintf("20250601%02d%02d00 +0000", 12+j, i%60)
			fin := fmt.Sprintf("20250601%02d%02d30 +0000", 12+j, i%60)
			if _, err := fmt.Fprintf(b,
				"<programme start=%q stop=%q channel=%q><title lang=\"fr\">Programme %06d-%d, diffuse</title><desc lang=\"fr\">Description synthétique, avec virgule.</desc></programme>\n",
				debut, fin, identite, i, j); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprint(b, "</tv>\n")
	if err != nil {
		return err
	}
	return b.Flush()
}
