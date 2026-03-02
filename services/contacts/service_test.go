package contacts

import (
	"testing"

	"github.com/fulgidus/revoco/services/core"
)

func TestContactsServiceRegistration(t *testing.T) {
	// Verify service is registered
	svc, ok := core.GetService("contacts")
	if !ok {
		t.Fatal("Contacts service not registered")
	}

	if svc.ID() != "contacts" {
		t.Errorf("Service ID = %q, want %q", svc.ID(), "contacts")
	}

	if svc.Name() != "Google Contacts" {
		t.Errorf("Service Name = %q, want %q", svc.Name(), "Google Contacts")
	}

	desc := svc.Description()
	if desc == "" {
		t.Error("Service Description is empty")
	}
}

func TestContactsServiceMetadata(t *testing.T) {
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

func TestContactsServiceIngesters(t *testing.T) {
	svc := New()

	ingesters := svc.Ingesters()
	if len(ingesters) != 3 {
		t.Errorf("Ingesters count = %d, want 3 (folder, zip, tgz)", len(ingesters))
	}

	// Verify ingester IDs have correct prefix
	wantPrefix := "contacts-"
	for _, ing := range ingesters {
		if ing.ID()[:len(wantPrefix)] != wantPrefix {
			t.Errorf("Ingester ID %q doesn't start with %q", ing.ID(), wantPrefix)
		}
	}
}

func TestContactsServiceProcessors(t *testing.T) {
	svc := New()

	processors := svc.Processors()
	if len(processors) != 1 {
		t.Errorf("Processors count = %d, want 1", len(processors))
	}

	if processors[0].ID() != "contacts-processor" {
		t.Errorf("Processor ID = %q, want %q", processors[0].ID(), "contacts-processor")
	}
}

func TestContactsServiceSupportedOutputs(t *testing.T) {
	svc := New()

	outputs := svc.SupportedOutputs()
	if len(outputs) < 3 {
		t.Errorf("SupportedOutputs count = %d, want at least 3", len(outputs))
	}

	wantOutputs := map[string]bool{
		"contacts-vcf":  true,
		"contacts-json": true,
		"contacts-csv":  true,
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

func TestContactsServiceDefaultConfig(t *testing.T) {
	svc := New()

	cfg := svc.DefaultConfig()
	if cfg.Settings == nil {
		t.Fatal("DefaultConfig.Settings is nil")
	}

	// Check for expected settings
	if _, ok := cfg.Settings["merge_duplicates"]; !ok {
		t.Error("DefaultConfig missing 'merge_duplicates' setting")
	}

	if _, ok := cfg.Settings["extract_photos"]; !ok {
		t.Error("DefaultConfig missing 'extract_photos' setting")
	}
}
