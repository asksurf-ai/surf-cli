package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cyberconnecthq/surf-cli/cli"
	"github.com/spf13/viper"
	"github.com/zalando/go-keyring"
)

// --- pickRecentUserAssistantLines -----------------------------------------

func TestPickRecentUserAssistantLines(t *testing.T) {
	line := func(typ, id string) string {
		b, _ := json.Marshal(map[string]string{"type": typ, "uuid": id})
		return string(b)
	}

	tests := []struct {
		name    string
		data    string
		n       int
		wantIDs []string // uuids in order
	}{
		{
			name:    "empty input yields no lines",
			data:    "",
			n:       10,
			wantIDs: nil,
		},
		{
			name: "only user/assistant are kept, others dropped",
			data: strings.Join([]string{
				line("system", "s1"),
				line("user", "u1"),
				line("tool_result", "tr1"),
				line("assistant", "a1"),
				line("summary", "sum1"),
			}, "\n"),
			n:       10,
			wantIDs: []string{"u1", "a1"},
		},
		{
			name: "exceeding n keeps the tail in order",
			data: strings.Join([]string{
				line("user", "u1"),
				line("assistant", "a1"),
				line("user", "u2"),
				line("assistant", "a2"),
			}, "\n"),
			n:       3,
			wantIDs: []string{"a1", "u2", "a2"},
		},
		{
			name: "malformed json lines are skipped silently",
			data: strings.Join([]string{
				"not json",
				line("user", "u1"),
				`{"type": "user", "uuid":`, // truncated
				line("assistant", "a1"),
			}, "\n"),
			n:       10,
			wantIDs: []string{"u1", "a1"},
		},
		{
			name: "blank lines are skipped",
			data: "\n\n" + line("user", "u1") + "\n\n" + line("assistant", "a1") + "\n",
			n:       10,
			wantIDs: []string{"u1", "a1"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pickRecentUserAssistantLines([]byte(tc.data), tc.n)
			if len(got) != len(tc.wantIDs) {
				t.Fatalf("got %d lines, want %d; got=%v", len(got), len(tc.wantIDs), got)
			}
			for i, line := range got {
				var e struct {
					UUID string `json:"uuid"`
				}
				if err := json.Unmarshal([]byte(line), &e); err != nil {
					t.Fatalf("line %d is not valid JSON: %v", i, err)
				}
				if e.UUID != tc.wantIDs[i] {
					t.Errorf("line %d uuid = %q, want %q", i, e.UUID, tc.wantIDs[i])
				}
			}
		})
	}
}

// --- readTail -------------------------------------------------------------

