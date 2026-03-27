package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	authURL  = "https://accounts.spotify.com/authorize"
	tokenURL = "https://accounts.spotify.com/api/token"
	scopes   = "user-read-recently-played user-top-read"
)

const successPage = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Spotify Garden - Authorized</title>
<style>
  *{margin:0;padding:0;box-sizing:border-box}
  body{
    min-height:100vh;display:flex;align-items:center;justify-content:center;
    font-family:'Segoe UI',system-ui,-apple-system,sans-serif;
    overflow:hidden;
  }
  /* wild SVG background via inline data URI */
  body{
    background-color:#0d1117;
    background-image:url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='120' height='120'%3E%3Cdefs%3E%3ClinearGradient id='a' x1='0' y1='0' x2='1' y2='1'%3E%3Cstop offset='0' stop-color='%231DB954'/%3E%3Cstop offset='1' stop-color='%2300d4ff'/%3E%3C/linearGradient%3E%3C/defs%3E%3Crect width='120' height='120' fill='%230d1117'/%3E%3Ccircle cx='10' cy='10' r='8' fill='%231DB954' opacity='.7'/%3E%3Ccircle cx='60' cy='5' r='4' fill='%23ff6ec7' opacity='.8'/%3E%3Ccircle cx='110' cy='15' r='6' fill='%23ffd700' opacity='.6'/%3E%3Cpath d='M0 60 Q30 40 60 60 T120 60' stroke='%23ff4500' stroke-width='1.5' fill='none' opacity='.5'/%3E%3Cpath d='M0 65 Q30 85 60 65 T120 65' stroke='%238b5cf6' stroke-width='1.5' fill='none' opacity='.5'/%3E%3Crect x='25' y='80' width='10' height='25' rx='2' fill='%2300d4ff' opacity='.5'/%3E%3Crect x='40' y='85' width='10' height='20' rx='2' fill='%231DB954' opacity='.5'/%3E%3Crect x='55' y='75' width='10' height='30' rx='2' fill='%23ff6ec7' opacity='.4'/%3E%3Crect x='70' y='82' width='10' height='23' rx='2' fill='%23ffd700' opacity='.5'/%3E%3Crect x='85' y='78' width='10' height='27' rx='2' fill='%23ff4500' opacity='.4'/%3E%3Ccircle cx='30' cy='40' r='3' fill='%2300ffc8' opacity='.6'/%3E%3Ccircle cx='90' cy='45' r='5' fill='%23ff85a2' opacity='.5'/%3E%3Cpolygon points='105,35 110,45 100,45' fill='%238b5cf6' opacity='.6'/%3E%3Ccircle cx='50' cy='30' r='2' fill='%23fff' opacity='.4'/%3E%3Cpath d='M15 110 Q20 95 30 110' stroke='%2300ffc8' stroke-width='1.2' fill='none' opacity='.5'/%3E%3Ccircle cx='100' cy='100' r='3' fill='%23ff85a2' opacity='.5'/%3E%3C/svg%3E");
    background-size:120px 120px;
    animation:bgScroll 12s linear infinite;
  }
  @keyframes bgScroll{to{background-position:120px 120px}}
  .card{
    background:rgba(13,17,23,.85);
    backdrop-filter:blur(16px);-webkit-backdrop-filter:blur(16px);
    border:1px solid rgba(29,185,84,.3);
    border-radius:24px;
    padding:3rem 3.5rem;
    text-align:center;
    box-shadow:
      0 0 40px rgba(29,185,84,.15),
      0 0 80px rgba(0,212,255,.08),
      0 25px 50px rgba(0,0,0,.4);
    animation:cardIn .6s cubic-bezier(.16,1,.3,1) both;
    max-width:460px;
  }
  @keyframes cardIn{from{opacity:0;transform:translateY(30px) scale(.95)}to{opacity:1;transform:none}}
  .icon{
    width:72px;height:72px;margin:0 auto 1.5rem;
    animation:pulse 2s ease-in-out infinite;
  }
  @keyframes pulse{0%,100%{transform:scale(1)}50%{transform:scale(1.08)}}
  .icon svg{width:100%;height:100%}
  h1{
    font-size:1.75rem;font-weight:700;
    background:linear-gradient(135deg,#1DB954,#00d4ff,#ff6ec7);
    background-size:200% 200%;
    -webkit-background-clip:text;-webkit-text-fill-color:transparent;
    background-clip:text;
    animation:gradShift 4s ease infinite;
    margin-bottom:.75rem;
  }
  @keyframes gradShift{0%,100%{background-position:0% 50%}50%{background-position:100% 50%}}
  p{color:rgba(255,255,255,.55);font-size:1rem;line-height:1.5}
</style>
</head>
<body>
  <div class="card">
    <div class="icon">
      <svg viewBox="0 0 72 72" fill="none" xmlns="http://www.w3.org/2000/svg">
        <circle cx="36" cy="36" r="34" fill="#1DB954"/>
        <path d="M26 48 L33 40 L40 46 L52 28" stroke="#fff" stroke-width="4" stroke-linecap="round" stroke-linejoin="round" fill="none"/>
      </svg>
    </div>
    <h1>Authorization successful!</h1>
    <p>You may close this tab.</p>
  </div>
</body>
</html>`

func getRedirectURI() string {
	if r := os.Getenv("SPOTIFY_REDIRECT_URI"); r != "" {
		return r
	}
	return "http://localhost:8888/callback"
}

// TokenResponse holds the stored OAuth token data.
type TokenResponse struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// spotifyAPIResponse holds the raw Spotify token API response.
type spotifyAPIResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
}

// randomState generates a cryptographically random hex state string.
func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// StartAuthFlow runs the full OAuth 2.0 authorization code flow.
// For localhost redirect URIs it starts a local callback server.
// For external redirect URIs it prompts the user to paste the redirect URL.
func StartAuthFlow(tokensPath string) error {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	if clientID == "" {
		return fmt.Errorf("SPOTIFY_CLIENT_ID not set")
	}

	redir := getRedirectURI()

	state, err := randomState()
	if err != nil {
		return fmt.Errorf("failed to generate state: %w", err)
	}

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", clientID)
	params.Set("redirect_uri", redir)
	params.Set("scope", scopes)
	params.Set("state", state)

	fullAuthURL := authURL + "?" + params.Encode()

	fmt.Println("Opening browser for Spotify authorization...")
	fmt.Println("If the browser does not open, visit:", fullAuthURL)
	_ = exec.Command("open", fullAuthURL).Start()

	var code string
	if strings.HasPrefix(redir, "http://localhost") || strings.HasPrefix(redir, "http://127.0.0.1") {
		code, err = captureLocalCallback(state, redir)
	} else {
		code, err = captureManualCallback(state)
	}
	if err != nil {
		return err
	}

	tokens, err := exchangeCode(code, redir)
	if err != nil {
		return fmt.Errorf("token exchange failed: %w", err)
	}

	if err := SaveTokens(tokensPath, tokens); err != nil {
		return fmt.Errorf("failed to save tokens: %w", err)
	}

	fmt.Printf("Authenticated successfully. Tokens saved to %s\n", tokensPath)
	return nil
}

// captureLocalCallback starts a local HTTP server and waits for Spotify to redirect to it.
func captureLocalCallback(state, redir string) (string, error) {
	u, err := url.Parse(redir)
	if err != nil {
		return "", fmt.Errorf("invalid redirect URI: %w", err)
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}

	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)

	mux := http.NewServeMux()
	srv := &http.Server{Addr: ":" + port, Handler: mux}

	mux.HandleFunc(u.Path, func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if got := q.Get("state"); got != state {
			errCh <- fmt.Errorf("state mismatch (got %q)", got)
			fmt.Fprintln(w, "Authorization failed (state mismatch). You may close this tab.")
			return
		}
		code := q.Get("code")
		if code == "" {
			errCh <- fmt.Errorf("no code in callback: %s", r.URL.RawQuery)
			fmt.Fprintln(w, "Authorization failed. You may close this tab.")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, successPage) //nolint: errcheck
		codeCh <- code
	})

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- fmt.Errorf("callback server error: %w", err)
		}
	}()

	fmt.Printf("Waiting for OAuth callback on %s ...\n", redir)

	var code string
	select {
	case code = <-codeCh:
	case err := <-errCh:
		return "", err
	case <-time.After(5 * time.Minute):
		return "", fmt.Errorf("auth flow timed out after 5 minutes")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)

	return code, nil
}

// captureManualCallback prompts the user to paste the redirect URL from their browser.
func captureManualCallback(state string) (string, error) {
	fmt.Println("\nAfter approving, Spotify will redirect your browser to your callback URL.")
	fmt.Println("Paste the full redirect URL here and press Enter:")
	fmt.Print("> ")

	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		return "", fmt.Errorf("failed to read input")
	}
	pasted := strings.TrimSpace(scanner.Text())

	u, err := url.Parse(pasted)
	if err != nil {
		return "", fmt.Errorf("could not parse pasted URL: %w", err)
	}

	q := u.Query()
	if got := q.Get("state"); got != state {
		return "", fmt.Errorf("state mismatch (got %q, want %q)", got, state)
	}

	code := q.Get("code")
	if code == "" {
		return "", fmt.Errorf("no code found in pasted URL")
	}

	return code, nil
}

// exchangeCode trades an authorization code for tokens using HTTP Basic auth.
func exchangeCode(code, redir string) (TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redir)

	return postTokenRequest(data)
}

// postTokenRequest sends a POST to the Spotify token endpoint with Basic auth.
func postTokenRequest(data url.Values) (TokenResponse, error) {
	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return TokenResponse{}, fmt.Errorf("SPOTIFY_CLIENT_ID and SPOTIFY_CLIENT_SECRET must be set")
	}

	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return TokenResponse{}, fmt.Errorf("failed to build token request: %w", err)
	}

	creds := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req.Header.Set("Authorization", "Basic "+creds)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return TokenResponse{}, fmt.Errorf("token endpoint returned %d", resp.StatusCode)
	}

	var raw spotifyAPIResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return TokenResponse{}, fmt.Errorf("failed to decode token response: %w", err)
	}

	tokens := TokenResponse{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAt:    time.Now().Add(time.Duration(raw.ExpiresIn) * time.Second),
	}
	return tokens, nil
}

// SaveTokens writes tokens to tokensPath with 0600 permissions.
func SaveTokens(tokensPath string, tokens TokenResponse) error {
	data, err := json.MarshalIndent(tokens, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tokensPath, data, 0600)
}

// LoadTokens reads tokens from tokensPath.
func LoadTokens(tokensPath string) (TokenResponse, error) {
	data, err := os.ReadFile(tokensPath)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("tokens not found (run 'music-garden auth' first): %w", err)
	}
	var tokens TokenResponse
	if err := json.Unmarshal(data, &tokens); err != nil {
		return TokenResponse{}, fmt.Errorf("failed to parse tokens.json: %w", err)
	}
	return tokens, nil
}

// RefreshIfNeeded checks token expiry and refreshes if necessary.
// Returns the valid access token.
func RefreshIfNeeded(tokensPath string) (string, error) {
	tokens, err := LoadTokens(tokensPath)
	if err != nil {
		return "", err
	}

	// Valid if not expiring within 5 minutes.
	if time.Now().Add(5 * time.Minute).Before(tokens.ExpiresAt) {
		return tokens.AccessToken, nil
	}

	fmt.Println("Access token expiring soon, refreshing...")
	refreshed, err := refreshTokens(tokens.RefreshToken)
	if err != nil {
		return "", fmt.Errorf("token refresh failed: %w", err)
	}

	if err := SaveTokens(tokensPath, refreshed); err != nil {
		return "", fmt.Errorf("failed to save refreshed tokens: %w", err)
	}

	return refreshed.AccessToken, nil
}

// refreshTokens exchanges a refresh token for a new token set.
func refreshTokens(refreshToken string) (TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	return postTokenRequest(data)
}
