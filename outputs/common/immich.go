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

// ── Immich Output ────────────────────────────────────────────────────────────

// ImmichOutput uploads files to an Immich server via REST API.
type ImmichOutput struct {
	baseURL      string
	apiKey       string
	albumID      string // optional: upload to specific album
	client       *http.Client
	createAlbums bool
	albumCache   map[string]string // album name -> album ID
}

// NewImmich creates a new Immich output.
func NewImmich() *ImmichOutput {
	return &ImmichOutput{
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
		albumCache: make(map[string]string),
	}
}

func (o *ImmichOutput) ID() string          { return "immich" }
func (o *ImmichOutput) Name() string        { return "Immich" }
func (o *ImmichOutput) Description() string { return "Upload to Immich photo management server" }

func (o *ImmichOutput) SupportedItemTypes() []string {
	return []string{"photo", "video"}
}

func (o *ImmichOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "server_url",
			Name:        "Server URL",
			Description: "Immich server URL (e.g., https://photos.example.com)",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "api_key",
			Name:        "API Key",
			Description: "Immich API key for authentication",
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
		{
			ID:          "album_id",
			Name:        "Target Album",
			Description: "Upload all files to a specific album ID (optional)",
			Type:        "string",
		},
	}
}

func (o *ImmichOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	if url, ok := cfg.Settings["server_url"].(string); ok {
		o.baseURL = strings.TrimSuffix(url, "/")
	}
	if o.baseURL == "" {
		return fmt.Errorf("server_url is required")
	}

	if key, ok := cfg.Settings["api_key"].(string); ok {
		o.apiKey = key
	}
	if o.apiKey == "" {
		return fmt.Errorf("api_key is required")
	}

	if v, ok := cfg.Settings["create_albums"].(bool); ok {
		o.createAlbums = v
	} else {
		o.createAlbums = true
	}

	if id, ok := cfg.Settings["album_id"].(string); ok {
		o.albumID = id
	}

	// Validate connection
	return o.ping(ctx)
}

func (o *ImmichOutput) ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/api/server/ping", nil)
	if err != nil {
		return err
	}
	req.Header.Set("x-api-key", o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to Immich: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Immich server returned %d", resp.StatusCode)
	}
	return nil
}

func (o *ImmichOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	// Upload the asset
	assetID, err := o.uploadAsset(ctx, item)
	if err != nil {
		return err
	}

	// Add to album if configured
	albumName := ""
	if v, ok := item.Metadata["album"].(string); ok && v != "" {
		albumName = v
	}

	if o.albumID != "" {
		return o.addToAlbum(ctx, o.albumID, []string{assetID})
	}

	if o.createAlbums && albumName != "" {
		albumID, err := o.getOrCreateAlbum(ctx, albumName)
		if err != nil {
			return fmt.Errorf("create album %s: %w", albumName, err)
		}
		return o.addToAlbum(ctx, albumID, []string{assetID})
	}

	return nil
}

func (o *ImmichOutput) uploadAsset(ctx context.Context, item core.ProcessedItem) (string, error) {
	f, err := os.Open(item.ProcessedPath)
	if err != nil {
		return "", fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return "", err
	}

	// Prepare multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add device asset ID (unique identifier)
	deviceAssetID := filepath.Base(item.ProcessedPath)
	writer.WriteField("deviceAssetId", deviceAssetID)
	writer.WriteField("deviceId", "revoco")
	writer.WriteField("fileCreatedAt", stat.ModTime().UTC().Format(time.RFC3339))
	writer.WriteField("fileModifiedAt", stat.ModTime().UTC().Format(time.RFC3339))

	// Add the file
	part, err := writer.CreateFormFile("assetData", filepath.Base(item.ProcessedPath))
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", err
	}
	writer.Close()

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/assets", body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("x-api-key", o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("upload request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	return result.ID, nil
}

func (o *ImmichOutput) getOrCreateAlbum(ctx context.Context, name string) (string, error) {
	// Check cache first
	if id, ok := o.albumCache[name]; ok {
		return id, nil
	}

	// Search for existing album
	req, err := http.NewRequestWithContext(ctx, "GET", o.baseURL+"/api/albums", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("x-api-key", o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var albums []struct {
		ID        string `json:"id"`
		AlbumName string `json:"albumName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&albums); err != nil {
		return "", err
	}

	for _, album := range albums {
		if album.AlbumName == name {
			o.albumCache[name] = album.ID
			return album.ID, nil
		}
	}

	// Create new album
	createBody, _ := json.Marshal(map[string]string{"albumName": name})
	req, err = http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/albums", bytes.NewReader(createBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", o.apiKey)

	resp, err = o.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("create album failed: %d", resp.StatusCode)
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

func (o *ImmichOutput) addToAlbum(ctx context.Context, albumID string, assetIDs []string) error {
	body, _ := json.Marshal(map[string][]string{"ids": assetIDs})
	req, err := http.NewRequestWithContext(ctx, "PUT", o.baseURL+"/api/albums/"+albumID+"/assets", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", o.apiKey)

	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("add to album failed: %d", resp.StatusCode)
	}
	return nil
}

func (o *ImmichOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
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

func (o *ImmichOutput) Finalize(ctx context.Context) error {
	return nil
}

func init() {
	_ = core.RegisterOutput(NewImmich())
}
