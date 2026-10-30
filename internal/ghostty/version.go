package ghostty

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"sync"
	"time"
)

// Version is the release of the ghostty binary installed, as
// `ghostty --version` names it on its first line. Settings that only a newer
// release reads ask it first: the validator of an older one rejects a key or
// value it does not know, and the tool runs it after every write.
type Version struct {
	Major, Minor, Patch int
	// Known is false when there was no binary to ask or its answer could not
	// be read. Such a version counts as the newest the tool knows of, 1.4,
	// the likeliest case for a binary built from source.
	Known bool
}

// AtLeast is whether v is major.minor or later. An unknown version is.
func (v Version) AtLeast(major, minor int) bool {
	if !v.Known {
		return true
	}
	if v.Major != major {
		return v.Major > major
	}
	return v.Minor >= minor
}

// String is the version as Ghostty writes it, or "unknown".
func (v Version) String() string {
	if !v.Known {
		return "unknown"
	}
	return strconv.Itoa(v.Major) + "." + strconv.Itoa(v.Minor) + "." + strconv.Itoa(v.Patch)
}

var versionLine = regexp.MustCompile(`(?m)^\s*Ghostty\s+v?(\d+)\.(\d+)(?:\.(\d+))?`)

// ParseVersion reads the output of `ghostty --version`: "Ghostty 1.4.0",
// with a suffix such as "-dev+abc" on a build from source.
func ParseVersion(out string) Version {
	m := versionLine.FindStringSubmatch(out)
	if m == nil {
		return Version{}
	}
	major, _ := strconv.Atoi(m[1])
	minor, _ := strconv.Atoi(m[2])
	patch, _ := strconv.Atoi(m[3])
	return Version{Major: major, Minor: minor, Patch: patch, Known: true}
}

// VersionTimeout bounds the one run of `ghostty --version`.
var VersionTimeout = 5 * time.Second

var versions sync.Map // binary path → *versionAsk

// versionAsk is the one run of `ghostty --version` for a path, which every
// caller asking at the same time waits on.
type versionAsk struct {
	once sync.Once
	v    Version
}

// BinaryVersion asks a ghostty binary its version, once per path: the menu
// is rebuilt often and the answer does not change while the tool runs.
func BinaryVersion(binary string) Version {
	if binary == "" {
		return Version{}
	}
	a, _ := versions.LoadOrStore(binary, &versionAsk{})
	ask := a.(*versionAsk)
	ask.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), VersionTimeout)
		defer cancel()
		out, _ := exec.CommandContext(ctx, binary, "--version").Output()
		ask.v = ParseVersion(string(out))
	})
	return ask.v
}

// Prefetch starts asking, in the background, what the editor needs to know
// before its first frame and only programs can tell: the version of the
// binary and whether the desktop is dark. Each takes a program run of
// several milliseconds; started first, they run while the themes are read.
func Prefetch(binary string) {
	go BinaryVersion(binary)
	prefetchDarkDesktop()
}
