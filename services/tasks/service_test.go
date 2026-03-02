package tasks

import (
	"testing"

	"github.com/fulgidus/revoco/services/core"
)

func TestService_ID(t *testing.T) {
	svc := New()
	if got := svc.ID(); got != "tasks" {
		t.Errorf("ID() = %s, want 'tasks'", got)
	}
}

func TestService_Name(t *testing.T) {
	svc := New()
	if got := svc.Name(); got != "Google Tasks" {
		t.Errorf("Name() = %s, want 'Google Tasks'", got)
	}
}

func TestService_Description(t *testing.T) {
	svc := New()
	desc := svc.Description()
	if desc == "" {
		t.Error("Description() returned empty string")
	}
	if desc != "Process Google Tasks Takeout exports - Task lists to JSON/Markdown/CSV" {
		t.Errorf("Description() = %s", desc)
	}
}

func TestService_Ingesters(t *testing.T) {
	svc := New()
	ingesters := svc.Ingesters()
	if len(ingesters) != 3 {
		t.Errorf("Ingesters() returned %d ingesters, want 3", len(ingesters))
	}

	// Check ingester IDs
	expectedIDs := []string{"tasks-folder", "tasks-zip", "tasks-tgz"}
	for i, ing := range ingesters {
		if ing.ID() != expectedIDs[i] {
			t.Errorf("Ingester %d ID = %s, want %s", i, ing.ID(), expectedIDs[i])
		}
	}
}

func TestService_Processors(t *testing.T) {
	svc := New()
	processors := svc.Processors()
	if len(processors) != 1 {
		t.Errorf("Processors() returned %d processors, want 1", len(processors))
	}

	if processors[0].ID() != "tasks-processor" {
		t.Errorf("Processor ID = %s, want 'tasks-processor'", processors[0].ID())
	}
}

func TestService_SupportedOutputs(t *testing.T) {
	svc := New()
	outputs := svc.SupportedOutputs()
	if len(outputs) != 4 {
		t.Errorf("SupportedOutputs() returned %d outputs, want 4", len(outputs))
	}

	expectedOutputs := map[string]bool{
		"local-folder":   true,
		"tasks-json":     true,
		"tasks-markdown": true,
		"tasks-csv":      true,
	}

	for _, output := range outputs {
		if !expectedOutputs[output] {
			t.Errorf("Unexpected output: %s", output)
		}
	}

	// Verify specific outputs are present
	hasJSON := false
	hasMarkdown := false
	hasCSV := false
	for _, output := range outputs {
		switch output {
		case "tasks-json":
			hasJSON = true
		case "tasks-markdown":
			hasMarkdown = true
		case "tasks-csv":
			hasCSV = true
		}
	}

	if !hasJSON {
		t.Error("SupportedOutputs() missing 'tasks-json'")
	}
	if !hasMarkdown {
		t.Error("SupportedOutputs() missing 'tasks-markdown'")
	}
	if !hasCSV {
		t.Error("SupportedOutputs() missing 'tasks-csv'")
	}
}

func TestService_DefaultConfig(t *testing.T) {
	svc := New()
	cfg := svc.DefaultConfig()

	if cfg.Settings == nil {
		t.Error("DefaultConfig() Settings is nil")
	}

	// Tasks service has no default settings (empty map)
	if len(cfg.Settings) != 0 {
		t.Errorf("DefaultConfig() Settings has %d entries, want 0", len(cfg.Settings))
	}
}

func TestService_Registration(t *testing.T) {
	// Verify service is registered
	svc, ok := core.GetService("tasks")
	if !ok {
		t.Fatal("Tasks service not registered")
	}

	if svc.ID() != "tasks" {
		t.Errorf("Registered service ID = %s, want 'tasks'", svc.ID())
	}
}

func TestService_OutputsRegistered(t *testing.T) {
	// Verify outputs are registered
	outputs := []string{"tasks-json", "tasks-markdown", "tasks-csv"}

	for _, outputID := range outputs {
		output, ok := core.GetOutput(outputID)
		if !ok {
			t.Errorf("Output %s not registered", outputID)
		} else if output == nil {
			t.Errorf("Output %s is nil", outputID)
		}
	}
}
