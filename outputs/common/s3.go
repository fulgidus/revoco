package common

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/fulgidus/revoco/services/core"
)

// ── S3 Output ────────────────────────────────────────────────────────────────

// S3Output uploads files to S3-compatible object storage.
type S3Output struct {
	endpoint        string
	region          string
	bucket          string
	accessKeyID     string
	secretAccessKey string
	prefix          string
	client          *http.Client
}

// NewS3 creates a new S3 output.
func NewS3() *S3Output {
	return &S3Output{
		client: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}
}

func (o *S3Output) ID() string   { return "s3" }
func (o *S3Output) Name() string { return "S3 / MinIO" }
func (o *S3Output) Description() string {
	return "Upload to S3-compatible object storage (AWS S3, MinIO, etc.)"
}

func (o *S3Output) SupportedItemTypes() []string {
	return []string{"photo", "video", "audio", "document"}
}

func (o *S3Output) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "endpoint",
			Name:        "Endpoint",
			Description: "S3 endpoint URL (e.g., s3.amazonaws.com or minio.example.com)",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "region",
			Name:        "Region",
			Description: "AWS region (e.g., us-east-1)",
			Type:        "string",
			Default:     "us-east-1",
		},
		{
			ID:          "bucket",
			Name:        "Bucket",
			Description: "Target bucket name",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "access_key_id",
			Name:        "Access Key ID",
			Description: "AWS access key ID",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "secret_access_key",
			Name:        "Secret Access Key",
			Description: "AWS secret access key",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "prefix",
			Name:        "Key Prefix",
			Description: "Prefix for all object keys (e.g., photos/)",
			Type:        "string",
		},
	}
}

func (o *S3Output) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	if v, ok := cfg.Settings["endpoint"].(string); ok {
		o.endpoint = strings.TrimSuffix(v, "/")
	}
	if o.endpoint == "" {
		return fmt.Errorf("endpoint is required")
	}

	if v, ok := cfg.Settings["region"].(string); ok {
		o.region = v
	}
	if o.region == "" {
		o.region = "us-east-1"
	}

	if v, ok := cfg.Settings["bucket"].(string); ok {
		o.bucket = v
	}
	if o.bucket == "" {
		return fmt.Errorf("bucket is required")
	}

	if v, ok := cfg.Settings["access_key_id"].(string); ok {
		o.accessKeyID = v
	}
	if v, ok := cfg.Settings["secret_access_key"].(string); ok {
		o.secretAccessKey = v
	}
	if o.accessKeyID == "" || o.secretAccessKey == "" {
		return fmt.Errorf("access_key_id and secret_access_key are required")
	}

	if v, ok := cfg.Settings["prefix"].(string); ok {
		o.prefix = v
	}

	// Verify bucket access with a HEAD request
	return o.checkBucket(ctx)
}

func (o *S3Output) checkBucket(ctx context.Context) error {
	url := fmt.Sprintf("%s/%s", o.endpoint, o.bucket)
	req, err := http.NewRequestWithContext(ctx, "HEAD", url, nil)
	if err != nil {
		return err
	}

	o.signRequest(req, nil)

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("connect to S3: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("bucket %q not found", o.bucket)
	}
	if resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("access denied to bucket %q", o.bucket)
	}

	return nil
}

func (o *S3Output) Export(ctx context.Context, item core.ProcessedItem) error {
	f, err := os.Open(item.ProcessedPath)
	if err != nil {
		return fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return err
	}

	key := o.prefix + item.DestRelPath
	return o.putObject(ctx, key, f, stat.Size(), detectContentType(item.ProcessedPath))
}

