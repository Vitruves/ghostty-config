// Package update asks GitHub whether a newer release of this tool exists.
//
// The check is best effort: it runs in the background, gives up quickly
// without a network, and every failure is silent. Nothing is downloaded.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// latestURL is the GitHub API endpoint for the newest published release.
const latestURL = "https://api.github.com/repos/vitruves/ghostty-config/releases/latest"

// InstallHint is the command that fetches the newest release.
const InstallHint = "go install github.com/vitruves/ghostty-config/cmd/ghostty-config@latest"

// Timeout bounds the whole request, DNS included, so an offline machine or a
// captive portal costs a few seconds of a background goroutine and nothing else.
const Timeout = 3 * time.Second

// Interval is how long a check stays fresh; GitHub is asked at most this often.
const Interval = 24 * time.Hour

// Latest returns the tag of the newest release, without its leading "v".
func Latest(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var release struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", err
	}
	if release.Tag == "" {
		return "", fmt.Errorf("release has no tag")
	}
	return strings.TrimPrefix(release.Tag, "v"), nil
}

// Newer reports whether version a is strictly newer than b. Versions are
// dotted numbers with an optional "v" and pre-release suffix, which is
// ignored; anything unparseable is never newer, so a dev build stays quiet.
func Newer(a, b string) bool {
	pa, okA := parse(a)
	pb, okB := parse(b)
	if !okA || !okB {
		return false
	}
	for i := range pa {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
