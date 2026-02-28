package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIndexFiles_JSONMatching(t *testing.T) {
	// Create a temp directory structure mimicking Google Photos Takeout
	tmpDir := t.TempDir()
	gfotoDir := filepath.Join(tmpDir, "Google Foto")
	albumDir := filepath.Join(gfotoDir, "Album1")
	os.MkdirAll(albumDir, 0755)

	// Test case 1: JSON named "photo.jpg.json" should match "photo.jpg"
	writeFile(t, filepath.Join(albumDir, "IMG_001.jpg"), "fake image data")
	writeFile(t, filepath.Join(albumDir, "IMG_001.jpg.json"), `{
		"title": "IMG_001.jpg",
		"photoTakenTime": {"timestamp": "1595618611"}
	}`)

	// Test case 2: JSON with title field matching media
	writeFile(t, filepath.Join(albumDir, "IMG_002.jpg"), "fake image data")
	writeFile(t, filepath.Join(albumDir, "IMG_002.jpg.json"), `{
		"title": "IMG_002.jpg",
		"photoTakenTime": {"timestamp": "1595618612"}
	}`)

	// Test case 3: Supplemental metadata JSON
	writeFile(t, filepath.Join(albumDir, "IMG_003.jpg"), "fake image data")
	writeFile(t, filepath.Join(albumDir, "IMG_003.jpg.supplemental-metadata.json"), `{
		"title": "IMG_003.jpg",
		"photoTakenTime": {"timestamp": "1595618613"}
	}`)

	// Test case 4: Truncated supplemental suffix (common in Takeout)
	writeFile(t, filepath.Join(albumDir, "IMG_004.jpg"), "fake image data")
	writeFile(t, filepath.Join(albumDir, "IMG_004.jpg.suppleme.json"), `{
		"title": "IMG_004.jpg",
		"photoTakenTime": {"timestamp": "1595618614"}
	}`)

	// Run indexing
	result, err := IndexFiles(gfotoDir, nil)
	if err != nil {
		t.Fatalf("IndexFiles failed: %v", err)
	}

	// Verify results
	if result.TotalMedia != 4 {
		t.Errorf("Expected 4 media files, got %d", result.TotalMedia)
	}
	if result.TotalJSON != 4 {
		t.Errorf("Expected 4 JSON files, got %d", result.TotalJSON)
	}
	if result.TotalMatched != 4 {
		t.Errorf("Expected 4 matched, got %d (orphans: %d)", result.TotalMatched, result.TotalOrphans)
		for _, orphan := range result.OrphanJSONs {
			t.Logf("  Orphan: %s (title=%s)", orphan.Path, orphan.Title)
		}
	}

	// Check each media file has its JSON matched
	for _, name := range []string{"IMG_001.jpg", "IMG_002.jpg", "IMG_003.jpg", "IMG_004.jpg"} {
		mediaPath := filepath.Join(albumDir, name)
		jsonPath := result.MediaFiles[mediaPath]
		if jsonPath == "" {
			t.Errorf("Media %s has no matched JSON", name)
		} else {
			t.Logf("Matched: %s → %s", name, filepath.Base(jsonPath))
		}
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write %s: %v", path, err)
	}
}
