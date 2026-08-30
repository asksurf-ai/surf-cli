package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"gopkg.in/h2non/gock.v1"
)

func TestMinCachedTransport(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").Get("/success").Reply(200)
	gock.New("http://example.com").Get("/cached").Reply(200).SetHeader("cache-control", "max-age=10")
	gock.New("http://example.com").Get("/expires").Reply(200).SetHeader("expires", "Sun, 1 Jan 2020 12:00:00 GMT")
	gock.New("http://example.com").Get("/modify").Reply(200).SetHeader("cache-control", "public")
	gock.New("http://example.com").Get("/error").Reply(400)

	tx := MinCachedTransport(1 * time.Hour)

	// Missing cache, should get added.
	req, _ := http.NewRequest(http.MethodGet, "http://example.com/success", nil)
	resp, err := tx.RoundTrip(req)

	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	assert.Equal(t, resp.Header.Get("cache-control"), "max-age=3600")

	// Already-cached requests should not be touched.
	req, _ = http.NewRequest(http.MethodGet, "http://example.com/cached", nil)
	resp, err = tx.RoundTrip(req)

	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	assert.Equal(t, resp.Header.Get("cache-control"), "max-age=10")

	// Already-cached requests should not be touched.
	req, _ = http.NewRequest(http.MethodGet, "http://example.com/expires", nil)
	resp, err = tx.RoundTrip(req)

	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	assert.NotEmpty(t, resp.Header.Get("expires"))
	assert.Equal(t, resp.Header.Get("cache-control"), "")

	// Already-set header should be modified instead of replaced.
	req, _ = http.NewRequest(http.MethodGet, "http://example.com/modify", nil)
	resp, err = tx.RoundTrip(req)

	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 200)
	assert.Equal(t, resp.Header.Get("cache-control"), "public,max-age=3600")

	// Errors should not get cache headers added.
	req, _ = http.NewRequest(http.MethodGet, "http://example.com/error", nil)
	resp, err = tx.RoundTrip(req)

	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, 400)
	assert.Equal(t, resp.Header.Get("cache-control"), "")
}

func TestCachedTransportDoesNotShareCredentialedResponses(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write([]byte(r.Header.Get("Authorization")))
	}))
	defer server.Close()

	client := &http.Client{Transport: CachedTransport()}
	for _, token := range []string{"Bearer first-user", "Bearer second-user"} {
		req, err := http.NewRequest(http.MethodGet, server.URL+"/private", nil)
		assert.NoError(t, err)
		req.Header.Set("Authorization", token)

		resp, err := client.Do(req)
		assert.NoError(t, err)
		body, err := io.ReadAll(resp.Body)
		assert.NoError(t, err)
		assert.NoError(t, resp.Body.Close())
		assert.Equal(t, token, string(body))
	}

	assert.Equal(t, 2, requests, "credentialed requests must reach the origin independently")
}
