package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// TestNoDoubleSurfInUsage builds the surf binary and verifies that
// "surf surf" never appears in help or error output.
func TestNoDoubleSurfInUsage(t *testing.T) {
	// Build the binary into a temp dir.
	bin := t.TempDir() + "/surf"
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	// Ensure a cached API spec exists so operation commands are registered.
	// If no cache, commands won't appear — skip gracefully.
	home, _ := os.UserHomeDir()
	if _, err := os.Stat(home + "/.surf/surf.cbor"); os.IsNotExist(err) {
		t.Skip("no cached API spec — run `surf sync` first")
	}

	tests := []struct {
		name string
		args []string
	}{
		{"root help", []string{"--help"}},
		{"operation help", []string{"market-price", "--help"}},
		{"flag error", []string{"market-price", "--bogus"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(bin, tt.args...)
			out, _ := cmd.CombinedOutput() // ignore exit code, errors are expected
			output := string(out)

			if strings.Contains(output, "surf surf") {
				t.Errorf("found 'surf surf' in output:\n%s", output)
			}
		})
	}
}
