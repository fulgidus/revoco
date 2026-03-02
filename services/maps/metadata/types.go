// Package metadata provides Google Maps location history parsing and structures.
package metadata

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"time"

	conncore "github.com/fulgidus/revoco/connectors"
)

// LocationRecord represents a single location point from Google Maps timeline.
type LocationRecord struct {
	Latitude  float64   `json:"latitude"`
	Longitude float64   `json:"longitude"`
	Accuracy  int       `json:"accuracy"`
	Timestamp time.Time `json:"timestamp"`
	Source    string    `json:"source,omitempty"`
	Altitude  float64   `json:"altitude,omitempty"`
	Velocity  int       `json:"velocity,omitempty"`
	Heading   int       `json:"heading,omitempty"`
}

// SavedPlace represents a saved location from Google Maps.
type SavedPlace struct {
	Name          string   `json:"name"`
	Address       string   `json:"address,omitempty"`
	Latitude      float64  `json:"latitude"`
	Longitude     float64  `json:"longitude"`
	GoogleMapsURL string   `json:"google_maps_url,omitempty"`
	Note          string   `json:"note,omitempty"`
	Lists         []string `json:"lists,omitempty"`
}

// PlaceVisit represents a visit to a location from semantic timeline data.
type PlaceVisit struct {
	Location   SavedPlace    `json:"location"`
	StartTime  time.Time     `json:"start_time"`
	EndTime    time.Time     `json:"end_time"`
	Duration   time.Duration `json:"duration"`
	Confidence string        `json:"confidence,omitempty"`
}

// MapsLibrary represents the complete collection of parsed Maps data.
type MapsLibrary struct {
	LocationHistory []LocationRecord `json:"location_history"`
	SavedPlaces     []SavedPlace     `json:"saved_places"`
	Timeline        []PlaceVisit     `json:"timeline"`
	Stats           map[string]int   `json:"stats"`
}

// rawRecordsJSON represents the structure of Records.json from Google Takeout.
type rawRecordsJSON struct {
	Locations []rawLocationPoint `json:"locations"`
}

// rawLocationPoint represents a single location point with E7 coordinates.
type rawLocationPoint struct {
	LatitudeE7       int    `json:"latitudeE7"`
	LongitudeE7      int    `json:"longitudeE7"`
	Accuracy         int    `json:"accuracy"`
	Timestamp        string `json:"timestamp"`
	Source           string `json:"source,omitempty"`
	Altitude         int    `json:"altitude,omitempty"`
	Velocity         int    `json:"velocity,omitempty"`
	Heading          int    `json:"heading,omitempty"`
	VerticalAccuracy int    `json:"verticalAccuracy,omitempty"`
}

// kmlDocument represents the KML structure for saved places.
type kmlDocument struct {
	XMLName  xml.Name      `xml:"kml"`
	Document kmlDocElement `xml:"Document"`
}

type kmlDocElement struct {
	Placemarks []kmlPlacemark `xml:"Placemark"`
}

type kmlPlacemark struct {
	Name         string     `xml:"name"`
	Description  string     `xml:"description"`
	Point        kmlPoint   `xml:"Point"`
	ExtendedData kmlExtData `xml:"ExtendedData,omitempty"`
}

type kmlPoint struct {
	Coordinates string `xml:"coordinates"`
}

type kmlExtData struct {
	Data []kmlData `xml:"Data"`
}

type kmlData struct {
	Name  string `xml:"name,attr"`
	Value string `xml:"value"`
}

// rawTimelineJSON represents monthly semantic location history.
type rawTimelineJSON struct {
	TimelineObjects []rawTimelineObject `json:"timelineObjects"`
}

type rawTimelineObject struct {
	PlaceVisit      *rawPlaceVisit      `json:"placeVisit,omitempty"`
	ActivitySegment *rawActivitySegment `json:"activitySegment,omitempty"`
}

type rawPlaceVisit struct {
	Location rawTimelineLocation `json:"location"`
	Duration rawDuration         `json:"duration"`
}

type rawActivitySegment struct {
	StartLocation rawTimelineLocation `json:"startLocation"`
	EndLocation   rawTimelineLocation `json:"endLocation"`
	Duration      rawDuration         `json:"duration"`
	ActivityType  string              `json:"activityType,omitempty"`
	Distance      int                 `json:"distance,omitempty"`
}

