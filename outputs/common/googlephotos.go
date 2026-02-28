package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fulgidus/revoco/services/core"
)

// ── Google Photos API Output ─────────────────────────────────────────────────

// GooglePhotosOutput uploads files to Google Photos via the API.
// Supports both OAuth and cookie-based authentication.
type GooglePhotosOutput struct {
	client       *http.Client
	authType     string // "oauth" or "cookie"
	accessToken  string
	refreshToken string
	clientID     string
	clientSecret string
	cookies      []*http.Cookie
	createAlbums bool
	albumCache   map[string]string
}

// NewGooglePhotos creates a new Google Photos API output.
func NewGooglePhotos() *GooglePhotosOutput {
	jar, _ := cookiejar.New(nil)
	return &GooglePhotosOutput{
		client: &http.Client{
			Timeout: 10 * time.Minute,
			Jar:     jar,
		},
		albumCache: make(map[string]string),
	}
}

func (o *GooglePhotosOutput) ID() string   { return "google-photos-api" }
func (o *GooglePhotosOutput) Name() string { return "Google Photos" }
func (o *GooglePhotosOutput) Description() string {
	return "Upload to Google Photos via API (OAuth or cookie auth)"
}

func (o *GooglePhotosOutput) SupportedItemTypes() []string {
	return []string{"photo", "video"}
}

