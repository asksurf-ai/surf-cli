package cli

import (
	"bytes"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/h2non/gock.v1"
)

type captureFormatter struct {
	resp Response
}

func (f *captureFormatter) Format(resp Response) error {
	f.resp = resp
	return nil
}

func TestFixAddress(t *testing.T) {
	reset(false)
	assert.Equal(t, "https://example.com", fixAddress("example.com"))
	assert.Equal(t, "http://localhost:8000", fixAddress(":8000"))
	assert.Equal(t, "http://localhost:8000", fixAddress("localhost:8000"))

	configs["test"] = &APIConfig{
		Base: "https://example.com",
	}
	assert.Equal(t, "https://example.com/foo", fixAddress("test/foo"))
	delete(configs, "test")
}

func TestRequestPagination(t *testing.T) {
	defer gock.Off()

	gock.New("http://example.com").
		Get("/paginated").
		Reply(http.StatusOK).
		// Page 1 links to page 2
		SetHeader("Link", "</paginated2>; rel=\"next\"").
		SetHeader("Content-Length", "7").
		JSON([]any{1, 2, 3})
	gock.New("http://example.com").
		Get("/paginated2").
		Reply(http.StatusOK).
		// Page 2 links to page 3
		SetHeader("Link", "</paginated3>; rel=\"next\"").
		SetHeader("Content-Length", "5").
		JSON([]any{4, 5})
	gock.New("http://example.com").
		Get("/paginated3").
		Reply(http.StatusOK).
		SetHeader("Content-Length", "3").
		JSON([]any{6})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/paginated", nil)
	resp, err := GetParsedResponse(req)

	assert.NoError(t, err)
	assert.Equal(t, resp.Status, http.StatusOK)

	// Content length should be the sum of all combined.
	assert.Equal(t, resp.Headers["Content-Length"], "15")

	// Response body should be a concatenation of all pages.
	assert.Equal(t, []any{1.0, 2.0, 3.0, 4.0, 5.0, 6.0}, resp.Body)
}

func TestMakeRequestAndFormatTransformsOnchainTxResponse(t *testing.T) {
	defer gock.Off()

	reset(false)

	oldFormatter := Formatter
	oldCommand := currentCommand
	defer func() {
		Formatter = oldFormatter
		currentCommand = oldCommand
	}()

	capture := &captureFormatter{}
	Formatter = capture
	currentCommand = "onchain-tx"
	viper.Set("rsh-agent-view", "")
	viper.Set("rsh-shape", false)

	gock.New("http://example.com").
		Get("/tx").
		Reply(http.StatusOK).
		JSON(map[string]any{
			"data": []any{
				map[string]any{
					"hash":        "0xabc",
					"blockNumber": "0x157f411",
					"value":       "0xde0b6b3a7640000",
				},
			},
		})

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/tx", nil)
	MakeRequestAndFormat(req)

	require.True(t, gock.IsDone())
	body, ok := capture.resp.Body.(map[string]any)
	require.True(t, ok)
	data, ok := body["data"].([]any)
	require.True(t, ok)
	require.Len(t, data, 1)
	tx, ok := data[0].(map[string]any)
	require.True(t, ok)

	assert.Equal(t, "0x157f411", tx["blockNumber"])
	assert.Equal(t, "22541329", tx["blockNumberDecimal"])
	assert.Equal(t, "1000000000000000000", tx["valueDecimal"])
	assert.Equal(t, "1", tx["valueNativeDecimal"])
}

func TestGetStatus(t *testing.T) {
	defer gock.Off()

	reset(false)
	lastStatus = 0

	gock.New("http://example.com").
		Get("/").
		Reply(http.StatusOK)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	resp, err := MakeRequest(req)

	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	assert.Equal(t, http.StatusOK, GetLastStatus())
}

func TestIgnoreStatus(t *testing.T) {
	defer gock.Off()

	reset(false)
	lastStatus = 0

	gock.New("http://example.com").
		Get("/").
		Reply(http.StatusOK)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	resp, err := MakeRequest(req, IgnoreStatus())

	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)

	assert.Equal(t, 0, GetLastStatus())
}

func TestRequestRetryIn(t *testing.T) {
	defer gock.Off()

	reset(false)
	viper.Set("rsh-retry", 1)

	// Duration string value (with units)
	gock.New("http://example.com").
		Get("/").
		Times(1).
		Reply(http.StatusTooManyRequests).
		SetHeader("X-Retry-In", "1ms")

	gock.New("http://example.com").
		Get("/").
		Times(1).
		Reply(http.StatusOK)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	resp, err := MakeRequest(req)

	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
}

