#!/usr/bin/env python3
"""Packs the release binaries of ghostty-config into wheels for PyPI.

    python3 pypi/build_wheels.py VERSION RELEASE_DIR OUT_DIR

RELEASE_DIR holds the folders the release workflow builds,
ghostty-config_VERSION_OS_ARCH/ghostty-config. Each binary becomes one wheel
tagged for its system, with the binary in the wheel's scripts: pip, pipx and
uv put it straight on the PATH, with no Python between the user and the tool.

There is no source distribution: building from source needs Go, and pip
reports a system with no wheel plainly. Standard library only.
"""

import base64
import hashlib
import sys
import zipfile
from pathlib import Path

NAME = "ghostty-config"
DIST = "ghostty_config"
REPO = "https://github.com/vitruves/ghostty-config"
SITE = "https://vitruves.github.io/ghostty-config/"
RAW = "https://raw.githubusercontent.com/vitruves/ghostty-config"

# Go 1.26 needs macOS 12 or newer. The Linux binaries are static
# (CGO_ENABLED=0), so they run on glibc and musl systems alike.
PLATFORMS = {
    "darwin_amd64": "macosx_12_0_x86_64",
    "darwin_arm64": "macosx_12_0_arm64",
    "linux_amd64": "manylinux_2_17_x86_64.manylinux2014_x86_64.musllinux_1_1_x86_64",
    "linux_arm64": "manylinux_2_17_aarch64.manylinux2014_aarch64.musllinux_1_1_aarch64",
}

# A fixed date keeps the wheels byte for byte reproducible.
DATE = (2020, 1, 1, 0, 0, 0)


def readme(root: Path, version: str) -> str:
    """The README with its relative links pointed at GitHub, as PyPI shows it."""
    text = (root / "README.md").read_text(encoding="utf-8")
    text = text.replace('src="img/', f'src="{RAW}/{version}/img/')
    text = text.replace('href="LICENSE"', f'href="{REPO}/blob/{version}/LICENSE"')
    text = text.replace("](CHANGELOG.md)", f"]({REPO}/blob/{version}/CHANGELOG.md)")
    return text


def metadata(root: Path, version: str) -> str:
    head = [
        "Metadata-Version: 2.4",
        f"Name: {NAME}",
        f"Version: {version}",
        "Summary: Themes, fonts, shortcuts and every everyday setting of Ghostty, in a full-screen interface inside the terminal.",
        "Author: Johan H.G. Natter",
        "License-Expression: MIT",
        "License-File: LICENSE",
        f"Project-URL: Homepage, {SITE}",
        f"Project-URL: Source, {REPO}",
        f"Project-URL: Changelog, {REPO}/blob/main/CHANGELOG.md",
        f"Project-URL: Issues, {REPO}/issues",
        "Keywords: ghostty,terminal,config,themes,tui",
        "Classifier: Environment :: Console",
        "Classifier: Operating System :: MacOS",
        "Classifier: Operating System :: POSIX :: Linux",
        "Classifier: Topic :: Terminals :: Terminal Emulators/X Terminals",
        "Description-Content-Type: text/markdown",
    ]
    return "\n".join(head) + "\n\n" + readme(root, version)


def build(root: Path, version: str, binary: Path, platform: str, out: Path) -> Path:
    info = f"{DIST}-{version}.dist-info"
    files = [
        (f"{DIST}-{version}.data/scripts/{NAME}", binary.read_bytes(), 0o755),
        (f"{info}/licenses/LICENSE", (root / "LICENSE").read_bytes(), 0o644),
        (f"{info}/METADATA", metadata(root, version).encode(), 0o644),
        (
            f"{info}/WHEEL",
            (
                "Wheel-Version: 1.0\n"
                "Generator: ghostty-config build_wheels.py\n"
                "Root-Is-Purelib: false\n"
                + "".join(f"Tag: py3-none-{p}\n" for p in platform.split("."))
            ).encode(),
            0o644,
        ),
    ]
    record = []
    for path, data, _ in files:
        digest = base64.urlsafe_b64encode(hashlib.sha256(data).digest()).rstrip(b"=").decode()
        record.append(f"{path},sha256={digest},{len(data)}")
    record.append(f"{info}/RECORD,,")
    files.append((f"{info}/RECORD", ("\n".join(record) + "\n").encode(), 0o644))

    wheel = out / f"{DIST}-{version}-py3-none-{platform}.whl"
    with zipfile.ZipFile(wheel, "w", zipfile.ZIP_DEFLATED) as z:
        for path, data, mode in files:
            entry = zipfile.ZipInfo(path, DATE)
            entry.external_attr = (0o100000 | mode) << 16
            entry.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(entry, data)
    return wheel


def main() -> None:
    if len(sys.argv) != 4:
        sys.exit(__doc__.split("\n\n")[1])
    version, release, out = sys.argv[1], Path(sys.argv[2]), Path(sys.argv[3])
    root = Path(__file__).resolve().parent.parent
    out.mkdir(parents=True, exist_ok=True)
    for target, platform in PLATFORMS.items():
        binary = release / f"{NAME}_{version}_{target}" / NAME
        if not binary.is_file():
            sys.exit(f"✗ missing {binary}")
        print(build(root, version, binary, platform, out).name)


if __name__ == "__main__":
    main()
