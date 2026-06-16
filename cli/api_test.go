package cli

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

type overrideLoader struct {
	detect        func(resp *http.Response) bool
	load          func(entrypoint, spec url.URL, resp *http.Response) (API, error)
	locationHints func() []string
}

func (l *overrideLoader) Detect(resp *http.Response) bool {
	if l.detect != nil {
		return l.detect(resp)
	}
	return true
}

func (l *overrideLoader) Load(entrypoint url.URL, spec url.URL, resp *http.Response) (API, error) {
	if l.load != nil {
		return l.load(entrypoint, spec, resp)
	}
	return API{}, nil
}
func (l *overrideLoader) LocationHints() []string {
	if l.locationHints != nil {
		return l.locationHints()
	}
	return []string{}
}

func TestLoadFromFile(t *testing.T) {
	reset(false)
	viper.Set("rsh-no-cache", true)
	AddLoader(&overrideLoader{
		load: func(entrypoint, spec url.URL, resp *http.Response) (API, error) {
			assert.Equal(t, "testdata/petstore.json", spec.String())
			return API{}, nil
		},
	})

	configs["file-load-test"] = &APIConfig{
		Base:      "https://api.example.com",
		SpecFiles: []string{"testdata/petstore.json"},
	}

	_, err := Load("https://api.example.com", &cobra.Command{})

	assert.NoError(t, err)
}

func TestBadSpecURL(t *testing.T) {
	reset(false)
	viper.Set("rsh-no-cache", true)
	AddLoader(&overrideLoader{
		load: func(entrypoint, spec url.URL, resp *http.Response) (API, error) {
			assert.Equal(t, "testdata/petstore.json", spec.String())
			return API{}, nil
		},
	})

	configs["bad-spec-url-test"] = &APIConfig{
		Base:      "https://api.example.com",
		SpecFiles: []string{"http://abc{def@ghi}"},
	}

	_, err := Load("https://api.example.com", &cobra.Command{})
	assert.Error(t, err)
}

func TestLoadAppliesOperationCommandHooks(t *testing.T) {
	reset(false)
	viper.Set("rsh-no-cache", true)

	called := false
	AddOperationCommandHook(func(cmd *cobra.Command) {
		if cmd.Name() != "search-web" {
			return
		}
		called = true
		cmd.Flags().Bool("hooked", false, "hooked by test")
	})

	AddLoader(&overrideLoader{
		load: func(entrypoint, spec url.URL, resp *http.Response) (API, error) {
			return API{
				Operations: []Operation{
					{
						Name:        "search web",
						Method:      http.MethodGet,
						URITemplate: "https://api.example.com/search",
					},
				},
			}, nil
		},
	})

	configs["hook-test"] = &APIConfig{
		Base:      "https://api.example.com",
		SpecFiles: []string{"testdata/petstore.json"},
	}

	root := &cobra.Command{Use: "test"}
	_, err := Load("https://api.example.com", root)
	assert.NoError(t, err)
	assert.True(t, called)

	cmd, _, err := root.Find([]string{"search-web"})
	assert.NoError(t, err)
	assert.NotNil(t, cmd.Flags().Lookup("hooked"))
}