func TestCapRetryDelay(t *testing.T) {
	viper.Set("rsh-max-retry-after", 2*time.Second)
	defer viper.Set("rsh-max-retry-after", 5*time.Second)

	if got := capRetryDelay(30 * time.Second); got != 2*time.Second {
		t.Errorf("capRetryDelay(30s) = %s, want 2s", got)
	}
	if got := capRetryDelay(time.Second); got != time.Second {
		t.Errorf("capRetryDelay(1s) = %s, want 1s", got)
	}
	viper.Set("rsh-max-retry-after", time.Duration(0))
	if got := capRetryDelay(30 * time.Second); got != 30*time.Second {
		t.Errorf("capRetryDelay with no cap = %s, want 30s", got)
	}
}

func TestRetryTooSlow(t *testing.T) {
	viper.Set("rsh-retry-max-attempt-time", 2*time.Second)
	defer viper.Set("rsh-retry-max-attempt-time", 10*time.Second)

	if !retryTooSlow(2 * time.Second) {
		t.Error("retryTooSlow(2s) = false, want true")
	}
	if retryTooSlow(time.Second) {
		t.Error("retryTooSlow(1s) = true, want false")
	}
	viper.Set("rsh-retry-max-attempt-time", time.Duration(0))
	if retryTooSlow(time.Hour) {
		t.Error("retryTooSlow with no limit = true, want false")
	}
}

func TestRequestDoesNotRetrySlowFailure(t *testing.T) {
	defer gock.Off()

	reset(false)
	viper.Set("rsh-retry", 2)
	viper.Set("rsh-retry-max-attempt-time", 5*time.Millisecond)

	gock.New("http://example.com").
		Get("/").
		Times(1).
		Reply(http.StatusServiceUnavailable).
		Delay(20 * time.Millisecond)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	resp, err := MakeRequest(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	assert.True(t, gock.IsDone())
}

func TestRequestRetriesFastFailure(t *testing.T) {
	defer gock.Off()

	reset(false)
	viper.Set("rsh-retry", 1)
	viper.Set("rsh-retry-max-attempt-time", time.Second)

	gock.New("http://example.com").
		Get("/").
		Times(1).
		Reply(http.StatusServiceUnavailable).
		SetHeader("Retry-After", "0")

	gock.New("http://example.com").
		Get("/").
		Times(1).
		Reply(http.StatusOK)

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	resp, err := MakeRequest(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, gock.IsDone())
}

func TestRequestRetryAfter(t *testing.T) {
	defer gock.Off()

	reset(false)
	viper.Set("rsh-retry", 2)

	// Seconds value
	gock.New("http://example.com").
		Put("/").
		Times(1).
		Reply(http.StatusTooManyRequests).
		SetHeader("Retry-After", "0")

	// HTTP date value
	gock.New("http://example.com").
		Put("/").
		Times(1).
		Reply(http.StatusTooManyRequests).
		SetHeader("Retry-After", time.Now().Add(time.Second).UTC().Format(http.TimeFormat))

	gock.New("http://example.com").
		Put("/").
		Times(1).
		Reply(http.StatusOK)

	req, _ := http.NewRequest(http.MethodPut, "http://example.com/", bytes.NewReader([]byte("hello")))
	resp, err := MakeRequest(req)

	assert.NoError(t, err)
	assert.Equal(t, resp.StatusCode, http.StatusOK)
}

func TestSurfAPIKeyEnvVar(t *testing.T) {
	defer gock.Off()

	reset(false)

	// Mock an endpoint that checks the Authorization header
	gock.New("http://example.com").
		Get("/test").
		MatchHeader("Authorization", "Bearer test-api-key-123").
		Reply(http.StatusOK).
		JSON(map[string]string{"status": "ok"})

	// Set SURF_API_KEY
	os.Setenv("SURF_API_KEY", "test-api-key-123")
	defer os.Unsetenv("SURF_API_KEY")

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/test", nil)
	resp, err := MakeRequest(req)

	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestRequestRetryTimeout(t *testing.T) {
	defer gock.Off()

	reset(false)
	viper.Set("rsh-retry", 1)
	viper.Set("rsh-timeout", 1*time.Millisecond)

	// Duration string value (with units)
	gock.New("http://example.com").
		Get("/").
		Times(2).
		Reply(http.StatusOK).
		Delay(2 * time.Millisecond)
		// Note: delay seems to have a bug where subsequent requests without the
		// delay are still delayed... For now just have it reply twice.

	req, _ := http.NewRequest(http.MethodGet, "http://example.com/", nil)
	_, err := MakeRequest(req)

	assert.Error(t, err)
	assert.ErrorContains(t, err, "timed out")
}
