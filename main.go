package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"os"
	"strings"
	"time"
)

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
	Name  string `json:"name,omitempty"`
	Title string `json:"title,omitempty"`
	URL   string `json:"url,omitempty"`
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

const (
	mockVideoURL = "https://cdn.frankie.local/media/The.Matrix.1999.1080p.BluRay.x264.mkv"
	cinemetaBase = "https://v3-cinemeta.strem.io/meta"
)

var manifest = Manifest{
	ID:          "com.javimolina.frankie",
	Version:     "0.1.0",
	Name:        "Frankie",
	Description: "Simple Stremio addon bootstrap",
	Resources:   []string{"stream"},
	Types:       []string{"movie", "series"},
	IDPrefixes:  []string{"tt"},
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/manifest.json", manifestHandler)
	mux.HandleFunc("/stream/", streamHandler)

	addr := ":" + envOrDefault("PORT", "3593")
	log.Printf("listening on %s", addr)

	if err := http.ListenAndServe(addr, withCORS(logging(mux))); err != nil {
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

	log.Printf("stremio stream request parsed type=%q id=%q", contentType, id)

	lookupID := metadataLookupID(contentType, id)
	meta, raw, err := fetchCinemeta(contentType, lookupID)
	if err != nil {
		log.Printf("cinemeta lookup failed type=%q id=%q lookup_id=%q err=%v", contentType, id, lookupID, err)
	} else {
		log.Printf("cinemeta raw response type=%q lookup_id=%q: %s", contentType, lookupID, truncateForLog(raw, 4000))
		log.Printf("cinemeta parsed meta imdb_id=%q name=%q year=%q type=%q", meta.IMDbID, meta.Name, meta.Year, meta.Type)
	}

	respondJSON(w, http.StatusOK, StreamResponse{
		Streams: []Stream{
			{
				Name:  manifest.Name,
				Title: buildMockTitle(contentType, id, meta),
				URL:   mockVideoURL,
			},
		},
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
	if resp.StatusCode != http.StatusOK {
		return CinemetaMeta{}, raw, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var envelope CinemetaEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return CinemetaMeta{}, raw, err
	}

	return envelope.Meta, raw, nil
}

func buildMockTitle(contentType, id string, meta CinemetaMeta) string {
	if meta.Name == "" {
		return fmt.Sprintf("Mock %s stream for %s", contentType, id)
	}

	if meta.Year == "" {
		return fmt.Sprintf("Mock %s stream for %s", contentType, meta.Name)
	}

	return fmt.Sprintf("Fake %s stream for %s (%s) · 1080p MKV", contentType, meta.Name, meta.Year)
}

func truncateForLog(value string, max int) string {
	if len(value) <= max {
		return value
	}

	return value[:max] + "... [truncated]"
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
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept, Origin")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dump, err := httputil.DumpRequest(r, false)
		if err != nil {
			log.Printf("request dump error: %v", err)
		} else {
			log.Printf("incoming request:\n%s", dump)
		}

		next.ServeHTTP(w, r)
	})
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
