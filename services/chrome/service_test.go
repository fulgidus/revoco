package chrome

import (
	"testing"

	"github.com/fulgidus/revoco/services/core"
)

// ── Service Registration Tests ────────────────────────────────────────────────

func TestChromeService_Registration(t *testing.T) {
	svc, ok := core.GetService("chrome")
	if !ok {
		t.Fatal("chrome service not registered")
	}

	if svc.ID() != "chrome" {
		t.Errorf("expected ID 'chrome', got %q", svc.ID())
	}

	if svc.Name() != "Chrome" {
		t.Errorf("expected name 'Chrome', got %q", svc.Name())
	}

	if svc.Description() == "" {
		t.Error("description should not be empty")
	}
}

// ── Service Metadata Tests ────────────────────────────────────────────────────

func TestChromeService_ID(t *testing.T) {
	svc := New()
	if svc.ID() != ServiceID {
		t.Errorf("expected ID %q, got %q", ServiceID, svc.ID())
	}
	if svc.ID() != "chrome" {
		t.Errorf("expected ID 'chrome', got %q", svc.ID())
	}
}

func TestChromeService_Name(t *testing.T) {
	svc := New()
	if svc.Name() != "Chrome" {
		t.Errorf("expected name 'Chrome', got %q", svc.Name())
	}
}

func TestChromeService_Description(t *testing.T) {
	svc := New()
	desc := svc.Description()
	if desc == "" {
		t.Error("description should not be empty")
	}
	if len(desc) < 10 {
		t.Errorf("description too short: %q", desc)
	}
}

// ── Ingesters Tests ───────────────────────────────────────────────────────────

func TestChromeService_Ingesters(t *testing.T) {
	svc := New()
	ingesters := svc.Ingesters()

	if len(ingesters) == 0 {
		t.Fatal("expected at least one ingester")
	}

	// Check for common ingester types (folder, zip, tgz)
	hasFolder := false
	for _, ing := range ingesters {
		if ing.ID() == "chrome-folder" {
			hasFolder = true
		}
	}

	if !hasFolder {
		t.Error("expected folder ingester to be present")
	}
}

// ── Processors Tests ──────────────────────────────────────────────────────────

func TestChromeService_Processors(t *testing.T) {
	svc := New()
	processors := svc.Processors()

	if len(processors) == 0 {
		t.Fatal("expected at least one processor")
	}

	proc := processors[0]
	if proc.ID() != "chrome-processor" {
		t.Errorf("expected processor ID 'chrome-processor', got %q", proc.ID())
	}

	// Check processor has config schema
	schema := proc.ConfigSchema()
	if len(schema) == 0 {
		t.Error("expected processor to have config schema")
	}

	// Check for expected config options
	hasSearchEngines := false
	hasAutofill := false
	for _, opt := range schema {
		if opt.ID == "include_search_engines" {
			hasSearchEngines = true
		}
		if opt.ID == "include_autofill" {
			hasAutofill = true
		}
	}

	if !hasSearchEngines {
		t.Error("expected 'include_search_engines' config option")
	}
	if !hasAutofill {
		t.Error("expected 'include_autofill' config option")
	}
}

// ── SupportedOutputs Tests ────────────────────────────────────────────────────

func TestChromeService_SupportedOutputs(t *testing.T) {
	svc := New()
	outputs := svc.SupportedOutputs()

	if len(outputs) != 4 {
		t.Fatalf("expected 4 outputs, got %d", len(outputs))
	}

	// Expected outputs
	expectedOutputs := map[string]bool{
		"local-folder": true,
		"chrome-json":  true,
		"chrome-html":  true,
		"chrome-csv":   true,
	}

	for _, out := range outputs {
		if !expectedOutputs[out] {
			t.Errorf("unexpected output: %q", out)
		}
		delete(expectedOutputs, out)
	}

	if len(expectedOutputs) > 0 {
		t.Errorf("missing outputs: %v", expectedOutputs)
	}
}

func TestChromeService_OutputsRegistered(t *testing.T) {
	// Verify outputs are actually registered
	outputs := []string{"chrome-json", "chrome-html", "chrome-csv"}

	for _, id := range outputs {
		out, ok := core.GetOutput(id)
		if !ok {
			t.Errorf("output %q not registered", id)
			continue
		}

		// Check basic metadata
		if out.ID() != id {
			t.Errorf("output ID mismatch: expected %q, got %q", id, out.ID())
		}
		if out.Name() == "" {
			t.Errorf("output %q has empty name", id)
		}
		if out.Description() == "" {
			t.Errorf("output %q has empty description", id)
		}
	}
}

// ── DefaultConfig Tests ───────────────────────────────────────────────────────

func TestChromeService_DefaultConfig(t *testing.T) {
	svc := New()
	cfg := svc.DefaultConfig()

	if cfg.Settings == nil {
		t.Fatal("default config settings should not be nil")
	}

	// Check expected settings
	if val, ok := cfg.Settings["include_search_engines"].(bool); !ok || val {
		t.Error("expected 'include_search_engines' to be false by default")
	}

	if val, ok := cfg.Settings["include_autofill"].(bool); !ok || val {
		t.Error("expected 'include_autofill' to be false by default")
	}
}

// ── Integration Test ──────────────────────────────────────────────────────────

func TestChromeService_FullIntegration(t *testing.T) {
	// Verify complete service pipeline
	svc, ok := core.GetService("chrome")
	if !ok {
		t.Fatal("chrome service not found in registry")
	}

	// Check all components are wired
	ingesters := svc.Ingesters()
	if len(ingesters) == 0 {
		t.Error("no ingesters available")
	}

	processors := svc.Processors()
	if len(processors) == 0 {
		t.Error("no processors available")
	}

	outputs := svc.SupportedOutputs()
	if len(outputs) < 3 {
		t.Errorf("expected at least 3 outputs, got %d", len(outputs))
	}

	// Verify each declared output is actually registered
	for _, outID := range outputs {
		if outID == "local-folder" {
			continue // Built-in output
		}
		out, ok := core.GetOutput(outID)
		if !ok {
		_ = out // silence unused warning
			t.Errorf("declared output %q not found in registry", outID)
		}
	}

	// Verify default config is usable
	cfg := svc.DefaultConfig()
	if cfg.Settings == nil {
		t.Error("default config has nil settings")
	}
}
