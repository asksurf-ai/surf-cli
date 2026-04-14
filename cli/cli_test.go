package cli

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/zalando/go-keyring"
	"gopkg.in/h2non/gock.v1"
)

func reset(color bool) {
	viper.Reset()
	viper.Set("tty", true)
	if color {
		viper.Set("color", true)
	} else {
		viper.Set("nocolor", true)
	}

	// Most tests are easier to write without retries.
	viper.Set("rsh-retry", 0)

	Init("test", "1.0.0'")
	Defaults()

	// Clear SURF_API_BASE_URL override so gock-mocked URIs aren't silently
	// rewritten by overrideServer. Tests needing the override can re-set it.
	viper.Set("surf-api-base-url", "")
}

func run(cmd string, color ...bool) string {
	if len(color) == 0 || !color[0] {
		reset(false)
	} else {
		reset(true)
	}

	return runNoReset(cmd)
}

func runNoReset(cmd string) string {
	capture := &strings.Builder{}
	Stdout = capture
	Stderr = capture
	Root.SetOut(capture)
	os.Args = strings.Split("restish "+cmd, " ")
	Run()

	return capture.String()
}

func expectJSON(t *testing.T, cmd string, expected string) {
	captured := run("-o json -f body " + cmd)
	assert.JSONEq(t, expected, captured)
}

func expectExitCode(t *testing.T, expected int) {
	assert.Equal(t, expected, GetExitCode())
}

func TestGetURI(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").Get("/foo").Reply(200).JSON(map[string]any{
		"Hello": "World",
	})

	expectJSON(t, "http://example.com/foo", `{
		"Hello": "World"
	}`)
	expectExitCode(t, 0)
}

func TestPostURI(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").Post("/foo").Reply(200).JSON(map[string]any{
		"id":    1,
		"value": 123,
	})

	expectJSON(t, "post http://example.com/foo value: 123", `{
		"id": 1,
		"value": 123
	}`)
}

func TestPutURI400(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").Put("/foo/1").Reply(422).JSON(map[string]any{
		"detail": "Invalid input",
	})

	expectJSON(t, "put http://example.com/foo/1 value: 123", `{
		"detail": "Invalid input"
	}`)
	expectExitCode(t, 4)
}

func TestIgnoreStatusCodeExit(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").Put("/foo/1").Reply(400).JSON(map[string]any{
		"detail": "Invalid input",
	})

	expectJSON(t, "put http://example.com/foo/1 value: 123 --rsh-ignore-status-code", `{
		"detail": "Invalid input"
	}`)
	expectExitCode(t, 0)
}

func TestHeaderWithComma(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").Get("/").MatchHeader("Foo", "a,b,c").Reply(204)

	out := run("http://example.com/ -H Foo:a,b,c")
	assert.Contains(t, out, "204 No Content")
}

type TestAuth struct{}

// Parameters returns a list of OAuth2 Authorization Code inputs.
func (h *TestAuth) Parameters() []AuthParam {
	return []AuthParam{}
}

// OnRequest gets run before the request goes out on the wire.
func (h *TestAuth) OnRequest(request *http.Request, key string, params map[string]string) error {
	request.Header.Set("Authorization", "abc123")
	return nil
}

func TestAuthHeader(t *testing.T) {
	reset(false)

	AddAuth("test-auth", &TestAuth{})

	configs["test-auth-header"] = &APIConfig{
		name: "test-auth-header",
		Base: "https://auth-header-test.example.com",
		Profiles: map[string]*APIProfile{
			"default": {
				Auth: &APIAuth{
					Name: "test-auth",
				},
			},
			"no-auth": {},
		},
	}

	captured := runNoReset("auth-header bad-api")
	assert.Contains(t, captured, "no matched API")

	captured = runNoReset("auth-header test-auth-header")
	assert.Equal(t, "abc123\n", captured)

	captured = runNoReset("auth-header test-auth-header -p bad")
	assert.Contains(t, captured, "invalid profile bad")

	captured = runNoReset("auth-header test-auth-header -p no-auth")
	assert.Contains(t, captured, "no auth set up")
}

func TestAPIKeyAuthFromCache(t *testing.T) {
	reset(false)
	t.Setenv("SURF_API_KEY", "")

	Cache.Set("surf-test:default.api_key", "cached-key-123")
	Cache.WriteConfig()
	defer func() {
		Cache.Set("surf-test:default.api_key", "")
		Cache.WriteConfig()
	}()

	req, _ := http.NewRequest(http.MethodGet, "https://example.com/foo", nil)
	auth := &APIKeyAuth{}
	err := auth.OnRequest(req, "surf-test:default", nil)
	assert.NoError(t, err)
	assert.Equal(t, "Bearer cached-key-123", req.Header.Get("Authorization"))
}

