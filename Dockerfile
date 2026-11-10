# Image Zapette : binaire statique dans FROM scratch, certificats CA
# copiés d'une image distroless épinglée PAR DIGEST (jamais :latest),
# utilisateur non-root 65532. Le frontend/dist/ committé est embarqué dans
# le binaire à la compilation (embed.FS) — aucun Node ni outil Node à
# l'exécution.
#
# Les digests sont mis à jour par Renovate (manager dockerfile) : chaque
# ligne FROM garde son commentaire de tag d'origine.
# syntax=docker/dockerfile:1

# Étape des certificats CA : distroless static-debian12 (Debian stable),
# épinglée par digest de l'index multi-arch.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS ca-certificates

# Compilation croisée : l'étape tourne sur l'architecture de la machine de
# build ($BUILDPLATFORM) et cible TARGETARCH — compilation croisée native,
# pas d'émulation qemu.
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:fa3d596898d9f126af7c01d14ee7597bab3f39cccf392ef36b69fe77cd681f18 AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags="-s -w" -o /out/zapette . \
    && mkdir -p /out/data && chown 65532:65532 /out/data

FROM scratch
COPY --from=ca-certificates /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build --chown=65532:65532 /out/zapette /zapette
COPY --from=build --chown=65532:65532 /out/data /data

# Pas de shell, pas de gestionnaire de paquets : rien d'autre que le
# binaire et les CA. La config et les données passent par des montages
# (voir README) ; ZAPETTE_DATA_DIR pointe vers le répertoire possédé par
# l'utilisateur d'exécution.
ENV ZAPETTE_DATA_DIR=/data
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/zapette"]
