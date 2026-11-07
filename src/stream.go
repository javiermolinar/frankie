package main

import (
	"context"
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
		log.Printf("prowlarr search results type=%s id=%s count=%d", contentType, id, len(results))
	}

	sortedResults := sortProwlarrResults(results)
	if len(sortedResults) > debridResolveCandidateLimit {
		sortedResults = sortedResults[:debridResolveCandidateLimit]
	}
	log.Printf("debrid candidate results type=%s id=%s count=%d", contentType, id, len(sortedResults))
	if !alldebridConfigured() {
		respondJSON(w, http.StatusOK, StreamResponse{Streams: []Stream{}})
		return
	}

	readyResults := resolveDebridResults(sortedResults)
	if len(readyResults) > streamResponseLimit {
		readyResults = readyResults[:streamResponseLimit]
	}
	log.Printf("ready stream results type=%s id=%s count=%d", contentType, id, len(readyResults))

	respondJSON(w, http.StatusOK, StreamResponse{
		Streams: buildStreams(contentType, readyResults),
	})
}

func resolveDebridResults(sortedResults []ProwlarrSearchResult) []DebridStreamResult {
	if len(sortedResults) == 0 {
		return []DebridStreamResult{}
	}

	failureCounts := map[string]int{}
	var failureMu sync.Mutex
	countFailure := func(reason string) {
		failureMu.Lock()
		failureCounts[reason]++
		failureMu.Unlock()
	}

	// Resolving Prowlarr redirects does not create any AllDebrid state.
	magnetURIs := make([]string, len(sortedResults))
	runDebridWorkers(len(sortedResults), func(index int) {
		magnetURI, err := resolveMagnetURI(sortedResults[index])
		if err != nil {
			countFailure("magnet_resolve")
			log.Printf("resolveMagnetURI error: %v", err)
			return
		}
		magnetURIs[index] = magnetURI
	})

	candidateHashes := make([]string, len(sortedResults))
	uniqueMagnets := make([]string, 0, len(sortedResults))
	seenMagnets := make(map[string]struct{}, len(sortedResults))
	for index, magnetURI := range magnetURIs {
		if magnetURI == "" {
			continue
		}
		hash := magnetInfoHash(magnetURI)
		if hash == "" {
			countFailure("invalid_magnet")
			continue
		}
		candidateHashes[index] = hash
		if _, seen := seenMagnets[hash]; seen {
			continue
		}
		seenMagnets[hash] = struct{}{}
		uniqueMagnets = append(uniqueMagnets, magnetURI)
	}

	// Instant availability is read-only. Never upload a candidate unless AllDebrid
	// says it is already cached; this prevents dead torrents accumulating.
	available := map[string]bool{}
	var instantErr error
	if len(uniqueMagnets) > 0 {
		available, instantErr = checkInstantMagnets(uniqueMagnets)
		if instantErr != nil {
			log.Printf("instant availability check failed: %v", instantErr)
			for range uniqueMagnets {
				countFailure("instant_error")
			}
		}
	}

	eligible := make([]int, 0, len(sortedResults))
	seenUploads := make(map[string]struct{}, len(sortedResults))
	for index, hash := range candidateHashes {
		if hash == "" || instantErr != nil {
			continue
		}
		if _, seen := seenUploads[hash]; seen {
			continue
		}
		seenUploads[hash] = struct{}{}
		if !available[hash] {
			countFailure("not_cached")
			continue
		}
		eligible = append(eligible, index)
	}

	resolved := make([]DebridStreamResult, len(sortedResults))
	runDebridWorkers(len(eligible), func(workerIndex int) {
		index := eligible[workerIndex]
		debridResult, err := getStreamLink(magnetURIs[index])
		if err != nil {
			countFailure(classifyDebridError(err))
			log.Printf("resolve cached magnet error: %v", err)
			return
		}

		debridResult = decorateDebridResult(sortedResults[index], debridResult)
		if debridResult.URL == "" {
			return
		}
		resolved[index] = debridResult
	})

	if len(failureCounts) > 0 {
		total := 0
		for _, count := range failureCounts {
			total += count
		}
		log.Printf("debrid resolve failures total=%d details=%v", total, failureCounts)
	}

	readyResults := make([]DebridStreamResult, 0, len(resolved))
	for _, result := range resolved {
		if strings.TrimSpace(result.URL) == "" {
			continue
		}
		readyResults = append(readyResults, result)
	}

	return readyResults
}

func runDebridWorkers(count int, work func(index int)) {
	workerCount := debridResolveConcurrency
	if count < workerCount {
		workerCount = count
	}
	if workerCount < 1 {
		return
	}

	sem := make(chan struct{}, workerCount)
	var wg sync.WaitGroup
	for index := 0; index < count; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			work(index)
		}()
	}
	wg.Wait()
}

func decorateDebridResult(searchResult ProwlarrSearchResult, debridResult DebridStreamResult) DebridStreamResult {
	debridResult.URL = strings.TrimSpace(debridResult.URL)
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
	debridResult.Indexer = strings.TrimSpace(searchResult.Indexer)
	return debridResult
}

func classifyDebridError(err error) string {
	if err == nil {
		return "unknown"
	}

	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "not ready"):
		return "not_ready"
	case strings.Contains(message, "unexpected status"):
		return "http_error"
	case strings.Contains(message, "upload"):
		return "upload_error"
	case strings.Contains(message, "unlock"):
		return "unlock_error"
	case strings.Contains(message, "no files"):
		return "no_files"
	default:
		return "other"
	}
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

func fetchCinemeta(ctx context.Context, contentType, id string) (CinemetaMeta, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	url := fmt.Sprintf("%s/%s/%s.json", cinemetaBase, contentType, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return CinemetaMeta{}, "", err
	}

	resp, err := doRequest(req)
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
