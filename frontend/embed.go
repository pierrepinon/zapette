// Package frontend : la page dashboard construite par Vite (npm run build
// produit dist/) et embarquée dans le binaire Go par embed.FS — story 18 :
// le dashboard est 100 % autonome, zéro requête réseau externe au runtime,
// la page se sert même hors-ligne (tout le CSS et le JS sont dans le
// binaire ; seule la donnée vient de GET /api/status).
package frontend

import (
	"embed"
	"io/fs"
)

// distContenu : le build de production Vite, racine dist/. all: embarque
// aussi les fichiers à préfixe point/tiret bas que la convention pourrait
// exclure (Vite n'en produit pas, « all: » évite la surprise).
//
//go:embed all:dist
var distContenu embed.FS

// Dist retourne la page embarquée enracinée sur dist/, prête pour http.FS.
// L'erreur ne surviendrait que si « dist » disparaissait du build
// (impossible : présence garantie par le dépôt et le build npm) ; elle est
// restituée à l'appelant plutôt qu'étouffée par un panic package-level.
func Dist() (fs.FS, error) {
	return fs.Sub(distContenu, "dist")
}
