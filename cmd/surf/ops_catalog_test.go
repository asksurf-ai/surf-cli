package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/asksurf-ai/surf-cli/cli"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func catalogFixture() []cli.Operation {
	return []cli.Operation{
		{
			Name:   "news-feed",
			Method: "GET",
			Group:  "news",
			Short:  "Crypto news",
			QueryParams: []*cli.Param{
				{Name: "sort_by", Type: "string", Enum: []string{"recency", "trending"}, Default: "recency",
					Example: "recency", Description: "Sort order"},
				{Name: "order", Type: "string", Description: "Sort by recency order", Example: "recency"},
				{
					Name:        "limit",
					Type:        "integer",
					Minimum:     fptr(1),
					Maximum:     fptr(50),
					Default:     float64(20),
					Description: "Results per page",
				},
			},
		},
		{
			Name:   "search-token",
			Method: "GET",
			Short:  "Search tokens by symbol",
			QueryParams: []*cli.Param{
				{Name: "q", Type: "string", Required: true, Example: "uniswap", Description: "Query"},
			},
		},
		{
			Name:        "job-get",
			Method:      "GET",
			Short:       "Get a job",
			URITemplate: "https://example.com/jobs/{job_id}",
			PathParams: []*cli.Param{
				{Name: "job_id", Type: "string", Required: true, Example: "job-123", Description: "Job identifier"},
			},
		},
		{
			Name:          "submit",
			Method:        "POST",
			Short:         "Submit a query",
			BodyMediaType: "application/json",
			BodyRequired:  true,
		},
		{
			Name:       "retired",
			Method:     "GET",
			Deprecated: "do not use",
		},
		{
			Name:   "internal",
			Method: "GET",
			Hidden: true,
		},
	}
}

func fptr(f float64) *float64 { return &f }

func decodeCatalog(t *testing.T, ops []cli.Operation, compact bool) []map[string]any {
	t.Helper()
	var buf bytes.Buffer
	old := cli.Stdout
	cli.Stdout = &buf
	t.Cleanup(func() { cli.Stdout = old })

	require.NoError(t, printOperationsJSON(ops, compact))
	var out []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &out))
	return out
}

func TestOperationsJSONIncludesEnumsAndBounds(t *testing.T) {
	out := decodeCatalog(t, catalogFixture(), false)
	require.Len(t, out, 4)

	// Hidden and deprecated operations must not be advertised.
	for _, op := range out {
		assert.NotEqual(t, "retired", op["name"])
		assert.NotEqual(t, "internal", op["name"])
	}

	sortBy := flagByName(t, out[0]["optional_flags"], "sort-by")
	assert.Equal(t, []any{"recency", "trending"}, sortBy["enum"])

	limit := flagByName(t, out[0]["optional_flags"], "limit")
	assert.Equal(t, float64(1), limit["min"])
	assert.Equal(t, float64(50), limit["max"])
	assert.Equal(t, float64(20), limit["default"])

	required := out[1]["required_flags"].([]any)
	require.Len(t, required, 1)
	assert.Equal(t, "q", required[0].(map[string]any)["name"])
	assert.Equal(t, "uniswap", required[0].(map[string]any)["example"],
		"an example that adds information should be kept")

	// An example equal to the default is noise and must be dropped.
	redundantSort := flagByName(t, out[0]["optional_flags"], "sort-by")
	assert.NotContains(t, redundantSort, "example")
}

// TestOperationsJSONPathParamsArePositional guards against advertising path
// parameters as flags: `surf job-get --job-id 1` does not exist.
func TestOperationsJSONPathParamsArePositional(t *testing.T) {
	out := decodeCatalog(t, catalogFixture(), false)
	job := out[2]

	assert.NotContains(t, job, "path_flags")

	args, ok := job["positional_args"].([]any)
	require.True(t, ok, "positional args should be listed separately: %v", job)
	require.Len(t, args, 1)

	arg := args[0].(map[string]any)
	assert.Equal(t, "job_id", arg["name"])
	assert.Equal(t, float64(1), arg["position"])
	assert.Equal(t, true, arg["required"])
	assert.Equal(t, "job-123", arg["example"], "path parameter examples should survive")
}

