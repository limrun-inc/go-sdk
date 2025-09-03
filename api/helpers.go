package api

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
)

// PutAndUploadAsset makes sure the Asset is created and given file is uploaded. If the asset already exists, we compare
// the MD5 of the uploaded file with our local file to prevent uploading the same file.
// If the local file is different, we upload and override the existing file in the asset storage.
func (c *Client) PutAndUploadAsset(ctx context.Context, filePath string, params PutAssetParams) (*Asset, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result, err := c.PutAsset(ctx, &AssetPut{
		Name: path.Base(filePath),
	}, params)
	if err != nil {
		return nil, fmt.Errorf("failed to put asset: %w", err)
	}
	// No need to override if the same file was already uploaded, verified by md5.
	if result.MD5.IsSet() {
		hasher := md5.New()
		if _, err := io.Copy(hasher, file); err != nil {
			return nil, fmt.Errorf("failed to calculate MD5: %w", err)
		}
		localMd5Hex := fmt.Sprintf("%x", hasher.Sum(nil))
		if localMd5Hex == result.MD5.Value {
			return result, nil
		}
		// If it's a different file, then we'll seek back to start for upload to stream from beginning.
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to reset file pointer: %w", err)
		}
	}
	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPut, result.SignedUploadUrl.Value, file)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}
	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}
	uploadReq.ContentLength = stat.Size()
	uploadReq.Header.Set("Content-Type", "application/octet-stream")
	resp, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		return nil, fmt.Errorf("failed to execute upload request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to upload asset: %s %s", resp.Status, string(body))
	}
	return result, nil
}