func TestAPIKeyAuthEnvOverridesCache(t *testing.T) {
	reset(false)
	t.Setenv("SURF_API_KEY", "env-key-456")

	Cache.Set("surf-test:default.api_key", "cached-key-123")
	Cache.WriteConfig()
	defer func() {
		Cache.Set("surf-test:default.api_key", "")
		Cache.WriteConfig()
	}()

	req, _ := http.NewRequest(http.MethodGet, "https://example.com/foo", nil)
	auth := &APIKeyAuth{}
	err := auth.OnRequest(req, "surf-test:default", nil)
	assert.NoError(t, err)
	assert.Equal(t, "Bearer env-key-456", req.Header.Get("Authorization"))
}

func TestAPIKeyAuthNoKey(t *testing.T) {
	reset(false)
	t.Setenv("SURF_API_KEY", "")
	keyring.MockInit()

	req, _ := http.NewRequest(http.MethodGet, "https://example.com/foo", nil)
	auth := &APIKeyAuth{}
	err := auth.OnRequest(req, "nonexistent:default", nil)
	assert.NoError(t, err)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestAPIKeyAuthFromKeychain(t *testing.T) {
	reset(false)
	t.Setenv("SURF_API_KEY", "")
	keyring.MockInit()
	keyring.Set(KeyringService, "surf-test:default", "keychain-key-789")

	req, _ := http.NewRequest(http.MethodGet, "https://example.com/foo", nil)
	auth := &APIKeyAuth{}
	err := auth.OnRequest(req, "surf-test:default", nil)
	assert.NoError(t, err)
	assert.Equal(t, "Bearer keychain-key-789", req.Header.Get("Authorization"))
}

func TestAPIKeyAuthKeychainOverridesFile(t *testing.T) {
	reset(false)
	t.Setenv("SURF_API_KEY", "")
	keyring.MockInit()
	keyring.Set(KeyringService, "surf-test:default", "keychain-key-789")

	Cache.Set("surf-test:default.api_key", "file-key-123")
	Cache.WriteConfig()
	defer func() {
		Cache.Set("surf-test:default.api_key", "")
		Cache.WriteConfig()
	}()

	req, _ := http.NewRequest(http.MethodGet, "https://example.com/foo", nil)
	auth := &APIKeyAuth{}
	err := auth.OnRequest(req, "surf-test:default", nil)
	assert.NoError(t, err)
	assert.Equal(t, "Bearer keychain-key-789", req.Header.Get("Authorization"))
}

func TestAPIKeyAuthEnvOverridesKeychain(t *testing.T) {
	reset(false)
	t.Setenv("SURF_API_KEY", "env-key-456")
	keyring.MockInit()
	keyring.Set(KeyringService, "surf-test:default", "keychain-key-789")

	req, _ := http.NewRequest(http.MethodGet, "https://example.com/foo", nil)
	auth := &APIKeyAuth{}
	err := auth.OnRequest(req, "surf-test:default", nil)
	assert.NoError(t, err)
	assert.Equal(t, "Bearer env-key-456", req.Header.Get("Authorization"))
}

func TestAPIKeyAuthFallsBackToFileWhenKeychainEmpty(t *testing.T) {
	reset(false)
	t.Setenv("SURF_API_KEY", "")
	keyring.MockInit() // empty keychain

	Cache.Set("surf-test:default.api_key", "file-key-123")
	Cache.WriteConfig()
	defer func() {
		Cache.Set("surf-test:default.api_key", "")
		Cache.WriteConfig()
	}()

	req, _ := http.NewRequest(http.MethodGet, "https://example.com/foo", nil)
	auth := &APIKeyAuth{}
	err := auth.OnRequest(req, "surf-test:default", nil)
	assert.NoError(t, err)
	assert.Equal(t, "Bearer file-key-123", req.Header.Get("Authorization"))
}

// Integration tests: verify auth flows through MakeRequest to the wire.

func TestMakeRequestAuthFromEnv(t *testing.T) {
	defer gock.Off()
	reset(false)
	t.Setenv("SURF_API_KEY", "env-integration-key")

	gock.New("http://example.com").
		Get("/test-auth").
		MatchHeader("Authorization", "Bearer env-integration-key").
		Reply(200).
		JSON(map[string]any{"ok": true})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test-auth", nil)
	resp, err := MakeRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, gock.IsDone(), "expected Authorization header was not sent")
}

