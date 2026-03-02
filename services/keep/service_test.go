package keep_test

import (
	"testing"

	"github.com/fulgidus/revoco/services/keep"
)

func TestKeepServiceRegistration(t *testing.T) {
	svc := keep.New()

	if svc.ID() != "keep" {
		t.Errorf("Expected ID 'keep', got %q", svc.ID())
	}

	if svc.Name() != "Google Keep" {
		t.Errorf("Expected Name 'Google Keep', got %q", svc.Name())
	}

	if svc.Description() == "" {
		t.Error("Expected non-empty Description")
	}
}

func TestKeepServiceIngesters(t *testing.T) {
	svc := keep.New()
	ingesters := svc.Ingesters()

	if len(ingesters) == 0 {
		t.Fatal("Expected at least one ingester")
	}

	// Should have folder, ZIP, TGZ ingesters
	if len(ingesters) != 3 {
		t.Errorf("Expected 3 ingesters, got %d", len(ingesters))
	}
}

func TestKeepServiceProcessors(t *testing.T) {
	svc := keep.New()
	processors := svc.Processors()

	if len(processors) == 0 {
		t.Fatal("Expected at least one processor")
	}

	if len(processors) != 1 {
		t.Errorf("Expected 1 processor, got %d", len(processors))
	}

	proc := processors[0]
	if proc.ID() != "keep-processor" {
		t.Errorf("Expected processor ID 'keep-processor', got %q", proc.ID())
	}
}

func TestKeepServiceOutputs(t *testing.T) {
	svc := keep.New()
	outputs := svc.SupportedOutputs()

	if len(outputs) == 0 {
		t.Fatal("Expected at least one output")
	}

	expectedOutputs := map[string]bool{
		"local-folder": true,
		"keep-md":      true,
		"keep-json":    true,
		"keep-html":    true,
	}

	for _, output := range outputs {
		if !expectedOutputs[output] {
			t.Errorf("Unexpected output: %s", output)
		}
	}
}

func TestKeepServiceDefaultConfig(t *testing.T) {
	svc := keep.New()
	cfg := svc.DefaultConfig()

	if cfg.Settings == nil {
		t.Fatal("Expected non-nil default settings")
	}

	// Check default settings
	expectedSettings := []string{
		"preserve_colors",
		"export_archived",
		"markdown_format",
		"include_labels",
		"timestamp_format",
	}

	for _, key := range expectedSettings {
		if _, ok := cfg.Settings[key]; !ok {
			t.Errorf("Expected default setting %q not found", key)
		}
	}
}

