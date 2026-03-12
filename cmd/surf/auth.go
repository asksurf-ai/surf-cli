package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/cyberconnecthq/surf-cli/cli"
	"golang.org/x/oauth2"
)

// ---------------------------------------------------------------------------
// Callback HTML — override these to brand the login completion page.
// ---------------------------------------------------------------------------

var successHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Surf — Logged In</title>
<style>
  @keyframes rise {
    from { transform: translateY(20px) scale(0.98); opacity: 0; }
    to { transform: translateY(0) scale(1); opacity: 1; }
  }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
    display: flex; justify-content: center; align-items: center;
    min-height: 100vh; margin: 0;
    background: #0a0a0a url('https://surf-oauth.vercel.app/bg.svg') center / cover;
    color: #e4e4e7;
  }
  .card {
    width: 380px; padding: 3rem 2.5rem; text-align: center;
    background: rgba(255,255,255,0.02);
    border: 1px solid rgba(255,255,255,0.06);
    border-radius: 24px;
    backdrop-filter: blur(40px); -webkit-backdrop-filter: blur(40px);
    box-shadow: 0 0 0 1px rgba(255,255,255,0.03) inset, 0 24px 48px -12px rgba(0,0,0,0.5);
    animation: rise 0.6s cubic-bezier(0.16, 1, 0.3, 1);
  }
  .logo { height: 36px; opacity: 0.95; margin-bottom: 2rem; }
  .divider { width: 32px; height: 1px; background: rgba(255,255,255,0.08); margin: 0 auto 1.5rem; }
  .status { font-size: 0.8125rem; font-weight: 500; letter-spacing: 0.02em; color: #34d399; }
  .status .check { display: inline-block; vertical-align: middle; margin-right: 6px; }
</style>
</head>
<body>
  <div class="card">
    <img class="logo" src="https://surf-oauth.vercel.app/logo.avif" alt="Surf">
    <div class="divider"></div>
    <div class="status"><span class="check">&#10003;</span> Authorized — you can close this window</div>
  </div>
</body>
</html>`

var errorHTML = `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>Surf — Login Error</title>
<style>
  @keyframes rise {
    from { transform: translateY(20px) scale(0.98); opacity: 0; }
    to { transform: translateY(0) scale(1); opacity: 1; }
  }
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', system-ui, sans-serif;
    display: flex; justify-content: center; align-items: center;
    min-height: 100vh; margin: 0;
    background: #0a0a0a url('https://surf-oauth.vercel.app/bg.svg') center / cover;
    color: #e4e4e7;
  }
  .card {
    width: 380px; padding: 3rem 2.5rem; text-align: center;
    background: rgba(255,255,255,0.02);
    border: 1px solid rgba(255,255,255,0.06);
    border-radius: 24px;
    backdrop-filter: blur(40px); -webkit-backdrop-filter: blur(40px);
    box-shadow: 0 0 0 1px rgba(255,255,255,0.03) inset, 0 24px 48px -12px rgba(0,0,0,0.5);
    animation: rise 0.6s cubic-bezier(0.16, 1, 0.3, 1);
  }
  .logo { height: 36px; opacity: 0.95; margin-bottom: 2rem; }
  .divider { width: 32px; height: 1px; background: rgba(255,255,255,0.08); margin: 0 auto 1.5rem; }
  .status { font-size: 0.8125rem; font-weight: 400; line-height: 1.5; color: #fb7185; }
</style>
</head>
<body>
  <div class="card">
    <img class="logo" src="https://surf-oauth.vercel.app/logo.avif" alt="Surf">
    <div class="divider"></div>
    <div class="status">$ERROR<br>$DETAILS</div>
  </div>
</body>
</html>`

// ---------------------------------------------------------------------------
// Token request (reimplements restish's unexported requestToken)
// ---------------------------------------------------------------------------

type tokenResponse struct {
	TokenType    string        `json:"token_type"`
	AccessToken  string        `json:"access_token"`
	RefreshToken string        `json:"refresh_token,omitempty"`
	ExpiresIn    time.Duration `json:"expires_in"`
	Expiry       time.Time     `json:"expiry,omitempty"`
}

func requestToken(tokenURL, payload string) (*oauth2.Token, error) {
	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Add("content-type", "application/x-www-form-urlencoded")

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	if res.StatusCode > 200 {
		return nil, fmt.Errorf("bad response from token endpoint:\n%s", body)
	}

	decoded := tokenResponse{}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, err
	}

	expiry := decoded.Expiry
	if expiry.IsZero() {
		expiry = time.Now().Add(decoded.ExpiresIn * time.Second)
	}

	return &oauth2.Token{
		AccessToken:  decoded.AccessToken,
		TokenType:    decoded.TokenType,
		RefreshToken: decoded.RefreshToken,
		Expiry:       expiry,
	}, nil
}

// ---------------------------------------------------------------------------
// Refresh token source (reimplements restish's — needed because requestToken
// is unexported and RefreshTokenSource calls it internally)
// ---------------------------------------------------------------------------

type surfRefreshTokenSource struct {
	ClientID       string
	TokenURL       string
	Scopes         []string
	EndpointParams *url.Values
	RefreshToken   string
	TokenSource    oauth2.TokenSource
}

func (ts *surfRefreshTokenSource) Token() (*oauth2.Token, error) {
	if ts.RefreshToken != "" {
		cli.LogDebug("Trying refresh token to get a new access token")
		params := url.Values{
			"grant_type":    {"refresh_token"},
			"client_id":     {ts.ClientID},
			"refresh_token": {ts.RefreshToken},
			"scope":         {strings.Join(ts.Scopes, " ")},
		}
		if ts.EndpointParams != nil {
			for k, v := range *ts.EndpointParams {
				params[k] = v
			}
		}
		token, err := requestToken(ts.TokenURL, params.Encode())
		if err == nil {
			return token, nil
		}
		cli.LogDebug("Refresh token failed: %v — falling back to browser login", err)
	}

	token, err := ts.TokenSource.Token()
	if err != nil {
		return nil, err
	}
	ts.RefreshToken = token.RefreshToken
	return token, nil
}

// ---------------------------------------------------------------------------
// Auth code token source (PKCE) — uses custom callback HTML
// ---------------------------------------------------------------------------

type surfAuthCodeTokenSource struct {
	ClientID       string
	ClientSecret   string
	AuthorizeURL   string
	TokenURL       string
	RedirectURL    string
	EndpointParams *url.Values
	Scopes         []string
}

func (ac *surfAuthCodeTokenSource) redirectURL() string {
	if ac.RedirectURL != "" {
		return ac.RedirectURL
	}
	return "http://localhost:8484"
}

func (ac *surfAuthCodeTokenSource) Token() (*oauth2.Token, error) {
	// PKCE: generate code verifier + challenge.
	verifierBytes := make([]byte, 32)
	if _, err := rand.Read(verifierBytes); err != nil {
		return nil, err
	}
	verifier := base64.RawURLEncoding.EncodeToString(verifierBytes)
	shaBytes := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(shaBytes[:])

	// Build authorize URL.
	authorizeURL, err := url.Parse(ac.AuthorizeURL)
	if err != nil {
		return nil, fmt.Errorf("invalid authorize URL: %w", err)
	}
	aq := authorizeURL.Query()
	aq.Set("response_type", "code")
	aq.Set("code_challenge", challenge)
	aq.Set("code_challenge_method", "S256")
	aq.Set("client_id", ac.ClientID)
	aq.Set("redirect_uri", ac.redirectURL())
	aq.Set("scope", strings.Join(ac.Scopes, " "))
	if ac.EndpointParams != nil {
		for k, v := range *ac.EndpointParams {
			aq.Set(k, v[0])
		}
	}
	authorizeURL.RawQuery = aq.Encode()

	// Start local callback server.
	codeChan := make(chan string, 1)
	handler := &callbackHandler{c: codeChan}

	u, err := url.Parse(ac.redirectURL())
	if err != nil {
		return nil, fmt.Errorf("invalid redirect URL: %w", err)
	}
	addr := fmt.Sprintf("%s:%s", u.Hostname(), u.Port())

	srv := &http.Server{
		Addr:           addr,
		Handler:        handler,
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxHeaderBytes: 1024,
	}
	go func() {
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "Callback server error: %v\n", err)
		}
	}()

	// Open browser.
	fmt.Fprintln(os.Stderr, "Open your browser to log in using the URL:")
	fmt.Fprintln(os.Stderr, authorizeURL.String())
	openBrowser(authorizeURL.String())

	// Also accept manual code input from terminal.
	manualChan := make(chan string, 1)
	if isatty.IsTerminal(os.Stdin.Fd()) || isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		fmt.Fprint(os.Stderr, "Alternatively, enter the code manually: ")
		go func() {
			r := bufio.NewReader(os.Stdin)
			result, err := r.ReadString('\n')
			if err != nil {
				return
			}
			manualChan <- strings.TrimRight(result, "\n")
		}()
	}

	// Wait for code.
	var code string
	select {
	case code = <-codeChan:
	case code = <-manualChan:
	}
	fmt.Fprintln(os.Stderr)
	srv.Shutdown(context.Background())

	if code == "" {
		fmt.Fprintln(os.Stderr, "Unable to get a code. See browser for details.")
		os.Exit(1)
	}

	// Exchange code for token.
	payload := url.Values{
		"grant_type":    {"authorization_code"},
		"client_id":     {ac.ClientID},
		"code_verifier": {verifier},
		"code":          {code},
		"redirect_uri":  {ac.redirectURL()},
	}
	if ac.ClientSecret != "" {
		payload.Set("client_secret", ac.ClientSecret)
	}
	return requestToken(ac.TokenURL, payload.Encode())
}

// callbackHandler serves the OAuth redirect and renders custom HTML.
type callbackHandler struct {
	c chan string
}

func (h *callbackHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	if errMsg := r.URL.Query().Get("error"); errMsg != "" {
		details := r.URL.Query().Get("error_description")
		rendered := strings.Replace(strings.Replace(errorHTML, "$ERROR", errMsg, 1), "$DETAILS", details, 1)
		w.Write([]byte(rendered))
		h.c <- ""
		return
	}
	h.c <- r.URL.Query().Get("code")
	w.Write([]byte(successHTML))
}

// ---------------------------------------------------------------------------
// Token cache handler — reads/writes tokens from cli.Cache.
// Future: swap cli.Cache for OS keychain here.
// ---------------------------------------------------------------------------

func handleToken(source oauth2.TokenSource, key string, request *http.Request) error {
	var cached *oauth2.Token

	expiresKey := key + ".expires"
	typeKey := key + ".type"
	tokenKey := key + ".token"
	refreshKey := key + ".refresh"

	expiry := cli.Cache.GetTime(expiresKey)
	if !expiry.IsZero() {
		cli.LogDebug("Loading OAuth2 token from cache.")
		cached = &oauth2.Token{
			AccessToken:  cli.Cache.GetString(tokenKey),
			RefreshToken: cli.Cache.GetString(refreshKey),
			TokenType:    cli.Cache.GetString(typeKey),
			Expiry:       expiry,
		}
	}

	if cached != nil {
		source = oauth2.ReuseTokenSource(cached, source)
	}

	token, err := source.Token()
	if err != nil {
		return err
	}

	if cached == nil || token.AccessToken != cached.AccessToken {
		cli.LogDebug("Token refreshed. Updating cache.")
		cli.Cache.Set(expiresKey, token.Expiry)
		cli.Cache.Set(typeKey, token.Type())
		cli.Cache.Set(tokenKey, token.AccessToken)
		if token.RefreshToken != "" {
			cli.Cache.Set(refreshKey, token.RefreshToken)
		}
		if err := cli.Cache.WriteConfig(); err != nil {
			return err
		}
	}

	token.SetAuthHeader(request)
	return nil
}

// ---------------------------------------------------------------------------
// SurfAuthHandler — implements cli.AuthHandler
// ---------------------------------------------------------------------------

// SurfAuthHandler implements the OAuth 2.0 authorization code flow with PKCE,
// custom callback HTML, and a token cache that can be swapped for OS keychain.
type SurfAuthHandler struct{}

func (h *SurfAuthHandler) Parameters() []cli.AuthParam {
	return []cli.AuthParam{
		{Name: "client_id", Required: true, Help: "OAuth 2.0 Client ID"},
		{Name: "client_secret", Help: "OAuth 2.0 Client Secret (optional)"},
		{Name: "authorize_url", Required: true, Help: "OAuth 2.0 authorization URL"},
		{Name: "token_url", Required: true, Help: "OAuth 2.0 token URL"},
		{Name: "scopes", Help: "Comma-separated scopes to request"},
		{Name: "redirect_url", Help: "Redirect URL (default: http://localhost:8484)"},
	}
}

func (h *SurfAuthHandler) OnRequest(request *http.Request, key string, params map[string]string) error {
	if request.Header.Get("Authorization") != "" {
		return nil
	}

	endpointParams := url.Values{}
	knownParams := map[string]bool{
		"client_id": true, "client_secret": true, "scopes": true,
		"authorize_url": true, "token_url": true, "redirect_url": true,
	}
	for k, v := range params {
		if !knownParams[k] {
			endpointParams.Add(k, v)
		}
	}

	source := &surfAuthCodeTokenSource{
		ClientID:       params["client_id"],
		ClientSecret:   params["client_secret"],
		AuthorizeURL:   params["authorize_url"],
		TokenURL:       params["token_url"],
		RedirectURL:    params["redirect_url"],
		EndpointParams: &endpointParams,
		Scopes:         strings.Split(params["scopes"], ","),
	}

	refreshSource := &surfRefreshTokenSource{
		ClientID:       params["client_id"],
		TokenURL:       params["token_url"],
		Scopes:         strings.Split(params["scopes"], ","),
		EndpointParams: &endpointParams,
		RefreshToken:   cli.Cache.GetString(key + ".refresh"),
		TokenSource:    source,
	}

	return handleToken(refreshSource, key, request)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func openBrowser(url string) {
	var cmd string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	case "windows":
		cmd = "cmd"
		args = []string{"/c", "start"}
	default:
		cmd = "xdg-open"
	}
	args = append(args, url)
	exec.Command(cmd, args...).Start()
}
