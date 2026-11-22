package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/logrusorgru/aurora"
)

// setupDebugLogTest enables file logging in a temp dir and restores all
// mutated globals (au, enableVerbose) afterwards.
func setupDebugLogTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	EnableDebugLog(dir, time.Now())
	t.Cleanup(closeDebugLog)

	oldAu := au
	if au == nil {
		au = aurora.NewAurora(false)
	}
	t.Cleanup(func() { au = oldAu })
	return dir
}

func readDebugLog(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "debug.log"))
	if err != nil {
		t.Fatalf("read debug.log: %v", err)
	}
	return string(data)
}

func TestDebugLogWritesSanitizedLines(t *testing.T) {
	dir := setupDebugLogTest(t)

	oldVerbose := enableVerbose
	enableVerbose = true
	t.Cleanup(func() { enableVerbose = oldVerbose })

	LogDebug("colored \x1b[38;5;243mtext\x1b[0m")
	DebugLogSummary("market-price", 0, "", 123*time.Millisecond)

	content := readDebugLog(t, dir)

	if !strings.Contains(content, "[DEBUG]") {
		t.Errorf("expected DEBUG level tag in log:\n%s", content)
	}
	if strings.Contains(content, "\x1b[") {
		t.Errorf("ANSI escapes should be stripped:\n%s", content)
	}
	if !strings.Contains(content, "colored text") {
		t.Errorf("expected stripped message in log:\n%s", content)
	}
	if !strings.Contains(content, "elapsed=123ms") || !strings.Contains(content, "exit=0") {
		t.Errorf("expected summary with elapsed and exit code:\n%s", content)
	}
	if !strings.Contains(content, "[DEBUG] +") {
		t.Errorf("expected elapsed-time prefix on lines:\n%s", content)
	}
}

func TestDebugLogRedactsSecretHeaders(t *testing.T) {
	dir := setupDebugLogTest(t)

	writeDebugLine("DEBUG", "GET /v1/price HTTP/1.1\r\nAuthorization: Bearer sk-abcdef123456\r\n"+
		"X-Api-Key: live-key-789\r\nCookie: session=abc123\r\nHost: api.example.com\r\n")
	writeDebugLine("DEBUG", "HTTP/1.1 200 OK\r\nSet-Cookie: sid=xyz789; HttpOnly\r\nContent-Type: application/json\r\n")
	writeDebugLine("debug", "authorization:basic dXNlcg==")

	content := readDebugLog(t, dir)

	for _, secret := range []string{"sk-abcdef123456", "live-key-789", "session=abc123", "sid=xyz789", "dXNlcg=="} {
		if strings.Contains(content, secret) {
			t.Errorf("secret %q must be redacted:\n%s", secret, content)
		}
	}
	for _, kept := range []string{"Authorization: <redacted>", "X-Api-Key: <redacted>", "Set-Cookie: <redacted>", "Host: api.example.com", "Content-Type: application/json"} {
		if !strings.Contains(content, kept) {
			t.Errorf("expected %q in log:\n%s", kept, content)
		}
	}
}

func TestDebugLogRedactsQueryCredentials(t *testing.T) {
	dir := setupDebugLogTest(t)

	writeDebugLine("DEBUG", "GET /v1/things?api_key=SECRET1&symbol=BTC&access_token=SECRET2 HTTP/1.1")

	content := readDebugLog(t, dir)

	if strings.Contains(content, "SECRET1") || strings.Contains(content, "SECRET2") {
		t.Errorf("query credentials must be redacted:\n%s", content)
	}
	if !strings.Contains(content, "api_key=***") || !strings.Contains(content, "access_token=***") {
		t.Errorf("expected redacted query params:\n%s", content)
	}
	if !strings.Contains(content, "symbol=BTC") {
		t.Errorf("non-sensitive query params should be preserved:\n%s", content)
	}
}

func TestDebugLogTruncatesOversizedEntries(t *testing.T) {
	dir := setupDebugLogTest(t)

	writeDebugLine("DEBUG", strings.Repeat("x", debugEntryMaxLen*2))

	content := readDebugLog(t, dir)

	if !strings.Contains(content, "[truncated") {
		t.Errorf("expected truncation marker for oversized entry")
	}
	if len(content) > debugEntryMaxLen*2 {
		t.Errorf("log should be smaller than the original oversized message, got %d bytes", len(content))
	}
}

