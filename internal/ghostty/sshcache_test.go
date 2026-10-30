package ghostty

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeSSHCache stands a script in for +ssh-cache, keeping its cache in a
// file beside it, in the format Ghostty 1.4 prints, and logging each call.
func fakeSSHCache(t *testing.T, entries string) (bin, cache string) {
	bin = scriptGhostty(t, `
dir="$(dirname "$0")"
echo "$@" >> "$dir/log"
[ "$1" = +ssh-cache ] || exit 2
case "$2" in
"") cat "$dir/cache" ;;
--remove=*)
	d="${2#--remove=}"
	grep -q "^$d " "$dir/cache" || { echo "Error: '$d' is not in the cache." >&2; exit 1; }
	grep -v "^$d " "$dir/cache" > "$dir/cache.new"; mv "$dir/cache.new" "$dir/cache" ;;
--prune=*) echo "Pruned cache entries: 2" ;;
--clear) : > "$dir/cache" ;;
*) echo "Error: unexpected $2" >&2; exit 1 ;;
esac`)
	cache = filepath.Join(filepath.Dir(bin), "cache")
	if err := os.WriteFile(cache, []byte(entries), 0o644); err != nil {
		t.Fatal(err)
	}
	return bin, cache
}

const sampleSSHCache = `me@[2001:db8::1]:2222  2026-10-30T22:57:25Z (now)
other.org:2222         2026-09-01T08:00:00Z (59 days ago)
user@example.com       2026-10-29T10:00:00Z (1 day ago)
`

func TestSSHCacheList(t *testing.T) {
	bin, _ := fakeSSHCache(t, sampleSSHCache)
	got, err := SSHCache(bin)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries: %+v", len(got), got)
	}
	if got[0].Dest != "me@[2001:db8::1]:2222" || got[0].Age != "now" {
		t.Errorf("IPv6 with a port: %+v", got[0])
	}
	if got[1].Dest != "other.org:2222" || got[1].Age != "59 days ago" || !got[1].Added.Equal(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)) {
		t.Errorf("host and port: %+v", got[1])
	}
	empty, _ := fakeSSHCache(t, "")
	if got, err := SSHCache(empty); err != nil || len(got) != 0 {
		t.Errorf("an empty cache: %v, %v", got, err)
	}
}

func TestSSHCacheForget(t *testing.T) {
	bin, cache := fakeSSHCache(t, sampleSSHCache)
	if err := SSHCacheRemove(bin, "other.org:2222"); err != nil {
		t.Fatal(err)
	}
	left, _ := os.ReadFile(cache)
	if strings.Contains(string(left), "other.org") || !strings.Contains(string(left), "user@example.com") {
		t.Errorf("after remove:\n%s", left)
	}
	// Ghostty's own message comes back, without its Error: prefix.
	if err := SSHCacheRemove(bin, "nope"); err == nil || err.Error() != "'nope' is not in the cache." {
		t.Errorf("removing a destination not cached: %v", err)
	}
	// A destination is never passed on as a flag of its own.
	if err := SSHCacheRemove(bin, "--clear"); err == nil {
		t.Error("a flag was taken for a destination")
	}
	if n, err := SSHCachePrune(bin, "30d"); err != nil || n != 2 {
		t.Errorf("prune: %d, %v", n, err)
	}
	if err := SSHCacheClear(bin); err != nil {
		t.Fatal(err)
	}
	if got, _ := SSHCache(bin); len(got) != 0 {
		t.Errorf("after clear: %+v", got)
	}
	log, _ := os.ReadFile(filepath.Join(filepath.Dir(bin), "log"))
	want := "+ssh-cache --remove=other.org:2222\n+ssh-cache --remove=nope\n+ssh-cache --prune=30d\n+ssh-cache --clear\n+ssh-cache\n"
	if string(log) != want {
		t.Errorf("calls:\n%s\nwant:\n%s", log, want)
	}
}

func TestSSHCacheNoBinary(t *testing.T) {
	if _, err := SSHCache(""); !errors.Is(err, ErrNoBinary) {
		t.Errorf("no binary: %v", err)
	}
}
