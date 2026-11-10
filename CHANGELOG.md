# Changelog

Toutes les évolutions de ce projet sont documentées ici.

Le format suit [Keep a Changelog](https://keepachangelog.com/fr/1.1.0/) ;
le versionnage suit [SemVer](https://semver.org/lang/fr/).

## [0.1.0] — 2026-11-10

### Release initiale

Zapette : filtre IPTV — récupère périodiquement les playlists M3U et le
guide XMLTV d'un fournisseur, n'en conserve que les groupes autorisés, et
resse le résultat tel quel à un lecteur (aucune réécriture ni proxy de flux).

### Ajouts

- **Fetch / filtre / fusion streaming** : playlists M3U + guide XMLTV filtrés
  à la volée selon les groupes autorisés (`allowed_groups`), sans jamais
  charger un artefact complet en mémoire (budget RSS < 100 Mio — mesuré
  51 Mio pour 216 000 entrées).
- **Snapshot disque + repli** : le dernier snapshot servi est persisté sur
  disque (`data_dir`) ; un fetch en échec laisse l'ancien snapshot servi
  (comportement « copie figée »), au démarrage comme en régime établi.
- **API HTTP** : `/m3u`, `/xmltv`, `/api/status` (état sources + snapshot,
  compteurs, ring des dernières erreurs), `/m3u?group=<nom>` pour la
  prévisualisation d'un seul groupe.
- **Dashboard web** (Svelte 5, embarqué dans le binaire via `embed.FS`) :
  état par source/artefact, compteurs du snapshot, IP publiques détectées,
  ring des erreurs récentes, historique dupliqués, bascule de thème
  sombre/clair, routage par hash (`#/` dashboard, `#/canaux` page Chaînes).
- **Stamp de version** : `version` exposé sur `/api/status` et au footer du
  dashboard, injecté à la construction par `-ldflags -X main.version=`.
- **CI** (`ci.yml`) : frontend (`npm ci`, Vitest, `svelte-check`, build de
  production + détection de dérive de `frontend/dist/`) et Go (`go test`,
  `go vet`, `gofmt`, `go test -race`, harnais d'acceptation).
- **Image multi-arch** (`image.yml`) : buildx amd64 + arm64 publiée sur
  `ghcr.io`, digest exposé en sortie de job, attestations provenance (mode
  max) + SBOM + signature cosign keyless.
- **Recette de release** (`release.yml`) : déclenchée uniquement par tag
  `^v[0-9]` ; re-joue la recette (tests Go, frontend, harnais
  d'acceptation), cross-compile linux amd64 + arm64 (`CGO_ENABLED=0`),
  tarballs `zapette-<version>-linux-<arch>.tar.gz` (binaire + README +
  LICENSE) avec `SHA256SUMS`, signatures cosign `sign-blob` keyless, tout
  attaché à la GitHub Release (`gh release create --verify-tag
  --generate-notes`).

[0.1.0]: https://github.com/pierrepinon/zapette/releases/tag/v0.1.0
