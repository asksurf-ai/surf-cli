#!/bin/sh
# Installs the surf CLI from GitHub Releases.
#
#   curl -fsSL https://github.com/asksurf-ai/surf-cli/releases/latest/download/install.sh | sh
#   curl -fsSL https://github.com/asksurf-ai/surf-cli/releases/latest/download/install.sh | sh -s v1.2.3
#
# The binary is always checked against the release's checksums.txt. When the
# GitHub CLI (gh) is installed and signed in, its build provenance attestation
# is verified too.
set -e

REPO="asksurf-ai/surf-cli"
RELEASES_URL="https://github.com/${REPO}/releases"
SIGNER_WORKFLOW="${REPO}/.github/workflows/release.yml"

# Require curl or wget.
if command -v curl >/dev/null 2>&1; then
  download() { curl -fSL --proto '=https' --tlsv1.2 -o "$2" "$1"; }
  download_quiet() { curl -fsSL --proto '=https' --tlsv1.2 -o "$2" "$1"; }
  # Print the redirect target of a URL without following it.
  redirect_target() { curl -fsS --proto '=https' --tlsv1.2 -o /dev/null -w '%{redirect_url}' "$1"; }
elif command -v wget >/dev/null 2>&1; then
  # Options that GNU wget and BusyBox wget (Alpine) both support.
  download() { wget -q -O "$2" "$1"; }
  download_quiet() { wget -q -O "$2" "$1"; }
  # Print the first redirect target of a URL.
  redirect_target() { wget -S --spider "$1" 2>&1 | sed -n 's/^ *[Ll]ocation: *//p' | tr -d '\r' | head -n 1; }
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

# Get version: the argument, or the tag GitHub's /releases/latest redirects to.
# (The binaries are static, CGO_ENABLED=0, so one Linux build serves glibc and
# musl alike.)
if [ -n "${1:-}" ]; then
  VERSION="$1"
else
  LATEST_URL=$(redirect_target "${RELEASES_URL}/latest" || true)
  case "$LATEST_URL" in
    */releases/tag/*) VERSION="${LATEST_URL##*/releases/tag/}" ;;
    *) VERSION="" ;;
  esac
fi
if [ -z "$VERSION" ]; then
  echo "Error: could not determine latest version" >&2
  exit 1
fi
# The version becomes part of a URL and a directory name: allow only v<semver>.
if ! printf '%s\n' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
  echo "Error: invalid version: $VERSION (expected a tag like v1.2.3)" >&2
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
download "${RELEASES_URL}/download/${VERSION}/${FILENAME}" "${TMPDIR}/${FILENAME}"
download_quiet "${RELEASES_URL}/download/${VERSION}/checksums.txt" "${TMPDIR}/checksums.txt"

# Verify checksum (exact file name match).
EXPECTED=$(awk -v f="$FILENAME" '$2 == f { print $1 }' "${TMPDIR}/checksums.txt")
if ! printf '%s\n' "$EXPECTED" | grep -Eq '^[0-9a-f]{64}$'; then
  echo "Error: checksum not found for ${FILENAME}" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "${TMPDIR}/${FILENAME}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "${TMPDIR}/${FILENAME}" | awk '{print $1}')
else
  echo "Error: sha256sum or shasum is required to verify the download" >&2
  exit 1
fi

if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Error: checksum mismatch" >&2
  echo "  expected: ${EXPECTED}" >&2
  echo "  actual:   ${ACTUAL}" >&2
  exit 1
fi
echo "Checksum verified."

# Verify the build provenance attestation when gh can (it needs a signed-in gh).
if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
  if ! gh attestation verify "${TMPDIR}/${FILENAME}" \
      --repo "$REPO" \
      --signer-workflow "$SIGNER_WORKFLOW" \
      --source-ref "refs/tags/${VERSION}" \
      --deny-self-hosted-runners >/dev/null; then
    echo "Error: build provenance attestation verification failed for ${FILENAME}" >&2
    exit 1
  fi
  echo "Build provenance attestation verified."
else
  echo "Note: build provenance not verified (needs a signed-in GitHub CLI). To check it yourself:"
  echo "  gh attestation verify <file> --repo ${REPO} --signer-workflow ${SIGNER_WORKFLOW}"
fi

# Run surf install.
chmod +x "${TMPDIR}/${FILENAME}"
"${TMPDIR}/${FILENAME}" install --local
