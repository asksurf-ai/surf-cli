package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

func TestIsUserKey(t *testing.T) {
	tests := []struct {
		header string
		want   bool
	}{
		{"Bearer sk-719bc951719243fa27263b46dd56b777364a96c9b909a6116918b8057d962203", true},
		{"Bearer sk-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", true},
		{"Bearer sk-12345", false},
		{"Bearer sk_sess_719bc951719243fa27263b46dd56b777364a96c9b909a6116918b8057d962203", false},
		{"Bearer sk_deploy_719bc951719243fa27263b46dd56b777364a96c9b909a6116918b8057d962203", false},
		{"Bearer sk-", false},
		{"Bearer other-token", false},
		{"", false},
	}
	for _, tt := range tests {
		got := isUserKey(tt.header)
		assert.Equal(t, tt.want, got, "isUserKey(%q)", tt.header)
	}
}

func TestGetSessionID_New(t *testing.T) {
	t.Setenv("SURF_SESSION_ID", "")
	dir := t.TempDir()

	id := getSessionID(dir)
	assert.NotEmpty(t, id)

	// File should exist.
	data, err := os.ReadFile(filepath.Join(dir, "session.json"))
	assert.NoError(t, err)

	var s sessionData
	assert.NoError(t, json.Unmarshal(data, &s))
	assert.Equal(t, id, s.ID)
	assert.InDelta(t, time.Now().Unix(), s.UpdatedAt, 2)
}

func TestGetSessionID_Reuse(t *testing.T) {
	t.Setenv("SURF_SESSION_ID", "")
	dir := t.TempDir()

	// Create a session 5 minutes ago.
	s := sessionData{
		ID:        "existing-session-id",
		UpdatedAt: time.Now().Add(-5 * time.Minute).Unix(),
	}
	data, _ := json.Marshal(s)
	os.WriteFile(filepath.Join(dir, "session.json"), data, 0600)

	id := getSessionID(dir)
	assert.Equal(t, "existing-session-id", id)

	// Timestamp should be updated.
	data, _ = os.ReadFile(filepath.Join(dir, "session.json"))
	var updated sessionData
	json.Unmarshal(data, &updated)
	assert.InDelta(t, time.Now().Unix(), updated.UpdatedAt, 2)
}

func TestGetSessionID_Expired(t *testing.T) {
	t.Setenv("SURF_SESSION_ID", "")
	dir := t.TempDir()

	// Create a session 31 minutes ago.
	s := sessionData{
		ID:        "old-session-id",
		UpdatedAt: time.Now().Add(-31 * time.Minute).Unix(),
	}
	data, _ := json.Marshal(s)
	os.WriteFile(filepath.Join(dir, "session.json"), data, 0600)

	id := getSessionID(dir)
	assert.NotEqual(t, "old-session-id", id)
	assert.NotEmpty(t, id)
}

func TestGetSessionID_EnvOverride(t *testing.T) {
	t.Setenv("SURF_SESSION_ID", "custom-agent-session")
	dir := t.TempDir()

	id := getSessionID(dir)
	assert.Equal(t, "custom-agent-session", id)

	// No file should be created.
	_, err := os.Stat(filepath.Join(dir, "session.json"))
	assert.True(t, os.IsNotExist(err))
}

func TestSetTelemetryHeaders_UserKey(t *testing.T) {
	reset(false)
	viper.Set("config-directory", t.TempDir())
	currentCommand = "market-price"

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("Authorization", "Bearer sk-719bc951719243fa27263b46dd56b777364a96c9b909a6116918b8057d962203")
	setTelemetryHeaders(req)

	assert.NotEmpty(t, req.Header.Get("X-Surf-CLI-Version"))
	assert.Equal(t, "market-price", req.Header.Get("X-Surf-CLI-Command"))
	assert.NotEmpty(t, req.Header.Get("X-Surf-Session-ID"))
}

func TestSetTelemetryHeaders_DeployKey_NoHeadersNoSessionFile(t *testing.T) {
	reset(false)
	t.Setenv("SURF_SESSION_ID", "")
	dir := t.TempDir()
	viper.Set("config-directory", dir)
	currentCommand = "market-price"

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("Authorization", "Bearer sk-deploy-134124")
	setTelemetryHeaders(req)

	assert.Empty(t, req.Header.Get("X-Surf-CLI-Version"))
	assert.Empty(t, req.Header.Get("X-Surf-CLI-Command"))
	assert.Empty(t, req.Header.Get("X-Surf-Session-ID"))
	// Session file must not be created.
	_, err := os.Stat(filepath.Join(dir, "session.json"))
	assert.True(t, os.IsNotExist(err), "session.json should not exist for deploy keys")
}

