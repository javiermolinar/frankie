package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Port            string `json:"port,omitempty"`
	ProwlarrURL     string `json:"prowlarr_url,omitempty"`
	ProwlarrAPIKey  string `json:"prowlarr_api_key,omitempty"`
	AlldebridAPIKey string `json:"alldebrid_api_key,omitempty"`
	PublicURL       string `json:"public_url,omitempty"`
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
	cinemetaBase             = "https://v3-cinemeta.strem.io/meta"
	debridResolveConcurrency = 4
	defaultPort              = "3593"
	defaultConfigPath        = "config.json"
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

var configurePageTemplate = template.Must(template.New("configure").Parse(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Frankie configuration</title>
  <style>
    :root {
      color-scheme: dark;
      --bg: #0d1117;
      --card: #161b22;
      --muted: #8b949e;
      --text: #e6edf3;
      --accent: #2f81f7;
      --success: #3fb950;
      --error: #f85149;
      --border: #30363d;
    }

    body {
      margin: 0;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, Inter, sans-serif;
      background: radial-gradient(circle at top, #1c2430 0%, var(--bg) 45%);
      color: var(--text);
      min-height: 100vh;
      display: grid;
      place-items: center;
      padding: 24px;
    }

    .card {
      width: 100%;
      max-width: 640px;
      background: var(--card);
      border: 1px solid var(--border);
      border-radius: 14px;
      box-shadow: 0 12px 40px rgba(0, 0, 0, 0.35);
      padding: 24px;
    }

    .header {
      display: flex;
      align-items: center;
      gap: 14px;
      margin-bottom: 16px;
    }

    .logo {
      width: 56px;
      height: 56px;
      border-radius: 10px;
      object-fit: cover;
      border: 1px solid var(--border);
      background: #111;
    }

    h1 {
      margin: 0;
      font-size: 1.25rem;
    }

    .subtitle {
      margin: 4px 0 0;
      color: var(--muted);
      font-size: 0.92rem;
    }

    .notice {
      padding: 10px 12px;
      border-radius: 10px;
      margin-bottom: 12px;
      border: 1px solid var(--border);
      font-size: 0.93rem;
    }

    .notice.success { border-color: color-mix(in oklab, var(--success), transparent 50%); color: var(--success); }
    .notice.error { border-color: color-mix(in oklab, var(--error), transparent 45%); color: var(--error); }

    label {
      display: block;
      margin-bottom: 6px;
      font-size: 0.92rem;
      color: var(--muted);
    }

    .checkbox-row {
      margin-top: -4px;
      margin-bottom: 12px;
    }

    label.checkbox {
      display: flex;
      align-items: center;
      gap: 8px;
      margin: 0;
      color: var(--muted);
      font-size: 0.84rem;
    }

    label.checkbox input {
      width: auto;
      margin: 0;
      accent-color: var(--accent);
    }

    input {
      width: 100%;
      box-sizing: border-box;
      border: 1px solid var(--border);
      background: #0f141b;
      color: var(--text);
      border-radius: 10px;
      padding: 10px 12px;
      font-size: 0.95rem;
      margin-bottom: 12px;
    }

    input:focus {
      outline: none;
      border-color: var(--accent);
      box-shadow: 0 0 0 3px rgba(47, 129, 247, 0.2);
    }

    button {
      margin-top: 6px;
      border: 0;
      border-radius: 10px;
      background: var(--accent);
      color: white;
      font-weight: 600;
      padding: 10px 14px;
      cursor: pointer;
    }

    .install {
      margin-top: 18px;
      padding-top: 14px;
      border-top: 1px solid var(--border);
    }

    .install-row {
      display: flex;
      gap: 8px;
      align-items: center;
      flex-wrap: wrap;
    }

    .install-row input {
      margin-bottom: 0;
      flex: 1 1 320px;
    }

    .install-actions {
      display: flex;
      gap: 8px;
      margin-top: 10px;
      flex-wrap: wrap;
    }

    .install-button {
      margin-top: 0;
      border: 1px solid color-mix(in oklab, var(--accent), transparent 35%);
      border-radius: 10px;
      background: color-mix(in oklab, var(--accent), #0d1117 40%);
      color: white;
      font-weight: 600;
      padding: 10px 14px;
      text-decoration: none;
      white-space: nowrap;
    }

    .install-button.secondary {
      border-color: color-mix(in oklab, var(--border), white 15%);
      background: color-mix(in oklab, var(--card), white 8%);
    }

    .hint {
      margin-top: -2px;
      margin-bottom: 12px;
      color: var(--muted);
      font-size: 0.83rem;
    }

    .form-actions {
      display: flex;
      gap: 8px;
      flex-wrap: wrap;
    }

    .secondary-button {
      background: color-mix(in oklab, var(--card), white 8%);
      border: 1px solid color-mix(in oklab, var(--border), white 12%);
    }

    .test-results {
      margin-top: 12px;
      margin-bottom: 4px;
    }

    .test-results .notice {
      margin-bottom: 8px;
    }

    .path {
      margin-top: 14px;
      color: var(--muted);
      font-size: 0.85rem;
      word-break: break-all;
    }
  </style>
</head>
<body>
  <main class="card">
    <div class="header">
      <img class="logo" src="/assets/logo.png" alt="Frankie logo" />
      <div>
        <h1>Frankie configuration</h1>
        <p class="subtitle">Connect your Prowlarr and AllDebrid credentials</p>
      </div>
    </div>

    {{if .Saved}}<p class="notice success">Saved successfully.</p>{{end}}
    {{if .Error}}<p class="notice error">{{.Error}}</p>{{end}}

    <form method="post" action="/configure">
      <label for="prowlarr_url">Prowlarr endpoint</label>
      <input id="prowlarr_url" name="prowlarr_url" type="url" placeholder="https://prowlarr.local:9696" value="{{.ProwlarrURL}}" />

      <label for="prowlarr_api_key">Prowlarr API key</label>
      <input id="prowlarr_api_key" name="prowlarr_api_key" type="password" value="" autocomplete="new-password" placeholder="Enter new key (optional)" />
      <div class="checkbox-row">
        <label class="checkbox"><input type="checkbox" name="clear_prowlarr_api_key" value="1" /> Clear saved Prowlarr API key</label>
      </div>

      <label for="alldebrid_api_key">AllDebrid API key</label>
      <input id="alldebrid_api_key" name="alldebrid_api_key" type="password" value="" autocomplete="new-password" placeholder="Enter new key (optional)" />
      <div class="checkbox-row">
        <label class="checkbox"><input type="checkbox" name="clear_alldebrid_api_key" value="1" /> Clear saved AllDebrid API key</label>
      </div>
      <p class="hint">Leave API key fields empty to keep existing values.</p>

      <div class="form-actions">
        <button type="submit" name="action" value="save">Save configuration</button>
        <button class="secondary-button" type="submit" name="action" value="test">Test connections</button>
      </div>
    </form>

    {{if .ConnectionTests}}
    <section class="test-results">
      <p class="subtitle">Connection test results</p>
      <p class="notice {{if .ConnectionTests.Prowlarr.OK}}success{{else}}error{{end}}">Prowlarr: {{.ConnectionTests.Prowlarr.Message}}</p>
      <p class="notice {{if .ConnectionTests.Alldebrid.OK}}success{{else}}error{{end}}">AllDebrid: {{.ConnectionTests.Alldebrid.Message}}</p>
    </section>
    {{end}}

    <section class="install">
      <label for="manifest_url">Manifest URL</label>
      <div class="install-row">
        <input id="manifest_url" type="text" value="{{.ManifestURL}}" readonly />
      </div>
      <div class="install-actions">
        <a class="install-button" href="{{.StremioInstallURL}}">Install in Stremio</a>
        {{if .ProwlarrURL}}<a class="install-button secondary" href="{{.ProwlarrURL}}" target="_blank" rel="noreferrer">Open Prowlarr</a>{{end}}
      </div>
    </section>

    <p class="path">Config file: <code>{{.ConfigPath}}</code></p>
  </main>
</body>
</html>
`))

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

type configureConnectionResult struct {
	OK      bool
	Message string
}

type configureConnectionTests struct {
	Prowlarr  configureConnectionResult
	Alldebrid configureConnectionResult
}

type configurePageData struct {
	ProwlarrURL       string
	ConfigPath        string
	ManifestURL       string
	StremioInstallURL template.URL
	ConnectionTests   *configureConnectionTests
	Saved             bool
	Error             string
}

func configureHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		renderConfigurePage(w, r, getConfig(), "", nil)
		return
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Error(w, "invalid form body", http.StatusBadRequest)
			return
		}

		updated, err := configFromConfigureForm(getConfig(), r)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			renderConfigurePage(w, r, getConfig(), err.Error(), nil)
			return
		}

		action := strings.TrimSpace(r.FormValue("action"))
		if action == "test" {
			results := runConfigureConnectionTests(updated)
			renderConfigurePage(w, r, updated, "", &results)
			return
		}

		if err := saveConfigFile(configFilePath(), updated); err != nil {
			log.Printf("save config: %v", err)
			http.Error(w, "failed to save config", http.StatusInternalServerError)
			return
		}

		setConfig(updated)
		http.Redirect(w, r, "/configure?saved=1", http.StatusSeeOther)
		return
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
}

func configFromConfigureForm(base Config, r *http.Request) (Config, error) {
	prowlarrURL := strings.TrimRight(strings.TrimSpace(r.FormValue("prowlarr_url")), "/")
	if prowlarrURL != "" {
		parsedURL, err := url.Parse(prowlarrURL)
		if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			return Config{}, fmt.Errorf("invalid Prowlarr endpoint URL")
		}
	}

	clearProwlarrAPIKey := strings.TrimSpace(r.FormValue("clear_prowlarr_api_key")) != ""
	clearAlldebridAPIKey := strings.TrimSpace(r.FormValue("clear_alldebrid_api_key")) != ""

	updated := base
	updated.ProwlarrURL = prowlarrURL
	if clearProwlarrAPIKey {
		updated.ProwlarrAPIKey = ""
	} else if prowlarrAPIKey := strings.TrimSpace(r.FormValue("prowlarr_api_key")); prowlarrAPIKey != "" {
		updated.ProwlarrAPIKey = prowlarrAPIKey
	}
	if clearAlldebridAPIKey {
		updated.AlldebridAPIKey = ""
	} else if alldebridAPIKey := strings.TrimSpace(r.FormValue("alldebrid_api_key")); alldebridAPIKey != "" {
		updated.AlldebridAPIKey = alldebridAPIKey
	}

	return normalizeConfig(updated), nil
}

func renderConfigurePage(w http.ResponseWriter, r *http.Request, cfg Config, errorMessage string, tests *configureConnectionTests) {
	manifestURL := requestBaseURL(r) + "/manifest.json"
	data := configurePageData{
		ProwlarrURL:       cfg.ProwlarrURL,
		ConfigPath:        configFilePath(),
		ManifestURL:       manifestURL,
		StremioInstallURL: template.URL(stremioInstallURL(manifestURL)),
		ConnectionTests:   tests,
		Saved:             tests == nil && r.URL.Query().Get("saved") == "1",
		Error:             strings.TrimSpace(errorMessage),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := configurePageTemplate.Execute(w, data); err != nil {
		log.Printf("render configure page: %v", err)
	}
}

func manifestHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	respondJSON(w, http.StatusOK, manifestForRequest(r))
}

func manifestForRequest(r *http.Request) Manifest {
	m := manifest
	baseURL := addonBaseURL(r)
	m.Logo = baseURL + "/assets/logo.png"
	m.Background = baseURL + "/assets/background.jpg"
	return m
}

func addonBaseURL(r *http.Request) string {
	cfg := getConfig()
	if cfg.PublicURL != "" {
		return strings.TrimRight(cfg.PublicURL, "/")
	}
	return requestBaseURL(r)
}

func requestBaseURL(r *http.Request) string {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = strings.Split(forwardedProto, ",")[0]
	}

	return scheme + "://" + r.Host
}

func stremioInstallURL(manifestURL string) string {
	trimmed := strings.TrimSpace(manifestURL)
	trimmed = strings.TrimPrefix(trimmed, "https://")
	trimmed = strings.TrimPrefix(trimmed, "http://")
	return "stremio://" + trimmed
}

func runConfigureConnectionTests(cfg Config) configureConnectionTests {
	return configureConnectionTests{
		Prowlarr:  testProwlarrConnection(cfg),
		Alldebrid: testAllDebridConnection(cfg),
	}
}

func testProwlarrConnection(cfg Config) configureConnectionResult {
	if strings.TrimSpace(cfg.ProwlarrURL) == "" {
		return configureConnectionResult{Message: "Not configured: missing Prowlarr URL"}
	}
	if strings.TrimSpace(cfg.ProwlarrAPIKey) == "" {
		return configureConnectionResult{Message: "Not configured: missing Prowlarr API key"}
	}

	baseURL, err := url.Parse(cfg.ProwlarrURL)
	if err != nil {
		return configureConnectionResult{Message: "Invalid Prowlarr URL"}
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/api/v1/system/status"

	req, err := http.NewRequest(http.MethodGet, baseURL.String(), nil)
	if err != nil {
		return configureConnectionResult{Message: "Failed to build Prowlarr request"}
	}
	req.Header.Set("X-Api-Key", cfg.ProwlarrAPIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return configureConnectionResult{Message: "Connection failed: " + err.Error()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return configureConnectionResult{Message: "Failed to read Prowlarr response"}
	}
	if resp.StatusCode != http.StatusOK {
		return configureConnectionResult{Message: fmt.Sprintf("Prowlarr returned HTTP %d", resp.StatusCode)}
	}

	var payload struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return configureConnectionResult{OK: true, Message: "Connected (non-standard response payload)"}
	}

	msg := "Connected"
	if strings.TrimSpace(payload.Version) != "" {
		msg += " (version " + strings.TrimSpace(payload.Version) + ")"
	}
	return configureConnectionResult{OK: true, Message: msg}
}

func testAllDebridConnection(cfg Config) configureConnectionResult {
	if strings.TrimSpace(cfg.AlldebridAPIKey) == "" {
		return configureConnectionResult{Message: "Not configured: missing AllDebrid API key"}
	}

	req, err := http.NewRequest(http.MethodGet, allDebridBaseURL+"/v4/user", nil)
	if err != nil {
		return configureConnectionResult{Message: "Failed to build AllDebrid request"}
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.AlldebridAPIKey))
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return configureConnectionResult{Message: "Connection failed: " + err.Error()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return configureConnectionResult{Message: "Failed to read AllDebrid response"}
	}
	if resp.StatusCode != http.StatusOK {
		return configureConnectionResult{Message: fmt.Sprintf("AllDebrid returned HTTP %d", resp.StatusCode)}
	}

	var payload struct {
		Status string `json:"status"`
		Data   struct {
			User struct {
				Username string `json:"username"`
			} `json:"user"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return configureConnectionResult{Message: "Invalid AllDebrid response payload"}
	}
	if !strings.EqualFold(payload.Status, "success") {
		return configureConnectionResult{Message: "AllDebrid authentication failed"}
	}

	msg := "Connected"
	if strings.TrimSpace(payload.Data.User.Username) != "" {
		msg += " as " + strings.TrimSpace(payload.Data.User.Username)
	}
	return configureConnectionResult{OK: true, Message: msg}
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
		Port:            os.Getenv("PORT"),
		ProwlarrURL:     os.Getenv("PROWLARR_URL"),
		ProwlarrAPIKey:  os.Getenv("PROWLARR_API_KEY"),
		AlldebridAPIKey: os.Getenv("ALLDEBRID_API_KEY"),
		PublicURL:       os.Getenv("PUBLIC_URL"),
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
