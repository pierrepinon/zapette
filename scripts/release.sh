#!/usr/bin/env bash
# Recette de release — rejoue EXACTEMENT la ligne ci.yml. Appelé par le job
# « recette » de .github/workflows/release.yml (et disponible en local :
# ./scripts/release.sh, depuis la racine du dépôt ou ailleurs).
#
# Contenu couvert (tout ce que ci.yml vérifie, rien de moins) :
#   1. Go    : tests unitaires, vet, gofmt (échec si fichier non formaté) ;
#   2. Go    : tests avec détecteur de course (certification data-race) ;
#   3. Front : npm ci + vitest + svelte-check + build de production ;
#   4. Front : détection de dérive dist (git diff --exit-code -- frontend/dist).
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

echo "=== Go : tests ==="
go test ./... -count=1

echo "=== Go : vet ==="
go vet ./...

echo "=== Go : race (certification data-race — même ligne que le job go-race de ci.yml) ==="
CGO_ENABLED=1 CC=clang go test -race ./... -count=1

echo "=== Frontend : npm ci ==="
npm ci --prefix frontend

echo "=== Frontend : vitest ==="
npx --prefix frontend vitest run --root frontend

echo "=== Frontend : svelte-check ==="
npx --prefix frontend svelte-check

echo "=== Frontend : build + dérive dist ==="
npm run build --prefix frontend
git diff --exit-code -- frontend/dist \
  || { echo '::error::frontend/dist déroge au build de production — npm run build et commite le dist.'; exit 1; }

echo "=== Stamp : -ldflags -X main.version atteint le binaire ==="
# Build d'essai avec EXACTEMENT les ldflags de release du job « binaires »,
# puis assertion sur la VALEUR stampée seule (grep -a -F : valeur seule,
# les drapeaux passés autour ne font pas partie du motif).
go build -trimpath -ldflags "-s -w -X main.version=vRECETTE-STAMP" -o /tmp/zapette-stamp-recette .
grep -a -F -q -- 'vRECETTE-STAMP' /tmp/zapette-stamp-recette \
  || { echo '::error::stamp version absent du binaire — ldflags -X inefficace'; rm -f /tmp/zapette-stamp-recette; exit 1; }
rm -f /tmp/zapette-stamp-recette

echo "Recette complète : OK"
