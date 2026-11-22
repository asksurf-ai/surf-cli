package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
)

func TestInstallDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got := installDir(home)
	want := filepath.Join(home, ".local", "bin")
	if got != want {
		t.Errorf("installDir() = %q, want %q", got, want)
	}
}

func TestVersionDirIsTheSameWithOrWithoutV(t *testing.T) {
	home := t.TempDir()
	want := filepath.Join(home, ".local", "share", "surf", "versions", "0.2.0")
	for _, ver := range []string{"v0.2.0", "0.2.0"} {
		if got := versionDir(home, ver); got != want {
			t.Errorf("versionDir(%q) = %q, want %q", ver, got, want)
		}
	}
}

func TestInstallFile(t *testing.T) {
	src := filepath.Join(t.TempDir(), "surf")
	dst := filepath.Join(t.TempDir(), "versions", "1.2.3", "surf")
	os.WriteFile(src, []byte("binary-content"), 0600)

	if err := installFile(src, dst); err != nil {
		t.Fatalf("installFile() error: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "binary-content" {
		t.Errorf("installed content = %q, want %q", got, "binary-content")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(dst)
		if info.Mode().Perm() != 0755 {
			t.Errorf("installed mode = %v, want 0755", info.Mode().Perm())
		}
	}
	assertOnlyEntries(t, filepath.Dir(dst), "surf")
}

// installFile must replace dst through a rename, never by writing into the
// existing file, which may be the running binary (ETXTBSY on Linux).
func TestInstallFileReplacesThroughRename(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows cannot rename over an open file; installAside covers it")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "new")
	dst := filepath.Join(dir, "surf")
	os.WriteFile(src, []byte("new"), 0600)
	os.WriteFile(dst, []byte("old"), 0755)

	running, err := os.Open(dst)
	if err != nil {
		t.Fatal(err)
	}
	defer running.Close()
	before, _ := running.Stat()

	if err := installFile(src, dst); err != nil {
		t.Fatalf("installFile() error: %v", err)
	}

	after, _ := os.Stat(dst)
	if os.SameFile(before, after) {
		t.Error("installFile wrote into the existing file instead of renaming a new one over it")
	}
	if got, _ := io.ReadAll(running); string(got) != "old" {
		t.Errorf("the open (running) file now reads %q, want it untouched", got)
	}
	if got, _ := os.ReadFile(dst); string(got) != "new" {
		t.Errorf("dst = %q, want %q", got, "new")
	}
	assertOnlyEntries(t, dir, "new", "surf")
}

func TestInstallAside(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "surf.exe")
	src := filepath.Join(dir, "src")

	os.WriteFile(src, []byte("v1"), 0600)
	if err := installAside(src, dst); err != nil {
		t.Fatalf("first installAside() error: %v", err)
	}
	os.WriteFile(src, []byte("v2"), 0600)
	if err := installAside(src, dst); err != nil {
		t.Fatalf("second installAside() error: %v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "v2" {
		t.Errorf("dst = %q, want v2", got)
	}
	if got, _ := os.ReadFile(dst + ".old"); string(got) != "v1" {
		t.Errorf("dst.old = %q, want v1", got)
	}

	os.WriteFile(src, []byte("v3"), 0600)
	if err := installAside(src, dst); err != nil {
		t.Fatalf("third installAside() error: %v", err)
	}
	if got, _ := os.ReadFile(dst + ".old"); string(got) != "v2" {
		t.Errorf("dst.old = %q, want v2 (the earlier .old removed)", got)
	}
}

// A failed install puts the renamed-aside binary back.
func TestInstallAsideRollsBack(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "surf.exe")
	os.WriteFile(dst, []byte("running"), 0755)

	if err := installAside(filepath.Join(dir, "missing"), dst); err == nil {
		t.Fatal("expected an error for a missing source")
	}
	if got, _ := os.ReadFile(dst); string(got) != "running" {
		t.Errorf("dst = %q after a failed install, want the original back", got)
	}
	assertOnlyEntries(t, dir, "surf.exe")
}

func TestCreateSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks not used on windows")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	os.WriteFile(target, []byte("x"), 0755)

	if err := createSymlink(target, link); err != nil {
		t.Fatalf("createSymlink() error: %v", err)
	}
	resolved, _ := os.Readlink(link)
	if resolved != target {
		t.Errorf("symlink points to %q, want %q", resolved, target)
	}
}

func TestCreateSymlinkOverwrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks not used on windows")
	}
	dir := t.TempDir()
	oldTarget := filepath.Join(dir, "old")
	newTarget := filepath.Join(dir, "new")
	link := filepath.Join(dir, "link")
	os.WriteFile(oldTarget, []byte("old"), 0755)
	os.WriteFile(newTarget, []byte("new"), 0755)
	os.Symlink(oldTarget, link)

	if err := createSymlink(newTarget, link); err != nil {
		t.Fatalf("createSymlink() error: %v", err)
	}
	resolved, _ := os.Readlink(link)
	if resolved != newTarget {
		t.Errorf("symlink points to %q, want %q", resolved, newTarget)
	}
	assertOnlyEntries(t, dir, "link", "new", "old")
}

// A failed switch must leave the old link in place.
func TestCreateSymlinkKeepsOldLinkOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks not used on windows")
	}
	dir := t.TempDir()
	oldTarget := filepath.Join(dir, "old")
	link := filepath.Join(dir, "link")
	os.WriteFile(oldTarget, []byte("old"), 0755)
	os.Symlink(oldTarget, link)

	// A directory with the temporary link's name makes the switch fail.
	tmp := filepath.Join(dir, "link.tmp-"+strconv.Itoa(os.Getpid()))
	os.MkdirAll(filepath.Join(tmp, "busy"), 0755)

	if err := createSymlink(filepath.Join(dir, "new"), link); err == nil {
		t.Fatal("expected an error")
	}
	if resolved, _ := os.Readlink(link); resolved != oldTarget {
		t.Errorf("link points to %q after a failed switch, want %q", resolved, oldTarget)
	}
}

func TestDetectShellRC(t *testing.T) {
	home := t.TempDir()
	tests := []struct {
		shell string
		want  string
	}{
		{"/bin/zsh", filepath.Join(home, ".zshrc")},
		{"/bin/bash", filepath.Join(home, ".bashrc")},
		{"/usr/bin/fish", filepath.Join(home, ".config", "fish", "config.fish")},
		{"/bin/unknown", ""},
	}
	for _, tt := range tests {
		t.Run(tt.shell, func(t *testing.T) {
			got := detectShellRC(home, tt.shell)
			if got != tt.want {
				t.Errorf("detectShellRC(%q) = %q, want %q", tt.shell, got, tt.want)
			}
		})
	}
}

func TestAddToPathAppendsToRC(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	rcFile := filepath.Join(home, ".zshrc")
	os.WriteFile(rcFile, []byte("# existing content\n"), 0644)

	msg := addToPath(home, binDir, "/bin/zsh")
	if !strings.Contains(msg, "Added") {
		t.Errorf("expected 'Added' message, got %q", msg)
	}

	content, _ := os.ReadFile(rcFile)
	if !strings.Contains(string(content), binDir) {
		t.Error("rc file should contain binDir after addToPath")
	}
	if !strings.Contains(string(content), "# surf CLI") {
		t.Error("rc file should contain '# surf CLI' comment")
	}
}

func TestAddToPathSkipsIfPresent(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	rcFile := filepath.Join(home, ".zshrc")
	os.WriteFile(rcFile, []byte("export PATH=\""+binDir+":$PATH\"\n"), 0644)

	msg := addToPath(home, binDir, "/bin/zsh")
	if msg != "" {
		t.Errorf("expected empty message when path already present, got %q", msg)
	}
}

func TestAddToPathNoShell(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")

	msg := addToPath(home, binDir, "")
	if !strings.Contains(msg, "Add this to your shell profile") {
		t.Errorf("expected fallback message, got %q", msg)
	}
}

