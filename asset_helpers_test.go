package limrun_test

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	limrun "github.com/limrun-inc/go-sdk"
	"github.com/limrun-inc/go-sdk/option"
)

func assetUploadClient(t testing.TB, upload http.HandlerFunc) limrun.Client {
	t.Helper()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/assets" {
			upload(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"id": "asset_test", "name": "app.zip", "signedUploadUrl": server.URL + "/upload",
		})
	}))
	t.Cleanup(server.Close)
	return limrun.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("test"))
}

func assetFile(t testing.TB, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "app.zip")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestGetOrUploadResponseCodes(t *testing.T) {
	data := []byte("a simulator app bundle")
	path := assetFile(t, data)
	for _, status := range []int{200, 201, 204, 299, 400, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := assetUploadClient(t, func(w http.ResponseWriter, r *http.Request) {
				got, err := io.ReadAll(r.Body)
				if err != nil || !bytes.Equal(got, data) {
					t.Errorf("upload=%q, err=%v", got, err)
				}
				w.WriteHeader(status)
			})
			result, err := client.Assets.GetOrUpload(context.Background(), limrun.AssetGetOrUploadParams{Path: path})
			if status >= 300 {
				if err == nil || !strings.Contains(err.Error(), fmt.Sprint(status)) || result != nil {
					t.Fatalf("failed upload: result=%+v, err=%v", result, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := fmt.Sprintf("%x", md5.Sum(data)); result.Md5 != want {
				t.Fatalf("MD5=%s, want %s", result.Md5, want)
			}
		})
	}
}

func TestGetOrUploadEmptyFile(t *testing.T) {
	client := assetUploadClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			t.Errorf("empty upload has length=%d, encoding=%v", r.ContentLength, r.TransferEncoding)
		}
		if n, err := io.Copy(io.Discard, r.Body); n != 0 || err != nil {
			t.Errorf("empty body: bytes=%d, err=%v", n, err)
		}
		w.WriteHeader(http.StatusNoContent)
	})
	result, err := client.Assets.GetOrUpload(context.Background(), limrun.AssetGetOrUploadParams{Path: assetFile(t, nil)})
	if err != nil {
		t.Fatal(err)
	}
	if result.Md5 != fmt.Sprintf("%x", md5.Sum(nil)) {
		t.Fatalf("empty file MD5=%s", result.Md5)
	}
}

func TestGetOrUploadStreamingProgress(t *testing.T) {
	data := bytes.Repeat([]byte("bundle data\n"), 100000)
	checksum := fmt.Sprintf("%x", md5.Sum(data))
	client := assetUploadClient(t, func(w http.ResponseWriter, r *http.Request) {
		hash := md5.New()
		n, err := io.Copy(hash, r.Body)
		if err != nil || n != int64(len(data)) || fmt.Sprintf("%x", hash.Sum(nil)) != checksum {
			t.Errorf("streamed body: bytes=%d, err=%v", n, err)
		}
		if r.ContentLength != int64(len(data)) || r.Header.Get("Content-Type") != "application/octet-stream" || len(r.TransferEncoding) != 0 {
			t.Errorf("upload framing: length=%d, encoding=%v, headers=%v", r.ContentLength, r.TransferEncoding, r.Header)
		}
		w.WriteHeader(http.StatusCreated)
	})
	var mu sync.Mutex
	var progress [][2]int64
	observed := md5.New()
	result, err := client.Assets.GetOrUpload(context.Background(), limrun.AssetGetOrUploadParams{
		Path: assetFile(t, data), ProgressWriter: observed,
		OnUploadProgress: func(uploaded, total int64) {
			mu.Lock()
			defer mu.Unlock()
			progress = append(progress, [2]int64{uploaded, total})
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if result.Md5 != checksum || fmt.Sprintf("%x", observed.Sum(nil)) != checksum {
		t.Fatal("upload or ProgressWriter received different file content")
	}
	if len(progress) < 2 {
		t.Fatalf("expected incremental progress, got %v", progress)
	}
	previous := int64(0)
	for _, p := range progress {
		if p[0] <= previous || p[0]-previous > 256*1024 || p[0] > p[1] || p[1] != int64(len(data)) {
			t.Fatalf("invalid progress %v after %d bytes", p, previous)
		}
		previous = p[0]
	}
	if previous != int64(len(data)) {
		t.Fatalf("progress ended at %d bytes, want %d", previous, len(data))
	}
}

func TestGetOrUploadReplaysRedirect(t *testing.T) {
	data := []byte("redirected bundle")
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			client := assetUploadClient(t, func(w http.ResponseWriter, r *http.Request) {
				got, err := io.ReadAll(r.Body)
				if err != nil || !bytes.Equal(got, data) || r.Method != http.MethodPut || r.ContentLength != int64(len(data)) {
					t.Errorf("redirect body=%q, method=%s, length=%d, err=%v", got, r.Method, r.ContentLength, err)
				}
				if r.URL.Path == "/upload" {
					w.Header().Set("Location", "/redirected")
					w.WriteHeader(status)
					return
				}
				w.WriteHeader(http.StatusNoContent)
			})
			_, err := client.Assets.GetOrUpload(context.Background(), limrun.AssetGetOrUploadParams{Path: assetFile(t, data)})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

// Compare allocations across file sizes to catch accidental full-file buffering.
func BenchmarkGetOrUpload(b *testing.B) {
	for _, size := range []int64{1 << 20, 64 << 20} {
		b.Run(fmt.Sprintf("%dMiB", size>>20), func(b *testing.B) {
			path := assetFile(b, nil)
			if err := os.Truncate(path, size); err != nil {
				b.Fatal(err)
			}
			client := assetUploadClient(b, func(w http.ResponseWriter, r *http.Request) {
				if n, err := io.Copy(io.Discard, r.Body); err != nil || n != size {
					b.Errorf("uploaded=%d, want %d, err=%v", n, size, err)
				}
				w.WriteHeader(http.StatusOK)
			})
			b.SetBytes(size)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := client.Assets.GetOrUpload(context.Background(), limrun.AssetGetOrUploadParams{Path: path}); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
