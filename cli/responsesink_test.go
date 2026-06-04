package cli

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
)

func TestMirrorResponseWritesRecord(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(responseSinkEnv, dir)

	// currentCommand is a package global set by the operation runner before the
	// request; emulate that and restore it after.
	prev := currentCommand
	currentCommand = "market-ranking"
	t.Cleanup(func() { currentCommand = prev })

	req := httptest.NewRequest("GET", "/gateway/market/ranking?limit=2&sort=mcap", nil)
	resp := Response{
		Status: 200,
		Body: map[string]any{
			"data": []any{
				map[string]any{"id": "abc", "symbol": "BTC"},
			},
		},
	}

	mirrorResponse(req, resp)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one mirror file should be written")
	// Published atomically: the final name is .json, no leftover .tmp.
	assert.Equal(t, ".json", filepath.Ext(entries[0].Name()))

	raw, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	require.NoError(t, err)

	var rec responseRecord
	require.NoError(t, json.Unmarshal(raw, &rec))
	assert.Equal(t, 1, rec.V)
	assert.Equal(t, "market-ranking", rec.Operation)
	assert.Equal(t, "GET", rec.Method)
	assert.Equal(t, "/gateway/market/ranking", rec.Path)
	assert.Equal(t, "limit=2&sort=mcap", rec.Query)
	assert.Equal(t, 200, rec.Status)

	// The mirror must carry the full structured envelope (so a consumer can read
	// `data[].id` etc.), NOT formatted text.
	body, ok := rec.Body.(map[string]any)
	require.True(t, ok)
	data, ok := body["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	row, ok := data[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "abc", row["id"])
	assert.Equal(t, "BTC", row["symbol"])
}

func TestMirrorResponseNoopWhenUnset(t *testing.T) {
	t.Setenv(responseSinkEnv, "") // empty == disabled
	req := httptest.NewRequest("GET", "/x", nil)
	// Must neither panic nor touch the filesystem when the feature is off.
	assert.NotPanics(t, func() {
		mirrorResponse(req, Response{Status: 200, Body: map[string]any{"a": 1}})
	})
}

// readSingleMirror asserts exactly one mirror file was published and returns it.
func readSingleMirror(t *testing.T, dir string) responseRecord {
	t.Helper()
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "exactly one mirror file expected")
	require.Equal(t, ".json", filepath.Ext(entries[0].Name()))
	raw, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	require.NoError(t, err)
	var rec responseRecord
	require.NoError(t, json.Unmarshal(raw, &rec))
	return rec
}

// End-to-end: a parsed HTTP response flowing through MakeRequestAndFormat is
// mirrored (catches placement regressions in the success branch).
func TestMakeRequestAndFormatMirrorsParsedResponse(t *testing.T) {
	defer gock.Off()
	reset(false)
	dir := t.TempDir()
	t.Setenv(responseSinkEnv, dir)
	prev := currentCommand
	currentCommand = "market-ranking"
	t.Cleanup(func() { currentCommand = prev })

	gock.New("http://example.com").Get("/foo").Reply(200).JSON(map[string]any{
		"data": []any{map[string]any{"id": "abc", "symbol": "BTC"}},
	})

	req, err := http.NewRequest("GET", "http://example.com/foo", nil)
	require.NoError(t, err)
	MakeRequestAndFormat(req)

	rec := readSingleMirror(t, dir)
	assert.Equal(t, "market-ranking", rec.Operation)
	assert.Equal(t, 200, rec.Status)
	body, ok := rec.Body.(map[string]any)
	require.True(t, ok)
	data, ok := body["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	row, ok := data[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "BTC", row["symbol"])
}

// End-to-end: a transport failure synthesizes an error envelope on stdout — it
// must be mirrored too, so the sink reflects every response surf formats (this
// is the branch that used to `return` before mirroring).
func TestMakeRequestAndFormatMirrorsTransportError(t *testing.T) {
	defer gock.Off()
	reset(false)
	dir := t.TempDir()
	t.Setenv(responseSinkEnv, dir)
	prev := currentCommand
	currentCommand = "market-ranking"
	t.Cleanup(func() { currentCommand = prev })

	gock.New("http://example.com").
		Get("/foo").
		ReplyError(&net.OpError{Op: "dial", Err: errors.New("connection refused")})

	req, err := http.NewRequest("GET", "http://example.com/foo", nil)
	require.NoError(t, err)
	MakeRequestAndFormat(req)

	rec := readSingleMirror(t, dir)
	assert.Equal(t, 599, rec.Status)
	body, ok := rec.Body.(map[string]any)
	require.True(t, ok)
	errObj, ok := body["error"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, ErrCodeNetworkError, errObj["code"])
}
