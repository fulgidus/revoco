package googledrive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	core "github.com/fulgidus/revoco/connectors"
)

// ══════════════════════════════════════════════════════════════════════════════
// Google Drive Connector
// ══════════════════════════════════════════════════════════════════════════════

// Connector implements the core.ConnectorReader interface for Google Drive.
// It can read both regular files and export Google Workspace documents.
type Connector struct {
	cfg       core.ConnectorConfig
	oauth     *OAuth2Client
	drive     *DriveClient
	pathCache map[string]string // Cache for folder path lookups
	exportFmt map[string]string // Document type -> export extension preference
}

// NewConnector creates a new Google Drive connector.
func NewConnector() *Connector {
	return &Connector{
		pathCache: make(map[string]string),
		exportFmt: make(map[string]string),
	}
}

// ── Identity ──────────────────────────────────────────────────────────────────

func (c *Connector) ID() string   { return "google-drive" }
func (c *Connector) Name() string { return "Google Drive" }
func (c *Connector) Description() string {
	return "Import files from Google Drive, including export of Google Docs/Sheets/Slides to local formats"
}

// ── Capabilities ──────────────────────────────────────────────────────────────

func (c *Connector) Capabilities() []core.ConnectorCapability {
	return []core.ConnectorCapability{
		core.CapabilityRead,
		core.CapabilityList,
		core.CapabilitySearch,
	}
}

func (c *Connector) SupportedDataTypes() []core.DataType {
	return []core.DataType{
		core.DataTypeDocument,
		core.DataTypePhoto,
		core.DataTypeVideo,
		core.DataTypeAudio,
		core.DataTypeUnknown,
	}
}

func (c *Connector) RequiresAuth() bool { return true }
func (c *Connector) AuthType() string   { return "oauth2" }

// ── Configuration ─────────────────────────────────────────────────────────────

func (c *Connector) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "client_id",
			Name:        "Client ID",
			Description: "Google OAuth2 Client ID (from Google Cloud Console)",
			Type:        "string",
			Required:    true,
			Sensitive:   true,
		},
		{
			ID:          "client_secret",
			Name:        "Client Secret",
			Description: "Google OAuth2 Client Secret",
			Type:        "password",
			Required:    true,
			Sensitive:   true,
		},
		{
			ID:          "folder_id",
			Name:        "Folder ID",
			Description: "Specific folder ID to import from (empty = entire Drive)",
			Type:        "string",
			Required:    false,
		},
		{
			ID:          "include_shared",
			Name:        "Include Shared Files",
			Description: "Include files shared with you (not just owned files)",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "include_trashed",
			Name:        "Include Trashed Files",
			Description: "Include files in the trash",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "doc_format",
			Name:        "Google Docs Export Format",
			Description: "Format to export Google Docs as",
			Type:        "select",
			Default:     ".docx",
			Options:     []string{".docx", ".odt", ".pdf", ".txt", ".html", ".rtf", ".epub"},
		},
		{
			ID:          "sheet_format",
			Name:        "Google Sheets Export Format",
			Description: "Format to export Google Sheets as",
			Type:        "select",
			Default:     ".xlsx",
			Options:     []string{".xlsx", ".ods", ".pdf", ".csv", ".tsv"},
		},
		{
			ID:          "slides_format",
			Name:        "Google Slides Export Format",
			Description: "Format to export Google Slides as",
			Type:        "select",
			Default:     ".pptx",
			Options:     []string{".pptx", ".odp", ".pdf", ".txt"},
		},
		{
			ID:          "drawing_format",
			Name:        "Google Drawings Export Format",
			Description: "Format to export Google Drawings as",
			Type:        "select",
			Default:     ".svg",
			Options:     []string{".svg", ".png", ".pdf", ".jpg"},
		},
	}
}

