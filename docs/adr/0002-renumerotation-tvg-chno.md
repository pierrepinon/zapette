# Renumérotation des `tvg-chno` du M3U servi

Statut : accepté. Remplace partiellement l'ADR-0001 (clause « M3U : octet pour octet »).

## Contexte

Les lecteurs M3U numérotent les chaînes d'après l'attribut `tvg-chno` des lignes `#EXTINF`
(Jellyfin le fait nativement). Celui du fournisseur est hors ordre, troué ou absent : le
tableau de bord de zapette numérote déjà les chaînes selon le plan officiel TNT de l'Arcom
(v0.2.0, `internal/tnt`), mais cette numérotation ne vivait que dans `canaux.json`, que seul
le dashboard lit. Un lecteur qui consomme `/m3u` — Jellyfin en tête — continuait donc
d'afficher l'ancienne numérotation du fournisseur, quel que soit le nombre de resynchronisations.

## Décision

Le snapshot M3U servi réécrit **le seul attribut `tvg-chno`** de chaque `#EXTINF`, présent ou
absent — au numéro calculé : plan Arcom du `tvg-id` d'après la table statique `internal/tnt`,
premier-arrivé sur collision de `tvg-id` dupliqué du fournisseur, sinon la suite séquentielle
de repli démarrant juste après le plan (27…). **Tout le reste traverse à l'octet près** :
autres attributs (ordre compris), nom affiché, URL de flux, directives rattachées, fins de
ligne `\n` ou `\r\n` d'origine, en-tête `#EXTM3U`. C'est la seule réécriture admise du M3U.

La réécriture vit dans la fusion streaming du snapshot (`fusionnerM3U`, une passe) ; la
règle est le type `store.Numerotation`, le même que
`canaux.json`, rejoué sur les mêmes copies dans le même ordre : les deux artefacts publient
des numéros coïncidants sans état partagé. La primitive de réécriture d'une ligne est
`m3u.Numeroter` (remplacement de valeur, insertion avant la virgule du nom affiché, fins de
ligne préservées, refus des lignes hors `#EXTINF`).

## Conséquences

- Les lecteurs (Jellyfin et les autres) affichent la numérotation Arcom/repli sans
  configuration supplémentaire ; le tableau de bord et le M3U servis sont d'accord.
- Le `tvg-chno` d'origine du fournisseur est perdu dans le M3U servi — délibéré : il ne
  numérote rien de cohérent et demeure dans les copies de source sur disque.
- Le budget RSS de l'ADR-0001 est intact : la seule structure résidente nouvelle est
  l'état de la numérotation, borné par la taille du plan Arcom (26 entrées + un compteur),
  négligeable face aux ensembles de dédup.
- Le critère d'acceptation « URL de flux strictement inchangées » normalise désormais le
  `tvg-chno` des deux côtés du diff binaire (`m3u.Numeroter(ligne, "0")`) : tout autre
  octet reste exigé à l'identique.
- Un lecteur qui se fie à l'ordre du fichier ne voit pas les chaînes triées par numéro :
  les numéros identifient les chaînes, l'ordre de lecture reste l'ordre du merge.