func TestAddToPathFish(t *testing.T) {
	home := t.TempDir()
	binDir := filepath.Join(home, ".local", "bin")
	fishDir := filepath.Join(home, ".config", "fish")
	os.MkdirAll(fishDir, 0755)
	rcFile := filepath.Join(fishDir, "config.fish")
	os.WriteFile(rcFile, []byte("# fish config\n"), 0644)

	msg := addToPath(home, binDir, "/usr/bin/fish")
	if !strings.Contains(msg, "Added") {
		t.Errorf("expected 'Added' message, got %q", msg)
	}

	content, _ := os.ReadFile(rcFile)
	if !strings.Contains(string(content), "set -gx PATH") {
		t.Error("fish rc should contain 'set -gx PATH'")
	}
}

// fakeReleaseServer serves a GitHub-Releases-shaped tree over TLS:
// /releases/latest redirects to /releases/tag/<latestTag>, and
// /releases/download/<tag>/<file> serves files[tag+"/"+file]. It records the
// paths it was asked for.
type fakeReleaseServer struct {
	mu       sync.Mutex
	requests []string
}

func (f *fakeReleaseServer) downloads() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, p := range f.requests {
		if strings.HasPrefix(p, "/releases/download/") {
			out = append(out, p)
		}
	}
	return out
}

// fakeReleases points releasesURL at a fake release server, makes httpClient
// trust its certificate, and empties PATH so no real gh runs.
func fakeReleases(t *testing.T, latestTag string, files map[string]string) *fakeReleaseServer {
	t.Helper()
	fake := &fakeReleaseServer{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fake.mu.Lock()
		fake.requests = append(fake.requests, r.URL.Path)
		fake.mu.Unlock()
		switch {
		case r.URL.Path == "/releases/latest":
			if latestTag == "" {
				http.Redirect(w, r, "/releases", http.StatusFound)
				return
			}
			http.Redirect(w, r, "/releases/tag/"+latestTag, http.StatusFound)
		case strings.HasPrefix(r.URL.Path, "/releases/download/"):
			body, ok := files[strings.TrimPrefix(r.URL.Path, "/releases/download/")]
			if !ok {
				http.NotFound(w, r)
				return
			}
			io.WriteString(w, body)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	trustServer(t, srv)
	oldURL := releasesURL
	releasesURL = srv.URL + "/releases"
	t.Cleanup(func() { releasesURL = oldURL })
	t.Setenv("PATH", t.TempDir())
	return fake
}

// trustServer makes httpClient trust a TLS test server's certificate.
func trustServer(t *testing.T, srv *httptest.Server) {
	t.Helper()
	old := httpClient.Transport
	httpClient.Transport = srv.Client().Transport
	t.Cleanup(func() { httpClient.Transport = old })
}

// fakeGh puts a gh on PATH whose `auth token` and `attestation verify` exit
// with the given codes, and returns the file where it logs its arguments.
func fakeGh(t *testing.T, authExit, verifyExit int) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake gh is a shell script")
	}
	dir := t.TempDir()
	log := filepath.Join(dir, "gh.log")
	script := "#!/bin/sh\n" +
		"echo \"$*\" >> '" + log + "'\n" +
		"case \"$1 $2\" in\n" +
		"  'auth token') exit " + strconv.Itoa(authExit) + " ;;\n" +
		"  'attestation verify') exit " + strconv.Itoa(verifyExit) + " ;;\n" +
		"esac\n" +
		"exit 2\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return log
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func assertOnlyEntries(t *testing.T, dir string, want ...string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, e.Name())
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("%s holds %v, want %v (no temporary files left)", dir, got, want)
	}
}

// releaseFiles is a v1.2.3 release with a correct binary and checksums.txt.
func releaseFiles(binary string) map[string]string {
	name := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	return map[string]string{
		"v1.2.3/" + name: binary,
		"v1.2.3/checksums.txt": sha256Hex("other") + "  " + name + "-musl\n" +
			sha256Hex(binary) + "  " + name + "\n" +
			sha256Hex("install") + "  install.sh\n",
		"v1.2.3/surf_unused_amd64": "unused",
	}
}

func setVersion(t *testing.T, v string) {
	t.Helper()
	old := version
	version = v
	t.Cleanup(func() { version = old })
}

