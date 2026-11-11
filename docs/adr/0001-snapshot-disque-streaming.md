# Snapshot filtré sur disque, servi en streaming

> Partiellement remplacé par l'ADR-0002 : le M3U servi réécrit désormais son attribut
> `tvg-chno` (seule exception à la fidélité « octet pour octet » ci-dessous).

Les playlists IPTV dépassent souvent 200 000 entrées (~1 Go brut) et font mourir les proxys
existants (Threadfin et forks) par OOM, parce qu'ils chargent tout en mémoire. Zapette ne
conserve jamais la playlist complète en RAM : chaque source est filtrée en streaming à la
réception et sa dernière bonne copie est persistée sur disque ; le snapshot fusionné servi
est reconstruit à chaque tick par fusion streaming des copies de source, puis swap atomique.

Deux décisions de fidélité encadrent cette reconstruction :

- **M3U : octet pour octet.** Le snapshot servi est la concaténation des lignes brutes des
  copies d'origine (en-tête du premier arrivé, entrées retenues réémises telles quelles,
  fins de ligne comprises) — jamais réécrites ni proxifiées.
- **XMLTV : ré-encodage sémantique.** Le snapshot est réémis token par token par le parseur
  (déclaration XML et DOCTYPE du premier arrivé conservés) : l'écriture du snapshot peut
  différer à l'octet près des copies (échappements, indentation), seul le contenu sémantique
  du guide est contractuel.

Résident en mémoire : l'ensemble des `tvg-id` retenus, l'ensemble de dédup des programmes
(chaîne + start) — croissance O(guide) dans les deux cas —, les métadonnées et les
compteurs. Le budget RSS (< 100 Mio pour 216 000 entrées) en dépend : il est vérifié par le
harnais d'acceptation (`internal/acceptance`, `scripts/acceptance.sh`), sur playlist
synthétique en forme réelle — tout le reste est streaming. Conséquence acceptée : un lecteur
peut voir très brièvement un `/m3u` neuf avec un `/xmltv` de la génération précédente pendant
le swap.
