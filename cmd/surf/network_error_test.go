package main

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// TestNetworkErrorEnvelope verifies that transport-layer failures
// (connection refused, DNS failure, local timeout) produce a structured
// error envelope on stdout + exit 4, rather than exit 1 + stderr text.
//
// See docs/CLI_DESIGN_PRINCIPLES.md §4.1 and §4.2.1, and
// docs/CLI_V1_ALPHA_26_REVIEW.md items #3 and #4 for the rationale.
func TestNetworkErrorEnvelope(t *testing.T) {
	bin := buildSurfBin(t)

	type envelope struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}

	t.Run("connection refused", func(t *testing.T) {
		cmd := exec.Command(bin,
			"market-price",
			"--symbol", "BTC",
			"--time-range", "1d",
			"--rsh-server", "http://127.0.0.1:1",
			"--rsh-retry", "0",
		)
		// Capture stdout and stderr separately.
		stdout, stderrPipe := splitOutput(cmd)

		err := cmd.Run()
		wantExitCode(t, err, 4)

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
		cmd := exec.Command(bin,
			"market-price",
			"--symbol", "BTC",
			"--time-range", "1d",
			"--rsh-server", "http://nonexistent-host-for-test-12345.invalid",
			"--rsh-retry", "0",
		)
		stdout, _ := splitOutput(cmd)

		err := cmd.Run()
		wantExitCode(t, err, 4)

		var env envelope
		if jerr := json.Unmarshal(stdout.Bytes(), &env); jerr != nil {
			t.Fatalf("stdout is not valid JSON: %v\nstdout: %s", jerr, stdout.String())
		}
		if env.Error.Code != "NETWORK_ERROR" {
			t.Errorf("want error.code = NETWORK_ERROR, got %q", env.Error.Code)
		}
	})

	t.Run("local timeout", func(t *testing.T) {
		cmd := exec.Command(bin,
			"market-price",
			"--symbol", "BTC",
			"--time-range", "1d",
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
		cmd := exec.Command(bin, "market-price", "--nonexistent-flag")
		err := cmd.Run()
		wantExitCode(t, err, 1)
	})
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
func (s *strBuf) Bytes() []byte                { return s.b }
func (s *strBuf) String() string               { return string(s.b) }

// wantExitCode asserts that the error from cmd.Run() corresponds to the
// expected exit code. nil err means exit 0.
func wantExitCode(t *testing.T, err error, want int) {
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
		t.Errorf("want exit code %d, got %d", want, got)
	}
}
