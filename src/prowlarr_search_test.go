package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSearchProwlarrMergesPrimaryAndFallbackResults(t *testing.T) {
	originalConfig := getConfig()
	defer setConfig(originalConfig)
	originalHTTPClient := httpClient
	defer func() { httpClient = originalHTTPClient }()

	setConfig(Config{
		Port:                         defaultPort,
		ProwlarrURL:                  "http://prowlarr.local:9696",
		ProwlarrAPIKey:               "test-key",
		ProwlarrSearchTimeoutSeconds: 5,
	})

	primaryCalls := 0
	fallbackCalls := 0

	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "prowlarr.local:9696" && req.URL.Path == "/api/v1/search":
			query := req.URL.Query().Get("query")
			switch query {
			case "{ImdbId:tt1234567}":
				primaryCalls++
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`[{"title":"Primary.Result","guid":"magnet:?xt=urn:btih:abc"}]`)),
					Header:     make(http.Header),
				}, nil
			case "Fallback Show S01E02":
				fallbackCalls++
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`[{"title":"Fallback.Result","guid":"magnet:?xt=urn:btih:def"}]`)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected prowlarr query %q", query)
			}
		case req.URL.Host == "v3-cinemeta.strem.io":
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"meta":{"name":"Fallback Show","year":"2024-01-01"}}`)),
				Header:     make(http.Header),
			}, nil
		}

		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Header:     make(http.Header),
		}, nil
	})}

	results, _, err := searchProwlarr("series", "tt1234567:1:2")
	if err != nil {
		t.Fatalf("searchProwlarr returned error: %v", err)
	}
	if primaryCalls != 1 {
		t.Fatalf("expected primary search once, got %d", primaryCalls)
	}
	if fallbackCalls != 1 {
		t.Fatalf("expected fallback search once, got %d", fallbackCalls)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 merged results, got %d", len(results))
	}
	if results[0].Title != "Primary.Result" || results[1].Title != "Fallback.Result" {
		t.Fatalf("unexpected merged results: %#v", results)
	}
}

func TestSearchProwlarrUsesFallbackWhenPrimaryIsEmpty(t *testing.T) {
	originalConfig := getConfig()
	defer setConfig(originalConfig)
	originalHTTPClient := httpClient
	defer func() { httpClient = originalHTTPClient }()

	setConfig(Config{
		Port:                         defaultPort,
		ProwlarrURL:                  "http://prowlarr.local:9696",
		ProwlarrAPIKey:               "test-key",
		ProwlarrSearchTimeoutSeconds: 5,
	})

	primaryCalls := 0
	fallbackCalls := 0

	httpClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "prowlarr.local:9696" && req.URL.Path == "/api/v1/search":
			query := req.URL.Query().Get("query")
			switch query {
			case "{ImdbId:tt7654321}":
				primaryCalls++
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`[]`)),
					Header:     make(http.Header),
				}, nil
			case "Fallback Movie 2024":
				fallbackCalls++
				return &http.Response{
					StatusCode: http.StatusOK,
					Body:       io.NopCloser(strings.NewReader(`[{"title":"Fallback.Result","guid":"magnet:?xt=urn:btih:def"}]`)),
					Header:     make(http.Header),
				}, nil
			default:
				t.Fatalf("unexpected prowlarr fallback query %q", query)
			}
		case req.URL.Host == "v3-cinemeta.strem.io":
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"meta":{"name":"Fallback Movie","year":"2024-01-01"}}`)),
				Header:     make(http.Header),
			}, nil
		}

		return &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(`{}`)),
			Header:     make(http.Header),
		}, nil
	})}

	results, _, err := searchProwlarr("movie", "tt7654321")
	if err != nil {
		t.Fatalf("searchProwlarr returned error: %v", err)
	}
	if primaryCalls != 1 {
		t.Fatalf("expected primary search once, got %d", primaryCalls)
	}
	if fallbackCalls != 1 {
		t.Fatalf("expected fallback search once, got %d", fallbackCalls)
	}
	if len(results) != 1 || results[0].Title != "Fallback.Result" {
		t.Fatalf("unexpected fallback results: %#v", results)
	}
}
