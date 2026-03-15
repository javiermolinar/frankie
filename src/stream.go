package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
)

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
	var results []ProwlarrSearchResult

	if prowlarrConfigured() {
		searchResults, _, err := searchProwlarr(contentType, id)
		if err != nil {
			log.Printf("prowlarr search failed type=%s id=%s err=%v", contentType, id, err)
			respondJSON(w, http.StatusOK, StreamResponse{Streams: []Stream{}})
			return
		}
		results = searchResults
	}

	sortedResults := sortProwlarrResults(results)
	if len(sortedResults) > 10 {
		sortedResults = sortedResults[:10]
	}
	if !alldebridConfigured() {
		respondJSON(w, http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	readyResults := resolveDebridResults(sortedResults)

	respondJSON(w, http.StatusOK, StreamResponse{
		Streams: buildStreams(contentType, readyResults),
	})
}

func resolveDebridResults(sortedResults []ProwlarrSearchResult) []DebridStreamResult {
	if len(sortedResults) == 0 {
		return []DebridStreamResult{}
	}

	workerCount := debridResolveConcurrency
	if len(sortedResults) < workerCount {
		workerCount = len(sortedResults)
	}
	if workerCount < 1 {
		return []DebridStreamResult{}
	}

	sem := make(chan struct{}, workerCount)
	resolved := make([]DebridStreamResult, len(sortedResults))

	var wg sync.WaitGroup
	for i, sr := range sortedResults {
		wg.Add(1)
		go func(index int, searchResult ProwlarrSearchResult) {
			defer wg.Done()

			sem <- struct{}{}
			defer func() { <-sem }()

			magnetURI, err := resolveMagnetURI(searchResult)
			if err != nil {
				log.Printf("resolveMagnetURI error: %v", err)
				return
			}

			debridResult, err := getStreamLink(magnetURI)
			if err != nil {
				return
			}

			debridResult.URL = strings.TrimSpace(debridResult.URL)
			if debridResult.URL == "" {
				return
			}
			if debridResult.Filename == "" {
				debridResult.Filename = streamFilename(searchResult)
			}
			if debridResult.Size <= 0 {
				debridResult.Size = searchResult.Size
			}
			if debridResult.Host == "" {
				debridResult.Host = "alldebrid"
			}
			debridResult.Language = extractLanguageFromTitle(searchResult.Title)
			if debridResult.Language == "" {
				debridResult.Language = extractLanguageFromTitle(debridResult.Filename)
			}

			resolved[index] = debridResult
		}(i, sr)
	}

	wg.Wait()

	readyResults := make([]DebridStreamResult, 0, len(resolved))
	for _, result := range resolved {
		if strings.TrimSpace(result.URL) == "" {
			continue
		}
		readyResults = append(readyResults, result)
	}

	return readyResults
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