func (o *S3Output) putObject(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	url := fmt.Sprintf("%s/%s/%s", o.endpoint, o.bucket, key)
	req, err := http.NewRequestWithContext(ctx, "PUT", url, body)
	if err != nil {
		return err
	}

	req.ContentLength = size
	req.Header.Set("Content-Type", contentType)

	// Read body for signing (S3 requires content hash)
	bodyBytes, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	req.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))

	o.signRequest(req, bodyBytes)

	resp, err := o.client.Do(req)
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload failed (%d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// signRequest adds AWS Signature Version 4 headers to the request.
func (o *S3Output) signRequest(req *http.Request, payload []byte) {
	t := time.Now().UTC()
	amzDate := t.Format("20060102T150405Z")
	dateStamp := t.Format("20060102")

	// Add required headers
	req.Header.Set("Host", req.URL.Host)
	req.Header.Set("X-Amz-Date", amzDate)

	// Calculate payload hash
	var payloadHash string
	if payload != nil {
		h := sha256.Sum256(payload)
		payloadHash = hex.EncodeToString(h[:])
	} else {
		payloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" // empty string hash
	}
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)

	// Create canonical request
	canonicalURI := req.URL.Path
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQueryString := req.URL.RawQuery

	// Signed headers
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalHeaders := fmt.Sprintf("host:%s\nx-amz-content-sha256:%s\nx-amz-date:%s\n",
		req.URL.Host, payloadHash, amzDate)

	canonicalRequest := fmt.Sprintf("%s\n%s\n%s\n%s\n%s\n%s",
		req.Method,
		canonicalURI,
		canonicalQueryString,
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	)

	// Create string to sign
	credentialScope := fmt.Sprintf("%s/%s/s3/aws4_request", dateStamp, o.region)
	hashedCanonicalRequest := sha256.Sum256([]byte(canonicalRequest))
	stringToSign := fmt.Sprintf("AWS4-HMAC-SHA256\n%s\n%s\n%s",
		amzDate,
		credentialScope,
		hex.EncodeToString(hashedCanonicalRequest[:]),
	)

	// Calculate signature
	kDate := hmacSHA256([]byte("AWS4"+o.secretAccessKey), []byte(dateStamp))
	kRegion := hmacSHA256(kDate, []byte(o.region))
	kService := hmacSHA256(kRegion, []byte("s3"))
	kSigning := hmacSHA256(kService, []byte("aws4_request"))
	signature := hex.EncodeToString(hmacSHA256(kSigning, []byte(stringToSign)))

	// Add authorization header
	authHeader := fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=%s, Signature=%s",
		o.accessKeyID, credentialScope, signedHeaders, signature)
	req.Header.Set("Authorization", authHeader)
}

func hmacSHA256(key, data []byte) []byte {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	types := map[string]string{
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".webp": "image/webp",
		".heic": "image/heic",
		".mp4":  "video/mp4",
		".mov":  "video/quicktime",
		".avi":  "video/x-msvideo",
		".mkv":  "video/x-matroska",
		".webm": "video/webm",
		".mp3":  "audio/mpeg",
		".m4a":  "audio/mp4",
		".flac": "audio/flac",
		".json": "application/json",
	}
	if ct, ok := types[ext]; ok {
		return ct
	}
	return "application/octet-stream"
}

func (o *S3Output) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
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

func (o *S3Output) Finalize(ctx context.Context) error {
	return nil
}

// Streaming support
func (o *S3Output) SupportsStreaming() bool {
	return true
}

func (o *S3Output) ExportStream(ctx context.Context, item core.ProcessedItem, reader io.Reader, size int64) error {
	key := o.prefix + item.DestRelPath
	return o.putObject(ctx, key, reader, size, detectContentType(item.DestRelPath))
}

func init() {
	_ = core.RegisterOutput(NewS3())
}

// Helper for canonical headers
func canonicalHeaders(headers http.Header) (string, string) {
	var keys []string
	for k := range headers {
		keys = append(keys, strings.ToLower(k))
	}
	sort.Strings(keys)

	var canonical strings.Builder
	for _, k := range keys {
		canonical.WriteString(k)
		canonical.WriteString(":")
		canonical.WriteString(strings.TrimSpace(headers.Get(k)))
		canonical.WriteString("\n")
	}
	return canonical.String(), strings.Join(keys, ";")
}
