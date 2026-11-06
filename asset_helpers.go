package limrun

import (
	"bytes"
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
	data, err := os.ReadFile(body.Path)
	if err != nil {
		return nil, err
	}
	md5sum := fmt.Sprintf("%x", md5.Sum(data))
	if result.Md5 != "" && result.Md5 == md5sum {
		return result, nil
	}
	var reader io.Reader = bytes.NewReader(data)
	if body.ProgressWriter != nil {
		reader = io.TeeReader(reader, body.ProgressWriter)
	}
	if body.OnUploadProgress != nil {
		reader = &assetUploadProgressReader{reader: reader, total: int64(len(data)), onProgress: body.OnUploadProgress}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, result.SignedUploadURL, reader)
	if err != nil {
		return nil, err
	}
	req.ContentLength = int64(len(data))
	req.Header.Set("Content-Type", "application/octet-stream")
	res, err := proxy.Client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to upload asset: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
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
