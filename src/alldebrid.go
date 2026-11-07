package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	magnetUploadPath  = "/v4/magnet/upload"
	magnetInstantPath = "/v4/magnet/instant"
	magnetStatusPath  = "/v4.1/magnet/status"
	magnetDeletePath  = "/v4/magnet/delete"
	unlockLinkPath    = "/v4/link/unlock"

	magnetStatusReady = "ready"
)

var allDebridBaseURL = "https://api.alldebrid.com"

// UploadMagnetResponse represents the top-level upload response JSON structure.
type UploadMagnetResponse struct {
	Status string           `json:"status"`
	Data   UploadMagnetData `json:"data"`
	Error  *FileError       `json:"error,omitempty"`
}

type UploadMagnetData struct {
	Magnets []Magnet `json:"magnets"`
}

type UnlockLinkRespose struct {
	Status string         `json:"status"`
	Data   UnlockLinkData `json:"data"`
	Error  *FileError     `json:"error,omitempty"`
}

// MagnetStatusResponse represents /magnet/status response.
type MagnetStatusResponse struct {
	Status string           `json:"status"`
	Data   MagnetStatusData `json:"data"`
	Error  *FileError       `json:"error,omitempty"`
}

type MagnetStatusData struct {
	Magnets []MagnetStatus `json:"magnets"`
}

