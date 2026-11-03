package openapi

import (
	"bytes"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/url"
	"os"
	"path"
	"sort"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/pb33f/libopenapi/datamodel/high/base"
	v3 "github.com/pb33f/libopenapi/datamodel/high/v3"
	"github.com/pb33f/libopenapi/orderedmap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/asksurf-ai/surf-cli/cli"
)

// parseURL parses the input as a URL ignoring any errors
func parseURL(s string) *url.URL {
	output, _ := url.Parse(s)
	return output
}

func TestGetBasePath(t *testing.T) {
	cases := []struct {
		name     string
		location *url.URL
		servers  []*v3.Server
		output   string
		hasError bool
	}{
		{
			name:     "Should return location if server is only a path",
			location: parseURL("http://localhost:12345/api"),
			servers:  []*v3.Server{{URL: "/api"}},
			output:   "/api",
		},
		{
			name:     "Should return the empty string if no matches can be found",
			location: parseURL("http://localhost:1245"),
			servers:  []*v3.Server{{URL: "http://my-api.foo.bar/foo"}},
			output:   "",
		},
		{
			name:     "Should return the prefix of the matched URL 1",
			location: parseURL("http://my-api.foo.bar"),
			servers:  []*v3.Server{{URL: "http://my-api.foo.bar/mount/api"}},
			output:   "/mount/api",
		},
		{
			name:     "Should return the prefix of the matched URL 2",
			location: parseURL("http://my-api.foo.bar/mount/api"),
			servers:  []*v3.Server{{URL: "http://my-api.foo.bar/mount/api"}},
			output:   "/mount/api",
		},
		{
			name:     "Should use default value for expanded url parameter 1",
			location: parseURL("http://my-api.foo.bar"),
			servers: []*v3.Server{
				{
					URL: "http://my-api.foo.bar/{mount}/api",
					Variables: orderedmap.From(maps.All(map[string]*v3.ServerVariable{
						"mount": {Default: "point"},
					})),
				},
			},
			output: "/point/api",
		},
		{
			name:     "Should use default value for expanded url parameter 2",
			location: parseURL("http://my-api.foo.bar"),
			servers: []*v3.Server{
				{URL: "http://my-api.some.other.domain:12456"},
				{
					URL: "http://my-api.foo.bar/{mount}/api",
					Variables: orderedmap.From(maps.All(map[string]*v3.ServerVariable{
						"mount": {Default: "point"},
					})),
				},
			},
			output: "/point/api",
		},
		{
			name:     "Should match with enum values over default for expanded url parameter 1",
			location: parseURL("http://my-api.foo.bar"),
			servers: []*v3.Server{
				{
					URL: "http://my-api.foo.bar/{mount}/api",
					Variables: orderedmap.From(maps.All(map[string]*v3.ServerVariable{
						"mount": {Default: "point", Enum: []string{"vec", "point"}},
					})),
				},
			},
			output: "/vec/api",
		},
		{
			name:     "Should match with enum values over default for expanded url parameter 2",
			location: parseURL("http://my-api.foo.bar"),
			servers: []*v3.Server{
				{URL: "http://my-api.some.other.domain:12456"},
				{
					URL: "http://my-api.foo.bar/{mount}/api",
					Variables: orderedmap.From(maps.All(map[string]*v3.ServerVariable{
						"mount": {Default: "point", Enum: []string{"vec", "point"}},
					})),
				},
			},
			output: "/vec/api",
		},
		{
			name:     "Should match against all expanded parameters",
			location: parseURL("http://ppmy-api.foo.bar/vec"),
			servers: []*v3.Server{
				{
					URL: "http://{env}my-api.foo.bar/{mount}/api",
					Variables: orderedmap.From(maps.All(map[string]*v3.ServerVariable{
						"env":   {Default: "pp"},
						"mount": {Default: "point", Enum: []string{"vec", "point"}},
					})),
				},
			},
			output: "/vec/api",
		},
		{
			name:     "Should return an error if the openapi server can't be parsed",
			location: parseURL("http://localhost"),
			servers:  []*v3.Server{{URL: "http://localhost@1224:foo"}},
			hasError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			output, err := getBasePath(tc.location, tc.servers)
			if !tc.hasError {
				if assert.NoError(t, err) {
					assert.Equal(t, tc.output, output)
				}
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestDetectViaHeader(t *testing.T) {
	resp := http.Response{
		Header: http.Header{},
	}
	resp.Header.Set("Content-Type", "application/vnd.oai.openapi;version=3.0")

	loader := New()
	assert.True(t, loader.Detect(&resp))
}

func TestDetectViaBody(t *testing.T) {
	resp := http.Response{
		Header: http.Header{},
		Body:   io.NopCloser(strings.NewReader("openapi: 3.1")),
	}

	loader := New()
	assert.True(t, loader.Detect(&resp))
}

func TestLocationHints(t *testing.T) {
	assert.Contains(t, New().LocationHints(), "/openapi.json")
}

func TestBrokenRequest(t *testing.T) {
	base, _ := url.Parse("http://api.example.com")
	spec, _ := url.Parse("/openapi.yaml")

	resp := &http.Response{
		Body: io.NopCloser(iotest.ErrReader(fmt.Errorf("request closed"))),
	}

	_, err := New().Load(*base, *spec, resp)
	assert.Error(t, err)
}

func TestEmptyDocument(t *testing.T) {
	base, _ := url.Parse("http://api.example.com")
	spec, _ := url.Parse("/openapi.yaml")

	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader("")),
	}

	_, err := New().Load(*base, *spec, resp)
	assert.Error(t, err)
}

func TestUnsupported(t *testing.T) {
	base, _ := url.Parse("http://api.example.com")
	spec, _ := url.Parse("/openapi.yaml")

	resp := &http.Response{
		Body: io.NopCloser(strings.NewReader(`swagger: 2.0`)),
	}

	_, err := New().Load(*base, *spec, resp)
	assert.Error(t, err)
}

func TestLoader(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	require.NoError(t, err)

	for _, entry := range entries {
		t.Run(entry.Name(), func(t *testing.T) {
			input, err := os.ReadFile(path.Join("testdata", entry.Name(), "openapi.yaml"))
			require.NoError(t, err)

			outBytes, err := os.ReadFile(path.Join("testdata", entry.Name(), "output.yaml"))
			require.NoError(t, err)

			var output cli.API
			require.NoError(t, yaml.Unmarshal(outBytes, &output))

			base, _ := url.Parse("http://api.example.com")
			spec, _ := url.Parse("http://api.example.com/openapi.yaml")

			resp := &http.Response{
				Body: io.NopCloser(bytes.NewReader(input)),
			}

			api, err := New().Load(*base, *spec, resp)
			assert.NoError(t, err)

			sort.Slice(api.Operations, func(i, j int) bool {
				return strings.Compare(api.Operations[i].Name, api.Operations[j].Name) < 0
			})

			assert.Equal(t, output, api)
		})
	}
}

// TestOpenAPIOperationParameterWithoutSchema covers parameters declared with
// `content` instead of `schema`. The spec allows it, and a nil schema used to
// panic while building the catalog and min/max bounds.
func TestOpenAPIOperationParameterWithoutSchema(t *testing.T) {
	uri, _ := url.Parse("https://example.com/things")
	op := &v3.Operation{
		OperationId: "contentOnly",
		Parameters: []*v3.Parameter{
			{Name: "filter", In: "query"},
		},
		Responses: &v3.Responses{Codes: orderedmap.New[string, *v3.Response]()},
	}

	require.NotPanics(t, func() {
		got := openapiOperation(nil, http.MethodGet, uri, &v3.PathItem{}, op)
		require.Len(t, got.QueryParams, 1)
		assert.Nil(t, got.QueryParams[0].Minimum)
		assert.Nil(t, got.QueryParams[0].Maximum)
	})
}

// TestOpenAPIParameterUsesPluralExamples covers OpenAPI 3.1 specs, which use
// `examples`; the Surf spec has 400+ such parameters and no singular ones.
func TestOpenAPIParameterUsesPluralExamples(t *testing.T) {
	min, max := 1.0, 10.0
	schema := base.CreateSchemaProxy(&base.Schema{
		Type: []string{"string"},
		Examples: []*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "ethereum"},
		},
		Minimum: &min,
		Maximum: &max,
	})
	uri, _ := url.Parse("https://example.com/things")
	op := &v3.Operation{
		OperationId: "pluralExamples",
		Parameters: []*v3.Parameter{
			{Name: "chain", In: "query", Schema: schema},
		},
		Responses: &v3.Responses{Codes: orderedmap.New[string, *v3.Response]()},
	}

	got := openapiOperation(nil, http.MethodGet, uri, &v3.PathItem{}, op)
	require.Len(t, got.QueryParams, 1)
	assert.Equal(t, "ethereum", got.QueryParams[0].Example)
	require.NotNil(t, got.QueryParams[0].Minimum)
	require.NotNil(t, got.QueryParams[0].Maximum)
	assert.Equal(t, 1.0, *got.QueryParams[0].Minimum)
	assert.Equal(t, 10.0, *got.QueryParams[0].Maximum)
}

// TestAppendSchemaHints_LargeBoundNotScientific guards against %g formatting,
// which renders 1000000 as 1e+06 in help and completion output.
func TestAppendSchemaHints_LargeBoundNotScientific(t *testing.T) {
	maximum := 1000000.0
	got := appendSchemaHints("Page size", &base.Schema{Maximum: &maximum})
	assert.Contains(t, got, "max 1000000")
	assert.NotContains(t, got, "1e+06")
}