func (c *Connector) ValidateConfig(cfg core.ConnectorConfig) error {
	clientID, _ := cfg.Settings["client_id"].(string)
	if clientID == "" {
		return fmt.Errorf("google drive: client_id is required")
	}

	clientSecret, _ := cfg.Settings["client_secret"].(string)
	if clientSecret == "" {
		return fmt.Errorf("google drive: client_secret is required")
	}

	return nil
}

func (c *Connector) FallbackOptions() []core.FallbackOption {
	return []core.FallbackOption{
		{
			ConnectorID:       "google-photos-api",
			Name:              "Google Photos API",
			Description:       "Use Google Photos API to repair missing media files",
			SetupInstructions: "Enable Google Photos API in Google Cloud Console and grant access",
			RequiredCapabilities: []core.ConnectorCapability{
				core.CapabilityRead,
				core.CapabilityRepair,
			},
		},
	}
}

// SetupInstructions returns detailed setup instructions for Google Drive OAuth.
func (c *Connector) SetupInstructions() string {
	return `# Google Drive Setup

To use Google Drive, you need to create OAuth2 credentials in Google Cloud Console.

## Step 1: Create a Google Cloud Project

1. Go to: https://console.cloud.google.com/
2. Click "Select a project" → "New Project"
3. Enter a name (e.g., "Revoco Backup") and click "Create"

## Step 2: Enable Google Drive API

1. In your project, go to "APIs & Services" → "Library"
2. Search for "Google Drive API"
3. Click on it and press "Enable"

## Step 3: Configure OAuth Consent Screen

1. Go to "APIs & Services" → "OAuth consent screen"
2. Select "External" (or "Internal" if using Google Workspace)
3. Fill in the required fields:
   - App name: "Revoco"
   - User support email: your email
   - Developer contact: your email
4. Click "Save and Continue"

5. On the "Scopes" page:
   - Click "Add or Remove Scopes"
   - Find and add: https://www.googleapis.com/auth/drive.readonly
   - Click "Update" then "Save and Continue"

6. IMPORTANT - On the "Test users" page:
   - Click "Add Users"
   - Enter YOUR Google account email address (the one you'll use to access Drive)
   - Click "Add"
   - Click "Save and Continue"

Note: While your app is in "Testing" status, ONLY users added as test users
can authorize the app. If you skip this step, authorization will fail with
"Access blocked: This app's request is invalid" error.

## Step 4: Create OAuth Credentials

1. Go to "APIs & Services" → "Credentials"
2. Click "Create Credentials" → "OAuth client ID"
3. Application type: "Desktop app"
4. Name: "Revoco Desktop"
5. Click "Create"
6. Copy the Client ID and Client Secret shown

## Step 5: Enter Credentials Below

Paste the Client ID and Client Secret in the settings page.
When you test or use the connector, a browser window will open for authorization.

Note: Your credentials are stored locally and never sent anywhere except Google.
The app will remain in "Testing" mode - you don't need to publish it.
`
}

// Ensure Connector implements ConnectorWithSetup
var _ core.ConnectorWithSetup = (*Connector)(nil)

