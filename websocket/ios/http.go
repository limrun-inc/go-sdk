package ios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
)

// activeRecordingFilename is the server-side filename of the active recording.
const activeRecordingFilename = "recording.mp4"

// apiBaseURL parses the client's API URL.
func (c *Client) apiBaseURL() (*url.URL, error) {
	u, err := url.Parse(c.apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid API URL: %w", err)
	}
	return u, nil
}

// httpJSON performs an HTTP request with an optional JSON body and decodes an
// optional JSON response. Non-2xx responses are returned as errors that
// include the response body.
func (c *Client) httpJSON(ctx context.Context, method, rawURL string, reqBody, respBody any) error {
	var body io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s failed: %d %s", method, rawURL, resp.StatusCode, string(b))
	}
	if respBody != nil {
		if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}

// Cp copies a local file into the simulator's sandbox under the given name and
// returns the path usable in simctl commands.
func (c *Client) Cp(ctx context.Context, name, filePath string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}

	u, err := c.apiBaseURL()
	if err != nil {
		return "", err
	}
	u = u.JoinPath("files")
	q := u.Query()
	q.Set("name", name)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), f)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = info.Size()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed: %d %s", resp.StatusCode, string(b))
	}
	var result struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return result.Path, nil
}

// SetStoreKitConfig registers a StoreKit local-test configuration for an app on
// the simulator. The bytes are the contents of a .storekit file.
func (c *Client) SetStoreKitConfig(ctx context.Context, bundleID string, storekit []byte) error {
	u, err := c.apiBaseURL()
	if err != nil {
		return err
	}
	u = u.JoinPath("payments", "storeKitConfigs", bundleID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u.String(), bytes.NewReader(storekit))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.ContentLength = int64(len(storekit))

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("setStoreKitConfig failed: %d %s", resp.StatusCode, string(b))
	}
	return nil
}

// ClearStoreKitConfig clears the registered StoreKit configuration for the
// bundle. It is safe to call when nothing is registered.
func (c *Client) ClearStoreKitConfig(ctx context.Context, bundleID string) error {
	u, err := c.apiBaseURL()
	if err != nil {
		return err
	}
	u = u.JoinPath("payments", "storeKitConfigs", bundleID)
	return c.httpJSON(ctx, http.MethodDelete, u.String(), nil, nil)
}

// StoreKitDiscoverOptions configures DiscoverStoreKitConfig.
type StoreKitDiscoverOptions struct {
	// TimeoutSeconds is how long the server blocks while polling for a sandbox
	// response, clamped server-side to [1, 300]. A zero value uses the default
	// (120).
	TimeoutSeconds int
}

// StoreKitDiscoverResult is the result of auto-discovering a StoreKit
// configuration.
type StoreKitDiscoverResult struct {
	ItemsFound              int
	ProductsCount           int
	SubscriptionsCount      int
	SubscriptionGroupsCount int
}

// DiscoverStoreKitConfig auto-generates and registers a .storekit configuration
// for the bundle by polling the simulator's storekitd cache for a captured
// sandbox response. It blocks server-side for up to TimeoutSeconds.
func (c *Client) DiscoverStoreKitConfig(ctx context.Context, bundleID string, opts *StoreKitDiscoverOptions) (*StoreKitDiscoverResult, error) {
	timeoutSeconds := 120
	if opts != nil && opts.TimeoutSeconds != 0 {
		timeoutSeconds = opts.TimeoutSeconds
	}

	u, err := c.apiBaseURL()
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("payments", "storeKitConfigs", bundleID, "discover")

	reqBody := map[string]any{"timeoutSeconds": timeoutSeconds}
	var result struct {
		ItemsFound              int `json:"itemsFound"`
		ProductsCount           int `json:"productsCount"`
		SubscriptionsCount      int `json:"subscriptionsCount"`
		SubscriptionGroupsCount int `json:"subscriptionGroupsCount"`
	}
	if err := c.httpJSON(ctx, http.MethodPost, u.String(), reqBody, &result); err != nil {
		return nil, fmt.Errorf("discoverStoreKitConfig: %w", err)
	}
	return &StoreKitDiscoverResult{
		ItemsFound:              result.ItemsFound,
		ProductsCount:           result.ProductsCount,
		SubscriptionsCount:      result.SubscriptionsCount,
		SubscriptionGroupsCount: result.SubscriptionGroupsCount,
	}, nil
}

