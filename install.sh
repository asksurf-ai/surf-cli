#!/bin/sh
# Installs the surf CLI from a GitHub release.
#
# Download this script from a named release, check it, then run it; the
# script runs only if the download and the check pass:
#
#   VERSION=v1.2.3
#   DIR=$(mktemp -d)
#   ( cd "$DIR" &&
#     curl -fsSLO "https://github.com/asksurf-ai/surf-cli/releases/download/${VERSION}/install.sh" &&
#     gh attestation verify install.sh --repo asksurf-ai/surf-cli \
#       --signer-workflow asksurf-ai/surf-cli/.github/workflows/release.yml \
#       --source-ref "refs/tags/${VERSION}" --deny-self-hosted-runners &&
#     sh install.sh "$VERSION" )
#   rm -rf "$DIR"
#
# It installs only v1.0.12 or newer: earlier releases have no checksums or
# attestations and can still be changed. Without a version it installs the
# latest release, and refuses one older than the surf already installed in
# ~/.local/bin. The binary is always checked against the release's
# checksums.txt. When the GitHub CLI (gh) is installed and signed in to
# github.com, its build provenance attestation is checked too, and any
# failure of that check aborts the install.
set -e
LC_ALL=C
export LC_ALL

REPO="asksurf-ai/surf-cli"
RELEASES_URL="https://github.com/${REPO}/releases"
SIGNER_WORKFLOW="${REPO}/.github/workflows/release.yml"
MAX_REDIRECTS=5
# The oldest release this script installs: the first one published with
# checksums.txt and build provenance attestations, and the first immutable one.
# cmd/surf/install.go has the same floor.
MIN_VERSION="v1.0.12"

WORKDIR=$(mktemp -d)
cleanup() { rm -rf "$WORKDIR"; }
trap cleanup EXIT
trap 'cleanup; exit 130' INT
trap 'cleanup; exit 143' TERM

# Require curl or wget. Every URL and every redirect must be https.
if command -v curl >/dev/null 2>&1; then
  CURL="curl --proto =https --proto-redir =https --max-redirs ${MAX_REDIRECTS} --tlsv1.2 --connect-timeout 15 --max-time 600"
  download() { $CURL -fSL -o "$2" "$1"; }
  download_quiet() { $CURL -fsSL -o "$2" "$1"; }
  # Print the redirect target of a URL without following it.
  redirect_target() { $CURL -fsS -o /dev/null -w '%{redirect_url}' "$1"; }
elif command -v wget >/dev/null 2>&1; then
  # wget cannot be told to refuse a redirect to http: GNU wget's --https-only
  # applies only to recursive downloads, and BusyBox wget (Alpine) has neither
  # it nor --max-redirect. So read every redirect wget reports (-S) and fail
  # on a non-https one, or on more than MAX_REDIRECTS. Options that GNU wget
  # and BusyBox wget both support.
  # Print the Location of every redirect in a wget -S log.
  locations() { sed -n 's/^ *[Ll][Oo][Cc][Aa][Tt][Ii][Oo][Nn]: *//p' "$1" | tr -d '\r'; }
  check_redirects() {
    LOCATIONS=$(locations "$1")
    [ -z "$LOCATIONS" ] && return 0
    if [ "$(printf '%s\n' "$LOCATIONS" | wc -l)" -gt "$MAX_REDIRECTS" ]; then
      echo "Error: more than ${MAX_REDIRECTS} redirects" >&2
      return 1
    fi
    if printf '%s\n' "$LOCATIONS" | grep -qv '^https://'; then
      echo "Error: refusing a redirect to a non-https URL" >&2
      return 1
    fi
  }
  download() {
    if ! wget -q -S -T 60 -O "$2" "$1" 2>"${WORKDIR}/wget.log" || ! check_redirects "${WORKDIR}/wget.log"; then
      rm -f "$2"
      echo "Error: could not download $1" >&2
      return 1
    fi
  }
  download_quiet() { download "$@"; }
  # Print the first redirect target of a URL.
  redirect_target() {
    wget -q -S -T 60 --spider "$1" 2>"${WORKDIR}/wget.log" || true
    check_redirects "${WORKDIR}/wget.log" || return 1
    locations "${WORKDIR}/wget.log" | head -n 1
  }
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
NAMED_VERSION=""
if [ -n "${1:-}" ]; then
  VERSION="$1"
  NAMED_VERSION=1
else
  LATEST_URL=$(redirect_target "${RELEASES_URL}/latest" || true)
  case "$LATEST_URL" in
    "${RELEASES_URL}/tag/"*) VERSION="${LATEST_URL#"${RELEASES_URL}/tag/"}" ;;
    *) VERSION="" ;;
  esac
fi
if [ -z "$VERSION" ]; then
  echo "Error: could not determine latest version" >&2
  exit 1
fi
# The version becomes part of a URL and a directory name: allow only v<semver>.
# Check the characters of the whole string first (grep matches line by line,
# so a newline could hide anything after a valid first line), then its shape.
case "$VERSION" in
  *[!0-9A-Za-z.-]*)
    echo "Error: invalid version (expected a tag like v1.2.3)" >&2
    exit 1
    ;;
esac
if ! printf '%s\n' "$VERSION" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$'; then
  echo "Error: invalid version: $VERSION (expected a tag like v1.2.3)" >&2
  exit 1
fi

