package syncfolder

import (
	"bytes"
	"crypto/rand"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDeltaDecodesWithDaemonCodec(t *testing.T) {
	tool, err := exec.LookPath("xdelta3")
	if err != nil {
		t.Skip("xdelta3 decoder not installed")
	}
	dir := t.TempDir()
	basis := make([]byte, 2<<20)
	if _, err = rand.Read(basis); err != nil {
		t.Fatal(err)
	}
	target := append([]byte{}, basis[:100000]...)
	target = append(target, []byte("inserted build data")...)
	target = append(target, basis[100000:]...)
	basisPath := filepath.Join(dir, "basis")
	targetPath := filepath.Join(dir, "target")
	patch := filepath.Join(dir, "delta")
	decoded := filepath.Join(dir, "decoded")
	os.WriteFile(basisPath, basis, 0600)
	os.WriteFile(targetPath, target, 0600)
	size, err := EncodePatch(basisPath, targetPath, patch, int64(len(target)/2))
	if err != nil || size < 0 {
		t.Fatalf("patch size=%d err=%v", size, err)
	}
	if out, err := exec.Command(tool, "-d", "-s", basisPath, patch, decoded).CombinedOutput(); err != nil {
		t.Fatalf("xdelta3: %v: %s", err, out)
	}
	actual, err := os.ReadFile(decoded)
	if err != nil || !bytes.Equal(actual, target) {
		t.Fatalf("decoded mismatch: %v", err)
	}
}
