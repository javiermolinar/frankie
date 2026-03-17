package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Port              string `json:"port,omitempty"`
	ProwlarrURL       string `json:"prowlarr_url,omitempty"`
	ProwlarrAPIKey    string `json:"prowlarr_api_key,omitempty"`
	AlldebridAPIKey   string `json:"alldebrid_api_key,omitempty"`
	PrimaryLanguage   string `json:"primary_language,omitempty"`
	SecondaryLanguage string `json:"secondary_language,omitempty"`
	PublicURL         string `json:"public_url,omitempty"`
}

type Manifest struct {
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Resources   []string `json:"resources"`
	Types       []string `json:"types"`
	IDPrefixes  []string `json:"idPrefixes,omitempty"`
	Logo        string   `json:"logo,omitempty"`
	Background  string   `json:"background,omitempty"`
}

type StreamResponse struct {
	Streams []Stream `json:"streams"`
}

type Stream struct {
	Name          string         `json:"name,omitempty"`
	Title         string         `json:"title,omitempty"`
	Description   string         `json:"description,omitempty"`
	URL           string         `json:"url,omitempty"`
	BehaviorHints *BehaviorHints `json:"behaviorHints,omitempty"`
}

type BehaviorHints struct {
	NotWebReady bool   `json:"notWebReady,omitempty"`
	Filename    string `json:"filename,omitempty"`
	VideoSize   int64  `json:"videoSize,omitempty"`
	BingeGroup  string `json:"bingeGroup,omitempty"`
}

type CinemetaEnvelope struct {
	Meta CinemetaMeta `json:"meta"`
}

type CinemetaMeta struct {
	IMDbID string `json:"imdb_id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Year   string `json:"year"`
}

type SeriesRequest struct {
	IMDbID  string
	Season  int
	Episode int
}

const (
	cinemetaBase                = "https://v3-cinemeta.strem.io/meta"
	debridResolveConcurrency    = 6
	prowlarrSearchLimit         = 100
	debridResolveCandidateLimit = 50
	streamResponseLimit         = 15
	defaultPort                 = "3593"
	defaultConfigPath           = "config.json"
)

var manifest = Manifest{
	ID:          "com.frankie",
	Version:     "0.1.0",
	Name:        "Frankie",
	Description: "Search and find, locally",
	Resources:   []string{"stream"},
	Types:       []string{"movie", "series"},
	IDPrefixes:  []string{"tt"},
}

var (
	httpClient = &http.Client{Timeout: 10 * time.Second}
	configMu   sync.RWMutex
	config     = loadConfig()
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/configure", configureHandler)
	mux.HandleFunc("/manifest.json", manifestHandler)
	mux.HandleFunc("/stream/", streamHandler)
	mux.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	cfg := getConfig()
	addr := ":" + cfg.Port
	log.Printf("listening on %s", addr)
	if cfg.ProwlarrURL == "" {
		log.Printf("prowlarr disabled: set PROWLARR_URL and PROWLARR_API_KEY or use /configure to enable search")
	} else {
		log.Printf("prowlarr configured url=%q", cfg.ProwlarrURL)
	}
	log.Printf("config file path: %s", configFilePath())

	if err := http.ListenAndServe(addr, withCORS(mux)); err != nil {
		log.Fatal(err)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{
		"name":      manifest.Name,
		"manifest":  "/manifest.json",
		"configure": "/configure",
	})
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(payload); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Origin, X-Api-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loadConfig() Config {
	cfg := Config{Port: defaultPort}

	filePath := configFilePath()
	fileConfig, err := readConfigFile(filePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("read config file %s: %v", filePath, err)
		}
	} else {
		cfg = mergeConfig(cfg, fileConfig)
	}

	cfg = mergeConfig(cfg, Config{
		Port:              os.Getenv("PORT"),
		ProwlarrURL:       os.Getenv("PROWLARR_URL"),
		ProwlarrAPIKey:    os.Getenv("PROWLARR_API_KEY"),
		AlldebridAPIKey:   os.Getenv("ALLDEBRID_API_KEY"),
		PrimaryLanguage:   os.Getenv("PRIMARY_LANGUAGE"),
		SecondaryLanguage: os.Getenv("SECONDARY_LANGUAGE"),
		PublicURL:         os.Getenv("PUBLIC_URL"),
	})

	cfg = normalizeConfig(cfg)
	if cfg.Port == "" {
		cfg.Port = defaultPort
	}
	return cfg
}

func configFilePath() string {
	path := strings.TrimSpace(os.Getenv("CONFIG_FILE"))
	if path == "" {
		return defaultConfigPath
	}
	return path
}

func normalizeConfig(cfg Config) Config {
	cfg.Port = strings.TrimSpace(cfg.Port)
	cfg.ProwlarrURL = strings.TrimRight(strings.TrimSpace(cfg.ProwlarrURL), "/")
	cfg.ProwlarrAPIKey = strings.TrimSpace(cfg.ProwlarrAPIKey)
	cfg.AlldebridAPIKey = strings.TrimSpace(cfg.AlldebridAPIKey)
	cfg.PrimaryLanguage = canonicalLanguageLabel(cfg.PrimaryLanguage)
	cfg.SecondaryLanguage = canonicalLanguageLabel(cfg.SecondaryLanguage)
	if cfg.PrimaryLanguage == "" {
		cfg.SecondaryLanguage = ""
	}
	if cfg.PrimaryLanguage != "" && strings.EqualFold(cfg.PrimaryLanguage, cfg.SecondaryLanguage) {
		cfg.SecondaryLanguage = ""
	}
	cfg.PublicURL = strings.TrimRight(strings.TrimSpace(cfg.PublicURL), "/")
	return cfg
}

func mergeConfig(base Config, override Config) Config {
	if strings.TrimSpace(override.Port) != "" {
		base.Port = override.Port
	}
	if strings.TrimSpace(override.ProwlarrURL) != "" {
		base.ProwlarrURL = override.ProwlarrURL
	}
	if strings.TrimSpace(override.ProwlarrAPIKey) != "" {
		base.ProwlarrAPIKey = override.ProwlarrAPIKey
	}
	if strings.TrimSpace(override.AlldebridAPIKey) != "" {
		base.AlldebridAPIKey = override.AlldebridAPIKey
	}
	if strings.TrimSpace(override.PrimaryLanguage) != "" {
		base.PrimaryLanguage = override.PrimaryLanguage
	}
	if strings.TrimSpace(override.SecondaryLanguage) != "" {
		base.SecondaryLanguage = override.SecondaryLanguage
	}
	if strings.TrimSpace(override.PublicURL) != "" {
		base.PublicURL = override.PublicURL
	}
	return base
}

func readConfigFile(path string) (Config, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := json.Unmarshal(content, &cfg); err != nil {
		return Config{}, err
	}

	return normalizeConfig(cfg), nil
}

func saveConfigFile(path string, cfg Config) error {
	cfg = normalizeConfig(cfg)
	payload, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o600)
}

func getConfig() Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return config
}

func setConfig(next Config) {
	configMu.Lock()
	defer configMu.Unlock()
	config = normalizeConfig(next)
}
