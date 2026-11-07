package main

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

const (
	readyMagnetRetention   = 24 * time.Hour
	magnetCleanupInterval  = time.Hour
	magnetRegistryFilename = "frankie_magnets.json"
)

type trackedMagnet struct {
	ID       int       `json:"id"`
	Hash     string    `json:"hash,omitempty"`
	LastUsed time.Time `json:"last_used"`
}

type magnetRegistry struct {
	mu      sync.Mutex
	path    string
	magnets map[string]trackedMagnet
}

var readyMagnets = newMagnetRegistry(filepath.Join(filepath.Dir(configFilePath()), magnetRegistryFilename))

func newMagnetRegistry(path string) *magnetRegistry {
	registry := &magnetRegistry{
		path:    path,
		magnets: make(map[string]trackedMagnet),
	}
	body, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			log.Printf("read magnet registry %s: %v", path, err)
		}
		return registry
	}
	if err := json.Unmarshal(body, &registry.magnets); err != nil {
		log.Printf("decode magnet registry %s: %v", path, err)
		registry.magnets = make(map[string]trackedMagnet)
	}
	return registry
}

func (r *magnetRegistry) track(id int, hash string, lastUsed time.Time) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	key := strconv.Itoa(id)
	previous, existed := r.magnets[key]
	r.magnets[key] = trackedMagnet{ID: id, Hash: hash, LastUsed: lastUsed}
	if err := r.saveLocked(); err != nil {
		if existed {
			r.magnets[key] = previous
		} else {
			delete(r.magnets, key)
		}
		return err
	}
	return nil
}

func (r *magnetRegistry) remove(id int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.magnets, strconv.Itoa(id))
	if err := r.saveLocked(); err != nil {
		log.Printf("save magnet registry %s: %v", r.path, err)
	}
}

func (r *magnetRegistry) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(r.magnets, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')

	temporary, err := os.CreateTemp(filepath.Dir(r.path), ".frankie-magnets-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, r.path)
}

func (r *magnetRegistry) cleanupExpired(now time.Time) {
	r.mu.Lock()
	expired := make([]trackedMagnet, 0)
	for _, magnet := range r.magnets {
		if magnet.LastUsed.IsZero() || now.Sub(magnet.LastUsed) >= readyMagnetRetention {
			expired = append(expired, magnet)
		}
	}
	r.mu.Unlock()

	for _, magnet := range expired {
		if err := deleteMagnet(magnet.ID); err != nil {
			log.Printf("delete expired magnet id=%d: %v", magnet.ID, err)
			continue
		}
		r.remove(magnet.ID)
		log.Printf("deleted expired Frankie magnet id=%d", magnet.ID)
	}
}

func runMagnetCleanup() {
	readyMagnets.cleanupExpired(time.Now())
	ticker := time.NewTicker(magnetCleanupInterval)
	defer ticker.Stop()
	for now := range ticker.C {
		readyMagnets.cleanupExpired(now)
	}
}
