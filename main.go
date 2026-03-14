package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type Config struct {
	Port            string
	ProwlarrURL     string
	ProwlarrAPIKey  string
	AlldebridAPIKey string
}

type Manifest struct {
	ID          string   `json:"id"`
	Version     string   `json:"version"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Resources   []string `json:"resources"`
	Types       []string `json:"types"`
	IDPrefixes  []string `json:"idPrefixes,omitempty"`
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
	cinemetaBase = "https://v3-cinemeta.strem.io/meta"
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
	config     = loadConfig()
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/manifest.json", manifestHandler)
	mux.HandleFunc("/stream/", streamHandler)

	addr := ":" + config.Port
	log.Printf("listening on %s", addr)
	if config.ProwlarrURL == "" {
		log.Printf("prowlarr disabled: set PROWLARR_URL and PROWLARR_API_KEY to enable search")
	} else {
		log.Printf("prowlarr configured url=%q", config.ProwlarrURL)
	}

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
		"name":     manifest.Name,
		"manifest": "/manifest.json",
	})
}

func manifestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	respondJSON(w, http.StatusOK, manifest)
}

func streamHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contentType, id, ok := parseStreamPath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}

	lookupID := metadataLookupID(contentType, id)
	meta, _, err := fetchCinemeta(contentType, lookupID)
	if err != nil {
		respondJSON(w, http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	var results []ProwlarrSearchResult

	query := buildProwlarrQuery(contentType, id, meta)
	if query != "" && prowlarrConfigured() {
		searchResults, _, err := searchProwlarr(query)
		if err != nil {
			respondJSON(w, http.StatusOK, StreamResponse{Streams: []Stream{}})
			return
		}
		results = searchResults
	}

	readyResults := make([]ProwlarrSearchResult, 0, len(results))
	for _, sr := range results {
		link, err := getStreamLink(sr.Guid)
		if err != nil {
			continue
		}
		link = strings.TrimSpace(link)
		if link == "" {
			continue
		}

		sr.DownloadURL = link
		fmt.Println(link)
		readyResults = append(readyResults, sr)
	}

	respondJSON(w, http.StatusOK, StreamResponse{
		Streams: buildStreams(contentType, readyResults),
	})
}

func parseStreamPath(path string) (contentType, id string, ok bool) {
	if !strings.HasPrefix(path, "/stream/") || !strings.HasSuffix(path, ".json") {
		return "", "", false
	}

	trimmed := strings.TrimSuffix(strings.TrimPrefix(path, "/stream/"), ".json")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}

	contentType = parts[0]
	id = parts[1]
	if id == "" {
		return "", "", false
	}

	switch contentType {
	case "movie", "series":
		return contentType, id, true
	default:
		return "", "", false
	}
}

func metadataLookupID(contentType, id string) string {
	if contentType != "series" {
		return id
	}

	parts := strings.SplitN(id, ":", 2)
	if len(parts) == 0 || parts[0] == "" {
		return id
	}
	return parts[0]
}

func fetchCinemeta(contentType, id string) (CinemetaMeta, string, error) {
	url := fmt.Sprintf("%s/%s/%s.json", cinemetaBase, contentType, id)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return CinemetaMeta{}, "", err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return CinemetaMeta{}, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CinemetaMeta{}, "", err
	}

	raw := string(body)
	if resp.StatusCode == http.StatusNotFound {
		return CinemetaMeta{}, raw, fmt.Errorf("cinemeta meta not found")
	}
	if resp.StatusCode != http.StatusOK {
		return CinemetaMeta{}, raw, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var envelope CinemetaEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return CinemetaMeta{}, raw, err
	}
	if envelope.Meta.Name == "" {
		return CinemetaMeta{}, raw, fmt.Errorf("cinemeta returned empty meta")
	}

	return envelope.Meta, raw, nil
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
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Origin, X-Api-Key")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func loadConfig() Config {
	return Config{
		Port:            envOrDefault("PORT", "3593"),
		ProwlarrURL:     strings.TrimRight(strings.TrimSpace(os.Getenv("PROWLARR_URL")), "/"),
		ProwlarrAPIKey:  strings.TrimSpace(os.Getenv("PROWLARR_API_KEY")),
		AlldebridAPIKey: strings.TrimSpace(os.Getenv("ALLDEBRID_API_KEY")),
	}
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
