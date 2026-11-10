# Zapette

Zapette est un filtre IPTV : il récupère périodiquement la playlist M3U et le
guide XMLTV d'un fournisseur, n'en conserve que les groupes autorisés, et
ressert le résultat tel quel à un lecteur (Jellyfin, ou tout lecteur M3U).
Aucune transformation du contenu, seulement du filtrage — les URL de flux sont
retransmises octet par octet, jamais réécrites ni proxifiées.

- **Un unique binaire Go statique** (`CGO_ENABLED=0`), dashboard web embarqué
  (Svelte 5 + Tailwind CSS v4, servi via `embed.FS`, aucune ressource externe).
- **Filtrage en streaming** : la playlist complète n'est jamais chargée en
  mémoire (objectif RSS < 100 Mio pour 216 000 entrées) — voir
  [l'ADR-0001](docs/adr/0001-snapshot-disque-streaming.md).
- **Résilient** : la dernière bonne copie de chaque source est persistée sur
  disque et reste servie quand un fetch échoue ; le vocabulaire du domaine est
  défini dans [GLOSSARY.md](GLOSSARY.md).

## Fonctionnement

1. Au démarrage puis à chaque `refresh_interval`, chaque `source` déclarée est
   téléchargée (séquentiellement, timeout et 3 reprises par source, backoff
   fixe de 5 s) et filtrée en streaming selon la liste de groupes autorisés.
2. Chaque source filtrée est stockée sur disque (*copie de source*) ; un fetch
   en échec fige uniquement la copie de la source concernée.
3. Les copies sont fusionnées et dédoublonnées (identité = `tvg-id`, sinon URL
   de flux ; première occurrence conservée) en un *snapshot* sur disque, publié
   par renommage atomique, et servi tel quel.

Pendant qu'aucun snapshot n'existe encore (démarrage à froid avec un
fournisseur indisponible), `/m3u` et `/xmltv` répondent explicitement **503**.

## Lancer le binaire

```sh
# Go et Node épinglés via mise (voir mise.toml)
mise install
mise exec -- go build -o zapette .

cp config.example.yaml config.yaml   # puis éditez-le
ZAPETTE_CONFIG=./config.yaml ./zapette
```

Le binaire écoute sur `[::]:8080` par défaut et démarre même si aucun fetch
n'a encore réussi : le dashboard reste accessible dans tous les cas.

## Lancer l'image

L'image multi-arch (amd64 + arm64) est publiée sur `ghcr.io/pierrepinon/zapette`
par GitHub Actions ([`.github/workflows/image.yml`](.github/workflows/image.yml)).
L'image est construite `FROM scratch` : elle ne contient que le binaire statique
et les certificats CA, et tourne en utilisateur non-root `65532`. Elle ne
contient ni shell ni gestionnaire de paquets.

Épinglez le **digest** publié en sortie du workflow (`jobs.image.outputs.digest`)
— la balise `latest` n'est pas immuable, le digest l'est :

```sh
docker run -d --name zapette \
  -p 127.0.0.1:8080:8080 \
  -v ./config.yaml:/config/config.yaml:ro \
  -v zapette-data:/data \
  -e ZAPETTE_CONFIG=/config/config.yaml \
  -e IPTV_A_M3U_URL="https://…/playlist.m3u" \
  ghcr.io/pierrepinon/zapette@sha256:<digest publié par le workflow Image>
```

Voir [`docker-compose.yml`](docker-compose.yml) pour un exemple complet
derrière un reverse proxy.

## Configuration

Chemin du fichier : `ZAPETTE_CONFIG` (défaut `./config.yaml`). Le fichier est
relu à chaque tick ; en cas d'erreur de lecture, la dernière bonne config est
conservée. Les secrets passent par l'expansion `${VAR}` — une référence quotée
garde une valeur texte, une référence non quotée reçoit le type de la valeur
développée. **Une `${VAR}` indéfinie est une erreur de chargement, jamais un
retour au défaut** : les défauts du tableau ci-dessous ne s'appliquent que
quand la clé elle-même est absente du fichier. Exemple complet : [`config.example.yaml`](config.example.yaml).

| Clé | Type / défaut | Rôle |
| --- | --- | --- |
| `refresh_interval` | durée Go, `6h` | Intervalle entre deux cycles de fetch |
| `allowed_groups` | liste de chaînes | Valeurs exactes (sensibles à la casse) de `group-title` conservées ; vide = tout conserver ; une chaîne sans `group-title` n'est jamais retenue |
| `sources[].name` | chaîne | Nom de la source (nom de fichier de la copie de source) |
| `sources[].m3u_url` | URL | Playlist M3U du fournisseur |
| `sources[].xmltv_url` | URL, optionnel | Guide XMLTV du fournisseur |
| `fetch_timeout` | durée Go, `60s` | Timeout d'un fetch de source |
| `fetch_retries` | entier, `3` | Reprises après échec, pause fixe de 5 s ; `0` est impossible — la valeur 0 est indistinguable d'une clé absente et est coercée au défaut |
| `server_addr` | chaîne, `[::]:8080` | Adresse d'écoute HTTP — le SEUL chemin de l'adresse : aucune variable d'environnement n'est relue directement |
| `data_dir` | chemin, `./data` | Répertoire des copies de source et du snapshot |
| `public_ipv4_url` / `public_ipv6_url` | URL, ipify | Endpoints de détection de l'IP publique affichée dans le dashboard |

Variables d'environnement : `ZAPETTE_CONFIG` (chemin du fichier).
`ZAPETTE_ADDR` et `ZAPETTE_DATA_DIR` ne sont JAMAIS relues directement : la
config est le seul chemin (une ressource = un chemin) — elles n'y entrent que
si votre fichier les développe explicitement (ex. `server_addr: ${ZAPETTE_ADDR}`
; alors elles doivent être définies, sinon erreur de chargement). Dans l'image,
`ZAPETTE_DATA_DIR` vaut `/data` (répertoire possédé par l'utilisateur `65532`).

## Points d'accès HTTP

| Route | Réponse |
| --- | --- |
| `GET /m3u` | Snapshot M3U filtré (`audio/x-mpegurl; charset=utf-8`), `Cache-Control: no-cache` |
| `GET /xmltv` | Snapshot XMLTV filtré (`application/xml; charset=utf-8`), `Cache-Control: no-cache` |
| `GET /m3u?group=<nom>` | Prévisualisation du filtrage pour un groupe |
| `GET /api/status` | État JSON : par source/artefact (`last_ok`, `last_error`, `next_at`), âge et compteurs du snapshot, RSS, recent errors, config, IP publiques |
| `GET /` | Dashboard (poll `/api/status` toutes les 5 s, theme sombre/clair) |

Les URL à donner au lecteur sont donc `http://<hôte>:8080/m3u` et
`http://<hôte>:8080/xmltv`.

## Déploiement derrière un reverse proxy

Zapette n'embarque **aucune authentification** : c'est le reverse proxy amont
qui gère l'exposition, l'authentification et le TLS. Le service doit rester sur
un réseau interne — ne publiez que le proxy publiquement.

Caddy (le plus simple, TLS automatique) :

```caddy
playlist.example.com {
    # Ajoutez ici votre authentification (basic_auth, forward_auth, …)
    reverse_proxy 127.0.0.1:8080
}
```

nginx :

```nginx
server {
    listen 443 ssl;
    server_name playlist.example.com;
    #ssl_certificate     /etc/ssl/…;  # + votre authentification
    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
    }
}
```

Aucune configuration particulière n'est nécessaire : pas de WebSocket, pas de
flux proxifié — seules des réponses HTTP simples, donc compatibles avec
n'importe quel proxy. L'URL du fournisseur ne fuit pas : le lecteur ne voit
jamais les URL amont.

## Image de débogage (distroless)

L'image de production étant `FROM scratch` (aucun shell), une variante de
débogage est documentée pour introspecter les données ou le réseau au besoin :
**`gcr.io/distroless/static-debian12:debug-nonroot`** — dernière Debian stable,
épinglée par digest, suivie par Renovate (via [`docker-compose.yml`](docker-compose.yml)),
mais **jamais construite ni publiée par la CI** : elle est tirée de gcr.io à la
demande.

Elle contient busybox (`/busybox/sh`) et tourne en `65532`. Pour ouvrir un
shell sur les données du conteneur de production :

```sh
docker run --rm -it --entrypoint /busybox/sh \
  -v zapette-data:/data:ro --network container:zapette \
  gcr.io/distroless/static-debian12:debug-nonroot@sha256:d5563cc7f2f44313f332e91138cc8c6a158899afeeeab2fce3b0f9ccdb3cf9ee
```

(ou `docker compose run --rm debug` avec l'exemple de compose.)

## Intégration continue

- **CI** ([`.github/workflows/ci.yml`](.github/workflows/ci.yml)) : `npm ci` +
  tests Vitest + build de production du frontend, **détection de dérive** sur
  `frontend/dist/` (le dist committé doit être exactement le produit du build),
  `go test ./... -count=1`, `go vet`, `gofmt`, puis build `CGO_ENABLED=0` dont
  on vérifie qu'il embarque bien le frontend.
- **Job `go-race`** : `go test -race` ne « teste » pas plus vite — c'est une
  **certification data-race** du runtime (harnais et serveur concurrents)
  clang épinglé via mise.
- **Harnais d'acceptation** ([`scripts/acceptance.sh`](scripts/acceptance.sh),
  hors CI courte) : le budget RSS de la spec (100 Mio pour 216 000 entrées)
  est mesuré par `internal/acceptance` sur une playlist **SYNTHÉTIQUE en
  forme réelle** (générée à la volée, entrées `#EXTINF`/URL plausibles) —
  **aucune donnée fournisseur dans le dépôt** (règle secrets) : les mesures
  tiennent par la forme, pas par un contenu réel ; reproductible à la demande
  via le script.
- **Image** ([`.github/workflows/image.yml`](.github/workflows/image.yml)) :
  buildx multi-arch amd64 + arm64 → `ghcr.io`, digest publié en sortie de job.
- **Renovate** (`renovate.json`) : mise, npm, github-actions et dockerfile ;
  les actions GitHub sont épinglées par SHA, les images par digest.

## Écarts assumés (hors plan initial, validés à la revue)

Le dépôt contient trois choses qui débordent le plan du ticket fondateur et
qui restent là délibérément :

- **[`docker-compose.yml`](docker-compose.yml) est un exemple de déploiement**
  (reverse proxy, volume de données, service `debug`) — pas un artefact de
  production : l'image reste le chemin d'entrée.

- **Le job `go-race` de la CI est une certification data-race**, pas un test
  de rapidité ni de redondance : il fait tourner le même runtime concurrent
  (harnais + serveur + résolveur) sous le détecteur de course.

- **`frontend/dist/` est committé** : c'est ce que `embed.FS` embarque — le
  binaire sert toujours un dashboard complet, y compris construit hors CI.
  La CI vérifie le zéro-dérive (`npm run build` doit reproduire exactement
  le dist committé).

## Licences et limites

Pas d'authentification intégrée, pas de transcodage, pas de proxy de flux :
Zapette ne fait que filtrer et re-servir. Voir
[le ticket fondateur](https://github.com/pierrepinon/zapette/issues/1).
