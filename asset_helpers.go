package limrun

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"

	"github.com/limrun-inc/go-sdk/option"
)

type AssetGetOrUploadParams struct {
	// Name of the asset. If not given, file name of the given file is used.
	Name *string

	// Path to the file to upload.
	Path string
}

// GetOrUpload makes sure the given file is either uploaded or validates that it was uploaded and return a working
// download URL in either case.
func (r *AssetService) GetOrUpload(ctx context.Context, body AssetGetOrUploadParams, opts ...option.RequestOption) (res *AssetGetOrNewResponse, err error) {
	name := path.Base(body.Path)
	if body.Name != nil && len(*body.Name) > 0 {
		name = *body.Name
	}
	localFile, err := os.Open(body.Path)
	if err != nil {
		return nil, err
	}
	defer localFile.Close()
	result, err := r.GetOrNew(ctx, AssetGetOrNewParams{
		Name: name,
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to put asset: %w", err)
	}
	// No need to override if the same localFile was already uploaded, verified by md5.
	if len(result.Md5) > 0 {
		hasher := md5.New()
		if _, err := io.Copy(hasher, localFile); err != nil {
			return nil, fmt.Errorf("failed to calculate MD5: %w", err)
		}
		localMd5Hex := fmt.Sprintf("%x", hasher.Sum(nil))
		if localMd5Hex == result.Md5 {
			return result, nil
		}
		// If it's a different localFile, then we'll seek back to start for upload to stream from beginning.
		if _, err := localFile.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("failed to reset localFile pointer: %w", err)
		}
	}
	uploadReq, err := http.NewRequestWithContext(ctx, http.MethodPut, result.SignedUploadURL, localFile)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload request: %w", err)
	}
	stat, err := localFile.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get localFile info: %w", err)
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
