package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/zalando/go-keyring"
)

const sessionTimeout = 30 * time.Minute

// currentCommand is set by operation.command() before making the HTTP request.
var currentCommand string

type sessionData struct {
	ID        string `json:"id"`
	UpdatedAt int64  `json:"updated_at"`
}

var userKeyPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// isUserKey returns true if the Authorization header contains a user API key
// (sk- followed by hex/digits, not sk-deploy-* or sk-sees-*).
func isUserKey(authHeader string) bool {
	if !strings.HasPrefix(authHeader, "Bearer sk-") {
		return false
	}
	suffix := strings.TrimPrefix(authHeader, "Bearer sk-")
	return userKeyPattern.MatchString(suffix)
}

// newUUID generates a random UUID v4 string.
func newUUID() string {
	var b [16]byte
	rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// getSessionID returns the current session ID. If SURF_SESSION_ID is set,
// it is used directly. Otherwise, a file-based session at ~/.surf/session.json
// is used with a 30-minute inactivity timeout.
func getSessionID(configDir string) string {
	if id := os.Getenv("SURF_SESSION_ID"); id != "" {
		return id
	}

	sessionFile := filepath.Join(configDir, "session.json")
	now := time.Now()

	// Try to read existing session.
	if data, err := os.ReadFile(sessionFile); err == nil {
		var s sessionData
		if err := json.Unmarshal(data, &s); err == nil && s.ID != "" {
			if now.Unix()-s.UpdatedAt < int64(sessionTimeout.Seconds()) {
				// Session still valid — update timestamp.
				s.UpdatedAt = now.Unix()
				if updated, err := json.Marshal(s); err == nil {
					os.WriteFile(sessionFile, updated, 0600)
				}
				return s.ID
			}
		}
	}

	// Create new session.
	s := sessionData{
		ID:        newUUID(),
		UpdatedAt: now.Unix(),
	}
	if data, err := json.Marshal(s); err == nil {
		os.WriteFile(sessionFile, data, 0600)
	}
	return s.ID
}

// telemetryDisabled returns true if the user has opted out of telemetry via
// SURF_TELEMETRY_DISABLED=1 env var or `surf telemetry disable` (config).
// TelemetryDisabled reports whether telemetry is opted out.
func TelemetryDisabled() bool {
	if v := os.Getenv("SURF_TELEMETRY_DISABLED"); v == "1" || v == "true" {
		return true
	}
	if Cache != nil && Cache.GetBool("telemetry_disabled") {
		return true
	}
	return false
}

// setTelemetryHeaders adds telemetry headers to the request if the API key
// is a user key. Deploy keys, sees keys, and anonymous requests are skipped.
func setTelemetryHeaders(req *http.Request) {
	if TelemetryDisabled() {
		return
	}
	if !isUserKey(req.Header.Get("Authorization")) {
		return
	}

	configDir := viper.GetString("config-directory")
	req.Header.Set("X-Surf-CLI-Version", Root.Version)
	req.Header.Set("X-Surf-CLI-Command", currentCommand)
	req.Header.Set("X-Surf-Session-ID", getSessionID(configDir))
}

// SetCurrentCommand sets the current command name for telemetry.
// Called from PersistentPreRun in main.go; overwritten by operation.go RunE
// for API operations with the more specific operation name.
func SetCurrentCommand(name string) {
	currentCommand = name
}

// GetCurrentCommand returns the current command name.
func GetCurrentCommand() string {
	return currentCommand
}

// getAPIKey returns the configured API key (env > keychain > cache).
// Returns "" if none configured.
func getAPIKey() string {
	if key := os.Getenv("SURF_API_KEY"); key != "" {
		return key
	}
	profile := viper.GetString("rsh-profile")
	if profile == "" {
		profile = "default"
	}
	keychainUser := "surf:" + profile
	if token, err := keyring.Get(KeyringService, keychainUser); err == nil && token != "" {
		return token
	}
	if token := Cache.GetString(keychainUser + ".api_key"); token != "" {
		return token
	}
	return ""
}

// ReportCLIEvent sends a CLI invocation event to the backend.
// Fire-and-forget: launches a goroutine with a 2-second timeout.
// Reports for all invocations. With API key: hermod resolves user_id.
// Without: hermod logs as anonymous with client IP.
func ReportCLIEvent(command string, exitCode int, errMsg string) {
	if TelemetryDisabled() {
		return
	}
	configDir := viper.GetString("config-directory")
	sessionID := getSessionID(configDir)

	// Base URL from API config.
	baseURL := ""
	if cfg, ok := configs["surf"]; ok && cfg.Base != "" {
		baseURL = cfg.Base
	}
	if override := viper.GetString("rsh-server"); override != "" {
		baseURL = override
	}
	if baseURL == "" {
		return
	}

	// Truncate error to 500 chars.
	if len(errMsg) > 500 {
		errMsg = errMsg[:500]
	}

	apiKey := getAPIKey()

	payload, _ := json.Marshal(map[string]any{
		"command":    command,
		"exit_code":  exitCode,
		"error":      errMsg,
		"version":    Root.Version,
		"session_id": sessionID,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		baseURL+"/v1/cli/event", bytes.NewReader(payload))
	if err != nil {
		return
	}
	if apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}
