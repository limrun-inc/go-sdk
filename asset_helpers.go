package limrun

import (
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/limrun-inc/go-sdk/internal/proxy"

	"github.com/limrun-inc/go-sdk/build"
	"github.com/limrun-inc/go-sdk/option"
	"github.com/limrun-inc/go-sdk/packages/param"
)

type AssetUploadOptions = build.AssetUploadMetadata

type AssetGetOrUploadParams struct {
	// Name defaults to the file's base name when omitted.
	Name          param.Opt[string]
	Path          string
	Ttl           param.Opt[string]
	Kind          AssetGetOrNewParamsKind
	Platform      AssetGetOrNewParamsPlatform
	UploadOptions *AssetUploadOptions
	// OnUploadProgress is called only when content is uploaded.
	OnUploadProgress func(uploadedBytes, totalBytes int64)
	// ProgressWriter receives the uploaded bytes for compatibility with earlier SDKs.
	ProgressWriter io.Writer
}

// GetOrUpload records the asset metadata and uploads content unless its MD5 already matches.
func (r *AssetService) GetOrUpload(ctx context.Context, body AssetGetOrUploadParams, opts ...option.RequestOption) (*AssetGetOrNewResponse, error) {
	name := filepath.Base(body.Path)
	if body.Name.Valid() {
		name = body.Name.Value
	}
	kind := body.Kind
	if kind == "" {
		kind = AssetGetOrNewParamsKindApp
	}
	params := AssetGetOrNewParams{Name: name, Kind: kind, Platform: body.Platform, Ttl: body.Ttl}
	if m := body.UploadOptions; m != nil {
		if m.DisplayName != nil {
			params.DisplayName = param.NewOpt(*m.DisplayName)
		}
		if m.BundleIdentifier != nil {
			params.BundleIdentifier = param.NewOpt(*m.BundleIdentifier)
		}
		if m.ShortVersion != nil {
			params.ShortVersion = param.NewOpt(*m.ShortVersion)
		}
		if m.BuildVersion != nil {
			params.BuildVersion = param.NewOpt(*m.BuildVersion)
		}
		if m.Deeplink != nil {
			params.Deeplink = param.NewOpt(*m.Deeplink)
		}
	}
	result, err := r.GetOrNew(ctx, params, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to put asset: %w", err)
	}
	file, err := os.Open(body.Path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	// Hash and upload in bounded chunks, so large bundles do not have to fit in
	// memory. Progress observers see only the upload, not this checksum pass.
	hash := md5.New()
	size, err := io.Copy(hash, file)
	if err != nil {
		return nil, fmt.Errorf("failed to calculate asset MD5: %w", err)
	}
	md5sum := fmt.Sprintf("%x", hash.Sum(nil))
	if result.Md5 != "" && result.Md5 == md5sum {
		return result, nil
	}
	var reader io.Reader = io.NewSectionReader(file, 0, size)
	if body.ProgressWriter != nil {
		reader = io.TeeReader(reader, body.ProgressWriter)
	}
	if body.OnUploadProgress != nil {
		reader = &assetUploadProgressReader{reader: reader, total: size, onProgress: body.OnUploadProgress}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, result.SignedUploadURL, reader)
	if err != nil {
		return nil, err
	}
	req.ContentLength = size
	if size == 0 {
		// A non-nil body with ContentLength 0 otherwise means an unknown length
		// to net/http, causing chunked encoding instead of Content-Length: 0.
		req.Body = http.NoBody
		req.GetBody = func() (io.ReadCloser, error) { return http.NoBody, nil }
	} else if body.ProgressWriter == nil && body.OnUploadProgress == nil {
		// Preserve the replay support that bytes.Reader provided for redirects.
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(io.NewSectionReader(file, 0, size)), nil
		}
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := proxy.Client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to upload asset: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode < http.StatusOK || res.StatusCode >= http.StatusMultipleChoices {
		text, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("failed to upload asset: %d %s", res.StatusCode, text)
	}
	result.Md5 = md5sum
	return result, nil
}

type assetUploadProgressReader struct {
	reader          io.Reader
	uploaded, total int64
	onProgress      func(int64, int64)
}

func (r *assetUploadProgressReader) Read(p []byte) (int, error) {
	if len(p) > 256*1024 {
		p = p[:256*1024]
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		r.uploaded += int64(n)
		r.onProgress(r.uploaded, r.total)
	}
	return n, err
}