// TestOperationsJSONBodyMetadata covers body-type operations: an agent must be
// able to tell that a body is required before calling.
func TestOperationsJSONBodyMetadata(t *testing.T) {
	out := decodeCatalog(t, catalogFixture(), false)
	submit := out[3]

	assert.Equal(t, true, submit["has_body"])
	assert.Equal(t, true, submit["body_required"])
	assert.Equal(t, "application/json", submit["body_media_type"])
}

func TestOperationsJSONCompactDropsDescriptions(t *testing.T) {
	full := decodeCatalog(t, catalogFixture(), false)
	compact := decodeCatalog(t, catalogFixture(), true)

	assert.Contains(t, full[0], "short")
	assert.NotContains(t, compact[0], "short", "compact drops the operation summary")

	fullFlag := flagByName(t, full[0]["optional_flags"], "limit")
	assert.Contains(t, fullFlag, "description")
	assert.Equal(t, []any{"recency", "trending"},
		flagByName(t, full[0]["optional_flags"], "sort-by")["enum"],
		"enum survives in full mode")

	compactFlag := flagByName(t, compact[0]["optional_flags"], "limit")
	assert.NotContains(t, compactFlag, "description")
	assert.Equal(t, float64(50), compactFlag["max"], "bounds survive compaction")

	// An example that only repeats the description is dropped when the
	// description is shown, but must reappear in compact output where it is not.
	assert.NotContains(t, flagByName(t, full[0]["optional_flags"], "order"), "example")
	assert.Equal(t, "recency", flagByName(t, compact[0]["optional_flags"], "order")["example"])
	assert.Equal(t, []any{"recency", "trending"},
		flagByName(t, compact[0]["optional_flags"], "sort-by")["enum"],
		"enum survives compaction")
}

func TestOperationsJSONCompactIsSmaller(t *testing.T) {
	var full, compact bytes.Buffer
	old := cli.Stdout
	t.Cleanup(func() { cli.Stdout = old })

	cli.Stdout = &full
	require.NoError(t, printOperationsJSON(catalogFixture(), false))
	cli.Stdout = &compact
	require.NoError(t, printOperationsJSON(catalogFixture(), true))

	assert.Less(t, compact.Len(), full.Len())
	assert.Contains(t, compact.String(), `"enum"`)
	// Compact output is meant to be pasted into a prompt, so it carries no
	// pretty-printing whitespace.
	assert.NotContains(t, compact.String(), "\n  ")
	assert.Equal(t, 1, strings.Count(compact.String(), "\n"))
}

func flagByName(t *testing.T, flags any, name string) map[string]any {
	t.Helper()
	list, ok := flags.([]any)
	require.True(t, ok, "flags should be an array")
	for _, f := range list {
		m := f.(map[string]any)
		if m["name"] == name {
			return m
		}
	}
	t.Fatalf("flag %q not found", name)
	return nil
}

// TestOperationsIndexIsCompact pins the discovery payload: names, groups and
// summaries only, no flag tables or descriptions. It is the form an agent can
// afford to load before choosing a command.
func TestOperationsIndexIsCompact(t *testing.T) {
	var buf bytes.Buffer
	old := cli.Stdout
	cli.Stdout = &buf
	t.Cleanup(func() { cli.Stdout = old })

	require.NoError(t, printOperationsIndex(catalogFixture()))

	var got []map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &got))
	require.Len(t, got, 4) // deprecated and hidden entries are excluded

	assert.Equal(t, "news-feed", got[0]["name"])
	assert.Equal(t, "news", got[0]["group"])
	assert.Equal(t, "Crypto news", got[0]["short"])
	for _, entry := range got {
		assert.NotContains(t, entry, "optional_flags")
		assert.NotContains(t, entry, "description")
	}

	// The whole fixture index stays well under the full catalog's size.
	assert.Less(t, buf.Len(), 1200, "index should be a discovery-sized payload:\n%s", buf.String())
}