func TestReadTail_SmallerThanCap(t *testing.T) {
	f := filepath.Join(t.TempDir(), "small.jsonl")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readTail(f, 1<<20)
	if err != nil {
		t.Fatalf("readTail: %v", err)
	}
	if string(got) != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestReadTail_LargerThanCapReturnsTailBytes(t *testing.T) {
	f := filepath.Join(t.TempDir(), "big.jsonl")
	// 1000 bytes total; cap to last 100.
	content := strings.Repeat("a", 900) + strings.Repeat("b", 100)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := readTail(f, 100)
	if err != nil {
		t.Fatalf("readTail: %v", err)
	}
	if len(got) != 100 {
		t.Errorf("got %d bytes, want 100", len(got))
	}
	if string(got) != strings.Repeat("b", 100) {
		t.Errorf("expected the tail 100 bytes, got %q", got[:20])
	}
}

func TestReadTail_MissingFileReturnsError(t *testing.T) {
	_, err := readTail(filepath.Join(t.TempDir(), "nope.jsonl"), 1<<10)
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// --- loadRecentClaudeMessages --------------------------------------------

// setupSessionFile writes a jsonl file under <base>/projects/<encoded-cwd>/
// and returns (base, cwd, fullPath). It sets the file's mtime to mtime.
func setupSessionFile(t *testing.T, base, cwd, fname, contents string, mtime time.Time) string {
	t.Helper()
	encoded := strings.ReplaceAll(cwd, "/", "-")
	projectDir := filepath.Join(base, "projects", encoded)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(projectDir, fname)
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadRecentClaudeMessages_NoClaudecode(t *testing.T) {
	t.Setenv("CLAUDECODE", "")
	data, n, note := loadRecentClaudeMessages(t.TempDir(), "/fake/cwd", 10)
	if data != "" || n != 0 || note != "" {
		t.Errorf("expected silent no-op when CLAUDECODE unset; got data=%q n=%d note=%q", data, n, note)
	}
}

func TestLoadRecentClaudeMessages_ProjectDirMissing(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(home, ".claude"))
	data, n, note := loadRecentClaudeMessages(home, "/nonexistent/project", 10)
	if data != "" || n != 0 {
		t.Errorf("expected empty data; got data len %d n %d", len(data), n)
	}
	if !strings.Contains(note, "no Claude session found") {
		t.Errorf("expected discovery note, got %q", note)
	}
}

func TestLoadRecentClaudeMessages_PicksNewestJsonl(t *testing.T) {
	t.Setenv("CLAUDECODE", "1")
	base := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", base)
	cwd := "/Users/me/my-project"

	old := `{"type":"user","uuid":"old-u"}`
	newer := `{"type":"user","uuid":"new-u"}
{"type":"assistant","uuid":"new-a"}`

	setupSessionFile(t, base, cwd, "old.jsonl", old, time.Now().Add(-2*time.Hour))
	setupSessionFile(t, base, cwd, "new.jsonl", newer, time.Now())

	data, n, note := loadRecentClaudeMessages("/unused-home", cwd, 10)
	if note != "" {
		t.Errorf("unexpected discovery note: %q", note)
	}
	if n != 2 {
		t.Errorf("expected 2 messages from newest file, got %d (%q)", n, data)
	}
	if !strings.Contains(data, "new-u") || !strings.Contains(data, "new-a") {
		t.Errorf("expected newest file's contents, got %q", data)
	}
	if strings.Contains(data, "old-u") {
		t.Errorf("should not have included older session file, got %q", data)
	}
}

// --- postFeedback ---------------------------------------------------------

// resetAuthSources makes GetAPIKey()'s three lookups (env / keychain / cache)
// start from a clean slate for each test: mock keychain, fresh viper, and
// initialize cli.Cache (GetAPIKey panics on a nil Cache).
func resetAuthSources(t *testing.T) {
	t.Helper()
	keyring.MockInit()
	viper.Reset()
	t.Setenv("SURF_API_KEY", "")
	t.Setenv("SURF_CONFIG_DIR", t.TempDir())
	cli.Init("surf-test", "test")
}

func TestPostFeedback_Success(t *testing.T) {
	var seen struct {
		method string
		path   string
		auth   string
		ct     string
		body   feedbackRecord
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.method = r.Method
		seen.path = r.URL.Path
		seen.auth = r.Header.Get("Authorization")
		seen.ct = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&seen.body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	resetAuthSources(t)
	viper.Set("surf-api-base-url", srv.URL)
	t.Setenv("SURF_API_KEY", "sk-test-abc")

	rec := feedbackRecord{SessionID: "sess-1", Feedback: "hello", SessionData: "raw"}
	if err := postFeedback(context.Background(), rec); err != nil {
		t.Fatalf("postFeedback returned err: %v", err)
	}

	if seen.method != http.MethodPost {
		t.Errorf("method = %q, want POST", seen.method)
	}
	if seen.path != "/cli/feedback" {
		t.Errorf("path = %q, want /cli/feedback", seen.path)
	}
	if seen.auth != "Bearer sk-test-abc" {
		t.Errorf("Authorization = %q, want Bearer sk-test-abc", seen.auth)
	}
	if seen.ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", seen.ct)
	}
	if seen.body != rec {
		t.Errorf("body = %+v, want %+v", seen.body, rec)
	}
}

func TestPostFeedback_AnonymousNoAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	resetAuthSources(t)
	viper.Set("surf-api-base-url", srv.URL)

	if err := postFeedback(context.Background(), feedbackRecord{Feedback: "hi"}); err != nil {
		t.Fatalf("postFeedback returned err: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("expected no Authorization header for anonymous submit, got %q", gotAuth)
	}
}

func TestPostFeedback_NonSuccessReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"bad"}`)
	}))
	defer srv.Close()

	resetAuthSources(t)
	viper.Set("surf-api-base-url", srv.URL)

	err := postFeedback(context.Background(), feedbackRecord{Feedback: "hi"})
	if err == nil {
		t.Fatal("expected error on 400 response, got nil")
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Errorf("error should mention status code, got %v", err)
	}
	if !strings.Contains(err.Error(), `{"error":"bad"}`) {
		t.Errorf("error should include response body, got %v", err)
	}
}

func TestPostFeedback_MissingBaseURL(t *testing.T) {
	resetAuthSources(t)
	viper.Set("surf-api-base-url", "") // cli.Init registers a default; clear it explicitly.
	err := postFeedback(context.Background(), feedbackRecord{Feedback: "hi"})
	if err == nil || !strings.Contains(err.Error(), "surf-api-base-url") {
		t.Errorf("expected missing base URL error, got %v", err)
	}
}
