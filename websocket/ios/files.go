package ios

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type ContainerTargetOptions struct {
	BundleID      string
	ContainerType string
}
type FileEntry struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	IsDirectory bool   `json:"isDirectory"`
	Size        *int64 `json:"size,omitempty"`
}

func (c *Client) fileURL(endpoint, key, value string, target *ContainerTargetOptions) (string, error) {
	u, err := c.apiBaseURL()
	if err != nil {
		return "", err
	}
	u = u.JoinPath(endpoint)
	q := u.Query()
	if key != "" {
		q.Set(key, value)
	}
	if target != nil && target.BundleID != "" {
		q.Set("bundleId", target.BundleID)
		if target.ContainerType != "" {
			q.Set("containerType", target.ContainerType)
		}
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
func (c *Client) PushFile(ctx context.Context, path, destination string, target *ContainerTargetOptions) (string, error) {
	u, err := c.fileURL("files", "name", destination, target)
	if err != nil {
		return "", err
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, f)
	if err != nil {
		return "", err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		return "", fmt.Errorf("upload failed: %d %s", res.StatusCode, b)
	}
	var result struct {
		Path string `json:"path"`
	}
	err = json.NewDecoder(res.Body).Decode(&result)
	return result.Path, err
}
func (c *Client) PullFile(ctx context.Context, name string, target *ContainerTargetOptions) ([]byte, error) {
	u, err := c.fileURL("files", "name", name, target)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return nil, fmt.Errorf("download of %q failed: %d %s", name, res.StatusCode, body)
	}
	return body, nil
}
func (c *Client) ListFiles(ctx context.Context, path string, target *ContainerTargetOptions) ([]FileEntry, error) {
	key := "path"
	if path == "" || path == "." {
		key = ""
	}
	u, err := c.fileURL("files/list", key, path, target)
	if err != nil {
		return nil, err
	}
	var result struct {
		Entries []FileEntry `json:"entries"`
	}
	err = c.httpJSON(ctx, http.MethodGet, u, nil, &result)
	return result.Entries, err
}
func (c *Client) DeleteFile(ctx context.Context, name string, target *ContainerTargetOptions) error {
	u, err := c.fileURL("files", "name", name, target)
	if err != nil {
		return err
	}
	return c.httpJSON(ctx, http.MethodDelete, u, nil, nil)
}
func (c *Client) SetCameraVideo(ctx context.Context, path string, loop ...bool) error {
	remote, err := c.PushFile(ctx, path, "limrun-camera-"+filepath.Base(path), nil)
	if err != nil {
		return err
	}
	repeats := true
	if len(loop) > 0 {
		repeats = loop[0]
	}
	return c.call(ctx, "cameraControl", map[string]any{"action": "setSource", "source": "video", "arg": remote, "loop": repeats}, nil, 0)
}
func (c *Client) ClearCameraVideo(ctx context.Context) error {
	return c.call(ctx, "cameraControl", map[string]any{"action": "reset"}, nil, 0)
}

type TrustedCACertificate struct {
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
}

func (c *Client) AddCACertificate(ctx context.Context, pem string) (*TrustedCACertificate, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(c.apiURL, "/")+"/trusted-cas", strings.NewReader(pem))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-pem-file")
	req.Header.Set("Authorization", "Bearer "+c.token)
	res, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(res.Body)
		var body struct{ Message, Reason string }
		reason := strings.TrimSpace(string(b))
		if json.Unmarshal(b, &body) == nil {
			if body.Message != "" {
				reason = body.Message
			} else if body.Reason != "" {
				reason = body.Reason
			}
		}
		return nil, fmt.Errorf("adding the CA certificate failed (%d): %s", res.StatusCode, reason)
	}
	var result TrustedCACertificate
	err = json.NewDecoder(res.Body).Decode(&result)
	return &result, err
}