func TestDebugLogSummarySanitizesError(t *testing.T) {
	dir := setupDebugLogTest(t)

	DebugLogSummary("market-price", 1, "request failed:\nAuthorization: Bearer sk-secret\r\nretry later", 5*time.Millisecond)

	content := readDebugLog(t, dir)

	if strings.Contains(content, "sk-secret") {
		t.Errorf("error message must be sanitized:\n%s", content)
	}
	if strings.Contains(content, "\nAuthorization") {
		t.Errorf("error message newlines should be flattened:\n%s", content)
	}
	if !strings.Contains(content, "exit=1") || !strings.Contains(content, "retry later") {
		t.Errorf("expected summary content:\n%s", content)
	}
}

func TestDebugLogDisabledWritesNothing(t *testing.T) {
	closeDebugLog() // ensure clean state
	oldVerbose := enableVerbose
	enableVerbose = true
	t.Cleanup(func() { enableVerbose = oldVerbose })
	oldAu := au
	if au == nil {
		au = aurora.NewAurora(false)
	}
	t.Cleanup(func() { au = oldAu })

	dir := t.TempDir()
	LogDebug("should not be written")
	DebugLogSummary("x", 0, "", time.Millisecond)

	if _, err := os.Stat(filepath.Join(dir, "debug.log")); !os.IsNotExist(err) {
		t.Errorf("debug.log should not be created when file logging is disabled")
	}
}

func TestDebugLogCommandRedactsSecrets(t *testing.T) {
	dir := setupDebugLogTest(t)

	DebugLogCommand([]string{"surf", "auth", "--api-key", "sk-secret-value", "--json"})
	DebugLogCommand([]string{"surf", "market-price", "--token=abc123", "--symbol", "BTC"})
	DebugLogCommand([]string{"surf", "market-price", "-H", "X-Api-Key: live-key", "-q", "api_key=sk-live"})

	content := readDebugLog(t, dir)

	for _, secret := range []string{"sk-secret-value", "abc123", "live-key", "sk-live"} {
		if strings.Contains(content, secret) {
			t.Errorf("secret %q must be redacted:\n%s", secret, content)
		}
	}
	for _, kept := range []string{"--api-key ***", "--token=***", "-H ***", "-q ***", "--symbol BTC"} {
		if !strings.Contains(content, kept) {
			t.Errorf("expected %q in header:\n%s", kept, content)
		}
	}
}

func TestDebugLogRotation(t *testing.T) {
	dir := t.TempDir()
	oldMax := debugLogMaxSize
	debugLogMaxSize = 64
	t.Cleanup(func() { debugLogMaxSize = oldMax })

	path := filepath.Join(dir, "debug.log")
	if err := os.WriteFile(path, []byte(strings.Repeat("x", 100)), 0600); err != nil {
		t.Fatal(err)
	}

	EnableDebugLog(dir, time.Now())
	t.Cleanup(closeDebugLog)

	st, err := os.Stat(path + ".1")
	if err != nil {
		t.Fatalf("expected rotated debug.log.1: %v", err)
	}
	if st.Size() != 100 {
		t.Errorf("rotated file should keep old content, got size %d", st.Size())
	}
	st, err = os.Stat(path)
	if err != nil {
		t.Fatalf("expected fresh debug.log: %v", err)
	}
	if st.Size() != 0 {
		t.Errorf("fresh debug.log should start empty, got size %d", st.Size())
	}
}

func TestRedactDebugArgs(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{[]string{"surf", "auth", "--api-key", "sk-x"}, []string{"surf", "auth", "--api-key", "***"}},
		{[]string{"surf", "--token=abc"}, []string{"surf", "--token=***"}},
		{[]string{"surf", "market-price", "--symbol", "BTC"}, []string{"surf", "market-price", "--symbol", "BTC"}},
		{[]string{"surf", "--api-key"}, []string{"surf", "--api-key"}},
		{[]string{"surf", "-H", "X-Api-Key: v"}, []string{"surf", "-H", "***"}},
		{[]string{"surf", "--rsh-query=api_key=x"}, []string{"surf", "--rsh-query=***"}},
		{[]string{"surf", "-q", "api_key=x"}, []string{"surf", "-q", "***"}},
	}
	for _, c := range cases {
		got := redactDebugArgs(c.in)
		if strings.Join(got, " ") != strings.Join(c.want, " ") {
			t.Errorf("redactDebugArgs(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}
