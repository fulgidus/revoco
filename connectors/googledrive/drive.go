package googledrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Drive API endpoints
const (
	driveAPIBase     = "https://www.googleapis.com/drive/v3"
	driveFilesURL    = driveAPIBase + "/files"
	driveAboutURL    = driveAPIBase + "/about"
	driveExportURL   = driveAPIBase + "/files/%s/export"
	driveDownloadURL = driveAPIBase + "/files/%s"
)

// Google Workspace MIME types (cloud-only documents)
const (
	MimeTypeGoogleDoc      = "application/vnd.google-apps.document"
	MimeTypeGoogleSheet    = "application/vnd.google-apps.spreadsheet"
	MimeTypeGoogleSlides   = "application/vnd.google-apps.presentation"
	MimeTypeGoogleDrawing  = "application/vnd.google-apps.drawing"
	MimeTypeGoogleForm     = "application/vnd.google-apps.form"
	MimeTypeGoogleSite     = "application/vnd.google-apps.site"
	MimeTypeGoogleMap      = "application/vnd.google-apps.map"
	MimeTypeGoogleFolder   = "application/vnd.google-apps.folder"
	MimeTypeGoogleShortcut = "application/vnd.google-apps.shortcut"
)

// Export formats for Google Workspace documents
type ExportFormat struct {
	MimeType  string
	Extension string
	Label     string
}

// Available export formats per document type
var (
	DocExportFormats = []ExportFormat{
		{"application/vnd.openxmlformats-officedocument.wordprocessingml.document", ".docx", "Microsoft Word"},
		{"application/vnd.oasis.opendocument.text", ".odt", "OpenDocument Text"},
		{"application/pdf", ".pdf", "PDF"},
		{"text/plain", ".txt", "Plain Text"},
		{"text/html", ".html", "HTML"},
		{"application/rtf", ".rtf", "Rich Text Format"},
		{"application/epub+zip", ".epub", "EPUB"},
	}

	SheetExportFormats = []ExportFormat{
		{"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", ".xlsx", "Microsoft Excel"},
		{"application/vnd.oasis.opendocument.spreadsheet", ".ods", "OpenDocument Spreadsheet"},
		{"application/pdf", ".pdf", "PDF"},
		{"text/csv", ".csv", "CSV"},
		{"text/tab-separated-values", ".tsv", "TSV"},
	}

	SlidesExportFormats = []ExportFormat{
		{"application/vnd.openxmlformats-officedocument.presentationml.presentation", ".pptx", "Microsoft PowerPoint"},
		{"application/vnd.oasis.opendocument.presentation", ".odp", "OpenDocument Presentation"},
		{"application/pdf", ".pdf", "PDF"},
		{"text/plain", ".txt", "Plain Text"},
	}

	DrawingExportFormats = []ExportFormat{
		{"image/svg+xml", ".svg", "SVG"},
		{"image/png", ".png", "PNG"},
		{"application/pdf", ".pdf", "PDF"},
		{"image/jpeg", ".jpg", "JPEG"},
	}
)

// DriveFile represents a file in Google Drive.
type DriveFile struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	MimeType        string            `json:"mimeType"`
	Size            int64             `json:"size,string,omitempty"`
	CreatedTime     time.Time         `json:"createdTime,omitempty"`
	ModifiedTime    time.Time         `json:"modifiedTime,omitempty"`
	Parents         []string          `json:"parents,omitempty"`
	WebViewLink     string            `json:"webViewLink,omitempty"`
	IconLink        string            `json:"iconLink,omitempty"`
	Owners          []DriveOwner      `json:"owners,omitempty"`
	MD5Checksum     string            `json:"md5Checksum,omitempty"`
	Trashed         bool              `json:"trashed,omitempty"`
	Capabilities    *FileCapabilities `json:"capabilities,omitempty"`
	ShortcutDetails *ShortcutDetails  `json:"shortcutDetails,omitempty"`
}

// DriveOwner represents a file owner.
type DriveOwner struct {
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
}

// FileCapabilities describes what can be done with a file.
type FileCapabilities struct {
	CanDownload bool `json:"canDownload"`
	CanEdit     bool `json:"canEdit"`
	CanCopy     bool `json:"canCopy"`
}

