package main

import (
	"net/http"
	"strings"
)

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
