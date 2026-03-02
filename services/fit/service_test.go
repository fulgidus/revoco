package fit

import (
	"testing"

	"github.com/fulgidus/revoco/services/core"
)

func TestService_Register(t *testing.T) {
	// Clear registry to test registration
	// Note: Can't easily clear in production code, so test assumes fresh state

	service := New()
	if service == nil {
		t.Fatal("New() returned nil")
	}
}

func TestService_ID(t *testing.T) {
	service := New()
	if service.ID() != "fit" {
		t.Errorf("Expected ID 'fit', got '%s'", service.ID())
	}
}

func TestService_Name(t *testing.T) {
	service := New()
	if service.Name() != "Google Fit" {
		t.Errorf("Expected Name 'Google Fit', got '%s'", service.Name())
	}
}

func TestService_Description(t *testing.T) {
	service := New()
	desc := service.Description()
	if desc == "" {
		t.Error("Description should not be empty")
	}
	if desc != "Process Google Fit Takeout - daily aggregations and activity sessions" {
		t.Errorf("Unexpected description: %s", desc)
	}
}

func TestService_Ingesters(t *testing.T) {
	service := New()
	ingesters := service.Ingesters()

	if len(ingesters) == 0 {
		t.Error("Expected at least one ingester")
	}

	// Should have 3 ingesters: folder, zip, tgz
	if len(ingesters) != 3 {
		t.Errorf("Expected 3 ingesters, got %d", len(ingesters))
	}

	// Check ingester IDs
	expectedIDs := map[string]bool{
		"fit-folder": false,
		"fit-zip":    false,
		"fit-tgz":    false,
	}

	for _, ing := range ingesters {
		if _, ok := expectedIDs[ing.ID()]; ok {
			expectedIDs[ing.ID()] = true
		}
	}

	for id, found := range expectedIDs {
		if !found {
			t.Errorf("Expected ingester with ID '%s' not found", id)
		}
	}
}

func TestService_Processors(t *testing.T) {
	service := New()
	processors := service.Processors()

	if len(processors) == 0 {
		t.Error("Expected at least one processor")
	}

	// Should have 1 processor
	if len(processors) != 1 {
		t.Errorf("Expected 1 processor, got %d", len(processors))
	}

	// Check processor ID
	if processors[0].ID() != "fit-processor" {
		t.Errorf("Expected processor ID 'fit-processor', got '%s'", processors[0].ID())
	}
}

func TestService_SupportedOutputs(t *testing.T) {
	service := New()
	outputs := service.SupportedOutputs()

	if len(outputs) == 0 {
		t.Error("Expected at least one supported output")
	}

	// Should support: local-folder, fit-json, fit-csv
	expectedOutputs := map[string]bool{
		"local-folder": false,
		"fit-json":     false,
		"fit-csv":      false,
	}

	for _, output := range outputs {
		if _, ok := expectedOutputs[output]; ok {
			expectedOutputs[output] = true
		}
	}

	for output, found := range expectedOutputs {
		if !found {
			t.Errorf("Expected output '%s' not found in SupportedOutputs", output)
		}
	}
}

func TestService_DefaultConfig(t *testing.T) {
	service := New()
	config := service.DefaultConfig()

	if config.Settings == nil {
		t.Error("DefaultConfig.Settings should not be nil")
	}

	// Check for expected settings
	if _, ok := config.Settings["include_activities"]; !ok {
		t.Error("Expected 'include_activities' setting in DefaultConfig")
	}

	// Verify default value
	if val, ok := config.Settings["include_activities"].(bool); !ok || !val {
		t.Error("Expected 'include_activities' to be true by default")
	}
}

func TestService_OutputsRegistered(t *testing.T) {
	// Test that outputs are registered on import

	// Check fit-json output
	output, ok := core.GetOutput("fit-json")
	if !ok {
		t.Error("fit-json output not registered")
	}
	if output == nil {
		t.Error("fit-json output is nil")
	}

	// Check fit-csv output
	output, ok = core.GetOutput("fit-csv")
	if !ok {
		t.Error("fit-csv output not registered")
	}
	if output == nil {
		t.Error("fit-csv output is nil")
	}
}

func TestService_ServiceRegistered(t *testing.T) {
	// Test that service is registered on import

	service, ok := core.GetService("fit")
	if !ok {
		t.Error("fit service not registered")
	}
	if service == nil {
		t.Error("fit service is nil")
	}

	// Verify it's the correct service
	if service.ID() != "fit" {
		t.Errorf("Expected service ID 'fit', got '%s'", service.ID())
	}
}
