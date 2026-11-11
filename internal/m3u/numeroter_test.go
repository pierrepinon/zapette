package m3u

import "testing"

// TestNumeroter : l'attribut tvg-chno d'une ligne #EXTINF brute est le
// SEUL octet réécrit (ADR-0002) — valeur présente remplacée, absente
// insérée avant la virgule du nom, fins de ligne d'origine préservées.
func TestNumeroter(t *testing.T) {
	cas := []struct {
		nom     string
		extinf  string
		numero  string
		attendu string
	}{
		{
			"valeur quotée remplacée",
			`#EXTINF:-1 tvg-id="arte.fr" tvg-chno="4711" group-title="Sports",Arte`,
			"7",
			`#EXTINF:-1 tvg-id="arte.fr" tvg-chno="7" group-title="Sports",Arte`,
		},
		{
			"absente : insérée avant le nom",
			`#EXTINF:-1 tvg-id="bfm.fr" group-title="News",BFM`,
			"27",
			`#EXTINF:-1 tvg-id="bfm.fr" group-title="News" tvg-chno="27",BFM`,
		},
		{
			"aucun attribut : insérée après la durée",
			`#EXTINF:-1,Nue`,
			"30",
			`#EXTINF:-1 tvg-chno="30",Nue`,
		},
		{
			"valeur non quotée remplacée jusqu'à l'espace",
			`#EXTINF:-1 tvg-chno=42 group-title="g",Nom`,
			"11",
			`#EXTINF:-1 tvg-chno=11 group-title="g",Nom`,
		},
		{
			"valeur non quotée en bout d'attributs (virgule du nom)",
			`#EXTINF:-1 tvg-id="x.fr" tvg-chno=9,Nom`,
			"5",
			`#EXTINF:-1 tvg-id="x.fr" tvg-chno=5,Nom`,
		},
		{
			"virgule dans un attribut quoté : l'insertion vise la BONNE virgule",
			`#EXTINF:-1 group-title="A,B",Nom, avec virgule`,
			"3",
			`#EXTINF:-1 group-title="A,B" tvg-chno="3",Nom, avec virgule`,
		},
		{
			"fins de ligne \\r\\n préservées",
			"#EXTINF:-1 tvg-chno=\"1\",Arte\r\n",
			"7",
			"#EXTINF:-1 tvg-chno=\"7\",Arte\r\n",
		},
		{
			"valeur quotée jamais fermée (dégénérée) : remplacée, le reste est perdu",
			`#EXTINF:-1 tvg-chno="4711,Arte`,
			"7",
			`#EXTINF:-1 tvg-chno="7"`,
		},
		{
			"ligne sans virgule (dégénérée) : attribut en fin de ligne",
			`#EXTINF:-1 tvg-id="x.fr"`,
			"2",
			`#EXTINF:-1 tvg-id="x.fr" tvg-chno="2"`,
		},
	}
	for _, c := range cas {
		if obtenu := Numeroter(c.extinf, c.numero); obtenu != c.attendu {
			t.Errorf("%s :\n obtenu %s\nattendu %s", c.nom, obtenu, c.attendu)
		}
	}
}

// TestNumeroterRefuseLigneHorsEXTINF : une ligne qui n'est pas une entrée
// #EXTINF (en-tête, directive, URL) sort INCHANGÉE — jamais de corruption
// silencieuse du snapshot si un appelant se trompe de ligne.
func TestNumeroterRefuseLigneHorsEXTINF(t *testing.T) {
	brutes := []string{
		"#EXTM3U url-tvg=\"https://exemple.tld/guide.xml\"\n",
		"#EXTVLCOPT:http-user-agent=Zapette\n",
		"https://flux.exemple.tld/arte.m3u8\n",
	}
	for _, brute := range brutes {
		if obtenu := Numeroter(brute, "42"); obtenu != brute {
			t.Errorf("ligne hors #EXTINF modifiée : %q → %q", brute, obtenu)
		}
	}
}
