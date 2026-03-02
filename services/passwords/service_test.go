package passwords_test

import (
	"testing"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/passwords"
)

func TestServiceID(t *testing.T) {
	svc := passwords.New()
	if got := svc.ID(); got != "passwords" {
		t.Errorf("ID() = %q, want %q", got, "passwords")
	}
}

func TestServiceName(t *testing.T) {
	svc := passwords.New()
	if got := svc.Name(); got != "Google Passwords" {
		t.Errorf("Name() = %q, want %q", got, "Google Passwords")
	}
}

func TestServiceDescription(t *testing.T) {
	svc := passwords.New()
	desc := svc.Description()
	if desc == "" {
		t.Error("Description() returned empty string")
	}
}

func TestServiceIngesters(t *testing.T) {
	svc := passwords.New()
	ings := svc.Ingesters()
	if len(ings) != 3 {
		t.Errorf("Ingesters() returned %d ingesters, want 3", len(ings))
	}
}

func TestServiceProcessors(t *testing.T) {
	svc := passwords.New()
	procs := svc.Processors()
	if len(procs) != 1 {
		t.Errorf("Processors() returned %d processors, want 1", len(procs))
	}
}

func TestServiceSupportedOutputs(t *testing.T) {
	svc := passwords.New()
	outputs := svc.SupportedOutputs()
	if len(outputs) < 2 {
		t.Errorf("SupportedOutputs() returned %d outputs, want at least 2", len(outputs))
	}
}

func TestServiceDefaultConfig(t *testing.T) {
	svc := passwords.New()
	cfg := svc.DefaultConfig()
	if cfg.Settings == nil {
		t.Error("DefaultConfig().Settings is nil")
	}
}

func TestServiceRegistration(t *testing.T) {
	// Service should auto-register via init()
	svc, ok := core.GetService("passwords")
	if !ok || svc == nil {
		t.Fatal("Service 'passwords' not registered")
	}

	if svc.ID() != "passwords" {
		t.Errorf("Registered service ID = %q, want %q", svc.ID(), "passwords")
	}
}

func TestOutputsRegistered(t *testing.T) {
	// Check KeePass CSV output
	out, ok := core.GetOutput("passwords-keepass-csv")
	if !ok || out == nil {
		t.Error("Output 'passwords-keepass-csv' not registered")
	}

	// Check JSON output
	out, ok = core.GetOutput("passwords-json")
	if !ok || out == nil {
		t.Error("Output 'passwords-json' not registered")
	}
}
