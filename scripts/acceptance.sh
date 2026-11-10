#!/bin/sh
# Harnais d'acceptation mesuré du ticket 09 (issue #10) — zapette.
#
# Reproductible à la demande, hors CI courte :
#   scripts/acceptance.sh
#
# Couvre les critères :
#   - RSS VmRSS < 100 Mio sur une playlist synthétique de 216 000 entrées,
#     mesurée pendant le fetch ET sous requêtes simultanées /m3u + /xmltv ;
#   - diff binaire : lignes #EXTINF/URL retenues servies octet pour octet ;
#   - kill -9 en plein fetch → redémarrage → service opérationnel sur le
#     disque.
#
# Aucune donnée réelle du fournisseur, aucun secret : tout est synthétique.
set -eu
cd "$(dirname "$0")/.."

mise exec -- go test ./internal/acceptance -run 'TestAcceptation' -v -count=1 "$@"
