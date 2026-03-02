package maps

import (
	"testing"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/maps/metadata"
)

func TestMapsServiceRegistration(t *testing.T) {
	// Verify service is registered
	svc, ok := core.GetService("maps")
	if !ok {
		t.Fatal("Maps service not registered")
	}

	if svc.ID() != "maps" {
		t.Errorf("Service ID = %q, want %q", svc.ID(), "maps")
	}

	if svc.Name() != "Google Maps" {
		t.Errorf("Service Name = %q, want %q", svc.Name(), "Google Maps")
	}

	desc := svc.Description()
	if desc == "" {
		t.Error("Service Description is empty")
	}
}

func TestMapsServiceMetadata(t *testing.T) {
	svc := New()

	if svc.ID() != ServiceID {
		t.Errorf("ID = %q, want %q", svc.ID(), ServiceID)
	}

	if svc.Name() == "" {
		t.Error("Name is empty")
	}

	if svc.Description() == "" {
		t.Error("Description is empty")
	}

	if svc.Name() != "Google Maps" {
		t.Errorf("Name = %q, want %q", svc.Name(), "Google Maps")
	}
}

func TestMapsServiceIngesters(t *testing.T) {
	svc := New()

	ingesters := svc.Ingesters()
	if len(ingesters) != 3 {
		t.Errorf("Ingesters count = %d, want 3 (folder, zip, tgz)", len(ingesters))
	}

	// Verify ingester IDs have correct prefix
	wantPrefix := "maps-"
	for _, ing := range ingesters {
		if len(ing.ID()) < len(wantPrefix) || ing.ID()[:len(wantPrefix)] != wantPrefix {
			t.Errorf("Ingester ID %q doesn't start with %q", ing.ID(), wantPrefix)
		}
	}
}

func TestMapsServiceProcessors(t *testing.T) {
	svc := New()

	processors := svc.Processors()
	if len(processors) != 1 {
		t.Errorf("Processors count = %d, want 1", len(processors))
	}

	if processors[0].ID() != "maps-processor" {
		t.Errorf("Processor ID = %q, want %q", processors[0].ID(), "maps-processor")
	}

	if processors[0].Name() != "Maps Processor" {
		t.Errorf("Processor Name = %q, want %q", processors[0].Name(), "Maps Processor")
	}
}

func TestMapsServiceSupportedOutputs(t *testing.T) {
	svc := New()

	outputs := svc.SupportedOutputs()
	if len(outputs) != 5 {
		t.Errorf("SupportedOutputs count = %d, want 5 (local-folder + 4 maps outputs)", len(outputs))
	}

	wantOutputs := map[string]bool{
		"local-folder": true,
		"maps-geojson": true,
		"maps-kml":     true,
		"maps-json":    true,
		"maps-csv":     true,
	}

	for _, id := range outputs {
		if !wantOutputs[id] {
			t.Errorf("Unexpected output ID: %s", id)
		}
		delete(wantOutputs, id)
	}

	if len(wantOutputs) > 0 {
		t.Errorf("Missing expected outputs: %v", wantOutputs)
	}
}

func TestMapsServiceDefaultConfig(t *testing.T) {
	svc := New()

	cfg := svc.DefaultConfig()
	if cfg.Settings == nil {
		t.Fatal("DefaultConfig.Settings is nil")
	}

	// Check for expected settings
	if _, ok := cfg.Settings["coordinate_precision"]; !ok {
		t.Error("DefaultConfig missing 'coordinate_precision' setting")
	}

	if _, ok := cfg.Settings["include_timeline"]; !ok {
		t.Error("DefaultConfig missing 'include_timeline' setting")
	}

	if _, ok := cfg.Settings["min_accuracy"]; !ok {
		t.Error("DefaultConfig missing 'min_accuracy' setting")
	}

	// Verify default values
	if val, ok := cfg.Settings["coordinate_precision"].(int); !ok || val != 6 {
		t.Errorf("coordinate_precision = %v, want 6", cfg.Settings["coordinate_precision"])
	}

	if val, ok := cfg.Settings["include_timeline"].(bool); !ok || !val {
		t.Errorf("include_timeline = %v, want true", cfg.Settings["include_timeline"])
	}

	if val, ok := cfg.Settings["min_accuracy"].(int); !ok || val != 0 {
		t.Errorf("min_accuracy = %v, want 0", cfg.Settings["min_accuracy"])
	}
}

