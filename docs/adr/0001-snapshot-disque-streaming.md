# Snapshot filtré sur disque, servi en streaming

Les playlists IPTV dépassent souvent 200 000 entrées (~1 Go brut) et font mourir les proxys
existants (Threadfin et forks) par OOM, parce qu'ils chargent tout en mémoire. Zapette ne
conserve jamais la playlist complète en RAM : chaque source est filtrée en streaming à la
réception et sa dernière bonne copie est persistée sur disque ; le snapshot fusionné servi
est reconstruit à chaque tick par fusion streaming des copies de source, puis swap atomique.
Seuls l'ensemble des `tvg-id` retenus, les métadonnées et les compteurs résident en mémoire,
ce qui rend le budget RSS (< 100 Mio pour 216 000 entrées) indépendant de la taille de la
liste de groupes autorisés. Conséquence acceptée : un lecteur peut voir très brièvement un
`/m3u` neuf avec un `/xmltv` de la génération précédente pendant le swap.
