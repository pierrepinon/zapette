package store

import (
	"encoding/xml"
	"io"
	"os"
	"strings"
	"testing"
)

// TestReconstruireXmltvConserveDeclarationEtDoctype épingle le critère de la
// spec « déclaration XML et doctype conservés » sur le snapshot servi par
// /xmltv : un guide fournisseur avec doctype (forme canonique des guides
// XMLTV, comme le fixture du filtre internal/xmltv) doit resservir avec sa
// déclaration et son doctype après fusion.
func TestReconstruireXmltvConserveDeclarationEtDoctype(t *testing.T) {
	s, err := Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("Ouvrir : %v", err)
	}
	if err := s.EcrireCopie("arte", "xmltv", ecrire(guidageAvecDoctype)); err != nil {
		t.Fatalf("EcrireCopie arte : %v", err)
	}

	if err := s.Reconstruire(nil, []string{s.CheminCopie("arte", "xmltv")}); err != nil {
		t.Fatalf("Reconstruire : %v", err)
	}

	lu, err := os.ReadFile(s.CheminSnapshot("xmltv"))
	if err != nil {
		t.Fatalf("lecture du snapshot : %v", err)
	}
	servi := string(lu)
	if !strings.HasPrefix(servi, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Errorf("la déclaration XML doit être conservée en tête du snapshot :\n%s", servi)
	}
	if !strings.Contains(servi, `<!DOCTYPE tv SYSTEM "xmltv.dtd">`) {
		t.Errorf("le doctype doit être conservé dans le snapshot :\n%s", servi)
	}
	if !strings.Contains(servi, `<title>Journal</title>`) {
		t.Errorf("le contenu du guide doit être fusionné :\n%s", servi)
	}
	// Le snapshot reste du XML bien formé, déclaration et doctype compris.
	if err := decodeDocument(lu); err != nil {
		t.Errorf("snapshot mal formé : %v\n%s", err, servi)
	}
}

// guidageAvecDoctype : fixture fournisseur — la forme canonique d'un guide
// XMLTV, déclaration + doctype + racine <tv> (miroir du fixture du filtre
// internal/xmltv).
const guidageAvecDoctype = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE tv SYSTEM "xmltv.dtd">
<tv generator-info-name="fournisseur">
  <channel id="arte.fr"><display-name>Arte</display-name></channel>
  <programme start="20250101120000 +0100" channel="arte.fr"><title>Journal</title></programme>
</tv>
`

// decodeDocument décode le document complet (déclaration, doctype et racine)
// pour vérifier que le snapshot servi reste du XML bien formé.
func decodeDocument(lu []byte) error {
	decodeur := xml.NewDecoder(strings.NewReader(string(lu)))
	for {
		_, err := decodeur.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}
