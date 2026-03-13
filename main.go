package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port           string
	ProwlarrURL    string
	ProwlarrAPIKey string
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

type ProwlarrSearchResult struct {
	ID               int               `json:"id"`
	Guid             string            `json:"guid"`
	Age              float64           `json:"age"`
	AgeHours         float64           `json:"ageHours"`
	AgeMinutes       float64           `json:"ageMinutes"`
	Size             int64             `json:"size"`
	Files            int               `json:"files"`
	Grabs            int               `json:"grabs"`
	IndexerID        int               `json:"indexerId"`
	Indexer          string            `json:"indexer"`
	SubGroup         string            `json:"subGroup"`
	ReleaseHash      string            `json:"releaseHash"`
	Title            string            `json:"title"`
	SortTitle        string            `json:"sortTitle"`
	IMDbID           int               `json:"imdbId"`
	TMDbID           int               `json:"tmdbId"`
	TVDbID           int               `json:"tvdbId"`
	TVMazeID         int               `json:"tvMazeId"`
	PublishDate      string            `json:"publishDate"`
	CommentURL       string            `json:"commentUrl"`
	DownloadURL      string            `json:"downloadUrl"`
	InfoURL          string            `json:"infoUrl"`
	PosterURL        string            `json:"posterUrl"`
	IndexerFlags     []string          `json:"indexerFlags"`
	Categories       []json.RawMessage `json:"categories"`
	MagnetURL        string            `json:"magnetUrl"`
	InfoHash         string            `json:"infoHash"`
	Seeders          int               `json:"seeders"`
	Leechers         int               `json:"leechers"`
	Protocol         string            `json:"protocol"`
	FileName         string            `json:"fileName"`
	DownloadClientID int               `json:"downloadClientId"`
}

type ProwlarrCategory struct {
	ID            int      `json:"id"`
	Name          string   `json:"name"`
	Description   string   `json:"description"`
	SubCategories []string `json:"subCategories"`
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

var (
	httpClient = &http.Client{Timeout: 10 * time.Second}
	config     = loadConfig()

	resolutionPattern = regexp.MustCompile(`(?i)\b(4320p|2160p|1080p|720p|480p)\b`)
	codecPattern      = regexp.MustCompile(`(?i)\b(x265|x264|h\.?265|h\.?264|hevc|av1)\b`)
	audioPattern      = regexp.MustCompile(`(?i)\b(truehd|atmos|ddp?\+?\s?\d\.\d|dd\s?\d\.\d|dts(?:-hd)?(?:\.ma)?|aac(?:\s?\d\.\d)?)\b`)
)

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", homeHandler)
	mux.HandleFunc("/manifest.json", manifestHandler)
	mux.HandleFunc("/stream/", streamHandler)
	mux.HandleFunc("/debug/prowlarr/search", prowlarrDebugSearchHandler)

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
	meta, _, _ := fetchCinemeta(contentType, lookupID)

	var results []ProwlarrSearchResult

	query := buildProwlarrQuery(contentType, id, meta)
	if query != "" && prowlarrConfigured() {
		searchResults, _, err := searchProwlarr(query)
		if err == nil {
			results = searchResults
			logProwlarrLinks(results)
		}
	}

	respondJSON(w, http.StatusOK, StreamResponse{
		Streams: buildStreams(contentType, id, meta, results),
	})
}

func prowlarrDebugSearchHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !prowlarrConfigured() {
		http.Error(w, "missing PROWLARR_URL or PROWLARR_API_KEY", http.StatusServiceUnavailable)
		return
	}

	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		http.Error(w, "missing q", http.StatusBadRequest)
		return
	}

	results, _, err := searchProwlarr(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"query":   query,
		"count":   len(results),
		"results": results,
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

func parseSeriesRequest(id string) (SeriesRequest, bool) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return SeriesRequest{}, false
	}

	season, err := strconv.Atoi(parts[1])
	if err != nil {
		return SeriesRequest{}, false
	}

	episode, err := strconv.Atoi(parts[2])
	if err != nil {
		return SeriesRequest{}, false
	}

	return SeriesRequest{
		IMDbID:  parts[0],
		Season:  season,
		Episode: episode,
	}, true
}

