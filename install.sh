#!/bin/sh
set -e

CDN_BASE="https://downloads.asksurf.ai/cli/releases"

# Require curl or wget.
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1"; }
  download() { curl -fSL -o "$2" "$1"; }
  download_quiet() { curl -fsSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO- "$1"; }
  download() { wget -q -O "$2" "$1"; }
  download_quiet() { wget -q -O "$2" "$1"; }
else
  echo "Error: curl or wget is required" >&2
  exit 1
fi

# Detect OS.
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  mingw*|msys*|cygwin*) OS="windows" ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture.
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Rosetta 2 detection: prefer native arm64 binary on Apple Silicon.
if [ "$OS" = "darwin" ] && [ "$ARCH" = "amd64" ]; then
  if [ "$(sysctl -n sysctl.proc_translated 2>/dev/null)" = "1" ]; then
    ARCH="arm64"
  fi
fi

# musl detection on Linux.
if [ "$OS" = "linux" ]; then
  if [ -f /lib/libc.musl-x86_64.so.1 ] || [ -f /lib/libc.musl-aarch64.so.1 ] || ldd /bin/ls 2>&1 | grep -q musl; then
    ARCH="${ARCH}-musl"
  fi
fi

# Get version.
VERSION="${1:-$(fetch "${CDN_BASE}/latest")}"
if [ -z "$VERSION" ]; then
  echo "Error: could not determine latest version" >&2
  exit 1
fi

# Build filename (bare binary, no archive).
FILENAME="surf_${OS}_${ARCH}"
if [ "$OS" = "windows" ]; then
  FILENAME="${FILENAME}.exe"
fi

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading surf ${VERSION} for ${OS}/${ARCH}..."
download "${CDN_BASE}/${VERSION}/${FILENAME}" "${TMPDIR}/${FILENAME}"
download_quiet "${CDN_BASE}/${VERSION}/checksums.txt" "${TMPDIR}/checksums.txt"

# Verify checksum.
EXPECTED=$(grep "${FILENAME}" "${TMPDIR}/checksums.txt" | awk '{print $1}')
if [ -z "$EXPECTED" ]; then
  echo "Error: checksum not found for ${FILENAME}" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "${TMPDIR}/${FILENAME}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "${TMPDIR}/${FILENAME}" | awk '{print $1}')
else
  echo "Warning: no sha256 tool found, skipping checksum verification"
  ACTUAL="$EXPECTED"
fi

if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Error: checksum mismatch" >&2
  echo "  expected: ${EXPECTED}" >&2
  echo "  actual:   ${ACTUAL}" >&2
  exit 1
fi

# Run surf install.
chmod +x "${TMPDIR}/${FILENAME}"
"${TMPDIR}/${FILENAME}" install --local
