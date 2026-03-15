package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
)

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
      <p class="hint">Saved key: {{if .HasProwlarrAPIKey}}<code>{{.MaskedProwlarrAPIKey}}</code>{{else}}no{{end}}</p>
      <div class="checkbox-row">
        <label class="checkbox"><input type="checkbox" name="clear_prowlarr_api_key" value="1" /> Clear saved Prowlarr API key</label>
      </div>

      <label for="alldebrid_api_key">AllDebrid API key</label>
      <input id="alldebrid_api_key" name="alldebrid_api_key" type="password" value="" autocomplete="new-password" placeholder="Enter new key (optional)" />
      <p class="hint">Saved key: {{if .HasAlldebridAPIKey}}<code>{{.MaskedAlldebridAPIKey}}</code>{{else}}no{{end}}</p>
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

type configureConnectionResult struct {
	OK      bool
	Message string
}

type configureConnectionTests struct {
	Prowlarr  configureConnectionResult
	Alldebrid configureConnectionResult
}

type configurePageData struct {
	ProwlarrURL           string
	ConfigPath            string
	ManifestURL           string
	StremioInstallURL     template.URL
	HasProwlarrAPIKey     bool
	HasAlldebridAPIKey    bool
	MaskedProwlarrAPIKey  string
	MaskedAlldebridAPIKey string
	ConnectionTests       *configureConnectionTests
	Saved                 bool
	Error                 string
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
			if err := saveConfigFile(configFilePath(), updated); err != nil {
				log.Printf("save config: %v", err)
				http.Error(w, "failed to save config", http.StatusInternalServerError)
				return
			}
			setConfig(updated)
			log.Printf("config saved via test action: prowlarr_url=%q prowlarr_key_set=%t alldebrid_key_set=%t", updated.ProwlarrURL, strings.TrimSpace(updated.ProwlarrAPIKey) != "", strings.TrimSpace(updated.AlldebridAPIKey) != "")

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
		log.Printf("config saved successfully: prowlarr_url=%q prowlarr_key_set=%t alldebrid_key_set=%t", updated.ProwlarrURL, strings.TrimSpace(updated.ProwlarrAPIKey) != "", strings.TrimSpace(updated.AlldebridAPIKey) != "")
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
		ProwlarrURL:           cfg.ProwlarrURL,
		ConfigPath:            configFilePath(),
		ManifestURL:           manifestURL,
		StremioInstallURL:     template.URL(stremioInstallURL(manifestURL)),
		HasProwlarrAPIKey:     strings.TrimSpace(cfg.ProwlarrAPIKey) != "",
		HasAlldebridAPIKey:    strings.TrimSpace(cfg.AlldebridAPIKey) != "",
		MaskedProwlarrAPIKey:  maskSecret(cfg.ProwlarrAPIKey),
		MaskedAlldebridAPIKey: maskSecret(cfg.AlldebridAPIKey),
		ConnectionTests:       tests,
		Saved:                 tests == nil && r.URL.Query().Get("saved") == "1",
		Error:                 strings.TrimSpace(errorMessage),
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := configurePageTemplate.Execute(w, data); err != nil {
		log.Printf("render configure page: %v", err)
	}
}

func maskSecret(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return "********"
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
