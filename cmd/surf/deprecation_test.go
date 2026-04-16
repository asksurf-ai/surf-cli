package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestDeprecationWarningGoesToStderr verifies that Cobra's deprecation
// message goes to stderr, not stdout. Stdout must be clean JSON so
// agents can parse it without interference.
func TestDeprecationWarningGoesToStderr(t *testing.T) {
	bin := buildSurfBin(t)

	// Find a deprecated command to test with.
	// list-operations excludes deprecated commands, but they still work
	// when called directly.
	cmd := exec.Command(bin, "search-polymarket", "--q", "test", "--json")
	stdout := &strBuf{}
	stderr := &strBuf{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	_ = cmd.Run() // may fail (no API key), that's fine

	// Stdout must NOT contain deprecation text.
	if strings.Contains(stdout.String(), "is deprecated") {
		t.Errorf("deprecation warning leaked to stdout:\n%s", stdout.String())
	}

	// Stderr should contain the deprecation warning.
	if !strings.Contains(stderr.String(), "is deprecated") {
		t.Errorf("deprecation warning missing from stderr:\n%s", stderr.String())
	}
}
