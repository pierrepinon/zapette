// Package store : la couche disque de zapette — copies de source filtrées
// (`sources/<name>.m3u|.xmltv`), snapshot fusionné (`snapshot.m3u`,
// `snapshot.xmltv`) et meta.json. Tout artefact est publié par renommage
// atomique (fichier temporaire voisin + rename sur le même répertoire) :
// un lecteur ne peut jamais voir un fichier partiel. meta.json est écrit
// en dernier à chaque passe.
package store

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Store désigne le répertoire de données (data_dir) : copies de source dans
// sources/, snapshot.m3u et snapshot.xmltv à la racine.
type Store struct {
	racine string
}

// Ouvrir prépare l'arborescence du répertoire de données : racine et
// racine/sources.
func Ouvrir(racine string) (*Store, error) {
	if err := os.MkdirAll(filepath.Join(racine, "sources"), 0o755); err != nil {
		return nil, fmt.Errorf("préparation de %q : %w", racine, err)
	}
	return &Store{racine: racine}, nil
}

// CheminCopie retourne le chemin de la copie de source d'une source.
func (s *Store) CheminCopie(nom, artefact string) string {
	return filepath.Join(s.racine, "sources", nom+"."+artefact)
}

// CheminsCopies retourne le chemin de copie de chaque source nommée pour
// l'artefact donné, dans l'ordre fourni (l'ordre de fusion de
// Reconstruire) : la clé de copie est le domaine du store, le cadran des
// sources au store de fetch.
func (s *Store) CheminsCopies(sources []string, artefact string) []string {
	chemins := make([]string, 0, len(sources))
	for _, nom := range sources {
		chemins = append(chemins, s.CheminCopie(nom, artefact))
	}
	return chemins
}

// CheminSnapshot retourne le chemin du snapshot fusionné d'un artefact.
func (s *Store) CheminSnapshot(artefact string) string {
	return filepath.Join(s.racine, "snapshot."+artefact)
}

// EcrireCopie persiste une copie de source filtrée : le contenu remplit un
// fichier temporaire voisin de la copie puis est renommé sous son nom
// définitif. Une erreur de remplissage (fetch interrompu, filtre en échec)
// laisse la copie précédente intacte : échec d'une source ⇒ copie figée.
func (s *Store) EcrireCopie(nom, artefact string, remplir func(io.Writer) error) error {
	if err := publier(s.CheminCopie(nom, artefact), remplir); err != nil {
		return fmt.Errorf("écriture de la copie %s.%s : %w", nom, artefact, err)
	}
	return nil
}

// publier écrit contenu dans un fichier temporaire voisin de chemin puis le
// renomme par-dessus chemin : le renommage est atomique sur le même
// répertoire, jamais de fichier partiel servi.
func publier(chemin string, remplir func(io.Writer) error) error {
	temp, err := os.CreateTemp(filepath.Dir(chemin), ".tmp-")
	if err != nil {
		return err
	}
	defer func() {
		temp.Close()
		os.Remove(temp.Name())
	}()
	if err := temp.Chmod(0o644); err != nil {
		return err
	}
	bufioWriter := bufio.NewWriterSize(temp, 64*1024)
	if err := remplir(bufioWriter); err != nil {
		return err
	}
	if err := bufioWriter.Flush(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(temp.Name(), chemin)
}
