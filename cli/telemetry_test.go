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
		{"Bearer sk-719bc951719243fa27263b46dd56b777", true},
		{"Bearer sk-12345", true},
		{"Bearer sk-deploy-134124", false},
		{"Bearer sk-sees-abc123", false},
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
	req.Header.Set("Authorization", "Bearer sk-12345")
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