// TestConnection verifies the OAuth credentials and Drive API access.
// This will trigger the OAuth flow if no valid token is cached.
func (c *Connector) TestConnection(ctx context.Context, cfg core.ConnectorConfig) error {
	// Initialize if not already done
	if c.oauth == nil {
		if err := c.Initialize(ctx, cfg); err != nil {
			return fmt.Errorf("initialization failed: %w", err)
		}
	}

	// Try to get a valid token - if we have one, just verify it works
	token, err := c.oauth.GetToken(ctx)
	if err != nil {
		// No valid token - need to run OAuth flow
		token, err = c.runOAuthFlow(ctx)
		if err != nil {
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Verify the token works by making an API call
	if token != nil {
		resp, err := c.drive.GetAbout(ctx)
		if err != nil {
			return fmt.Errorf("API access failed: %w", err)
		}

		// Check we got valid user info
		if resp.User == nil || resp.User.EmailAddress == "" {
			return fmt.Errorf("could not verify account information")
		}
	}

	return nil
}

// runOAuthFlow starts the OAuth flow and opens the browser for user authentication.
func (c *Connector) runOAuthFlow(ctx context.Context) (*OAuth2Token, error) {
	// Start the OAuth flow
	authURL, tokenChan, errChan, err := c.oauth.StartAuthFlow(ctx)
	if err != nil {
		return nil, fmt.Errorf("start auth flow: %w", err)
	}

	// Open browser for user to authenticate
	if err := openBrowser(authURL); err != nil {
		// If we can't open browser, print the URL
		fmt.Printf("\nPlease open this URL in your browser to authenticate:\n%s\n\n", authURL)
	}

	// Wait for callback or timeout
	select {
	case token := <-tokenChan:
		if token != nil {
			return token, nil
		}
		return nil, errors.New("no token received")
	case err := <-errChan:
		if err != nil {
			return nil, err
		}
		return nil, errors.New("authentication cancelled")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// openBrowser opens the URL in the default browser.
func openBrowser(url string) error {
	var cmd string
	var args []string

	switch {
	case isWSL():
		cmd = "wslview"
		args = []string{url}
	case fileExists("/usr/bin/xdg-open"):
		cmd = "xdg-open"
		args = []string{url}
	case fileExists("/usr/bin/open"):
		cmd = "open"
		args = []string{url}
	default:
		// Try xdg-open anyway
		cmd = "xdg-open"
		args = []string{url}
	}

	return execCommand(cmd, args...)
}

// isWSL checks if we're running in Windows Subsystem for Linux.
func isWSL() bool {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft")
}

// fileExists checks if a file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// execCommand executes a command and returns any error.
func execCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start() // Don't wait for browser to close
}

// Ensure Connector implements ConnectorTester interface
var _ core.ConnectorTester = (*Connector)(nil)

// ── Reader Implementation ─────────────────────────────────────────────────────

func (c *Connector) Initialize(ctx context.Context, cfg core.ConnectorConfig) error {
	if err := c.ValidateConfig(cfg); err != nil {
		return err
	}

	c.cfg = cfg

	// Extract OAuth config
	clientID := cfg.Settings["client_id"].(string)
	clientSecret := cfg.Settings["client_secret"].(string)

	// Determine token directory - use user's home directory for persistence
	// Tokens in /tmp would be lost on reboot
	tokenDir := ""
	homeDir, err := os.UserHomeDir()
	if err == nil {
		if cfg.CredentialID != "" {
			tokenDir = filepath.Join(homeDir, ".revoco", "tokens", cfg.CredentialID)
		} else {
			// Fallback: use connector instance ID if no credential ID
			tokenDir = filepath.Join(homeDir, ".revoco", "tokens", "google-drive-"+cfg.InstanceID)
		}
	}

	// Create OAuth client
	c.oauth = NewOAuth2Client(OAuth2Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
	}, tokenDir)

	// Try to load cached token
	if err := c.oauth.LoadTokenFromCache(); err != nil {
		// Token not cached - will need to authenticate
		// This is fine, we'll handle it when GetToken is called
	}

	// Create Drive client
	c.drive = NewDriveClient(c.oauth)

	// Store export format preferences
	if ext, ok := cfg.Settings["doc_format"].(string); ok {
		c.exportFmt[MimeTypeGoogleDoc] = ext
	} else {
		c.exportFmt[MimeTypeGoogleDoc] = ".docx"
	}
	if ext, ok := cfg.Settings["sheet_format"].(string); ok {
		c.exportFmt[MimeTypeGoogleSheet] = ext
	} else {
		c.exportFmt[MimeTypeGoogleSheet] = ".xlsx"
	}
	if ext, ok := cfg.Settings["slides_format"].(string); ok {
		c.exportFmt[MimeTypeGoogleSlides] = ext
	} else {
		c.exportFmt[MimeTypeGoogleSlides] = ".pptx"
	}
	if ext, ok := cfg.Settings["drawing_format"].(string); ok {
		c.exportFmt[MimeTypeGoogleDrawing] = ext
	} else {
		c.exportFmt[MimeTypeGoogleDrawing] = ".svg"
	}

	return nil
}

// GetOAuth returns the OAuth client for external authentication handling.
func (c *Connector) GetOAuth() *OAuth2Client {
	return c.oauth
}

// List retrieves all files from Google Drive.
func (c *Connector) List(ctx context.Context, progress core.ProgressFunc) ([]core.DataItem, error) {
	// Build query - we need folders too for path resolution
	var queryParts []string

	// Exclude trashed files by default
	includeTrashed, _ := c.cfg.Settings["include_trashed"].(bool)
	if !includeTrashed {
		queryParts = append(queryParts, "trashed = false")
	}

	// Optionally filter to a specific folder
	rootFolderID := ""
	if folderID, ok := c.cfg.Settings["folder_id"].(string); ok && folderID != "" {
		rootFolderID = folderID
	}

	query := strings.Join(queryParts, " and ")

	// Phase 1: Fetch ALL files and folders in one pass
	var allFiles []DriveFile
	pageToken := ""

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		fileList, err := c.drive.ListFiles(ctx, query, pageToken)
		if err != nil {
			return nil, fmt.Errorf("list files: %w", err)
		}

		allFiles = append(allFiles, fileList.Files...)

		if progress != nil {
			progress(len(allFiles), -1) // Total unknown during pagination
		}

		if fileList.NextPageToken == "" {
			break
		}
		pageToken = fileList.NextPageToken
	}

	// Phase 2: Build folder map for local path resolution (no API calls)
	folderMap := make(map[string]*DriveFile) // ID -> folder info
	for i := range allFiles {
		if IsFolder(allFiles[i].MimeType) {
			folderMap[allFiles[i].ID] = &allFiles[i]
		}
	}

	// Phase 3: Convert to DataItems with locally-resolved paths
	items := make([]core.DataItem, 0, len(allFiles))
	total := len(allFiles)

	for i, file := range allFiles {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Skip folders, forms, sites, etc. that can't be downloaded
		if IsFolder(file.MimeType) || file.MimeType == MimeTypeGoogleForm ||
			file.MimeType == MimeTypeGoogleSite || file.MimeType == MimeTypeGoogleMap {
			continue
		}

		// Handle shortcuts by resolving to target
		if IsShortcut(file.MimeType) && file.ShortcutDetails != nil {
			// Skip shortcuts for now - they point to other files we should get directly
			continue
		}

		item := c.driveFileToDataItemFast(&file, folderMap, rootFolderID)
		items = append(items, item)

		if progress != nil {
			progress(i+1, total)
		}
	}

	return items, nil
}

// driveFileToDataItemFast converts a DriveFile to a core.DataItem using a local folder map.
// This avoids API calls by resolving folder paths from pre-fetched folder data.
func (c *Connector) driveFileToDataItemFast(file *DriveFile, folderMap map[string]*DriveFile, rootFolderID string) core.DataItem {
	// Build folder path locally without API calls
	var folderPath string
	if len(file.Parents) > 0 {
		folderPath = c.buildFolderPathLocal(file.Parents[0], folderMap, rootFolderID)
	}

	// Determine file name and extension for Google Workspace docs
	fileName := file.Name
	if IsGoogleWorkspaceDoc(file.MimeType) {
		ext := c.exportFmt[file.MimeType]
		if ext == "" {
			if fmt := GetDefaultExportFormat(file.MimeType); fmt != nil {
				ext = fmt.Extension
			}
		}
		// Only add extension if not already present
		if ext != "" && !strings.HasSuffix(strings.ToLower(fileName), strings.ToLower(ext)) {
			fileName = fileName + ext
		}
	}

	// Build relative path
	relativePath := fileName
	if folderPath != "" {
		relativePath = folderPath + "/" + fileName
	}

	item := core.DataItem{
		ID:           file.ID,
		Type:         detectDataTypeFromMime(file.MimeType),
		Path:         relativePath, // Relative path in Drive
		RemoteID:     file.ID,
		SourceConnID: c.cfg.InstanceID,
		Size:         file.Size,
		Checksum:     file.MD5Checksum,
		Metadata: map[string]any{
			"name":             file.Name,
			"mime_type":        file.MimeType,
			"created_time":     file.CreatedTime.Unix(),
			"modified_time":    file.ModifiedTime.Unix(),
			"web_view_link":    file.WebViewLink,
			"folder_path":      folderPath,
			"is_google_doc":    IsGoogleWorkspaceDoc(file.MimeType),
			"google_doc_type":  GetDocumentType(file.MimeType),
			"export_extension": c.exportFmt[file.MimeType],
		},
	}

	// Add owner info if available
	if len(file.Owners) > 0 {
		item.Metadata["owner_email"] = file.Owners[0].EmailAddress
		item.Metadata["owner_name"] = file.Owners[0].DisplayName
	}

	return item
}

// buildFolderPathLocal builds folder path using pre-fetched folder data (no API calls).
func (c *Connector) buildFolderPathLocal(folderID string, folderMap map[string]*DriveFile, rootFolderID string) string {
	// Check cache first
	if path, ok := c.pathCache[folderID]; ok {
		return path
	}

	// Stop if we've reached the root folder
	if rootFolderID != "" && folderID == rootFolderID {
		return ""
	}

	folder, ok := folderMap[folderID]
	if !ok {
		// Folder not in our map (might be root or shared folder)
		// Return empty to avoid partial paths
		return ""
	}

	// If no parents, this is a root-level folder
	if len(folder.Parents) == 0 {
		c.pathCache[folderID] = folder.Name
		return folder.Name
	}

	// Stop if parent is the root folder we're filtering to
	if rootFolderID != "" && len(folder.Parents) > 0 && folder.Parents[0] == rootFolderID {
		c.pathCache[folderID] = folder.Name
		return folder.Name
	}

	// Recursively build parent path
	parentPath := c.buildFolderPathLocal(folder.Parents[0], folderMap, rootFolderID)
	var fullPath string
	if parentPath != "" {
		fullPath = parentPath + "/" + folder.Name
	} else {
		fullPath = folder.Name
	}

	c.pathCache[folderID] = fullPath
	return fullPath
}

// driveFileToDataItem converts a DriveFile to a core.DataItem.
// Deprecated: Use driveFileToDataItemFast for better performance.
func (c *Connector) driveFileToDataItem(ctx context.Context, file *DriveFile) core.DataItem {
	// Get folder path for organizing output
	var folderPath string
	if len(file.Parents) > 0 {
		folderPath, _ = c.drive.GetFolderPath(ctx, file.Parents[0], c.pathCache)
	}

	// Determine file name and extension for Google Workspace docs
	fileName := file.Name
	if IsGoogleWorkspaceDoc(file.MimeType) {
		ext := c.exportFmt[file.MimeType]
		if ext == "" {
			if fmt := GetDefaultExportFormat(file.MimeType); fmt != nil {
				ext = fmt.Extension
			}
		}
		// Only add extension if not already present
		if ext != "" && !strings.HasSuffix(strings.ToLower(fileName), strings.ToLower(ext)) {
			fileName = fileName + ext
		}
	}

	// Build relative path
	relativePath := fileName
	if folderPath != "" {
		relativePath = folderPath + "/" + fileName
	}

	item := core.DataItem{
		ID:           file.ID,
		Type:         detectDataTypeFromMime(file.MimeType),
		Path:         relativePath, // Relative path in Drive
		RemoteID:     file.ID,
		SourceConnID: c.cfg.InstanceID,
		Size:         file.Size,
		Checksum:     file.MD5Checksum,
		Metadata: map[string]any{
			"name":             file.Name,
			"mime_type":        file.MimeType,
			"created_time":     file.CreatedTime.Unix(),
			"modified_time":    file.ModifiedTime.Unix(),
			"web_view_link":    file.WebViewLink,
			"folder_path":      folderPath,
			"is_google_doc":    IsGoogleWorkspaceDoc(file.MimeType),
			"google_doc_type":  GetDocumentType(file.MimeType),
			"export_extension": c.exportFmt[file.MimeType],
		},
	}

	// Add owner info if available
	if len(file.Owners) > 0 {
		item.Metadata["owner_email"] = file.Owners[0].EmailAddress
		item.Metadata["owner_name"] = file.Owners[0].DisplayName
	}

	return item
}

// Read retrieves a file's content as a stream.
func (c *Connector) Read(ctx context.Context, item core.DataItem) (io.ReadCloser, error) {
	fileID := item.RemoteID
	if fileID == "" {
		fileID = item.ID
	}

	mimeType, _ := item.Metadata["mime_type"].(string)

	// For Google Workspace docs, export to the preferred format
	if IsGoogleWorkspaceDoc(mimeType) {
		ext := c.exportFmt[mimeType]
		exportFmt := GetExportFormatByExtension(mimeType, ext)
		if exportFmt == nil {
			exportFmt = GetDefaultExportFormat(mimeType)
		}
		if exportFmt == nil {
			return nil, fmt.Errorf("no export format available for %s", mimeType)
		}

		return c.drive.ExportFile(ctx, fileID, exportFmt.MimeType)
	}

	// For regular files, download directly
	return c.drive.DownloadFile(ctx, fileID)
}

// ReadTo downloads a file to a local path.
func (c *Connector) ReadTo(ctx context.Context, item core.DataItem, destPath string, mode core.ImportMode) error {
	// Ensure destination directory exists
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return fmt.Errorf("create dest dir: %w", err)
	}

	// Get file content
	reader, err := c.Read(ctx, item)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}
	defer reader.Close()

	// Create destination file
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create file: %w", err)
	}
	defer f.Close()

	// Copy content
	_, err = io.Copy(f, reader)
	if err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	// Preserve modification time if available
	if modTime, ok := item.Metadata["modified_time"].(int64); ok && modTime > 0 {
		t := time.Unix(modTime, 0)
		os.Chtimes(destPath, t, t)
	}

	return nil
}

