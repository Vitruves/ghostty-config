package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Installing a release from inside the tool: the archive the release
// workflow publishes for this system is downloaded, checked against the
// release's checksums, and its binary replaces the one running, in place and
// in one rename, so an interrupted update leaves the old binary whole.

// releasesURL is where the release archives are downloaded from; a variable
// so tests can serve their own.
var releasesURL = "https://github.com/vitruves/ghostty-config/releases/download"

// executable is the binary to replace; a variable so tests never touch theirs.
var executable = os.Executable

// InstallTimeout bounds the whole download.
const InstallTimeout = 2 * time.Minute

// maxArchive is more than any release archive will ever be.
const maxArchive = 64 << 20

// ErrUnsupported says no release archive is built for this system.
var ErrUnsupported = errors.New("no release is built for this system")

// Archive is the name of the release archive of a version for this system.
func Archive(version string) (string, error) {
	return archiveFor(version, runtime.GOOS, runtime.GOARCH)
}

// archiveFor names the release archive of a version for a system, so every
// published target can be checked from any machine.
func archiveFor(version, goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "darwin/amd64", "darwin/arm64", "linux/amd64", "linux/arm64",
		"android/amd64", "android/arm64":
		return fmt.Sprintf("ghostty-config_%s_%s_%s.tar.gz", version, goos, goarch), nil
	}
	return "", fmt.Errorf("%w: %s/%s", ErrUnsupported, goos, goarch)
}

// Install downloads a version and puts it in place of the running binary.
// It returns the path it replaced.
func Install(ctx context.Context, version string) (string, error) {
	name, err := Archive(version)
	if err != nil {
		return "", err
	}
	exe, err := executable()
	if err != nil {
		return "", fmt.Errorf("cannot tell where this binary is: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)

	ctx, cancel := context.WithTimeout(ctx, InstallTimeout)
	defer cancel()
	sums, err := fetch(ctx, releasesURL+"/"+version+"/checksums.txt", 1<<20)
	if err != nil {
		return "", fmt.Errorf("checksums: %w", err)
	}
	want := checksumOf(sums, name)
	if want == "" {
		return "", fmt.Errorf("the release lists no checksum for %s", name)
	}
	archive, err := fetch(ctx, releasesURL+"/"+version+"/"+name, maxArchive)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	got := sha256.Sum256(archive)
	if hex.EncodeToString(got[:]) != want {
		return "", fmt.Errorf("the download does not match its checksum")
	}
	bin, err := binaryIn(archive)
	if err != nil {
		return "", err
	}

	// Written beside the old binary, then renamed over it: the same
	// directory keeps the rename on one file system, which makes it atomic.
	tmp, err := os.CreateTemp(dir, ".ghostty-config-update-*")
	if err != nil {
		return "", fmt.Errorf("cannot write to %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(bin); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Chmod(tmp.Name(), 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), exe); err != nil {
		return "", fmt.Errorf("cannot replace %s: %w", exe, err)
	}
	return exe, nil
}

// fetch downloads a URL whole, up to limit bytes.
func fetch(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s answered %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s is larger than expected", url)
	}
	return data, nil
}

// checksumOf finds a file's SHA-256 in a sha256sum-style list.
func checksumOf(sums []byte, name string) string {
	sc := bufio.NewScanner(strings.NewReader(string(sums)))
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) == 2 && strings.TrimPrefix(fields[1], "*") == name {
			return strings.ToLower(fields[0])
		}
	}
	return ""
}

// binaryIn takes the ghostty-config binary out of a release archive.
func binaryIn(archive []byte) ([]byte, error) {
	zr, err := gzip.NewReader(strings.NewReader(string(archive)))
	if err != nil {
		return nil, fmt.Errorf("the archive is not gzip: %w", err)
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("the archive holds no ghostty-config binary")
		}
		if err != nil {
			return nil, fmt.Errorf("the archive is damaged: %w", err)
		}
		if h.Typeflag == tar.TypeReg && filepath.Base(h.Name) == "ghostty-config" {
			return io.ReadAll(io.LimitReader(tr, maxArchive))
		}
	}
}