func TestDownloadReleaseFromGitHubRelease(t *testing.T) {
	fakeReleases(t, "v1.2.3", releaseFiles("surf-binary-v1.2.3"))

	path, err := downloadRelease("v1.2.3")
	if err != nil {
		t.Fatalf("downloadRelease() error: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(path))
	got, _ := os.ReadFile(path)
	if string(got) != "surf-binary-v1.2.3" {
		t.Errorf("downloaded %q, want the v1.2.3 binary", got)
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(path)
		if info.Mode().Perm()&0111 != 0 {
			t.Errorf("downloaded file mode = %v, want it not executable before install", info.Mode().Perm())
		}
	}
}

func TestDownloadReleaseRejectsChecksumMismatch(t *testing.T) {
	name := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	fakeReleases(t, "v1.2.3", map[string]string{
		"v1.2.3/" + name:       "tampered",
		"v1.2.3/checksums.txt": sha256Hex("original") + "  " + name + "\n",
	})

	path, err := downloadRelease("v1.2.3")
	if err == nil {
		os.RemoveAll(filepath.Dir(path))
		t.Fatal("expected a checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %v, want checksum mismatch", err)
	}
}

func TestDownloadReleaseRejectsOversizedChecksums(t *testing.T) {
	name := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	fakeReleases(t, "v1.2.3", map[string]string{
		"v1.2.3/" + name:       "binary",
		"v1.2.3/checksums.txt": sha256Hex("binary") + "  " + name + "\n" + strings.Repeat("x", maxChecksumsBytes),
	})

	path, err := downloadRelease("v1.2.3")
	if err == nil {
		os.RemoveAll(filepath.Dir(path))
		t.Fatal("expected an error for an oversized checksums.txt")
	}
	if !strings.Contains(err.Error(), "larger than") {
		t.Errorf("error = %v, want a size error", err)
	}
}

func TestHTTPClientRefusesRedirectToHTTP(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("followed a redirect to http: %s", r.URL)
		io.WriteString(w, "downgraded")
	}))
	defer plain.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL+"/file", http.StatusFound)
	}))
	defer srv.Close()
	trustServer(t, srv)

	if _, err := fetchURL(srv.URL+"/checksums.txt", maxChecksumsBytes); err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Errorf("fetchURL() error = %v, want a non-https redirect refusal", err)
	}
	dst := filepath.Join(t.TempDir(), "surf")
	if err := downloadToFile(srv.URL+"/surf", dst, maxBinaryBytes); err == nil || !strings.Contains(err.Error(), "non-https") {
		t.Errorf("downloadToFile() error = %v, want a non-https redirect refusal", err)
	}
}

