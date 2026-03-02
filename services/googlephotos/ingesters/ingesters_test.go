package ingesters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIngesterIDs(t *testing.T) {
	ingesters := NewGooglePhotosIngesters()

	if len(ingesters) != 3 {
		t.Fatalf("expected 3 ingesters, got %d", len(ingesters))
	}

	expectedIDs := []string{
		"google-photos-folder",
		"google-photos-zip",
		"google-photos-tgz",
	}

	for i, ingester := range ingesters {
		if ingester.ID() != expectedIDs[i] {
			t.Errorf("ingester[%d]: expected ID %q, got %q", i, expectedIDs[i], ingester.ID())
		}
	}
}

func TestLegacyConstructors(t *testing.T) {
	tests := []struct {
		name        string
		constructor func() interface{}
		expectedID  string
	}{
		{"NewFolder", func() interface{} { return NewFolder() }, "google-photos-folder"},
		{"NewZip", func() interface{} { return NewZip() }, "google-photos-zip"},
		{"NewTGZ", func() interface{} { return NewTGZ() }, "google-photos-tgz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ingester := tt.constructor()
			if ing, ok := ingester.(interface{ ID() string }); ok {
				if ing.ID() != tt.expectedID {
					t.Errorf("expected ID %q, got %q", tt.expectedID, ing.ID())
				}
			} else {
				t.Errorf("constructor did not return an Ingester")
			}
		})
	}
}

func TestCanIngestBehavior(t *testing.T) {
	// Create a temporary directory structure
	tmpDir := t.TempDir()

	// Test 1: Folder with "Google Photos" directory
	photosDir := filepath.Join(tmpDir, "Takeout", "Google Photos")
	if err := os.MkdirAll(photosDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Test 2: Folder with Italian variant
	italianDir := filepath.Join(tmpDir, "Takeout2", "Google Foto")
	if err := os.MkdirAll(italianDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Test 3: Folder with no Google Photos
	emptyDir := filepath.Join(tmpDir, "Empty")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Test 4: ZIP file
	zipPath := filepath.Join(tmpDir, "test.zip")
	if err := os.WriteFile(zipPath, []byte("fake zip"), 0644); err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}

	// Test 5: TGZ file
	tgzPath := filepath.Join(tmpDir, "test.tgz")
	if err := os.WriteFile(tgzPath, []byte("fake tgz"), 0644); err != nil {
		t.Fatalf("failed to create tgz file: %v", err)
	}

	ingesters := NewGooglePhotosIngesters()
	folderIngester := ingesters[0]
	zipIngester := ingesters[1]
	tgzIngester := ingesters[2]

	tests := []struct {
		name     string
		ingester interface{ CanIngest(string) bool }
		path     string
		expected bool
	}{
		{"Folder with Google Photos", folderIngester, filepath.Join(tmpDir, "Takeout"), true},
		{"Folder with Italian variant", folderIngester, filepath.Join(tmpDir, "Takeout2"), true},
		{"Empty folder", folderIngester, emptyDir, false},
		{"ZIP file", zipIngester, zipPath, true},
		{"TGZ file", tgzIngester, tgzPath, true},
		{"ZIP ingester on folder", zipIngester, emptyDir, false},
		{"Folder ingester on zip", folderIngester, zipPath, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ingester.CanIngest(tt.path)
			if result != tt.expected {
				t.Errorf("CanIngest(%q) = %v, expected %v", tt.path, result, tt.expected)
			}
		})
	}
}