type rawTimelineLocation struct {
	LatitudeE7          int    `json:"latitudeE7"`
	LongitudeE7         int    `json:"longitudeE7"`
	PlaceID             string `json:"placeId,omitempty"`
	Name                string `json:"name,omitempty"`
	Address             string `json:"address,omitempty"`
	SourceInfoDeviceTag int    `json:"sourceInfo,omitempty"`
}

type rawDuration struct {
	StartTimestamp string `json:"startTimestamp"`
	EndTimestamp   string `json:"endTimestamp"`
}

// convertE7 converts E7 coordinate format to decimal degrees.
// Google stores coordinates as integers multiplied by 1e7.
func convertE7(e7val int) float64 {
	return float64(e7val) / 1e7
}

// ParseRecordsJSON streams and parses Records.json location history file.
// CRITICAL: Uses json.NewDecoder for streaming to handle large files (100MB+).
func ParseRecordsJSON(reader io.Reader) ([]LocationRecord, error) {
	dec := json.NewDecoder(reader)

	// Read opening brace
	t, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("read opening token: %w", err)
	}
	if delim, ok := t.(json.Delim); !ok || delim != '{' {
		return nil, fmt.Errorf("expected opening brace, got %v", t)
	}

	var records []LocationRecord

	// Read tokens until we find "locations" array
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("read token: %w", err)
		}

		if key, ok := t.(string); ok && key == "locations" {
			// Read opening bracket of locations array
			t, err := dec.Token()
			if err != nil {
				return nil, fmt.Errorf("read locations array: %w", err)
			}
			if delim, ok := t.(json.Delim); !ok || delim != '[' {
				return nil, fmt.Errorf("expected array start, got %v", t)
			}

			// Stream each location point
			for dec.More() {
				var raw rawLocationPoint
				if err := dec.Decode(&raw); err != nil {
					return nil, fmt.Errorf("decode location point: %w", err)
				}

				// Parse timestamp
				timestamp, err := time.Parse(time.RFC3339, raw.Timestamp)
				if err != nil {
					// Try alternative format
					timestamp, err = time.Parse("2006-01-02T15:04:05.000Z", raw.Timestamp)
					if err != nil {
						continue // Skip records with invalid timestamps
					}
				}

				// Convert E7 coordinates to decimal degrees
				record := LocationRecord{
					Latitude:  convertE7(raw.LatitudeE7),
					Longitude: convertE7(raw.LongitudeE7),
					Accuracy:  raw.Accuracy,
					Timestamp: timestamp,
					Source:    raw.Source,
					Altitude:  float64(raw.Altitude),
					Velocity:  raw.Velocity,
					Heading:   raw.Heading,
				}

				records = append(records, record)
			}

			// Read closing bracket
			t, err = dec.Token()
			if err != nil {
				return nil, fmt.Errorf("read closing bracket: %w", err)
			}
			if delim, ok := t.(json.Delim); !ok || delim != ']' {
				return nil, fmt.Errorf("expected array end, got %v", t)
			}

			break // Found and processed locations array
		}
	}

	// Read remaining tokens until closing brace
	for dec.More() {
		_, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("read remaining tokens: %w", err)
		}
	}

	return records, nil
}

// ParseSavedPlacesKML parses saved places from KML file.
func ParseSavedPlacesKML(reader io.Reader) ([]SavedPlace, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read KML: %w", err)
	}

	var doc kmlDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("unmarshal KML: %w", err)
	}

	var places []SavedPlace
	for _, pm := range doc.Document.Placemarks {
		// Parse coordinates (format: "lon,lat,alt" or "lon,lat")
		coords := pm.Point.Coordinates
		var lon, lat float64
		n, err := fmt.Sscanf(coords, "%f,%f", &lon, &lat)
		if err != nil || n < 2 {
			continue // Skip invalid coordinates
		}

		place := SavedPlace{
			Name:      pm.Name,
			Address:   pm.Description,
			Latitude:  lat,
			Longitude: lon,
		}

		// Extract Google Maps URL from extended data
		for _, data := range pm.ExtendedData.Data {
			if data.Name == "gx_media_links" {
				place.GoogleMapsURL = data.Value
			}
		}

		places = append(places, place)
	}

	return places, nil
}

