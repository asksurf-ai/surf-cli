package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitHelpBlocks(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantAbove   []string
		wantBelow   []string
		notAbove    []string
		notAnywhere []string
	}{
		{
			name:      "prose only stays above",
			in:        "Returns historical price data.\n\nUse --time-range or --from/--to.",
			wantAbove: []string{"Returns historical price data.", "Use --time-range"},
			wantBelow: nil,
		},
		{
			name: "option and argument schemas are dropped",
			in: strings.Join([]string{
				"Returns historical price data.",
				"",
				"## Option Schema:",
				"```schema",
				"{ --symbol: (string) Token symbol }",
				"```",
				"",
				"## Argument Schema:",
				"```schema",
				"{ item-id: (string) Item ID }",
				"```",
			}, "\n"),
			wantAbove:   []string{"Returns historical price data."},
			notAnywhere: []string{"Option Schema", "--symbol: (string)", "Argument Schema", "item-id:"},
		},
		{
			name: "success responses move below",
			in: strings.Join([]string{
				"Token price history.",
				"",
				"## Response 200 (application/json)",
				"",
				"OK",
				"",
				"```schema",
				"{ data*: [...] }",
				"```",
			}, "\n"),
			wantAbove: []string{"Token price history."},
			wantBelow: []string{"## Response 200", "data*: [...]"},
			notAbove:  []string{"## Response 200", "data*: [...]"},
		},
		{
			name: "error responses move below",
			in: strings.Join([]string{
				"Token price history.",
				"",
				"## Response default (application/json)",
				"",
				"Error",
				"",
				"```schema",
				"{ error*: {...} }",
				"```",
			}, "\n"),
			wantAbove: []string{"Token price history."},
			wantBelow: []string{"## Response default", "error*:"},
			notAbove:  []string{"## Response default", "error*:"},
		},
		{
			name: "merged success and error responses move below",
			in: strings.Join([]string{
				"SQL executor.",
				"",
				"## Responses 200/408 (application/json)",
				"",
				"```schema",
				"{ data*: [...] }",
				"```",
			}, "\n"),
			wantBelow: []string{"## Responses 200/408"},
			notAbove:  []string{"## Responses 200/408"},
		},
		{
			name:      "2XX range code moves below",
			in:        "Token price history.\n\n## Response 2XX (application/json)\n\n```schema\n{ data*: [...] }\n```",
			wantAbove: []string{"Token price history."},
			wantBelow: []string{"## Response 2XX", "data*: [...]"},
			notAbove:  []string{"## Response 2XX"},
		},
		{
			name:        "CRLF input is normalized",
			in:          "Token price history.\r\n\r\n## Response 200 (application/json)\r\n\r\nOK\r\n",
			wantAbove:   []string{"Token price history."},
			wantBelow:   []string{"## Response 200", "OK"},
			notAbove:    []string{"## Response 200"},
			notAnywhere: []string{"\r"},
		},
		{
			name: "input example and request schema stay above",
			in: strings.Join([]string{
				"Execute SQL.",
				"",
				"## Input Example",
				"",
				"```json",
				`{"sql": "SELECT 1"}`,
				"```",
				"",
				"## Request Schema (application/json)",
				"",
				"```schema",
				"{ sql*: (string) SQL query }",
				"```",
				"",
				"## Response 200 (application/json)",
				"```schema",
				"{ data*: [...] }",
				"```",
			}, "\n"),
			wantAbove: []string{"## Input Example", `"sql": "SELECT 1"`, "## Request Schema", "sql*: (string) SQL query"},
			wantBelow: []string{"## Response 200", "data*: [...]"},
		},
		{
			name: "user-authored sections stay above",
			in: strings.Join([]string{
				"SQL executor.",
				"",
				"## Rules",
				"- Only SELECT allowed",
				"",
				"## Example",
				"",
				"```sql",
				"SELECT 1",
				"```",
			}, "\n"),
			wantAbove: []string{"## Rules", "Only SELECT allowed", "## Example", "SELECT 1"},
			wantBelow: nil,
		},
		{
			name: "literal ## inside a fence is not a heading",
			in: strings.Join([]string{
				"Example.",
				"",
				"```bash",
				"## This is a bash comment, not a heading",
				"echo hi",
				"```",
			}, "\n"),
			wantAbove: []string{"## This is a bash comment", "echo hi"},
			wantBelow: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			above, below := splitHelpBlocks(tt.in)
			for _, s := range tt.wantAbove {
				assert.Contains(t, above, s, "expected above to contain %q\n\nabove:\n%s\n\nbelow:\n%s", s, above, below)
			}
			for _, s := range tt.wantBelow {
				assert.Contains(t, below, s, "expected below to contain %q\n\nabove:\n%s\n\nbelow:\n%s", s, above, below)
			}
			for _, s := range tt.notAbove {
				assert.NotContains(t, above, s, "did not expect above to contain %q\n\nabove:\n%s", s, above)
			}
			for _, s := range tt.notAnywhere {
				assert.NotContains(t, above+below, s, "did not expect output to contain %q", s)
			}
		})
	}
}

func TestIsDroppedSchemaHeading(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"## Option Schema:", true},
		{"## Option Schema", true},
		{"## Argument Schema:", true},
		{"## Response 200 (application/json)", false},
		{"## Response default (application/json)", false},
		{"## Request Schema (application/json)", false},
		{"## Input Example", false},
		{"## Rules", false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			assert.Equal(t, tt.want, isDroppedSchemaHeading(tt.line))
		})
	}
}

func TestIsResponseSchemaHeading(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"## Response 200 (application/json)", true},
		{"## Response 201", true},
		{"## Responses 200/201 (application/json)", true},
		{"## Response 204", true},
		{"## Response 408 (application/json)", true},
		{"## Response 429 (application/json)", true},
		{"## Response default (application/json)", true},
		{"## Responses 200/408 (application/json)", true},
		{"## Response", false},
		{"## Request Schema (application/json)", false},
		{"## Input Example", false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			assert.Equal(t, tt.want, isResponseSchemaHeading(tt.line))
		})
	}
}

// TestResponseBlocksOrderedSuccessFirst pins the order inside the response
// block: success codes, then error codes, then the shared default envelope,
// regardless of the order the spec emitted them in.
func TestResponseBlocksOrderedSuccessFirst(t *testing.T) {
	in := strings.Join([]string{
		"SQL executor.",
		"",
		"## Response default (application/json)",
		"",
		"Error",
		"",
		"## Response 408 (application/json)",
		"",
		"Timed out",
		"",
		"## Response 200 (application/json)",
		"",
		"OK",
		"",
		"## Response 202 (application/json)",
		"",
		"Accepted",
	}, "\n")

	above, below := splitHelpBlocks(in)
	assert.Contains(t, above, "SQL executor.")
	for _, s := range []string{"## Response 200", "## Response 202", "## Response 408", "## Response default"} {
		assert.NotContains(t, above, s)
		assert.Contains(t, below, s)
	}

	order := []string{"## Response 200", "## Response 202", "## Response 408", "## Response default"}
	last := -1
	for _, s := range order {
		pos := strings.Index(below, s)
		if pos < last {
			t.Fatalf("response blocks out of order at %q:\n%s", s, below)
		}
		last = pos
	}
}