// ShortcutDetails contains info about a shortcut target.
type ShortcutDetails struct {
	TargetID       string `json:"targetId"`
	TargetMimeType string `json:"targetMimeType"`
}

// DriveClient provides access to the Google Drive API.
type DriveClient struct {
	oauth *OAuth2Client
	http  *http.Client
}

// NewDriveClient creates a new Drive API client.
func NewDriveClient(oauth *OAuth2Client) *DriveClient {
	return &DriveClient{
		oauth: oauth,
		http:  &http.Client{Timeout: 5 * time.Minute}, // Longer timeout for downloads
	}
}

// doRequest executes an authenticated request.
func (c *DriveClient) doRequest(ctx context.Context, method, url string, body io.Reader) (*http.Response, error) {
	token, err := c.oauth.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("get token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token.AccessToken)

	return c.http.Do(req)
}

// ListFiles lists files in Drive with optional query.
func (c *DriveClient) ListFiles(ctx context.Context, query string, pageToken string) (*FileList, error) {
	params := url.Values{
		"fields":   {"nextPageToken,files(id,name,mimeType,size,createdTime,modifiedTime,parents,webViewLink,md5Checksum,trashed,owners,capabilities,shortcutDetails)"},
		"pageSize": {"1000"},
		"orderBy":  {"folder,name"},
	}

	if query != "" {
		params.Set("q", query)
	}
	if pageToken != "" {
		params.Set("pageToken", pageToken)
	}

	reqURL := driveFilesURL + "?" + params.Encode()
	resp, err := c.doRequest(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list files: status %d: %s", resp.StatusCode, body)
	}

	var result FileList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

// FileList represents a paginated list of files.
type FileList struct {
	Files         []DriveFile `json:"files"`
	NextPageToken string      `json:"nextPageToken,omitempty"`
}

// AboutResponse contains Drive account information.
type AboutResponse struct {
	User         *DriveUser    `json:"user,omitempty"`
	StorageQuota *StorageQuota `json:"storageQuota,omitempty"`
}

// DriveUser represents the authenticated user.
type DriveUser struct {
	DisplayName  string `json:"displayName"`
	EmailAddress string `json:"emailAddress"`
	PhotoLink    string `json:"photoLink,omitempty"`
}

// StorageQuota represents Drive storage information.
type StorageQuota struct {
	Limit             int64 `json:"limit,string,omitempty"`
	Usage             int64 `json:"usage,string,omitempty"`
	UsageInDrive      int64 `json:"usageInDrive,string,omitempty"`
	UsageInDriveTrash int64 `json:"usageInDriveTrash,string,omitempty"`
}

// GetFile retrieves metadata for a single file.
func (c *DriveClient) GetFile(ctx context.Context, fileID string) (*DriveFile, error) {
	params := url.Values{
		"fields": {"id,name,mimeType,size,createdTime,modifiedTime,parents,webViewLink,md5Checksum,trashed,owners,capabilities,shortcutDetails"},
	}

	reqURL := fmt.Sprintf("%s/%s?%s", driveFilesURL, fileID, params.Encode())
	resp, err := c.doRequest(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get file: status %d: %s", resp.StatusCode, body)
	}

	var file DriveFile
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &file, nil
}

// GetAbout retrieves information about the user's Drive account.
// Useful for testing connectivity and verifying authentication.
func (c *DriveClient) GetAbout(ctx context.Context) (*AboutResponse, error) {
	params := url.Values{
		"fields": {"user(displayName,emailAddress,photoLink),storageQuota(limit,usage,usageInDrive,usageInDriveTrash)"},
	}

	reqURL := driveAboutURL + "?" + params.Encode()
	resp, err := c.doRequest(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get about: status %d: %s", resp.StatusCode, body)
	}

	var about AboutResponse
	if err := json.NewDecoder(resp.Body).Decode(&about); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &about, nil
}

// DownloadFile downloads a regular file (not Google Workspace).
func (c *DriveClient) DownloadFile(ctx context.Context, fileID string) (io.ReadCloser, error) {
	params := url.Values{
		"alt": {"media"},
	}

	reqURL := fmt.Sprintf("%s/%s?%s", driveFilesURL, fileID, params.Encode())
	resp, err := c.doRequest(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("download file: status %d: %s", resp.StatusCode, body)
	}

	return resp.Body, nil
}

// ExportFile exports a Google Workspace document to the specified MIME type.
func (c *DriveClient) ExportFile(ctx context.Context, fileID string, exportMimeType string) (io.ReadCloser, error) {
	params := url.Values{
		"mimeType": {exportMimeType},
	}

	reqURL := fmt.Sprintf(driveExportURL, fileID) + "?" + params.Encode()
	resp, err := c.doRequest(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("export file: status %d: %s", resp.StatusCode, body)
	}

	return resp.Body, nil
}

// GetFolderPath builds the full path for a file by traversing its parent folders.
func (c *DriveClient) GetFolderPath(ctx context.Context, fileID string, cache map[string]string) (string, error) {
	// Check cache first
	if path, ok := cache[fileID]; ok {
		return path, nil
	}

	file, err := c.GetFile(ctx, fileID)
	if err != nil {
		return "", err
	}

	// If no parents or is root, use the file name
	if len(file.Parents) == 0 {
		cache[fileID] = file.Name
		return file.Name, nil
	}

	// Get parent path recursively
	parentPath, err := c.GetFolderPath(ctx, file.Parents[0], cache)
	if err != nil {
		// If we can't get parent, just use current name
		cache[fileID] = file.Name
		return file.Name, nil
	}

	fullPath := parentPath + "/" + file.Name
	cache[fileID] = fullPath
	return fullPath, nil
}

// IsGoogleWorkspaceDoc returns true if the MIME type is a Google Workspace format.
func IsGoogleWorkspaceDoc(mimeType string) bool {
	switch mimeType {
	case MimeTypeGoogleDoc, MimeTypeGoogleSheet, MimeTypeGoogleSlides,
		MimeTypeGoogleDrawing, MimeTypeGoogleForm, MimeTypeGoogleSite,
		MimeTypeGoogleMap:
		return true
	}
	return false
}

// IsFolder returns true if the MIME type is a folder.
func IsFolder(mimeType string) bool {
	return mimeType == MimeTypeGoogleFolder
}

// IsShortcut returns true if the MIME type is a shortcut.
func IsShortcut(mimeType string) bool {
	return mimeType == MimeTypeGoogleShortcut
}

// GetExportFormats returns the available export formats for a MIME type.
func GetExportFormats(mimeType string) []ExportFormat {
	switch mimeType {
	case MimeTypeGoogleDoc:
		return DocExportFormats
	case MimeTypeGoogleSheet:
		return SheetExportFormats
	case MimeTypeGoogleSlides:
		return SlidesExportFormats
	case MimeTypeGoogleDrawing:
		return DrawingExportFormats
	default:
		return nil
	}
}

// GetDefaultExportFormat returns the default (first) export format for a type.
func GetDefaultExportFormat(mimeType string) *ExportFormat {
	formats := GetExportFormats(mimeType)
	if len(formats) > 0 {
		return &formats[0]
	}
	return nil
}

// GetExportFormatByExtension finds an export format by file extension.
func GetExportFormatByExtension(mimeType, ext string) *ExportFormat {
	ext = strings.ToLower(ext)
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	for _, fmt := range GetExportFormats(mimeType) {
		if fmt.Extension == ext {
			return &fmt
		}
	}
	return nil
}

// GetDocumentType returns a human-readable type name.
func GetDocumentType(mimeType string) string {
	switch mimeType {
	case MimeTypeGoogleDoc:
		return "Google Doc"
	case MimeTypeGoogleSheet:
		return "Google Sheet"
	case MimeTypeGoogleSlides:
		return "Google Slides"
	case MimeTypeGoogleDrawing:
		return "Google Drawing"
	case MimeTypeGoogleForm:
		return "Google Form"
	case MimeTypeGoogleSite:
		return "Google Site"
	case MimeTypeGoogleMap:
		return "Google Map"
	case MimeTypeGoogleFolder:
		return "Folder"
	case MimeTypeGoogleShortcut:
		return "Shortcut"
	default:
		return "File"
	}
}
