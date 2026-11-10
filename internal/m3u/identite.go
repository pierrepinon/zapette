package m3u

import "strings"

// Chaine est une entrée de playlist M3U. Son identité est son tvg-id quand il
// est présent, sinon son URL de flux ; c'est cette identité qui sert de clé au
// dédoublonnage des chaînes lors de la fusion des sources.
type Chaine struct {
	// TvgID est l'attribut tvg-id de la ligne #EXTINF, vide s'il est absent.
	TvgID string
	// Groupe est l'attribut group-title de la ligne #EXTINF, vide s'il est absent.
	Groupe string
	// TvgChno est l'attribut tvg-chno de la ligne #EXTINF (numéro de chaîne
	// tel que publié par le fournisseur, jamais réinterprété en entier —
	// c'est une chaîne brute), vide s'il est absent ou vide.
	TvgChno string
	// TvgLogo est l'attribut tvg-logo de la ligne #EXTINF (URL d'origine du
	// fournisseur, jamais proxifiée), vide s'il est absent ou vide.
	TvgLogo string
	// Nom est le nom affiché de la chaîne, après le premier domaine
	// (ou la virgule de séparateur) de la ligne #EXTINF.
	Nom string
	// URLFlux est l'URL du flux, telle que publiée par le fournisseur.
	// Elle n'est jamais réécrite (ADR-0001).
	URLFlux string
}

// Identite retourne l'identité de la chaîne : son tvg-id s'il est présent,
// sinon son URL de flux.
func (c Chaine) Identite() string {
	if c.TvgID != "" {
		return c.TvgID
	}
	return c.URLFlux
}

// nomAffiche extrait le nom affiché d'une ligne #EXTINF : la portion qui suit
// la première virgule de séparateur non quotée (les virgules des attributs
// quotés ne comptent pas). Une virgule suivante dans le nom affiché fait
// partie du nom.
// Exemple : #EXTINF:-1 tvg-id="x" group-title="Sports",Nom, avec virgule
// retourne « Nom, avec virgule ».
func nomAffiche(extinf string) string {
	// Le nom affiché vient après la première virgule qui n'est pas quotée ;
	// les virgules des attributs quotés sont ignorées.
	dansQuote := false
	for i, r := range extinf {
		switch r {
		case '"':
			dansQuote = !dansQuote
		case ',':
			if !dansQuote {
				return extinf[i+1:]
			}
		}
	}
	return ""
}

// attribut extrait la valeur d'un attribut quoté d'une ligne #EXTINF
// (tvg-id, group-title, ...). Rend ok=false si l'attribut est absent.
func attribut(extinf, nom string) (string, bool) {
	// Les attributs sont précédés d'un espace : « tvg-id="..." ».
	cle := " " + nom + "="
	i := strings.Index(extinf, cle)
	if i < 0 {
		return "", false
	}
	reste := extinf[i+len(cle):]
	if len(reste) > 0 && reste[0] == '"' {
		fin := strings.IndexByte(reste[1:], '"')
		if fin < 0 {
			return reste[1:], true
		}
		return reste[1 : 1+fin], true
	}
	// Valeur non quotée : jusqu'au premier espace ou à la fin de ligne.
	fin := strings.IndexAny(reste, " \t")
	if fin < 0 {
		return reste, true
	}
	return reste[:fin], true
}