// Close releases resources.
func (c *Connector) Close() error {
	// Nothing to clean up for HTTP-based client
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Helpers
// ══════════════════════════════════════════════════════════════════════════════

func detectDataTypeFromMime(mimeType string) core.DataType {
	// Google Workspace documents
	switch mimeType {
	case MimeTypeGoogleDoc, MimeTypeGoogleSheet, MimeTypeGoogleSlides, MimeTypeGoogleDrawing:
		return core.DataTypeDocument
	}

	// Check MIME type prefix
	switch {
	case strings.HasPrefix(mimeType, "image/"):
		return core.DataTypePhoto
	case strings.HasPrefix(mimeType, "video/"):
		return core.DataTypeVideo
	case strings.HasPrefix(mimeType, "audio/"):
		return core.DataTypeAudio
	case strings.HasPrefix(mimeType, "application/pdf"),
		strings.HasPrefix(mimeType, "application/msword"),
		strings.HasPrefix(mimeType, "application/vnd.ms-"),
		strings.HasPrefix(mimeType, "application/vnd.openxmlformats-"),
		strings.HasPrefix(mimeType, "application/vnd.oasis.opendocument"),
		strings.HasPrefix(mimeType, "text/"):
		return core.DataTypeDocument
	}

	return core.DataTypeUnknown
}

// ══════════════════════════════════════════════════════════════════════════════
// Registration
// ══════════════════════════════════════════════════════════════════════════════

func init() {
	// Auto-register with global registry
	core.RegisterConnector(func() core.Connector {
		return NewConnector()
	})
}

// Ensure Connector implements ConnectorReader interface
var _ core.ConnectorReader = (*Connector)(nil)
