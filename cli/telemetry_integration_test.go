package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReportCLIEvent_ActuallySendsPost starts a local HTTP server and
// verifies that ReportCLIEvent sends a POST with the correct payload.
// This catches the os.Exit race that killed the goroutine-based version.
func TestReportCLIEvent_ActuallySendsPost(t *testing.T) {
	var received map[string]any
	var authHeader string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	t.Setenv("SURF_API_KEY", "sk-0000000000000000000000000000000000000000000000000000000000000001")
	t.Setenv("SURF_TELEMETRY_DISABLED", "")
	reset(false)
	viper.Set("config-directory", t.TempDir())
	Root.Version = "test-version"
	viper.Set("surf-api-base-url", srv.URL)

	ReportCLIEvent("market-price", 1, "missing required flag(s): --symbol")

	require.NotNil(t, received, "server should have received the POST")
	assert.Equal(t, "market-price", received["command"])
	assert.Equal(t, float64(1), received["exit_code"])
	assert.Equal(t, "missing required flag(s): --symbol", received["error"])
	assert.Equal(t, "test-version", received["version"])
	assert.NotEmpty(t, received["session_id"])
	assert.Contains(t, authHeader, "Bearer sk-")
}

// TestReportCLIEvent_SuccessEvent verifies a success event (exit 0) is sent.
func TestReportCLIEvent_SuccessEvent(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	t.Setenv("SURF_TELEMETRY_DISABLED", "")
	reset(false)
	viper.Set("config-directory", t.TempDir())
	viper.Set("surf-api-base-url", srv.URL)

	ReportCLIEvent("auth", 0, "")

	require.NotNil(t, received)
	assert.Equal(t, "auth", received["command"])
	assert.Equal(t, float64(0), received["exit_code"])
	assert.Empty(t, received["error"])
}

// TestReportCLIEvent_Disabled verifies no POST when telemetry is disabled.
func TestReportCLIEvent_TelemetryOff(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(204)
	}))
	defer srv.Close()

	t.Setenv("SURF_TELEMETRY_DISABLED", "1")
	reset(false)
	viper.Set("config-directory", t.TempDir())
	viper.Set("surf-api-base-url", srv.URL)

	ReportCLIEvent("market-price", 0, "")

	assert.False(t, called, "server should NOT be called when telemetry is disabled")
}

// TestReportCLIEvent_ErrorTruncation verifies long errors are truncated.
func TestReportCLIEvent_ErrorTruncation(t *testing.T) {
	var received map[string]any

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&received)
		w.WriteHeader(204)
	}))
	defer srv.Close()

	t.Setenv("SURF_API_KEY", "sk-0000000000000000000000000000000000000000000000000000000000000001")
	t.Setenv("SURF_TELEMETRY_DISABLED", "")
	reset(false)
	viper.Set("config-directory", t.TempDir())
	viper.Set("surf-api-base-url", srv.URL)

	longErr := make([]byte, 1000)
	for i := range longErr {
		longErr[i] = 'x'
	}

	ReportCLIEvent("crash", 1, string(longErr))

	require.NotNil(t, received)
	errMsg, _ := received["error"].(string)
	assert.Len(t, errMsg, 500, "error should be truncated to 500 chars")
}
