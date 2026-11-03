package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNetworkErrorEnvelope verifies that transport-layer failures
// (connection refused, DNS failure, local timeout) produce a structured
// error envelope on stdout + exit 4, rather than exit 1 + stderr text.
//
// See docs/CLI_DESIGN_PRINCIPLES.md §4.1 and §4.2.1, and
// docs/CLI_V1_ALPHA_26_REVIEW.md items #3 and #4 for the rationale.
func TestNetworkErrorEnvelope(t *testing.T) {
	bin := buildBareSurfBin(t)

	type envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	t.Run("connection refused", func(t *testing.T) {
		home := t.TempDir()
		srv, _ := newSpecAndOperationServerWithOpenAPIServer(t, "marketPrice", "http://127.0.0.1:1", nil)
		defer srv.Close()
		seedSurfSpecCache(t, bin, home, "http://127.0.0.1:1/gateway", srv.URL+"/gateway/openapi.json")

		cmd := surfCmd(t, bin, home, nil,
			"market-price",
			"--address", "BTC",
			"--surf-api-base-url", "http://127.0.0.1:1/gateway/v1",
			"--rsh-retry", "0",
		)
		// Capture stdout and stderr separately.
		stdout, stderrPipe := splitOutput(cmd)

		err := cmd.Run()
		wantExitCodeWithStderr(t, err, 4, stderrPipe.String())

		// stdout must contain a JSON envelope with error.code = NETWORK_ERROR.
		var env envelope
		if jerr := json.Unmarshal(stdout.Bytes(), &env); jerr != nil {
			t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", jerr, stdout.String())
		}
		if env.Error.Code != "NETWORK_ERROR" {
			t.Errorf("want error.code = NETWORK_ERROR, got %q (stdout: %s)", env.Error.Code, stdout.String())
		}
		if env.Error.Message == "" {
			t.Errorf("want non-empty error.message")
		}

		// stderr must NOT contain the old "ERROR: Caught error:" line.
		if strings.Contains(stderrPipe.String(), "Caught error") {
			t.Errorf("stderr should not contain legacy 'Caught error' text:\n%s", stderrPipe.String())
		}
	})

	t.Run("dns failure", func(t *testing.T) {
		home := t.TempDir()
		srv, _ := newSpecAndOperationServerWithOpenAPIServer(t, "marketPrice", "http://nonexistent-host-for-test-12345.invalid", nil)
		defer srv.Close()
		seedSurfSpecCache(t, bin, home, "http://nonexistent-host-for-test-12345.invalid/gateway", srv.URL+"/gateway/openapi.json")

		cmd := surfCmd(t, bin, home, nil,
			"market-price",
			"--address", "BTC",
			"--surf-api-base-url", "http://nonexistent-host-for-test-12345.invalid/gateway/v1",
			"--rsh-retry", "0",
		)
		stdout, stderrPipe := splitOutput(cmd)

		err := cmd.Run()
		wantExitCodeWithStderr(t, err, 4, stderrPipe.String())

		var env envelope
		if jerr := json.Unmarshal(stdout.Bytes(), &env); jerr != nil {
			t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", jerr, stdout.String())
		}
		if env.Error.Code != "NETWORK_ERROR" {
			t.Errorf("want error.code = NETWORK_ERROR, got %q (stdout: %s)", env.Error.Code, stdout.String())
		}
	})

	t.Run("local timeout", func(t *testing.T) {
		home := t.TempDir()
		srv, _ := newSpecAndOperationServerWithOpenAPIServer(t, "marketPrice", "", func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(50 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		})
		defer srv.Close()
		seedSurfSpecCache(t, bin, home, srv.URL+"/gateway", srv.URL+"/gateway/openapi.json")

		cmd := surfCmd(t, bin, home, nil,
			"market-price",
			"--address", "BTC",
			"--surf-api-base-url", srv.URL+"/gateway/v1",
			"--rsh-timeout", "1ms",
		)
		stdout, _ := splitOutput(cmd)

		err := cmd.Run()
		wantExitCode(t, err, 4)

		var env envelope
		if jerr := json.Unmarshal(stdout.Bytes(), &env); jerr != nil {
			t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", jerr, stdout.String())
		}
		if env.Error.Code != "TIMEOUT" {
			t.Errorf("want error.code = TIMEOUT, got %q", env.Error.Code)
		}
		if !strings.Contains(env.Error.Message, "timed out") {
			t.Errorf("want message containing 'timed out', got %q", env.Error.Message)
		}
	})

	// Sanity check: unknown flag errors still exit 1 with stderr text
	// (they are CLI-layer errors, not transport errors).
	t.Run("unknown flag still exit 1", func(t *testing.T) {
		home := t.TempDir()
		srv, _ := newSpecServer(t, "marketPrice")
		defer srv.Close()
		seedSurfSpecCache(t, bin, home, srv.URL+"/gateway", srv.URL+"/gateway/openapi.json")

		cmd := surfCmd(t, bin, home, nil, "market-price", "--nonexistent-flag")
		err := cmd.Run()
		wantExitCode(t, err, 1)
	})
}

func seedSurfSpecCache(t *testing.T, bin string, home string, base string, specFile string) {
	t.Helper()
	surfDir := filepath.Join(home, ".surf")
	if err := os.MkdirAll(surfDir, 0o700); err != nil {
		t.Fatal(err)
	}
	apisJSON := fmt.Sprintf(`{
  "$schema": "https://rest.sh/schemas/apis.json",
  "surf": {
    "base": %q,
    "spec_files": [%q],
    "profiles": {"default": {}}
  }
}`, base, specFile)
	if err := os.WriteFile(filepath.Join(surfDir, "apis.json"), []byte(apisJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	runSurf(t, bin, home, nil, "sync")
	if _, err := os.Stat(filepath.Join(surfDir, "surf.cbor")); err != nil {
		t.Fatalf("expected seeded surf.cbor cache: %v", err)
	}
	configBytes, err := os.ReadFile(filepath.Join(surfDir, "config.json"))
	if err != nil {
		t.Fatalf("expected seeded config.json: %v", err)
	}
	if !strings.Contains(string(configBytes), base) {
		t.Fatalf("seeded cache base mismatch: want %q in config.json:\n%s", base, string(configBytes))
	}
}

// splitOutput wires the cmd's Stdout and Stderr to separate buffers.
func splitOutput(cmd *exec.Cmd) (stdout, stderr *strBuf) {
	stdout = &strBuf{}
	stderr = &strBuf{}
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return
}

// strBuf is a tiny helper to avoid pulling in bytes.Buffer just for String()/Bytes().
type strBuf struct{ b []byte }

func (s *strBuf) Write(p []byte) (int, error) { s.b = append(s.b, p...); return len(p), nil }
func (s *strBuf) Bytes() []byte               { return s.b }
func (s *strBuf) String() string              { return string(s.b) }

// wantExitCode asserts that the error from cmd.Run() corresponds to the
// expected exit code. nil err means exit 0.
func wantExitCode(t *testing.T, err error, want int) {
	t.Helper()
	wantExitCodeWithStderr(t, err, want, "")
}

func wantExitCodeWithStderr(t *testing.T, err error, want int, stderr string) {
	t.Helper()
	got := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			got = exitErr.ExitCode()
		} else {
			t.Fatalf("unexpected error type: %v", err)
		}
	}
	if got != want {
		t.Errorf("want exit code %d, got %d\nstderr: %s", want, got, stderr)
	}
}
