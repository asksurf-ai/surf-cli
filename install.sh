#!/bin/sh
set -e

REPO="cyberconnecthq/surf-cli"
CDN_BASE="https://agent.asksurf.ai/cli/releases"
INSTALL_DIR="/usr/local/bin"

# Detect OS
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$OS" in
  linux)  OS="linux" ;;
  darwin) OS="darwin" ;;
  mingw*|msys*|cygwin*) OS="windows" ;;
  *) echo "Unsupported OS: $OS" >&2; exit 1 ;;
esac

# Detect architecture
ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

# Get latest version from GitHub tags API
get_latest_version() {
  local url="https://api.github.com/repos/${REPO}/tags?per_page=1"
  if command -v curl >/dev/null 2>&1; then
    curl -sL "$url" | grep '"name"' | head -1 | sed -E 's/.*"name": *"([^"]+)".*/\1/'
  elif command -v wget >/dev/null 2>&1; then
    wget -qO- "$url" | grep '"name"' | head -1 | sed -E 's/.*"name": *"([^"]+)".*/\1/'
  else
    echo "Error: curl or wget is required" >&2
    exit 1
  fi
}

VERSION="${1:-$(get_latest_version)}"
if [ -z "$VERSION" ]; then
  echo "Error: could not determine latest version" >&2
  exit 1
fi

# Build download URL
if [ "$OS" = "windows" ]; then
  FILENAME="surf_${OS}_${ARCH}.zip"
else
  FILENAME="surf_${OS}_${ARCH}.tar.gz"
fi
URL="${CDN_BASE}/${VERSION}/${FILENAME}"
CHECKSUM_URL="${CDN_BASE}/${VERSION}/checksums.txt"

TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading surf ${VERSION} for ${OS}/${ARCH}..."

# Download archive and checksums
if command -v curl >/dev/null 2>&1; then
  curl -fSL -o "${TMPDIR}/${FILENAME}" "$URL"
  curl -fSL -o "${TMPDIR}/checksums.txt" "$CHECKSUM_URL"
else
  wget -q -O "${TMPDIR}/${FILENAME}" "$URL"
  wget -q -O "${TMPDIR}/checksums.txt" "$CHECKSUM_URL"
fi

# Verify checksum
echo "Verifying checksum..."
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

# Extract
echo "Extracting..."
if [ "$OS" = "windows" ]; then
  unzip -q "${TMPDIR}/${FILENAME}" -d "${TMPDIR}"
else
  tar -xzf "${TMPDIR}/${FILENAME}" -C "${TMPDIR}"
fi

# Install
BINARY="surf"
if [ "$OS" = "windows" ]; then
  BINARY="surf.exe"
fi

if [ -w "$INSTALL_DIR" ]; then
  mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mv "${TMPDIR}/${BINARY}" "${INSTALL_DIR}/${BINARY}"
fi

chmod +x "${INSTALL_DIR}/${BINARY}"

echo "surf ${VERSION} installed to ${INSTALL_DIR}/${BINARY}"
