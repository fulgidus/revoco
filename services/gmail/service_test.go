package gmail

import (
	"testing"

	"github.com/fulgidus/revoco/services/core"
)

func TestGmailServiceRegistration(t *testing.T) {
	// Verify service is registered
	svc, ok := core.GetService("gmail")
	if !ok {
		t.Fatal("Gmail service not registered")
	}

	if svc.ID() != "gmail" {
		t.Errorf("Service ID = %q, want %q", svc.ID(), "gmail")
	}

	if svc.Name() != "Gmail" {
		t.Errorf("Service Name = %q, want %q", svc.Name(), "Gmail")
	}

	desc := svc.Description()
	if desc == "" {
		t.Error("Service Description is empty")
	}
}

func TestGmailServiceMetadata(t *testing.T) {
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
}

func TestGmailServiceIngesters(t *testing.T) {
	svc := New()

	ingesters := svc.Ingesters()
	if len(ingesters) != 3 {
		t.Errorf("Ingesters count = %d, want 3 (folder, zip, tgz)", len(ingesters))
	}

	// Verify ingester IDs have correct prefix
	wantPrefix := "gmail-"
	for _, ing := range ingesters {
		if ing.ID()[:len(wantPrefix)] != wantPrefix {
			t.Errorf("Ingester ID %q doesn't start with %q", ing.ID(), wantPrefix)
		}
	}
}

func TestGmailServiceProcessors(t *testing.T) {
	svc := New()

	processors := svc.Processors()
	if len(processors) != 1 {
		t.Errorf("Processors count = %d, want 1", len(processors))
	}

	if processors[0].ID() != "gmail-processor" {
		t.Errorf("Processor ID = %q, want %q", processors[0].ID(), "gmail-processor")
	}
}

func TestGmailServiceSupportedOutputs(t *testing.T) {
	svc := New()

	outputs := svc.SupportedOutputs()
	if len(outputs) < 3 {
		t.Errorf("SupportedOutputs count = %d, want at least 3", len(outputs))
	}

	wantOutputs := map[string]bool{
		"gmail-json": true,
		"gmail-eml":  true,
		"gmail-csv":  true,
	}

	for _, id := range outputs {
		if wantOutputs[id] {
			delete(wantOutputs, id)
		}
	}

	if len(wantOutputs) > 0 {
		t.Errorf("Missing expected outputs: %v", wantOutputs)
	}
}

func TestGmailServiceDefaultConfig(t *testing.T) {
	svc := New()

	cfg := svc.DefaultConfig()
	if cfg.Settings == nil {
		t.Fatal("DefaultConfig.Settings is nil")
	}

	// Check for expected settings
	if _, ok := cfg.Settings["extract_attachments"]; !ok {
		t.Error("DefaultConfig missing 'extract_attachments' setting")
	}

	if _, ok := cfg.Settings["body_preview_length"]; !ok {
		t.Error("DefaultConfig missing 'body_preview_length' setting")
	}
}
