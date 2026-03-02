package calendar

import (
	"testing"

	"github.com/fulgidus/revoco/services/core"
)

func TestCalendarServiceRegistration(t *testing.T) {
	// Test that service is registered
	svc, exists := core.GetService(ServiceID)
	if !exists {
		t.Fatalf("calendar service not registered")
	}

	if svc.ID() != ServiceID {
		t.Errorf("expected service ID %q, got %q", ServiceID, svc.ID())
	}

	if svc.Name() != "Google Calendar" {
		t.Errorf("expected service name 'Google Calendar', got %q", svc.Name())
	}

	desc := svc.Description()
	if desc == "" {
		t.Error("service description is empty")
	}
}

func TestCalendarServiceIngesters(t *testing.T) {
	svc := New()

	ingesters := svc.Ingesters()
	if len(ingesters) == 0 {
		t.Fatal("expected at least one ingester")
	}

	// Should have folder, zip, tgz ingesters
	if len(ingesters) < 3 {
		t.Errorf("expected at least 3 ingesters (folder, zip, tgz), got %d", len(ingesters))
	}

	// Check that each ingester has basic properties
	for i, ing := range ingesters {
		if ing.ID() == "" {
			t.Errorf("ingester %d has empty ID", i)
		}
		if ing.Name() == "" {
			t.Errorf("ingester %d has empty name", i)
		}
	}
}

func TestCalendarServiceProcessors(t *testing.T) {
	svc := New()

	processors := svc.Processors()
	if len(processors) != 1 {
		t.Fatalf("expected 1 processor, got %d", len(processors))
	}

	proc := processors[0]
	if proc.ID() != "calendar-processor" {
		t.Errorf("expected processor ID 'calendar-processor', got %q", proc.ID())
	}

	if proc.Name() == "" {
		t.Error("processor name is empty")
	}

	if proc.Description() == "" {
		t.Error("processor description is empty")
	}

	// Check config schema
	schema := proc.ConfigSchema()
	if len(schema) == 0 {
		t.Error("processor config schema is empty")
	}
}

func TestCalendarServiceOutputs(t *testing.T) {
	svc := New()

	outputs := svc.SupportedOutputs()
	if len(outputs) == 0 {
		t.Fatal("expected at least one supported output")
	}

	// Only check service-specific outputs (local-folder is a common output)
	expectedOutputs := []string{"calendar-ics", "calendar-json", "calendar-csv"}

	// Verify outputs are registered
	for _, outID := range expectedOutputs {
		_, exists := core.GetOutput(outID)
		if !exists {
			t.Errorf("output %q not registered", outID)
		}
	}
}

func TestCalendarServiceDefaultConfig(t *testing.T) {
	svc := New()

	cfg := svc.DefaultConfig()
	if cfg.Settings == nil {
		t.Fatal("default config settings is nil")
	}

	// Check expected settings
	if _, ok := cfg.Settings["timezone_convert"]; !ok {
		t.Error("default config missing 'timezone_convert' setting")
	}

	if _, ok := cfg.Settings["exclude_cancelled"]; !ok {
		t.Error("default config missing 'exclude_cancelled' setting")
	}
}
