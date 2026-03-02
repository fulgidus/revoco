package ingesters

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fulgidus/revoco/services/core"
)

// ── Factory Tests ────────────────────────────────────────────────────────────

func TestNewServiceIngesters(t *testing.T) {
	detector := NewServiceFolderDetector([]string{"Test Service"})
	ingesters := NewServiceIngesters("test-service", detector)

	if len(ingesters) != 3 {
		t.Fatalf("expected 3 ingesters, got %d", len(ingesters))
	}

	// Check IDs are properly prefixed
	expectedIDs := []string{"test-service-folder", "test-service-zip", "test-service-tgz"}
	for i, ing := range ingesters {
		if ing.ID() != expectedIDs[i] {
			t.Errorf("ingester %d: expected ID %q, got %q", i, expectedIDs[i], ing.ID())
		}
	}
}

func TestNewServiceFolderDetector(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test service folder
	serviceDir := filepath.Join(tmpDir, "My Service")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Create detector with case-insensitive variants
	detector := NewServiceFolderDetector([]string{"My Service", "my service", "MY SERVICE"})

	// Test direct match
	if !detector(serviceDir) {
		t.Error("detector failed to match service folder directly")
	}

	// Test nested match
	parentDir := filepath.Join(tmpDir, "parent")
	nestedService := filepath.Join(parentDir, "subdir", "My Service")
	if err := os.MkdirAll(nestedService, 0755); err != nil {
		t.Fatal(err)
	}

	if !detector(parentDir) {
		t.Error("detector failed to match nested service folder")
	}

	// Test non-match
	emptyDir := filepath.Join(tmpDir, "empty")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatal(err)
	}

	if detector(emptyDir) {
		t.Error("detector incorrectly matched empty folder")
	}
}

// ── Folder Ingester Tests ────────────────────────────────────────────────────

func TestFolderIngester_CanIngest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a service folder
	serviceDir := filepath.Join(tmpDir, "Test Service")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		t.Fatal(err)
	}

	detector := NewServiceFolderDetector([]string{"Test Service"})
	ing := newFolderIngester("test", detector)

	if !ing.CanIngest(serviceDir) {
		t.Error("CanIngest returned false for valid service folder")
	}

	// Test non-directory
	file := filepath.Join(tmpDir, "file.txt")
	if err := os.WriteFile(file, []byte("test"), 0644); err != nil {
		t.Fatal(err)
	}
	if ing.CanIngest(file) {
		t.Error("CanIngest returned true for non-directory")
	}

	// Test non-service folder
	otherDir := filepath.Join(tmpDir, "Other")
	if err := os.MkdirAll(otherDir, 0755); err != nil {
		t.Fatal(err)
	}
	if ing.CanIngest(otherDir) {
		t.Error("CanIngest returned true for non-service folder")
	}
}

func TestFolderIngester_Ingest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source folder with files
	srcDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(filepath.Join(srcDir, "subdir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "file1.txt"), []byte("content1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "subdir", "file2.txt"), []byte("content2"), 0644); err != nil {
		t.Fatal(err)
	}

	// Ingest
	destDir := filepath.Join(tmpDir, "dest")
	detector := func(string) bool { return true }
	ing := newFolderIngester("test", detector)

	progressCalls := 0
	progress := func(done, total int) {
		progressCalls++
		if done > total {
			t.Errorf("progress: done (%d) > total (%d)", done, total)
		}
	}

	ctx := context.Background()
	result, err := ing.Ingest(ctx, srcDir, destDir, progress)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if result != destDir {
		t.Errorf("expected result %q, got %q", destDir, result)
	}

	// Verify files were copied
	verifyFileContent(t, filepath.Join(destDir, "file1.txt"), "content1")
	verifyFileContent(t, filepath.Join(destDir, "subdir", "file2.txt"), "content2")

	// Verify progress was called (2 files)
	if progressCalls != 2 {
		t.Errorf("expected 2 progress calls, got %d", progressCalls)
	}
}

