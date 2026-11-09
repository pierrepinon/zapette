package config

import (
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// Source désigne une entrée de configuration : un couple (URL M3U, URL XMLTV
// optionnelle) chez un fournisseur.
type Source struct {
	Name     string `yaml:"name"`
	M3UURL   string `yaml:"m3u_url"`
	XMLTVURL string `yaml:"xmltv_url"`
}

// Config est la configuration complète du service.
type Config struct {
	RefreshInterval time.Duration `yaml:"refresh_interval"`
	AllowedGroups   []string      `yaml:"allowed_groups"`
	Sources         []Source      `yaml:"sources"`
	FetchTimeout    time.Duration `yaml:"fetch_timeout"`
	FetchRetries    int           `yaml:"fetch_retries"`
	ServerAddr      string        `yaml:"server_addr"`
	DataDir         string        `yaml:"data_dir"`
	PublicIPv4URL   string        `yaml:"public_ipv4_url"`
	PublicIPv6URL   string        `yaml:"public_ipv6_url"`
}

// Load lit le fichier de config indiqué et retourne la configuration validée.
// Toute valeur de la forme ${VAR} voit VAR remplacée par la variable
// d'environnement correspondante ; une variable indéfinie est une erreur.
func Load(path string) (Config, error) {
	brut, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("lecture de la config %q : %w", path, err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(brut, &document); err != nil {
		return Config{}, fmt.Errorf("analyse de la config %q : %w", path, err)
	}

	if err := etendreEnvironnement(&document); err != nil {
		return Config{}, fmt.Errorf("config %q : %w", path, err)
	}

	var cfg Config
	if err := document.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("analyse de la config %q : %w", path, err)
	}
	appliquerDefauts(&cfg)
	if err := valider(&cfg); err != nil {
		return Config{}, fmt.Errorf("config %q : %w", path, err)
	}
	return cfg, nil
}

// appliquerDefauts complète les champs absents du fichier.
func appliquerDefauts(cfg *Config) {
	if cfg.ServerAddr == "" {
		cfg.ServerAddr = "[::]:8080"
	}
	if cfg.DataDir == "" {
		cfg.DataDir = "./data"
	}
	if cfg.PublicIPv4URL == "" {
		cfg.PublicIPv4URL = "https://api4.ipify.org"
	}
	if cfg.PublicIPv6URL == "" {
		cfg.PublicIPv6URL = "https://api6.ipify.org"
	}
	if cfg.FetchTimeout == 0 {
		cfg.FetchTimeout = 60 * time.Second
	}
	if cfg.FetchRetries == 0 {
		cfg.FetchRetries = 3
	}
}

// valider exige les champs sans valeur par défaut raisonnables : un intervalle
// de rafraîchissement strictement positif et, pour chaque source, un nom (clé
// des copies de source sur disque) et une URL M3U.
func valider(cfg *Config) error {
	if cfg.RefreshInterval <= 0 {
		return fmt.Errorf("refresh_interval doit être une durée strictement positive, obtenu %q", cfg.RefreshInterval)
	}
	for i, source := range cfg.Sources {
		if source.Name == "" {
			return fmt.Errorf("sources[%d] : name manquant", i)
		}
		if source.M3UURL == "" {
			return fmt.Errorf("sources[%d] (%s) : m3u_url manquant", i, source.Name)
		}
	}
	return nil
}

var modeleVariable = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// etendreEnvironnement remplace ${VAR} par la variable d'environnement
// correspondante dans toute valeur scalaire du document, en réinitialisant le
// tag résolu pour que le type du nœud soit inféré à partir de la valeur
// développée (durées, entiers, booléens).
func etendreEnvironnement(noeud *yaml.Node) error {
	if noeud.Kind != yaml.DocumentNode && noeud.Kind != yaml.SequenceNode && noeud.Kind != yaml.MappingNode {
		if noeud.Tag != "!!str" || !modeleVariable.MatchString(noeud.Value) {
			return nil
		}
		// Détection explicite des variables indéfinies, avant toute expansion.
		for _, correspondance := range modeleVariable.FindAllStringSubmatch(noeud.Value, -1) {
			nom := correspondance[1]
			if _, present := os.LookupEnv(nom); !present {
				return fmt.Errorf("ligne %d : variable d'environnement %q indéfinie", noeud.Line, nom)
			}
		}
		noeud.Value = modeleVariable.ReplaceAllStringFunc(noeud.Value, func(ref string) string {
			nom := ref[2 : len(ref)-1]
			return os.Getenv(nom)
		})
		if noeud.Style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle|yaml.LiteralStyle|yaml.FoldedStyle) == 0 {
			// Référence non quotée : le type est réinféré à partir de la
			// valeur développée (durées, entiers, booléens).
			noeud.Tag = ""
		}
		return nil
	}
	for _, enfant := range noeud.Content {
		if err := etendreEnvironnement(enfant); err != nil {
			return err
		}
	}
	return nil
}
