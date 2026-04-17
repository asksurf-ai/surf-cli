package cli

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyTransportError(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantOK  bool
		wantCode string
	}{
		{
			name:   "nil",
			err:    nil,
			wantOK: false,
		},
		{
			name:     "context deadline exceeded",
			err:      context.DeadlineExceeded,
			wantOK:   true,
			wantCode: ErrCodeTimeout,
		},
		{
			name:     "wrapped context deadline",
			err:      fmt.Errorf("Request timed out after 1ms: %w", context.DeadlineExceeded),
			wantOK:   true,
			wantCode: ErrCodeTimeout,
		},
		{
			name:     "net.OpError connection refused",
			err:      &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			wantOK:   true,
			wantCode: ErrCodeNetworkError,
		},
		{
			name:     "net.DNSError",
			err:      &net.DNSError{Err: "no such host", Name: "example.invalid"},
			wantOK:   true,
			wantCode: ErrCodeNetworkError,
		},
		{
			name: "url.Error wrapping net.OpError",
			err: &url.Error{
				Op:  "Get",
				URL: "http://127.0.0.1:1/",
				Err: &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")},
			},
			wantOK:   true,
			wantCode: ErrCodeNetworkError,
		},
		{
			name: "url.Error wrapping context deadline",
			err: &url.Error{
				Op:  "Get",
				URL: "http://example.com/",
				Err: context.DeadlineExceeded,
			},
			wantOK:   true,
			wantCode: ErrCodeTimeout,
		},
		{
			name:   "random non-transport error",
			err:    errors.New("something went wrong"),
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _, ok := classifyTransportError(tt.err)
			assert.Equal(t, tt.wantOK, ok, "ok mismatch")
			if tt.wantOK {
				assert.Equal(t, tt.wantCode, code, "code mismatch")
			}
		})
	}
}

func TestExtractMessage(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "strip Get prefix",
			err:  &url.Error{Op: "Get", URL: "http://127.0.0.1:1/path", Err: errors.New("dial tcp 127.0.0.1:1: connect: connection refused")},
			want: "dial tcp 127.0.0.1:1: connect: connection refused",
		},
		{
			name: "strip Request timed out wrapper",
			err:  fmt.Errorf(`Request timed out after 1ms: Get "http://example.com/": context deadline exceeded`),
			want: "request timed out after 1ms",
		},
		{
			name: "no prefix to strip",
			err:  errors.New("dial tcp: lookup nonexistent: no such host"),
			want: "dial tcp: lookup nonexistent: no such host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractMessage(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestNewTransportErrorResponse(t *testing.T) {
	resp := newTransportErrorResponse(ErrCodeNetworkError, "connection refused")
	assert.Equal(t, 599, resp.Status)
	assert.Equal(t, "application/json", resp.Headers["Content-Type"])

	body, ok := resp.Body.(map[string]any)
	assert.True(t, ok, "Body must be a map")

	errObj, ok := body["error"].(map[string]any)
	assert.True(t, ok, "Body must have an 'error' object")
	assert.Equal(t, ErrCodeNetworkError, errObj["code"])
	assert.Equal(t, "connection refused", errObj["message"])
}
