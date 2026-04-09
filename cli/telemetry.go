package cli

import (
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

// setTelemetryHeaders adds telemetry headers to the request if the API key
// is a user key. Deploy keys, sees keys, and anonymous requests are skipped.
func setTelemetryHeaders(req *http.Request) {
	if !isUserKey(req.Header.Get("Authorization")) {
		return
	}

	configDir := viper.GetString("config-directory")
	req.Header.Set("X-Surf-CLI-Version", Root.Version)
	req.Header.Set("X-Surf-CLI-Command", currentCommand)
	req.Header.Set("X-Surf-Session-ID", getSessionID(configDir))
}
