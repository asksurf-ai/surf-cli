package main

import (
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
