package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
)

// buildSurfBin builds the surf binary into a temp dir and returns its path.
// Tests skip gracefully when no cached API spec exists, since operation
// commands are registered from the cache.
func buildSurfBin(t *testing.T) string {
	t.Helper()
	bin := t.TempDir() + "/surf"
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = "."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}

	home, _ := os.UserHomeDir()
	if _, err := os.Stat(home + "/.surf/surf.cbor"); os.IsNotExist(err) {
		t.Skip("no cached API spec — run `surf sync` first")
	}
	return bin
}

// TestNoDoubleSurfInUsage builds the surf binary and verifies that
// "surf surf" never appears in help or error output.
func TestNoDoubleSurfInUsage(t *testing.T) {
	bin := buildSurfBin(t)

	tests := []struct {
		name string
		args []string
	}{
		{"root help", []string{"--help"}},
		{"operation help", []string{"market-price", "--help"}},
		{"flag error", []string{"market-price", "--bogus"}},
		{"unknown command", []string{"nonexistent-command"}},
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

// TestUnknownCommandShowsError verifies that an unknown command prints a
// clean error message instead of dumping the API subcommand's help.
func TestUnknownCommandShowsError(t *testing.T) {
	bin := buildSurfBin(t)

	cmd := exec.Command(bin, "nonexistent-command")
	out, err := cmd.CombinedOutput()
	output := string(out)

	if err == nil {
		t.Fatal("expected non-zero exit code for unknown command")
	}
	if !strings.Contains(output, `unknown command "nonexistent-command"`) {
		t.Errorf("expected unknown command error, got:\n%s", output)
	}
	if strings.Contains(output, "surf surf") {
		t.Errorf("found 'surf surf' in unknown command output:\n%s", output)
	}
}

// TestOperationHelpShowsParametersAndDescription guards against a regression
// where `surf <op> -h` resolves to a lightweight stub on Root that only
// carries the short title — no Long description, no Example, no flags.
//
// The bug looked like:
//
//	$ surf kalshi-markets -h
//	Kalshi Markets
//
//	Usage:
//	  surf kalshi-markets [flags]
//
//	Flags:
//	  -h, --help   help for kalshi-markets
//
// Help output for an API operation must include its parameter flags and
// the prose description sourced from the cached OpenAPI spec. We assert
// on a distinctive phrase from each operation's prose rather than on
// auto-generated section headers like `## Option Schema` — those are
// stripped by stripSchemaBlocks (see cli/schema_strip.go and review #5).
func TestOperationHelpShowsParametersAndDescription(t *testing.T) {
	bin := buildSurfBin(t)

	cases := []struct {
		op         string
		wantFlag   string // parameter flag that must appear in help
		wantProse  string // distinctive prose phrase proving Long description was rendered
		dontAppear string // auto-generated heading that must NOT appear (stripped)
	}{
		{
			op:         "kalshi-markets",
			wantFlag:   "--limit",
			wantProse:  "Returns Kalshi markets",
			dontAppear: "## Option Schema",
		},
		{
			op:         "market-price",
			wantFlag:   "--symbol",
			wantProse:  "Returns historical price data points",
			dontAppear: "## Option Schema",
		},
	}

	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			cmd := exec.Command(bin, tc.op, "-h")
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s -h failed: %v\n%s", tc.op, err, out)
			}
			output := string(out)

			if !strings.Contains(output, tc.wantFlag) {
				t.Errorf("%s -h is missing parameter flag %q — stub command did not inherit operation flags:\n%s", tc.op, tc.wantFlag, output)
			}

			if !strings.Contains(output, tc.wantProse) {
				t.Errorf("%s -h is missing prose %q — stub command did not inherit Long description:\n%s", tc.op, tc.wantProse, output)
			}

			if strings.Contains(output, tc.dontAppear) {
				t.Errorf("%s -h should not contain %q after schema stripping:\n%s", tc.op, tc.dontAppear, output)
			}
		})
	}
}

// TestUnknownCommandSuggestsTypoFix verifies that a close typo triggers a
// "Did you mean?" suggestion while a far-off name does not.
func TestUnknownCommandSuggestsTypoFix(t *testing.T) {
	bin := buildSurfBin(t)

	tests := []struct {
		name       string
		args       []string
		wantHint   string // substring that must appear (empty = must NOT suggest)
		wantNoHint bool   // true = "Did you mean" must NOT appear
	}{
		{
			name:     "operation typo",
			args:     []string{"market-pric"},
			wantHint: "market-price",
		},
		{
			name:     "root command typo",
			args:     []string{"catlog"},
			wantHint: "catalog",
		},
		{
			name:     "auth typo",
			args:     []string{"auht"},
			wantHint: "auth",
		},
		{
			name:       "far typo no suggestion",
			args:       []string{"xyz"},
			wantNoHint: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(bin, tt.args...)
			out, _ := cmd.CombinedOutput()
			output := string(out)

			hasSuggestion := strings.Contains(output, "Did you mean")
			if tt.wantNoHint {
				if hasSuggestion {
					t.Errorf("expected no suggestion for %v, got:\n%s", tt.args, output)
				}
				return
			}
			if !hasSuggestion {
				t.Errorf("expected 'Did you mean' for %v, got:\n%s", tt.args, output)
			}
			if !strings.Contains(output, tt.wantHint) {
				t.Errorf("expected suggestion %q for %v, got:\n%s", tt.wantHint, tt.args, output)
			}
		})
	}
}

