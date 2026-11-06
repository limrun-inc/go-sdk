// Package storebackend contains the local secret store and CLI workflows shared
// by the device-install and publish-to-stores browser examples.
package storebackend

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

	"github.com/limrun-inc/go-sdk/examples/internal/exampleutil"
)

type Secret struct {
	Type      string            `json:"type"`
	Name      string            `json:"name"`
	Data      map[string]string `json:"data,omitempty"`
	CreatedAt string            `json:"createdAt"`
}
type SecretStore struct {
	Directory string
	mu        sync.Mutex
}

func NewSecretStore(defaultDir string) (*SecretStore, error) {
	if configured := os.Getenv("SECRETS_DIR"); configured != "" {
		defaultDir = configured
	}
	absolute, err := filepath.Abs(defaultDir)
	return &SecretStore{Directory: absolute}, err
}
func (s *SecretStore) directory(dir string) string {
	if strings.TrimSpace(dir) == "" {
		return s.Directory
	}
	return dir
}

// Match encodeURIComponent in the TypeScript secret store, including spaces.
func encodeComponent(value string) string {
	encoded := strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
	return strings.NewReplacer("%21", "!", "%27", "'", "%28", "(", "%29", ")", "%2A", "*", "%7E", "~").Replace(encoded)
}
func (s *SecretStore) file(kind, name, dir string) string {
	return filepath.Join(s.directory(dir), encodeComponent(kind)+"__"+encodeComponent(name)+".json")
}
func (s *SecretStore) get(kind, name, dir string) (*Secret, error) {
	data, err := os.ReadFile(s.file(kind, name, dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var secret Secret
	if err := json.Unmarshal(data, &secret); err != nil {
		return nil, err
	}
	return &secret, nil
}
func (s *SecretStore) Get(kind, name, dir string) (*Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.get(kind, name, dir)
}
func (s *SecretStore) Put(kind, name string, data map[string]string, dir string) (*Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(s.directory(dir), 0700); err != nil {
		return nil, err
	}
	existing, err := s.get(kind, name, dir)
	if err != nil {
		return nil, err
	}
	secret := &Secret{Type: kind, Name: name, Data: data, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if existing != nil {
		secret.CreatedAt = existing.CreatedAt
	}
	encoded, err := json.MarshalIndent(secret, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(s.file(kind, name, dir), encoded, 0600); err != nil {
		return nil, err
	}
	return secret, nil
}
func (s *SecretStore) List(dir string) ([]Secret, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := os.ReadDir(s.directory(dir))
	if errors.Is(err, os.ErrNotExist) {
		return []Secret{}, nil
	}
	if err != nil {
		return nil, err
	}
	secrets := []Secret{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(s.directory(dir), entry.Name()))
		if err != nil {
			return nil, err
		}
		var secret Secret
		if err := json.Unmarshal(data, &secret); err != nil {
			return nil, err
		}
		secrets = append(secrets, secret)
	}
	return secrets, nil
}
func (s *SecretStore) Delete(kind, name, dir string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.file(kind, name, dir))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
func (s *SecretStore) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /secrets", func(w http.ResponseWriter, r *http.Request) {
		secrets, err := s.List(r.URL.Query().Get("dir"))
		if err != nil {
			exampleutil.Error(w, 500, err)
			return
		}
		for i := range secrets {
			secrets[i].Data = nil
		}
		exampleutil.JSON(w, 200, secrets)
	})
	mux.HandleFunc("GET /secrets/{type}/{name}", func(w http.ResponseWriter, r *http.Request) {
		secret, err := s.Get(r.PathValue("type"), r.PathValue("name"), r.URL.Query().Get("dir"))
		if err != nil {
			exampleutil.Error(w, 500, err)
			return
		}
		if secret == nil {
			exampleutil.Error(w, 404, fmt.Errorf("secret not found"))
			return
		}
		exampleutil.JSON(w, 200, secret)
	})
	mux.HandleFunc("PUT /secrets/{type}/{name}", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Data map[string]string `json:"data"`
		}
		if err := exampleutil.Decode(w, r, &body); err != nil {
			exampleutil.Error(w, 400, err)
			return
		}
		if body.Data == nil {
			exampleutil.Error(w, 400, fmt.Errorf("body must contain a data object"))
			return
		}
		secret, err := s.Put(r.PathValue("type"), r.PathValue("name"), body.Data, r.URL.Query().Get("dir"))
		if err != nil {
			exampleutil.Error(w, 500, err)
			return
		}
		exampleutil.JSON(w, 200, secret)
	})
	mux.HandleFunc("DELETE /secrets/{type}/{name}", func(w http.ResponseWriter, r *http.Request) {
		if err := s.Delete(r.PathValue("type"), r.PathValue("name"), r.URL.Query().Get("dir")); err != nil {
			exampleutil.Error(w, 500, err)
			return
		}
		exampleutil.JSON(w, 200, map[string]string{"status": "success"})
	})
}
func unexpired(value string, allowInvalid bool) bool {
	if value == "" {
		return true
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.After(time.Now())
		}
	}
	return allowInvalid
}
func includes(values, value string) bool {
	for _, item := range strings.Split(values, ",") {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}
func normalizeUDID(value string) string {
	return strings.Map(func(c rune) rune {
		if c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' {
			return c
		}
		return -1
	}, strings.ToUpper(value))
}
func hasDevice(raw, udid string) bool {
	var devices []struct {
		UDID string `json:"udid"`
	}
	if json.Unmarshal([]byte(raw), &devices) != nil {
		return false
	}
	for _, device := range devices {
		if device.UDID != "" && normalizeUDID(device.UDID) == normalizeUDID(udid) {
			return true
		}
	}
	return false
}

// Select a current profile for the exact team, bundle, certificate and (for OTA) device.
func (s *SecretStore) SigningCredentials(request Request, device bool) (*Secret, *Secret, error) {
	certificate, err := s.Get("appleCertificate", request.TeamID+"/DISTRIBUTION", request.SecretsDir)
	if err != nil {
		return nil, nil, err
	}
	if certificate == nil || !unexpired(certificate.Data["expirationDate"], device) || certificate.Data["serialNumber"] == "" {
		return nil, nil, fmt.Errorf("no valid distribution certificate for team %s; run Connect first", request.TeamID)
	}
	secrets, err := s.List(request.SecretsDir)
	if err != nil {
		return nil, nil, err
	}
	sort.SliceStable(secrets, func(i, j int) bool { return secrets[i].CreatedAt > secrets[j].CreatedAt })
	for _, profile := range secrets {
		data := profile.Data
		if profile.Type != "appleProvisioningProfile" || data["teamID"] != request.TeamID || !includes(data["bundleIDs"], request.BundleID) || !includes(data["certificateSerialNumbers"], certificate.Data["serialNumber"]) || !unexpired(data["expirationDate"], device) {
			continue
		}
		if device {
			if !hasDevice(data["deviceIDs"], request.DeviceUDID) {
				continue
			}
		} else if data["deviceIDs"] != "" {
			continue
		}
		return certificate, &profile, nil
	}
	return nil, nil, fmt.Errorf("no matching current provisioning profile for %s; run Connect first", request.BundleID)
}
