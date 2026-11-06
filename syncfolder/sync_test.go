package syncfolder

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestSyncRecoversMissingRemoteBasisAndPreservesZeroLengthFiles(t *testing.T) {
	dir := t.TempDir()
	cache := t.TempDir()
	os.WriteFile(filepath.Join(dir, "app"), []byte("build output"), 0755)
	os.WriteFile(filepath.Join(dir, "empty"), nil, 0600)
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/sync" || r.Header.Get("Authorization") != "Bearer test" {
			t.Errorf("request=%s", r.URL)
		}
		var input io.Reader = r.Body
		if r.Header.Get("Content-Encoding") == "zstd" {
			decoder, err := zstd.NewReader(r.Body)
			if err != nil {
				t.Error(err)
				return
			}
			defer decoder.Close()
			input = decoder
		}
		var prefix [4]byte
		if _, err := io.ReadFull(input, prefix[:]); err != nil {
			t.Error(err)
			return
		}
		metaBytes := make([]byte, binary.BigEndian.Uint32(prefix[:]))
		io.ReadFull(input, metaBytes)
		var m manifest
		if err := json.Unmarshal(metaBytes, &m); err != nil {
			t.Error(err)
		}
		if len(m.Files) != 2 {
			t.Errorf("manifest=%s", metaBytes)
		}
		body, _ := io.ReadAll(input)
		if requests == 1 && len(body) != 12 {
			t.Errorf("payload=%q", body)
		}
		w.Header().Set("Content-Type", "application/json")
		if requests == 2 {
			w.Write([]byte(`{"ok":false,"needFull":["app","empty"]}`))
		} else {
			w.Write([]byte(`{"ok":true,"bundleId":"com.test","installedAppPath":"/app"}`))
		}
	}))
	defer srv.Close()
	opts := Options{APIURL: srv.URL, Token: "test", BasisCacheDir: cache}
	first, err := Sync(context.Background(), dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	if first.BytesSent != 12 {
		t.Errorf("sent=%d", first.BytesSent)
	}
	second, err := Sync(context.Background(), dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.BytesSent != 12 || requests != 3 {
		t.Errorf("retry bytes=%d requests=%d", second.BytesSent, requests)
	}
}
