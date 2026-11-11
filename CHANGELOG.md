# Changelog

Toutes les évolutions de ce projet sont documentées ici.

Le format suit [Keep a Changelog](https://keepachangelog.com/fr/1.1.0/) ;
le versionnage suit [SemVer](https://semver.org/lang/fr/).

## [0.2.1] — 2026-11-11

### Corrigé

- **Numérotation visible dans les lecteurs M3U** (Jellyfin en tête) : ceux-ci
  numérotent les chaînes d'après l'attribut `tvg-chno` du M3U, que zapette
  resservait tel quel du fournisseur — la numérotation officielle de la v0.2.0
  ne vivait que dans `canaux.json` (dashboard) et restait invisible des
  lecteurs. Le snapshot M3U servi réécrit désormais le SEUL attribut
  `tvg-chno` de chaque `#EXTINF` au numéro calculé (plan Arcom du `tvg_id`,
  sinon suite de repli) ; tout le reste — URL de flux, noms, logos, fins de
  ligne, en-tête — traverse à l'octet près (ADR-0002, remplacement partiel
  de l'ADR-0001). Dashboard et lecteur affichent la même numérotation : la
  règle est partagée et rejouée à l'identique. Budget RSS inchangé (état de
  numérotation borné par le plan, 26 entrées).

## [0.2.0] — 2026-11-11

### Changements

- **Numérotation officielle des chaînes (plan Arcom)** : le champ `numero`
  de `canaux.json`, qui reprenait tel quel le `tvg-chno` du fournisseur
  (souvent hors ordre, troué, parfois absent), est désormais calculé :
  numéro du plan TNT officiel fixé par l'Arcom (numérotation en vigueur
  depuis le 6 juin 2025) quand le `tvg_id` de la chaîne y figure —
  comparaison insensible à la casse, premier-arrivé en cas de `tvg_id`
  dupliqué du fournisseur —, sinon suite séquentielle de repli démarrant
  juste après le plan, dans l'ordre déclaré des sources de la config puis
  l'ordre du fichier. Changement de contrat : `numero` était null quand le
  fournisseur ne publiait pas de `tvg-chno`, il est maintenant TOUJOURS
  défini (type du dashboard mis à jour en conséquence). Les snapshots M3U
  et XMLTV servis ne sont pas touchés.

### Maintenance

- Renovate : clé invalide `managers` remplacée par `enabledManagers` (la
  configuration sinon cassée à chaque passage de Renovate).

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

[0.2.1]: https://github.com/pierrepinon/zapette/releases/tag/v0.2.1
[0.2.0]: https://github.com/pierrepinon/zapette/releases/tag/v0.2.0
[0.1.0]: https://github.com/pierrepinon/zapette/releases/tag/v0.1.0
