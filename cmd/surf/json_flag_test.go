package main

import (
	"os/exec"
	"strings"
	"testing"
)

// TestJSONFlagAlias verifies that `--json` works as an alias for `-o json`
// on all command types: root-level, operation commands, and catalog
// subcommands (which have a local --json flag for backward compatibility).
//
// See docs/CLI_DESIGN_PRINCIPLES.md §2.2 and §3.2, and
// docs/CLI_V1_ALPHA_26_REVIEW.md item #1 for the rationale.
func TestJSONFlagAlias(t *testing.T) {
	bin := buildSurfBin(t)

	// --help must list --json as a flag
	t.Run("root help lists --json", func(t *testing.T) {
		out, err := exec.Command(bin, "--help").CombinedOutput()
		if err != nil {
			t.Fatalf("--help failed: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "--json") {
			t.Errorf("root --help does not list --json:\n%s", out)
		}
	})

	t.Run("operation help lists --json", func(t *testing.T) {
		out, err := exec.Command(bin, "market-price", "--help").CombinedOutput()
		if err != nil {
			t.Fatalf("market-price --help failed: %v\n%s", err, out)
		}
		if !strings.Contains(string(out), "--json") {
			t.Errorf("market-price --help does not list --json:\n%s", out)
		}
	})

	// --json on an operation command must not fail with "unknown flag"
	// (we can't check full success without an API key, but we can verify
	// the flag is recognized — i.e. the error is NOT "unknown flag").
	t.Run("operation --json recognized", func(t *testing.T) {
		out, _ := exec.Command(bin, "market-price", "--json", "--symbol", "BTC", "--time-range", "1d").CombinedOutput()
		if strings.Contains(string(out), "unknown flag: --json") {
			t.Errorf("operation --json should be recognized, got: %s", out)
		}
	})

	// Catalog subcommand's local --json should still work (it shadows the
	// persistent --json on Root but produces the same "JSON output" intent).
	t.Run("catalog --json recognized", func(t *testing.T) {
		out, err := exec.Command(bin, "catalog", "list", "--json").CombinedOutput()
		if err != nil {
			t.Fatalf("catalog list --json failed: %v\n%s", err, out)
		}
		// Output should start with `[` (JSON array), not "N tables"
		s := strings.TrimSpace(string(out))
		if !strings.HasPrefix(s, "[") {
			t.Errorf("catalog list --json did not produce JSON array, got:\n%s", s)
		}
	})

	// --json as a root-level flag (before the subcommand) should also work
	t.Run("root --json before operation", func(t *testing.T) {
		out, _ := exec.Command(bin, "--json", "market-price", "--symbol", "BTC", "--time-range", "1d").CombinedOutput()
		if strings.Contains(string(out), "unknown flag: --json") {
			t.Errorf("root-level --json should be recognized, got: %s", out)
		}
	})
}