func TestMapsServiceOutputsRegistered(t *testing.T) {
	// Test that all 4 service-specific outputs are registered
	expectedOutputs := []string{
		"maps-geojson",
		"maps-kml",
		"maps-json",
		"maps-csv",
	}

	for _, id := range expectedOutputs {
		output, ok := core.GetOutput(id)
		if !ok {
			t.Errorf("Output %q not registered", id)
			continue
		}

		if output.ID() != id {
			t.Errorf("Output ID mismatch: got %q, want %q", output.ID(), id)
		}

		// Verify outputs support "maps_library" item type
		supportedTypes := output.SupportedItemTypes()
		supportsLibrary := false
		for _, typ := range supportedTypes {
			if typ == "maps_library" {
				supportsLibrary = true
				break
			}
		}
		if !supportsLibrary {
			t.Errorf("Output %q does not support 'maps_library' item type", id)
		}
	}
}

func TestE7ConversionAccuracy(t *testing.T) {
	// Test E7 coordinate conversion accuracy
	tests := []struct {
		name     string
		e7val    int
		expected float64
	}{
		{"Standard positive", 374216440, 37.421644},
		{"Standard negative", -1220840920, -122.084092},
		{"Zero", 0, 0.0},
		{"Small positive", 1, 0.0000001},
		{"Small negative", -1, -0.0000001},
		{"Max int32", 2147483647, 214.7483647},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := float64(tt.e7val) / 1e7
			// Allow for floating point precision
			diff := result - tt.expected
			if diff < -0.0000001 || diff > 0.0000001 {
				t.Errorf("E7 conversion of %d = %f; want %f (diff: %f)", tt.e7val, result, tt.expected, diff)
			}
		})
	}
}

func TestMapsLibraryMetadataInterface(t *testing.T) {
	// Test that MapsLibrary implements conncore.Metadata interface
	lib := &metadata.MapsLibrary{
		LocationHistory: []metadata.LocationRecord{
			{Latitude: 37.421644, Longitude: -122.084092},
			{Latitude: 40.827, Longitude: -74.0006},
		},
		SavedPlaces: []metadata.SavedPlace{
			{Name: "Home", Latitude: 37.421644, Longitude: -122.084092},
		},
		Timeline: []metadata.PlaceVisit{},
		Stats:    map[string]int{"test": 1},
	}

	// Test interface methods
	if title := lib.GetTitle(); title != "Google Maps Location History" {
		t.Errorf("GetTitle() = %q, want %q", title, "Google Maps Location History")
	}

	desc := lib.GetDescription()
	if desc == "" {
		t.Error("GetDescription() returned empty string")
	}

	if dataType := lib.GetDataType(); dataType != "location" {
		t.Errorf("GetDataType() = %q, want %q", dataType, "location")
	}

	if mediaType := lib.GetMediaType(); mediaType != "application/json" {
		t.Errorf("GetMediaType() = %q, want %q", mediaType, "application/json")
	}

	if size := lib.GetSize(); size != 2 {
		t.Errorf("GetSize() = %d, want 2", size)
	}

	meta := lib.GetMetadata()
	if meta == nil {
		t.Fatal("GetMetadata() returned nil")
	}

	if meta["location_count"] != 2 {
		t.Errorf("metadata location_count = %v, want 2", meta["location_count"])
	}

	if meta["saved_places"] != 1 {
		t.Errorf("metadata saved_places = %v, want 1", meta["saved_places"])
	}

	// Test read-only nature
	if err := lib.SetMetadata("test", "value"); err == nil {
		t.Error("SetMetadata should return error (read-only)")
	}

	if err := lib.SetTags([]string{"tag1"}); err == nil {
		t.Error("SetTags should return error (not supported)")
	}

	tags := lib.GetTags()
	if len(tags) != 0 {
		t.Errorf("GetTags() returned %d tags, want 0", len(tags))
	}
}

func TestMapsServiceConfigSchema(t *testing.T) {
	svc := New()
	processors := svc.Processors()
	if len(processors) == 0 {
		t.Fatal("No processors available")
	}

	schema := processors[0].ConfigSchema()
	if len(schema) == 0 {
		t.Error("ConfigSchema returned empty slice")
	}

	// Verify required config options exist
	foundOptions := make(map[string]bool)
	for _, opt := range schema {
		foundOptions[opt.ID] = true

		// Verify option has required fields
		if opt.Name == "" {
			t.Errorf("Config option %q has empty Name", opt.ID)
		}
		if opt.Type == "" {
			t.Errorf("Config option %q has empty Type", opt.ID)
		}
	}

	requiredOptions := []string{"coordinate_precision", "include_timeline", "min_accuracy"}
	for _, req := range requiredOptions {
		if !foundOptions[req] {
			t.Errorf("ConfigSchema missing required option: %q", req)
		}
	}
}