func TestHTTPClientStopsAfterMaxRedirects(t *testing.T) {
	var mu sync.Mutex
	requests := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests++
		mu.Unlock()
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	}))
	defer srv.Close()
	trustServer(t, srv)

	if _, err := fetchURL(srv.URL+"/a", maxChecksumsBytes); err == nil || !strings.Contains(err.Error(), "redirects") {
		t.Errorf("fetchURL() error = %v, want a redirect limit error", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if requests != maxRedirects+1 {
		t.Errorf("made %d requests, want the first plus %d redirects", requests, maxRedirects)
	}
}

func TestFetchURLRefusesHTTP(t *testing.T) {
	if _, err := fetchURL("http://example.com/checksums.txt", maxChecksumsBytes); err == nil {
		t.Error("expected fetchURL to refuse an http URL")
	}
}

func TestLatestReleaseTagRejectsNonVersionTags(t *testing.T) {
	for _, tag := range []string{"..", "v1.2.3%2F..%2F..", "latest", "1.2.3", "v1.2"} {
		t.Run(tag, func(t *testing.T) {
			fakeReleases(t, tag, nil)
			if got, err := latestReleaseTag(); err == nil {
				t.Errorf("latestReleaseTag() = %q, want an error", got)
			}
		})
	}
}

func TestLatestReleaseTagWithoutRelease(t *testing.T) {
	fakeReleases(t, "", nil)
	if got, err := latestReleaseTag(); err == nil {
		t.Errorf("latestReleaseTag() = %q, want an error", got)
	}
}

func TestChecksumForMatchesExactName(t *testing.T) {
	checksums := sha256Hex("a") + "  surf_linux_amd64-musl\n" +
		sha256Hex("b") + "  surf_linux_amd64\n"
	got, err := checksumFor(checksums, "surf_linux_amd64")
	if err != nil {
		t.Fatalf("checksumFor() error: %v", err)
	}
	if got != sha256Hex("b") {
		t.Errorf("checksumFor() = %q, want the surf_linux_amd64 line", got)
	}
	if _, err := checksumFor(checksums, "surf_linux_arm64"); err == nil {
		t.Error("expected an error for a missing file")
	}
	if _, err := checksumFor("nothex  surf_linux_amd64\n", "surf_linux_amd64"); err == nil {
		t.Error("expected an error for a malformed checksum")
	}
}

func TestChecksumForRejectsDuplicateAndExtraFieldRows(t *testing.T) {
	tests := map[string]string{
		"duplicate row":         sha256Hex("b") + "  surf_linux_amd64\n" + sha256Hex("b") + "  surf_linux_amd64\n",
		"conflicting rows":      sha256Hex("b") + "  surf_linux_amd64\n" + sha256Hex("evil") + "  surf_linux_amd64\n",
		"extra field":           sha256Hex("b") + "  surf_linux_amd64 extra\n",
		"extra field elsewhere": sha256Hex("b") + "  surf_linux_amd64\n" + sha256Hex("a") + "  install.sh extra\n",
	}
	for name, checksums := range tests {
		t.Run(name, func(t *testing.T) {
			if got, err := checksumFor(checksums, "surf_linux_amd64"); err == nil {
				t.Errorf("checksumFor() = %q, want an error", got)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"v1.2.3", "1.2.3", 0},
		{"v1.2.3", "v1.2.4", -1},
		{"v1.10.0", "v1.9.9", 1},
		{"v2.0.0", "v1.99.99", 1},
		{"v1.0.0-rc.1", "v1.0.0", -1},
		{"v1.0.0-alpha", "v1.0.0-alpha.1", -1},
		{"v1.0.0-alpha.1", "v1.0.0-alpha.beta", -1},
		{"v1.0.0-beta.2", "v1.0.0-beta.11", -1},
		{"v1.0.0-rc.1", "v1.0.0-beta.11", 1},
	}
	for _, tt := range tests {
		got, ok := compareVersions(tt.a, tt.b)
		if !ok || got != tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, %v; want %d, true", tt.a, tt.b, got, ok, tt.want)
		}
		if back, _ := compareVersions(tt.b, tt.a); back != -tt.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.b, tt.a, back, -tt.want)
		}
	}
	if _, ok := compareVersions("v1.2.3", "dev"); ok {
		t.Error("compareVersions with a dev build should not compare")
	}
}

func TestReleaseAssetName(t *testing.T) {
	if got := releaseAssetName("windows", "arm64"); got != "surf_windows_arm64.exe" {
		t.Errorf("releaseAssetName(windows) = %q", got)
	}
	if got := releaseAssetName("darwin", "arm64"); got != "surf_darwin_arm64" {
		t.Errorf("releaseAssetName(darwin) = %q", got)
	}
}

func installedBinary(home, ver string) string {
	name := "surf"
	if runtime.GOOS == "windows" {
		name = "surf.exe"
	}
	return filepath.Join(versionDir(home, ver), name)
}

func TestSelfUpdateInstallsLatest(t *testing.T) {
	fakeReleases(t, "v1.2.3", releaseFiles("surf-binary-v1.2.3"))
	setVersion(t, "1.2.2")
	t.Setenv("SHELL", "")
	home := t.TempDir()

	if err := selfUpdate(home, nil); err != nil {
		t.Fatalf("selfUpdate() error: %v", err)
	}
	if got, _ := os.ReadFile(installedBinary(home, "1.2.3")); string(got) != "surf-binary-v1.2.3" {
		t.Errorf("installed %q, want the v1.2.3 binary in versions/1.2.3", got)
	}
	if _, err := os.Stat(filepath.Join(home, ".local", "share", "surf", "versions", "v1.2.3")); !os.IsNotExist(err) {
		t.Error("a versions/v1.2.3 directory exists; versions must have one spelling")
	}
}