func (d *MagnetStatusData) UnmarshalJSON(data []byte) error {
	var payload struct {
		Magnets json.RawMessage `json:"magnets"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	d.Magnets = []MagnetStatus{}
	trimmed := bytes.TrimSpace(payload.Magnets)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	switch trimmed[0] {
	case '[':
		return json.Unmarshal(trimmed, &d.Magnets)
	case '{':
		var single MagnetStatus
		if err := json.Unmarshal(trimmed, &single); err == nil {
			if single.ID != 0 || single.Filename != "" || single.Status != "" || len(single.Files) > 0 {
				d.Magnets = append(d.Magnets, single)
				return nil
			}
		}

		var keyed map[string]MagnetStatus
		if err := json.Unmarshal(trimmed, &keyed); err != nil {
			return err
		}

		d.Magnets = make([]MagnetStatus, 0, len(keyed))
		for key, magnet := range keyed {
			if magnet.ID == 0 {
				if parsedID, err := strconv.Atoi(key); err == nil {
					magnet.ID = parsedID
				}
			}
			d.Magnets = append(d.Magnets, magnet)
		}
		return nil
	default:
		return fmt.Errorf("unexpected magnets payload shape")
	}
}

// Magnet represents a magnet entry returned by /magnet/upload.
type Magnet struct {
	Mag   string     `json:"magnet"`
	Name  string     `json:"name,omitempty"`
	Size  int64      `json:"size,omitempty"`
	Hash  string     `json:"hash,omitempty"`
	Ready bool       `json:"ready,omitempty"`
	ID    int        `json:"id,omitempty"`
	Error *FileError `json:"error,omitempty"`
}

// MagnetStatus represents a magnet entry returned by /magnet/status.
type MagnetStatus struct {
	ID             int          `json:"id"`
	Filename       string       `json:"filename,omitempty"`
	Size           int64        `json:"size,omitempty"`
	Status         string       `json:"status,omitempty"`
	StatusCode     int          `json:"statusCode,omitempty"`
	Downloaded     int64        `json:"downloaded,omitempty"`
	Uploaded       int64        `json:"uploaded,omitempty"`
	Seeders        int          `json:"seeders,omitempty"`
	DownloadSpeed  int64        `json:"downloadSpeed,omitempty"`
	UploadSpeed    int64        `json:"uploadSpeed,omitempty"`
	UploadDate     int64        `json:"uploadDate,omitempty"`
	CompletionDate int64        `json:"completionDate,omitempty"`
	Files          []MagnetFile `json:"files,omitempty"`
}

// MagnetFile maps file leaf nodes from AllDebrid status response.
type MagnetFile struct {
	Name string `json:"n"`
	Size int64  `json:"s"`
	Link string `json:"l"`
}

type magnetFileNode struct {
	Name    string           `json:"n"`
	Size    int64            `json:"s,omitempty"`
	Link    string           `json:"l,omitempty"`
	Entries []magnetFileNode `json:"e,omitempty"`
}

type magnetStatusWire struct {
	ID             int             `json:"id"`
	Filename       string          `json:"filename,omitempty"`
	Size           int64           `json:"size,omitempty"`
	Status         string          `json:"status,omitempty"`
	StatusCode     int             `json:"statusCode,omitempty"`
	Downloaded     int64           `json:"downloaded,omitempty"`
	Uploaded       int64           `json:"uploaded,omitempty"`
	Seeders        int             `json:"seeders,omitempty"`
	DownloadSpeed  int64           `json:"downloadSpeed,omitempty"`
	UploadSpeed    int64           `json:"uploadSpeed,omitempty"`
	UploadDate     int64           `json:"uploadDate,omitempty"`
	CompletionDate int64           `json:"completionDate,omitempty"`
	Files          json.RawMessage `json:"files,omitempty"`
}

func (m *MagnetStatus) UnmarshalJSON(data []byte) error {
	var payload magnetStatusWire
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	m.ID = payload.ID
	m.Filename = payload.Filename
	m.Size = payload.Size
	m.Status = payload.Status
	m.StatusCode = payload.StatusCode
	m.Downloaded = payload.Downloaded
	m.Uploaded = payload.Uploaded
	m.Seeders = payload.Seeders
	m.DownloadSpeed = payload.DownloadSpeed
	m.UploadSpeed = payload.UploadSpeed
	m.UploadDate = payload.UploadDate
	m.CompletionDate = payload.CompletionDate

	files, err := flattenMagnetFiles(payload.Files)
	if err != nil {
		return err
	}
	m.Files = files

	return nil
}

func flattenMagnetFiles(raw json.RawMessage) ([]MagnetFile, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil, nil
	}

	var nodes []magnetFileNode
	switch trimmed[0] {
	case '[':
		if err := json.Unmarshal(trimmed, &nodes); err != nil {
			return nil, err
		}
	case '{':
		var single magnetFileNode
		if err := json.Unmarshal(trimmed, &single); err != nil {
			return nil, err
		}
		nodes = []magnetFileNode{single}
	default:
		return nil, fmt.Errorf("unexpected files payload shape")
	}

	flattened := make([]MagnetFile, 0)
	collectFileNodes(nodes, &flattened)
	return flattened, nil
}

func collectFileNodes(nodes []magnetFileNode, files *[]MagnetFile) {
	for _, node := range nodes {
		if len(node.Entries) > 0 {
			collectFileNodes(node.Entries, files)
		}

		if strings.TrimSpace(node.Link) == "" {
			continue
		}

		*files = append(*files, MagnetFile{
			Name: node.Name,
			Size: node.Size,
			Link: node.Link,
		})
	}
}

type UnlockLinkData struct {
	Link     string `json:"link"`
	Host     string `json:"host,omitempty"`
	FileName string `json:"filename,omitempty"`
	FileSize int64  `json:"filesize,omitempty"`
	Ready    bool   `json:"ready,omitempty"`
	ID       string `json:"id,omitempty"`
}

type DebridStreamResult struct {
	URL      string
	Filename string
	Size     int64
	Host     string
	Language string
	Indexer  string
}

// FileError represents the error details if a file is invalid.
type FileError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func getStreamLink(magnet string) (DebridStreamResult, error) {
	if !alldebridConfigured() {
		return DebridStreamResult{}, fmt.Errorf("alldebrid is not configured")
	}

	uploadResp, err := uploadMagnet(magnet)
	if err != nil {
		return DebridStreamResult{}, err
	}

	if len(uploadResp.Data.Magnets) == 0 {
		return DebridStreamResult{}, fmt.Errorf("upload succeeded but returned no magnets")
	}

	uploaded := uploadResp.Data.Magnets[0]
	magnetID := uploaded.ID
	if uploaded.Error != nil {
		if magnetID > 0 {
			if err := deleteMagnet(magnetID); err != nil {
				log.Printf("delete rejected magnet id=%d: %v", magnetID, err)
			}
		}
		return DebridStreamResult{}, allDebridAPIError("upload magnet", "error", uploaded.Error)
	}

	if magnetID == 0 {
		return DebridStreamResult{}, fmt.Errorf("upload succeeded but returned invalid magnet id")
	}

	hash := strings.ToLower(strings.TrimSpace(uploaded.Hash))
	if hash == "" {
		hash = magnetInfoHash(magnet)
	}
	if err := readyMagnets.track(magnetID, hash, time.Now()); err != nil {
		if deleteErr := deleteMagnet(magnetID); deleteErr != nil {
			log.Printf("delete untracked magnet id=%d: %v", magnetID, deleteErr)
		}
		return DebridStreamResult{}, fmt.Errorf("track uploaded magnet: %w", err)
	}

	keepMagnet := false
	defer func() {
		if keepMagnet {
			return
		}
		if err := deleteMagnet(magnetID); err != nil {
			log.Printf("delete failed magnet id=%d: %v", magnetID, err)
			return
		}
		readyMagnets.remove(magnetID)
	}()

	statusResp, err := magnetStatus(magnetID, magnetStatusReady)
	if err != nil {
		return DebridStreamResult{}, err
	}

	readyMagnet, ok := findMagnetByID(statusResp.Data.Magnets, magnetID)
	if !ok || (!strings.EqualFold(readyMagnet.Status, "ready") && readyMagnet.StatusCode != 4) {
		return DebridStreamResult{}, fmt.Errorf("magnet %d is not ready", magnetID)
	}
	if len(readyMagnet.Files) == 0 {
		return DebridStreamResult{}, fmt.Errorf("magnet %d is ready but has no files", magnetID)
	}

	selectedFile, ok := pickPrimaryFile(readyMagnet.Files)
	if !ok {
		return DebridStreamResult{}, fmt.Errorf("magnet %d returned no valid file links", magnetID)
	}

	unlockResp, err := unlockLink(selectedFile.Link)
	if err != nil {
		return DebridStreamResult{}, err
	}
	if strings.TrimSpace(unlockResp.Data.Link) == "" {
		return DebridStreamResult{}, fmt.Errorf("unlock link response did not include a stream link")
	}

	filename := strings.TrimSpace(unlockResp.Data.FileName)
	if filename == "" {
		filename = strings.TrimSpace(selectedFile.Name)
	}

	size := unlockResp.Data.FileSize
	if size <= 0 {
		size = selectedFile.Size
	}

	keepMagnet = true
	return DebridStreamResult{
		URL:      unlockResp.Data.Link,
		Filename: filename,
		Size:     size,
		Host:     strings.TrimSpace(unlockResp.Data.Host),
	}, nil
}

func findMagnetByID(magnets []MagnetStatus, id int) (MagnetStatus, bool) {
	for _, magnet := range magnets {
		if magnet.ID == id {
			return magnet, true
		}
	}
	return MagnetStatus{}, false
}

func pickPrimaryFile(files []MagnetFile) (MagnetFile, bool) {
	var selected MagnetFile
	found := false

	for _, file := range files {
		if strings.TrimSpace(file.Link) == "" {
			continue
		}
		if !found || file.Size > selected.Size {
			selected = file
			found = true
		}
	}

	return selected, found
}

func magnetStatus(id int, statusFilter string) (MagnetStatusResponse, error) {
	cfg := getConfig()
	baseURL, _ := url.Parse(allDebridBaseURL)
	baseURL.Path = magnetStatusPath

	form := url.Values{}
	if id > 0 {
		form.Set("id", strconv.Itoa(id))
	}
	if statusFilter != "" {
		form.Set("status", statusFilter)
	}

	encodedData := form.Encode()
	req, err := http.NewRequest(http.MethodPost, baseURL.String(), strings.NewReader(encodedData))
	if err != nil {
		return MagnetStatusResponse{}, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.AlldebridAPIKey))

	resp, err := httpClient.Do(req)
	if err != nil {
		return MagnetStatusResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return MagnetStatusResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return MagnetStatusResponse{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result MagnetStatusResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return MagnetStatusResponse{}, err
	}
	if result.Status != "success" {
		return MagnetStatusResponse{}, allDebridAPIError("magnet status failed", result.Status, result.Error)
	}

	return result, nil
}

func unlockLink(link string) (UnlockLinkRespose, error) {
	cfg := getConfig()
	baseURL, _ := url.Parse(allDebridBaseURL)
	baseURL.Path = unlockLinkPath
	form := url.Values{}
	form.Set("link", link)

	encodedData := form.Encode()

	req, err := http.NewRequest(http.MethodPost, baseURL.String(), strings.NewReader(encodedData))
	if err != nil {
		return UnlockLinkRespose{}, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.AlldebridAPIKey))
	resp, err := httpClient.Do(req)
	if err != nil {
		return UnlockLinkRespose{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UnlockLinkRespose{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return UnlockLinkRespose{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result UnlockLinkRespose
	if err := json.Unmarshal(body, &result); err != nil {
		return UnlockLinkRespose{}, err
	}

	if result.Status != "success" {
		return UnlockLinkRespose{}, allDebridAPIError("unlock link failed", result.Status, result.Error)
	}

	return result, nil
}

func uploadMagnet(magnet string) (UploadMagnetResponse, error) {
	cfg := getConfig()
	baseURL, _ := url.Parse(allDebridBaseURL)
	baseURL.Path = magnetUploadPath
	form := url.Values{}
	form.Set("agent", "Frankie")
	form.Set("magnets[]", magnet)

	encodedData := form.Encode()

	req, err := http.NewRequest(http.MethodPost, baseURL.String(), strings.NewReader(encodedData))
	if err != nil {
		return UploadMagnetResponse{}, err
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.AlldebridAPIKey))
	resp, err := httpClient.Do(req)
	if err != nil {
		return UploadMagnetResponse{}, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return UploadMagnetResponse{}, err
	}

	if resp.StatusCode != http.StatusOK {
		return UploadMagnetResponse{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	var result UploadMagnetResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return UploadMagnetResponse{}, err
	}

	if result.Status != "success" {
		return UploadMagnetResponse{}, allDebridAPIError("upload failed", result.Status, result.Error)
	}

	return result, nil
}

func alldebridConfigured() bool {
	cfg := getConfig()
	return strings.TrimSpace(cfg.AlldebridAPIKey) != ""
}
