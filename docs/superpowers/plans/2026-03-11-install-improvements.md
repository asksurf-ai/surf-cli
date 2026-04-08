# Install Improvements Implementation Plan

> **For agentic workers:** REQUIRED: Use superpowers:subagent-driven-development (if subagents available) or superpowers:executing-plans to implement this plan. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Adopt Claude Code's install pattern — self-installing binary, bare binary distribution, versioned installs with symlinks, XDG paths, Rosetta 2 and musl detection.

**Architecture:** New `surf install` Go subcommand handles binary placement (`~/.local/share/surf/versions/<ver>/surf`) and symlink creation (`~/.local/bin/surf`). GoReleaser drops archives for bare binaries. install.sh becomes a thin bootstrap that downloads, verifies, and delegates to `surf install`.

**Tech Stack:** Go (cobra), GoReleaser, shell script (POSIX sh)

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `cmd/surf/install.go` | Create | `surf install` subcommand — copy self to versioned dir, symlink, PATH setup |
| `cmd/surf/install_test.go` | Create | Tests for install logic (path resolution, symlink, shell detection) |
| `cmd/surf/main.go` | Modify | Register install command, add to local commands map |
| `.goreleaser.yaml` | Modify | Remove archives section for bare binary output |
| `install.sh` | Rewrite | Thin bootstrap with Rosetta 2 + musl detection |

---

## Chunk 1: `surf install` subcommand

### Task 1: Write install command tests

**Files:**
- Create: `cmd/surf/install_test.go`

- [ ] **Step 1: Write tests for install helper functions**

```go
package main

import (
	"os"
	"path/filepath"
	"runtime"
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
	info, _ := os.Stat(dst)
	if info.Mode().Perm()&0111 == 0 {
		t.Error("copied file is not executable")
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ryanli/Code/agent/surf-cli && go test ./cmd/surf/ -run 'TestInstallDir|TestVersionDir|TestCopyBinary|TestCreateSymlink|TestDetectShellRC' -v`
Expected: FAIL — functions not defined

- [ ] **Step 3: Commit test file**

```bash
git add cmd/surf/install_test.go
git commit -m "test: add install subcommand tests"
```

### Task 2: Implement install command

**Files:**
- Create: `cmd/surf/install.go`

- [ ] **Step 4: Write the install command implementation**

```go
package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

func installDir(home string) string {
	return filepath.Join(home, ".local", "bin")
}

func versionDir(home, ver string) string {
	return filepath.Join(home, ".local", "share", "surf", "versions", ver)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func createSymlink(target, link string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
		return err
	}
	// Remove existing symlink or file.
	os.Remove(link)
	return os.Symlink(target, link)
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

func addToPath(home, binDir string) string {
	shell := os.Getenv("SHELL")
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

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "install",
		Short:  "Install surf to ~/.local/bin",
		Long:   "Copies the surf binary to a versioned directory and creates a symlink at ~/.local/bin/surf.",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}

			exe, err := os.Executable()
			if err != nil {
				return fmt.Errorf("cannot determine executable path: %w", err)
			}

			binDir := installDir(home)
			verDir := versionDir(home, version)
			binaryName := "surf"
			if runtime.GOOS == "windows" {
				binaryName = "surf.exe"
			}

			dst := filepath.Join(verDir, binaryName)
			if err := copyFile(exe, dst); err != nil {
				return fmt.Errorf("failed to copy binary: %w", err)
			}

			linkPath := filepath.Join(binDir, binaryName)
			if runtime.GOOS == "windows" {
				// Windows: copy directly instead of symlink.
				if err := copyFile(dst, linkPath); err != nil {
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
				pathMsg = addToPath(home, binDir)
			}

			// Print success.
			fmt.Println()
			fmt.Printf("  surf successfully installed!\n\n")
			fmt.Printf("  Version:  %s\n", version)
			fmt.Printf("  Location: %s\n", linkPath)
			fmt.Println()
			if pathMsg != "" {
				fmt.Printf("  %s\n\n", pathMsg)
			}
			fmt.Println("  Next: Run surf --help to get started")
			fmt.Println()

			return nil
		},
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/ryanli/Code/agent/surf-cli && go test ./cmd/surf/ -run 'TestInstallDir|TestVersionDir|TestCopyBinary|TestCreateSymlink|TestDetectShellRC' -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add cmd/surf/install.go
git commit -m "feat: add surf install subcommand"
```

### Task 3: Register install command in main.go

