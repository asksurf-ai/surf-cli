package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// debugLogMaxSize is the size at which debug.log is rotated to debug.log.1
// when a new --debug session starts.
var debugLogMaxSize int64 = 5 << 20 // 5 MiB

// debugEntryMaxLen caps a single log entry. HTTP dumps include full bodies,
// so without a cap one large response could blow past debugLogMaxSize within
// a single session — the rotation check only runs at EnableDebugLog.
const debugEntryMaxLen = 64 << 10 // 64 KiB

var (
	debugLogMu   sync.Mutex
	debugLogFile *os.File
	debugStart   time.Time
)

var (
	ansiPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")
	// secretHeaderPattern matches dump lines for headers that can carry
	// credentials: Authorization/Proxy-Authorization, Cookie/Set-Cookie, and
	// any header whose name contains key/token/secret (X-Api-Key, X-Auth-Token,
	// ...). Profile headers, -H flags, and external auth tools can all put
	// secrets in arbitrary header names, so a name heuristic is required.
	secretHeaderPattern = regexp.MustCompile(`(?i)((?:proxy-authorization|authorization|cookie|set-cookie|[a-z0-9_-]*(?:key|token|secret)[a-z0-9_-]*)\s*:\s*)[^\r\n]+`)
	// secretQueryPattern redacts the values of query parameters whose name
	// names a credential, e.g. ?api_key=... or &access_token=... in dumped
	// request lines and logged URLs.
	secretQueryPattern = regexp.MustCompile(`(?i)([?&][^\s&#]*(?:key|token|secret|password)[^\s&#]*=)[^\s&#]+`)
)

const debugTimeFormat = "2006-01-02T15:04:05.000Z07:00"

// EnableDebugLog starts appending debug output to <dir>/debug.log. If the
// existing log exceeds debugLogMaxSize it is rotated to debug.log.1 first.
// start anchors the "+elapsed" prefix on each line; pass the time the
// process started. Failures are silent — debug logging is best-effort.
//
// Note: two concurrent --debug processes race on rotation (last renamer
// wins on debug.log.1, and a process holding the old fd keeps appending to
// the renamed inode). Acceptable for a human-driven debug log.
func EnableDebugLog(dir string, start time.Time) {
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if debugLogFile != nil {
		debugLogFile.Close()
		debugLogFile = nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return
	}
	path := filepath.Join(dir, "debug.log")
	if st, err := os.Stat(path); err == nil && st.Size() > debugLogMaxSize {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	// O_CREATE mode only applies to new files; tighten a pre-existing one.
	_ = f.Chmod(0600)
	debugLogFile = f
	debugStart = start
}

// DebugLogEnabled reports whether file-based debug logging is active.
func DebugLogEnabled() bool {
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	return debugLogFile != nil
}

func closeDebugLog() {
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if debugLogFile != nil {
		debugLogFile.Close()
		debugLogFile = nil
	}
}

// sanitizeDebugLine strips ANSI color escapes and redacts secrets from a
// log message before it is persisted: credential-carrying HTTP headers,
// and query-string parameters whose name looks like a key/token/secret.
func sanitizeDebugLine(msg string) string {
	msg = ansiPattern.ReplaceAllString(msg, "")
	msg = secretHeaderPattern.ReplaceAllString(msg, "${1}<redacted>")
	msg = secretQueryPattern.ReplaceAllString(msg, "${1}***")
	if len(msg) > debugEntryMaxLen {
		end := debugEntryMaxLen
		// Don't split a multi-byte UTF-8 rune at the cut point.
		for end > 0 && msg[end]&0xC0 == 0x80 {
			end--
		}
		msg = fmt.Sprintf("%s\n... [truncated %d bytes]", msg[:end], len(msg)-end)
	}
	return msg
}

// writeDebugLine appends one sanitized line to the debug log file.
// No-op when file logging is not enabled.
func writeDebugLine(level, msg string) {
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if debugLogFile == nil {
		return
	}
	elapsed := time.Since(debugStart).Round(time.Millisecond)
	fmt.Fprintf(debugLogFile, "%s [%-5s] +%-13s%s\n",
		time.Now().Format(debugTimeFormat), level, elapsed, sanitizeDebugLine(msg))
}

// sensitiveFlagName reports whether a CLI flag (e.g. "--api-key") carries a
// secret value that must not be written to the debug log. Includes the
// restish -H/--rsh-header and -q/--rsh-query flags, whose values commonly
// embed credentials even though the flag name doesn't say so.
func sensitiveFlagName(arg string) bool {
	n := strings.ToLower(strings.TrimLeft(arg, "-"))
	switch n {
	case "h", "rsh-header", "q", "rsh-query":
		return true
	}
	return strings.Contains(n, "key") || strings.Contains(n, "token") ||
		strings.Contains(n, "secret") || strings.Contains(n, "password")
}

// redactDebugArgs replaces the values of sensitive flags with "***".
func redactDebugArgs(args []string) []string {
	out := make([]string, 0, len(args))
	redactNext := false
	for _, a := range args {
		if redactNext {
			out = append(out, "***")
			redactNext = false
			continue
		}
		name, hasValue := a, false
		if idx := strings.Index(a, "="); idx >= 0 {
			name, hasValue = a[:idx], true
		}
		if strings.HasPrefix(a, "-") && sensitiveFlagName(name) {
			if hasValue {
				out = append(out, name+"=***")
			} else {
				out = append(out, a)
				redactNext = true
			}
			continue
		}
		out = append(out, a)
	}
	return out
}

// DebugLogCommand writes a header marking the start of one command
// invocation. Pass the args as typed by the user (before any rewriting),
// with args[0] already replaced by the program name — this string is
// persisted verbatim otherwise.
func DebugLogCommand(args []string) {
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if debugLogFile == nil {
		return
	}
	fmt.Fprintf(debugLogFile, "\n=== %s %s ===\n",
		time.Now().Format(debugTimeFormat), sanitizeDebugLine(strings.Join(redactDebugArgs(args), " ")))
}

// DebugLogSummary writes a footer with the command name, total wall-clock
// elapsed time, and exit code for the invocation.
//
// The footer is best-effort: code paths that call os.Exit directly
// (interactive auth prompts, panics) skip it.
func DebugLogSummary(command string, exitCode int, errMsg string, elapsed time.Duration) {
	debugLogMu.Lock()
	defer debugLogMu.Unlock()
	if debugLogFile == nil {
		return
	}
	line := fmt.Sprintf("=== done: command=%s elapsed=%s exit=%d",
		command, elapsed.Round(time.Millisecond), exitCode)
	if errMsg != "" {
		// Sanitize before flattening so header redaction stops at the
		// original line boundaries instead of eating the rest of the error.
		errMsg = sanitizeDebugLine(errMsg)
		errMsg = strings.Map(func(r rune) rune {
			if r == '\n' || r == '\r' {
				return ' '
			}
			return r
		}, errMsg)
		line += " error=" + errMsg
	}
	fmt.Fprintln(debugLogFile, line+" ===")
}
