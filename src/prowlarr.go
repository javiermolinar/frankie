package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
)

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

var languageMatchers = []struct {
	Label  string
	Tokens []string
}{
	{Label: "English", Tokens: []string{"ENGLISH", "ENG"}},
	{Label: "Spanish", Tokens: []string{"SPANISH", "ESP", "ESPANOL", "CASTELLANO", "LATINO"}},
	{Label: "French", Tokens: []string{"FRENCH", "VFF", "VFQ"}},
	{Label: "Italian", Tokens: []string{"ITALIAN", "ITA"}},
	{Label: "German", Tokens: []string{"GERMAN", "GER", "DEUTSCH"}},
	{Label: "Portuguese", Tokens: []string{"PORTUGUESE", "POR", "PTBR", "BRAZILIAN"}},
	{Label: "Hindi", Tokens: []string{"HINDI"}},
	{Label: "Japanese", Tokens: []string{"JAPANESE", "JAP", "JP"}},
	{Label: "Korean", Tokens: []string{"KOREAN", "KOR"}},
	{Label: "Chinese", Tokens: []string{"CHINESE", "MANDARIN", "CANTONESE"}},
	{Label: "Russian", Tokens: []string{"RUSSIAN", "RUS"}},
	{Label: "Arabic", Tokens: []string{"ARABIC", "ARA"}},
	{Label: "Turkish", Tokens: []string{"TURKISH", "TUR"}},
}

func languageOptions() []string {
	options := make([]string, 0, len(languageMatchers))
	for _, matcher := range languageMatchers {
		options = append(options, matcher.Label)
	}
	return options
}

func canonicalLanguageLabel(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "english":
		return "English"
	case "spanish":
		return "Spanish"
	case "french":
		return "French"
	case "italian":
		return "Italian"
	case "german":
		return "German"
	case "portuguese":
		return "Portuguese"
	case "hindi":
		return "Hindi"
	case "japanese":
		return "Japanese"
	case "korean":
		return "Korean"
	case "chinese":
		return "Chinese"
	case "russian":
		return "Russian"
	case "arabic":
		return "Arabic"
	case "turkish":
		return "Turkish"
	default:
		return ""
	}
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

func buildProwlarrQuery(contentType, id string) (query, searchType string, ok bool) {
	lookupID := strings.TrimSpace(metadataLookupID(contentType, id))
	if lookupID == "" {
		return "", "", false
	}

	switch contentType {
	case "movie":
		searchType = "movie"
	case "series":
		searchType = "tvsearch"
	default:
		return "", "", false
	}

	return fmt.Sprintf("{ImdbId:%s}", lookupID), searchType, true
}

func buildProwlarrFallbackQuery(contentType, id string, meta CinemetaMeta) string {
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

func searchProwlarr(contentType, id string) ([]ProwlarrSearchResult, string, error) {
	query, searchType, ok := buildProwlarrQuery(contentType, id)
	if !ok {
		return nil, "", fmt.Errorf("cannot build prowlarr query")
	}

	type searchOutcome struct {
		attempted bool
		results   []ProwlarrSearchResult
		raw       string
		err       error
	}

	var primary searchOutcome
	var fallback searchOutcome

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		primary.attempted = true
		primary.results, primary.raw, primary.err = runProwlarrSearch(query, searchType, prowlarrSearchLimit)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()

		lookupID := strings.TrimSpace(metadataLookupID(contentType, id))
		if lookupID == "" {
			return
		}

		meta, _, err := fetchCinemeta(contentType, lookupID)
		if err != nil {
			return
		}

		fallbackQuery := buildProwlarrFallbackQuery(contentType, id, meta)
		if fallbackQuery == "" {
			return
		}

		fallbackSearchType := ""
		if contentType == "series" {
			fallbackSearchType = "tvsearch"
		}

		fallback.attempted = true
		fallback.results, fallback.raw, fallback.err = runProwlarrSearch(fallbackQuery, fallbackSearchType, prowlarrSearchLimit)
	}()

	wg.Wait()

	if primary.err != nil {
		if fallback.attempted && fallback.err == nil {
			return dedupeProwlarrResults(fallback.results), fallback.raw, nil
		}
		return nil, primary.raw, primary.err
	}

	merged := append([]ProwlarrSearchResult{}, primary.results...)
	if fallback.attempted && fallback.err == nil && len(fallback.results) > 0 {
		merged = append(merged, fallback.results...)
	}

	merged = dedupeProwlarrResults(merged)
	raw := strings.TrimSpace(primary.raw)
	if strings.TrimSpace(fallback.raw) != "" {
		if raw == "" {
			raw = fallback.raw
		} else {
			raw = raw + "\n" + fallback.raw
		}
	}

	return merged, raw, nil
}

