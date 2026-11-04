package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const errorOutputSpec = `{
  "openapi": "3.0.0",
  "info": {"title": "Surf Test API", "version": "1.0.0"},
  "paths": {
    "/gateway/v1/error-fixture/{job_id}": {
      "get": {
        "operationId": "error-fixture",
        "summary": "Error output fixture",
        "tags": ["test"],
        "parameters": [
          {"name": "job_id", "in": "path", "required": true, "schema": {"type": "string"}},
          {"name": "address", "in": "query", "schema": {"type": "string"}}
        ],
        "responses": {"200": {"description": "ok"}}
      }
    }
  }
}`

// TestOperationErrorsShowUsageOnce pins the error contract for operation
// commands: one error line, no `ERROR: Error:` prefix, and Cobra's Usage with
// the valid flags/positional args still visible. Regressing either half costs
// an agent a discovery round-trip.
func TestOperationErrorsShowUsageOnce(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()
	specPath := filepath.Join(t.TempDir(), "openapi.json")
	if err := os.WriteFile(specPath, []byte(errorOutputSpec), 0o600); err != nil {
		t.Fatal(err)
	}
	seedSurfSpecCache(t, bin, home, "http://stub.invalid/gateway", specPath)

	run := func(t *testing.T, args ...string) (string, string) {
		t.Helper()
		cmd := surfCmd(t, bin, home, nil, args...)
		stdout, stderr := splitOutput(cmd)
		if err := cmd.Run(); err == nil {
			t.Fatalf("surf %s unexpectedly succeeded", strings.Join(args, " "))
		}
		if got := cmd.ProcessState.ExitCode(); got != 1 {
			t.Errorf("exit code = %d, want 1", got)
		}
		return stdout.String(), stderr.String()
	}

	t.Run("unknown flag", func(t *testing.T) {
		stdout, stderr := run(t, "error-fixture", "job-1", "--query", "x")

		if stdout != "" {
			t.Errorf("diagnostics must not go to stdout:\n%s", stdout)
		}
		if got := strings.Count(stderr, "ERROR:"); got != 1 {
			t.Errorf("error should be reported once with the ERROR: prefix, got %d:\n%s", got, stderr)
		}
		if !strings.Contains(stderr, "ERROR: unknown flag: --query") {
			t.Errorf("missing prefixed error line:\n%s", stderr)
		}
		if strings.Contains(stderr, "ERROR: Error:") {
			t.Errorf("error should not be double-prefixed:\n%s", stderr)
		}
		if !strings.Contains(stderr, "Usage:") {
			t.Errorf("unknown flag should show Usage:\n%s", stderr)
		}
		if !strings.Contains(stderr, "--address") {
			t.Errorf("Usage should list the valid flags:\n%s", stderr)
		}
	})

	t.Run("missing positional argument", func(t *testing.T) {
		_, stderr := run(t, "error-fixture")

		if strings.Count(stderr, "ERROR:") != 1 || !strings.Contains(stderr, "ERROR: accepts 1 arg(s)") {
			t.Errorf("argument error should be reported once with the ERROR: prefix:\n%s", stderr)
		}
		if !strings.Contains(stderr, "Usage:") || !strings.Contains(stderr, "error-fixture job_id") {
			t.Errorf("argument error should show the expected usage line:\n%s", stderr)
		}
	})

	// Cobra prints its own "Error: ..." for local commands too; the fix is
	// root-level, so cover a command that is not an API operation.
	t.Run("local command", func(t *testing.T) {
		_, stderr := run(t, "list-operations", "--bogus")

		if strings.Count(stderr, "ERROR:") != 1 || !strings.Contains(stderr, "ERROR: unknown flag: --bogus") {
			t.Errorf("local command error should be reported once with the ERROR: prefix:\n%s", stderr)
		}
		if strings.Contains(stderr, "Error: unknown flag") {
			t.Errorf("Cobra should not print its own unprefixed error:\n%s", stderr)
		}
	})
}
