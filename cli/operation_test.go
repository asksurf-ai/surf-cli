package cli

import (
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestOperation(t *testing.T) {
	defer gock.Off()

	gock.
		New("http://example2.com").
		Get("/prefix/test/id1").
		MatchParam("search", "foo").
		MatchParam("def3", "abc").
		MatchHeader("Accept", "application/json").
		Reply(200).
		JSON(map[string]any{
			"hello": "world",
		})

	op := Operation{
		Name:        "test",
		Short:       "short",
		Long:        "long",
		Method:      http.MethodGet,
		URITemplate: "http://example.com/test/{id}",
		PathParams: []*Param{
			{
				Type:        "string",
				Name:        "id",
				DisplayName: "id",
				Description: "desc",
			},
		},
		QueryParams: []*Param{
			{
				Type:        "string",
				Name:        "search",
				DisplayName: "search",
				Description: "desc",
			},
			{
				Type:        "string",
				Name:        "def",
				DisplayName: "def",
				Description: "desc",
			},
			{
				Type:        "string",
				Name:        "def2",
				DisplayName: "def2",
				Description: "desc",
				Default:     "",
			},
			{
				Type:        "string",
				Name:        "def3",
				DisplayName: "def3",
				Description: "desc",
				Default:     "abc",
			},
		},
		HeaderParams: []*Param{
			{
				Type:        "string",
				Name:        "Accept",
				DisplayName: "Accept",
				Description: "desc",
				Default:     "application/json",
			},
			{
				Type:        "string",
				Name:        "Accept-Encoding",
				DisplayName: "Accept-Encoding",
				Description: "desc",
				Default:     "gz",
			},
		},
	}

	cmd := op.command()

	viper.Reset()
	viper.Set("nocolor", true)
	viper.Set("tty", true)
	Init("test", "1.0.0")
	Defaults()
	capture := &strings.Builder{}
	Stdout = capture
	Stderr = capture
	cmd.SetOutput(Stdout)
	viper.Set("rsh-server", "http://example2.com/prefix")
	cmd.Flags().Parse([]string{"--search=foo", "--def-3=abc", "--accept=application/json"})
	err := cmd.RunE(cmd, []string{"id1"})

	assert.NoError(t, err)
	assert.Equal(t, "HTTP/1.1 200 OK\nContent-Type: application/json\n\n{\n  hello: \"world\"\n}\n", capture.String())
}

func TestOperationSnakeCaseFlag(t *testing.T) {
	defer gock.Off()

	gock.
		New("http://example.com").
		Get("/test").
		MatchParam("time_range", "1h").
		Reply(200).
		JSON(map[string]any{"ok": true})

	op := Operation{
		Name:        "test",
		Method:      http.MethodGet,
		URITemplate: "http://example.com/test",
		QueryParams: []*Param{
			{
				Type: "string",
				Name: "time_range",
			},
		},
	}

	cmd := op.command()

	viper.Reset()
	viper.Set("nocolor", true)
	viper.Set("tty", true)
	Init("test", "1.0.0")
	Defaults()
	capture := &strings.Builder{}
	Stdout = capture
	Stderr = capture

	// snake_case --time_range should be accepted and mapped to --time-range
	cmd.Flags().Parse([]string{"--time_range=1h"})
	err := cmd.RunE(cmd, []string{})

	assert.NoError(t, err)
	assert.Contains(t, capture.String(), "ok")
}

// TestOperationSnakeCaseFlagWarningOnce verifies that the deprecation warning
// for snake_case flag aliases is printed exactly once per command invocation,
// even when the command is dispatched through Root.Execute() (which is what
// happens for real users). Previously the normalize function ran multiple
// times during cobra's parsing pipeline, producing duplicate warnings like:
//
//	$ surf social-ranking --time_range 24h
//	Warning: flag --time_range is deprecated, use --time-range instead
//	Warning: flag --time_range is deprecated, use --time-range instead
//	Warning: flag --time_range is deprecated, use --time-range instead
//	Warning: flag --time_range is deprecated, use --time-range instead
//	Warning: flag --time_range is deprecated, use --time-range instead
func TestOperationSnakeCaseFlagWarningOnce(t *testing.T) {
	defer gock.Off()

	gock.
		New("http://example.com").
		Get("/social-ranking").
		MatchParam("time_range", "24h").
		Reply(200).
		JSON(map[string]any{"ok": true})

	op := Operation{
		Name:        "social-ranking",
		Method:      http.MethodGet,
		URITemplate: "http://example.com/social-ranking",
		QueryParams: []*Param{
			{
				Type: "string",
				Name: "time_range",
			},
		},
	}

	reset(false)
	cmd := op.command()
	Root.AddCommand(cmd)
	defer Root.RemoveCommand(cmd)

	// Capture stdout/err for the response body and any cobra-side output.
	capture := &strings.Builder{}
	Stdout = capture
	Stderr = capture
	Root.SetOut(capture)
	Root.SetErr(capture)

	// NormalizeSnakeCaseFlags writes to os.Stderr directly, so redirect the
	// real file descriptor through a pipe to capture its output.
	origStderr := os.Stderr
	r, w, err := os.Pipe()
	assert.NoError(t, err)
	os.Stderr = w
	defer func() { os.Stderr = origStderr }()

	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	Root.SetArgs([]string{"social-ranking", "--time_range", "24h"})
	err = Root.Execute()

	// Close writer so the reader goroutine returns.
	w.Close()
	stderrOut := <-done

	assert.NoError(t, err)

	count := strings.Count(stderrOut, "is deprecated")
	assert.Equal(t, 1, count,
		"expected exactly one deprecation warning per invocation, got %d. Stderr:\n%s",
		count, stderrOut)
}

func TestOperationMissingRequiredFlag(t *testing.T) {
	op := Operation{
		Name:        "kalshi-markets",
		Method:      http.MethodGet,
		URITemplate: "http://example.com/markets",
		QueryParams: []*Param{
			{
				Type:     "string",
				Name:     "market_ticker",
				Required: true,
			},
			{
				Type: "string",
				Name: "status",
			},
		},
	}

	cmd := op.command()

	viper.Reset()
	viper.Set("nocolor", true)
	viper.Set("tty", true)
	Init("test", "1.0.0")
	Defaults()

	// Don't pass the required --market-ticker flag
	cmd.Flags().Parse([]string{"--status=open"})
	err := cmd.RunE(cmd, []string{})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "--market-ticker")
	assert.Contains(t, err.Error(), "missing required flag")
}

func TestOperationRequiredFlagProvided(t *testing.T) {
	defer gock.Off()

	gock.
		New("http://example.com").
		Get("/markets").
		MatchParam("market_ticker", "BTC-USD").
		Reply(200).
		JSON(map[string]any{"ticker": "BTC-USD"})

	op := Operation{
		Name:        "kalshi-markets",
		Method:      http.MethodGet,
		URITemplate: "http://example.com/markets",
		QueryParams: []*Param{
			{
				Type:     "string",
				Name:     "market_ticker",
				Required: true,
			},
		},
	}

	cmd := op.command()

	viper.Reset()
	viper.Set("nocolor", true)
	viper.Set("tty", true)
	Init("test", "1.0.0")
	Defaults()
	capture := &strings.Builder{}
	Stdout = capture
	Stderr = capture

	// Provide the required flag (snake_case also tests #1 normalization)
	cmd.Flags().Parse([]string{"--market_ticker=BTC-USD"})
	err := cmd.RunE(cmd, []string{})

	assert.NoError(t, err)
	assert.Contains(t, capture.String(), "BTC-USD")
}
