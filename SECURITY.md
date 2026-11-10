# Politique de sécurité

## Signaler une vulnérabilité

Utilisez les **GitHub security advisories** du dépôt
(onglet *Security* → *Report a vulnerability*) — canal **privé** : ne signalez
jamais une vulnérabilité dans une issue publique.

- Accusé de réception : sous **72 h** (indicatif).
- Évaluation et correction ou contournement : sous **14 jours** (indicatif,
  selon la gravité).
- Versions corrigées publiées sous forme de tag `vX.Y.Z` avec
  [CHANGELOG.md](CHANGELOG.md) à jour.

## Versions supportées

Seule la dernière release est supportée : mettez à jour avant de signaler.

| Version | Support |
| --- | --- |
| 0.1.x | ✅ corrections de sécurité |
| < 0.1.0 | ❌ aucune |

## Recommandations d'exposition

- Zapette n'embarque **aucune authentification** : exposez-le derrière un
  reverse proxy qui gère l'authentification (basic auth, forward auth, SSO) et
  le TLS — jamais directement sur Internet (voir la section « Déploiement
  derrière un reverse proxy » du [README](README.md)).
- L'image est `FROM scratch` (pas de shell, pas de gestionnaire de paquets),
  en utilisateur non-root `65532`.
- Épinglez toujours l'**image par digest** (`@sha256:…`) — les balises ne sont
  pas immuables.
- Les artefacts de release (binaires tarballs + `SHA256SUMS`) sont signés
  cosign keyless : vérifiez-les avec
  `cosign verify-blob --certificate-identity-regexp '^https://github.com/pierrepinon/zapette/' --certificate-oidc-issuer https://token.actions.githubusercontent.com`.