// ParseTimelineJSON parses semantic location history from monthly timeline JSON.
func ParseTimelineJSON(reader io.Reader) ([]PlaceVisit, error) {
	var timeline rawTimelineJSON
	if err := json.NewDecoder(reader).Decode(&timeline); err != nil {
		return nil, fmt.Errorf("decode timeline JSON: %w", err)
	}

	var visits []PlaceVisit
	for _, obj := range timeline.TimelineObjects {
		if obj.PlaceVisit != nil {
			pv := obj.PlaceVisit

			startTime, err := time.Parse(time.RFC3339, pv.Duration.StartTimestamp)
			if err != nil {
				continue
			}
			endTime, err := time.Parse(time.RFC3339, pv.Duration.EndTimestamp)
			if err != nil {
				continue
			}

			visit := PlaceVisit{
				Location: SavedPlace{
					Name:      pv.Location.Name,
					Address:   pv.Location.Address,
					Latitude:  convertE7(pv.Location.LatitudeE7),
					Longitude: convertE7(pv.Location.LongitudeE7),
				},
				StartTime: startTime,
				EndTime:   endTime,
				Duration:  endTime.Sub(startTime),
			}

			visits = append(visits, visit)
		}
	}

	return visits, nil
}

// Implement conncore.Metadata interface

// GetTitle returns the title for display.
func (l *MapsLibrary) GetTitle() string {
	return "Google Maps Location History"
}

// GetDescription returns a description of the data.
func (l *MapsLibrary) GetDescription() string {
	totalLocations := len(l.LocationHistory)
	totalPlaces := len(l.SavedPlaces)
	totalVisits := len(l.Timeline)
	return fmt.Sprintf("%d location records, %d saved places, %d timeline visits", totalLocations, totalPlaces, totalVisits)
}

// GetCreatedDate returns the earliest location timestamp.
func (l *MapsLibrary) GetCreatedDate() time.Time {
	if len(l.LocationHistory) == 0 {
		return time.Time{}
	}
	earliest := l.LocationHistory[0].Timestamp
	for _, loc := range l.LocationHistory[1:] {
		if loc.Timestamp.Before(earliest) {
			earliest = loc.Timestamp
		}
	}
	return earliest
}

// GetModifiedDate returns the latest location timestamp.
func (l *MapsLibrary) GetModifiedDate() time.Time {
	if len(l.LocationHistory) == 0 {
		return time.Time{}
	}
	latest := l.LocationHistory[0].Timestamp
	for _, loc := range l.LocationHistory[1:] {
		if loc.Timestamp.After(latest) {
			latest = loc.Timestamp
		}
	}
	return latest
}

// GetSize returns the total number of location records.
func (l *MapsLibrary) GetSize() int64 {
	return int64(len(l.LocationHistory))
}

// GetDataType returns the data type.
func (l *MapsLibrary) GetDataType() string {
	return string(conncore.DataTypeLocation)
}

// GetMediaType returns the media type.
func (l *MapsLibrary) GetMediaType() string {
	return "application/json"
}

// GetMetadata returns all metadata as a map.
func (l *MapsLibrary) GetMetadata() map[string]any {
	return map[string]any{
		"location_count":   len(l.LocationHistory),
		"saved_places":     len(l.SavedPlaces),
		"timeline_visits":  len(l.Timeline),
		"date_range_start": l.GetCreatedDate().Format(time.RFC3339),
		"date_range_end":   l.GetModifiedDate().Format(time.RFC3339),
		"stats":            l.Stats,
	}
}

// SetMetadata updates metadata (no-op for immutable fields).
func (l *MapsLibrary) SetMetadata(key string, value any) error {
	// Location history is read-only after parsing
	return fmt.Errorf("metadata is read-only")
}

// GetTags returns tags/labels (not applicable for location data).
func (l *MapsLibrary) GetTags() []string {
	return []string{}
}

// SetTags sets tags/labels (not supported).
func (l *MapsLibrary) SetTags(tags []string) error {
	return fmt.Errorf("tags not supported for location data")
}
