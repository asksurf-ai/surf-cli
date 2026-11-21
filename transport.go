package cli

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/gbl08ma/httpcache"
	"github.com/gbl08ma/httpcache/diskcache"
)

// cacheKey returns the cache key for req.
func cacheKey(req *http.Request) string {
	if req.Method == http.MethodGet {
		return req.URL.String()
	}

	return req.Method + " " + req.URL.String()
}

// shouldCache returns whether a response should be manually cached.
func shouldCache(resp *http.Response) bool {
	// Error responses should not be cached.
	if resp.StatusCode >= 400 {
		return false
	}

	// The older "Expires" header means we should not touch it.
	if resp.Header.Get("expires") != "" {
		return false
	}

	// There is a "Cache-Control" header *AND* it has a cache age set, so we
	// should not touch it.
	if strings.Contains(resp.Header.Get("cache-control"), "max-age") {
		return false
	}

	return true
}

// cachedTransport creates the disk-backed transport used for requests that
// do not carry caller-specific credentials.
func cachedTransport() *httpcache.Transport {
	t := httpcache.NewTransport(diskcache.New(filepath.Join(getCacheDir(), "responses")))
	t.MarkCachedResponses = false
	return t
}

type credentialAwareTransport struct {
	cached   http.RoundTripper
	uncached http.RoundTripper
}

func (t credentialAwareTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// The cache dependency keys entries by URL unless the server sends Vary.
	// Credentials must therefore bypass it to prevent one local profile from
	// receiving another profile's response for the same endpoint.
	if req.Header.Get("Authorization") != "" ||
		req.Header.Get("Proxy-Authorization") != "" ||
		req.Header.Get("Cookie") != "" {
		return t.uncached.RoundTrip(req)
	}
	return t.cached.RoundTrip(req)
}

// CachedTransport returns an HTTP transport with caching for anonymous
// requests and direct transport for credentialed requests.
func CachedTransport() http.RoundTripper {
	return credentialAwareTransport{
		cached:   cachedTransport(),
		uncached: http.DefaultTransport,
	}
}

type minCachedTransport struct {
	min time.Duration
}

func (m minCachedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Automatically cache for the minimum time if the request is successful and
	// the response doesn't already have cache headers.
	if shouldCache(resp) {
		// Add the minimum max-age.
		ma := fmt.Sprintf("max-age=%d", int(m.min.Seconds()))
		if cc := resp.Header.Get("cache-control"); cc != "" {
			resp.Header.Set("cache-control", cc+","+ma)
		} else {
			resp.Header.Set("cache-control", ma)
		}
	}

	// HACK: httpcache expects reads rather than close, so for now we special-case
	// the 204 response type and do a dummy read that immediately results in
	// an EOF.
	if resp.StatusCode == http.StatusNoContent {
		io.ReadAll(resp.Body)
	}

	return resp, nil
}

// MinCachedTransport returns an HTTP transport with caching abilities and
// a minimum cache duration for any responses if no cache headers are set.
func MinCachedTransport(min time.Duration) http.RoundTripper {
	t := cachedTransport()
	t.Transport = &minCachedTransport{min}
	return credentialAwareTransport{
		cached:   t,
		uncached: http.DefaultTransport,
	}
}

type invalidateCachedTransport struct {
	transport *httpcache.Transport
}

func (i invalidateCachedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Invalidate cache entry.
	key := cacheKey(req)
	i.transport.Cache.Delete(key)

	// Make the request.
	return i.transport.RoundTrip(req)
}

// InvalidateCachedTransport returns an HTTP transport which will not read
// cached items (it deletes them) and then refreshes the cache when new items
// are fetched.
func InvalidateCachedTransport() http.RoundTripper {
	return credentialAwareTransport{
		cached: &invalidateCachedTransport{
			transport: cachedTransport(),
		},
		uncached: http.DefaultTransport,
	}
}
