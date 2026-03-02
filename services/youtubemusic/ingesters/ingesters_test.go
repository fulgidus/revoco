package ingesters

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIngesterIDs(t *testing.T) {
	ingesters := NewYouTubeMusicIngesters()

	if len(ingesters) != 3 {
		t.Fatalf("expected 3 ingesters, got %d", len(ingesters))
	}

	expectedIDs := []string{
		"youtube-music-folder",
		"youtube-music-zip",
		"youtube-music-tgz",
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
		{"NewFolder", func() interface{} { return NewFolder() }, "youtube-music-folder"},
		{"NewZip", func() interface{} { return NewZip() }, "youtube-music-zip"},
		{"NewTGZ", func() interface{} { return NewTGZ() }, "youtube-music-tgz"},
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

	// Test 1: Folder with "YouTube Music" directory
	musicDir := filepath.Join(tmpDir, "Takeout", "YouTube Music")
	if err := os.MkdirAll(musicDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Test 2: Folder with "YouTube and YouTube Music"
	andVariantDir := filepath.Join(tmpDir, "Takeout2", "YouTube and YouTube Music")
	if err := os.MkdirAll(andVariantDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Test 3: Folder with Italian variant "YouTube e YouTube Music"
	italianDir := filepath.Join(tmpDir, "Takeout3", "YouTube e YouTube Music")
	if err := os.MkdirAll(italianDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Test 4: Folder with no YouTube Music
	emptyDir := filepath.Join(tmpDir, "Empty")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}

	// Test 5: ZIP file
	zipPath := filepath.Join(tmpDir, "test.zip")
	if err := os.WriteFile(zipPath, []byte("fake zip"), 0644); err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}

	// Test 6: TGZ file
	tgzPath := filepath.Join(tmpDir, "test.tgz")
	if err := os.WriteFile(tgzPath, []byte("fake tgz"), 0644); err != nil {
		t.Fatalf("failed to create tgz file: %v", err)
	}

	ingesters := NewYouTubeMusicIngesters()
	folderIngester := ingesters[0]
	zipIngester := ingesters[1]
	tgzIngester := ingesters[2]

	tests := []struct {
		name     string
		ingester interface{ CanIngest(string) bool }
		path     string
		expected bool
	}{
		{"Folder with YouTube Music", folderIngester, filepath.Join(tmpDir, "Takeout"), true},
		{"Folder with YouTube and YouTube Music", folderIngester, filepath.Join(tmpDir, "Takeout2"), true},
		{"Folder with Italian variant", folderIngester, filepath.Join(tmpDir, "Takeout3"), true},
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