func TestFolderIngester_Ingest_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create source with many files
	srcDir := filepath.Join(tmpDir, "source")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		filename := fmt.Sprintf("file%03d.txt", i)
		path := filepath.Join(srcDir, filename)
		if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	destDir := filepath.Join(tmpDir, "dest")
	detector := func(string) bool { return true }
	ing := newFolderIngester("test", detector)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ing.Ingest(ctx, srcDir, destDir, nil)
	if err == nil {
		t.Error("expected context cancellation error")
	}
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("expected context cancellation error, got %v", err)
	}
}

// ── ZIP Ingester Tests ───────────────────────────────────────────────────────

func TestZipIngester_CanIngest(t *testing.T) {
	ing := newZipIngester("test")

	tests := []struct {
		path string
		want bool
	}{
		{"archive.zip", true},
		{"ARCHIVE.ZIP", true},
		{"archive.tar.gz", false},
		{"archive.tgz", false},
		{"archive.txt", false},
		{"", false},
	}

	for _, tt := range tests {
		got := ing.CanIngest(tt.path)
		if got != tt.want {
			t.Errorf("CanIngest(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestZipIngester_Ingest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a ZIP archive
	zipPath := filepath.Join(tmpDir, "test.zip")
	if err := createTestZip(zipPath, map[string]string{
		"file1.txt":        "content1",
		"subdir/file2.txt": "content2",
	}); err != nil {
		t.Fatal(err)
	}

	// Ingest
	destDir := filepath.Join(tmpDir, "dest")
	ing := newZipIngester("test")

	progressCalls := 0
	progress := func(done, total int) {
		progressCalls++
	}

	ctx := context.Background()
	result, err := ing.Ingest(ctx, zipPath, destDir, progress)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if result != destDir {
		t.Errorf("expected result %q, got %q", destDir, result)
	}

	// Verify files were extracted
	verifyFileContent(t, filepath.Join(destDir, "file1.txt"), "content1")
	verifyFileContent(t, filepath.Join(destDir, "subdir", "file2.txt"), "content2")

	// Progress should be called for each file
	if progressCalls < 2 {
		t.Errorf("expected at least 2 progress calls, got %d", progressCalls)
	}
}

func TestZipIngester_ZipSlip(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a malicious ZIP with path traversal
	zipPath := filepath.Join(tmpDir, "malicious.zip")
	if err := createTestZip(zipPath, map[string]string{
		"../../etc/passwd":      "hacked",
		"../escape.txt":         "escaped",
		"safe.txt":              "safe content",
		"subdir/../../evil.txt": "traversal",
	}); err != nil {
		t.Fatal(err)
	}

	// Ingest
	destDir := filepath.Join(tmpDir, "dest")
	ing := newZipIngester("test")

	ctx := context.Background()
	_, err := ing.Ingest(ctx, zipPath, destDir, nil)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	// Verify only safe file was extracted
	verifyFileContent(t, filepath.Join(destDir, "safe.txt"), "safe content")

	// Verify malicious files were NOT extracted
	evilPaths := []string{
		filepath.Join(tmpDir, "etc", "passwd"),
		filepath.Join(tmpDir, "escape.txt"),
		filepath.Join(tmpDir, "evil.txt"),
	}
	for _, p := range evilPaths {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("malicious file was extracted: %s", p)
		}
	}
}

func TestZipIngester_IngestMulti(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple ZIP archives
	zip1 := filepath.Join(tmpDir, "archive1.zip")
	if err := createTestZip(zip1, map[string]string{
		"file1.txt": "content1",
	}); err != nil {
		t.Fatal(err)
	}

	zip2 := filepath.Join(tmpDir, "archive2.zip")
	if err := createTestZip(zip2, map[string]string{
		"file2.txt": "content2",
	}); err != nil {
		t.Fatal(err)
	}

	// Ingest both
	destDir := filepath.Join(tmpDir, "dest")
	ing := newZipIngester("test")

	ctx := context.Background()
	result, err := ing.IngestMulti(ctx, []string{zip1, zip2}, destDir, nil)
	if err != nil {
		t.Fatalf("IngestMulti failed: %v", err)
	}

	if result != destDir {
		t.Errorf("expected result %q, got %q", destDir, result)
	}

	// Verify files from both archives
	verifyFileContent(t, filepath.Join(destDir, "file1.txt"), "content1")
	verifyFileContent(t, filepath.Join(destDir, "file2.txt"), "content2")
}

func TestZipIngester_IngestMulti_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a large ZIP
	zipPath := filepath.Join(tmpDir, "large.zip")
	files := make(map[string]string)
	for i := 0; i < 100; i++ {
		files[fmt.Sprintf("file%03d.txt", i)] = "data"
	}
	if err := createTestZip(zipPath, files); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(tmpDir, "dest")
	ing := newZipIngester("test")

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ing.IngestMulti(ctx, []string{zipPath}, destDir, nil)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

// ── TGZ Ingester Tests ───────────────────────────────────────────────────────

func TestTGZIngester_CanIngest(t *testing.T) {
	ing := newTGZIngester("test")

	tests := []struct {
		path string
		want bool
	}{
		{"archive.tgz", true},
		{"archive.tar.gz", true},
		{"ARCHIVE.TGZ", true},
		{"ARCHIVE.TAR.GZ", true},
		{"archive.zip", false},
		{"archive.tar", false},
		{"archive.gz", false},
		{"", false},
	}

	for _, tt := range tests {
		got := ing.CanIngest(tt.path)
		if got != tt.want {
			t.Errorf("CanIngest(%q) = %v, want %v", tt.path, got, tt.want)
		}
	}
}

func TestTGZIngester_Ingest(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a TGZ archive
	tgzPath := filepath.Join(tmpDir, "test.tgz")
	if err := createTestTGZ(tgzPath, map[string]string{
		"file1.txt":        "content1",
		"subdir/file2.txt": "content2",
	}); err != nil {
		t.Fatal(err)
	}

	// Ingest
	destDir := filepath.Join(tmpDir, "dest")
	ing := newTGZIngester("test")

	progressCalls := 0
	progress := func(done, total int) {
		progressCalls++
	}

	ctx := context.Background()
	result, err := ing.Ingest(ctx, tgzPath, destDir, progress)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if result != destDir {
		t.Errorf("expected result %q, got %q", destDir, result)
	}

	// Verify files were extracted
	verifyFileContent(t, filepath.Join(destDir, "file1.txt"), "content1")
	verifyFileContent(t, filepath.Join(destDir, "subdir", "file2.txt"), "content2")

	if progressCalls != 2 {
		t.Errorf("expected 2 progress calls, got %d", progressCalls)
	}
}

func TestTGZIngester_Ingest_PathTraversal(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a TGZ with path traversal attempts
	tgzPath := filepath.Join(tmpDir, "malicious.tgz")
	if err := createTestTGZ(tgzPath, map[string]string{
		"../../etc/passwd": "hacked",
		"../escape.txt":    "escaped",
		"safe.txt":         "safe content",
	}); err != nil {
		t.Fatal(err)
	}

	// Ingest
	destDir := filepath.Join(tmpDir, "dest")
	ing := newTGZIngester("test")

	ctx := context.Background()
	_, err := ing.Ingest(ctx, tgzPath, destDir, nil)
	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	// Verify only safe file was extracted
	verifyFileContent(t, filepath.Join(destDir, "safe.txt"), "safe content")

	// Verify malicious files were NOT extracted
	evilPaths := []string{
		filepath.Join(tmpDir, "etc", "passwd"),
		filepath.Join(tmpDir, "escape.txt"),
	}
	for _, p := range evilPaths {
		if _, err := os.Stat(p); err == nil {
			t.Errorf("malicious file was extracted: %s", p)
		}
	}
}

func TestTGZIngester_IngestMulti(t *testing.T) {
	tmpDir := t.TempDir()

	// Create multiple TGZ archives
	tgz1 := filepath.Join(tmpDir, "archive1.tgz")
	if err := createTestTGZ(tgz1, map[string]string{
		"file1.txt": "content1",
	}); err != nil {
		t.Fatal(err)
	}

	tgz2 := filepath.Join(tmpDir, "archive2.tar.gz")
	if err := createTestTGZ(tgz2, map[string]string{
		"file2.txt": "content2",
	}); err != nil {
		t.Fatal(err)
	}

	// Ingest both
	destDir := filepath.Join(tmpDir, "dest")
	ing := newTGZIngester("test")

	ctx := context.Background()
	result, err := ing.IngestMulti(ctx, []string{tgz1, tgz2}, destDir, nil)
	if err != nil {
		t.Fatalf("IngestMulti failed: %v", err)
	}

	if result != destDir {
		t.Errorf("expected result %q, got %q", destDir, result)
	}

	// Verify files from both archives
	verifyFileContent(t, filepath.Join(destDir, "file1.txt"), "content1")
	verifyFileContent(t, filepath.Join(destDir, "file2.txt"), "content2")
}

func TestTGZIngester_IngestMulti_ContextCancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a TGZ
	tgzPath := filepath.Join(tmpDir, "test.tgz")
	if err := createTestTGZ(tgzPath, map[string]string{
		"file.txt": "data",
	}); err != nil {
		t.Fatal(err)
	}

	destDir := filepath.Join(tmpDir, "dest")
	ing := newTGZIngester("test")

	// Cancel context before ingestion
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ing.IngestMulti(ctx, []string{tgzPath}, destDir, nil)
	if err == nil {
		t.Error("expected context cancellation error")
	}
}

// ── Test Helpers ─────────────────────────────────────────────────────────────

func verifyFileContent(t *testing.T, path, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Errorf("failed to read %s: %v", path, err)
		return
	}
	if string(data) != expected {
		t.Errorf("file %s: expected content %q, got %q", path, expected, string(data))
	}
}

