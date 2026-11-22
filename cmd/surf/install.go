package main

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// releasesURL is the GitHub Releases page that carries the surf binaries,
// checksums.txt and install.sh. A variable so tests can point it at a local
// server.
var releasesURL = "https://github.com/asksurf-ai/surf-cli/releases"

// The repository and workflow that sign the build provenance attestation of
// every release file.
const (
	releaseRepo           = "asksurf-ai/surf-cli"
	releaseSignerWorkflow = "asksurf-ai/surf-cli/.github/workflows/release.yml"
)

// Limits on release downloads.
const (
	maxRedirects      = 5
	maxChecksumsBytes = 1 << 20   // checksums.txt
	maxBinaryBytes    = 512 << 20 // a surf binary
	downloadTimeout   = 10 * time.Minute
)

// releaseTagPattern is the shape of a release tag. The tag names a directory
// under ~/.local/share/surf/versions, so anything else is refused.
var releaseTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

// minReleaseTag is the oldest release surf installs: the first one published
// to GitHub Releases with checksums.txt and build provenance attestations, and
// the first immutable one. Earlier releases (v1.0.0 to v1.0.11) have no assets
// and can still be changed, so anyone who can write to the repository could
// attach files to one. install.sh has the same floor.
const minReleaseTag = "v1.0.12"

// releaseFloorReason says why a release below minReleaseTag is refused.
const releaseFloorReason = "surf installs only " + minReleaseTag + " or newer, the releases published with checksums and build provenance attestations; earlier releases can still be changed"

// belowReleaseFloor reports whether a release tag is older than minReleaseTag,
// by semver precedence, so prereleases of v1.0.12 are below it too. A tag that
// is not a version is below it.
func belowReleaseFloor(tag string) bool {
	c, ok := compareVersions(tag, minReleaseTag)
	return !ok || c < 0
}

// sha256Pattern is a hex-encoded SHA-256 digest.
var sha256Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// minSkillVersion is the minimum surf skill version required by this CLI release.
// Bump this when SKILL.md has breaking changes that require a skill update.
const minSkillVersion = "0.0.2"

// httpClient fetches release files. It follows only https redirects, at most
// maxRedirects of them, and gives up on a request after downloadTimeout.
var httpClient = &http.Client{
	Timeout: downloadTimeout,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return fmt.Errorf("refusing a redirect to non-https URL %s", req.URL.Redacted())
		}
		if len(via) > maxRedirects {
			return fmt.Errorf("stopped after %d redirects", maxRedirects)
		}
		return nil
	},
}

func installDir(home string) string {
	return filepath.Join(home, ".local", "bin")
}

// displayVersion is the one spelling of a version that surf uses for its
// directory and in messages: without a tag's leading "v", the way release
// builds embed it.
func displayVersion(ver string) string {
	return strings.TrimPrefix(ver, "v")
}

func versionDir(home, ver string) string {
	return filepath.Join(home, ".local", "share", "surf", "versions", displayVersion(ver))
}

// installFile copies src to dst through a temporary file in dst's directory,
// which it syncs and then renames over dst. It never writes into an existing
// dst, which may be the running binary.
func installFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), "."+filepath.Base(dst)+".tmp-*")
	if err != nil {
		return err
	}
	renamed := false
	defer func() {
		if !renamed {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()

	if _, err := io.Copy(tmp, in); err != nil {
		return err
	}
	if err := tmp.Chmod(0755); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		return err
	}
	renamed = true
	return nil
}

// installAside installs src at dst, which may be the running binary on
// Windows: Windows cannot replace a running executable but can rename it, so
// dst is first renamed to dst+".old", which the next install removes.
func installAside(src, dst string) error {
	old := dst + ".old"
	if err := os.Remove(old); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove %s from an earlier update: %w", old, err)
	}
	if err := os.Rename(dst, old); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := installFile(src, dst); err != nil {
		os.Rename(old, dst)
		return err
	}
	return nil
}

// createSymlink points link at target. It creates the new symlink under a
// temporary name in link's directory and renames it over link, so link names
// the old or the new target at every moment.
func createSymlink(target, link string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return err
	}
	tmp := fmt.Sprintf("%s.tmp-%d", link, os.Getpid())
	if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(target, tmp); err != nil {
		return err
	}
	if err := os.Rename(tmp, link); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("cannot replace %s: %w", link, err)
	}
	return nil
}

func detectShellRC(home, shell string) string {
	base := filepath.Base(shell)
	switch base {
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "bash":
		return filepath.Join(home, ".bashrc")
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish")
	default:
		return ""
	}
}