func buildProwlarrQuery(contentType, id string, meta CinemetaMeta) string {
	name := strings.TrimSpace(meta.Name)
	if name == "" {
		return ""
	}

	switch contentType {
	case "movie":
		year := extractStartYear(meta.Year)
		if year == "" {
			return name
		}
		return name + " " + year
	case "series":
		series, ok := parseSeriesRequest(id)
		if !ok {
			return name
		}
		return fmt.Sprintf("%s S%02dE%02d", name, series.Season, series.Episode)
	default:
		return name
	}
}

func extractStartYear(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 4 {
		return ""
	}
	return value[:4]
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

func searchProwlarr(query string) ([]ProwlarrSearchResult, string, error) {
	baseURL, err := url.Parse(config.ProwlarrURL)
	if err != nil {
		return nil, "", fmt.Errorf("invalid PROWLARR_URL: %w", err)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/api/v1/search"
	queryValues := baseURL.Query()
	queryValues.Set("query", query)
	baseURL.RawQuery = queryValues.Encode()

	req, err := http.NewRequest(http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("X-Api-Key", config.ProwlarrAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	raw := string(body)
	if resp.StatusCode != http.StatusOK {
		return nil, raw, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var results []ProwlarrSearchResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, raw, err
	}

	return results, raw, nil
}

type ReleaseInfo struct {
	Resolution string
	Source     string
	Codec      string
	Audio      string
	HDR        bool
	DV         bool
	Remux      bool
}

func buildStreams(contentType, id string, meta CinemetaMeta, results []ProwlarrSearchResult) []Stream {
	if len(results) == 0 {
		return []Stream{{
			Name:        buildFallbackName(contentType),
			Description: buildFallbackDescription(contentType, id, meta),
			URL:         mockVideoURL,
			BehaviorHints: &BehaviorHints{
				NotWebReady: true,
				Filename:    fallbackFilename(contentType, id, meta),
				BingeGroup:  buildBingeGroup(contentType, "mock-1080p"),
			},
		}}
	}

	sortedResults := sortProwlarrResults(results)

	maxResults := 10
	if len(sortedResults) < maxResults {
		maxResults = len(sortedResults)
	}

	streams := make([]Stream, 0, maxResults)
	for i := 0; i < maxResults; i++ {
		result := sortedResults[i]
		info := parseReleaseInfo(result.Title)
		qualityLabel := buildQualityLabel(contentType, info)
		streams = append(streams, Stream{
			Name:        qualityLabel,
			Description: buildProwlarrDescription(result, info),
			URL:         buildFakeStreamURL(result),
			BehaviorHints: &BehaviorHints{
				NotWebReady: true,
				Filename:    streamFilename(result),
				VideoSize:   result.Size,
				BingeGroup:  buildBingeGroup(contentType, qualityLabel),
			},
		})
	}

	return streams
}

func buildFallbackName(contentType string) string {
	emoji := "🎬"
	if contentType == "series" {
		emoji = "📺"
	}
	return emoji + " 1080P MKV"
}

func buildFallbackDescription(contentType, id string, meta CinemetaMeta) string {
	if meta.Name == "" {
		return fmt.Sprintf("🧪 Mock %s stream for %s", contentType, id)
	}
	if meta.Year == "" {
		return fmt.Sprintf("🧪 Mock %s stream for %s", contentType, meta.Name)
	}
	return fmt.Sprintf("🧪 Mock %s stream for %s (%s)", contentType, meta.Name, meta.Year)
}

func buildProwlarrDescription(result ProwlarrSearchResult, info ReleaseInfo) string {
	parts := []string{}
	if result.Title != "" {
		parts = append(parts, "🧾 "+result.Title)
	}
	if result.Indexer != "" {
		parts = append(parts, "🌐 "+result.Indexer)
	}
	if info.Audio != "" {
		parts = append(parts, "🔊 "+info.Audio)
	}
	if result.Size > 0 {
		parts = append(parts, "💾 "+humanSize(result.Size))
	}
	if result.Seeders > 0 {
		parts = append(parts, fmt.Sprintf("👥 %d", result.Seeders))
	}
	if result.Protocol != "" && result.Protocol != "unknown" {
		parts = append(parts, "📦 "+strings.ToUpper(result.Protocol))
	}
	if len(parts) == 0 {
		return "🧪 Mock result"
	}
	return strings.Join(parts, " · ")
}

func buildQualityLabel(contentType string, info ReleaseInfo) string {
	emoji := "🎬"
	if contentType == "series" {
		emoji = "📺"
	}

	parts := []string{emoji}
	if info.Resolution != "" {
		parts = append(parts, info.Resolution)
	} else {
		parts = append(parts, "AUTO")
	}
	if info.Source != "" {
		parts = append(parts, info.Source)
	}
	if info.Remux {
		parts = append(parts, "REMUX")
	}
	if info.HDR {
		parts = append(parts, "HDR")
	}
	if info.DV {
		parts = append(parts, "DV")
	}
	if info.Codec != "" {
		parts = append(parts, info.Codec)
	}

	return strings.Join(parts, " ")
}

func parseReleaseInfo(title string) ReleaseInfo {
	info := ReleaseInfo{
		Resolution: strings.ToUpper(matchFirst(resolutionPattern, title)),
		Source:     detectVisualTag(title),
		Codec:      normalizeCodec(matchFirst(codecPattern, title)),
		Audio:      normalizeAudio(matchFirst(audioPattern, title)),
	}

	upper := strings.ToUpper(title)
	info.HDR = strings.Contains(upper, " HDR ") || strings.Contains(upper, ".HDR.") || strings.Contains(upper, "HDR10") || strings.Contains(upper, "HDR10+")
	info.DV = strings.Contains(upper, "DOLBY VISION") || strings.Contains(upper, ".DV.") || strings.Contains(upper, " DV ")
	info.Remux = strings.Contains(upper, "REMUX")

	return info
}

func detectVisualTag(title string) string {
	upper := strings.ToUpper(title)
	switch {
	case strings.Contains(upper, "WEB-DL"):
		return "WEB-DL"
	case strings.Contains(upper, "WEBRIP") || strings.Contains(upper, "WEB-RIP"):
		return "WEBRIP"
	case strings.Contains(upper, "REMUX") && (strings.Contains(upper, "BLURAY") || strings.Contains(upper, "BDREMUX") || strings.Contains(upper, "BDMV")):
		return "BLURAY"
	case strings.Contains(upper, "BLURAY") || strings.Contains(upper, "BDRIP") || strings.Contains(upper, "BRRIP"):
		return "BLURAY"
	case strings.Contains(upper, "HDRIP"):
		return "HDRIP"
	case strings.Contains(upper, "HDTV"):
		return "HDTV"
	default:
		return ""
	}
}

func matchFirst(pattern *regexp.Regexp, value string) string {
	match := pattern.FindString(value)
	return strings.TrimSpace(match)
}

func normalizeCodec(value string) string {
	upper := strings.ToUpper(strings.ReplaceAll(strings.ReplaceAll(value, ".", ""), " ", ""))
	switch upper {
	case "X265", "H265", "HEVC":
		return "H265"
	case "X264", "H264":
		return "H264"
	case "AV1":
		return "AV1"
	default:
		return upper
	}
}

func normalizeAudio(value string) string {
	upper := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	switch {
	case strings.Contains(upper, "TRUEHD"):
		return "TRUEHD"
	case strings.Contains(upper, "ATMOS"):
		return "ATMOS"
	case strings.HasPrefix(upper, "DDP"):
		return strings.ReplaceAll(upper, "+", "")
	case strings.HasPrefix(upper, "DD"):
		return upper
	case strings.Contains(upper, "DTS-HD"):
		return "DTS-HD"
	case strings.Contains(upper, "DTS"):
		return "DTS"
	case strings.Contains(upper, "AAC"):
		return upper
	default:
		return upper
	}
}

func sortProwlarrResults(results []ProwlarrSearchResult) []ProwlarrSearchResult {
	sorted := append([]ProwlarrSearchResult(nil), results...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := scoreResult(sorted[i])
		right := scoreResult(sorted[j])
		if left != right {
			return left > right
		}
		if sorted[i].Seeders != sorted[j].Seeders {
			return sorted[i].Seeders > sorted[j].Seeders
		}
		if sorted[i].Size != sorted[j].Size {
			return sorted[i].Size > sorted[j].Size
		}
		return sorted[i].Title < sorted[j].Title
	})
	return sorted
}

func scoreResult(result ProwlarrSearchResult) int {
	info := parseReleaseInfo(result.Title)
	score := 0
	score += resolutionScore(info.Resolution)
	score += sourceScore(info.Source)
	score += codecScore(info.Codec)
	if info.HDR {
		score += 20
	}
	if info.DV {
		score += 15
	}
	if info.Remux {
		score += 25
	}
	if result.Seeders > 0 {
		score += min(result.Seeders, 100)
	}
	if result.Size > 0 {
		score += min(int(result.Size/(1024*1024*1024)), 20)
	}
	return score
}

func resolutionScore(value string) int {
	switch value {
	case "4320P":
		return 120
	case "2160P":
		return 100
	case "1080P":
		return 80
	case "720P":
		return 60
	case "480P":
		return 40
	default:
		return 10
	}
}

func sourceScore(value string) int {
	switch value {
	case "BLURAY":
		return 35
	case "WEB-DL":
		return 30
	case "WEBRIP":
		return 25
	case "HDTV":
		return 15
	case "HDRIP":
		return 10
	default:
		return 0
	}
}

func codecScore(value string) int {
	switch value {
	case "AV1":
		return 20
	case "H265":
		return 15
	case "H264":
		return 10
	default:
		return 0
	}
}

func buildFakeStreamURL(result ProwlarrSearchResult) string {
	identifier := result.Guid
	if identifier == "" {
		identifier = strconv.Itoa(result.ID)
	}
	if identifier == "" {
		identifier = "unknown"
	}

	return fmt.Sprintf("https://cdn.frankie.local/stream/%s/%s", url.PathEscape(identifier), url.PathEscape(streamFilename(result)))
}

func streamFilename(result ProwlarrSearchResult) string {
	fileName := result.FileName
	if fileName == "" {
		fileName = result.Title
	}
	fileName = strings.TrimSpace(fileName)
	if fileName == "" {
		fileName = "video"
	}
	fileName = strings.ReplaceAll(fileName, " ", ".")
	if !strings.HasSuffix(strings.ToLower(fileName), ".mkv") {
		fileName += ".mkv"
	}
	return fileName
}

func fallbackFilename(contentType, id string, meta CinemetaMeta) string {
	name := meta.Name
	if name == "" {
		name = contentType + "." + id
	}
	name = strings.ReplaceAll(strings.TrimSpace(name), " ", ".")
	if meta.Year != "" {
		name += "." + extractStartYear(meta.Year)
	}
	return name + ".mkv"
}

func buildBingeGroup(contentType, label string) string {
	label = strings.ToLower(strings.TrimSpace(label))
	replacer := strings.NewReplacer(" ", "-", "🎬", "", "📺", "", ".", "-", "_", "-")
	label = replacer.Replace(label)
	label = strings.Trim(label, "-")
	if label == "" {
		label = "default"
	}
	return "frankie-" + contentType + "-" + label
}

func humanSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(size)/float64(div), "KMGTPE"[exp])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func logProwlarrLinks(results []ProwlarrSearchResult) {
	for _, result := range results {
		if result.Guid != "" {
			log.Println(result.Guid)
		}
	}
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
		Port:           envOrDefault("PORT", "3593"),
		ProwlarrURL:    strings.TrimRight(strings.TrimSpace(os.Getenv("PROWLARR_URL")), "/"),
		ProwlarrAPIKey: strings.TrimSpace(os.Getenv("PROWLARR_API_KEY")),
	}
}

func prowlarrConfigured() bool {
	return config.ProwlarrURL != "" && config.ProwlarrAPIKey != ""
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
