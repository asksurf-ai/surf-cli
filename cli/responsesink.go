package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// responseSinkEnv names a directory where surf mirrors each command's parsed
// response as a JSON file. Unset (the default for interactive / CI use)
// disables the feature entirely. Intended for machine consumers — pipelines,
// auditing, and debugging output transformations — that need the structured
// response independent of the format selected on the command line.
const responseSinkEnv = "SURF_RESPONSE_SINK_DIR"

// responseRecord is the machine-readable mirror of the response surf emits for
// a single command — a parsed HTTP response, or (on network / TLS / timeout
// failure) the synthesized transport-error envelope. It captures the structured
// body BEFORE output formatting, so a consumer gets the complete response
// regardless of the human / JSON / gron format the caller selected and
// regardless of any downstream reshaping of stdout (e.g. piping through jq).
type responseRecord struct {
	V         int    `json:"v"`
	Operation string `json:"operation"` // the surf subcommand, e.g. "market-ranking"
	Method    string `json:"method"`
	Path      string `json:"path"`
	Query     string `json:"query,omitempty"`
	Status    int    `json:"status"`
	Body      any    `json:"body"` // full, pagination-merged structured response body
}

// mirrorResponse writes a responseRecord to the directory named by
// $SURF_RESPONSE_SINK_DIR, as an atomic per-invocation file. It is a no-op when
// the env var is unset.
//
// Best-effort by contract: every failure is swallowed (logged at debug) so the
// mirror can never change the command's stdout, exit code, or timing. The file
// is published via temp-write + rename so a concurrent reader never observes a
// partial write; the nanosecond+pid filename keeps multiple surf calls in one
// shell command from colliding and preserves their order for the reader.
func mirrorResponse(req *http.Request, resp Response) {
	dir := os.Getenv(responseSinkEnv)
	if dir == "" {
		return
	}
	// A panic here (e.g. an un-marshalable body) must never escape into the
	// command path.
	defer func() { _ = recover() }()

	data, err := json.Marshal(responseRecord{
		V:         1,
		Operation: currentCommand,
		Method:    req.Method,
		Path:      req.URL.Path,
		Query:     req.URL.RawQuery,
		Status:    resp.Status,
		Body:      resp.Body,
	})
	if err != nil {
		LogDebug("response sink: marshal failed: %v", err)
		return
	}

	base := strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.Itoa(os.Getpid())
	tmp, err := os.CreateTemp(dir, base+"-*.tmp")
	if err != nil {
		LogDebug("response sink: create temp failed: %v", err)
		return
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		LogDebug("response sink: write failed: %v", err)
		return
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		LogDebug("response sink: close failed: %v", err)
		return
	}
	if err := os.Rename(tmpName, filepath.Join(dir, base+".json")); err != nil {
		_ = os.Remove(tmpName)
		LogDebug("response sink: rename failed: %v", err)
	}
}
