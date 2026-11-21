package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// release serves one version the way GitHub does: the archive for this
// system and the checksums, the binary inside holding body.
func release(t *testing.T, version, body string, tamper bool) {
	t.Helper()
	name, err := Archive(version)
	if err != nil {
		t.Skip(err)
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	dir := strings.TrimSuffix(name, ".tar.gz")
	for _, f := range []struct{ name, body string }{{dir + "/README.md", "readme"}, {dir + "/ghostty-config", body}} {
		_ = tw.WriteHeader(&tar.Header{Name: f.name, Mode: 0o755, Size: int64(len(f.body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(f.body))
	}
	_ = tw.Close()
	_ = zw.Close()
	archive := buf.Bytes()
	sum := sha256.Sum256(archive)
	hexSum := hex.EncodeToString(sum[:])
	if tamper {
		hexSum = strings.Repeat("0", 64)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/" + version + "/checksums.txt":
			_, _ = w.Write([]byte(hexSum + "  " + name + "\n"))
		case "/" + version + "/" + name:
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := releasesURL
	releasesURL = srv.URL
	t.Cleanup(func() { releasesURL = old })
}

// fakeExecutable stands in for the running binary.
func fakeExecutable(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ghostty-config")
	if err := os.WriteFile(path, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := executable
	executable = func() (string, error) { return path, nil }
	t.Cleanup(func() { executable = old })
	return path
}

func TestArchiveNamesEveryPublishedSystem(t *testing.T) {
	for _, target := range []string{
		"darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64",
		"android/amd64", "android/arm64",
	} {
		goos, goarch, _ := strings.Cut(target, "/")
		got, err := archiveFor("1.2.3", goos, goarch)
		if err != nil {
			t.Errorf("%s: %v", target, err)
			continue
		}
		want := "ghostty-config_1.2.3_" + goos + "_" + goarch + ".tar.gz"
		if got != want {
			t.Errorf("%s: got %s, want %s", target, got, want)
		}
	}
	if _, err := archiveFor("1.2.3", "windows", "amd64"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("windows/amd64: got %v, want ErrUnsupported", err)
	}
}

func TestInstallReplacesTheBinary(t *testing.T) {
	release(t, "9.9.9", "new binary", false)
	path := fakeExecutable(t)
	got, err := Install(context.Background(), "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(path); got != want {
		t.Fatalf("replaced %s, want %s", got, path)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "new binary" {
		t.Fatalf("the binary holds %q", data)
	}
	if info, _ := os.Stat(path); info.Mode().Perm()&0o100 == 0 {
		t.Fatalf("the new binary is not executable: %v", info.Mode())
	}
	if left, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".ghostty-config-update-*")); len(left) > 0 {
		t.Fatalf("temporary files left behind: %v", left)
	}
}

func TestInstallRefusesABadChecksum(t *testing.T) {
	release(t, "9.9.9", "new binary", true)
	path := fakeExecutable(t)
	if _, err := Install(context.Background(), "9.9.9"); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("got %v, want a checksum error", err)
	}
	if data, _ := os.ReadFile(path); string(data) != "old" {
		t.Fatalf("the old binary was touched: %q", data)
	}
}

func TestInstallFailsCleanlyWithoutARelease(t *testing.T) {
	release(t, "9.9.9", "new binary", false)
	path := fakeExecutable(t)
	if _, err := Install(context.Background(), "9.9.10"); err == nil {
		t.Fatal("a missing release installed")
	}
	if data, _ := os.ReadFile(path); string(data) != "old" {
		t.Fatalf("the old binary was touched: %q", data)
	}
}