func addToPath(home, binDir, shell string) string {
	if shell == "" {
		return fmt.Sprintf("Add this to your shell profile:\n  export PATH=\"%s:$PATH\"", binDir)
	}

	rcFile := detectShellRC(home, shell)
	if rcFile == "" {
		return fmt.Sprintf("Add this to your shell profile:\n  export PATH=\"%s:$PATH\"", binDir)
	}

	// Check if already in rc file.
	content, _ := os.ReadFile(rcFile)
	if strings.Contains(string(content), binDir) {
		return ""
	}

	// Append PATH line.
	var line string
	if filepath.Base(shell) == "fish" {
		line = fmt.Sprintf("set -gx PATH %s $PATH", binDir)
	} else {
		line = fmt.Sprintf("export PATH=\"%s:$PATH\"", binDir)
	}

	f, err := os.OpenFile(rcFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Sprintf("Add this to your shell profile:\n  %s", line)
	}
	defer f.Close()
	fmt.Fprintf(f, "\n# surf CLI\n%s\n", line)

	return fmt.Sprintf("Added %s to PATH in %s\nRestart your shell or run: source %s", binDir, rcFile, rcFile)
}

// get fetches an https URL with httpClient and fails on any status but 200.
func get(url string) (*http.Response, error) {
	if !strings.HasPrefix(url, "https://") {
		return nil, fmt.Errorf("refusing non-https URL %s", url)
	}
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	return resp, nil
}

// fetchURL downloads a URL of at most limit bytes and returns the body.
func fetchURL(url string, limit int64) (string, error) {
	resp, err := get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return "", err
	}
	if int64(len(body)) > limit {
		return "", fmt.Errorf("%s is larger than %d bytes", url, limit)
	}
	return string(body), nil
}

// downloadToFile downloads a URL of at most limit bytes to a new file.
func downloadToFile(url, dst string, limit int64) error {
	resp, err := get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer out.Close()

	n, err := io.Copy(out, io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return fmt.Errorf("%s is larger than %d bytes", url, limit)
	}
	return out.Close()
}

// sha256File returns the hex-encoded SHA256 hash of a file.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// latestReleaseTag returns the tag of the latest GitHub release. GitHub
// answers /releases/latest with a redirect to /releases/tag/<tag>; the
// redirect is read, not followed.
func latestReleaseTag() (string, error) {
	client := *httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}
	resp, err := client.Get(releasesURL + "/latest")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return "", fmt.Errorf("HTTP %d from %s/latest, expected a redirect to the latest release", resp.StatusCode, releasesURL)
	}
	location := resp.Header.Get("Location")
	i := strings.LastIndex(location, "/releases/tag/")
	if i < 0 {
		return "", fmt.Errorf("no release found at %s/latest", releasesURL)
	}
	tag := location[i+len("/releases/tag/"):]
	if !releaseTagPattern.MatchString(tag) {
		return "", fmt.Errorf("latest release tag %q is not a version tag", tag)
	}
	return tag, nil
}

// semver is a parsed [v]<major>.<minor>.<patch>[-<prerelease>] version.
type semver struct {
	core [3]uint64
	pre  []string
}

func parseSemver(ver string) (semver, bool) {
	ver = displayVersion(ver)
	if !releaseTagPattern.MatchString("v" + ver) {
		return semver{}, false
	}
	core, pre, _ := strings.Cut(ver, "-")
	var s semver
	for i, part := range strings.Split(core, ".") {
		n, err := strconv.ParseUint(part, 10, 64)
		if err != nil {
			return semver{}, false
		}
		s.core[i] = n
	}
	if pre != "" {
		s.pre = strings.Split(pre, ".")
	}
	return s, true
}

// compareVersions orders two versions by semver precedence and returns -1, 0
// or +1. ok is false when either is not a version (a "dev" build).
func compareVersions(a, b string) (c int, ok bool) {
	x, okA := parseSemver(a)
	y, okB := parseSemver(b)
	if !okA || !okB {
		return 0, false
	}
	for i := range x.core {
		if x.core[i] != y.core[i] {
			return cmp.Compare(x.core[i], y.core[i]), true
		}
	}
	switch {
	case len(x.pre) == 0 && len(y.pre) == 0:
		return 0, true
	case len(x.pre) == 0:
		return 1, true
	case len(y.pre) == 0:
		return -1, true
	}
	for i := 0; i < len(x.pre) && i < len(y.pre); i++ {
		if c := comparePrereleaseIDs(x.pre[i], y.pre[i]); c != 0 {
			return c, true
		}
	}
	return cmp.Compare(len(x.pre), len(y.pre)), true
}