func createTestZip(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	defer w.Close()

	for name, content := range files {
		// For path traversal tests, we need to write the raw name
		// without normalization
		fw, err := w.CreateHeader(&zip.FileHeader{
			Name:     name,
			Method:   zip.Deflate,
			Modified: time.Now(),
		})
		if err != nil {
			return err
		}
		if _, err := io.WriteString(fw, content); err != nil {
			return err
		}
	}
	return nil
}

func createTestTGZ(path string, files map[string]string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	gw := gzip.NewWriter(f)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	for name, content := range files {
		hdr := &tar.Header{
			Name:    name,
			Mode:    0644,
			Size:    int64(len(content)),
			ModTime: time.Now(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if _, err := io.WriteString(tw, content); err != nil {
			return err
		}
	}
	return nil
}

// ── Integration Test ─────────────────────────────────────────────────────────

func TestIntegration_RealWorldUsage(t *testing.T) {
	// This test demonstrates how a service would use the shared ingesters
	tmpDir := t.TempDir()

	// Create a "Google Photos"-like folder structure
	serviceDir := filepath.Join(tmpDir, "Takeout", "Google Photos")
	if err := os.MkdirAll(filepath.Join(serviceDir, "2023"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(serviceDir, "2023", "photo.jpg"), []byte("photo data"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create ingesters as a service would
	detector := NewServiceFolderDetector([]string{"Google Photos", "Google Foto"})
	ingesters := NewServiceIngesters("google-photos", detector)

	// Find the right ingester
	var folderIng core.Ingester
	for _, ing := range ingesters {
		if ing.CanIngest(serviceDir) {
			folderIng = ing
			break
		}
	}

	if folderIng == nil {
		t.Fatal("no ingester matched the service folder")
	}

	// Ingest the data
	destDir := filepath.Join(tmpDir, "imported")
	ctx := context.Background()

	result, err := folderIng.Ingest(ctx, serviceDir, destDir, func(done, total int) {
		t.Logf("Progress: %d/%d", done, total)
	})

	if err != nil {
		t.Fatalf("Ingest failed: %v", err)
	}

	if !strings.HasPrefix(result, destDir) {
		t.Errorf("result path %q not under dest %q", result, destDir)
	}

	// Verify the photo was imported
	verifyFileContent(t, filepath.Join(destDir, "2023", "photo.jpg"), "photo data")
}
