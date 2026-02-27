// Package metadata defines Go types matching the Google Photos Takeout JSON schema.
package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

// Timestamp is a Google Photos timestamp with a Unix epoch value stored as a string.
type Timestamp struct {
	Timestamp string `json:"timestamp"`
	Formatted string `json:"formatted"`
}

// Unix returns the Unix timestamp as int64. Returns 0 on parse error.
func (t Timestamp) Unix() int64 {
	v, _ := strconv.ParseInt(t.Timestamp, 10, 64)
	return v
}

// GeoData holds GPS coordinates and altitude as they appear in Takeout JSON.
type GeoData struct {
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	Altitude      float64 `json:"altitude"`
	LatitudeSpan  float64 `json:"latitudeSpan"`
	LongitudeSpan float64 `json:"longitudeSpan"`
}

// IsZero returns true if the coordinates are both exactly 0.0 (no GPS data).
func (g GeoData) IsZero() bool {
	return g.Latitude == 0.0 && g.Longitude == 0.0
}

// Person represents a tagged face in the photo.
type Person struct {
	Name string `json:"name"`
}

// Photo is the top-level structure of a Google Photos .json metadata file.
type Photo struct {
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	ImageViews         string    `json:"imageViews"`
	CreationTime       Timestamp `json:"creationTime"`
	PhotoTakenTime     Timestamp `json:"photoTakenTime"`
	GeoData            GeoData   `json:"geoData"`
	GeoDataExif        GeoData   `json:"geoDataExif"`
	People             []Person  `json:"people"`
	URL                string    `json:"url"`
	GooglePhotosOrigin *struct {
		MobileUpload *struct {
			DeviceFolder *struct {
				LocalFolderName string `json:"localFolderName"`
			} `json:"deviceFolder"`
			DeviceType string `json:"deviceType"`
		} `json:"mobileUpload"`
	} `json:"googlePhotosOrigin"`
}

// ParseFile reads and parses a Takeout JSON metadata file.
func ParseFile(path string) (*Photo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var p Photo
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &p, nil
}

// BestGeo returns GeoDataExif if it has data, otherwise falls back to GeoData.
func (p *Photo) BestGeo() GeoData {
	if !p.GeoDataExif.IsZero() {
		return p.GeoDataExif
	}
	return p.GeoData
}

// Keywords returns a deduplicated list of people names suitable for EXIF keyword tags.
func (p *Photo) Keywords() []string {
	seen := make(map[string]struct{}, len(p.People))
	out := make([]string, 0, len(p.People))
	for _, person := range p.People {
		if person.Name == "" {
			continue
		}
		if _, ok := seen[person.Name]; !ok {
			seen[person.Name] = struct{}{}
			out = append(out, person.Name)
		}
	}
	return out
}

// MissingEntry represents one record in the missing-files.json report.
type MissingEntry struct {
	Title          string `json:"title"`
	URL            string `json:"url"`
	PhotoTakenDate string `json:"photoTakenDate"`
	SourceFolder   string `json:"sourceFolder"`
}

// FailedEntry is a recovery entry annotated with an error reason.
type FailedEntry struct {
	MissingEntry
	Error string `json:"error"`
}