func runProwlarrSearch(query, searchType string, limit int) ([]ProwlarrSearchResult, string, error) {
	cfg := getConfig()
	baseURL, err := url.Parse(cfg.ProwlarrURL)
	if err != nil {
		return nil, "", fmt.Errorf("invalid PROWLARR_URL: %w", err)
	}

	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/api/v1/search"
	queryValues := baseURL.Query()
	queryValues.Set("query", query)
	if searchType != "" {
		queryValues.Set("type", searchType)
	}
	if limit > 0 {
		queryValues.Set("limit", strconv.Itoa(limit))
	}
	baseURL.RawQuery = queryValues.Encode()

	req, err := http.NewRequest(http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("X-Api-Key", cfg.ProwlarrAPIKey)
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

func resolveMagnetURI(result ProwlarrSearchResult) (string, error) {
	candidates := []string{
		strings.TrimSpace(result.MagnetURL),
		strings.TrimSpace(result.Guid),
	}

	var lastErr error
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}

		magnetURI, err := extractMagnetURI(candidate)
		if err == nil {
			return magnetURI, nil
		}
		lastErr = err
	}

	if lastErr != nil {
		return "", lastErr
	}

	return "", fmt.Errorf("no magnet url or guid in search result")
}

func extractMagnetURI(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("empty magnet value")
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return "", fmt.Errorf("invalid magnet value: %w", err)
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme == "magnet" {
		return value, nil
	}
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", parsed.Scheme)
	}

	return resolveMagnetFromRedirect(value, 6)
}

