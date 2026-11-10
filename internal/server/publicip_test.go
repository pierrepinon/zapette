package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pierrepinon/zapette/internal/config"
	"github.com/pierrepinon/zapette/internal/publicip"
	"github.com/pierrepinon/zapette/internal/store"
)

// TestApiStatusExposeLEtatDuResolveur : l'état du résolveur IP externe
// branché par le câblage (AvecPublicIP) transite tel quel dans les champs
// public_ipv4 / public_ipv6 / public_ip_last_check — l'horodatage partant
// en RFC3339 UTC. Le FORMAT du JSON ne change pas (champs déjà figés au
// ticket serveur), seules les valeurs deviennent réelles.
func TestApiStatusExposeLEtatDuResolveur(t *testing.T) {
	boutique, err := store.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	horodatage := time.Date(2025, 7, 14, 21, 30, 5, 0, time.FixedZone("test", 2*3600))

	s := Nouveau(boutique, func() config.Config { return config.Config{} }).
		AvecPublicIP(func() publicip.Etat {
			return publicip.Etat{
				IPv4:                 "203.0.113.7",
				IPv6:                 "2001:db8::42",
				DerniereVerification: &horodatage,
			}
		})

	recette := httptest.NewServer(s.Handler())
	t.Cleanup(recette.Close)
	_, _, corps := obtenir(t, recette.URL+"/api/status")

	var statut struct {
		PublicIPv4        string  `json:"public_ipv4"`
		PublicIPv6        string  `json:"public_ipv6"`
		PublicIPLastCheck *string `json:"public_ip_last_check"`
	}
	if err := json.Unmarshal(corps, &statut); err != nil {
		t.Fatalf("/api/status : corps non conforme : %v\n%s", err, corps)
	}
	if statut.PublicIPv4 != "203.0.113.7" {
		t.Errorf("public_ipv4 = %q, voulu 203.0.113.7", statut.PublicIPv4)
	}
	if statut.PublicIPv6 != "2001:db8::42" {
		t.Errorf("public_ipv6 = %q, voulu 2001:db8::42", statut.PublicIPv6)
	}
	if statut.PublicIPLastCheck == nil || *statut.PublicIPLastCheck != "2025-07-14T19:30:05Z" {
		t.Errorf("public_ip_last_check = %v, voulu 2025-07-14T19:30:05Z (RFC3339 UTC)", statut.PublicIPLastCheck)
	}
}

// TestApiStatusSansResolveur : sans résolveur branché (état initial du
// service ou scénario offline avant la première passe), les champs gardent
// la forme figée « indisponible » / null — aucun crash, aucun champ manquant.
func TestApiStatusSansResolveur(t *testing.T) {
	boutique, err := store.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	s := Nouveau(boutique, func() config.Config { return config.Config{} })

	recette := httptest.NewServer(s.Handler())
	t.Cleanup(recette.Close)
	_, _, corps := obtenir(t, recette.URL+"/api/status")

	var statut struct {
		PublicIPv4        string  `json:"public_ipv4"`
		PublicIPv6        string  `json:"public_ipv6"`
		PublicIPLastCheck *string `json:"public_ip_last_check"`
	}
	if err := json.Unmarshal(corps, &statut); err != nil {
		t.Fatalf("/api/status : corps non conforme : %v\n%s", err, corps)
	}
	if statut.PublicIPv4 != "indisponible" || statut.PublicIPv6 != "indisponible" || statut.PublicIPLastCheck != nil {
		t.Errorf("état sans résolveur = (%q, %q, %v), voulu (indisponible, indisponible, nil)",
			statut.PublicIPv4, statut.PublicIPv6, statut.PublicIPLastCheck)
	}
}

// TestApiStatusResolveurIndisponible : un résolveur branché en échec
// (offline) expose « indisponible » et null — le dashboard reste
// fonctionnel, le reste du contrat est intact.
func TestApiStatusResolveurIndisponible(t *testing.T) {
	boutique, err := store.Ouvrir(t.TempDir())
	if err != nil {
		t.Fatalf("ouverture du store : %v", err)
	}
	s := Nouveau(boutique, func() config.Config { return config.Config{} }).
		AvecPublicIP(func() publicip.Etat {
			return publicip.Etat{IPv4: "indisponible", IPv6: "indisponible"}
		})

	recette := httptest.NewServer(s.Handler())
	t.Cleanup(recette.Close)
	code, _, corps := obtenir(t, recette.URL+"/api/status")

	if code != http.StatusOK {
		t.Fatalf("/api/status en échec IP : code %d, voulu 200 — offline OK", code)
	}
	var statut struct {
		PublicIPv4        string  `json:"public_ipv4"`
		PublicIPv6        string  `json:"public_ipv6"`
		PublicIPLastCheck *string `json:"public_ip_last_check"`
	}
	if err := json.Unmarshal(corps, &statut); err != nil {
		t.Fatalf("/api/status : corps non conforme : %v\n%s", err, corps)
	}
	if statut.PublicIPv4 != "indisponible" || statut.PublicIPv6 != "indisponible" || statut.PublicIPLastCheck != nil {
		t.Errorf("état en échec IP = (%q, %q, %v), voulu (indisponible, indisponible, nil)",
			statut.PublicIPv4, statut.PublicIPv6, statut.PublicIPLastCheck)
	}
}
