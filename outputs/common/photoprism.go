package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fulgidus/revoco/services/core"
)

// ── Photoprism Output ────────────────────────────────────────────────────────

// PhotoprismOutput uploads files to a Photoprism server via REST API.
type PhotoprismOutput struct {
	baseURL      string
	username     string
	password     string
	sessionToken string
	client       *http.Client
	createAlbums bool
	albumCache   map[string]string
}

// NewPhotoprism creates a new Photoprism output.
func NewPhotoprism() *PhotoprismOutput {
	return &PhotoprismOutput{
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
		albumCache: make(map[string]string),
	}
}

func (o *PhotoprismOutput) ID() string   { return "photoprism" }
func (o *PhotoprismOutput) Name() string { return "Photoprism" }
func (o *PhotoprismOutput) Description() string {
	return "Upload to Photoprism photo management server"
}

func (o *PhotoprismOutput) SupportedItemTypes() []string {
	return []string{"photo", "video"}
}

func (o *PhotoprismOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "server_url",
			Name:        "Server URL",
			Description: "Photoprism server URL (e.g., https://photos.example.com)",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "username",
			Name:        "Username",
			Description: "Photoprism username",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "password",
			Name:        "Password",
			Description: "Photoprism password",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "create_albums",
			Name:        "Create Albums",
			Description: "Automatically create albums based on source album metadata",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (o *PhotoprismOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	if url, ok := cfg.Settings["server_url"].(string); ok {
		o.baseURL = strings.TrimSuffix(url, "/")
	}
	if o.baseURL == "" {
		return fmt.Errorf("server_url is required")
	}

	if user, ok := cfg.Settings["username"].(string); ok {
		o.username = user
	}
	if pass, ok := cfg.Settings["password"].(string); ok {
		o.password = pass
	}
	if o.username == "" || o.password == "" {
		return fmt.Errorf("username and password are required")
	}

	if v, ok := cfg.Settings["create_albums"].(bool); ok {
		o.createAlbums = v
	} else {
		o.createAlbums = true
	}

	// Authenticate
	return o.authenticate(ctx)
}

func (o *PhotoprismOutput) authenticate(ctx context.Context) error {
	body, _ := json.Marshal(map[string]string{
		"username": o.username,
		"password": o.password,
	})

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/v1/session", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("authenticate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authentication failed: %d", resp.StatusCode)
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("parse auth response: %w", err)
	}

	o.sessionToken = result.ID
	return nil
}

func (o *PhotoprismOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// Upload the file
	fileUID, err := o.uploadFile(ctx, item)
	if err != nil {
		return err
	}

	// Add to album if configured
	albumName := ""
	if v, ok := item.Metadata["album"].(string); ok && v != "" {
		albumName = v
	}

	if o.createAlbums && albumName != "" {
		albumUID, err := o.getOrCreateAlbum(ctx, albumName)
		if err != nil {
			return fmt.Errorf("create album %s: %w", albumName, err)
		}
		return o.addToAlbum(ctx, albumUID, []string{fileUID})
	}

	return nil
}

func (o *PhotoprismOutput) uploadFile(ctx context.Context, item core.ProcessedItem) (string, error) {
	f, err := os.Open(item.ProcessedPath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	// Prepare multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("files", filepath.Base(item.ProcessedPath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", err
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/v1/upload/revoco", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-Session-ID", o.sessionToken)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(respBody))
	}

	// Trigger import
	importBody, _ := json.Marshal(map[string]any{
		"move": true,
		"path": "/upload/revoco",
	})
	req, err = http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/v1/import", bytes.NewReader(importBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", o.sessionToken)

	resp, err = o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("import request: %w", err)
	}
	resp.Body.Close()

	// Return a placeholder - Photoprism generates UIDs after import
	return filepath.Base(item.ProcessedPath), nil
}

func (o *PhotoprismOutput) getOrCreateAlbum(ctx context.Context, name string) (string, error) {
	if uid, ok := o.albumCache[name]; ok {
		return uid, nil
	}

	// Search for existing album
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/api/v1/albums?count=1000&q="+name, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("X-Session-ID", o.sessionToken)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var albums []struct {
		UID   string `json:"UID"`
		Title string `json:"Title"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&albums); err != nil {
		return "", err
	}

	for _, album := range albums {
		if album.Title == name {
			o.albumCache[name] = album.UID
			return album.UID, nil
		}
	}

	// Create new album
	createBody, _ := json.Marshal(map[string]string{"Title": name})
	req, err = http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/v1/albums", bytes.NewReader(createBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", o.sessionToken)

	resp, err = o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("create album failed: %d", resp.StatusCode)
	}

	var newAlbum struct {
		UID string `json:"UID"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&newAlbum); err != nil {
		return "", err
	}

	o.albumCache[name] = newAlbum.UID
	return newAlbum.UID, nil
}

func (o *PhotoprismOutput) addToAlbum(ctx context.Context, albumUID string, photoUIDs []string) error {
	body, _ := json.Marshal(map[string][]string{"photos": photoUIDs})
	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/v1/albums/"+albumUID+"/photos", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Session-ID", o.sessionToken)

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("add to album failed: %d", resp.StatusCode)
	}
	return nil
}

func (o *PhotoprismOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
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
	}
	return nil
}

func (o *PhotoprismOutput) Finalize(ctx context.Context) error {
	// Logout
	if o.sessionToken != "" {
		req, _ := http.NewRequest("DELETE", o.baseURL+"/api/v1/session/"+o.sessionToken, nil)
		o.client.Do(req) //nolint:errcheck
	}
	return nil
}

func init() {
	_ = core.RegisterOutput(NewPhotoprism())
}
