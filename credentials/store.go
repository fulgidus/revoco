// Package credentials provides storage and management for connector authentication.
// It supports both global credentials (shared across sessions) and session-specific
// credentials, with encryption at rest using the secrets package.
//
// Storage layout:
//
//	Global:  ~/.revoco/credentials/<connector_id>/<credential_id>.json
//	Session: <session_dir>/credentials/<credential_id>.json
package credentials

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/secrets"
)

// ══════════════════════════════════════════════════════════════════════════════
// Store Interface
// ══════════════════════════════════════════════════════════════════════════════

// Store manages credential storage and retrieval.
type Store struct {
	globalDir  string // ~/.revoco/credentials
	sessionDir string // session-specific credentials dir (optional)
	password   string // encryption password (required for sensitive data)
}

// NewGlobalStore creates a store for global credentials only.
func NewGlobalStore(password string) (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("credentials: home dir: %w", err)
	}

	globalDir := filepath.Join(home, ".revoco", "credentials")
	if err := os.MkdirAll(globalDir, 0o700); err != nil {
		return nil, fmt.Errorf("credentials: create global dir: %w", err)
	}

	return &Store{
		globalDir: globalDir,
		password:  password,
	}, nil
}

// NewStore creates a store with both global and session-specific credential access.
func NewStore(sessionDir, password string) (*Store, error) {
	store, err := NewGlobalStore(password)
	if err != nil {
		return nil, err
	}

	if sessionDir != "" {
		store.sessionDir = filepath.Join(sessionDir, "credentials")
		if err := os.MkdirAll(store.sessionDir, 0o700); err != nil {
			return nil, fmt.Errorf("credentials: create session dir: %w", err)
		}
	}

	return store, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Credential Operations
// ══════════════════════════════════════════════════════════════════════════════

// Save stores a credential. Global credentials are stored in ~/.revoco/credentials/,
// session credentials are stored in the session's credentials directory.
func (s *Store) Save(cred *core.Credential) error {
	if cred.ID == "" {
		return fmt.Errorf("credentials: ID cannot be empty")
	}
	if cred.ConnectorID == "" {
		return fmt.Errorf("credentials: ConnectorID cannot be empty")
	}

	// Update timestamps
	now := time.Now().Unix()
	if cred.CreatedAt == 0 {
		cred.CreatedAt = now
	}
	cred.UpdatedAt = now

	// Determine storage path
	path := s.pathFor(cred)

	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("credentials: create dir: %w", err)
	}

	// If credential has sensitive data and we have a password, encrypt it
	if s.password != "" && hasSensitiveData(cred) {
		return s.saveEncrypted(path, cred)
	}

	// Otherwise, save as plain JSON (for non-sensitive metadata)
	return s.savePlain(path, cred)
}

// Load retrieves a credential by ID. It searches session-specific credentials
// first, then falls back to global credentials.
func (s *Store) Load(id string) (*core.Credential, error) {
	// Try session-specific first
	if s.sessionDir != "" {
		path := filepath.Join(s.sessionDir, id+".json")
		if cred, err := s.loadFrom(path); err == nil {
			return cred, nil
		}
	}

	// Try global credentials
	cred, err := s.loadFromGlobal(id)
	if err != nil {
		return nil, fmt.Errorf("credentials: %s not found", id)
	}
	return cred, nil
}

// LoadForConnector retrieves a credential by ID that must match the specified connector.
func (s *Store) LoadForConnector(id, connectorID string) (*core.Credential, error) {
	cred, err := s.Load(id)
	if err != nil {
		return nil, err
	}
	if cred.ConnectorID != connectorID {
		return nil, fmt.Errorf("credentials: %s is for connector %s, not %s", id, cred.ConnectorID, connectorID)
	}
	return cred, nil
}

// Delete removes a credential by ID.
func (s *Store) Delete(id string) error {
	// Try session-specific first
	if s.sessionDir != "" {
		path := filepath.Join(s.sessionDir, id+".json")
		if err := os.Remove(path); err == nil {
			return nil
		}
	}

	// Try global credentials
	if err := s.deleteFromGlobal(id); err != nil {
		return fmt.Errorf("credentials: %s not found", id)
	}
	return nil
}

// List returns all available credentials (both global and session-specific).
func (s *Store) List() ([]*core.Credential, error) {
	var creds []*core.Credential
	seen := make(map[string]bool)

	// Session-specific credentials first (take precedence)
	if s.sessionDir != "" {
		sessionCreds, _ := s.listFrom(s.sessionDir)
		for _, c := range sessionCreds {
			creds = append(creds, c)
			seen[c.ID] = true
		}
	}

	// Global credentials
	globalCreds, _ := s.listFromGlobal()
	for _, c := range globalCreds {
		if !seen[c.ID] {
			creds = append(creds, c)
		}
	}

	return creds, nil
}

// ListForConnector returns credentials for a specific connector type.
func (s *Store) ListForConnector(connectorID string) ([]*core.Credential, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}

	var filtered []*core.Credential
	for _, c := range all {
		if c.ConnectorID == connectorID {
			filtered = append(filtered, c)
		}
	}
	return filtered, nil
}

// IsExpired checks if a credential has expired.
func IsExpired(cred *core.Credential) bool {
	if cred.ExpiresAt == 0 {
		return false
	}
	return time.Now().Unix() > cred.ExpiresAt
}

// ══════════════════════════════════════════════════════════════════════════════
// Internal Helpers
// ══════════════════════════════════════════════════════════════════════════════

