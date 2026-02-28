package metadata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseFile_PhotoTakenTime(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "test.json")

	// Real example from user's Google Photos Takeout
	content := `{
		"title": "IMG_20200724_212331.jpg",
		"photoTakenTime": {
			"timestamp": "1595618611",
			"formatted": "24 lug 2020, 19:23:31 UTC"
		},
		"geoData": {
			"latitude": 45.2204,
			"longitude": 7.599723999999999,
			"altitude": 387.5
		},
		"geoDataExif": {
			"latitude": 45.2204,
			"longitude": 7.599723999999999,
			"altitude": 387.5
		}
	}`

	if err := os.WriteFile(jsonPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	photo, err := ParseFile(jsonPath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Test title
	if photo.Title != "IMG_20200724_212331.jpg" {
		t.Errorf("Title = %q, want %q", photo.Title, "IMG_20200724_212331.jpg")
	}

	// Test photoTakenTime
	ts := photo.PhotoTakenTime.Unix()
	if ts != 1595618611 {
		t.Errorf("PhotoTakenTime.Unix() = %d, want %d", ts, 1595618611)
	}

	// Test geoData
	if photo.GeoData.Latitude != 45.2204 {
		t.Errorf("GeoData.Latitude = %f, want %f", photo.GeoData.Latitude, 45.2204)
	}
	if photo.GeoData.Longitude != 7.599723999999999 {
		t.Errorf("GeoData.Longitude = %f, want %f", photo.GeoData.Longitude, 7.599723999999999)
	}
	if photo.GeoData.Altitude != 387.5 {
		t.Errorf("GeoData.Altitude = %f, want %f", photo.GeoData.Altitude, 387.5)
	}

	// Test BestGeo()
	geo := photo.BestGeo()
	if geo.IsZero() {
		t.Error("BestGeo() returned zero coordinates")
	}
	if geo.Latitude != 45.2204 {
		t.Errorf("BestGeo().Latitude = %f, want %f", geo.Latitude, 45.2204)
	}
}

func TestParseFile_ZeroCoordinates(t *testing.T) {
	tmpDir := t.TempDir()
	jsonPath := filepath.Join(tmpDir, "test.json")

	content := `{
		"title": "IMG_001.jpg",
		"photoTakenTime": {"timestamp": "1595618611"},
		"geoData": {"latitude": 0.0, "longitude": 0.0, "altitude": 0.0}
	}`

	if err := os.WriteFile(jsonPath, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	photo, err := ParseFile(jsonPath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Zero coordinates should be detected
	if !photo.GeoData.IsZero() {
		t.Error("GeoData.IsZero() = false, want true for 0,0 coordinates")
	}
}
