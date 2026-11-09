// A deliberately simple file-backed signing-secret store: one JSON file per
// secret. It exists to show that the frontend's SigningSecretStore interface
// can be backed by anything; swap this file for a database, KMS, or Limrun's
// organization secret store without touching the wizard. The file format
// matches the TypeScript examples, so both can share one directory.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type secret struct {
	Type      string            `json:"type"`
	Name      string            `json:"name"`
	Data      map[string]string `json:"data,omitempty"`
	CreatedAt string            `json:"createdAt"`
}

type secretStore struct {
	// The directory used when a request names none; the UI shows it as the
	// default of its secrets-directory field.
	directory string
	mu        sync.Mutex
}

func newSecretStore(defaultDir string) (*secretStore, error) {
	if configured := os.Getenv("SECRETS_DIR"); configured != "" {
		defaultDir = configured
	}
	absolute, err := filepath.Abs(defaultDir)
	return &secretStore{directory: absolute}, err
}

// Every operation accepts an explicit directory, which the UI sends so the
// device-install example can share one store.
func (s *secretStore) resolve(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return s.directory
	}
	return dir
}

// Match encodeURIComponent in the TypeScript secret store, including spaces.
func encodeComponent(value string) string {
	encoded := strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
	return strings.NewReplacer("%21", "!", "%27", "'", "%28", "(", "%29", ")", "%2A", "*", "%7E", "~").Replace(encoded)
}

// Secret names contain slashes (TEAMID/DISTRIBUTION), so the file name is the
// encoded type and name pair.
func (s *secretStore) file(kind, name, dir string) string {
	return filepath.Join(s.resolve(dir), encodeComponent(kind)+"__"+encodeComponent(name)+".json")
}

func (s *secretStore) read(kind, name, dir string) (*secret, error) {
	data, err := os.ReadFile(s.file(kind, name, dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var stored secret
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	return &stored, nil
}

func (s *secretStore) get(kind, name, dir string) (*secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.read(kind, name, dir)
}

func (s *secretStore) put(kind, name string, data map[string]string, dir string) (*secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.resolve(dir), 0700); err != nil {
		return nil, err
	}
	existing, err := s.read(kind, name, dir)
	if err != nil {
		return nil, err
	}
	stored := &secret{Type: kind, Name: name, Data: data, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if existing != nil {
		stored.CreatedAt = existing.CreatedAt
	}
	encoded, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.file(kind, name, dir), encoded, 0600); err != nil {
		return nil, err
	}
	return stored, nil
}

func (s *secretStore) list(dir string) ([]secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.resolve(dir))
	if errors.Is(err, os.ErrNotExist) {
		return []secret{}, nil
	}
	if err != nil {
		return nil, err
	}
	secrets := []secret{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.resolve(dir), entry.Name()))
		if err != nil {
			return nil, err
		}
		var stored secret
		if err := json.Unmarshal(data, &stored); err != nil {
			return nil, err
		}
		secrets = append(secrets, stored)
	}
	return secrets, nil
}

func (s *secretStore) delete(kind, name, dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.file(kind, name, dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// register exposes the store with the same response shape as Limrun's
// organization secrets API, so the frontend's SigningSecretStore is a thin
// fetch wrapper. The optional dir query parameter selects the directory.
func (s *secretStore) register(mux *http.ServeMux) {
	// Metadata only: secret data never appears in listings.
	mux.HandleFunc("GET /secrets", func(w http.ResponseWriter, r *http.Request) {
		secrets, err := s.list(r.URL.Query().Get("dir"))
		if err != nil {
			writeError(w, 500, err)
			return
		}
		for i := range secrets {
			secrets[i].Data = nil
		}
		writeJSON(w, 200, secrets)
	})
	mux.HandleFunc("GET /secrets/{type}/{name}", func(w http.ResponseWriter, r *http.Request) {
		stored, err := s.get(r.PathValue("type"), r.PathValue("name"), r.URL.Query().Get("dir"))
		if err != nil {
			writeError(w, 500, err)
			return
		}
		if stored == nil {
			writeError(w, 404, fmt.Errorf("secret not found"))
			return
		}
		writeJSON(w, 200, stored)
	})
	mux.HandleFunc("PUT /secrets/{type}/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Data map[string]string `json:"data"`
		}
		if err := decodeJSON(w, r, &body); err != nil {
			writeError(w, 400, err)
			return
		}
		if body.Data == nil {
			writeError(w, 400, fmt.Errorf("body must contain a data object"))
			return
		}
		stored, err := s.put(r.PathValue("type"), r.PathValue("name"), body.Data, r.URL.Query().Get("dir"))
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, stored)
	})
	mux.HandleFunc("DELETE /secrets/{type}/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.delete(r.PathValue("type"), r.PathValue("name"), r.URL.Query().Get("dir")); err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, 200, map[string]string{"status": "success"})
	})
}

// unexpired treats a missing date as valid and an unparseable one as expired,
// like the TypeScript publish flow does.
func unexpired(value string) bool {
	if value == "" {
		return true
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.After(time.Now())
		}
	}
	return false
}

func includes(values, value string) bool {
	for _, item := range strings.Split(values, ",") {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}

// signingCredentials selects the distribution certificate and the newest
// current App Store profile for the exact team, bundle and certificate.
// Device-bound profiles belong to the device-install example and are skipped.
func (s *secretStore) signingCredentials(request publishRequest) (*secret, *secret, error) {
	certificate, err := s.get("appleCertificate", request.TeamID+"/DISTRIBUTION", request.SecretsDir)
	if err != nil {
		return nil, nil, err
	}
	if certificate == nil || !unexpired(certificate.Data["expirationDate"]) || certificate.Data["serialNumber"] == "" {
		return nil, nil, fmt.Errorf("no valid distribution certificate for team %s; run Connect first", request.TeamID)
	}
	secrets, err := s.list(request.SecretsDir)
	if err != nil {
		return nil, nil, err
	}
	sort.SliceStable(secrets, func(i, j int) bool { return secrets[i].CreatedAt > secrets[j].CreatedAt })
	for i := range secrets {
		profile, data := &secrets[i], secrets[i].Data
		if profile.Type != "appleProvisioningProfile" || data["teamID"] != request.TeamID || !includes(data["bundleIDs"], request.BundleID) || !includes(data["certificateSerialNumbers"], certificate.Data["serialNumber"]) || !unexpired(data["expirationDate"]) || data["deviceIDs"] != "" {
			continue
		}
		return certificate, profile, nil
	}
	return nil, nil, fmt.Errorf("no matching current provisioning profile for %s; run Connect first", request.BundleID)
}
