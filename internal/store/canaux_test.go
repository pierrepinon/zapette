package store

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// Le scénario de dédoublonnage de TestReconstruireM3UFusionPremierGagne est
// repris (issue #11) : la source arte gagne sur la chaîne doublon arte.fr,
// la source miroir apporte cnews.
const (
	canauxArteM3U = `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" tvg-chno="4" tvg-logo="https://img.tld/arte.png" group-title="Sports",Arte
https://flux.exemple.tld/arte.m3u8
#EXTINF:-1 tvg-id="bfm.fr" group-title="News",BFM
https://flux.exemple.tld/bfm.m3u8
#EXTINF:-1 tvg-id="tfx.fr" group-title="News",TFX
https://flux.exemple.tld/tfx.m3u8
`
	canauxMiroirM3U = `#EXTM3U url-tvg="https://exemple.tld/guide.xml"
#EXTINF:-1 tvg-id="arte.fr" tvg-chno="9" group-title="Sports",Arte miroir
https://miroir.tld/arte.m3u8
#EXTINF:-1 tvg-id="TFX.fr" group-title="News",RMC LIFE
https://miroir.tld/rmclife.m3u8
#EXTINF:-1 tvg-chno="7" group-title="News",La chaîne sans guide
https://flux.exemple.tld/noguide.m3u8
`
	canauxArteXMLTV = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="arte.fr"><display-name>Arte</display-name></channel>
  <programme channel="arte.fr" start="20251110080000 +0100"><title>Journal</title></programme>
</tv>
`
	canauxMiroirXMLTV = `<?xml version="1.0" encoding="UTF-8"?>
<tv>
  <channel id="arte.fr"><display-name>Arte miroir</display-name></channel>
  <channel id="cfaitque.fr"><display-name>C fait que</display-name></channel>
</tv>
`
)

// lireCanaux lit canaux.json tel que publié.
func lireCanaux(t *testing.T, chemin string) []Canal {
	t.Helper()
	brut, err := os.ReadFile(chemin)
	if err != nil {
		t.Fatalf("lecture de canaux.json : %v", err)
	}
	var canaux []Canal
	if err := json.Unmarshal(brut, &canaux); err != nil {
		t.Fatalf("analyse de canaux.json : %v", err)
	}
	return canaux
}

// TestReconstruireEcritCanauxProvenancePremierGagne : le merge déduit la
// provenance en lisant les copies source par source dans l'ordre de la
// config — première source qui retient la chaîne (dédup first-wins) pour
// source_m3u, première copie de guide qui publie l'id pour source_xmltv.
// Le numero suit le plan officiel TNT (Arcom) : arte.fr → 7, tfx.fr → 11
// (le TFX.fr du miroir, pollution qui désigne RMC LIFE, reçoit un numéro
// de repli : premier-arrivé sur le numéro officiel) ; bfm.fr et la chaîne
// sans guide, hors plan, tombent dans la suite qui démarre à 27.
func TestReconstruireEcritCanauxProvenancePremierGagne(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	for nom, contenu := range map[string]string{
		"arte":   canauxArteM3U,
		"miroir": canauxMiroirM3U,
	} {
		if err := s.EcrireCopie(nom, "m3u", ecrire(contenu)); err != nil {
			t.Fatalf("EcrireCopie %s : %v", nom, err)
		}
	}
	if err := s.EcrireCopie("arte", "xmltv", ecrire(canauxArteXMLTV)); err != nil {
		t.Fatalf("EcrireCopie arte xmltv : %v", err)
	}
	if err := s.EcrireCopie("miroir", "xmltv", ecrire(canauxMiroirXMLTV)); err != nil {
		t.Fatalf("EcrireCopie miroir xmltv : %v", err)
	}

	copiesM3U := []string{s.CheminCopie("arte", "m3u"), s.CheminCopie("miroir", "m3u")}
	copiesXMLTV := []string{s.CheminCopie("arte", "xmltv"), s.CheminCopie("miroir", "xmltv")}
	if err := s.Reconstruire(copiesM3U, copiesXMLTV); err != nil {
		t.Fatalf("Reconstruire : %v", err)
	}

	canaux := lireCanaux(t, s.CheminCanaux())
	attendu := []Canal{
		// arte.fr : conservée par la source arte (première) — son logo
		// vient d'ELLE (le miroir perd le dédup, avec son tvg-chno="9"
		// IGNORÉ) ; arte.fr est publié d'abord par le guide arte.
		// Numero : numéro OFFICIEL Arcom du tvg-id.
		{Numero: "7", Nom: "Arte", Logo: ptr("https://img.tld/arte.png"), Groupe: "Sports",
			TvgID: ptr("arte.fr"), SourceM3U: "arte", SourceXMLTV: ptr("arte")},
		// bfm.fr : tvg-id hors plan → premier numéro de la suite de
		// repli (27, juste après le canal 26 de la TNT).
		{Numero: "27", Nom: "BFM", Logo: nil, Groupe: "News",
			TvgID: ptr("bfm.fr"), SourceM3U: "arte", SourceXMLTV: nil},
		// tfx.fr : numéro officiel Arcom.
		{Numero: "11", Nom: "TFX", Logo: nil, Groupe: "News",
			TvgID: ptr("tfx.fr"), SourceM3U: "arte", SourceXMLTV: nil},
		// TFX.fr (RMC LIFE) : CASSE différente, même tvg-id que tfx.fr —
		// le numéro officiel 11 est déjà pris (premier-arrivé) : repli
		// sur la suite (28).
		{Numero: "28", Nom: "RMC LIFE", Logo: nil, Groupe: "News",
			TvgID: ptr("TFX.fr"), SourceM3U: "miroir", SourceXMLTV: nil},
		// Identité = URL de flux : sans tvg-id, tvg_id est null ET
		// source_xmltv est null (un id à null n'est jamais recherché) —
		// tvg-chno="7" est IGNORÉ : la suite continue (29).
		{Numero: "29", Nom: "La chaîne sans guide", Logo: nil, Groupe: "News",
			TvgID: nil, SourceM3U: "miroir", SourceXMLTV: nil},
	}
	if len(canaux) != len(attendu) {
		t.Fatalf("canaux.json : %d entrées, attendu %d : %+v", len(canaux), len(attendu), canaux)
	}
	// Comparaison PAR LE CONTRAT JSON (les champs pointeur de Canal rendent
	// == inutilisable ; c'est la sérialisation sur disque qui fait foi).
	for i := range attendu {
		obtenuJSON, err := json.Marshal(canaux[i])
		if err != nil {
			t.Fatalf("sérialisation de l'entrée %d : %v", i, err)
		}
		attenduJSON, err := json.Marshal(attendu[i])
		if err != nil {
			t.Fatalf("sérialisation de l'attendu %d : %v", i, err)
		}
		if string(obtenuJSON) != string(attenduJSON) {
			t.Errorf("entrée %d :\n obtenu %s\nattendu %s", i, obtenuJSON, attenduJSON)
		}
	}

	// Publication atomique : aucun temporaire résiduel.
	entrées, err := os.ReadDir(s.racine)
	if err != nil {
		t.Fatalf("lecture du répertoire de données : %v", err)
	}
	for _, e := range entrées {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Errorf("temporaire résiduel : %s", e.Name())
		}
	}
}

// TestReconstruireCanauxChampsVidesNuls : groupe absent → "", attributs
// absents (logo, tvg-id) → null — y compris tvg_id quand l'identité est
// l'URL de flux (contrat #11 : null si l'identité n'est pas un tvg-id).
// Le numero, LUI, est toujours défini : sans tvg-id au plan Arcom, la
// tête de la suite de repli (27).
func TestReconstruireCanauxChampsVidesNuls(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	nue := `#EXTM3U
#EXTINF:-1,Nue sans rien
https://flux.tld/nue.m3u8
`
	if err := s.EcrireCopie("nue", "m3u", ecrire(nue)); err != nil {
		t.Fatalf("EcrireCopie : %v", err)
	}
	if err := s.Reconstruire([]string{s.CheminCopie("nue", "m3u")}, nil); err != nil {
		t.Fatalf("Reconstruire : %v", err)
	}

	canaux := lireCanaux(t, s.CheminCanaux())
	if len(canaux) != 1 {
		t.Fatalf("1 entrée attendue, obtenu %d", len(canaux))
	}
	attendu := Canal{Numero: "27", Nom: "Nue sans rien", Groupe: "",
		TvgID: nil, SourceM3U: "nue"}
	obtenuJSON, err := json.Marshal(canaux[0])
	if err != nil {
		t.Fatalf("sérialisation de l'entrée : %v", err)
	}
	attenduJSON, err := json.Marshal(attendu)
	if err != nil {
		t.Fatalf("sérialisation de l'attendu : %v", err)
	}
	if string(obtenuJSON) != string(attenduJSON) {
		t.Errorf("obtenu %s, attendu %s", obtenuJSON, attenduJSON)
	}
}

// TestReconstruireCanauxSansCopieM3U : sans copie M3U, rien n'est publié —
// le disque (canaux.json compris) reste celui de la passe précédente.
func TestReconstruireCanauxSansCopieM3U(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	if err := s.Reconstruire(nil, nil); err != nil {
		t.Fatalf("Reconstruire : %v", err)
	}
	if _, err := os.Stat(s.CheminCanaux()); !os.IsNotExist(err) {
		t.Errorf("canaux.json ne doit pas exister sans copie M3U (err %v)", err)
	}
}

// ptr : helper minuscule partagé par les tests de ce fichier.
func ptr[T any](v T) *T { return &v }
