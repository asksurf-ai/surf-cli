package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"sync"
	"testing"
)

// TestOperationFlagValuesStartingWithDash covers values like a search query
// with an exclusion term. Before the operation is loaded, surf used to read
// them as global shorthand flags: `-twitter` as -t (crash), `-price` as -p
// (crash), `-quantum` as an extra -q query param, `-Hyperliquid` as an extra
// -H header and `-s<url>` as the API base URL.
func TestOperationFlagValuesStartingWithDash(t *testing.T) {
	bin := buildBareSurfBin(t)
	home := t.TempDir()

	var mu sync.Mutex
	var gotQuery url.Values
	var gotHeader http.Header
	srv, _ := newSpecAndOperationServer(t, "flagValues", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotQuery = r.URL.Query()
		gotHeader = r.Header.Clone()
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"data":[]}`)
	})
	defer srv.Close()

	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request went to the wrong host: %s %s", r.Method, r.URL)
		http.NotFound(w, r)
	}))
	defer other.Close()

	env := []string{"SURF_API_BASE_URL=" + srv.URL + "/gateway/v1"}
	runSurf(t, bin, home, env, "sync")

	values := []string{
		"-twitter bitcoin",
		"-price bitcoin",
		"-quantum",
		"-Hyperliquid",
		"-s" + other.URL + "/gateway",
	}
	for _, value := range values {
		t.Run(value, func(t *testing.T) {
			mu.Lock()
			gotQuery, gotHeader = nil, nil
			mu.Unlock()

			out, err := surfCmd(t, bin, home, env, "flag-values", "--address", value).CombinedOutput()
			if err != nil {
				t.Fatalf("surf flag-values --address %q failed: %v\n%s", value, err, out)
			}

			mu.Lock()
			defer mu.Unlock()
			if want := (url.Values{"address": {value}}); !reflect.DeepEqual(gotQuery, want) {
				t.Errorf("query = %v, want %v", gotQuery, want)
			}
			if _, ok := gotHeader["Yperliquid"]; ok {
				t.Errorf("value was sent as a header: %v", gotHeader)
			}
		})
	}
}