func (o *GooglePhotosOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "auth_type",
			Name:        "Auth Type",
			Description: "Authentication method",
			Type:        "select",
			Options:     []string{"oauth", "cookie"},
			Default:     "oauth",
		},
		{
			ID:          "client_id",
			Name:        "OAuth Client ID",
			Description: "Google OAuth 2.0 client ID (for OAuth auth)",
			Type:        "string",
		},
		{
			ID:          "client_secret",
			Name:        "OAuth Client Secret",
			Description: "Google OAuth 2.0 client secret (for OAuth auth)",
			Type:        "string",
		},
		{
			ID:          "refresh_token",
			Name:        "Refresh Token",
			Description: "OAuth refresh token (obtained after initial auth)",
			Type:        "string",
		},
		{
			ID:          "cookie_file",
			Name:        "Cookie File",
			Description: "Path to exported cookies file (for cookie auth)",
			Type:        "string",
		},
		{
			ID:          "create_albums",
			Name:        "Create Albums",
			Description: "Automatically create albums based on source metadata",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (o *GooglePhotosOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	if v, ok := cfg.Settings["auth_type"].(string); ok {
		o.authType = v
	}
	if o.authType == "" {
		o.authType = "oauth"
	}

	if v, ok := cfg.Settings["create_albums"].(bool); ok {
		o.createAlbums = v
	} else {
		o.createAlbums = true
	}

	switch o.authType {
	case "oauth":
		return o.initOAuth(ctx, cfg.Settings)
	case "cookie":
		return o.initCookie(ctx, cfg.Settings)
	default:
		return fmt.Errorf("unknown auth_type: %s", o.authType)
	}
}

func (o *GooglePhotosOutput) initOAuth(ctx context.Context, settings map[string]any) error {
	if v, ok := settings["client_id"].(string); ok {
		o.clientID = v
	}
	if v, ok := settings["client_secret"].(string); ok {
		o.clientSecret = v
	}
	if v, ok := settings["refresh_token"].(string); ok {
		o.refreshToken = v
	}

	if o.clientID == "" || o.clientSecret == "" {
		return fmt.Errorf("client_id and client_secret required for OAuth")
	}
	if o.refreshToken == "" {
		return fmt.Errorf("refresh_token required - run OAuth flow first")
	}

	return o.refreshAccessToken(ctx)
}

func (o *GooglePhotosOutput) refreshAccessToken(ctx context.Context) error {
	data := url.Values{}
	data.Set("client_id", o.clientID)
	data.Set("client_secret", o.clientSecret)
	data.Set("refresh_token", o.refreshToken)
	data.Set("grant_type", "refresh_token")

	req, err := http.NewRequestWithContext(ctx, "POST", "https://oauth2.googleapis.com/token",
		strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("refresh token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh token failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	o.accessToken = result.AccessToken
	return nil
}

func (o *GooglePhotosOutput) initCookie(ctx context.Context, settings map[string]any) error {
	cookieFile := ""
	if v, ok := settings["cookie_file"].(string); ok {
		cookieFile = v
	}

	if cookieFile == "" {
		return fmt.Errorf("cookie_file required for cookie auth")
	}

	// Read cookies from file (Netscape format)
	data, err := os.ReadFile(cookieFile)
	if err != nil {
		return fmt.Errorf("read cookie file: %w", err)
	}

	cookies, err := parseCookieFile(data)
	if err != nil {
		return fmt.Errorf("parse cookies: %w", err)
	}

	// Set cookies on the jar
	u, _ := url.Parse("https://photos.google.com")
	o.client.Jar.SetCookies(u, cookies)
	o.cookies = cookies

	// Verify authentication
	return o.verifyCookieAuth(ctx)
}

func parseCookieFile(data []byte) ([]*http.Cookie, error) {
	var cookies []*http.Cookie
	lines := strings.Split(string(data), "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "\t")
		if len(parts) < 7 {
			continue
		}

		expires, _ := time.Parse("2006-01-02T15:04:05Z", parts[4])
		cookies = append(cookies, &http.Cookie{
			Domain:   parts[0],
			Path:     parts[2],
			Secure:   parts[3] == "TRUE",
			Expires:  expires,
			Name:     parts[5],
			Value:    parts[6],
			HttpOnly: true,
		})
	}

	if len(cookies) == 0 {
		return nil, fmt.Errorf("no valid cookies found")
	}
	return cookies, nil
}

func (o *GooglePhotosOutput) verifyCookieAuth(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://photos.google.com", nil)
	if err != nil {
		return err
	}

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("verify cookies: %w", err)
	}
	defer resp.Body.Close()

	// Check if we're redirected to login
	if strings.Contains(resp.Request.URL.String(), "accounts.google.com") {
		return fmt.Errorf("cookies expired or invalid - login required")
	}

	return nil
}

func (o *GooglePhotosOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// Step 1: Get upload token
	uploadToken, err := o.uploadBytes(ctx, item)
	if err != nil {
		return fmt.Errorf("upload: %w", err)
	}

	// Step 2: Create media item
	mediaItemID, err := o.createMediaItem(ctx, uploadToken, filepath.Base(item.ProcessedPath))
	if err != nil {
		return fmt.Errorf("create media item: %w", err)
	}

	// Step 3: Add to album if needed
	albumName := ""
	if v, ok := item.Metadata["album"].(string); ok && v != "" {
		albumName = v
	}

	if o.createAlbums && albumName != "" {
		albumID, err := o.getOrCreateAlbum(ctx, albumName)
		if err != nil {
			return fmt.Errorf("get/create album: %w", err)
		}
		return o.addToAlbum(ctx, albumID, []string{mediaItemID})
	}

	return nil
}

func (o *GooglePhotosOutput) uploadBytes(ctx context.Context, item core.ProcessedItem) (string, error) {
	f, err := os.Open(item.ProcessedPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://photoslibrary.googleapis.com/v1/uploads", f)
	if err != nil {
		return "", err
	}

	o.addAuthHeaders(req)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("X-Goog-Upload-Content-Type", detectContentType(item.ProcessedPath))
	req.Header.Set("X-Goog-Upload-Protocol", "raw")
	req.ContentLength = stat.Size()

	resp, err := o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(body))
	}

	uploadToken, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(uploadToken), nil
}

func (o *GooglePhotosOutput) createMediaItem(ctx context.Context, uploadToken, filename string) (string, error) {
	body := map[string]any{
		"newMediaItems": []map[string]any{
			{
				"description": "",
				"simpleMediaItem": map[string]string{
					"uploadToken": uploadToken,
					"fileName":    filename,
				},
			},
		},
	}

	jsonBody, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, "POST",
		"https://photoslibrary.googleapis.com/v1/mediaItems:batchCreate",
		bytes.NewReader(jsonBody))
	if err != nil {
		return "", err
	}

	o.addAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create media item failed (%d): %s", resp.StatusCode, string(body))
	}

	var result struct {
		NewMediaItemResults []struct {
			MediaItem struct {
				ID string `json:"id"`
			} `json:"mediaItem"`
			Status struct {
				Message string `json:"message"`
			} `json:"status"`
		} `json:"newMediaItemResults"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	if len(result.NewMediaItemResults) == 0 {
		return "", fmt.Errorf("no media item created")
	}

	return result.NewMediaItemResults[0].MediaItem.ID, nil
}

func (o *GooglePhotosOutput) getOrCreateAlbum(ctx context.Context, name string) (string, error) {
	if id, ok := o.albumCache[name]; ok {
		return id, nil
	}

	// Search for existing album
	req, err := http.NewRequestWithContext(ctx, "GET",
		"https://photoslibrary.googleapis.com/v1/albums?pageSize=50", nil)
	if err != nil {
		return "", err
	}
	o.addAuthHeaders(req)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var albumList struct {
		Albums []struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"albums"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&albumList); err != nil {
		return "", err
	}

	for _, album := range albumList.Albums {
		if album.Title == name {
			o.albumCache[name] = album.ID
			return album.ID, nil
		}
	}

	// Create new album
	createBody, _ := json.Marshal(map[string]any{
		"album": map[string]string{"title": name},
	})
	req, err = http.NewRequestWithContext(ctx, "POST",
		"https://photoslibrary.googleapis.com/v1/albums",
		bytes.NewReader(createBody))
	if err != nil {
		return "", err
	}
	o.addAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err = o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("create album failed (%d): %s", resp.StatusCode, string(body))
	}

	var newAlbum struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&newAlbum); err != nil {
		return "", err
	}

	o.albumCache[name] = newAlbum.ID
	return newAlbum.ID, nil
}

func (o *GooglePhotosOutput) addToAlbum(ctx context.Context, albumID string, mediaItemIDs []string) error {
	body, _ := json.Marshal(map[string][]string{"mediaItemIds": mediaItemIDs})
	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("https://photoslibrary.googleapis.com/v1/albums/%s:batchAddMediaItems", albumID),
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	o.addAuthHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add to album failed (%d): %s", resp.StatusCode, string(body))
	}
	return nil
}

func (o *GooglePhotosOutput) addAuthHeaders(req *http.Request) {
	if o.authType == "oauth" && o.accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+o.accessToken)
	}
	// Cookie auth uses the cookie jar automatically
}

func (o *GooglePhotosOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	total := len(items)
	for i, item := range items {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := o.Export(ctx, item); err != nil {
			return fmt.Errorf("export %s: %w", item.DestRelPath, err)
		}

		if progress != nil {
			progress(i+1, total)
		}

		// Rate limiting for Google Photos API
		time.Sleep(500 * time.Millisecond)
	}
	return nil
}

func (o *GooglePhotosOutput) Finalize(ctx context.Context) error {
	return nil
}

func init() {
	_ = core.RegisterOutput(NewGooglePhotos())
}
