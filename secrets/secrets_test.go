package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

// tmpSecretsFile returns a temp file path for tests.
func tmpSecretsFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "secrets.json")
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	path := tmpSecretsFile(t)
	password := "test-password-42"

	payload := Payload{
		"chrome_v11": "my-secret-key",
		"api_token":  "tok_abc123",
	}

	if err := Encrypt(path, password, payload); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// File should exist
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("secrets file not created: %v", err)
	}

	// File permissions should be 0600
	info, _ := os.Stat(path)
	perm := info.Mode().Perm()
	if perm != 0o600 {
		t.Errorf("expected perm 0600, got %04o", perm)
	}

	// Decrypt with correct password
	got, err := Decrypt(path, password)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if got["chrome_v11"] != "my-secret-key" {
		t.Errorf("chrome_v11: got %q, want %q", got["chrome_v11"], "my-secret-key")
	}
	if got["api_token"] != "tok_abc123" {
		t.Errorf("api_token: got %q, want %q", got["api_token"], "tok_abc123")
	}
}

func TestDecryptWrongPassword(t *testing.T) {
	path := tmpSecretsFile(t)
	password := "correct-password"

	payload := Payload{"key": "value"}
	if err := Encrypt(path, password, payload); err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	_, err := Decrypt(path, "wrong-password")
	if err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
}

func TestDecryptMissingFile(t *testing.T) {
	_, err := Decrypt("/nonexistent/path/secrets.json", "pw")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestDecryptCorruptFile(t *testing.T) {
	path := tmpSecretsFile(t)
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := Decrypt(path, "pw")
	if err == nil {
		t.Fatal("expected error for corrupt file, got nil")
	}
}

func TestExists(t *testing.T) {
	path := tmpSecretsFile(t)

	if Exists(path) {
		t.Error("Exists returned true for nonexistent file")
	}

	payload := Payload{"k": "v"}
	if err := Encrypt(path, "pw", payload); err != nil {
		t.Fatal(err)
	}

	if !Exists(path) {
		t.Error("Exists returned false for existing file")
	}
}

func TestStoreAndGet(t *testing.T) {
	path := tmpSecretsFile(t)
	password := "store-pw"

	// Store first key (creates file)
	if err := Store(path, password, "key1", "val1"); err != nil {
		t.Fatalf("Store key1: %v", err)
	}

	// Store second key (adds to existing)
	if err := Store(path, password, "key2", "val2"); err != nil {
		t.Fatalf("Store key2: %v", err)
	}

	// Get both keys
	v1, err := Get(path, password, "key1")
	if err != nil {
		t.Fatalf("Get key1: %v", err)
	}
	if v1 != "val1" {
		t.Errorf("key1: got %q, want %q", v1, "val1")
	}

	v2, err := Get(path, password, "key2")
	if err != nil {
		t.Fatalf("Get key2: %v", err)
	}
	if v2 != "val2" {
		t.Errorf("key2: got %q, want %q", v2, "val2")
	}

	// Get nonexistent key
	_, err = Get(path, password, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent key, got nil")
	}
}

func TestDelete(t *testing.T) {
	path := tmpSecretsFile(t)
	password := "del-pw"

	// Create with two keys
	if err := Store(path, password, "keep", "kept"); err != nil {
		t.Fatal(err)
	}
	if err := Store(path, password, "remove", "gone"); err != nil {
		t.Fatal(err)
	}

	// Delete one key
	if err := Delete(path, password, "remove"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Verify removed key is gone
	_, err := Get(path, password, "remove")
	if err == nil {
		t.Error("expected error for deleted key, got nil")
	}

	// Verify kept key still exists
	v, err := Get(path, password, "keep")
	if err != nil {
		t.Fatalf("Get kept: %v", err)
	}
	if v != "kept" {
		t.Errorf("keep: got %q, want %q", v, "kept")
	}
}

func TestStoreOverwrite(t *testing.T) {
	path := tmpSecretsFile(t)
	password := "overwrite-pw"

	if err := Store(path, password, "key", "original"); err != nil {
		t.Fatal(err)
	}
	if err := Store(path, password, "key", "updated"); err != nil {
		t.Fatal(err)
	}

	v, err := Get(path, password, "key")
	if err != nil {
		t.Fatal(err)
	}
	if v != "updated" {
		t.Errorf("got %q, want %q", v, "updated")
	}
}

func TestEncryptEmptyPayload(t *testing.T) {
	path := tmpSecretsFile(t)
	password := "empty-pw"

	payload := Payload{}
	if err := Encrypt(path, password, payload); err != nil {
		t.Fatalf("Encrypt empty: %v", err)
	}

	got, err := Decrypt(path, password)
	if err != nil {
		t.Fatalf("Decrypt empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty payload, got %d keys", len(got))
	}
}

func TestEncryptCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "deep", "nested", "secrets.json")

	payload := Payload{"k": "v"}
	if err := Encrypt(path, "pw", payload); err != nil {
		t.Fatalf("Encrypt with nested path: %v", err)
	}

	if !Exists(path) {
		t.Error("file not created at nested path")
	}
}

func TestDefaultPath(t *testing.T) {
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("DefaultPath: %v", err)
	}
	if !filepath.IsAbs(path) {
		t.Errorf("expected absolute path, got %q", path)
	}
	if filepath.Base(path) != "secrets.json" {
		t.Errorf("expected secrets.json, got %q", filepath.Base(path))
	}
}
