package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/spf13/cobra"
)

const cdnBase = "https://agent.asksurf.ai/cli/releases"

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
	if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cannot remove existing file at %s: %w", link, err)
	}
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

// fetchURL downloads a URL and returns the body as a string.
func fetchURL(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

// downloadToFile downloads a URL to a local file path.
func downloadToFile(url, dst string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, resp.Body); err != nil {
		return err
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

// downloadLatestBinary downloads the latest surf binary from CDN to a temp file.
// Returns the path to the downloaded binary and the version string.
func downloadLatestBinary() (path string, ver string, err error) {
	// Get latest version.
	ver, err = fetchURL(cdnBase + "/latest")
	if err != nil {
		return "", "", fmt.Errorf("could not determine latest version: %w", err)
	}
	ver = strings.TrimSpace(ver)

	// Build platform filename.
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	if goarch == "amd64" {
		goarch = "amd64"
	}
	filename := fmt.Sprintf("surf_%s_%s", goos, goarch)
	if goos == "windows" {
		filename += ".exe"
	}

	fmt.Printf("Downloading surf %s for %s/%s...\n", ver, goos, goarch)

	// Download binary to temp dir.
	tmpDir, err := os.MkdirTemp("", "surf-install-*")
	if err != nil {
		return "", "", err
	}

	binaryPath := filepath.Join(tmpDir, filename)
	if err := downloadToFile(cdnBase+"/"+ver+"/"+filename, binaryPath); err != nil {
		os.RemoveAll(tmpDir)
		return "", "", fmt.Errorf("download failed: %w", err)
	}

	// Download and verify checksum.
	checksums, err := fetchURL(cdnBase + "/" + ver + "/checksums.txt")
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", "", fmt.Errorf("could not download checksums: %w", err)
	}

	var expected string
	for _, line := range strings.Split(checksums, "\n") {
		if strings.Contains(line, filename) {
			parts := strings.Fields(line)
			if len(parts) >= 1 {
				expected = parts[0]
			}
			break
		}
	}
	if expected == "" {
		os.RemoveAll(tmpDir)
		return "", "", fmt.Errorf("checksum not found for %s", filename)
	}

	actual, err := sha256File(binaryPath)
	if err != nil {
		os.RemoveAll(tmpDir)
		return "", "", fmt.Errorf("could not hash downloaded binary: %w", err)
	}

	if actual != expected {
		os.RemoveAll(tmpDir)
		return "", "", fmt.Errorf("checksum mismatch: expected %s, got %s", expected, actual)
	}

	return binaryPath, ver, nil
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
	if err := copyFile(srcPath, dst); err != nil {
		return fmt.Errorf("failed to copy binary: %w", err)
	}

	linkPath := filepath.Join(binDir, binaryName)
	if runtime.GOOS == "windows" {
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
	fmt.Printf("%s\u2714 surf successfully installed!%s\n", green, reset)
	fmt.Println()
	fmt.Printf("  Version: %s%s%s\n", green, ver, reset)
	fmt.Printf("  Location: %s\n", displayPath)
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
		Use:    "install",
		Short:  "Install or update surf",
		Long:   "Downloads the latest surf binary and installs it to ~/.local/bin/surf. Use --local to install the current binary without downloading.",
		Args:   cobra.NoArgs,
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("cannot determine home directory: %w", err)
			}

			if local {
				// Bootstrap mode: install the current binary.
				exe, err := os.Executable()
				if err != nil {
					return fmt.Errorf("cannot determine executable path: %w", err)
				}
				return installBinary(home, exe, version)
			}

			// Self-update mode: download latest from CDN.
			binaryPath, ver, err := downloadLatestBinary()
			if err != nil {
				return err
			}
			defer os.RemoveAll(filepath.Dir(binaryPath))

			return installBinary(home, binaryPath, ver)
		},
	}

	cmd.Flags().BoolVar(&local, "local", false, "Install the current binary without downloading")

	return cmd
}
