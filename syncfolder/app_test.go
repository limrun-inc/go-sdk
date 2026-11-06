package syncfolder

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveStablePathAndUnsafeEntries(t *testing.T) {
	archive := filepath.Join(t.TempDir(), "app.ipa")
	write := func(entries map[string]string) {
		f, e := os.Create(archive)
		if e != nil {
			t.Fatal(e)
		}
		w := zip.NewWriter(f)
		for name, content := range entries {
			entry, e := w.Create(name)
			if e != nil {
				t.Fatal(e)
			}
			entry.Write([]byte(content))
		}
		w.Close()
		f.Close()
	}
	write(map[string]string{"Payload/Test.app/Info.plist": "plist", "Payload/Test.app/old": "old"})
	first, e := PrepareAppBundlePath(archive)
	if e != nil {
		t.Fatal(e)
	}
	defer os.RemoveAll(filepath.Dir(first.AppPath))
	write(map[string]string{"Payload/Test.app/Info.plist": "new", "Payload/Test.app/new": "new"})
	second, e := PrepareAppBundlePath(archive)
	if e != nil {
		t.Fatal(e)
	}
	if first.AppPath != second.AppPath || second.CacheIdentityPath != archive {
		t.Fatalf("unstable path: %v %v", first, second)
	}
	if _, e = os.Stat(filepath.Join(first.AppPath, "old")); !os.IsNotExist(e) {
		t.Fatalf("old entry retained: %v", e)
	}
	write(map[string]string{"Payload/Test.app/Info.plist": "plist", "Payload/Test.app/../escape": "bad"})
	if _, e = PrepareAppBundlePath(archive); e == nil {
		t.Fatal("accepted traversal")
	}
}
func TestSeedReconstructionReusesShiftedBlocks(t *testing.T) {
	seed := []byte("abcdefgh12345678ABCDEFGH87654321ijklmnopIJKLMNOPzz")
	hash := sha256.Sum256(seed)
	s := SeedSignature{Version: 1, BlockSize: 8, FileSize: int64(len(seed)), SHA256: hex.EncodeToString(hash[:])}
	for start := 0; start < len(seed); start += 8 {
		end := start + 8
		if end > len(seed) {
			end = len(seed)
		}
		block := seed[start:end]
		s.Blocks = append(s.Blocks, SeedSignatureBlock{WeakRollingChecksum(block), strongHash(block)})
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "local.apk")
	local := append([]byte("!"), seed...)
	copy(local[17:25], []byte("changed!"))
	os.WriteFile(source, local, 0644)
	output := filepath.Join(dir, "seed.apk")
	var downloaded int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Error("missing bearer")
		}
		if r.URL.Path == "/seed.sig" {
			json.NewEncoder(w).Encode(s)
			return
		}
		var from, to int
		if _, e := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-%d", &from, &to); e != nil {
			t.Error(e)
			w.WriteHeader(400)
			return
		}
		downloaded += to - from + 1
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", from, to, len(seed)))
		w.WriteHeader(206)
		w.Write(seed[from : to+1])
	}))
	defer server.Close()
	result, e := ReconstructSeedFromLocalFiles(context.Background(), ReconstructSeedOptions{SignatureURL: server.URL + "/seed.sig", SeedURL: server.URL + "/seed", Token: "token", ExpectedSHA256: s.SHA256, SourcePaths: []string{source}, OutputPath: output})
	if e != nil {
		t.Fatal(e)
	}
	actual, e := os.ReadFile(output)
	if e != nil || !bytes.Equal(actual, seed) {
		t.Fatalf("reconstructed %q %v", actual, e)
	}
	if downloaded != 8 || result.MatchedBlocks != len(s.Blocks)-1 {
		t.Fatalf("result %+v bytes %d", result, downloaded)
	}
}