// comparePrereleaseIDs orders two dot-separated prerelease identifiers:
// numeric ones by value and before alphanumeric ones, which order as text.
func comparePrereleaseIDs(a, b string) int {
	an, errA := strconv.ParseUint(a, 10, 64)
	bn, errB := strconv.ParseUint(b, 10, 64)
	switch {
	case errA == nil && errB == nil:
		return cmp.Compare(an, bn)
	case errA == nil:
		return -1
	case errB == nil:
		return 1
	}
	return strings.Compare(a, b)
}

// releaseAssetName is the release asset name of the surf binary for a platform.
func releaseAssetName(goos, goarch string) string {
	name := fmt.Sprintf("surf_%s_%s", goos, goarch)
	if goos == "windows" {
		name += ".exe"
	}
	return name
}

// checksumFor returns the SHA-256 that a goreleaser checksums.txt lists for
// the named file. Every line must be "<sha256> <name>", and the file must be
// listed exactly once.
func checksumFor(checksums, filename string) (string, error) {
	var found []string
	for _, line := range strings.Split(checksums, "\n") {
		parts := strings.Fields(line)
		if len(parts) == 0 {
			continue
		}
		if len(parts) != 2 {
			return "", fmt.Errorf("malformed checksums.txt line %q", line)
		}
		if parts[1] == filename {
			found = append(found, parts[0])
		}
	}
	switch {
	case len(found) == 0:
		return "", fmt.Errorf("checksum not found for %s", filename)
	case len(found) > 1:
		return "", fmt.Errorf("checksums.txt lists %s %d times", filename, len(found))
	case !sha256Pattern.MatchString(found[0]):
		return "", fmt.Errorf("malformed checksum for %s", filename)
	}
	return found[0], nil
}

// verifyAttestation checks a downloaded release file's build provenance
// attestation with the GitHub CLI, as install.sh does. It runs when gh is
// installed and signed in to github.com, which gh answers from local state;
// then any failure, a network error included, aborts the install. Otherwise
// it warns that the attestation was not checked.
func verifyAttestation(path, tag string) error {
	gh, err := exec.LookPath("gh")
	if err != nil || exec.Command(gh, "auth", "token", "--hostname", "github.com").Run() != nil {
		fmt.Fprintln(os.Stderr, "Warning: build provenance not verified (needs the GitHub CLI, gh, signed in to github.com).")
		return nil
	}
	cmd := exec.Command(gh, "attestation", "verify", path,
		"--repo", releaseRepo,
		"--signer-workflow", releaseSignerWorkflow,
		"--source-ref", "refs/tags/"+tag,
		"--deny-self-hosted-runners")
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build provenance attestation verification failed for %s: %w", filepath.Base(path), err)
	}
	fmt.Println("Build provenance attestation verified.")
	return nil
}

// downloadRelease downloads the surf binary of a release to a temp file and
// verifies it against the release's checksums.txt and, when gh can, its build
// provenance attestation. Returns the path to the downloaded binary.
func downloadRelease(tag string) (string, error) {
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	filename := releaseAssetName(goos, goarch)
	base := releasesURL + "/download/" + tag

	fmt.Printf("Downloading surf %s for %s/%s...\n", tag, goos, goarch)

	tmpDir, err := os.MkdirTemp("", "surf-install-*")
	if err != nil {
		return "", err
	}
	fail := func(err error) (string, error) {
		os.RemoveAll(tmpDir)
		return "", err
	}

	binaryPath := filepath.Join(tmpDir, filename)
	if err := downloadToFile(base+"/"+filename, binaryPath, maxBinaryBytes); err != nil {
		return fail(fmt.Errorf("download failed: %w", err))
	}

	checksums, err := fetchURL(base+"/checksums.txt", maxChecksumsBytes)
	if err != nil {
		return fail(fmt.Errorf("could not download checksums: %w", err))
	}
	expected, err := checksumFor(checksums, filename)
	if err != nil {
		return fail(err)
	}
	actual, err := sha256File(binaryPath)
	if err != nil {
		return fail(fmt.Errorf("could not hash downloaded binary: %w", err))
	}
	if actual != expected {
		return fail(fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual))
	}
	fmt.Println("Checksum verified.")

	if err := verifyAttestation(binaryPath, tag); err != nil {
		return fail(err)
	}
	return binaryPath, nil
}

