package ghostty

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Ghostty 1.4's ssh-terminfo shell integration feature, and `ghostty +ssh`
// behind it, install Ghostty's terminfo on a remote host once and remember
// the destination in a cache, so later connections skip the upload.
// `ghostty +ssh-cache` reads and trims that cache; these wrap only what it
// documents: the list, --remove, --prune and --clear. A forgotten host gets
// the terminfo again on its next connection.

// sshCacheTimeout bounds a call: the cache is a small file read locally.
var sshCacheTimeout = 5 * time.Second

// SSHCacheEntry is one destination in the cache: a hostname or
// user@hostname, with :port when it is not 22, and when it was installed.
type SSHCacheEntry struct {
	Dest  string
	Added time.Time
	// Age is how Ghostty words the time since, "now" or "3 days ago".
	Age string
}

// SSHCache lists every cached destination, in Ghostty's order. An empty
// cache, or one not made yet, is no error.
func SSHCache(binary string) ([]SSHCacheEntry, error) {
	out, err := runSSHCache(binary)
	if err != nil {
		return nil, err
	}
	return ParseSSHCache(out), nil
}

// ParseSSHCache reads the list +ssh-cache prints: one destination a line,
// its time in RFC 3339 and the age in parentheses after it.
func ParseSSHCache(out string) []SSHCacheEntry {
	var entries []SSHCacheEntry
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		e := SSHCacheEntry{Dest: f[0]}
		if len(f) > 1 {
			e.Added, _ = time.Parse(time.RFC3339, f[1])
		}
		if len(f) > 2 {
			e.Age = strings.TrimSuffix(strings.TrimPrefix(strings.Join(f[2:], " "), "("), ")")
		}
		entries = append(entries, e)
	}
	return entries
}

// SSHCacheRemove forgets one destination, exactly as the list writes it.
func SSHCacheRemove(binary, dest string) error {
	if dest == "" || strings.HasPrefix(dest, "-") {
		return fmt.Errorf("%q is not a destination", dest)
	}
	_, err := runSSHCache(binary, "--remove="+dest)
	return err
}

// SSHCachePrune forgets every destination installed longer ago than age, a
// duration with Ghostty's suffixes, 30d or 6h, and says how many went.
func SSHCachePrune(binary, age string) (int, error) {
	out, err := runSSHCache(binary, "--prune="+age)
	if err != nil {
		return 0, err
	}
	// Ghostty says "Pruned cache entries: N".
	_, n, _ := strings.Cut(strings.TrimSpace(out), ":")
	count, _ := strconv.Atoi(strings.TrimSpace(n))
	return count, nil
}

// SSHCacheClear forgets every destination.
func SSHCacheClear(binary string) error {
	_, err := runSSHCache(binary, "--clear")
	return err
}

// runSSHCache runs +ssh-cache and returns what it printed. A failure is
// Ghostty's own message, which names the destination or the cache file.
func runSSHCache(binary string, args ...string) (string, error) {
	if binary == "" {
		return "", ErrNoBinary
	}
	ctx, cancel := context.WithTimeout(context.Background(), sshCacheTimeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, binary, append([]string{"+ssh-cache"}, args...)...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return "", fmt.Errorf("Ghostty took too long to answer")
	}
	if err != nil {
		msg := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(stderr.String()), "Error:"))
		if msg == "" {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				return "", fmt.Errorf("ghostty +ssh-cache exited with %d", exit.ExitCode())
			}
			return "", err
		}
		return "", errors.New(msg)
	}
	return stdout.String(), nil
}
