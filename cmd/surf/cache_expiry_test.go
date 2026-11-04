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

	// Close the server so any attempted sync fails fast; the command tree
	// must still be populated from the stale cache.
	srv.Close()

	cmd := surfCmd(t, bin, home, nil, "--help")
	stdout, stderr := splitOutput(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("surf --help failed: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}
	out := stdout.String() + stderr.String()
	if !strings.Contains(out, "cache-fixture") {
		t.Errorf("expired cache should still register API commands:\n%s", out)
	}
	if strings.Contains(out, "No cached API spec") || strings.Contains(out, "Auto-sync failed") {
		t.Errorf("--help should not sync when a stale cache is usable:\n%s", out)
	}
}

func readCacheConfig(t *testing.T, home string) map[string]map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(home, ".surf", "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]map[string]any
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("config.json is not JSON: %v\n%s", err, raw)
	}
	return cfg
}

func writeCacheConfig(t *testing.T, home string, cfg map[string]map[string]any) {
	t.Helper()
	updated, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".surf", "config.json"), updated, 0o600); err != nil {
		t.Fatal(err)
	}
}

// dropCacheVersion simulates a cache written by an older CLI (or one from
// before cache versioning existed).
func dropCacheVersion(t *testing.T, home string) {
	t.Helper()
	cfg := readCacheConfig(t, home)
	delete(cfg["surf"], "cache_version")
	writeCacheConfig(t, home, cfg)
}

// TestUpgradedCacheStillListsCommands covers the upgrade path: existing users
// have caches without cache_version, and registration must not treat those as
// missing. Only the catalog fields need a version match.
func TestUpgradedCacheStillListsCommands(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()

	srv, _ := newSpecAndOperationServer(t, "cache-fixture", nil)
	t.Cleanup(srv.Close)
	seedSurfSpecCache(t, bin, home, srv.URL+"/gateway", srv.URL+"/gateway/openapi.json")
	dropCacheVersion(t, home)
	srv.Close()

	cmd := surfCmd(t, bin, home, nil, "--help")
	stdout, stderr := splitOutput(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("surf --help failed: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}
	out := stdout.String() + stderr.String()
	if !strings.Contains(out, "cache-fixture") {
		t.Errorf("an unversioned cache should still register API commands:\n%s", out)
	}
	if strings.Contains(out, "No cached API spec") || strings.Contains(out, "Auto-sync failed") {
		t.Errorf("--help should not sync for an unversioned cache:\n%s", out)
	}
}

// TestUpgradedCacheRefreshesCatalog asserts the other half: a catalog read
// rejects the unversioned cache, which makes main auto-sync and persist the
// current version.
func TestUpgradedCacheRefreshesCatalog(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()

	srv, _ := newSpecAndOperationServer(t, "cache-fixture", nil)
	t.Cleanup(srv.Close)
	seedSurfSpecCache(t, bin, home, srv.URL+"/gateway", srv.URL+"/gateway/openapi.json")
	dropCacheVersion(t, home)

	cmd := surfCmd(t, bin, home, nil, "list-operations")
	stdout, stderr := splitOutput(cmd)
	if err := cmd.Run(); err != nil {
		t.Fatalf("surf list-operations failed: %v\n%s\n%s", err, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String()+stderr.String(), "cache-fixture") {
		t.Errorf("catalog should be refreshed after auto-sync:\n%s", stdout.String())
	}
	cfg := readCacheConfig(t, home)
	if got := cfg["surf"]["cache_version"]; got == nil || got == float64(0) {
		t.Errorf("cache_version = %v, want a persisted version", got)
	}
}

// TestUpgradedCacheOperationDoesNotSync is the upgrade regression that the
// pre-sync guard used to miss: a normal operation with an unversioned cache
// must run from the cache, not attempt a spec download. Offline, that attempt
// turned into a panic inside cli.Run.
func TestUpgradedCacheOperationDoesNotSync(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()

	srv, _ := newSpecAndOperationServer(t, "cache-fixture", nil)
	t.Cleanup(srv.Close)
	seedSurfSpecCache(t, bin, home, srv.URL+"/gateway", srv.URL+"/gateway/openapi.json")
	dropCacheVersion(t, home)
	srv.Close()

	cmd := surfCmd(t, bin, home, nil, "cache-fixture")
	_, stderr := splitOutput(cmd)
	err := cmd.Run()
	if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 2 {
		t.Fatalf("unversioned cache must not panic offline:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "panic:") {
		t.Fatalf("unversioned cache must not panic offline:\n%s", stderr.String())
	}
	if strings.Contains(stderr.String(), "No cached API spec, syncing") {
		t.Errorf("a normal operation should not trigger a spec sync:\n%s", stderr.String())
	}
	if err == nil {
		t.Fatalf("expected the request to fail against the closed server")
	}
}
