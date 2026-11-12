package extensions

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// releases are the file lists of the real latest releases, as GitHub listed
// them on 2026-11-12.
func releases(t *testing.T) map[string][]Asset {
	t.Helper()
	data, err := os.ReadFile("testdata/releases.json")
	if err != nil {
		t.Fatal(err)
	}
	var list []struct {
		Repo   string
		Assets []string
	}
	if err := json.Unmarshal(data, &list); err != nil {
		t.Fatal(err)
	}
	out := map[string][]Asset{}
	for _, r := range list {
		for _, n := range r.Assets {
			out[strings.ToLower(r.Repo)] = append(out[strings.ToLower(r.Repo)], Asset{Name: n})
		}
	}
	return out
}

// Every program of the list, on every system it is offered for, resolves to
// exactly one file of its real release, and to the right one.
func TestEveryProgramPicksItsBuildFromItsRealRelease(t *testing.T) {
	all := releases(t)
	want := map[string]string{
		"starship linux/amd64":   "starship-x86_64-unknown-linux-musl.tar.gz",
		"atuin darwin/arm64":     "atuin-aarch64-apple-darwin.tar.gz",
		"atuin linux/arm64":      "atuin-aarch64-unknown-linux-musl.tar.gz",
		"fastfetch darwin/arm64": "fastfetch-macos-universal.tar.gz",
		"fastfetch linux/amd64":  "fastfetch-linux-amd64.tar.gz",
		"kitten darwin/arm64":    "kitten-darwin-arm64",
		"kitten linux/amd64":     "kitten-linux-amd64",
		"fzf darwin/amd64":       "fzf-0.75.0-darwin_amd64.tar.gz",
		"yazi linux/amd64":       "yazi-x86_64-unknown-linux-musl.zip",
		"eza linux/arm64":        "eza_aarch64-unknown-linux-gnu.tar.gz",
		"superfile darwin/arm64": "superfile-darwin-v1.6.0-arm64.tar.gz",
		"zmx darwin/amd64":       "zmx-1.0.1-macos-x86_64.tar.gz",
		"lig linux/amd64":        "lig-1.2.2-x86_64-unknown-linux-musl.tar.gz",
		"carapace linux/arm64":   "carapace-bin_1.8.2_linux_arm64.tar.gz",
	}
	for _, p := range Programs {
		assets := all[strings.ToLower(p.Repo)]
		if assets == nil {
			t.Fatalf("no release recorded for %s", p.Repo)
		}
		for _, sys := range []string{"darwin/arm64", "darwin/amd64", "linux/arm64", "linux/amd64"} {
			goos, goarch, _ := strings.Cut(sys, "/")
			if !releaseBuilt(p, goos, goarch) {
				continue
			}
			a, err := PickAsset(assets, goos, goarch, p.Skip)
			if err != nil {
				t.Errorf("%s on %s: %v", p.Name, sys, err)
				continue
			}
			if w, ok := want[p.Name+" "+sys]; ok && a.Name != w {
				t.Errorf("%s on %s picked %s, want %s", p.Name, sys, a.Name, w)
			}
		}
	}
}

func tarGz(t *testing.T, files map[string]string) []byte {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for name, body := range files {
		_ = tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = zw.Close()
	return buf.Bytes()
}

// serveRelease answers GitHub's API and download URLs for one release
// holding archive, under the digest given ("" for none, "good" for the
// right one).
func serveRelease(t *testing.T, archive []byte, digest string) {
	t.Helper()
	name := fmt.Sprintf("demo-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	if digest == "good" {
		sum := sha256.Sum256(archive)
		digest = "sha256:" + hex.EncodeToString(sum[:])
	}
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/someone/demo/releases/latest":
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.0.0", "assets": []Asset{
				{Name: name + ".sha256", URL: srv.URL + "/dl/" + name + ".sha256"},
				{Name: name, URL: srv.URL + "/dl/" + name, Digest: digest},
			}})
		case "/dl/" + name:
			_, _ = w.Write(archive)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	old := apiURL
	apiURL = srv.URL
	t.Cleanup(func() { apiURL = old })
}

var demo = Program{Name: "demo", Repo: "someone/demo", Bins: []string{"demo", "demo-helper"}}

