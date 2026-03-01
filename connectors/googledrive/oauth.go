// Package googledrive provides a connector for Google Drive with OAuth2 authentication.
// It supports exporting Google Workspace documents (Docs, Sheets, Slides) to local formats.
package googledrive

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// OAuth2 endpoints for Google
const (
	googleAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenURL    = "https://oauth2.googleapis.com/token"
	googleRevokeURL   = "https://oauth2.googleapis.com/revoke"
	googleUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"
)

// OAuth2 scopes required for Drive access
var driveScopes = []string{
	"https://www.googleapis.com/auth/drive.readonly",
	"https://www.googleapis.com/auth/userinfo.email",
}

// OAuth2Config holds the OAuth2 client credentials.
type OAuth2Config struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	RedirectURI  string `json:"redirect_uri,omitempty"` // If empty, uses local callback
}

// OAuth2Token represents an OAuth2 access token with refresh capability.
type OAuth2Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry,omitempty"`
	Scope        string    `json:"scope,omitempty"`
}

// Valid returns true if the token is not expired (with 1 minute buffer).
func (t *OAuth2Token) Valid() bool {
	if t.AccessToken == "" {
		return false
	}
	if t.Expiry.IsZero() {
		return true
	}
	return time.Now().Add(time.Minute).Before(t.Expiry)
}

// OAuth2Client handles OAuth2 authentication flow.
type OAuth2Client struct {
	config   OAuth2Config
	token    *OAuth2Token
	mu       sync.RWMutex
	http     *http.Client
	tokenDir string // Directory to cache tokens
}

// NewOAuth2Client creates a new OAuth2 client.
func NewOAuth2Client(config OAuth2Config, tokenDir string) *OAuth2Client {
	return &OAuth2Client{
		config:   config,
		http:     &http.Client{Timeout: 30 * time.Second},
		tokenDir: tokenDir,
	}
}

// GetToken returns the current token, refreshing if necessary.
func (c *OAuth2Client) GetToken(ctx context.Context) (*OAuth2Token, error) {
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()

	if token == nil {
		return nil, errors.New("not authenticated - please run OAuth2 flow")
	}

	if token.Valid() {
		return token, nil
	}

	// Try to refresh
	if token.RefreshToken == "" {
		return nil, errors.New("token expired and no refresh token available")
	}

	newToken, err := c.refreshToken(ctx, token.RefreshToken)
	if err != nil {
		return nil, fmt.Errorf("refresh token: %w", err)
	}

	c.mu.Lock()
	c.token = newToken
	c.mu.Unlock()

	// Cache the new token
	if c.tokenDir != "" {
		_ = c.saveTokenToCache(newToken)
	}

	return newToken, nil
}

// SetToken sets the current token.
func (c *OAuth2Client) SetToken(token *OAuth2Token) {
	c.mu.Lock()
	c.token = token
	c.mu.Unlock()
}

// LoadTokenFromCache attempts to load a cached token.
func (c *OAuth2Client) LoadTokenFromCache() error {
	if c.tokenDir == "" {
		return errors.New("no token directory configured")
	}

	tokenPath := filepath.Join(c.tokenDir, "google_drive_token.json")
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("read token cache: %w", err)
	}

	var token OAuth2Token
	if err := json.Unmarshal(data, &token); err != nil {
		return fmt.Errorf("parse token cache: %w", err)
	}

	c.mu.Lock()
	c.token = &token
	c.mu.Unlock()

	return nil
}

// saveTokenToCache saves the token to the cache directory.
func (c *OAuth2Client) saveTokenToCache(token *OAuth2Token) error {
	if c.tokenDir == "" {
		return nil
	}

	if err := os.MkdirAll(c.tokenDir, 0o700); err != nil {
		return err
	}

	tokenPath := filepath.Join(c.tokenDir, "google_drive_token.json")
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(tokenPath, data, 0o600)
}

