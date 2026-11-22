package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Tests of install.sh, at the repository root.

const scriptReleasesURL = "https://github.com/asksurf-ai/surf-cli/releases"

// runInstallScript runs install.sh with args and HOME set to home, with a fake
// curl ahead of the system tools on PATH. The fake curl answers the
// /releases/latest redirect with latestTag's release page and fails every
// download, so a run that gets past the script's checks stops at its first
// download, before anything is installed or run. It returns the script's
// output and the URLs curl was asked for.
func runInstallScript(t *testing.T, home, latestTag string, args ...string) (string, []string, error) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is a POSIX shell script")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	log := filepath.Join(bin, "curl.log")
	curl := "#!/bin/sh\n" +
		"for a in \"$@\"; do url=$a; done\n" +
		"echo \"$url\" >> \"$FAKE_CURL_LOG\"\n" +
		"case \" $* \" in\n" +
		"  *' -w '*) printf '%s' \"$FAKE_LATEST_URL\" ;;\n" +
		"  *) echo 'curl: (22) The requested URL returned error: 404' >&2; exit 22 ;;\n" +
		"esac\n"
	if err := os.WriteFile(filepath.Join(bin, "curl"), []byte(curl), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", append([]string{script}, args...)...)
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FAKE_CURL_LOG="+log,
		"FAKE_LATEST_URL="+scriptReleasesURL+"/tag/"+latestTag,
	)
	out, runErr := cmd.CombinedOutput()
	calls, _ := os.ReadFile(log)
	return string(out), strings.Fields(string(calls)), runErr
}

// downloadedVersion reports whether curl was asked for the surf binary of a
// release.
func downloadedVersion(urls []string, tag string) bool {
	for _, u := range urls {
		if strings.HasPrefix(u, scriptReleasesURL+"/download/"+tag+"/surf_") {
			return true
		}
	}
	return false
}

// A named version below v1.0.12 is refused before any request.
func TestInstallScriptRefusesNamedVersionBelowFloor(t *testing.T) {
	for _, v := range []string{"v1.0.11", "v1.0.9", "v1.0.0", "v1.0.12-rc.1", "v0.99.99"} {
		t.Run(v, func(t *testing.T) {
			out, urls, err := runInstallScript(t, t.TempDir(), "v1.2.3", v)
			if err == nil || !strings.Contains(out, "Error: refusing surf "+v+": this script installs only v1.0.12 or newer") {
				t.Errorf("install.sh %s: err = %v, output:\n%s\nwant a refusal below v1.0.12", v, err, out)
			}
			if len(urls) != 0 {
				t.Errorf("curl was asked for %v, want no request", urls)
			}
		})
	}
}

// A latest release below v1.0.12 is refused before any download.
func TestInstallScriptRefusesLatestBelowFloor(t *testing.T) {
	out, urls, err := runInstallScript(t, t.TempDir(), "v1.0.11")
	if err == nil || !strings.Contains(out, "Error: refusing the latest release v1.0.11: this script installs only v1.0.12 or newer") {
		t.Errorf("install.sh: err = %v, output:\n%s\nwant a refusal below v1.0.12", err, out)
	}
	if want := scriptReleasesURL + "/latest"; len(urls) != 1 || urls[0] != want {
		t.Errorf("curl was asked for %v, want only %s", urls, want)
	}
}

// A version comparison that fails is refused, not let through: here awk, which
// does the comparison, is broken.
func TestInstallScriptFloorFailsClosed(t *testing.T) {
	broken := t.TempDir()
	if err := os.WriteFile(filepath.Join(broken, "awk"), []byte("#!/bin/sh\nexit 2\n"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", broken+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, urls, err := runInstallScript(t, t.TempDir(), "v1.2.3", "v1.2.3")
	if err == nil || !strings.Contains(out, "Error: refusing surf v1.2.3: this script installs only v1.0.12 or newer") {
		t.Errorf("install.sh v1.2.3 with a broken awk: err = %v, output:\n%s\nwant a refusal", err, out)
	}
	if len(urls) != 0 {
		t.Errorf("curl was asked for %v, want no request", urls)
	}
}

// v1.0.12 and newer get past the floor to the download.
func TestInstallScriptDownloadsFromFloor(t *testing.T) {
	tests := map[string]struct {
		latest, version string
		args            []string
	}{
		"named v1.0.12":      {"v1.2.3", "v1.0.12", []string{"v1.0.12"}},
		"named v1.0.13-rc.1": {"v1.2.3", "v1.0.13-rc.1", []string{"v1.0.13-rc.1"}},
		"latest v1.0.12":     {"v1.0.12", "v1.0.12", nil},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			out, urls, _ := runInstallScript(t, t.TempDir(), tt.latest, tt.args...)
			if strings.Contains(out, "refusing") || !downloadedVersion(urls, tt.version) {
				t.Errorf("curl was asked for %v, output:\n%s\nwant a download of %s", urls, out, tt.version)
			}
		})
	}
}

// Without a version, install.sh refuses a latest release older than the surf
// that ~/.local/bin/surf links to. It reads that version from the whole link
// target: a target with a newline is an unknown version, not the version on
// its first line.
func TestInstallScriptRefusesLatestOlderThanInstalled(t *testing.T) {
	tests := map[string]struct {
		installed, latest string
		refused, note     bool
	}{
		"newer":                {installed: "1.0.13", refused: true},
		"newer with v":         {installed: "v1.0.13", refused: true},
		"older":                {installed: "1.0.11"},
		"same":                 {installed: "1.0.12"},
		"stable vs its rc":     {installed: "1.0.13", latest: "v1.0.13-rc.1", refused: true},
		"rc vs its stable":     {installed: "1.0.13-rc.1", latest: "v1.0.13"},
		"newer core vs rc":     {installed: "1.0.13-rc.1", latest: "v1.0.12", refused: true},
		"two lines":            {installed: "9.9.9\nevil", note: true},
		"trailing line":        {installed: "1.0.13\n", note: true},
		"not a version":        {installed: "latest", note: true},
		"version and subdir":   {installed: "1.0.13/x", note: true},
		"version in a subdir":  {installed: "x/1.0.13", note: true},
		"prefix before number": {installed: "x1.0.13", note: true},
	}
	for name, tt := range tests {
		if tt.latest == "" {
			tt.latest = "v1.0.12"
		}
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			target := filepath.Join(home, ".local", "share", "surf", "versions", tt.installed, "surf")
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, []byte("#!/bin/sh\nexit 99\n"), 0755); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(home, ".local", "bin", "surf")
			if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, link); err != nil {
				t.Fatal(err)
			}

			out, urls, _ := runInstallScript(t, home, tt.latest)
			refused := strings.Contains(out, "Error: the latest release "+tt.latest+" is older than the installed surf")
			note := strings.Contains(out, "Note: the version of")
			if refused != tt.refused || note != tt.note {
				t.Errorf("installed %q, latest %s: refused = %v, note = %v; want %v, %v. Output:\n%s", tt.installed, tt.latest, refused, note, tt.refused, tt.note, out)
			}
			if downloaded := downloadedVersion(urls, tt.latest); downloaded == tt.refused {
				t.Errorf("installed %q: download of %s = %v, want %v", tt.installed, tt.latest, downloaded, !tt.refused)
			}
		})
	}
}
