package main

import "testing"

func TestNormalizeConfigSetsDefaultProwlarrSearchTimeout(t *testing.T) {
	cfg := normalizeConfig(Config{ProwlarrSearchTimeoutSeconds: 0})
	if cfg.ProwlarrSearchTimeoutSeconds != defaultProwlarrSearchTimeoutSeconds {
		t.Fatalf("expected default timeout %d, got %d", defaultProwlarrSearchTimeoutSeconds, cfg.ProwlarrSearchTimeoutSeconds)
	}
}

func TestLoadConfigReadsProwlarrSearchTimeoutFromEnv(t *testing.T) {
	t.Setenv("CONFIG_FILE", t.TempDir()+"/missing-config.json")
	t.Setenv("PROWLARR_SEARCH_TIMEOUT_SECONDS", "7")

	cfg := loadConfig()
	if cfg.ProwlarrSearchTimeoutSeconds != 7 {
		t.Fatalf("expected timeout 7, got %d", cfg.ProwlarrSearchTimeoutSeconds)
	}
}

func TestLoadConfigInvalidProwlarrSearchTimeoutFallsBackToDefault(t *testing.T) {
	t.Setenv("CONFIG_FILE", t.TempDir()+"/missing-config.json")
	t.Setenv("PROWLARR_SEARCH_TIMEOUT_SECONDS", "invalid")

	cfg := loadConfig()
	if cfg.ProwlarrSearchTimeoutSeconds != defaultProwlarrSearchTimeoutSeconds {
		t.Fatalf("expected default timeout %d, got %d", defaultProwlarrSearchTimeoutSeconds, cfg.ProwlarrSearchTimeoutSeconds)
	}
}
