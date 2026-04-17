package cli

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/url"
	"strings"
)

// Error codes for CLI-synthesized error envelopes. These are emitted when
// the CLI itself couldn't reach a meaningful HTTP response from the server
// (network failure, DNS failure, local timeout, TLS error, etc.).
//
// Agents branch on `error.code` in the JSON error envelope. The codes here
// share the namespace with backend error codes (UNAUTHORIZED, RATE_LIMITED,
// etc.) but must not overlap. See CLI_DESIGN_PRINCIPLES.md §4.2 and §14.4.
const (
	// ErrCodeNetworkError covers TCP/DNS/TLS/connection failures — anything
	// that prevented the request from reaching the server or reading a
	// response. Agents should treat this like a 502/503: maybe retry with
	// backoff.
	ErrCodeNetworkError = "NETWORK_ERROR"

	// ErrCodeTimeout is raised when the CLI's own timeout (--rsh-timeout)
	// fired before a response arrived. Separate from server-side 408
	// (which still comes back as a normal API error with status 408).
	ErrCodeTimeout = "TIMEOUT"
)

// classifyTransportError inspects an error returned by the HTTP transport
// layer and decides whether it's a known transport failure that should be
// surfaced as a structured error envelope (stdout, exit 4) rather than as
// a raw panic (stderr, exit 1).
//
// Returns (code, message, true) for known transport errors, or ("", "", false)
// for everything else (which should fall through to the original panic path).
func classifyTransportError(err error) (code string, message string, ok bool) {
	if err == nil {
		return "", "", false
	}

	// Local timeout — matches both context.DeadlineExceeded and the
	// human-friendly wrapper from request.go ("Request timed out after X:").
	if errors.Is(err, context.DeadlineExceeded) {
		return ErrCodeTimeout, extractMessage(err), true
	}

	// Dial / TCP / DNS / TLS layer errors all classify as NETWORK_ERROR.
	var (
		netErr    net.Error
		dnsErr    *net.DNSError
		opErr     *net.OpError
		urlErr    *url.Error
		tlsRecErr tls.RecordHeaderError
	)
	switch {
	case errors.As(err, &dnsErr),
		errors.As(err, &opErr),
		errors.As(err, &tlsRecErr):
		return ErrCodeNetworkError, extractMessage(err), true
	case errors.As(err, &urlErr):
		// url.Error usually wraps the real transport error. Unwrap once
		// and re-classify; otherwise fall back to NETWORK_ERROR since
		// url.Error on a Get/Post almost always means transport failure.
		if inner := urlErr.Unwrap(); inner != nil {
			if c, m, ok := classifyTransportError(inner); ok {
				return c, m, true
			}
		}
		return ErrCodeNetworkError, extractMessage(err), true
	case errors.As(err, &netErr):
		if netErr.Timeout() {
			return ErrCodeTimeout, extractMessage(err), true
		}
		return ErrCodeNetworkError, extractMessage(err), true
	}

	return "", "", false
}

// extractMessage produces a concise user-facing message from a Go error.
// Strips noisy prefixes like `Get "https://…":` that don't help an agent
// make retry decisions.
func extractMessage(err error) string {
	msg := err.Error()

	// `Request timed out after Xms: Get "...": context deadline exceeded`
	// → `request timed out after Xms`. Keep the duration (it's useful)
	// but drop the URL and the Go stdlib cruft.
	if strings.HasPrefix(msg, "Request timed out after ") {
		if sep := strings.Index(msg, ": "); sep > 0 {
			return "request timed out after " + msg[len("Request timed out after "):sep]
		}
	}

	// Strip `Get "https://…": ` / `Post "…": ` etc. HTTP method prefixes.
	for _, method := range []string{"Get ", "Post ", "Put ", "Patch ", "Delete ", "Head ", "Options "} {
		if strings.HasPrefix(msg, method+"\"") {
			if sep := strings.Index(msg, "\": "); sep > 0 {
				msg = msg[sep+3:]
			}
			break
		}
	}

	return strings.TrimSpace(msg)
}

// newTransportErrorResponse builds a synthetic Response carrying a structured
// error envelope in its Body. Status 599 is a convention (not in RFC) that
// maps to exit 4 via GetExitCode() without colliding with any real HTTP
// status class.
func newTransportErrorResponse(code, message string) Response {
	return Response{
		Proto:  "HTTP/1.1",
		Status: 599,
		Headers: map[string]string{
			"Content-Type": "application/json",
		},
		Body: map[string]any{
			"error": map[string]any{
				"code":    code,
				"message": message,
			},
		},
	}
}