func resolveMagnetFromRedirect(startURL string, maxHops int) (string, error) {
	client := &http.Client{
		Timeout: httpClient.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	current := startURL
	for hop := 0; hop < maxHops; hop++ {
		req, err := http.NewRequest(http.MethodGet, current, nil)
		if err != nil {
			return "", err
		}
		resp, err := client.Do(req)
		if err != nil {
			return "", err
		}

		location := strings.TrimSpace(resp.Header.Get("Location"))
		resp.Body.Close()
		if location == "" {
			return "", fmt.Errorf("missing redirect location from %s (status=%d)", current, resp.StatusCode)
		}

		currentParsed, err := url.Parse(current)
		if err != nil {
			return "", err
		}
		locationParsed, err := url.Parse(location)
		if err != nil {
			return "", fmt.Errorf("invalid redirect location %q: %w", location, err)
		}

		next := location
		if !locationParsed.IsAbs() {
			next = currentParsed.ResolveReference(locationParsed).String()
			locationParsed, _ = url.Parse(next)
		}

		nextScheme := strings.ToLower(locationParsed.Scheme)
		switch nextScheme {
		case "magnet":
			return next, nil
		case "http", "https":
			current = next
		default:
			return "", fmt.Errorf("unsupported redirect protocol %q", locationParsed.Scheme)
		}
	}

	return "", fmt.Errorf("too many redirects resolving magnet url")
}

func buildStreams(contentType string, results []DebridStreamResult) []Stream {
	if len(results) == 0 {
		return []Stream{}
	}

	streams := make([]Stream, 0, len(results))

	for _, result := range results {
		if strings.TrimSpace(result.URL) == "" {
			continue
		}

		quality := extractResolutionFromText(result.Filename)
		name := "⚡Frankie"
		if quality != "" {
			name += "\n" + quality
		}

		descriptionParts := []string{}
		if result.Filename != "" {
			descriptionParts = append(descriptionParts, result.Filename)
		}
		if result.Size > 0 {
			descriptionParts = append(descriptionParts, "💾 "+humanSize(result.Size))
		}
		if result.Language != "" {
			descriptionParts = append(descriptionParts, "🗣️ "+result.Language)
		}
		if result.Host != "" {
			descriptionParts = append(descriptionParts, "🌐 "+result.Host)
		}

		streams = append(streams, Stream{
			Name:        name,
			Description: strings.Join(descriptionParts, "\n"),
			URL:         result.URL,
			BehaviorHints: &BehaviorHints{
				NotWebReady: true,
				Filename:    result.Filename,
				VideoSize:   result.Size,
				BingeGroup:  "frankie-" + contentType + "-alldebrid",
			},
		})
	}

	return streams
}

func dedupeProwlarrResults(results []ProwlarrSearchResult) []ProwlarrSearchResult {
	if len(results) == 0 {
		return []ProwlarrSearchResult{}
	}

	seen := make(map[string]struct{}, len(results))
	deduped := make([]ProwlarrSearchResult, 0, len(results))
	for _, result := range results {
		key := prowlarrResultKey(result)
		if key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		deduped = append(deduped, result)
	}

	return deduped
}

func prowlarrResultKey(result ProwlarrSearchResult) string {
	if infoHash := strings.ToLower(strings.TrimSpace(result.InfoHash)); infoHash != "" {
		return "infohash:" + infoHash
	}
	if magnetURL := strings.TrimSpace(result.MagnetURL); magnetURL != "" {
		return "magnet:" + magnetURL
	}
	if downloadURL := strings.TrimSpace(result.DownloadURL); downloadURL != "" {
		return "download:" + downloadURL
	}
	if guid := strings.TrimSpace(result.Guid); guid != "" {
		return "guid:" + guid
	}
	return ""
}

func prowlarrQualityScore(result ProwlarrSearchResult) int {
	resolution := extractResolutionFromText(result.Title + " " + result.FileName)
	switch resolution {
	case "4320p":
		return 7
	case "2160p":
		return 6
	case "1440p":
		return 5
	case "1080p":
		return 4
	case "720p":
		return 3
	case "576p":
		return 2
	case "480p":
		return 1
	default:
		return 0
	}
}

func sortProwlarrResults(results []ProwlarrSearchResult) []ProwlarrSearchResult {
	sorted := append([]ProwlarrSearchResult(nil), results...)
	cfg := getConfig()
	sort.SliceStable(sorted, func(i, j int) bool {
		leftQuality := prowlarrQualityScore(sorted[i])
		rightQuality := prowlarrQualityScore(sorted[j])
		if leftQuality != rightQuality {
			return leftQuality > rightQuality
		}

		leftLanguage := prowlarrLanguagePreferenceScore(sorted[i], cfg)
		rightLanguage := prowlarrLanguagePreferenceScore(sorted[j], cfg)
		if leftLanguage != rightLanguage {
			return leftLanguage > rightLanguage
		}

		if sorted[i].Size != sorted[j].Size {
			return sorted[i].Size > sorted[j].Size
		}
		if sorted[i].Seeders != sorted[j].Seeders {
			return sorted[i].Seeders > sorted[j].Seeders
		}
		return sorted[i].Title < sorted[j].Title
	})
	return sorted
}

func releaseTokenSet(value string) map[string]struct{} {
	tokens := tokenizeReleaseText(value)
	if len(tokens) == 0 {
		return nil
	}
	set := make(map[string]struct{}, len(tokens))
	for _, token := range tokens {
		set[token] = struct{}{}
	}
	return set
}

func detectedLanguagesFromTokenSet(tokenSet map[string]struct{}) []string {
	if len(tokenSet) == 0 {
		return nil
	}
	languages := make([]string, 0, 3)
	for _, matcher := range languageMatchers {
		for _, token := range matcher.Tokens {
			if _, ok := tokenSet[token]; ok {
				languages = append(languages, matcher.Label)
				break
			}
		}
	}
	return languages
}

func prowlarrLanguagePreferenceScore(result ProwlarrSearchResult, cfg Config) int {
	primary := canonicalLanguageLabel(cfg.PrimaryLanguage)
	secondary := canonicalLanguageLabel(cfg.SecondaryLanguage)
	if primary == "" && secondary == "" {
		return 0
	}

	tokenSet := releaseTokenSet(result.Title + " " + result.FileName)
	if len(tokenSet) == 0 {
		return 0
	}

	detected := detectedLanguagesFromTokenSet(tokenSet)
	if len(detected) == 0 {
		return 0
	}

	hasLanguage := func(target string) bool {
		for _, language := range detected {
			if language == target {
				return true
			}
		}
		return false
	}

	if primary != "" && hasLanguage(primary) {
		return 2
	}
	if secondary != "" && hasLanguage(secondary) {
		return 1
	}
	return 0
}

func extractLanguageFromTitle(title string) string {
	tokenSet := releaseTokenSet(title)
	if len(tokenSet) == 0 {
		return ""
	}

	languages := detectedLanguagesFromTokenSet(tokenSet)
	if len(languages) > 0 {
		return strings.Join(languages, ", ")
	}
	if _, ok := tokenSet["MULTI"]; ok {
		return "Multi Audio"
	}
	if _, ok := tokenSet["MULTIAUDIO"]; ok {
		return "Multi Audio"
	}
	if _, ok := tokenSet["DUAL"]; ok {
		return "Dual Audio"
	}
	if _, ok := tokenSet["DUALAUDIO"]; ok {
		return "Dual Audio"
	}

	return ""
}

func extractResolutionFromText(value string) string {
	tokens := tokenizeReleaseText(value)
	for _, token := range tokens {
		switch token {
		case "4320P", "2160P", "1440P", "1080P", "720P", "576P", "480P":
			return strings.ToLower(token)
		case "4K", "UHD":
			return "2160p"
		}
	}
	return ""
}

func tokenizeReleaseText(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	replacer := strings.NewReplacer(
		".", " ", "_", " ", "-", " ",
		"[", " ", "]", " ", "(", " ", ")", " ",
		"{", " ", "}", " ", "/", " ", "\\", " ",
		",", " ", ";", " ", ":", " ", "|", " ",
		"+", " ", "&", " ",
	)
	clean := strings.ToUpper(replacer.Replace(value))
	return strings.Fields(clean)
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

func extractStartYear(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 4 {
		return ""
	}
	return value[:4]
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

func prowlarrConfigured() bool {
	cfg := getConfig()
	return cfg.ProwlarrURL != "" && cfg.ProwlarrAPIKey != ""
}
