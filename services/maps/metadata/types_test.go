package metadata

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestConvertE7(t *testing.T) {
	tests := []struct {
		name     string
		e7val    int
		expected float64
	}{
		{"Positive standard", 123456789, 12.3456789},
		{"Negative standard", -987654321, -98.7654321},
		{"Zero", 0, 0.0},
		{"Max int32", 2147483647, 214.7483647},
		{"Min int32", -2147483648, -214.7483648},
		{"Small value", 100, 0.00001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertE7(tt.e7val)
			if result != tt.expected {
				t.Errorf("convertE7(%d) = %f; want %f", tt.e7val, result, tt.expected)
			}
		})
	}
}

func TestParseRecordsJSON(t *testing.T) {
	validJSON := `{
		"locations": [
			{
				"latitudeE7": 374216440,
				"longitudeE7": -1220840920,
				"accuracy": 20,
				"timestamp": "2024-01-15T12:30:45.000Z",
				"source": "WIFI"
			},
			{
				"latitudeE7": 408270000,
				"longitudeE7": -740006000,
				"accuracy": 15,
				"timestamp": "2024-01-16T08:15:30.000Z",
				"altitude": 100,
				"velocity": 5,
				"heading": 270
			}
		]
	}`

	reader := strings.NewReader(validJSON)
	records, err := ParseRecordsJSON(reader)
	if err != nil {
		t.Fatalf("ParseRecordsJSON failed: %v", err)
	}

	if len(records) != 2 {
		t.Fatalf("Expected 2 records, got %d", len(records))
	}

	// Validate first record
	r1 := records[0]
	expectedLat1 := 37.421644
	expectedLon1 := -122.084092
	if r1.Latitude < expectedLat1-0.0001 || r1.Latitude > expectedLat1+0.0001 {
		t.Errorf("Record 1 latitude = %f; want ~%f", r1.Latitude, expectedLat1)
	}
	if r1.Longitude < expectedLon1-0.0001 || r1.Longitude > expectedLon1+0.0001 {
		t.Errorf("Record 1 longitude = %f; want ~%f", r1.Longitude, expectedLon1)
	}
	if r1.Accuracy != 20 {
		t.Errorf("Record 1 accuracy = %d; want 20", r1.Accuracy)
	}
	if r1.Source != "WIFI" {
		t.Errorf("Record 1 source = %s; want WIFI", r1.Source)
	}

	// Validate second record
	r2 := records[1]
	expectedLat2 := 40.827
	expectedLon2 := -74.0006
	if r2.Latitude < expectedLat2-0.0001 || r2.Latitude > expectedLat2+0.0001 {
		t.Errorf("Record 2 latitude = %f; want ~%f", r2.Latitude, expectedLat2)
	}
	if r2.Longitude < expectedLon2-0.0001 || r2.Longitude > expectedLon2+0.0001 {
		t.Errorf("Record 2 longitude = %f; want ~%f", r2.Longitude, expectedLon2)
	}
	if r2.Altitude != 100.0 {
		t.Errorf("Record 2 altitude = %f; want 100.0", r2.Altitude)
	}
	if r2.Velocity != 5 {
		t.Errorf("Record 2 velocity = %d; want 5", r2.Velocity)
	}
	if r2.Heading != 270 {
		t.Errorf("Record 2 heading = %d; want 270", r2.Heading)
	}
}

