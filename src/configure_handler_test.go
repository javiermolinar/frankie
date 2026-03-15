package main

import (
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
