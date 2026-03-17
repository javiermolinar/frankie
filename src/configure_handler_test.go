package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureHandlerInvalidURLReturnsBadRequest(t *testing.T) {
	originalConfig := getConfig()
	defer setConfig(originalConfig)

	form := url.Values{}
	form.Set("prowlarr_url", "notaurl")

	req := httptest.NewRequest(http.MethodPost, "/configure", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	configureHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

func TestConfigureHandlerPreservesAndClearsAPIKeys(t *testing.T) {
	originalConfig := getConfig()
	defer setConfig(originalConfig)

	originalConfigFile, hadConfigFile := os.LookupEnv("CONFIG_FILE")
	defer func() {
		if hadConfigFile {
			_ = os.Setenv("CONFIG_FILE", originalConfigFile)
		} else {
			_ = os.Unsetenv("CONFIG_FILE")
		}
	}()

	tmpConfigPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.Setenv("CONFIG_FILE", tmpConfigPath); err != nil {
		t.Fatalf("set CONFIG_FILE: %v", err)
	}

	setConfig(Config{
		Port:            defaultPort,
		ProwlarrURL:     "http://prowlarr.local:9696",
		ProwlarrAPIKey:  "prowlarr-old",
		AlldebridAPIKey: "alldebrid-old",
	})

	// Empty API key fields should preserve existing keys.
	preserveForm := url.Values{}
	preserveForm.Set("prowlarr_url", "http://prowlarr.local:9696")

	preserveReq := httptest.NewRequest(http.MethodPost, "/configure", strings.NewReader(preserveForm.Encode()))
	preserveReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	preserveRR := httptest.NewRecorder()

	configureHandler(preserveRR, preserveReq)

	if preserveRR.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d on preserve submit, got %d", http.StatusSeeOther, preserveRR.Code)
	}

	preserved := getConfig()
	if preserved.ProwlarrAPIKey != "prowlarr-old" {
		t.Fatalf("expected prowlarr key to be preserved, got %q", preserved.ProwlarrAPIKey)
	}
	if preserved.AlldebridAPIKey != "alldebrid-old" {
		t.Fatalf("expected alldebrid key to be preserved, got %q", preserved.AlldebridAPIKey)
	}

	// Clear flags should remove existing keys.
	clearForm := url.Values{}
	clearForm.Set("prowlarr_url", "http://prowlarr.local:9696")
	clearForm.Set("clear_prowlarr_api_key", "1")
	clearForm.Set("clear_alldebrid_api_key", "1")

	clearReq := httptest.NewRequest(http.MethodPost, "/configure", strings.NewReader(clearForm.Encode()))
	clearReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	clearRR := httptest.NewRecorder()

	configureHandler(clearRR, clearReq)

	if clearRR.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d on clear submit, got %d", http.StatusSeeOther, clearRR.Code)
	}

	cleared := getConfig()
	if cleared.ProwlarrAPIKey != "" {
		t.Fatalf("expected prowlarr key to be cleared, got %q", cleared.ProwlarrAPIKey)
	}
	if cleared.AlldebridAPIKey != "" {
		t.Fatalf("expected alldebrid key to be cleared, got %q", cleared.AlldebridAPIKey)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func TestConfigureHandlerTestActionRunsConnectionChecks(t *testing.T) {
	originalConfig := getConfig()
	defer setConfig(originalConfig)

	originalConfigFile, hadConfigFile := os.LookupEnv("CONFIG_FILE")
	defer func() {
		if hadConfigFile {
			_ = os.Setenv("CONFIG_FILE", originalConfigFile)
		} else {
			_ = os.Unsetenv("CONFIG_FILE")
		}
	}()

	tmpConfigPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.Setenv("CONFIG_FILE", tmpConfigPath); err != nil {
		t.Fatalf("set CONFIG_FILE: %v", err)
	}

	originalHTTPClient := httpClient
	defer func() { httpClient = originalHTTPClient }()

	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Host, "prowlarr.local") && req.URL.Path == "/api/v1/system/status":
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"version":"1.2.3"}`)),
				Header:     make(http.Header),
			}, nil
		case req.URL.Host == "api.alldebrid.com" && req.URL.Path == "/v4/user":
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"status":"success","data":{"user":{"username":"tester"}}}`)),
				Header:     make(http.Header),
			}, nil
		default:
			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(strings.NewReader(`{}`)),
				Header:     make(http.Header),
			}, nil
		}
	})}

	setConfig(Config{
		Port:            defaultPort,
		ProwlarrURL:     "http://saved.example:9696",
		ProwlarrAPIKey:  "saved-prowlarr",
		AlldebridAPIKey: "saved-alldebrid",
	})

	form := url.Values{}
	form.Set("action", "test")
	form.Set("prowlarr_url", "http://prowlarr.local:9696")
	form.Set("prowlarr_api_key", "new-prowlarr")
	form.Set("alldebrid_api_key", "new-alldebrid")

	req := httptest.NewRequest(http.MethodPost, "/configure", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	configureHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d for test action, got %d", http.StatusOK, rr.Code)
	}

	body := rr.Body.String()
	if !strings.Contains(body, "Connection test results") {
		t.Fatalf("expected connection test results in response body")
	}
	if !strings.Contains(body, "Prowlarr: Connected") {
		t.Fatalf("expected prowlarr success in response body")
	}
	if !strings.Contains(body, "AllDebrid: Connected") {
		t.Fatalf("expected alldebrid success in response body")
	}

	// Test action should persist configuration so refresh keeps keys.
	after := getConfig()
	if after.ProwlarrURL != "http://prowlarr.local:9696" || after.ProwlarrAPIKey != "new-prowlarr" || after.AlldebridAPIKey != "new-alldebrid" {
		t.Fatalf("expected config to be updated after test action, got %+v", after)
	}
}

func TestConfigureHandlerSavesLanguageOrder(t *testing.T) {
	originalConfig := getConfig()
	defer setConfig(originalConfig)

	originalConfigFile, hadConfigFile := os.LookupEnv("CONFIG_FILE")
	defer func() {
		if hadConfigFile {
			_ = os.Setenv("CONFIG_FILE", originalConfigFile)
		} else {
			_ = os.Unsetenv("CONFIG_FILE")
		}
	}()

	tmpConfigPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.Setenv("CONFIG_FILE", tmpConfigPath); err != nil {
		t.Fatalf("set CONFIG_FILE: %v", err)
	}

	form := url.Values{}
	form.Set("primary_language", "Spanish")
	form.Set("secondary_language", "English")

	req := httptest.NewRequest(http.MethodPost, "/configure", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	configureHandler(rr, req)

	if rr.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, rr.Code)
	}

	after := getConfig()
	if after.PrimaryLanguage != "Spanish" || after.SecondaryLanguage != "English" {
		t.Fatalf("expected language order to be persisted, got primary=%q secondary=%q", after.PrimaryLanguage, after.SecondaryLanguage)
	}
}

func TestConfigureHandlerRejectsInvalidLanguageOrder(t *testing.T) {
	originalConfig := getConfig()
	defer setConfig(originalConfig)

	form := url.Values{}
	form.Set("primary_language", "")
	form.Set("secondary_language", "English")

	req := httptest.NewRequest(http.MethodPost, "/configure", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rr := httptest.NewRecorder()

	configureHandler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rr.Code)
	}
}