func TestSelfUpdateRefusesDowngrade(t *testing.T) {
	fake := fakeReleases(t, "v1.2.3", releaseFiles("surf-binary-v1.2.3"))
	setVersion(t, "1.3.0")
	t.Setenv("SHELL", "")
	home := t.TempDir()

	err := selfUpdate(home, nil)
	if err == nil || !strings.Contains(err.Error(), "older than this surf") {
		t.Fatalf("selfUpdate() error = %v, want a downgrade refusal", err)
	}
	if d := fake.downloads(); len(d) != 0 {
		t.Errorf("downloaded %v during a refused downgrade", d)
	}

	// Naming the older version installs it.
	if err := selfUpdate(home, []string{"v1.2.3"}); err != nil {
		t.Fatalf("selfUpdate(v1.2.3) error: %v", err)
	}
	if got, _ := os.ReadFile(installedBinary(home, "v1.2.3")); string(got) != "surf-binary-v1.2.3" {
		t.Errorf("installed %q, want the named v1.2.3 binary", got)
	}
}

func TestSelfUpdateAlreadyUpToDate(t *testing.T) {
	fake := fakeReleases(t, "v1.2.3", releaseFiles("surf-binary-v1.2.3"))
	setVersion(t, "1.2.3")
	home := t.TempDir()

	if err := selfUpdate(home, nil); err != nil {
		t.Fatalf("selfUpdate() error: %v", err)
	}
	if d := fake.downloads(); len(d) != 0 {
		t.Errorf("downloaded %v although surf is up to date", d)
	}
	if _, err := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(err) {
		t.Error("selfUpdate touched the install directory although surf is up to date")
	}
}

func TestSelfUpdateRejectsInvalidVersion(t *testing.T) {
	fake := fakeReleases(t, "v1.2.3", nil)
	for _, v := range []string{"1.2.3", "v1.2.3/../x", "v1.2.3\n", "latest"} {
		if err := selfUpdate(t.TempDir(), []string{v}); err == nil {
			t.Errorf("selfUpdate(%q) succeeded, want an error", v)
		}
	}
	if len(fake.requests) != 0 {
		t.Errorf("made requests %v for invalid versions", fake.requests)
	}
}

func TestSelfUpdateVerifiesAttestationWithSignedInGh(t *testing.T) {
	fakeReleases(t, "v1.2.3", releaseFiles("surf-binary-v1.2.3"))
	log := fakeGh(t, 0, 0)
	setVersion(t, "1.2.2")
	t.Setenv("SHELL", "")
	home := t.TempDir()

	if err := selfUpdate(home, nil); err != nil {
		t.Fatalf("selfUpdate() error: %v", err)
	}
	calls, _ := os.ReadFile(log)
	want := "--repo asksurf-ai/surf-cli --signer-workflow asksurf-ai/surf-cli/.github/workflows/release.yml --source-ref refs/tags/v1.2.3 --deny-self-hosted-runners"
	if !strings.Contains(string(calls), "auth token --hostname github.com") || !strings.Contains(string(calls), want) {
		t.Errorf("gh calls:\n%s\nwant auth token and attestation verify with %q", calls, want)
	}
}

func TestSelfUpdateFailsClosedWhenAttestationFails(t *testing.T) {
	fakeReleases(t, "v1.2.3", releaseFiles("surf-binary-v1.2.3"))
	fakeGh(t, 0, 1)
	setVersion(t, "1.2.2")
	home := t.TempDir()

	err := selfUpdate(home, nil)
	if err == nil || !strings.Contains(err.Error(), "attestation verification failed") {
		t.Fatalf("selfUpdate() error = %v, want an attestation failure", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(err) {
		t.Error("selfUpdate installed something after a failed attestation")
	}
}

func TestSelfUpdateSkipsAttestationWithoutSignedInGh(t *testing.T) {
	fakeReleases(t, "v1.2.3", releaseFiles("surf-binary-v1.2.3"))
	log := fakeGh(t, 1, 1)
	setVersion(t, "1.2.2")
	t.Setenv("SHELL", "")
	home := t.TempDir()

	if err := selfUpdate(home, nil); err != nil {
		t.Fatalf("selfUpdate() error: %v", err)
	}
	if calls, _ := os.ReadFile(log); strings.Contains(string(calls), "attestation") {
		t.Errorf("ran gh attestation verify without a signed-in gh:\n%s", calls)
	}
}
