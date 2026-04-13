package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStripSchemaBlocks(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantOut string
		// Substrings that must NOT appear in the output.
		mustNotContain []string
		// Substrings that MUST appear in the output.
		mustContain []string
	}{
		{
			name:    "empty",
			in:      "",
			wantOut: "",
		},
		{
			name:    "prose only",
			in:      "Returns historical price data.\n\nUse --time-range or --from/--to.",
			wantOut: "Returns historical price data.\n\nUse --time-range or --from/--to.",
		},
		{
			name: "strips option schema",
			in: strings.Join([]string{
				"Returns historical price data.",
				"",
				"## Option Schema:",
				"```schema",
				"{",
				"  --symbol: (string) Token symbol",
				"}",
				"```",
			}, "\n"),
			mustNotContain: []string{"Option Schema", "--symbol: (string)"},
			mustContain:    []string{"Returns historical price data."},
		},
		{
			name: "strips response blocks",
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
				"",
				"## Response default (application/json)",
				"",
				"Error",
				"",
				"```schema",
				"{ error*: {...} }",
				"```",
			}, "\n"),
			mustNotContain: []string{"Response 200", "Response default", "data*:", "error*:"},
			mustContain:    []string{"Token price history."},
		},
		{
			name: "strips argument schema",
			in: strings.Join([]string{
				"Get a single item.",
				"",
				"## Argument Schema:",
				"```schema",
				"{",
				"  item-id: (string) Item ID",
				"}",
				"```",
			}, "\n"),
			mustNotContain: []string{"Argument Schema", "item-id:"},
			mustContain:    []string{"Get a single item."},
		},
		{
			name: "keeps input example",
			in: strings.Join([]string{
				"Execute SQL.",
				"",
				"## Input Example",
				"",
				"```json",
				`{"sql": "SELECT 1"}`,
				"```",
				"",
				"## Response 200 (application/json)",
				"```schema",
				"{ data*: [...] }",
				"```",
			}, "\n"),
			mustNotContain: []string{"Response 200", "data*:"},
			mustContain:    []string{"Execute SQL.", "Input Example", `"sql": "SELECT 1"`},
		},
		{
			name: "keeps request schema (body fields not in Cobra Flags)",
			in: strings.Join([]string{
				"Execute SQL.",
				"",
				"## Request Schema (application/json)",
				"",
				"```schema",
				"{",
				"  sql*: (string) SQL query",
				"}",
				"```",
				"",
				"## Response 200 (application/json)",
				"```schema",
				"{ data*: [...] }",
				"```",
			}, "\n"),
			mustContain:    []string{"Request Schema", "sql*: (string) SQL query"},
			mustNotContain: []string{"Response 200", "data*:"},
		},
		{
			name: "keeps user-authored sections",
			in: strings.Join([]string{
				"SQL executor.",
				"",
				"## Rules",
				"- Only SELECT allowed",
				"- Max 10,000 rows",
				"",
				"## Example",
				"",
				"```sql",
				"SELECT 1",
				"```",
				"",
				"## Option Schema:",
				"```schema",
				"{ --sql: string }",
				"```",
			}, "\n"),
			mustContain:    []string{"## Rules", "Only SELECT allowed", "## Example", "SELECT 1"},
			mustNotContain: []string{"Option Schema", "--sql: string"},
		},
		{
			name: "interleaved sections — prose between schema blocks",
			in: strings.Join([]string{
				"Op description.",
				"",
				"## Option Schema:",
				"```schema",
				"{ --x: string }",
				"```",
				"",
				"## Rules",
				"Must use --x.",
				"",
				"## Response 200 (application/json)",
				"```schema",
				"{ data: [] }",
				"```",
			}, "\n"),
			mustContain:    []string{"Op description.", "## Rules", "Must use --x."},
			mustNotContain: []string{"Option Schema", "Response 200", "--x: string"},
		},
		{
			name: "code block containing a literal ## is not mistaken for a heading",
			in: strings.Join([]string{
				"Example.",
				"",
				"```bash",
				"## This is a bash comment, not a heading",
				"echo hi",
				"```",
				"",
				"## Option Schema:",
				"```schema",
				"{ --x: string }",
				"```",
			}, "\n"),
			mustContain:    []string{"## This is a bash comment", "echo hi"},
			mustNotContain: []string{"Option Schema", "--x: string"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripSchemaBlocks(tt.in)
			if tt.wantOut != "" {
				assert.Equal(t, tt.wantOut, got)
			}
			for _, s := range tt.mustContain {
				assert.Contains(t, got, s, "expected output to contain %q\n\noutput:\n%s", s, got)
			}
			for _, s := range tt.mustNotContain {
				assert.NotContains(t, got, s, "expected output NOT to contain %q\n\noutput:\n%s", s, got)
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
		{"## Response 200 (application/json)", true},
		{"## Response default (application/json)", true},
		{"## Responses 200/201 (application/json)", true},
		{"## Request Schema (application/json)", false}, // kept
		{"## Input Example", false},                     // kept
		{"## Rules", false},
		{"## Example", false},
		{"## Time range options", false},
	}
	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			assert.Equal(t, tt.want, isDroppedSchemaHeading(tt.line))
		})
	}
}
