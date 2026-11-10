// Package fetch : fetch des sources et passe séquentielle. Chaque passe
// fetch le M3U de chaque source (fetch réseau, filtre, copie disque), puis
// le XMLTV avec l'ensemble des identités retenues par le merge M3U, puis
// reconstruit le snapshot et écrit meta.json en dernier. La passe est
// strictement séquentielle : jamais deux fetch en parallèle.
package fetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fetcher est la fenêtre réseau : une URL et un contexte, un corps de
// réponse ou une erreur. Le corps est consommé en streaming par le filtre,
// jamais chargé en RAM (ADR-0001) ; la fermeture revient à l'appelant.
type Fetcher func(ctx context.Context, url string) (io.ReadCloser, error)

// NouveauFetcher retourne un fetch HTTP : timeout par tentative (défaut de
// config 60 s), corps servi tel quel. Un statut hors 2xx est une erreur —
// le corps de l'erreur est avalé dans la limite de 64 Kio pour rendre la
// connexion réutilisable.
func NouveauFetcher(timeout time.Duration) Fetcher {
	client := &http.Client{Timeout: timeout}
	return func(ctx context.Context, url string) (io.ReadCloser, error) {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		reponse, err := client.Do(request)
		if err != nil {
			return nil, err
		}
		if reponse.StatusCode < 200 || reponse.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, io.LimitReader(reponse.Body, 64*1024))
			reponse.Body.Close()
			return nil, fmt.Errorf("statut HTTP %d pour %s", reponse.StatusCode, url)
		}
		return reponse.Body, nil
	}
}
