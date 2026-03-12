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
			fmt.Printf("  Version: %s%s%s\n", green, version, reset)
			fmt.Printf("  Location: %s\n", displayPath)
			fmt.Println()
			if pathMsg != "" {
				fmt.Printf("  %s\n", pathMsg)
				fmt.Println()
			}
			fmt.Printf("  Next: Run %ssurf --help%s to get started\n", bold, reset)
			fmt.Println()

			return nil
		},
	}
}