// StartAuthFlow starts the OAuth2 authorization flow.
// It starts a local HTTP server to receive the callback.
// Returns the authorization URL the user should visit.
func (c *OAuth2Client) StartAuthFlow(ctx context.Context) (authURL string, tokenChan <-chan *OAuth2Token, errChan <-chan error, err error) {
	// Generate state for CSRF protection
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		return "", nil, nil, fmt.Errorf("generate state: %w", err)
	}
	state := base64.URLEncoding.EncodeToString(stateBytes)

	// Find an available port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, nil, fmt.Errorf("listen: %w", err)
	}

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	// Build authorization URL
	params := url.Values{
		"client_id":     {c.config.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {strings.Join(driveScopes, " ")},
		"state":         {state},
		"access_type":   {"offline"},
		"prompt":        {"consent"}, // Force consent to get refresh token
	}
	authURL = googleAuthURL + "?" + params.Encode()

	tokChan := make(chan *OAuth2Token, 1)
	eChan := make(chan error, 1)

	// Start callback server
	server := &http.Server{}
	go func() {
		defer listener.Close()
		defer close(tokChan)
		defer close(eChan)

		mux := http.NewServeMux()
		mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
			// Verify state
			if r.URL.Query().Get("state") != state {
				eChan <- errors.New("invalid state parameter")
				http.Error(w, "Invalid state", http.StatusBadRequest)
				return
			}

			// Check for error response
			if errMsg := r.URL.Query().Get("error"); errMsg != "" {
				errDesc := r.URL.Query().Get("error_description")
				eChan <- fmt.Errorf("oauth error: %s - %s", errMsg, errDesc)
				http.Error(w, errDesc, http.StatusBadRequest)
				return
			}

			// Get authorization code
			code := r.URL.Query().Get("code")
			if code == "" {
				eChan <- errors.New("no authorization code received")
				http.Error(w, "No code", http.StatusBadRequest)
				return
			}

			// Exchange code for token - use a fresh context since the original may have timed out
			// while waiting for user to complete browser auth
			exchangeCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			token, err := c.exchangeCode(exchangeCtx, code, redirectURI)
			if err != nil {
				eChan <- fmt.Errorf("exchange code: %w", err)
				http.Error(w, "Token exchange failed: "+err.Error(), http.StatusInternalServerError)
				return
			}

			// Save token
			c.mu.Lock()
			c.token = token
			c.mu.Unlock()

			if c.tokenDir != "" {
				_ = c.saveTokenToCache(token)
			}

			// Send success response
			w.Header().Set("Content-Type", "text/html")
			fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>Revoco - Authorization Complete</title></head>
<body style="font-family: system-ui; max-width: 600px; margin: 50px auto; text-align: center;">
<h1>Authorization Successful!</h1>
<p>You can close this window and return to Revoco.</p>
<p style="color: #666;">Your Google Drive data is now accessible.</p>
</body>
</html>`)

			tokChan <- token

			// Shutdown server after response
			go func() {
				time.Sleep(100 * time.Millisecond)
				server.Shutdown(context.Background())
			}()
		})

		server.Handler = mux
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			eChan <- fmt.Errorf("callback server: %w", err)
		}
	}()

	return authURL, tokChan, eChan, nil
}

// exchangeCode exchanges an authorization code for tokens.
func (c *OAuth2Client) exchangeCode(ctx context.Context, code, redirectURI string) (*OAuth2Token, error) {
	data := url.Values{
		"client_id":     {c.config.ClientID},
		"client_secret": {c.config.ClientSecret},
		"code":          {code},
		"grant_type":    {"authorization_code"},
		"redirect_uri":  {redirectURI},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&errResp); decodeErr != nil || errResp.Error == "" {
			return nil, fmt.Errorf("token exchange failed with status %d (check client_id and client_secret)", resp.StatusCode)
		}
		return nil, fmt.Errorf("token exchange: %s - %s", errResp.Error, errResp.Description)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		RefreshToken string `json:"refresh_token"`
		Scope        string `json:"scope"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}

	token := &OAuth2Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: tokenResp.RefreshToken,
		Scope:        tokenResp.Scope,
	}

	if tokenResp.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return token, nil
}

// refreshToken refreshes an expired access token.
func (c *OAuth2Client) refreshToken(ctx context.Context, refreshToken string) (*OAuth2Token, error) {
	data := url.Values{
		"client_id":     {c.config.ClientID},
		"client_secret": {c.config.ClientSecret},
		"refresh_token": {refreshToken},
		"grant_type":    {"refresh_token"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", googleTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error       string `json:"error"`
			Description string `json:"error_description"`
		}
		if decodeErr := json.NewDecoder(resp.Body).Decode(&errResp); decodeErr != nil || errResp.Error == "" {
			return nil, fmt.Errorf("token refresh failed with status %d (may need to re-authenticate)", resp.StatusCode)
		}
		// Provide helpful hints for common errors
		hint := ""
		if errResp.Error == "invalid_grant" {
			hint = " - refresh token may have been revoked or expired, please re-authenticate"
		}
		return nil, fmt.Errorf("token refresh: %s - %s%s", errResp.Error, errResp.Description, hint)
	}

	var tokenResp struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		ExpiresIn    int    `json:"expires_in"`
		Scope        string `json:"scope"`
		RefreshToken string `json:"refresh_token,omitempty"` // Google may return a new refresh token
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("decode token: %w", err)
	}

	// Use the new refresh token if Google returned one, otherwise keep the original
	newRefreshToken := refreshToken
	if tokenResp.RefreshToken != "" {
		newRefreshToken = tokenResp.RefreshToken
	}

	token := &OAuth2Token{
		AccessToken:  tokenResp.AccessToken,
		TokenType:    tokenResp.TokenType,
		RefreshToken: newRefreshToken,
		Scope:        tokenResp.Scope,
	}

	if tokenResp.ExpiresIn > 0 {
		token.Expiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	}

	return token, nil
}

// RevokeToken revokes the current token.
func (c *OAuth2Client) RevokeToken(ctx context.Context) error {
	c.mu.RLock()
	token := c.token
	c.mu.RUnlock()

	if token == nil {
		return nil
	}

	tokenToRevoke := token.RefreshToken
	if tokenToRevoke == "" {
		tokenToRevoke = token.AccessToken
	}

	data := url.Values{"token": {tokenToRevoke}}
	req, err := http.NewRequestWithContext(ctx, "POST", googleRevokeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()

	// Clear cached token
	c.mu.Lock()
	c.token = nil
	c.mu.Unlock()

	if c.tokenDir != "" {
		tokenPath := filepath.Join(c.tokenDir, "google_drive_token.json")
		os.Remove(tokenPath)
	}

	return nil
}

// GetUserEmail returns the email of the authenticated user.
func (c *OAuth2Client) GetUserEmail(ctx context.Context) (string, error) {
	token, err := c.GetToken(ctx)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "GET", googleUserInfoURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("get user info: status %d", resp.StatusCode)
	}

	var userInfo struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return "", err
	}

	return userInfo.Email, nil
}