// TestVersionFlag verifies that -v and --version both print the version string.
func TestVersionFlag(t *testing.T) {
	bin := buildSurfBin(t)

	for _, flag := range []string{"-v", "--version"} {
		t.Run(flag, func(t *testing.T) {
			out, err := exec.Command(bin, flag).CombinedOutput()
			if err != nil {
				t.Fatalf("%s failed: %v\n%s", flag, err, out)
			}
			if !strings.Contains(string(out), "surf version") {
				t.Errorf("%s output missing 'surf version': %s", flag, out)
			}
		})
	}
}

// TestDebugFlag verifies that --debug enables debug logging to stderr.
func TestDebugFlag(t *testing.T) {
	bin := buildSurfBin(t)

	cmd := exec.Command(bin, "--debug", "market-price", "--symbol", "BTC",
		"--surf-api-base-url", "http://127.0.0.1:1", "--rsh-retry", "0")
	out, _ := cmd.CombinedOutput()
	if !strings.Contains(string(out), "DEBUG:") {
		t.Errorf("--debug should produce DEBUG: lines, got:\n%s", string(out))
	}
}

// TestQuietFlag verifies that --quiet suppresses WARN/INFO but not errors.
func TestQuietFlag(t *testing.T) {
	bin := buildSurfBin(t)

	t.Run("errors still show", func(t *testing.T) {
		cmd := exec.Command(bin, "--quiet", "market-price", "--bogus")
		out, _ := cmd.CombinedOutput()
		if !strings.Contains(string(out), "unknown flag") {
			t.Errorf("--quiet should still show errors, got:\n%s", string(out))
		}
	})

	// Spin up a server that always returns 429 to trigger retry WARN lines.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	args := []string{"market-price", "--symbol", "BTC", "--time-range", "1d",
		"--surf-api-base-url", srv.URL, "--rsh-retry", "1"}

	t.Run("without --quiet shows WARN", func(t *testing.T) {
		out, _ := exec.Command(bin, args...).CombinedOutput()
		if !strings.Contains(string(out), "WARN:") {
			t.Errorf("expected WARN: line on 429 retry, got:\n%s", string(out))
		}
	})

	t.Run("--quiet suppresses WARN", func(t *testing.T) {
		out, _ := exec.Command(bin, append([]string{"--quiet"}, args...)...).CombinedOutput()
		if strings.Contains(string(out), "WARN:") {
			t.Errorf("--quiet should suppress WARN:, got:\n%s", string(out))
		}
	})
}

// TestListOperationsFlagsAreKebabCase guards against snake_case flag names
// leaking into the CLI surface from the OpenAPI spec. CLI conventions (and
// the rest of the surf CLI) use kebab-case for flag names — e.g.
// `--sort-by`, not `--sort_by`. The `surf list-operations` listing is the
// canonical surface where this regression is visible to users, so we parse
// its output and fail on any flag containing an underscore.
func TestListOperationsFlagsAreKebabCase(t *testing.T) {
	bin := buildSurfBin(t)

	cmd := exec.Command(bin, "list-operations")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("list-operations failed: %v\n%s", err, out)
	}

	// Match every long flag (e.g. --sort_by) on each line. Path params
	// are rendered as <name> and intentionally excluded.
	flagRe := regexp.MustCompile(`--[A-Za-z0-9_-]+`)

	type offender struct {
		op    string
		flags []string
	}
	var offenders []offender

	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		// Lines look like: "  GET    fund-portfolio   Fund Portfolio  (--id, ...)"
		// The op name is the second field after the HTTP method.
		if fields[0] != "GET" && fields[0] != "POST" && fields[0] != "PUT" && fields[0] != "DELETE" && fields[0] != "PATCH" {
			continue
		}
		op := fields[1]

		var bad []string
		for _, f := range flagRe.FindAllString(line, -1) {
			if strings.Contains(f, "_") {
				bad = append(bad, f)
			}
		}
		if len(bad) > 0 {
			offenders = append(offenders, offender{op: op, flags: bad})
		}
	}

	if len(offenders) > 0 {
		var b strings.Builder
		b.WriteString("found snake_case flags in `surf list-operations` output (expected kebab-case):\n")
		for _, o := range offenders {
			b.WriteString("  ")
			b.WriteString(o.op)
			b.WriteString(": ")
			b.WriteString(strings.Join(o.flags, ", "))
			b.WriteString("\n")
		}
		t.Error(b.String())
	}
}
