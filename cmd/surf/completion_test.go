package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCompleteProtocolListsAPIOperations pins the shell-completion contract:
// `surf __complete` must reach Cobra's hidden protocol command (not be routed
// through the hidden API subcommand), and its candidates must arrive on
// stdout where the shell reads them.
func TestCompleteProtocolListsAPIOperations(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()
	srv, _ := newSpecAndOperationServer(t, "news-feed", nil)
	t.Cleanup(srv.Close)
	seedSurfSpecCache(t, bin, home, srv.URL+"/gateway", srv.URL+"/gateway/openapi.json")

	cmd := surfCmd(t, bin, home, nil, "__complete", "new")
	stdout, stderr := splitOutput(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("surf __complete failed: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "news-feed") {
		t.Errorf("completion candidates missing from stdout:\n%s", out)
	}
	if strings.Contains(out, "unknown command") {
		t.Errorf("__complete was routed as a regular command:\n%s", out)
	}
	if strings.Contains(stderr.String(), "No cached API spec") {
		t.Errorf("completion should not auto-sync:\n%s", stderr.String())
	}
}

// TestCompletionScriptGoesToStdout guards the generated script path: piping
// `surf completion bash > file` must produce the script, not diagnostics.
func TestCompletionScriptGoesToStdout(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()

	cmd := surfCmd(t, bin, home, nil, "completion", "bash")
	stdout, stderr := splitOutput(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("surf completion bash failed: %v\n%s", err, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "# bash completion") {
		t.Errorf("completion script missing from stdout:\n%s", stdout.String())
	}
	if stderr.String() != "" {
		t.Errorf("completion script wrote to stderr:\n%s", stderr.String())
	}
}

// TestCompleteDoesNotSyncWithoutCache exercises the meta-command path: with
// no cache at all, completion must still answer immediately (empty candidates
// plus the no-file-comp directive) and must not attempt a spec download.
func TestCompleteDoesNotSyncWithoutCache(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()
	surfDir := filepath.Join(home, ".surf")
	if err := os.MkdirAll(surfDir, 0o700); err != nil {
		t.Fatal(err)
	}
	apisJSON := `{
  "$schema": "https://rest.sh/schemas/apis.json",
  "surf": {
    "base": "http://127.0.0.1:1/gateway",
    "spec_files": ["http://127.0.0.1:1/openapi.json"],
    "profiles": {"default": {}}
  }
}`
	if err := os.WriteFile(filepath.Join(surfDir, "apis.json"), []byte(apisJSON), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := surfCmd(t, bin, home, nil, "__complete", "new")
	stdout, stderr := splitOutput(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("surf __complete without a cache failed: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), ":4") {
		t.Errorf("expected the no-file-comp directive on stdout:\n%s", stdout.String())
	}
	out := stdout.String() + stderr.String()
	if strings.Contains(out, "syncing") || strings.Contains(out, "Auto-sync failed") {
		t.Errorf("completion must not sync without a cache:\n%s", out)
	}
}

// TestCompletionDiagnosticsStayOnStderr guards the redirect footgun:
// `surf completion bahs > file` (or `completion --help`) must not write a
// Usage page to stdout, while the real script path keeps stdout clean of
// diagnostics (covered by TestCompletionScriptGoesToStdout).
func TestCompletionDiagnosticsStayOnStderr(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()

	cases := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{name: "no shell", args: []string{"completion"}},
		{name: "help", args: []string{"completion", "bash", "--help"}},
		{name: "typo", args: []string{"completion", "bahs"}},
		{name: "extra arg", args: []string{"completion", "bash", "extra"}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := surfCmd(t, bin, home, nil, tc.args...)
			stdout, stderr := splitOutput(cmd)
			err := cmd.Run()
			if tc.wantErr && err == nil {
				t.Fatalf("surf %v unexpectedly succeeded", tc.args)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("surf %v failed: %v\n%s", tc.args, err, stderr.String())
			}
			if stdout.String() != "" {
				t.Errorf("completion diagnostics went to stdout:\n%s", stdout.String())
			}
			if stderr.String() == "" {
				t.Errorf("expected help/error on stderr")
			}
		})
	}
}