func TestMakeRequestAuthFromKeychain(t *testing.T) {
	defer gock.Off()
	reset(false)
	t.Setenv("SURF_API_KEY", "")
	keyring.MockInit()
	keyring.Set(KeyringService, "surf:default", "keychain-integration-key")

	gock.New("http://example.com").
		Get("/test-auth").
		MatchHeader("Authorization", "Bearer keychain-integration-key").
		Reply(200).
		JSON(map[string]any{"ok": true})

	viper.Set("api-name", "surf")
	configs["surf"] = &APIConfig{
		name: "surf",
		Base: "http://example.com",
		Profiles: map[string]*APIProfile{
			"default": {},
		},
	}
	defer delete(configs, "surf")

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test-auth", nil)
	resp, err := MakeRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, gock.IsDone(), "expected Authorization header was not sent")
}

func TestMakeRequestNoAuth(t *testing.T) {
	defer gock.Off()
	reset(false)
	t.Setenv("SURF_API_KEY", "")
	keyring.MockInit()

	gock.New("http://example.com").
		Get("/test-no-auth").
		Reply(200).
		JSON(map[string]any{"ok": true})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test-no-auth", nil)
	resp, err := MakeRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.Empty(t, req.Header.Get("Authorization"))
}

func TestMakeRequestEnvOverridesKeychainIntegration(t *testing.T) {
	defer gock.Off()
	reset(false)
	t.Setenv("SURF_API_KEY", "env-wins")
	keyring.MockInit()
	keyring.Set(KeyringService, "surf:default", "keychain-loses")

	gock.New("http://example.com").
		Get("/test-priority").
		MatchHeader("Authorization", "Bearer env-wins").
		Reply(200).
		JSON(map[string]any{"ok": true})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test-priority", nil)
	resp, err := MakeRequest(req)
	assert.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)
	assert.True(t, gock.IsDone(), "env var should take precedence over keychain")
}

func TestLinks(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").Get("/foo").Reply(204).SetHeader("Link", "</bar>; rel=\"item\"")

	captured := run("links http://example.com/foo")
	assert.JSONEq(t, `{
		"item": [
			{
				"rel": "item",
				"uri": "http://example.com/bar"
			}
		]
	}`, captured)
}

func TestDefaultOutput(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").Get("/foo").Reply(200).JSON(map[string]any{
		"hello": "world",
	})

	captured := run("http://example.com/foo", true)
	assert.Equal(t, "\x1b[38;5;204mHTTP\x1b[0m/\x1b[38;5;172m1.1\x1b[0m \x1b[38;5;172m200\x1b[0m \x1b[38;5;74mOK\x1b[0m\n\x1b[38;5;74mContent-Type\x1b[0m: application/json\n\n\x1b[38;5;172m{\x1b[0m\n  \x1b[38;5;74mhello\x1b[0m\x1b[38;5;247m:\x1b[0m \x1b[38;5;150m\"world\"\x1b[0m\x1b[38;5;247m\n\x1b[0m\x1b[38;5;172m}\x1b[0m\n", captured)
}

func TestHelp(t *testing.T) {
	captured := run("--help", false)
	assert.Contains(t, captured, "api")
	assert.Contains(t, captured, "get")
	assert.Contains(t, captured, "put")
	assert.Contains(t, captured, "delete")
	assert.Contains(t, captured, "edit")
}

func TestHelpHighlight(t *testing.T) {
	captured := run("--help", true)
	assert.Contains(t, captured, "api")
	assert.Contains(t, captured, "get")
	assert.Contains(t, captured, "put")
	assert.Contains(t, captured, "delete")
	assert.Contains(t, captured, "edit")
}

func TestLoadCache(t *testing.T) {
	// Invalidate any existin cache.
	Cache.Set("cache-test.expires", time.Now().Add(-24*time.Hour))
	Cache.WriteConfig()
	defer gock.Off()

	// Only *one* set of remote requests should be made. After that it should be
	// using the cache.
	gock.New("https://example.com/").Reply(404)
	gock.New("https://example.com/openapi.json").Reply(200).JSON(map[string]any{
		"openapi": "3.0.0",
	})

	reset(false)
	configs["cache-test"] = &APIConfig{
		name: "cache-test",
		Base: "https://example.com",
		Profiles: map[string]*APIProfile{
			"default": {},
		},
	}
	cmd := &cobra.Command{
		Use: "cache-test",
	}
	Root.AddCommand(cmd)

	AddLoader(&testLoader{
		API: API{
			Short:      "Cache Test API",
			Operations: []Operation{},
		},
	})

	// First run will generate the cache.
	runNoReset("cache-test --help")

	// These runs should *not* make any remote requests. If they do, then
	// gock will panic as only one call is mocked above.
	runNoReset("cache-test --help")
	runNoReset("cache-test --help")
}