// A release is installed only when the file matches the SHA-256 GitHub
// gives for it, and then every executable lands in the directory, runnable.
func TestAReleaseIsInstalledOnlyWhenItsChecksumMatches(t *testing.T) {
	archive := tarGz(t, map[string]string{"demo-1.0/demo": "#!/bin/sh\necho demo\n", "demo-1.0/demo-helper": "helper", "demo-1.0/README": "readme"})
	for _, c := range []struct{ digest, fails string }{
		{"", "no SHA-256"},
		{"sha256:" + strings.Repeat("0", 64), "does not match"},
		{"good", ""},
	} {
		dir := t.TempDir()
		serveRelease(t, archive, c.digest)
		err := installRelease(context.Background(), demo, dir)
		if c.fails != "" {
			if err == nil || !strings.Contains(err.Error(), c.fails) {
				t.Fatalf("digest %q should fail with %q, got %v", c.digest, c.fails, err)
			}
			if entries, _ := os.ReadDir(dir); len(entries) != 0 {
				t.Fatalf("nothing should be written when the check fails: %v", entries)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range demo.Bins {
			info, err := os.Stat(filepath.Join(dir, b))
			if err != nil || info.Mode().Perm()&0o100 == 0 {
				t.Fatalf("%s should be installed and executable: %v %v", b, info, err)
			}
		}
		if _, err := os.Stat(filepath.Join(dir, "README")); err == nil {
			t.Fatal("only the executables are taken out of the archive")
		}
	}
}

// Zips and bare executables are read too, and an archive without the
// executable is refused.
func TestExecutablesComeOutOfZipsAndBareFiles(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("yazi-x/yazi")
	_, _ = w.Write([]byte("yazi"))
	w, _ = zw.Create("yazi-x/ya")
	_, _ = w.Write([]byte("ya"))
	_ = zw.Close()
	got, err := executablesIn("yazi.zip", buf.Bytes(), []string{"yazi", "ya"})
	if err != nil || string(got["yazi"]) != "yazi" || string(got["ya"]) != "ya" {
		t.Fatalf("zip: %v %v", got, err)
	}
	if got, err := executablesIn("kitten-darwin-arm64", []byte("bin"), []string{"kitten"}); err != nil || string(got["kitten"]) != "bin" {
		t.Fatalf("bare: %v %v", got, err)
	}
	if _, err := executablesIn("x.tar.gz", tarGz(t, map[string]string{"other": "x"}), []string{"demo"}); err == nil {
		t.Fatal("an archive without the executable should be refused")
	}
}

// Homebrew is used when there is Homebrew and the program is in core; else
// the release, when one is built for the system.
func TestThePlanPrefersHomebrew(t *testing.T) {
	old := Brew
	t.Cleanup(func() { Brew = old })
	starship, _ := FindProgram("starship")
	lig, _ := FindProgram("lig")

	Brew = func() string { return "/opt/homebrew/bin/brew" }
	if r := starship.Plan(); r.Brew == "" {
		t.Fatalf("starship should come from Homebrew: %+v", r)
	}
	if r := lig.Plan(); r.Brew != "" || !r.Release {
		t.Fatalf("lig is not in homebrew/core, so it comes from its release: %+v", r)
	}
	Brew = func() string { return "" }
	if r := starship.Plan(); r.Brew != "" || !r.Release {
		t.Fatalf("without Homebrew starship comes from its release: %+v", r)
	}
	eza, _ := FindProgram("eza")
	if releaseBuilt(eza, "darwin", "arm64") || !releaseBuilt(eza, "linux", "arm64") {
		t.Fatal("eza builds only for Linux")
	}
}

// The block is the one part of the file written: added, it leaves the rest
// as it was; emptied, it leaves the file as it was before.
func TestTheBlockLeavesTheRestOfTheFileAlone(t *testing.T) {
	orig := "export EDITOR=vim\n# eval \"$(zoxide init zsh)\"\nalias g=git\n"
	one := WithBlock(orig, "zsh", []string{"starship"}, false)
	if !strings.HasPrefix(one, orig+"\n"+blockStart+"\n") || !strings.Contains(one, `eval "$(starship init zsh)"  # ghostty-config:starship`) {
		t.Fatalf("the block should be added at the end:\n%s", one)
	}
	two := WithBlock(one+"alias later=1\n", "zsh", []string{"zoxide", "starship"}, true)
	if strings.Count(two, blockStart) != 1 || !strings.HasSuffix(two, blockEnd+"\nalias later=1\n") {
		t.Fatalf("the block should be rewritten where it is:\n%s", two)
	}
	if got := Enabled(two); strings.Join(got, " ") != "starship zoxide" {
		t.Fatalf("the block lists its programs in the list's order: %v", got)
	}
	if !strings.Contains(two, "ghostty-config:PATH") {
		t.Fatal("local asks for ~/.local/bin on the PATH")
	}
	if back := WithBlock(one, "zsh", nil, false); back != orig {
		t.Fatalf("emptying the block should give the file back:\n%q\n%q", back, orig)
	}
	zoxide, _ := FindProgram("zoxide")
	if OwnLine(orig, zoxide) {
		t.Fatal("a commented-out line starts nothing")
	}
	if OwnLine(two, zoxide) || !OwnLine("eval \"$(zoxide init zsh)\"\n", zoxide) {
		t.Fatal("the block's own lines are not the user's")
	}
}

// lig's closing line must stay the file's last, so the block goes above it
// and the comment heading it.
func TestTheBlockGoesAboveLigsLastLine(t *testing.T) {
	rc := "eval \"$(lig init zsh pre)\"\nalias g=git\n# lig\neval \"$(lig init zsh post)\"\n"
	got := WithBlock(rc, "zsh", []string{"fzf"}, false)
	if !strings.HasSuffix(got, blockEnd+"\n\n# lig\neval \"$(lig init zsh post)\"\n") {
		t.Fatalf("the block should sit above lig's last lines:\n%s", got)
	}
	fish := "eval ('/home/u/.local/bin/lig' init fish post | string collect)\n"
	if got := WithBlock(fish, "fish", []string{"starship"}, false); !strings.HasPrefix(got, blockStart) {
		t.Fatalf("lig's quoted path is found too:\n%s", got)
	}
	lig, _ := FindProgram("lig")
	if !OwnLine("eval ('/home/u/.local/bin/lig' init fish pre | string collect)\n", lig) {
		t.Fatal("lig set up by its own install, under a quoted path, is on")
	}
}
