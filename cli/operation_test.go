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
		Get("/prefix/id1").
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

	cmd := op.Command()

	viper.Reset()
	viper.Set("nocolor", true)
	viper.Set("tty", true)
	Init("test", "1.0.0")
	Defaults()
	capture := &strings.Builder{}
	Stdout = capture
	Stderr = capture
	cmd.SetOutput(Stdout)
	viper.Set("surf-api-base-url", "http://example2.com/prefix")
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

	cmd := op.Command()

	reset(false)
	capture := &strings.Builder{}
	Stdout = capture
	Stderr = capture

	// snake_case --time_range should be accepted and mapped to --time-range
	cmd.Flags().Parse([]string{"--time_range=1h"})
	err := cmd.RunE(cmd, []string{})

	assert.NoError(t, err)
	assert.Contains(t, capture.String(), "ok")
}

// TestOperationSnakeCaseFlagSilent verifies that snake_case flags are
// accepted silently — no deprecation warnings on stderr. Warnings were
// removed because pflag calls NormalizeFunc during flag registration,
// producing hundreds of false warnings at startup (217 lines in one batch).
func TestOperationSnakeCaseFlagSilent(t *testing.T) {
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
	cmd := op.Command()
	Root.AddCommand(cmd)
	defer Root.RemoveCommand(cmd)

	capture := &strings.Builder{}
	Stdout = capture
	Stderr = capture
	Root.SetOut(capture)
	Root.SetErr(capture)

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

	w.Close()
	stderrOut := <-done

	assert.NoError(t, err)
	assert.NotContains(t, stderrOut, "deprecated",
		"normalize should be silent, got stderr:\n%s", stderrOut)
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

	cmd := op.Command()

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

	cmd := op.Command()

	reset(false)
	capture := &strings.Builder{}
	Stdout = capture
	Stderr = capture

	// Provide the required flag (snake_case also tests #1 normalization)
	cmd.Flags().Parse([]string{"--market_ticker=BTC-USD"})
	err := cmd.RunE(cmd, []string{})

	assert.NoError(t, err)
	assert.Contains(t, capture.String(), "BTC-USD")
}

func TestOverrideServer_PathAligned(t *testing.T) {
	// spec already contains the /gateway/v1 prefix and env agrees → no duplication
	viper.Reset()
	viper.Set("surf-api-base-url", "https://api.stg.ask.surf/gateway/v1")
	got := overrideServer("https://api.asksurf.ai/gateway/v1/market/price")
	assert.Equal(t, "https://api.stg.ask.surf/gateway/v1/market/price", got)
}

func TestOverrideServer_HostOnly(t *testing.T) {
	// env only has scheme+host, spec carries /gateway/v1
	viper.Reset()
	viper.Set("surf-api-base-url", "https://api.stg.ask.surf")
	got := overrideServer("https://api.asksurf.ai/gateway/v1/market/price")
	assert.Equal(t, "https://api.stg.ask.surf/gateway/v1/market/price", got)
}

func TestOverrideServer_PathReplacesPrefix(t *testing.T) {
	// env path diverges from spec path → replace first N segments (N = env seg count)
	viper.Reset()
	viper.Set("surf-api-base-url", "https://custom.example.com/other/v2")
	got := overrideServer("https://api.asksurf.ai/gateway/v1/market/price")
	assert.Equal(t, "https://custom.example.com/other/v2/market/price", got)
}

func TestOverrideServer_Empty(t *testing.T) {
	// env not set → uri unchanged
	viper.Reset()
	viper.Set("surf-api-base-url", "")
	got := overrideServer("https://api.asksurf.ai/gateway/v1/market/price")
	assert.Equal(t, "https://api.asksurf.ai/gateway/v1/market/price", got)
}
