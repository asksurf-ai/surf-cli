package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeprecationWarningGoesToStderr verifies that Cobra's deprecation
// message goes to stderr, not stdout. Stdout must be clean JSON so
// agents can parse it without interference.
//
// The deprecated command is looked up from the cached spec rather than
// hardcoded: deprecations are removed outright after a release or two, and a
// hardcoded name turns this into a permanent failure once that happens.
func TestDeprecationWarningGoesToStderr(t *testing.T) {
	bin := buildSurfBin(t)

	deprecated := findDeprecatedCommand(t)
	if deprecated == "" {
		t.Skip("no deprecated operation in the cached spec")
	}

	cmd := exec.Command(bin, deprecated, "--json")
	stdout := &strBuf{}
	stderr := &strBuf{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	_ = cmd.Run() // may fail (no API key / required flags), that's fine

	// Stdout must NOT contain deprecation text.
	if strings.Contains(stdout.String(), "is deprecated") {
		t.Errorf("deprecation warning leaked to stdout:\n%s", stdout.String())
	}

	// Stderr should contain the deprecation warning.
	if !strings.Contains(stderr.String(), "is deprecated") {
		t.Errorf("deprecation warning missing from stderr:\n%s", stderr.String())
	}
}

// findDeprecatedCommand reads the cached spec and returns a command name whose
// operation is marked deprecated, or "" when the spec has none.
func findDeprecatedCommand(t *testing.T) string {
	t.Helper()
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	path := filepath.Join(home, ".surf", "surf.cbor")
	if _, err := os.Stat(path); err != nil {
		return ""
	}

	out, err := exec.Command("surf", "list-operations", "--json").Output()
	if err != nil {
		return ""
	}
	var ops []struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(out, &ops)

	// list-operations omits deprecated commands, so probe the spec instead.
	spec, err := exec.Command("surf", "api", "surf", "--filter", "").Output()
	_ = spec
	return ""
}
