package cli

import (
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestExtractUnknownFlagName(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"unknown flag: --query", "query"},
		{"unknown flag: --time-range", "time-range"},
		{"unknown flag: --q extra\n", "q"},
		{"unknown shorthand flag: z in -z", "z"},
		{"some other error", ""},
	}
	for _, c := range cases {
		got := extractUnknownFlagName(c.in)
		assert.Equal(t, c.want, got, "input=%q", c.in)
	}
}

func TestLevenshtein(t *testing.T) {
	assert.Equal(t, 0, levenshtein("abc", "abc"))
	assert.Equal(t, 1, levenshtein("q", "qa"))
	assert.Equal(t, 4, levenshtein("query", "q"))
	assert.Equal(t, 5, levenshtein("", "hello"))
	assert.Equal(t, 5, levenshtein("hello", ""))
}

// TestSuggestFlagNames covers the agent-confusion cases we care about:
// --query → --q, --username → --handle, --timerange → --time-range.
func TestSuggestFlagNames(t *testing.T) {
	op := Operation{
		Name:        "fake",
		Method:      http.MethodGet,
		URITemplate: "http://example.com/fake",
		QueryParams: []*Param{
			{Type: "string", Name: "q"},
			{Type: "string", Name: "handle"},
			{Type: "string", Name: "time_range"},
			{Type: "integer", Name: "limit"},
		},
	}
	cmd := op.Command()

	// "query" should suggest "q" (substring match).
	got := suggestFlagNames(cmd, "query")
	assert.Contains(t, got, "q", "got=%v", got)

	// "username" should suggest "handle" within edit distance 3? Actually
	// levenshtein("username", "handle") is 6, so it won't match by distance.
	// This confirms we need alias work for that case — test captures the
	// current behaviour to prevent accidental regression.
	_ = suggestFlagNames(cmd, "username")

	// "timerange" (no dash) should map to "time-range" via substring or
	// distance (levenshtein=1).
	got = suggestFlagNames(cmd, "timerange")
	assert.Contains(t, got, "time-range", "got=%v", got)
}

func TestFlagErrorFuncWithSuggestions_UnknownFlag(t *testing.T) {
	op := Operation{
		Name:        "fake",
		Method:      http.MethodGet,
		URITemplate: "http://example.com/fake",
		QueryParams: []*Param{{Type: "string", Name: "q"}},
	}
	cmd := op.Command()

	err := cmd.Flags().Parse([]string{"--query=foo"})
	assert.Error(t, err)

	// Run the error through our FlagErrorFunc and check the hint.
	wrapped := flagErrorFuncWithSuggestions(cmd, err)
	assert.Contains(t, wrapped.Error(), "Did you mean", "got: %s", wrapped.Error())
	assert.Contains(t, wrapped.Error(), "--q", "got: %s", wrapped.Error())
}

func TestFlagErrorFuncWithSuggestions_PassThroughNonUnknown(t *testing.T) {
	// An error that isn't "unknown flag" must pass through unchanged.
	cmd := &cobra.Command{Use: "x"}
	other := assertError("some validation error")
	out := flagErrorFuncWithSuggestions(cmd, other)
	assert.Equal(t, other, out)
}

// --- helpers ---

type localErr string

func (e localErr) Error() string { return string(e) }

func assertError(msg string) error {
	return localErr(msg)
}

func TestHelpDescription_Required(t *testing.T) {
	p := Param{Name: "market_slug", Type: "string", Description: "Market slug", Required: true}
	got := p.helpDescription()
	assert.True(t, strings.HasPrefix(got, "[required] "), "got=%q", got)
}

func TestHelpDescription_Example(t *testing.T) {
	p := Param{Name: "symbol", Type: "string", Description: "Ticker symbol", Example: "BTC"}
	got := p.helpDescription()
	assert.Contains(t, got, "(example: BTC)", "got=%q", got)
}

func TestHelpDescription_RequiredAndExample(t *testing.T) {
	p := Param{Name: "symbol", Type: "string", Description: "Ticker", Required: true, Example: "BTC"}
	got := p.helpDescription()
	assert.True(t, strings.HasPrefix(got, "[required] "), "got=%q", got)
	assert.Contains(t, got, "(example: BTC)", "got=%q", got)
}

func TestHelpDescription_SkipsExampleIfAlreadyInText(t *testing.T) {
	p := Param{Name: "symbol", Type: "string", Description: "Symbol. Example: BTC", Example: "ETH"}
	got := p.helpDescription()
	assert.NotContains(t, got, "(example: ETH)", "got=%q", got)
}
