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
	"strings"
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

func TestVersionDir(t *testing.T) {
	home := t.TempDir()
	got := versionDir(home, "v0.2.0")
	want := filepath.Join(home, ".local", "share", "surf", "versions", "v0.2.0")
	if got != want {
		t.Errorf("versionDir() = %q, want %q", got, want)
	}
}

func TestCopyBinary(t *testing.T) {
	src := filepath.Join(t.TempDir(), "surf")
	dst := filepath.Join(t.TempDir(), "surf-copy")
	os.WriteFile(src, []byte("binary-content"), 0755)

	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile() error: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "binary-content" {
		t.Errorf("copied content = %q, want %q", got, "binary-content")
	}
	if runtime.GOOS != "windows" {
		info, _ := os.Stat(dst)
		if info.Mode().Perm()&0111 == 0 {
			t.Error("copied file is not executable")
		}
	}
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

// fakeReleases serves a GitHub-Releases-shaped tree: /releases/latest
// redirects to /releases/tag/<latestTag>, and /releases/download/<tag>/<file>
// serves files[tag+"/"+file].
func fakeReleases(t *testing.T, latestTag string, files map[string]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	old := releasesURL
	releasesURL = srv.URL + "/releases"
	t.Cleanup(func() { releasesURL = old })
}

func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestDownloadLatestBinaryFromGitHubRelease(t *testing.T) {
	name := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	binary := "surf-binary-v1.2.3"
	checksums := sha256Hex("other") + "  " + name + "-musl\n" +
		sha256Hex(binary) + "  " + name + "\n" +
		sha256Hex("install") + "  install.sh\n"
	fakeReleases(t, "v1.2.3", map[string]string{
		"v1.2.3/" + name:           binary,
		"v1.2.3/checksums.txt":     checksums,
		"v1.2.2/" + name:           "old",
		"v1.2.2/checksums.txt":     sha256Hex("old") + "  " + name + "\n",
		"v1.2.3/surf_unused_amd64": "unused",
	})

	path, ver, err := downloadLatestBinary()
	if err != nil {
		t.Fatalf("downloadLatestBinary() error: %v", err)
	}
	defer os.RemoveAll(filepath.Dir(path))
	if ver != "v1.2.3" {
		t.Errorf("version = %q, want v1.2.3", ver)
	}
	got, _ := os.ReadFile(path)
	if string(got) != binary {
		t.Errorf("downloaded %q, want %q", got, binary)
	}
}

func TestDownloadLatestBinaryRejectsChecksumMismatch(t *testing.T) {
	name := releaseAssetName(runtime.GOOS, runtime.GOARCH)
	fakeReleases(t, "v1.2.3", map[string]string{
		"v1.2.3/" + name:       "tampered",
		"v1.2.3/checksums.txt": sha256Hex("original") + "  " + name + "\n",
	})

	path, _, err := downloadLatestBinary()
	if err == nil {
		os.RemoveAll(filepath.Dir(path))
		t.Fatal("expected a checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Errorf("error = %v, want checksum mismatch", err)
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

func TestReleaseAssetName(t *testing.T) {
	if got := releaseAssetName("windows", "arm64"); got != "surf_windows_arm64.exe" {
		t.Errorf("releaseAssetName(windows) = %q", got)
	}
	if got := releaseAssetName("darwin", "arm64"); got != "surf_darwin_arm64" {
		t.Errorf("releaseAssetName(darwin) = %q", got)
	}
}