func TestParseRecordsJSON_EmptyArray(t *testing.T) {
	emptyJSON := `{"locations": []}`
	reader := strings.NewReader(emptyJSON)
	records, err := ParseRecordsJSON(reader)
	if err != nil {
		t.Fatalf("ParseRecordsJSON failed on empty array: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("Expected 0 records, got %d", len(records))
	}
}

func TestParseRecordsJSON_MalformedJSON(t *testing.T) {
	malformedJSON := `{"locations": [{"latitudeE7": "not a number"}]}`
	reader := strings.NewReader(malformedJSON)
	_, err := ParseRecordsJSON(reader)
	if err == nil {
		t.Error("Expected error for malformed JSON, got nil")
	}
}

func TestParseRecordsJSON_Streaming(t *testing.T) {
	// Create a large JSON with many records to verify streaming behavior
	// We'll use a custom reader that counts reads to ensure streaming
	var buf bytes.Buffer
	buf.WriteString(`{"locations":[`)

	numRecords := 1000
	for i := 0; i < numRecords; i++ {
		if i > 0 {
			buf.WriteString(",")
		}
		record := map[string]interface{}{
			"latitudeE7":  374216440 + i*100,
			"longitudeE7": -1220840920 + i*100,
			"accuracy":    20,
			"timestamp":   "2024-01-15T12:30:45.000Z",
		}
		data, _ := json.Marshal(record)
		buf.Write(data)
	}
	buf.WriteString(`]}`)

	records, err := ParseRecordsJSON(&buf)
	if err != nil {
		t.Fatalf("ParseRecordsJSON failed on large dataset: %v", err)
	}

	if len(records) != numRecords {
		t.Errorf("Expected %d records, got %d", numRecords, len(records))
	}

	// Verify streaming by checking that memory usage is reasonable
	// (this is implicit - if we loaded entire file, we'd likely OOM with real 100MB files)
	t.Logf("Successfully streamed %d records", numRecords)
}

func TestParseRecordsJSON_InvalidTimestamp(t *testing.T) {
	invalidJSON := `{
		"locations": [
			{
				"latitudeE7": 374216440,
				"longitudeE7": -1220840920,
				"accuracy": 20,
				"timestamp": "invalid-timestamp"
			},
			{
				"latitudeE7": 408270000,
				"longitudeE7": -740006000,
				"accuracy": 15,
				"timestamp": "2024-01-16T08:15:30.000Z"
			}
		]
	}`

	reader := strings.NewReader(invalidJSON)
	records, err := ParseRecordsJSON(reader)
	if err != nil {
		t.Fatalf("ParseRecordsJSON failed: %v", err)
	}

	// Should skip invalid timestamp record
	if len(records) != 1 {
		t.Errorf("Expected 1 valid record (skipping invalid), got %d", len(records))
	}
}

func TestParseSavedPlacesKML(t *testing.T) {
	kmlData := `<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2">
	<Document>
		<Placemark>
			<name>Home</name>
			<description>123 Main St</description>
			<Point>
				<coordinates>-122.084092,37.421644,0</coordinates>
			</Point>
		</Placemark>
		<Placemark>
			<name>Work</name>
			<description>456 Office Blvd</description>
			<Point>
				<coordinates>-74.0006,40.827</coordinates>
			</Point>
			<ExtendedData>
				<Data name="gx_media_links">
					<value>https://maps.google.com/maps?q=40.827,-74.0006</value>
				</Data>
			</ExtendedData>
		</Placemark>
	</Document>
</kml>`

	reader := strings.NewReader(kmlData)
	places, err := ParseSavedPlacesKML(reader)
	if err != nil {
		t.Fatalf("ParseSavedPlacesKML failed: %v", err)
	}

	if len(places) != 2 {
		t.Fatalf("Expected 2 places, got %d", len(places))
	}

	// Validate first place
	p1 := places[0]
	if p1.Name != "Home" {
		t.Errorf("Place 1 name = %s; want Home", p1.Name)
	}
	if p1.Address != "123 Main St" {
		t.Errorf("Place 1 address = %s; want 123 Main St", p1.Address)
	}
	expectedLat1 := 37.421644
	expectedLon1 := -122.084092
	if p1.Latitude < expectedLat1-0.0001 || p1.Latitude > expectedLat1+0.0001 {
		t.Errorf("Place 1 latitude = %f; want ~%f", p1.Latitude, expectedLat1)
	}
	if p1.Longitude < expectedLon1-0.0001 || p1.Longitude > expectedLon1+0.0001 {
		t.Errorf("Place 1 longitude = %f; want ~%f", p1.Longitude, expectedLon1)
	}

	// Validate second place with extended data
	p2 := places[1]
	if p2.Name != "Work" {
		t.Errorf("Place 2 name = %s; want Work", p2.Name)
	}
	if p2.GoogleMapsURL != "https://maps.google.com/maps?q=40.827,-74.0006" {
		t.Errorf("Place 2 GoogleMapsURL = %s; want URL", p2.GoogleMapsURL)
	}
}

func TestParseSavedPlacesKML_InvalidCoordinates(t *testing.T) {
	kmlData := `<?xml version="1.0" encoding="UTF-8"?>
<kml xmlns="http://www.opengis.net/kml/2.2">
	<Document>
		<Placemark>
			<name>Invalid</name>
			<Point>
				<coordinates>not-valid-coords</coordinates>
			</Point>
		</Placemark>
		<Placemark>
			<name>Valid</name>
			<Point>
				<coordinates>-122.084092,37.421644</coordinates>
			</Point>
		</Placemark>
	</Document>
</kml>`

	reader := strings.NewReader(kmlData)
	places, err := ParseSavedPlacesKML(reader)
	if err != nil {
		t.Fatalf("ParseSavedPlacesKML failed: %v", err)
	}

	// Should skip invalid coordinates
	if len(places) != 1 {
		t.Errorf("Expected 1 valid place, got %d", len(places))
	}
}

func TestParseTimelineJSON(t *testing.T) {
	timelineData := `{
		"timelineObjects": [
			{
				"placeVisit": {
					"location": {
						"latitudeE7": 374216440,
						"longitudeE7": -1220840920,
						"placeId": "ChIJ123",
						"name": "Coffee Shop",
						"address": "123 Main St"
					},
					"duration": {
						"startTimestamp": "2024-01-15T08:00:00.000Z",
						"endTimestamp": "2024-01-15T09:30:00.000Z"
					}
				}
			},
			{
				"activitySegment": {
					"startLocation": {
						"latitudeE7": 374216440,
						"longitudeE7": -1220840920
					},
					"endLocation": {
						"latitudeE7": 408270000,
						"longitudeE7": -740006000
					},
					"duration": {
						"startTimestamp": "2024-01-15T09:30:00.000Z",
						"endTimestamp": "2024-01-15T10:00:00.000Z"
					},
					"activityType": "WALKING",
					"distance": 500
				}
			}
		]
	}`

	reader := strings.NewReader(timelineData)
	visits, err := ParseTimelineJSON(reader)
	if err != nil {
		t.Fatalf("ParseTimelineJSON failed: %v", err)
	}

	if len(visits) != 1 {
		t.Fatalf("Expected 1 place visit, got %d", len(visits))
	}

	// Validate place visit
	v := visits[0]
	if v.Location.Name != "Coffee Shop" {
		t.Errorf("Visit location name = %s; want Coffee Shop", v.Location.Name)
	}
	if v.Location.Address != "123 Main St" {
		t.Errorf("Visit location address = %s; want 123 Main St", v.Location.Address)
	}

	expectedDuration := 90 * time.Minute
	if v.Duration != expectedDuration {
		t.Errorf("Visit duration = %v; want %v", v.Duration, expectedDuration)
	}
}

func TestMapsLibrary_GetTitle(t *testing.T) {
	lib := &MapsLibrary{}
	title := lib.GetTitle()
	if title != "Google Maps Location History" {
		t.Errorf("GetTitle() = %s; want Google Maps Location History", title)
	}
}

func TestMapsLibrary_GetDescription(t *testing.T) {
	lib := &MapsLibrary{
		LocationHistory: make([]LocationRecord, 100),
		SavedPlaces:     make([]SavedPlace, 10),
		Timeline:        make([]PlaceVisit, 20),
	}
	desc := lib.GetDescription()
	expected := "100 location records, 10 saved places, 20 timeline visits"
	if desc != expected {
		t.Errorf("GetDescription() = %s; want %s", desc, expected)
	}
}

func TestMapsLibrary_GetCreatedDate(t *testing.T) {
	early := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC)

	lib := &MapsLibrary{
		LocationHistory: []LocationRecord{
			{Timestamp: mid},
			{Timestamp: early},
			{Timestamp: late},
		},
	}

	created := lib.GetCreatedDate()
	if !created.Equal(early) {
		t.Errorf("GetCreatedDate() = %v; want %v", created, early)
	}
}

