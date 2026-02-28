package auth

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
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
		fmt.Fprintln(w, "Authorization successful! You may close this tab.")
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
		return TokenResponse{}, fmt.Errorf("tokens not found (run 'spotify-garden auth' first): %w", err)
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