func TestSetTelemetryHeaders_SeesKey_NoHeadersNoSessionFile(t *testing.T) {
	reset(false)
	t.Setenv("SURF_SESSION_ID", "")
	dir := t.TempDir()
	viper.Set("config-directory", dir)
	currentCommand = "market-price"

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	req.Header.Set("Authorization", "Bearer sk-sees-abc123")
	setTelemetryHeaders(req)

	assert.Empty(t, req.Header.Get("X-Surf-CLI-Version"))
	_, err := os.Stat(filepath.Join(dir, "session.json"))
	assert.True(t, os.IsNotExist(err), "session.json should not exist for sees keys")
}

func TestReportCLIEvent_NoKey_NoBaseURL(t *testing.T) {
	t.Setenv("SURF_API_KEY", "")
	t.Setenv("SURF_TELEMETRY_DISABLED", "")
	reset(false)
	viper.Set("config-directory", t.TempDir())
	// No base URL → returns before launching goroutine.
	ReportCLIEvent("market-price", 0, "")
}

func TestReportCLIEvent_WithKey_NoBaseURL(t *testing.T) {
	t.Setenv("SURF_API_KEY", "sk-719bc951719243fa27263b46dd56b777364a96c9b909a6116918b8057d962203")
	t.Setenv("SURF_TELEMETRY_DISABLED", "")
	reset(false)
	viper.Set("config-directory", t.TempDir())
	// No base URL → returns before launching goroutine.
	ReportCLIEvent("market-price", 1, "missing required flag(s): --symbol")
}

func TestReportCLIEvent_Disabled(t *testing.T) {
	t.Setenv("SURF_TELEMETRY_DISABLED", "1")
	reset(false)
	viper.Set("config-directory", t.TempDir())
	configs["surf"] = &APIConfig{Base: "http://127.0.0.1:1"}
	// Telemetry disabled → returns immediately, no goroutine.
	ReportCLIEvent("market-price", 0, "")
	delete(configs, "surf")
}

func TestTelemetryDisabled_EnvVar(t *testing.T) {
	reset(false)
	t.Setenv("SURF_TELEMETRY_DISABLED", "1")
	assert.True(t, TelemetryDisabled())

	t.Setenv("SURF_TELEMETRY_DISABLED", "true")
	assert.True(t, TelemetryDisabled())

	t.Setenv("SURF_TELEMETRY_DISABLED", "")
	assert.False(t, TelemetryDisabled())
}

func TestTelemetryDisabled_Config(t *testing.T) {
	reset(false)
	t.Setenv("SURF_TELEMETRY_DISABLED", "")
	Cache.Set("telemetry_disabled", true)
	assert.True(t, TelemetryDisabled())

	Cache.Set("telemetry_disabled", false)
	assert.False(t, TelemetryDisabled())
}

func TestTelemetryDisabled_EnvOverridesConfig(t *testing.T) {
	reset(false)
	Cache.Set("telemetry_disabled", false)
	t.Setenv("SURF_TELEMETRY_DISABLED", "1")
	assert.True(t, TelemetryDisabled(), "env var should override config")
}

func TestSetCurrentCommand(t *testing.T) {
	currentCommand = ""
	SetCurrentCommand("auth")
	assert.Equal(t, "auth", GetCurrentCommand())

	// Operation.go overwrites unconditionally.
	SetCurrentCommand("market-price")
	assert.Equal(t, "market-price", GetCurrentCommand())
}

func TestSetTelemetryHeaders_NoAuth_NoSessionFile(t *testing.T) {
	reset(false)
	t.Setenv("SURF_SESSION_ID", "")
	dir := t.TempDir()
	viper.Set("config-directory", dir)
	currentCommand = "market-price"

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	setTelemetryHeaders(req)

	assert.Empty(t, req.Header.Get("X-Surf-CLI-Version"))
	assert.Empty(t, req.Header.Get("X-Surf-CLI-Command"))
	assert.Empty(t, req.Header.Get("X-Surf-Session-ID"))
	_, err := os.Stat(filepath.Join(dir, "session.json"))
	assert.True(t, os.IsNotExist(err), "session.json should not exist without auth")
}
