#!/bin/sh
# Installs ghostty-config.
#
#   curl -fsSL https://ghostty-config.vitruves.workers.dev/install.sh | sh
#
# Downloads the binary of the newest release for this system (macOS, Linux or
# Android under Termux, Intel or ARM), checks it against the release's
# checksums and puts it in ~/.local/bin. Nothing else is needed. Where no
# binary fits, it builds the release from source instead, which needs git and
# Go 1.26 or newer.
#
# The script sends nothing anywhere: the only thing counted is that it was
# fetched.
#
#   GHOSTTY_CONFIG_BIN=dir          install there instead of ~/.local/bin
#   GHOSTTY_CONFIG_VERSION=1.2.3    that version instead of the newest
#   GHOSTTY_CONFIG_FROM_SOURCE=1    build from source even where a binary fits
set -eu

OWNER_REPO="vitruves/ghostty-config"
REPO="https://github.com/$OWNER_REPO.git"
RELEASES="${GHOSTTY_CONFIG_RELEASES:-https://github.com/$OWNER_REPO/releases/download}"
BIN="ghostty-config"
MIN_GO="1.26"
dest="${GHOSTTY_CONFIG_BIN:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
die() { printf '✗ %s\n' "$*" >&2; exit 1; }
has() { command -v "$1" >/dev/null 2>&1; }

# fetch URL FILE downloads with whichever of curl and wget is here.
fetch() {
	if has curl; then
		curl -fsSL "$1" -o "$2"
	elif has wget; then
		wget -q "$1" -O "$2"
	else
		die "curl or wget is needed"
	fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# --- which release ------------------------------------------------------------

tag="${GHOSTTY_CONFIG_VERSION:-}"
if [ -z "$tag" ]; then
	if fetch "https://api.github.com/repos/$OWNER_REPO/releases/latest" "$tmp/latest.json" 2>/dev/null; then
		tag="$(sed -n 's/.*"tag_name"[^"]*"\([^"]*\)".*/\1/p' "$tmp/latest.json" | head -n 1)"
	fi
fi
if [ -z "$tag" ] && has git; then
	# The API can be rate-limited; the tags say the same thing.
	tag="$(git ls-remote --tags --refs "$REPO" 2>/dev/null | sed 's|.*refs/tags/||' | sort -t. -k1,1n -k2,2n -k3,3n | tail -n 1)"
fi
[ -n "$tag" ] || die "could not find the newest release of ghostty-config"

# --- which binary ----------------------------------------------------------------

# Termux is Android behind a Linux shell. A binary built for Linux dies there:
# Android's seccomp policy refuses faccessat2, which Go's os/exec calls at
# startup. The Android build takes the branch Go keeps for that system, so
# Termux gets that one.
is_termux() {
	case "${PREFIX:-}" in
	*/com.termux/files/usr) return 0 ;;
	esac
	[ "$(uname -o 2>/dev/null)" = "Android" ]
}

os=""
case "$(uname -s)" in
Darwin) os="darwin" ;;
Linux)
	if is_termux; then os="android"; else os="linux"; fi
	;;
esac
arch=""
case "$(uname -m)" in
x86_64 | amd64) arch="amd64" ;;
arm64 | aarch64) arch="arm64" ;;
esac
# A shell running under Rosetta says x86_64 on an Apple chip: take the native
# binary all the same.
if [ "$os" = "darwin" ] && [ "$arch" = "amd64" ] && [ "$(sysctl -n hw.optional.arm64 2>/dev/null || echo 0)" = "1" ]; then
	arch="arm64"
fi

# sha256 FILE prints a file's SHA-256, with whichever tool is here.
sha256() {
	if has sha256sum; then
		sha256sum "$1" | cut -d' ' -f1
	elif has shasum; then
		shasum -a 256 "$1" | cut -d' ' -f1
	else
		openssl dgst -sha256 "$1" 2>/dev/null | sed 's/.*= //'
	fi
}

# download fetches the release archive for this system, checks it and unpacks
# the binary to $tmp/$BIN. It fails quietly when there is none.
download() {
	[ -n "$os" ] && [ -n "$arch" ] || return 1
	name="${BIN}_${tag}_${os}_${arch}"
	say "Downloading ghostty-config $tag for ${os}/${arch}…"
	fetch "$RELEASES/$tag/$name.tar.gz" "$tmp/$name.tar.gz" 2>/dev/null || return 1
	fetch "$RELEASES/$tag/checksums.txt" "$tmp/checksums.txt" 2>/dev/null || die "the release $tag has no checksums.txt"
	want="$(grep " \*\{0,1\}$name.tar.gz\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
	[ -n "$want" ] || die "checksums.txt does not list $name.tar.gz"
	got="$(sha256 "$tmp/$name.tar.gz")"
	[ "$got" = "$want" ] || die "the download does not match its checksum: try again"
	tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
	mv "$tmp/$name/$BIN" "$tmp/$BIN"
	# A binary can be built for this system and still be refused by it (an
	# older Linux build on Android, say): only take one that runs.
	"$tmp/$BIN" -version >/dev/null 2>&1 || return 1
}

# build compiles the release from source, as before there were binaries.
build() {
	if ! has git || ! has go; then
		if is_termux; then
			die "building ghostty-config on Termux needs git and Go: pkg install git golang"
		fi
		has git || die "no binary fits this system ($(uname -s) $(uname -m)), and building from source needs git"
		has go || die "no binary fits this system ($(uname -s) $(uname -m)), and building from source needs Go $MIN_GO or newer: https://go.dev/dl/"
	fi
	v="$(go version | sed -n 's/.*go\([0-9]*\)\.\([0-9]*\).*/\1 \2/p')"
	[ -n "$v" ] || die "could not read the Go version"
	set -- $v
	want_major="${MIN_GO%%.*}"
	want_minor="${MIN_GO#*.}"
	[ "$1" -gt "$want_major" ] || { [ "$1" -eq "$want_major" ] && [ "$2" -ge "$want_minor" ]; } ||
		die "Go $MIN_GO or newer is needed, this is go$1.$2: https://go.dev/dl/"
	say "Building ghostty-config $tag from source…"
	# Termux's own Go already targets Android; say it plainly all the same, so
	# a Go installed some other way still builds a binary Android runs.
	cgo=0
	if is_termux && [ -n "$arch" ]; then
		GOOS=android
		GOARCH="$arch"
		export GOOS GOARCH
		# Android on x86 links only through cgo; arm64 links either way.
		if [ "$arch" = "amd64" ]; then cgo=1; fi
	fi
	git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$tag" "$REPO" "$tmp/src"
	(cd "$tmp/src" && CGO_ENABLED=$cgo go build -trimpath -ldflags "-s -w -X main.version=$tag" -o "$tmp/$BIN" ./cmd/ghostty-config)
}

if [ -n "${GHOSTTY_CONFIG_FROM_SOURCE:-}" ] || ! download; then
	build
fi

# --- install ------------------------------------------------------------------------

mkdir -p "$dest"
install -m 755 "$tmp/$BIN" "$dest/$BIN"
say "Installed $("$dest/$BIN" -version 2>/dev/null || echo "$BIN") to $dest/$BIN"
case ":$PATH:" in
*":$dest:"*) say "Run it: $BIN" ;;
*) say "Add $dest to your PATH to run it as $BIN" ;;
esac