func TestAPISync(t *testing.T) {
	defer gock.Off()

	gock.New("https://sync-test.example.com/").Reply(404)
	gock.New("https://sync-test.example.com/openapi.json").Reply(404)

	reset(false)

	configs["sync-test"] = &APIConfig{
		name: "sync-test",
		Base: "https://sync-test.example.com",
		Profiles: map[string]*APIProfile{
			"default": {},
		},
	}

	runNoReset("api sync sync-test")
}

func TestDuplicateAPIBase(t *testing.T) {
	defer func() {
		os.Remove(filepath.Join(getConfigDir("test"), "apis.json"))
		reset(false)
	}()
	reset(false)

	configs["dupe1"] = &APIConfig{
		name: "dupe1",
		Base: "https://dupe.example.com",
		Profiles: map[string]*APIProfile{
			"default": {},
		},
	}
	configs["dupe2"] = &APIConfig{
		name: "dupe2",
		Base: "https://dupe.example.com",
		Profiles: map[string]*APIProfile{
			"default": {},
		},
	}

	configs["dupe1"].Save()
	configs["dupe2"].Save()

	assert.Panics(t, func() {
		run("--help")
	})
}

func TestCompletion(t *testing.T) {
	defer gock.Off()

	gock.New("https://api.example.com/").Reply(http.StatusNotFound)
	gock.New("https://api.example.com/openapi.json").Reply(http.StatusOK)

	Init("Completion test", "1.0.0")
	Defaults()

	configs["comptest"] = &APIConfig{
		name: "comptest",
		Base: "https://api.example.com",
	}

	Root.AddCommand(&cobra.Command{
		Use: "comptest",
	})

	AddLoader(&testLoader{
		API: API{
			Operations: []Operation{
				{
					Method:      http.MethodGet,
					URITemplate: "https://api.example.com/users",
				},
				{
					Method:      http.MethodGet,
					URITemplate: "https://api.example.com/users/{user-id}",
				},
				{
					Short:       "List item tags",
					Method:      http.MethodGet,
					URITemplate: "https://api.example.com/items/{item-id}/tags",
				},
				{
					Short:       "Get tag details",
					Method:      http.MethodGet,
					URITemplate: "https://api.example.com/items/{item-id}/tags/{tag-id}",
				},
				{
					Method:      http.MethodDelete,
					URITemplate: "https://api.example.com/items/{item-id}/tags/{tag-id}",
				},
			},
		},
	})

	// Force a cache-reload if needed.
	viper.Set("rsh-no-cache", true)
	Load("https://api.example.com/", &cobra.Command{})
	viper.Set("rsh-no-cache", false)

	currentConfig = nil

	// Show APIs
	possible, _ := completeGenericCmd(http.MethodGet, true)(nil, []string{}, "")
	assert.Equal(t, []string{
		"comptest",
	}, possible)

	currentConfig = configs["comptest"]

	// Short-name URL completion with variables filled in.
	possible, _ = completeGenericCmd(http.MethodGet, false)(nil, []string{}, "comptest/items/my-item")
	assert.Equal(t, []string{
		"comptest/items/my-item/tags\tList item tags",
		"comptest/items/my-item/tags/{tag-id}\tGet tag details",
	}, possible)

	// URL without scheme
	possible, _ = completeGenericCmd(http.MethodGet, false)(nil, []string{}, "api.example.com/items/my-item")
	assert.Equal(t, []string{
		"api.example.com/items/my-item/tags\tList item tags",
		"api.example.com/items/my-item/tags/{tag-id}\tGet tag details",
	}, possible)
}

func TestSurfAPIBaseURLFromEnv(t *testing.T) {
	t.Setenv("SURF_API_BASE_URL", "https://api.stg.ask.surf/gateway/v1")
	viper.Reset()
	initConfig("surf", "")
	assert.Equal(t, "https://api.stg.ask.surf/gateway/v1", viper.GetString("surf-api-base-url"))
}

func TestSurfAPIBaseURLDefault(t *testing.T) {
	t.Setenv("SURF_API_BASE_URL", "")
	viper.Reset()
	initConfig("surf", "")
	assert.Equal(t, "https://api.asksurf.ai/gateway/v1", viper.GetString("surf-api-base-url"))
}
