package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestExpiredCacheStillListsCommands pins the behaviour that broke after an
// upgrade or a day offline: command registration used the TTL-checked cache
// loader, so `surf --help` listed zero API commands until some other command
// refreshed the spec over the network.
func TestExpiredCacheStillListsCommands(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()

	srv, _ := newSpecAndOperationServer(t, "cache-fixture", nil)
	t.Cleanup(srv.Close)
	seedSurfSpecCache(t, bin, home, srv.URL+"/gateway", srv.URL+"/gateway/openapi.json")

	// Age the cache out without touching the spec itself.
	configPath := filepath.Join(home, ".surf", "config.json")
	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("config.json is not JSON: %v\n%s", err, raw)
	}
	cfg["surf"]["expires"] = "2020-01-01T00:00:00Z"
	updated, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, updated, 0o600); err != nil {
		t.Fatal(err)
	}

	// Point the base at a closed port so any attempted sync fails fast; the
	// command tree must still be populated from the stale cache.
	cmd := surfCmd(t, bin, home, nil, "--help")
	stdout, stderr := splitOutput(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("surf --help failed: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}
	out := stdout.String() + stderr.String()
	if !strings.Contains(out, "cache-fixture") {
		t.Errorf("expired cache should still register API commands:\n%s", out)
	}
	if strings.Contains(out, "No cached API spec") {
		t.Errorf("--help should not sync when a stale cache is usable:\n%s", out)
	}
}