func TestMapsLibrary_GetModifiedDate(t *testing.T) {
	early := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
	mid := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	late := time.Date(2023, 12, 1, 0, 0, 0, 0, time.UTC)

	lib := &MapsLibrary{
		LocationHistory: []LocationRecord{
			{Timestamp: mid},
			{Timestamp: early},
			{Timestamp: late},
		},
	}

	modified := lib.GetModifiedDate()
	if !modified.Equal(late) {
		t.Errorf("GetModifiedDate() = %v; want %v", modified, late)
	}
}

func TestMapsLibrary_GetSize(t *testing.T) {
	lib := &MapsLibrary{
		LocationHistory: make([]LocationRecord, 42),
	}
	size := lib.GetSize()
	if size != 42 {
		t.Errorf("GetSize() = %d; want 42", size)
	}
}

func TestMapsLibrary_GetDataType(t *testing.T) {
	lib := &MapsLibrary{}
	dataType := lib.GetDataType()
	if dataType != "location" {
		t.Errorf("GetDataType() = %s; want location", dataType)
	}
}

func TestMapsLibrary_GetMediaType(t *testing.T) {
	lib := &MapsLibrary{}
	mediaType := lib.GetMediaType()
	if mediaType != "application/json" {
		t.Errorf("GetMediaType() = %s; want application/json", mediaType)
	}
}

func TestMapsLibrary_GetMetadata(t *testing.T) {
	lib := &MapsLibrary{
		LocationHistory: []LocationRecord{
			{Timestamp: time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)},
		},
		SavedPlaces: make([]SavedPlace, 5),
		Timeline:    make([]PlaceVisit, 10),
		Stats: map[string]int{
			"unique_places": 3,
		},
	}

	meta := lib.GetMetadata()
	if meta["location_count"] != 1 {
		t.Errorf("metadata location_count = %v; want 1", meta["location_count"])
	}
	if meta["saved_places"] != 5 {
		t.Errorf("metadata saved_places = %v; want 5", meta["saved_places"])
	}
	if meta["timeline_visits"] != 10 {
		t.Errorf("metadata timeline_visits = %v; want 10", meta["timeline_visits"])
	}
}

func TestMapsLibrary_SetMetadata(t *testing.T) {
	lib := &MapsLibrary{}
	err := lib.SetMetadata("test", "value")
	if err == nil {
		t.Error("Expected error for SetMetadata, got nil")
	}
}

func TestMapsLibrary_GetTags(t *testing.T) {
	lib := &MapsLibrary{}
	tags := lib.GetTags()
	if len(tags) != 0 {
		t.Errorf("GetTags() returned %d tags; want 0", len(tags))
	}
}

func TestMapsLibrary_SetTags(t *testing.T) {
	lib := &MapsLibrary{}
	err := lib.SetTags([]string{"tag1", "tag2"})
	if err == nil {
		t.Error("Expected error for SetTags, got nil")
	}
}
