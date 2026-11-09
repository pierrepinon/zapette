# Zapette

Vocabulaire du domaine d'un filtre IPTV : un service qui récupère la playlist M3U et le
guide XMLTV d'un fournisseur, ne conserve que les groupes autorisés, et les ressert tels
quels à un lecteur. Aucune transformation du contenu, seulement du filtrage.

## Fournisseur et sources

**Fournisseur**:
Le service IPTV amont qui publie une playlist M3U et un guide XMLTV.
_Avoid_: provider, upstream

**Source**:
Une entrée de la configuration désignant un couple (URL M3U, URL XMLTV optionnelle) chez un
fournisseur. Plusieurs sources peuvent viser le même fournisseur ; leurs résultats sont
fusionnés avec dédoublonnage.
_Avoid_: playlist, URL amont

**Doublon**:
Deux entrées de même identité (même `tvg-id`, ou même URL de flux à défaut). La première
occurrence — dans l'ordre des sources de la config, puis l'ordre dans le fichier — est
conservée ; les suivantes sont ignorées.
_Avoid_: duplicate entry, doublon ignoré sans règle de priorité

**Flux**:
L'URL de stream d'une chaîne, telle que publiée par le fournisseur. Elle est toujours
resservie octet par octet, jamais réécrite ni proxifiée.
_Avoid_: lien, stream URL

## Filtrage

**Liste de groupes autorisés**:
La liste globale (unique, appliquée à toutes les sources) des valeurs exactes de
`group-title` retenues, sensibles à la casse ; liste vide = tout conserver. Hors liste vide,
toute chaîne dont le groupe n'y figure pas — ou qui n'a pas de `group-title` — est exclue.
_Avoid_: allowlist, whitelist, filtre

**Chaîne**:
Une entrée de la playlist M3U (ligne `#EXTINF` suivie de son URL de flux). Son identité est
son `tvg-id` quand il est présent, sinon son URL de flux.
_Avoid_: entry, item, chaîne TV sans précision d'identité

**Guide**:
Le document XMLTV du fournisseur : chaînes (`<channel>`) et programmes (`<programme>`).
_Avoid_: EPG, guide TV sans précision

## Artefacts servis

**Copie de source**:
La dernière bonne copie filtrée d'une source, persistée sur disque. Un fetch en échec fige
cette copie sans toucher aux autres sources.
_Avoid_: cache, dernier fichier reçu

**Snapshot**:
La copie **fusionnée** et dédoublonnée (M3U + XMLTV), persistée sur disque et servie telle
quelle. C'est aussi la copie de repli globale en cas d'échec total des fetchs.
_Avoid_: cache, buffer, résultat en mémoire
