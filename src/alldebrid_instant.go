package main

import (
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const instantAvailabilityBatchSize = 50

type InstantMagnetResponse struct {
	Status string            `json:"status"`
	Data   InstantMagnetData `json:"data"`
	Error  *FileError        `json:"error,omitempty"`
}

type InstantMagnetData struct {
	Magnets []InstantMagnet `json:"magnets"`
}

type InstantMagnet struct {
	Magnet  string     `json:"magnet,omitempty"`
	Hash    string     `json:"hash,omitempty"`
	Instant bool       `json:"instant,omitempty"`
	Error   *FileError `json:"error,omitempty"`
}

// checkInstantMagnets checks cache availability without creating AllDebrid magnets.
// Results are keyed by the lowercase BitTorrent info hash.
func checkInstantMagnets(magnets []string) (map[string]bool, error) {
	available := make(map[string]bool, len(magnets))
	for start := 0; start < len(magnets); start += instantAvailabilityBatchSize {
		end := start + instantAvailabilityBatchSize
		if end > len(magnets) {
			end = len(magnets)
		}
		batch, err := checkInstantMagnetBatch(magnets[start:end])
		if err != nil {
			return nil, err
		}
		for hash, instant := range batch {
			available[hash] = instant
		}
	}
	return available, nil
}

func checkInstantMagnetBatch(magnets []string) (map[string]bool, error) {
	baseURL, _ := url.Parse(allDebridBaseURL)
	baseURL.Path = magnetInstantPath
	form := url.Values{}
	form.Set("agent", "Frankie")
	for _, magnet := range magnets {
		form.Add("magnets[]", magnet)
	}

	req, err := http.NewRequest(http.MethodPost, baseURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", getConfig().AlldebridAPIKey))

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result InstantMagnetResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}
	if result.Status != "success" {
		return nil, allDebridAPIError("instant availability check failed", result.Status, result.Error)
	}

	available := make(map[string]bool, len(result.Data.Magnets))
	for _, magnet := range result.Data.Magnets {
		if magnet.Error != nil {
			log.Printf("instant availability magnet error: %v", allDebridAPIError("invalid magnet", "error", magnet.Error))
			continue
		}
		hash := strings.ToLower(strings.TrimSpace(magnet.Hash))
		if hash == "" {
			hash = magnetInfoHash(magnet.Magnet)
		}
		if hash != "" {
			available[hash] = magnet.Instant
		}
	}
	return available, nil
}

func magnetInfoHash(value string) string {
	value = strings.TrimSpace(value)
	if hash := normalizeInfoHash(value); hash != "" {
		return hash
	}

	parsed, err := url.Parse(value)
	if err != nil || !strings.EqualFold(parsed.Scheme, "magnet") {
		return ""
	}
	for _, xt := range parsed.Query()["xt"] {
		const prefix = "urn:btih:"
		if len(xt) >= len(prefix) && strings.EqualFold(xt[:len(prefix)], prefix) {
			if hash := normalizeInfoHash(xt[len(prefix):]); hash != "" {
				return hash
			}
		}
	}
	return ""
}

func normalizeInfoHash(value string) string {
	if isInfoHash(value) {
		return strings.ToLower(value)
	}
	if len(value) != 32 {
		return ""
	}
	decoded, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(value))
	if err != nil || len(decoded) != 20 {
		return ""
	}
	return hex.EncodeToString(decoded)
}

func isInfoHash(value string) bool {
	if len(value) != 40 {
		return false
	}
	for _, char := range value {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}

func deleteMagnet(id int) error {
	baseURL, _ := url.Parse(allDebridBaseURL)
	baseURL.Path = magnetDeletePath
	form := url.Values{}
	form.Set("id", strconv.Itoa(id))

	req, err := http.NewRequest(http.MethodPost, baseURL.String(), strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", getConfig().AlldebridAPIKey))

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result struct {
		Status string     `json:"status"`
		Error  *FileError `json:"error,omitempty"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return err
	}
	if result.Status != "success" {
		return allDebridAPIError("delete magnet failed", result.Status, result.Error)
	}
	return nil
}

func allDebridAPIError(operation, status string, apiError *FileError) error {
	if apiError != nil {
		code := strings.TrimSpace(apiError.Code)
		message := strings.TrimSpace(apiError.Message)
		switch {
		case code != "" && message != "":
			return fmt.Errorf("%s: %s (%s)", operation, message, code)
		case message != "":
			return fmt.Errorf("%s: %s", operation, message)
		case code != "":
			return fmt.Errorf("%s: %s", operation, code)
		}
	}
	return fmt.Errorf("%s with status %q", operation, status)
}