**Files:**
- Modify: `cmd/surf/main.go:90-94` (add command registration)
- Modify: `cmd/surf/main.go:106-108` (add to local commands map)

- [ ] **Step 7: Add install command registration**

In `cmd/surf/main.go`, after line 94 (`cli.Root.AddCommand(newVersionCmd())`), add:
```go
	cli.Root.AddCommand(newInstallCmd())
```

- [ ] **Step 8: Add "install" to shouldInjectAPIName local map**

In `cmd/surf/main.go`, change the local map at line 106-108 from:
```go
	local := map[string]bool{
		"login": true, "logout": true, "refresh": true, "sync": true,
		"help": true, "completion": true, "version": true,
	}
```
to:
```go
	local := map[string]bool{
		"login": true, "logout": true, "refresh": true, "sync": true,
		"help": true, "completion": true, "version": true, "install": true,
	}
```

- [ ] **Step 9: Verify build and run**

Run: `cd /Users/ryanli/Code/agent/surf-cli && go build ./cmd/surf/ && go test ./cmd/surf/ -v`
Expected: Build succeeds, all tests pass

- [ ] **Step 10: Commit**

```bash
git add cmd/surf/main.go
git commit -m "feat: register install command and add to local commands"
```

---

## Chunk 2: GoReleaser bare binaries

### Task 4: Switch GoReleaser to bare binary output

**Files:**
- Modify: `.goreleaser.yaml:23-30` (replace archives section)

- [ ] **Step 11: Replace archives section with bare binary format**

Replace the entire `archives:` block (lines 23-30) in `.goreleaser.yaml`:

```yaml
archives:
  - formats:
      - tar.gz
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"
    format_overrides:
      - goos: windows
        formats:
          - zip
```

with:

```yaml
archives:
  - format: binary
    name_template: "{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}"
```

This tells GoReleaser to skip archiving and upload raw binaries. The `name_template` produces filenames like `surf_darwin_arm64`. On Windows, GoReleaser automatically appends `.exe`.

- [ ] **Step 12: Verify GoReleaser config**

Run: `cd /Users/ryanli/Code/agent/surf-cli && goreleaser check`
Expected: config is valid

- [ ] **Step 13: Commit**

```bash
git add .goreleaser.yaml
git commit -m "chore: switch goreleaser to bare binary output"
```

---

## Chunk 3: install.sh rewrite

### Task 5: Rewrite install.sh as thin bootstrap

**Files:**
- Rewrite: `install.sh`

- [ ] **Step 14: Write the new install.sh**

```sh
#!/bin/sh
set -e

CDN_BASE="https://downloads.asksurf.ai/cli/releases"

# Require curl or wget.
if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL "$1"; }
  download() { curl -fSL -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -qO- "$1"; }
  download() { wget -q -O "$2" "$1"; }
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
download "${CDN_BASE}/${VERSION}/checksums.txt" "${TMPDIR}/checksums.txt"

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
"${TMPDIR}/${FILENAME}" install
```

- [ ] **Step 15: Verify install.sh syntax**

Run: `sh -n /Users/ryanli/Code/agent/surf-cli/install.sh`
Expected: No syntax errors

- [ ] **Step 16: Commit**

```bash
git add install.sh
git commit -m "feat: rewrite install.sh as thin bootstrap with Rosetta 2 and musl detection"
```

---

## Chunk 4: Final verification

### Task 6: End-to-end local test

- [ ] **Step 17: Build and test `surf install` locally**

```bash
cd /Users/ryanli/Code/agent/surf-cli
go build -o /tmp/surf-test -ldflags "-X main.version=v0.0.0-test" ./cmd/surf/
/tmp/surf-test install
```

Expected: Binary installed to `~/.local/share/surf/versions/v0.0.0-test/surf`, symlink at `~/.local/bin/surf`, success message printed.

- [ ] **Step 18: Verify symlink**

```bash
ls -la ~/.local/bin/surf
~/.local/bin/surf version
```

Expected: Symlink points to `~/.local/share/surf/versions/v0.0.0-test/surf`, prints `surf v0.0.0-test`.

- [ ] **Step 19: Run all tests**

```bash
cd /Users/ryanli/Code/agent/surf-cli && go test ./cmd/surf/ -v
```

Expected: All tests pass.

- [ ] **Step 20: Clean up test install**

```bash
rm -rf ~/.local/share/surf/versions/v0.0.0-test
rm ~/.local/bin/surf
```
