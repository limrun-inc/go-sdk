package syncfolder

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// deltaFixture builds a basis above one 4 MiB source block and a target spanning
// several 1 MiB input chunks, with scattered edits, an insertion and a deletion.
// The generator is deterministic so the expected patch digest below, produced by
// @limrun/xdelta3-wasm 0.2.0 through the TypeScript encodeXdelta3Patch, stays valid.
func deltaFixture(t *testing.T) (basisPath, targetPath string, target []byte) {
	t.Helper()
	rng := rand.New(rand.NewSource(1))
	basis := make([]byte, 6<<20)
	rng.Read(basis)
	target = append([]byte{}, basis...)
	for i := 0; i < 20; i++ {
		at := rng.Intn(len(target) - 7)
		copy(target[at:], []byte("edited!"))
	}
	insert := make([]byte, 5000)
	rng.Read(insert)
	target = append(target[:2500000], append(insert, target[2500000:]...)...)
	target = append(target[:4000000], target[4003000:]...)
	dir := t.TempDir()
	if fixtureDir := os.Getenv("DELTA_FIXTURE_DIR"); fixtureDir != "" {
		dir = fixtureDir
	}
	basisPath = filepath.Join(dir, "basis")
	targetPath = filepath.Join(dir, "target")
	if err := os.WriteFile(basisPath, basis, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, target, 0600); err != nil {
		t.Fatal(err)
	}
	return basisPath, targetPath, target
}

// The TypeScript SDK encodes the same fixture to a patch with this digest; both
// SDKs embed the same xdelta3 module and chunk the target identically.
const typescriptPatchSHA256 = "60291f1e00479b63128866442b4bbdddeb776bbc01742edaaf7f2bf60316e066"

func TestEncodePatchMatchesTypeScriptEncoder(t *testing.T) {
	basisPath, targetPath, _ := deltaFixture(t)
	patch := filepath.Join(t.TempDir(), "patch")
	size, err := EncodePatch(context.Background(), basisPath, targetPath, patch, -1)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(patch)
	if err != nil || int64(len(data)) != size {
		t.Fatalf("patch size %d, file %d bytes: %v", size, len(data), err)
	}
	sum := sha256.Sum256(data)
	if got := hex.EncodeToString(sum[:]); got != typescriptPatchSHA256 {
		t.Fatalf("patch sha256 %s differs from the TypeScript encoder's %s (size %d)", got, typescriptPatchSHA256, size)
	}
}

func TestEncodePatchDecodesWithDaemonCodec(t *testing.T) {
	tool, err := exec.LookPath("xdelta3")
	if err != nil {
		t.Skip("xdelta3 decoder not installed")
	}
	basisPath, targetPath, target := deltaFixture(t)
	dir := t.TempDir()
	patch := filepath.Join(dir, "patch")
	decoded := filepath.Join(dir, "decoded")
	if _, err := EncodePatch(context.Background(), basisPath, targetPath, patch, -1); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command(tool, "-d", "-f", "-D", "-s", basisPath, patch, decoded).CombinedOutput(); err != nil {
		t.Fatalf("xdelta3: %v: %s", err, out)
	}
	actual, err := os.ReadFile(decoded)
	if err != nil || !bytes.Equal(actual, target) {
		t.Fatalf("decoded output differs from the target: %v", err)
	}
}

func TestEncodePatchStopsAtSizeLimit(t *testing.T) {
	dir := t.TempDir()
	rng := rand.New(rand.NewSource(2))
	unrelated := func(name string) string {
		data := make([]byte, 300<<10)
		rng.Read(data)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	basisPath, targetPath := unrelated("basis"), unrelated("target")
	patch := filepath.Join(dir, "patch")
	size, err := EncodePatch(context.Background(), basisPath, targetPath, patch, 100<<10)
	if err != nil || size != -1 {
		t.Fatalf("size=%d err=%v, want -1", size, err)
	}
	if _, err := os.Stat(patch); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized patch left behind: %v", err)
	}
}

func TestEncodePatchHandlesEmptyTarget(t *testing.T) {
	dir := t.TempDir()
	basisPath := filepath.Join(dir, "basis")
	targetPath := filepath.Join(dir, "target")
	if err := os.WriteFile(basisPath, []byte("some basis content"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(targetPath, nil, 0600); err != nil {
		t.Fatal(err)
	}
	patch := filepath.Join(dir, "patch")
	size, err := EncodePatch(context.Background(), basisPath, targetPath, patch, -1)
	if err != nil || size <= 0 {
		t.Fatalf("size=%d err=%v", size, err)
	}
}

func TestEncodePatchHonoursCancellation(t *testing.T) {
	basisPath, targetPath, _ := deltaFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	patch := filepath.Join(t.TempDir(), "patch")
	if _, err := EncodePatch(ctx, basisPath, targetPath, patch, -1); !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, want context.Canceled", err)
	}
	if _, err := os.Stat(patch); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial patch left behind: %v", err)
	}
}