// selfUpdate installs the named release, or the latest one. It never installs
// a release older than minReleaseTag. It refuses to move to a release older
// than the running surf unless the user named that version, and does nothing
// when the release is the running version.
func selfUpdate(home string, args []string) error {
	var tag string
	if len(args) == 1 {
		tag = args[0]
		if !releaseTagPattern.MatchString(tag) {
			return fmt.Errorf("invalid version %q (expected a tag like v1.2.3)", tag)
		}
		if belowReleaseFloor(tag) {
			return fmt.Errorf("refusing surf %s: %s", tag, releaseFloorReason)
		}
	} else {
		latest, err := latestReleaseTag()
		if err != nil {
			return fmt.Errorf("could not determine latest version: %w", err)
		}
		if belowReleaseFloor(latest) {
			return fmt.Errorf("refusing the latest release %s: %s", latest, releaseFloorReason)
		}
		if c, ok := compareVersions(latest, version); ok && c < 0 {
			return fmt.Errorf("the latest release %s is older than this surf (%s); to install it anyway, run: surf install %s", latest, displayVersion(version), latest)
		}
		tag = latest
	}

	if displayVersion(tag) == displayVersion(version) {
		fmt.Printf("surf %s is already up to date.\n", displayVersion(version))
		return nil
	}

	binaryPath, err := downloadRelease(tag)
	if err != nil {
		return err
	}
	defer os.RemoveAll(filepath.Dir(binaryPath))

	return installBinary(home, binaryPath, tag)
}

// installBinary installs a binary from srcPath at the given version.
func installBinary(home, srcPath, ver string) error {
	binDir := installDir(home)
	verDir := versionDir(home, ver)
	binaryName := "surf"
	if runtime.GOOS == "windows" {
		binaryName = "surf.exe"
	}

	dst := filepath.Join(verDir, binaryName)
	if err := installFile(srcPath, dst); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	linkPath := filepath.Join(binDir, binaryName)
	if runtime.GOOS == "windows" {
		if err := installAside(dst, linkPath); err != nil {
			return fmt.Errorf("failed to copy binary to %s: %w", binDir, err)
		}
	} else {
		if err := createSymlink(dst, linkPath); err != nil {
			return fmt.Errorf("failed to create symlink: %w", err)
		}
	}

	// PATH setup.
	pathMsg := ""
	pathEnv := os.Getenv("PATH")
	if !strings.Contains(pathEnv, binDir) {
		pathMsg = addToPath(home, binDir, os.Getenv("SHELL"))
	}

	// Print success.
	displayPath := linkPath
	if strings.HasPrefix(linkPath, home) {
		displayPath = "~" + linkPath[len(home):]
	}

	green := "\033[32m"
	bold := "\033[1m"
	reset := "\033[0m"

	fmt.Println()
	fmt.Printf("%s✔ surf successfully installed!%s\n", green, reset)
	fmt.Println()
	fmt.Printf("  Version: %s%s%s\n", green, displayVersion(ver), reset)
	fmt.Printf("  Location: %s\n", displayPath)
	fmt.Printf("  Minimum skill version: v%s\n", minSkillVersion)
	fmt.Println()
	if pathMsg != "" {
		fmt.Printf("  %s\n", pathMsg)
		fmt.Println()
	}
	fmt.Printf("  Next: Run %ssurf --help%s to get started\n", bold, reset)
	fmt.Println()

	return nil
}

func newInstallCmd() *cobra.Command {
	var local bool

	cmd := &cobra.Command{
		Use:   "install [version]",
		Short: "Install or update surf",
		Long: "Downloads the latest surf release, or the named version (for example v1.2.3), and installs it to ~/.local/bin/surf. " +
			"It installs only " + minReleaseTag + " or newer. Without a version it refuses a release older than this surf. " +
			"Use --local to install the current binary without downloading.",
		Args:   cobra.MaximumNArgs(1),
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}

			if local {
				if len(args) > 0 {
					return fmt.Errorf("--local installs the running binary and takes no version")
				}
				// Bootstrap mode: install the current binary.
				exe, err := os.Executable()
				if err != nil {
					return fmt.Errorf("cannot determine executable path: %w", err)
				}
				return installBinary(home, exe, version)
			}

			// Self-update mode: download a GitHub release.
			return selfUpdate(home, args)
		},
	}

	cmd.Flags().BoolVar(&local, "local", false, "Install the current binary without downloading")

	return cmd
}