# version_cmp A B prints 1, 0 or -1 when version A is higher than, the same as
# or lower than version B: by major.minor.patch, then a stable release above a
# prerelease of the same version (two prereleases of one version are the
# same). A and B must already be checked to be versions.
version_cmp() {
  awk -v a="$1" -v b="$2" 'BEGIN {
      sub(/^v/, "", a); sub(/^v/, "", b)
      pa = index(a, "-"); pb = index(b, "-")
      sub(/-.*/, "", a); sub(/-.*/, "", b)
      split(a, x, "."); split(b, y, ".")
      for (i = 1; i <= 3; i++)
        if (x[i] + 0 != y[i] + 0) { r = (x[i] + 0 > y[i] + 0) ? 1 : -1; print r; exit }
      r = 0
      if (!pa && pb) r = 1
      if (pa && !pb) r = -1
      print r
    }'
}

# Never install a release older than MIN_VERSION, named or latest. Those
# releases predate immutable releases and have no checksums or attestations,
# so anyone who can write to the repository could attach files to one. A
# prerelease of MIN_VERSION is older too. Anything but "same" or "higher",
# a failed comparison included, is refused.
CMP=$(version_cmp "$VERSION" "$MIN_VERSION") || CMP=""
if [ "$CMP" != 0 ] && [ "$CMP" != 1 ]; then
  WHAT="surf ${VERSION}"
  [ -n "$NAMED_VERSION" ] || WHAT="the latest release ${VERSION}"
  echo "Error: refusing ${WHAT}: this script installs only ${MIN_VERSION} or newer, the releases published with checksums and build provenance attestations; earlier releases can still be changed." >&2
  exit 1
fi

# Build filename (bare binary, no archive).
FILENAME="surf_${OS}_${ARCH}"
EXE=""
if [ "$OS" = "windows" ]; then
  EXE=".exe"
  FILENAME="${FILENAME}.exe"
fi
INSTALLED_SURF="${HOME}/.local/bin/surf${EXE}"

# Latest only moves forward: refuse a latest release older than the installed
# surf. A named version skips this check. The installed version is read from
# the directory ~/.local/bin/surf links to (~/.local/share/surf/versions/<v>/surf,
# where surf install puts it), never by running that binary: it may have come
# from the retired download host.
if [ -z "$NAMED_VERSION" ] && { [ -e "$INSTALLED_SURF" ] || [ -L "$INSTALLED_SURF" ]; }; then
  INSTALLED=""
  VERSIONS_DIR="${HOME}/.local/share/surf/versions/"
  if [ -L "$INSTALLED_SURF" ]; then
    TARGET=$(readlink "$INSTALLED_SURF" || true)
    case "$TARGET" in
      "${VERSIONS_DIR}"*/surf"${EXE}")
        INSTALLED="${TARGET#"$VERSIONS_DIR"}"
        INSTALLED="${INSTALLED%/surf"${EXE}"}"
        ;;
    esac
  fi
  # The whole string must be a version. (awk matches ^ and $ at the ends of
  # the string, where grep would match each line of a target with a newline;
  # ENVIRON, unlike awk -v, takes the value as it is.)
  if V="$INSTALLED" awk 'BEGIN { exit !(ENVIRON["V"] ~ /^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$/) }'; then
    if [ "$(version_cmp "$INSTALLED" "$VERSION")" = 1 ]; then
      echo "Error: the latest release ${VERSION} is older than the installed surf ${INSTALLED}." >&2
      echo "To install ${VERSION} anyway, name it: sh install.sh ${VERSION}" >&2
      exit 1
    fi
  else
    echo "Note: the version of ${INSTALLED_SURF} is unknown (it is not a link into ${VERSIONS_DIR}), so the check for an older release is skipped."
  fi
fi

echo "Downloading surf ${VERSION} for ${OS}/${ARCH}..."
download "${RELEASES_URL}/download/${VERSION}/${FILENAME}" "${WORKDIR}/${FILENAME}"
download_quiet "${RELEASES_URL}/download/${VERSION}/checksums.txt" "${WORKDIR}/checksums.txt"

# Verify checksum: every line is "<sha256> <name>", and the file is listed
# exactly once.
if ! EXPECTED=$(awk -v f="$FILENAME" '
    NF == 0 { next }
    NF != 2 { bad = 1 }
    $2 == f { n++; sum = $1 }
    END { if (bad || n != 1) exit 1; print sum }' "${WORKDIR}/checksums.txt") ||
   ! printf '%s\n' "$EXPECTED" | grep -Eq '^[0-9a-f]{64}$'; then
  echo "Error: checksums.txt is malformed or does not list ${FILENAME} exactly once" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "${WORKDIR}/${FILENAME}" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "${WORKDIR}/${FILENAME}" | awk '{print $1}')
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

# Verify the build provenance attestation when gh is signed in to github.com.
# `gh auth token` answers from local state, so a network failure cannot skip
# the check; once it runs, any failure aborts.
if command -v gh >/dev/null 2>&1 && gh auth token --hostname github.com >/dev/null 2>&1; then
  if ! gh attestation verify "${WORKDIR}/${FILENAME}" \
      --repo "$REPO" \
      --signer-workflow "$SIGNER_WORKFLOW" \
      --source-ref "refs/tags/${VERSION}" \
      --deny-self-hosted-runners >/dev/null; then
    echo "Error: build provenance attestation verification failed for ${FILENAME}." >&2
    echo "If gh is old (the check needs --source-ref and --deny-self-hosted-runners) or its sign-in has expired, update gh or run 'gh auth login', then retry." >&2
    exit 1
  fi
  echo "Build provenance attestation verified."
else
  echo "Warning: build provenance not verified (needs the GitHub CLI, gh, signed in to github.com). After the install, check it yourself:" >&2
  echo "  gh attestation verify ${INSTALLED_SURF} --repo ${REPO} --signer-workflow ${SIGNER_WORKFLOW} --source-ref refs/tags/${VERSION} --deny-self-hosted-runners" >&2
fi

# Run surf install.
chmod +x "${WORKDIR}/${FILENAME}"
"${WORKDIR}/${FILENAME}" install --local
