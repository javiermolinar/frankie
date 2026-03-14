package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
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

type ReleaseInfo struct {
	Resolution string
	Source     string
	Codec      string
	Audio      string
	HDR        bool
	DV         bool
	Remux      bool
}

var (
	resolutionPattern = regexp.MustCompile(`(?i)\b(4320p|2160p|1080p|720p|480p)\b`)
	codecPattern      = regexp.MustCompile(`(?i)\b(x265|x264|h\.?265|h\.?264|hevc|av1)\b`)
	audioPattern      = regexp.MustCompile(`(?i)\b(truehd|atmos|ddp?\+?\s?\d\.\d|dd\s?\d\.\d|dts(?:-hd)?(?:\.ma)?|aac(?:\s?\d\.\d)?)\b`)
	titleCleanPattern = regexp.MustCompile(`[^a-z0-9]+`)
	episodeTagPattern = regexp.MustCompile(`(?i)^s\d{1,2}e\d{1,3}$`)
)

var ignoredTitleTokens = map[string]struct{}{
	"the": {}, "a": {}, "an": {}, "and": {}, "of": {}, "to": {}, "in": {}, "on": {}, "for": {}, "with": {},
	"movie": {}, "proper": {}, "repack": {}, "extended": {}, "internal": {}, "dubbed": {}, "subs": {},
	"x264": {}, "x265": {}, "h264": {}, "h265": {}, "hevc": {}, "av1": {},
	"webrip": {}, "webdl": {}, "web": {}, "bluray": {}, "brrip": {}, "bdrip": {}, "hdtv": {}, "hdrip": {},
	"hdr": {}, "dv": {}, "remux": {}, "10bit": {}, "8bit": {},
	"aac": {}, "dts": {}, "dd": {}, "ddp": {}, "atmos": {},
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

	lookupID := strings.TrimSpace(metadataLookupID(contentType, id))
	var cinemetaMeta CinemetaMeta
	hasCinemeta := false
	if lookupID != "" {
		meta, _, err := fetchCinemeta(contentType, lookupID)
		if err == nil {
			cinemetaMeta = meta
			hasCinemeta = true
		}
	}

	attemptedFallbackFirst := false
	if contentType == "series" && hasCinemeta {
		fallbackQuery := buildProwlarrFallbackQuery(contentType, id, cinemetaMeta)
		if fallbackQuery != "" {
			attemptedFallbackFirst = true
			fallbackResults, fallbackRaw, err := runProwlarrSearch(fallbackQuery, "tvsearch", 20)
			if err == nil {
				fallbackResults = filterResultsByTitleSimilarity(fallbackResults, cinemetaMeta.Name)
				if len(fallbackResults) > 0 {
					return fallbackResults, fallbackRaw, nil
				}
			}
		}
	}

	results, raw, err := runProwlarrSearch(query, searchType, 20)
	if err != nil {
		return nil, raw, err
	}
	if hasCinemeta {
		results = filterResultsByTitleSimilarity(results, cinemetaMeta.Name)
	}
	if len(results) > 0 {
		return results, raw, nil
	}

	if !hasCinemeta || attemptedFallbackFirst {
		return results, raw, nil
	}

	fallbackQuery := buildProwlarrFallbackQuery(contentType, id, cinemetaMeta)
	if fallbackQuery == "" {
		return results, raw, nil
	}

	fallbackSearchType := ""
	if contentType == "series" {
		fallbackSearchType = "tvsearch"
	}

	fallbackResults, fallbackRaw, err := runProwlarrSearch(fallbackQuery, fallbackSearchType, 20)
	if err != nil {
		return results, raw, nil
	}

	fallbackResults = filterResultsByTitleSimilarity(fallbackResults, cinemetaMeta.Name)
	return fallbackResults, fallbackRaw, nil
}

func runProwlarrSearch(query, searchType string, limit int) ([]ProwlarrSearchResult, string, error) {
	baseURL, err := url.Parse(config.ProwlarrURL)
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

func filterResultsByTitleSimilarity(results []ProwlarrSearchResult, expectedTitle string) []ProwlarrSearchResult {
	expectedTokens := tokenizeTitle(expectedTitle)
	if len(expectedTokens) == 0 {
		return results
	}

	filtered := make([]ProwlarrSearchResult, 0, len(results))
	for _, result := range results {
		if titleSimilarity(expectedTokens, result.Title) >= 0.6 {
			filtered = append(filtered, result)
		}
	}

	return filtered
}

func titleSimilarity(expectedTokens []string, candidate string) float64 {
	candidateTokens := tokenizeTitle(candidate)
	if len(candidateTokens) == 0 || len(expectedTokens) == 0 {
		return 0
	}

	expectedCompact := strings.Join(expectedTokens, "")
	candidateCompact := strings.Join(candidateTokens, "")
	if expectedCompact != "" && (strings.Contains(candidateCompact, expectedCompact) || strings.Contains(expectedCompact, candidateCompact)) {
		return 1
	}

	candidateSet := make(map[string]struct{}, len(candidateTokens))
	for _, token := range candidateTokens {
		candidateSet[token] = struct{}{}
	}

	matches := 0
	for _, token := range expectedTokens {
		if _, ok := candidateSet[token]; ok {
			matches++
		}
	}

	return float64(matches) / float64(len(expectedTokens))
}

func tokenizeTitle(value string) []string {
	clean := strings.ToLower(strings.TrimSpace(value))
	if clean == "" {
		return nil
	}

	clean = titleCleanPattern.ReplaceAllString(clean, " ")
	rawTokens := strings.Fields(clean)
	if len(rawTokens) == 0 {
		return nil
	}

	tokens := make([]string, 0, len(rawTokens))
	for _, token := range rawTokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if _, ignored := ignoredTitleTokens[token]; ignored {
			continue
		}
		if episodeTagPattern.MatchString(token) {
			continue
		}
		if strings.HasSuffix(token, "p") && len(token) >= 4 {
			digits := strings.TrimSuffix(token, "p")
			if _, err := strconv.Atoi(digits); err == nil {
				continue
			}
		}
		tokens = append(tokens, token)
	}

	return tokens
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

	maxResults := min(len(results), 10)
	streams := make([]Stream, 0, maxResults)

	for i := range maxResults {
		result := results[i]
		if strings.TrimSpace(result.URL) == "" {
			continue
		}

		name := "⚡ Alldebrid"
		descriptionParts := []string{}
		if result.Filename != "" {
			descriptionParts = append(descriptionParts, "🧾 "+result.Filename)
		}
		if result.Size > 0 {
			descriptionParts = append(descriptionParts, "💾 "+humanSize(result.Size))
		}
		if result.Host != "" {
			descriptionParts = append(descriptionParts, "🌐 "+result.Host)
		}

		streams = append(streams, Stream{
			Name:        name,
			Description: strings.Join(descriptionParts, " · "),
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
		return ""
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
		leftInfo := parseReleaseInfo(sorted[i].Title)
		rightInfo := parseReleaseInfo(sorted[j].Title)

		leftQuality := qualityScore(leftInfo)
		rightQuality := qualityScore(rightInfo)
		if leftQuality != rightQuality {
			return leftQuality > rightQuality
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

func qualityScore(info ReleaseInfo) int {
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
	return config.ProwlarrURL != "" && config.ProwlarrAPIKey != ""
}
