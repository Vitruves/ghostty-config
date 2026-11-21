#!/bin/sh
# Installs ghostty-config.
#
#   curl -fsSL https://ghostty-config.vitruves.workers.dev/install.sh | sh
#
# Downloads the binary of the newest release for this system (macOS, Linux or
# Android under Termux, Intel or ARM), checks it against the release's
# checksums and puts it in ~/.local/bin. Nothing else is needed. Where no
# binary fits, it builds the release from source instead, which needs git and
# Go 1.26 or newer (and clang on Termux).
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

# fetch URL FILE downloads with whichever of curl and wget is here. It returns
# 0 when the file is here, 1 when the server says there is no such file, and 2
# for anything else, so the caller can tell "this system has no binary" from
# "the network failed, try again".
fetch() {
	if has curl; then
		code="$(curl -fsSL -w '%{http_code}' "$1" -o "$2" 2>/dev/null)" && return 0
		if [ "$code" = "404" ]; then return 1; fi
		return 2
	elif has wget; then
		# --server-response puts the status on stderr, which is the only way
		# to tell a 404 from a 500: wget exits 8 for every HTTP error.
		wget -q --server-response "$1" -O "$2" 2>"$tmp/fetch.err" && return 0
		if grep -q '^[[:space:]]*HTTP/[0-9.]* 404' "$tmp/fetch.err"; then return 1; fi
		return 2
	else
		die "curl or wget is needed"
	fi
}

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"; [ -z "${new:-}" ] || rm -f "$new"' EXIT

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
# the binary to $tmp/$BIN. It fails quietly when the release has none for this
# system; a network failure stops the script instead, since building would not
# help there.
download() {
	[ -n "$os" ] && [ -n "$arch" ] || return 1
	name="${BIN}_${tag}_${os}_${arch}"
	say "Downloading ghostty-config $tag for ${os}/${arch}…"
	status=0
	fetch "$RELEASES/$tag/$name.tar.gz" "$tmp/$name.tar.gz" 2>/dev/null || status=$?
	case "$status" in
	0) ;;
	1) return 1 ;;
	*) die "could not download $name.tar.gz: check the network and run this again" ;;
	esac
	status=0
	fetch "$RELEASES/$tag/checksums.txt" "$tmp/checksums.txt" 2>/dev/null || status=$?
	case "$status" in
	0) ;;
	1) die "the release $tag has no checksums.txt" ;;
	*) die "could not download the checksums of $tag: check the network and run this again" ;;
	esac
	want="$(grep " \*\{0,1\}$name.tar.gz\$" "$tmp/checksums.txt" | cut -d' ' -f1)"
	[ -n "$want" ] || die "checksums.txt does not list $name.tar.gz"
	got="$(sha256 "$tmp/$name.tar.gz")"
	[ "$got" = "$want" ] || die "the download does not match its checksum: try again"
	tar -xzf "$tmp/$name.tar.gz" -C "$tmp"
	mv "$tmp/$name/$BIN" "$tmp/$BIN"
}

# build compiles the release from source, as before there were binaries.
build() {
	if is_termux && { ! has git || ! has go || ! has clang; }; then
		die "building ghostty-config on Termux needs git, Go and clang: pkg install git golang clang"
	fi
	if ! has git || ! has go; then
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
	# a Go installed some other way still builds a binary Android runs. cgo is
	# not optional: Android has no /etc/resolv.conf, so without libc's resolver
	# every name lookup fails with "connection refused".
	cgo=0
	if is_termux && [ -n "$arch" ]; then
		GOOS=android
		GOARCH="$arch"
		export GOOS GOARCH
		cgo=1
	fi
	git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$tag" "$REPO" "$tmp/src"
	(cd "$tmp/src" && CGO_ENABLED=$cgo go build -trimpath -ldflags "-s -w -X main.version=$tag" -o "$tmp/$BIN" ./cmd/ghostty-config)
}

downloaded=""
if [ -n "${GHOSTTY_CONFIG_FROM_SOURCE:-}" ] || ! download; then
	build
else
	downloaded=1
fi

# --- install ------------------------------------------------------------------------

mkdir -p "$dest"
new="$dest/.$BIN.new"
install -m 755 "$tmp/$BIN" "$new"

# A binary can be built for this system and still be refused by it (a Linux
# build under Termux, say): where the download does not run, build instead.
# The test happens where the binary will live, since a temporary directory may
# be mounted without permission to execute.
if [ -n "$downloaded" ] && ! "$new" -version >/dev/null 2>&1; then
	build
	install -m 755 "$tmp/$BIN" "$new"
fi
"$new" -version >/dev/null 2>&1 || die "the binary does not run on this system ($(uname -s) $(uname -m))"
mv -f "$new" "$dest/$BIN"
say "Installed $("$dest/$BIN" -version 2>/dev/null || echo "$BIN") to $dest/$BIN"
case ":$PATH:" in
*":$dest:"*) say "Run it: $BIN" ;;
*) say "Add $dest to your PATH to run it as $BIN" ;;
esac
