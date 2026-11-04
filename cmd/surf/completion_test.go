package main

import (
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
