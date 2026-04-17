package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/asksurf-ai/surf-cli/cli"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const feedbackRequestTimeout = 10 * time.Second

// feedbackRecord is the POST body for hermod's /gateway/v1/cli/feedback.
// session_data is raw jsonl lines (last N user/assistant entries, newline-
// joined, verbatim from the Claude Code session file); user_id /
// tenant_id / ip / created_at are filled server-side from the auth context.
type feedbackRecord struct {
	SessionID   string `json:"session_id"`
	Feedback    string `json:"feedback"`
	SessionData string `json:"session_data"`
}

func newFeedbackCmd() *cobra.Command {
	var quiet bool
	cmd := &cobra.Command{
		Use:   "feedback <message>",
		Short: "Send feedback about the Surf CLI",
		Long: `Send feedback about the Surf CLI.

When invoked inside a Claude Code session, the last 10 user/assistant
turns of that conversation are attached for context.`,
		Example: `  surf feedback "search-project ignored --q"
  surf feedback "great job" --quiet`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runFeedback(cmd.Context(), args[0], quiet)
		},
	}
	cmd.Flags().BoolVar(&quiet, "quiet", false, "Suppress confirmation output")
	return cmd
}

func runFeedback(ctx context.Context, message string, quiet bool) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("cannot determine home directory: %w", err)
	}
	cwd, _ := os.Getwd()

	sessionData, count, discoverNote := loadRecentClaudeMessages(home, cwd, 10)
	if discoverNote != "" && !quiet {
		fmt.Fprintln(os.Stderr, discoverNote)
	}

	record := feedbackRecord{
		SessionID:   cli.GetSessionID(configDir),
		Feedback:    message,
		SessionData: sessionData,
	}

	if err := postFeedback(ctx, record); err != nil {
		return err
	}

	if !quiet {
		fmt.Fprintf(os.Stderr, "Feedback submitted (attached %d message(s)).\n", count)
	}
	return nil
}

// postFeedback sends the feedback record to hermod's /cli/feedback endpoint.
// Anonymous submissions are allowed; the Authorization header is only set
// when an API key is configured.
func postFeedback(ctx context.Context, record feedbackRecord) error {
	baseURL := viper.GetString("surf-api-base-url")
	if baseURL == "" {
		return fmt.Errorf("surf-api-base-url is not configured")
	}

	payload, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("encode payload: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, feedbackRequestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost,
		strings.TrimRight(baseURL, "/")+"/cli/feedback", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if apiKey := cli.GetAPIKey(); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("submit feedback: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return fmt.Errorf("feedback endpoint returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

// loadRecentClaudeMessages returns the raw jsonl for the last n user/assistant
// entries in the current Claude Code session (newline-joined, verbatim lines).
// Best-effort: on any discovery failure it returns "" plus a human-readable
// note for stderr.
func loadRecentClaudeMessages(home, cwd string, n int) (sessionData string, count int, note string) {
	if os.Getenv("CLAUDECODE") != "1" {
		return "", 0, ""
	}

	base := os.Getenv("CLAUDE_CONFIG_DIR")
	if base == "" {
		base = filepath.Join(home, ".claude")
	}
	if cwd == "" {
		return "", 0, "feedback: could not determine working directory — attaching no messages"
	}

	encoded := strings.ReplaceAll(cwd, "/", "-")
	projectDir := filepath.Join(base, "projects", encoded)

	matches, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
	if err != nil || len(matches) == 0 {
		return "", 0, fmt.Sprintf("feedback: no Claude session found at %s — attaching no messages", projectDir)
	}

	type fileInfo struct {
		path  string
		mtime time.Time
	}
	var files []fileInfo
	for _, p := range matches {
		info, err := os.Stat(p)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		files = append(files, fileInfo{p, info.ModTime()})
	}
	if len(files) == 0 {
		return "", 0, "feedback: no regular session files found — attaching no messages"
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.After(files[j].mtime) })
	sessionFile := files[0].path

	// Cap the read so a multi-MB session file doesn't balloon memory. Newer
	// entries live at the tail, which is what we need — but going line-by-line
	// from the end is fiddly for jsonl. A 2 MiB window is enough for ~10
	// recent turns in practice; if more is truncated we simply pick from the
	// tail of what we read.
	const maxRead = 2 << 20
	data, err := readTail(sessionFile, maxRead)
	if err != nil {
		return "", 0, fmt.Sprintf("feedback: could not read session file %s: %v", sessionFile, err)
	}

	lines := pickRecentUserAssistantLines(data, n)
	return strings.Join(lines, "\n"), len(lines), ""
}

// readTail returns up to maxBytes from the end of the file, so jsonl
// sessions that grew to tens of MB don't get fully loaded into memory.
// The returned slice starts at a byte offset — the first (partial) line is
// dropped by the caller because pickRecentUserAssistantLines ignores any
// line that fails JSON unmarshal.
func readTail(path string, maxBytes int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()
	offset := int64(0)
	if size > maxBytes {
		offset = size - maxBytes
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(f)
}

// pickRecentUserAssistantLines returns the last n jsonl lines whose type is
// "user" or "assistant", preserving their original text verbatim.
func pickRecentUserAssistantLines(data []byte, n int) []string {
	type typeProbe struct {
		Type string `json:"type"`
	}
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var t typeProbe
		if err := json.Unmarshal([]byte(line), &t); err != nil {
			continue
		}
		if t.Type != "user" && t.Type != "assistant" {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) > n {
		kept = kept[len(kept)-n:]
	}
	return kept
}