func (s *Store) pathFor(cred *core.Credential) string {
	if cred.Scope == core.CredentialScopeSession && s.sessionDir != "" {
		return filepath.Join(s.sessionDir, cred.ID+".json")
	}
	return filepath.Join(s.globalDir, cred.ConnectorID, cred.ID+".json")
}

func (s *Store) savePlain(path string, cred *core.Credential) error {
	data, err := json.MarshalIndent(cred, "", "  ")
	if err != nil {
		return fmt.Errorf("credentials: marshal: %w", err)
	}
	return os.WriteFile(path, data, 0o600)
}

func (s *Store) saveEncrypted(path string, cred *core.Credential) error {
	// Convert credential to secrets.Payload format
	credJSON, err := json.Marshal(cred)
	if err != nil {
		return fmt.Errorf("credentials: marshal: %w", err)
	}

	payload := secrets.Payload{
		"credential": string(credJSON),
	}

	return secrets.Encrypt(path, s.password, payload)
}

func (s *Store) loadFrom(path string) (*core.Credential, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Try plain JSON first
	var cred core.Credential
	if err := json.Unmarshal(data, &cred); err == nil && cred.ID != "" {
		return &cred, nil
	}

	// Try encrypted format
	if s.password != "" {
		return s.loadEncrypted(path)
	}

	return nil, fmt.Errorf("credentials: invalid format")
}

func (s *Store) loadEncrypted(path string) (*core.Credential, error) {
	payload, err := secrets.Decrypt(path, s.password)
	if err != nil {
		return nil, err
	}

	credJSON, ok := payload["credential"]
	if !ok {
		return nil, fmt.Errorf("credentials: missing credential in encrypted payload")
	}

	var cred core.Credential
	if err := json.Unmarshal([]byte(credJSON), &cred); err != nil {
		return nil, fmt.Errorf("credentials: unmarshal: %w", err)
	}

	return &cred, nil
}

func (s *Store) loadFromGlobal(id string) (*core.Credential, error) {
	// Search all connector subdirectories
	entries, err := os.ReadDir(s.globalDir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(s.globalDir, e.Name(), id+".json")
		if cred, err := s.loadFrom(path); err == nil {
			return cred, nil
		}
	}

	return nil, fmt.Errorf("not found")
}

func (s *Store) deleteFromGlobal(id string) error {
	// Search all connector subdirectories
	entries, err := os.ReadDir(s.globalDir)
	if err != nil {
		return err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(s.globalDir, e.Name(), id+".json")
		if err := os.Remove(path); err == nil {
			return nil
		}
	}

	return fmt.Errorf("not found")
}

func (s *Store) listFrom(dir string) ([]*core.Credential, error) {
	var creds []*core.Credential

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if cred, err := s.loadFrom(path); err == nil {
			creds = append(creds, cred)
		}
	}

	return creds, nil
}

func (s *Store) listFromGlobal() ([]*core.Credential, error) {
	var creds []*core.Credential

	// Walk connector subdirectories
	entries, err := os.ReadDir(s.globalDir)
	if err != nil {
		return nil, err
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		subdir := filepath.Join(s.globalDir, e.Name())
		subCreds, _ := s.listFrom(subdir)
		creds = append(creds, subCreds...)
	}

	return creds, nil
}

func hasSensitiveData(cred *core.Credential) bool {
	if cred.Data == nil {
		return false
	}
	// Consider these auth types as sensitive
	sensitiveTypes := []string{"oauth", "apikey", "cookie", "token", "password"}
	for _, t := range sensitiveTypes {
		if cred.AuthType == t {
			return true
		}
	}
	return false
}

// ══════════════════════════════════════════════════════════════════════════════
// Factory Functions
// ══════════════════════════════════════════════════════════════════════════════

// NewCredential creates a new credential with a generated ID.
func NewCredential(connectorID, name, authType string, scope core.CredentialScope) *core.Credential {
	return &core.Credential{
		ID:          generateID(),
		Name:        name,
		ConnectorID: connectorID,
		Scope:       scope,
		AuthType:    authType,
		Data:        make(map[string]any),
		CreatedAt:   time.Now().Unix(),
	}
}

// NewOAuthCredential creates a credential for OAuth authentication.
func NewOAuthCredential(connectorID, name string, scope core.CredentialScope, accessToken, refreshToken string, expiresAt int64) *core.Credential {
	cred := NewCredential(connectorID, name, "oauth", scope)
	cred.Data["access_token"] = accessToken
	cred.Data["refresh_token"] = refreshToken
	cred.ExpiresAt = expiresAt
	return cred
}

// NewAPIKeyCredential creates a credential for API key authentication.
func NewAPIKeyCredential(connectorID, name string, scope core.CredentialScope, apiKey string) *core.Credential {
	cred := NewCredential(connectorID, name, "apikey", scope)
	cred.Data["api_key"] = apiKey
	return cred
}

// NewCookieCredential creates a credential for cookie-based authentication.
func NewCookieCredential(connectorID, name string, scope core.CredentialScope, cookies map[string]string) *core.Credential {
	cred := NewCredential(connectorID, name, "cookie", scope)
	for k, v := range cookies {
		cred.Data[k] = v
	}
	return cred
}

// generateID creates a simple unique ID based on timestamp.
func generateID() string {
	return fmt.Sprintf("cred_%d", time.Now().UnixNano())
}