// SoftResetStrategy is the strategy used by SoftReset.
type SoftResetStrategy string

const (
	// SoftResetData terminates and wipes the app's data container only.
	SoftResetData SoftResetStrategy = "data"
	// SoftResetFull restores freshly-opened-simulator state (uninstall +
	// reinstall, clear keychain, privacy, NSUserDefaults cache, and shared App
	// Group containers).
	SoftResetFull SoftResetStrategy = "full"
)

// SoftResetOptions configures SoftReset.
type SoftResetOptions struct {
	// Strategy is the reset strategy. An empty value defaults to SoftResetData
	// server-side.
	Strategy SoftResetStrategy
}

// SoftResetResult is the result of a SoftReset call.
type SoftResetResult struct {
	// Strategy is the strategy that was actually applied.
	Strategy SoftResetStrategy
	// BundleID is the bundle ID that was reset.
	BundleID string
	// ItemsCleared is the number of top-level entries removed from the app's
	// data container. Only populated when the data strategy is used.
	ItemsCleared int
	// DurationMs is the wall-clock duration of the reset on the server.
	DurationMs float64
}

// SoftReset soft-resets an installed app on the simulator. Both strategies
// relaunch the app after the reset completes.
func (c *Client) SoftReset(ctx context.Context, bundleID string, opts *SoftResetOptions) (*SoftResetResult, error) {
	body := map[string]any{"bundleId": bundleID}
	if opts != nil && opts.Strategy != "" {
		body["strategy"] = opts.Strategy
	}

	u, err := c.apiBaseURL()
	if err != nil {
		return nil, err
	}
	u = u.JoinPath("softReset")

	var result struct {
		Strategy     SoftResetStrategy `json:"strategy"`
		BundleID     string            `json:"bundleId"`
		ItemsCleared int               `json:"itemsCleared"`
		DurationMs   float64           `json:"durationMs"`
	}
	if err := c.httpJSON(ctx, http.MethodPost, u.String(), body, &result); err != nil {
		return nil, fmt.Errorf("softReset: %w", err)
	}
	return &SoftResetResult{
		Strategy:     result.Strategy,
		BundleID:     result.BundleID,
		ItemsCleared: result.ItemsCleared,
		DurationMs:   result.DurationMs,
	}, nil
}

// StopRecording stops the active server-side recording. If saveTo.PresignedURL
// is set, the server uploads the completed file there before resolving. If
// saveTo.LocalPath is set, the client downloads the completed file to that
// path. It returns a download URL for the completed recording that is valid
// while the instance is running.
func (c *Client) StopRecording(ctx context.Context, saveTo SaveRecordingTo) (string, error) {
	req := &request{Type: "stopVideoRecording"}
	if saveTo.PresignedURL != "" {
		req.Upload = &recordingUpload{PresignedURL: saveTo.PresignedURL}
	}
	if _, err := c.sendRequest(ctx, req); err != nil {
		return "", err
	}

	u, err := c.apiBaseURL()
	if err != nil {
		return "", err
	}
	u = u.JoinPath("files")
	q := u.Query()
	q.Set("name", activeRecordingFilename)
	u.RawQuery = q.Encode()
	downloadURL := u.String()

	if saveTo.LocalPath != "" {
		if err := c.downloadFile(ctx, downloadURL, saveTo.LocalPath); err != nil {
			return "", err
		}
	}
	return downloadURL, nil
}

// downloadFile downloads rawURL (with bearer auth) to localPath.
func (c *Client) downloadFile(ctx context.Context, rawURL, localPath string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed: %d %s", resp.StatusCode, string(b))
	}
	f, err := os.Create(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := io.Copy(f, resp.Body); err != nil {
		return err
	}
	return nil
}
