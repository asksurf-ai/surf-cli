package cli

import (
	"fmt"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/alecthomas/chroma/quick"
)

var enableVerbose bool
var enableQuiet bool

// EnableVerbose turns on debug-level logging (same as -d / --debug).
func EnableVerbose() {
	enableVerbose = true
}

// SetQuiet enables or disables quiet mode, suppressing non-error diagnostics.
func SetQuiet(q bool) {
	enableQuiet = q
}

// LogDebug logs a debug message to stderr when verbose mode is on, and to
// the debug log file when --debug enabled it. Cheap to call when both are
// off: the message is not formatted in that case.
func LogDebug(format string, values ...any) {
	if !DebugLogEnabled() && !enableVerbose {
		return
	}
	msg := fmt.Sprintf(format, values...)
	writeDebugLine("DEBUG", msg)
	if enableVerbose {
		fmt.Fprintf(Stderr, "%s %s\n", au.Index(243, "DEBUG:"), msg)
	}
}

// LogDebugRequest logs the request in a debug message if verbose output
// is enabled.
func LogDebugRequest(req *http.Request) {
	if enableVerbose {
		dumped, err := httputil.DumpRequest(req, true)
		if err != nil {
			return
		}

		if useColor {
			sb := &strings.Builder{}
			quick.Highlight(sb, string(dumped), "http", "terminal256", "cli-dark")
			dumped = []byte(sb.String())
		}

		LogDebug("Making request:\n%s", string(dumped))
	}
}

// LogDebugResponse logs the response in a debug message if verbose output
// is enabled.
func LogDebugResponse(start time.Time, resp *http.Response) {
	if enableVerbose {
		dumped, err := httputil.DumpResponse(resp, true)
		if err != nil {
			return
		}

		if useColor {
			sb := &strings.Builder{}
			quick.Highlight(sb, string(dumped), "http", "terminal256", "cli-dark")
			dumped = []byte(sb.String())
		}

		LogDebug("Got response from server in %s:\n%s", time.Since(start), string(dumped))
	}
}

// LogInfo logs an info message. Suppressed when --quiet is set. Also
// persisted to the debug log file when --debug enabled it.
func LogInfo(format string, values ...any) {
	writeDebugLine("INFO", fmt.Sprintf(format, values...))
	if enableQuiet {
		return
	}
	fmt.Fprintf(Stderr, "%s %s\n", au.Index(74, "INFO:"), fmt.Sprintf(format, values...))
}

// LogWarning logs a warning message. Suppressed when --quiet is set. Also
// persisted to the debug log file when --debug enabled it.
func LogWarning(format string, values ...any) {
	writeDebugLine("WARN", fmt.Sprintf(format, values...))
	if enableQuiet {
		return
	}
	fmt.Fprintf(Stderr, "%s %s\n", au.Index(222, "WARN:"), fmt.Sprintf(format, values...))
}

// LogError logs an error message.
func LogError(format string, values ...any) {
	// TODO: stack traces?
	// Also persisted to the debug log file when --debug enabled it.
	writeDebugLine("ERROR", fmt.Sprintf(format, values...))
	fmt.Fprintf(Stderr, "%s %s\n", au.BgIndex(204, "ERROR:").White().Bold(), fmt.Sprintf(format, values...))
}
