package main

import (
	"encoding/base32"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func setupAllDebridTest(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	originalBaseURL := allDebridBaseURL
	originalConfig := getConfig()
	originalRegistry := readyMagnets
	registryPath := t.TempDir() + "/frankie_magnets.json"
	allDebridBaseURL = server.URL
	readyMagnets = newMagnetRegistry(registryPath)
	setConfig(Config{AlldebridAPIKey: "test-key"})
	t.Cleanup(func() {
		server.Close()
		allDebridBaseURL = originalBaseURL
		setConfig(originalConfig)
		readyMagnets = originalRegistry
	})
	return registryPath
}

func TestMagnetInfoHashSupportsHexAndBase32(t *testing.T) {
	raw := []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
	want := hex.EncodeToString(raw)
	base32Hash := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(raw)
	for _, value := range []string{
		"magnet:?xt=urn:btih:" + strings.ToUpper(want),
		"magnet:?xt=urn:btih:" + base32Hash,
	} {
		if got := magnetInfoHash(value); got != want {
			t.Errorf("magnetInfoHash(%q) = %q, want %q", value, got, want)
		}
	}
}

func TestUploadMagnetUsesDocumentedFieldAndReportsAPIError(t *testing.T) {
	setupAllDebridTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != magnetUploadPath {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		if len(r.Form["magnets[]"]) != 1 || len(r.Form["magnets"]) != 0 {
			t.Errorf("unexpected magnet form fields: %#v", r.Form)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"error","error":{"code":"MAGNET_TOO_MANY","message":"Magnets limit reached"}}`)
	})

	_, err := uploadMagnet("magnet:?xt=urn:btih:" + strings.Repeat("a", 40))
	if err == nil || !strings.Contains(err.Error(), "MAGNET_TOO_MANY") || !strings.Contains(err.Error(), "Magnets limit reached") {
		t.Fatalf("expected structured AllDebrid error, got %v", err)
	}
}

func TestResolveDebridResultsUploadsOnlyInstantlyAvailableMagnets(t *testing.T) {
	cachedHash := strings.Repeat("a", 40)
	uncachedHash := strings.Repeat("b", 40)
	var uploadCalls atomic.Int32
	var deleteCalls atomic.Int32

	registryPath := setupAllDebridTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		switch r.URL.Path {
		case magnetInstantPath:
			if len(r.Form["magnets[]"]) != 2 {
				t.Errorf("expected two instant checks, got %#v", r.Form["magnets[]"])
			}
			fmt.Fprintf(w, `{"status":"success","data":{"magnets":[{"hash":%q,"instant":true},{"hash":%q,"instant":false}]}}`, cachedHash, uncachedHash)
		case magnetUploadPath:
			uploadCalls.Add(1)
			fmt.Fprintf(w, `{"status":"success","data":{"magnets":[{"id":101,"hash":%q,"ready":true}]}}`, cachedHash)
		case magnetStatusPath:
			fmt.Fprint(w, `{"status":"success","data":{"magnets":[{"id":101,"status":"Ready","statusCode":4,"files":[{"n":"cached.mkv","s":123,"l":"https://alldebrid.test/file"}]}]}}`)
		case unlockLinkPath:
			fmt.Fprint(w, `{"status":"success","data":{"link":"https://cdn.test/cached.mkv","filename":"cached.mkv","filesize":123}}`)
		case magnetDeletePath:
			deleteCalls.Add(1)
			fmt.Fprint(w, `{"status":"success"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	results := []ProwlarrSearchResult{
		{Title: "Cached", Guid: "magnet:?xt=urn:btih:" + cachedHash},
		{Title: "Not cached", Guid: "magnet:?xt=urn:btih:" + uncachedHash},
	}
	streams := resolveDebridResults(results)
	if len(streams) != 1 || streams[0].URL != "https://cdn.test/cached.mkv" {
		t.Fatalf("unexpected streams: %#v", streams)
	}
	if uploadCalls.Load() != 1 {
		t.Fatalf("expected only the cached magnet to be uploaded, got %d uploads", uploadCalls.Load())
	}
	if deleteCalls.Load() != 0 {
		t.Fatalf("ready magnet must remain available for playback, got %d deletes", deleteCalls.Load())
	}

	persisted := newMagnetRegistry(registryPath)
	persisted.mu.Lock()
	_, tracked := persisted.magnets["101"]
	persisted.mu.Unlock()
	if !tracked {
		t.Fatal("ready magnet was not persisted for later cleanup")
	}
}

func TestResolveDebridResultsFallsBackToGuardedUploadWhenInstantIsUnsupported(t *testing.T) {
	var uploadCalls atomic.Int32
	var deleteCalls atomic.Int32
	setupAllDebridTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case magnetInstantPath:
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"status":"error","error":{"code":"404","message":"Endpoint doesn't exist"}}`)
		case magnetUploadPath:
			uploadCalls.Add(1)
			fmt.Fprintf(w, `{"status":"success","data":{"magnets":[{"id":303,"hash":%q,"ready":false}]}}`, strings.Repeat("f", 40))
		case magnetStatusPath:
			t.Error("non-ready fallback upload should not require a status request")
			fmt.Fprint(w, `{"status":"success","data":{"magnets":[]}}`)
		case magnetDeletePath:
			deleteCalls.Add(1)
			fmt.Fprint(w, `{"status":"success"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	results := []ProwlarrSearchResult{{Title: "Not ready", Guid: "magnet:?xt=urn:btih:" + strings.Repeat("f", 40)}}
	streams := resolveDebridResults(results)
	if len(streams) != 0 {
		t.Fatalf("expected no streams, got %#v", streams)
	}
	if uploadCalls.Load() != 1 || deleteCalls.Load() != 1 {
		t.Fatalf("expected one guarded upload and cleanup, got uploads=%d deletes=%d", uploadCalls.Load(), deleteCalls.Load())
	}
}

func TestGetStreamLinkDeletesNonReadyMagnet(t *testing.T) {
	var deleteCalls atomic.Int32
	registryPath := setupAllDebridTest(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case magnetUploadPath:
			fmt.Fprintf(w, `{"status":"success","data":{"magnets":[{"id":202,"hash":%q,"ready":false}]}}`, strings.Repeat("c", 40))
		case magnetStatusPath:
			fmt.Fprint(w, `{"status":"success","data":{"magnets":[]}}`)
		case magnetDeletePath:
			deleteCalls.Add(1)
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse delete form: %v", err)
			}
			if r.Form.Get("id") != "202" {
				t.Errorf("deleted wrong magnet: %q", r.Form.Get("id"))
			}
			fmt.Fprint(w, `{"status":"success"}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	})

	_, err := getStreamLink("magnet:?xt=urn:btih:" + strings.Repeat("c", 40))
	if err == nil || !strings.Contains(err.Error(), "not ready") {
		t.Fatalf("expected non-ready error, got %v", err)
	}
	if deleteCalls.Load() != 1 {
		t.Fatalf("expected failed magnet to be deleted once, got %d", deleteCalls.Load())
	}
	persisted := newMagnetRegistry(registryPath)
	persisted.mu.Lock()
	count := len(persisted.magnets)
	persisted.mu.Unlock()
	if count != 0 {
		t.Fatalf("deleted magnet remains tracked: %#v", persisted.magnets)
	}
}

func TestMagnetRegistryDeletesOnlyExpiredFrankieMagnets(t *testing.T) {
	var deletedID atomic.Int32
	setupAllDebridTest(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != magnetDeletePath {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
		}
		var id int
		if _, err := fmt.Sscanf(r.Form.Get("id"), "%d", &id); err != nil {
			t.Errorf("parse id: %v", err)
		}
		deletedID.Store(int32(id))
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"status":"success"}`)
	})

	now := time.Now()
	registry := newMagnetRegistry(t.TempDir() + "/registry.json")
	if err := registry.track(301, strings.Repeat("d", 40), now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := registry.track(302, strings.Repeat("e", 40), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	registry.cleanupExpired(now)

	if deletedID.Load() != 301 {
		t.Fatalf("expected only expired magnet 301 to be deleted, got %d", deletedID.Load())
	}
	registry.mu.Lock()
	defer registry.mu.Unlock()
	if _, exists := registry.magnets["301"]; exists {
		t.Fatal("expired magnet remains tracked")
	}
	if _, exists := registry.magnets["302"]; !exists {
		t.Fatal("recent magnet was removed")
	}
}
