// Package session manages revoco work sessions.
//
// Each session is a folder under ~/.revoco/sessions/<name>/ containing:
//
//	config.json      – session configuration (source, output, settings)
//	process.log      – processing pipeline audit log
//	recovery.log     – recovery download audit log
//	missing-files.json – generated report from Phase 8
//	failed.json      – failed recovery entries
//	output/          – processed files (non-destructive)
//	recovered/       – recovered files
//	source/          – imported takeout archive (if imported)
//
// Sessions are non-destructive: originals are never modified. All work
// products live inside the session folder.
package session

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	core "github.com/fulgidus/revoco/connectors"
)

// baseDir returns ~/.revoco/sessions.
func baseDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("session: home dir: %w", err)
	}
	return filepath.Join(home, ".revoco", "sessions"), nil
}

// SourceType describes how the takeout archive was provided.
type SourceType string

const (
	SourceFolder SourceType = "folder"
	SourceZip    SourceType = "zip"
	SourceTGZ    SourceType = "tgz"
)

// Source describes the input data for a session.
type Source struct {
	Type         SourceType `json:"type"`
	OriginalPath string     `json:"original_path"` // path the user provided
	ImportedPath string     `json:"imported_path"` // path inside session (if imported)
}

// RecoverSettings holds recovery-specific configuration.
type RecoverSettings struct {
	InputJSON   string  `json:"input_json"` // relative to session dir
	OutputDir   string  `json:"output_dir"` // relative to session dir
	Concurrency int     `json:"concurrency"`
	Delay       float64 `json:"delay"`
	MaxRetry    int     `json:"max_retry"`
	StartFrom   int     `json:"start_from"`
}

// ProcessorSettings holds service-specific processor configuration.
// Each service can store arbitrary settings as a JSON object.
type ProcessorSettings map[string]any

// OutputSettings holds output-specific configuration.
type OutputSettings struct {
	OutputID string         `json:"output_id"` // registered output identifier
	Config   map[string]any `json:"config"`    // output-specific configuration
}

// PipelineConfig holds the complete pipeline configuration for a session.
// DEPRECATED: Use ConnectorsConfig for new sessions.
type PipelineConfig struct {
	ServiceID         string            `json:"service_id"`         // e.g., "googlephotos", "youtubemusic"
	IngesterID        string            `json:"ingester_id"`        // e.g., "folder", "zip", "tgz"
	ProcessorSettings ProcessorSettings `json:"processor_settings"` // service-specific processor config
	OutputSettings    []OutputSettings  `json:"output_settings"`    // one or more outputs
}

// ══════════════════════════════════════════════════════════════════════════════
// Connector-Based Configuration (New Architecture)
// ══════════════════════════════════════════════════════════════════════════════

// ConnectorsConfig holds the connector-based configuration for a session.
type ConnectorsConfig struct {
	// Connectors holds all configured connector instances
	Connectors []core.ConnectorConfig `json:"connectors"`

	// ProcessorConfigs holds processor configuration
	ProcessorConfigs []core.ProcessorConfig `json:"processor_configs,omitempty"`

	// AutoProcess enables automatic processing after retrieval
	AutoProcess bool `json:"auto_process"`

	// ParallelRetrieval enables parallel data retrieval from multiple input connectors
	ParallelRetrieval bool `json:"parallel_retrieval"`

	// DetectedDataTypes stores data types found during scan
	DetectedDataTypes []core.DataType `json:"detected_data_types,omitempty"`

	// Stats holds current statistics
	Stats *core.DataStats `json:"stats,omitempty"`
}

// GetInputConnectors returns all connectors configured as input.
func (cc *ConnectorsConfig) GetInputConnectors() []core.ConnectorConfig {
	var inputs []core.ConnectorConfig
	for _, c := range cc.Connectors {
		if c.Enabled && c.Roles.IsInput {
			inputs = append(inputs, c)
		}
	}
	return inputs
}

// GetOutputConnectors returns all connectors configured as output.
func (cc *ConnectorsConfig) GetOutputConnectors() []core.ConnectorConfig {
	var outputs []core.ConnectorConfig
	for _, c := range cc.Connectors {
		if c.Enabled && c.Roles.IsOutput {
			outputs = append(outputs, c)
		}
	}
	return outputs
}

// GetFallbackConnectors returns all connectors configured as fallback.
func (cc *ConnectorsConfig) GetFallbackConnectors() []core.ConnectorConfig {
	var fallbacks []core.ConnectorConfig
	for _, c := range cc.Connectors {
		if c.Enabled && c.Roles.IsFallback {
			fallbacks = append(fallbacks, c)
		}
	}
	return fallbacks
}

// GetFallbacksFor returns fallback connectors for a specific connector instance.
func (cc *ConnectorsConfig) GetFallbacksFor(instanceID string) []core.ConnectorConfig {
	var fallbacks []core.ConnectorConfig
	for _, c := range cc.Connectors {
		if !c.Enabled {
			continue
		}
		for _, fbFor := range c.FallbackFor {
			if fbFor == instanceID {
				fallbacks = append(fallbacks, c)
				break
			}
		}
	}
	return fallbacks
}

// GetConnector returns a connector by instance ID.
func (cc *ConnectorsConfig) GetConnector(instanceID string) (core.ConnectorConfig, bool) {
	for _, c := range cc.Connectors {
		if c.InstanceID == instanceID {
			return c, true
		}
	}
	return core.ConnectorConfig{}, false
}

// AddConnector adds a new connector configuration.
func (cc *ConnectorsConfig) AddConnector(cfg core.ConnectorConfig) {
	cc.Connectors = append(cc.Connectors, cfg)
}

// UpdateConnector updates an existing connector by instance ID.
func (cc *ConnectorsConfig) UpdateConnector(cfg core.ConnectorConfig) bool {
	for i, c := range cc.Connectors {
		if c.InstanceID == cfg.InstanceID {
			cc.Connectors[i] = cfg
			return true
		}
	}
	return false
}

// RemoveConnector removes a connector by instance ID.
func (cc *ConnectorsConfig) RemoveConnector(instanceID string) bool {
	for i, c := range cc.Connectors {
		if c.InstanceID == instanceID {
			cc.Connectors = append(cc.Connectors[:i], cc.Connectors[i+1:]...)
			return true
		}
	}
	return false
}

// EnableConnector enables a connector by instance ID.
func (cc *ConnectorsConfig) EnableConnector(instanceID string) bool {
	for i, c := range cc.Connectors {
		if c.InstanceID == instanceID {
			cc.Connectors[i].Enabled = true
			return true
		}
	}
	return false
}

// DisableConnector disables a connector by instance ID.
func (cc *ConnectorsConfig) DisableConnector(instanceID string) bool {
	for i, c := range cc.Connectors {
		if c.InstanceID == instanceID {
			cc.Connectors[i].Enabled = false
			return true
		}
	}
	return false
}

// Status describes the current state of a session.
type Status string

const (
	StatusIdle       Status = "idle"
	StatusProcessing Status = "processing"
	StatusRecovering Status = "recovering"
	StatusDone       Status = "done"
	StatusError      Status = "error"
)

// Config is the persistent configuration for a session, stored as config.json.
type Config struct {
	Name               string          `json:"name"`
	Created            time.Time       `json:"created"`
	Updated            time.Time       `json:"updated"`
	Source             Source          `json:"source"`
	OutputDir          string          `json:"output_dir"` // relative to session dir, default "output"
	UseMove            bool            `json:"use_move"`
	DryRun             bool            `json:"dry_run"`
	Recover            RecoverSettings `json:"recover"`
	Status             Status          `json:"status"`
	LastPhaseCompleted int             `json:"last_phase_completed"`
	LastError          string          `json:"last_error,omitempty"`

	// Multi-service pipeline configuration (legacy)
	// DEPRECATED: Use Connectors for new sessions
	Pipeline PipelineConfig `json:"pipeline,omitempty"`

	// Connector-based configuration (new architecture)
	Connectors ConnectorsConfig `json:"connectors,omitempty"`

	// Version indicates the config schema version
	// 1 = legacy pipeline-based, 2 = connector-based
	Version int `json:"version,omitempty"`
}

// IsConnectorBased returns true if this session uses the new connector architecture.
func (c *Config) IsConnectorBased() bool {
	return c.Version >= 2 || len(c.Connectors.Connectors) > 0
}

// Session is the in-memory representation of a work session.
type Session struct {
	Config Config
	Dir    string // absolute path to session folder
}

// Dir returns the absolute path to a named session folder.
func Dir(name string) (string, error) {
	base, err := baseDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, name), nil
}

// ConfigPath returns the config.json path for a session.
func (s *Session) ConfigPath() string {
	return filepath.Join(s.Dir, "config.json")
}

// OutputPath returns the absolute output directory.
func (s *Session) OutputPath() string {
	if filepath.IsAbs(s.Config.OutputDir) {
		return s.Config.OutputDir
	}
	return filepath.Join(s.Dir, s.Config.OutputDir)
}

// SourcePath returns the effective source directory for processing.
// If the takeout was imported into the session, this is the imported path.
// Otherwise it is the original external path.
func (s *Session) SourcePath() string {
	if s.Config.Source.ImportedPath != "" {
		if filepath.IsAbs(s.Config.Source.ImportedPath) {
			return s.Config.Source.ImportedPath
		}
		return filepath.Join(s.Dir, s.Config.Source.ImportedPath)
	}
	return s.Config.Source.OriginalPath
}

// LogPath returns the path for a log file within the session.
func (s *Session) LogPath(name string) string {
	return filepath.Join(s.Dir, name)
}

// ServiceID returns the configured service ID, defaulting to "googlephotos" for backwards compatibility.
func (s *Session) ServiceID() string {
	if s.Config.Pipeline.ServiceID == "" {
		return "googlephotos"
	}
	return s.Config.Pipeline.ServiceID
}

// SetServiceID sets the service for this session's pipeline.
func (s *Session) SetServiceID(serviceID string) {
	s.Config.Pipeline.ServiceID = serviceID
}

// SetIngesterID sets the ingester for this session's pipeline.
func (s *Session) SetIngesterID(ingesterID string) {
	s.Config.Pipeline.IngesterID = ingesterID
}

// SetProcessorSettings sets the processor configuration for this session.
func (s *Session) SetProcessorSettings(settings ProcessorSettings) {
	s.Config.Pipeline.ProcessorSettings = settings
}

// GetProcessorSetting retrieves a single processor setting by key.
func (s *Session) GetProcessorSetting(key string) (any, bool) {
	if s.Config.Pipeline.ProcessorSettings == nil {
		return nil, false
	}
	v, ok := s.Config.Pipeline.ProcessorSettings[key]
	return v, ok
}

// GetProcessorSettingBool retrieves a boolean processor setting with a default.
func (s *Session) GetProcessorSettingBool(key string, defaultVal bool) bool {
	v, ok := s.GetProcessorSetting(key)
	if !ok {
		return defaultVal
	}
	if b, ok := v.(bool); ok {
		return b
	}
	return defaultVal
}

// GetProcessorSettingString retrieves a string processor setting with a default.
func (s *Session) GetProcessorSettingString(key string, defaultVal string) string {
	v, ok := s.GetProcessorSetting(key)
	if !ok {
		return defaultVal
	}
	if str, ok := v.(string); ok {
		return str
	}
	return defaultVal
}

// AddOutputSetting adds an output configuration to the pipeline.
func (s *Session) AddOutputSetting(outputID string, config map[string]any) {
	s.Config.Pipeline.OutputSettings = append(s.Config.Pipeline.OutputSettings, OutputSettings{
		OutputID: outputID,
		Config:   config,
	})
}

// ClearOutputSettings removes all output configurations.
func (s *Session) ClearOutputSettings() {
	s.Config.Pipeline.OutputSettings = nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Connector-Based Session Methods
// ══════════════════════════════════════════════════════════════════════════════

// IsConnectorBased returns true if this session uses the new connector architecture.
func (s *Session) IsConnectorBased() bool {
	return s.Config.IsConnectorBased()
}

// AddConnector adds a connector to this session.
func (s *Session) AddConnector(cfg core.ConnectorConfig) {
	s.Config.Connectors.AddConnector(cfg)
	// Upgrade version if needed
	if s.Config.Version < 2 {
		s.Config.Version = 2
	}
}

// GetConnector returns a connector by instance ID.
func (s *Session) GetConnector(instanceID string) (core.ConnectorConfig, bool) {
	return s.Config.Connectors.GetConnector(instanceID)
}

// UpdateConnector updates an existing connector configuration.
func (s *Session) UpdateConnector(cfg core.ConnectorConfig) bool {
	return s.Config.Connectors.UpdateConnector(cfg)
}

// RemoveConnector removes a connector by instance ID.
func (s *Session) RemoveConnector(instanceID string) bool {
	return s.Config.Connectors.RemoveConnector(instanceID)
}

// ListConnectors returns all connector configurations.
func (s *Session) ListConnectors() []core.ConnectorConfig {
	return s.Config.Connectors.Connectors
}

// GetInputConnectors returns connectors configured for input.
func (s *Session) GetInputConnectors() []core.ConnectorConfig {
	return s.Config.Connectors.GetInputConnectors()
}

// GetOutputConnectors returns connectors configured for output.
func (s *Session) GetOutputConnectors() []core.ConnectorConfig {
	return s.Config.Connectors.GetOutputConnectors()
}

// GetFallbackConnectors returns connectors configured as fallbacks.
func (s *Session) GetFallbackConnectors() []core.ConnectorConfig {
	return s.Config.Connectors.GetFallbackConnectors()
}

// SetAutoProcess enables or disables automatic processing.
func (s *Session) SetAutoProcess(enabled bool) {
	s.Config.Connectors.AutoProcess = enabled
}

// SetParallelRetrieval enables or disables parallel data retrieval.
func (s *Session) SetParallelRetrieval(enabled bool) {
	s.Config.Connectors.ParallelRetrieval = enabled
}

// DataDir returns the path for storing imported data within the session.
func (s *Session) DataDir() string {
	return filepath.Join(s.Dir, "data")
}

// CredentialsDir returns the path for session-specific credentials.
func (s *Session) CredentialsDir() string {
	return filepath.Join(s.Dir, "credentials")
}

// Save persists the session config to disk.
func (s *Session) Save() error {
	s.Config.Updated = time.Now()
	data, err := json.MarshalIndent(s.Config, "", "  ")
	if err != nil {
		return fmt.Errorf("session: marshal config: %w", err)
	}
	return os.WriteFile(s.ConfigPath(), data, 0o644)
}

// ── CRUD operations ──────────────────────────────────────────────────────────

// Create makes a new session with the given name.
func Create(name string) (*Session, error) {
	if name == "" {
		return nil, fmt.Errorf("session: name cannot be empty")
	}
	if strings.ContainsAny(name, "/\\:*?\"<>|") {
		return nil, fmt.Errorf("session: name contains invalid characters")
	}

	dir, err := Dir(name)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("session: %q already exists", name)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("session: create dir: %w", err)
	}

	// Create sub-directories
	for _, sub := range []string{"output", "recovered"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("session: create %s dir: %w", sub, err)
		}
	}

	now := time.Now()
	s := &Session{
		Dir: dir,
		Config: Config{
			Name:      name,
			Created:   now,
			Updated:   now,
			OutputDir: "output",
			Status:    StatusIdle,
			Recover: RecoverSettings{
				InputJSON:   "missing-files.json",
				OutputDir:   "recovered",
				Concurrency: 3,
				Delay:       1.0,
				MaxRetry:    3,
				StartFrom:   1,
			},
			// Default pipeline configuration for backwards compatibility
			Pipeline: PipelineConfig{
				ServiceID:  "googlephotos",
				IngesterID: "folder",
				ProcessorSettings: ProcessorSettings{
					"embed_exif":      true,
					"organize_albums": true,
					"deduplicate":     true,
					"convert_motion":  true,
					"use_move":        false,
				},
			},
		},
	}

	if err := s.Save(); err != nil {
		return nil, err
	}
	return s, nil
}

// CreateV2 creates a new session using the connector-based architecture.
// The session starts empty and connectors are added later.
func CreateV2(name string) (*Session, error) {
	if name == "" {
		return nil, fmt.Errorf("session: name cannot be empty")
	}
	if strings.ContainsAny(name, "/\\:*?\"<>|") {
		return nil, fmt.Errorf("session: name contains invalid characters")
	}

	dir, err := Dir(name)
	if err != nil {
		return nil, err
	}

	if _, err := os.Stat(dir); err == nil {
		return nil, fmt.Errorf("session: %q already exists", name)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("session: create dir: %w", err)
	}

	// Create sub-directories for new architecture
	for _, sub := range []string{"output", "recovered", "data", "credentials"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, fmt.Errorf("session: create %s dir: %w", sub, err)
		}
	}

	now := time.Now()
	s := &Session{
		Dir: dir,
		Config: Config{
			Name:      name,
			Created:   now,
			Updated:   now,
			OutputDir: "output",
			Status:    StatusIdle,
			Version:   2, // Connector-based architecture
			Recover: RecoverSettings{
				InputJSON:   "missing-files.json",
				OutputDir:   "recovered",
				Concurrency: 3,
				Delay:       1.0,
				MaxRetry:    3,
				StartFrom:   1,
			},
			Connectors: ConnectorsConfig{
				Connectors:        []core.ConnectorConfig{},
				ParallelRetrieval: true, // Smart default
				AutoProcess:       false,
			},
		},
	}

	if err := s.Save(); err != nil {
		return nil, err
	}
	return s, nil
}

// Load reads an existing session from disk.
func Load(name string) (*Session, error) {
	dir, err := Dir(name)
	if err != nil {
		return nil, err
	}

	cfgPath := filepath.Join(dir, "config.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("session: read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("session: parse config: %w", err)
	}

	return &Session{Dir: dir, Config: cfg}, nil
}

// List returns all session names sorted alphabetically.
func List() ([]string, error) {
	base, err := baseDir()
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(base, 0o755); err != nil {
		return nil, fmt.Errorf("session: create base dir: %w", err)
	}

	entries, err := os.ReadDir(base)
	if err != nil {
		return nil, fmt.Errorf("session: read dir: %w", err)
	}

	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		// Verify it has a config.json
		cfgPath := filepath.Join(base, e.Name(), "config.json")
		if _, err := os.Stat(cfgPath); err == nil {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// ListSessions returns all sessions with their configs loaded.
func ListSessions() ([]*Session, error) {
	names, err := List()
	if err != nil {
		return nil, err
	}
	sessions := make([]*Session, 0, len(names))
	for _, name := range names {
		s, err := Load(name)
		if err != nil {
			continue // skip broken sessions
		}
		sessions = append(sessions, s)
	}
	return sessions, nil
}

// Rename changes the name of an existing session (renames the folder).
func Rename(oldName, newName string) error {
	if newName == "" {
		return fmt.Errorf("session: new name cannot be empty")
	}
	if strings.ContainsAny(newName, "/\\:*?\"<>|") {
		return fmt.Errorf("session: new name contains invalid characters")
	}

	oldDir, err := Dir(oldName)
	if err != nil {
		return err
	}
	newDir, err := Dir(newName)
	if err != nil {
		return err
	}

	if _, err := os.Stat(oldDir); os.IsNotExist(err) {
		return fmt.Errorf("session: %q does not exist", oldName)
	}
	if _, err := os.Stat(newDir); err == nil {
		return fmt.Errorf("session: %q already exists", newName)
	}

	if err := os.Rename(oldDir, newDir); err != nil {
		return fmt.Errorf("session: rename: %w", err)
	}

	// Update name in config
	s, err := Load(newName)
	if err != nil {
		return err
	}
	s.Config.Name = newName
	return s.Save()
}

// Remove deletes a session and all its data permanently.
func Remove(name string) error {
	dir, err := Dir(name)
	if err != nil {
		return err
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return fmt.Errorf("session: %q does not exist", name)
	}
	return os.RemoveAll(dir)
}

// ── Import operations ────────────────────────────────────────────────────────

// ImportFolder copies a takeout folder into the session's source/ directory.
func (s *Session) ImportFolder(srcPath string) error {
	destDir := filepath.Join(s.Dir, "source")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("session: create source dir: %w", err)
	}

	if err := copyDirRecursive(srcPath, destDir); err != nil {
		return fmt.Errorf("session: import folder: %w", err)
	}

	s.Config.Source = Source{
		Type:         SourceFolder,
		OriginalPath: srcPath,
		ImportedPath: "source",
	}
	return s.Save()
}

// ImportZip extracts a .zip archive into the session's source/ directory.
func (s *Session) ImportZip(zipPath string) error {
	destDir := filepath.Join(s.Dir, "source")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("session: create source dir: %w", err)
	}

	if err := extractZip(zipPath, destDir); err != nil {
		return fmt.Errorf("session: import zip: %w", err)
	}

	s.Config.Source = Source{
		Type:         SourceZip,
		OriginalPath: zipPath,
		ImportedPath: "source",
	}
	return s.Save()
}

// ImportTGZ extracts a .tar.gz / .tgz archive into the session's source/ directory.
func (s *Session) ImportTGZ(tgzPath string) error {
	destDir := filepath.Join(s.Dir, "source")
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("session: create source dir: %w", err)
	}

	if err := extractTGZ(tgzPath, destDir); err != nil {
		return fmt.Errorf("session: import tgz: %w", err)
	}

	s.Config.Source = Source{
		Type:         SourceTGZ,
		OriginalPath: tgzPath,
		ImportedPath: "source",
	}
	return s.Save()
}

// SetExternalSource points the session at an external folder without copying.
// This is the lightweight "link" mode — the original data stays in place.
func (s *Session) SetExternalSource(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("session: resolve path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("session: stat source: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("session: %q is not a directory", abs)
	}
	s.Config.Source = Source{
		Type:         SourceFolder,
		OriginalPath: abs,
	}
	return s.Save()
}

// ImportZipMulti extracts multiple .zip archives into a custom destination directory.
// If destDir is empty, it defaults to <session>/source.
// The original paths are stored as comma-separated values in OriginalPath.
func (s *Session) ImportZipMulti(zipPaths []string, destDir string) error {
	if destDir == "" {
		destDir = filepath.Join(s.Dir, "source")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("session: create dest dir: %w", err)
	}

	for _, zipPath := range zipPaths {
		if err := extractZip(zipPath, destDir); err != nil {
			return fmt.Errorf("session: import zip %s: %w", filepath.Base(zipPath), err)
		}
	}

	// Store all original paths
	var originalPaths []string
	for _, p := range zipPaths {
		originalPaths = append(originalPaths, p)
	}

	// Determine imported path relative to session dir if possible
	importedPath := destDir
	if rel, err := filepath.Rel(s.Dir, destDir); err == nil && !strings.HasPrefix(rel, "..") {
		importedPath = rel
	}

	s.Config.Source = Source{
		Type:         SourceZip,
		OriginalPath: strings.Join(originalPaths, ","),
		ImportedPath: importedPath,
	}
	return s.Save()
}

// ImportTGZMulti extracts multiple .tgz/.tar.gz archives into a custom destination directory.
// If destDir is empty, it defaults to <session>/source.
// The original paths are stored as comma-separated values in OriginalPath.
func (s *Session) ImportTGZMulti(tgzPaths []string, destDir string) error {
	if destDir == "" {
		destDir = filepath.Join(s.Dir, "source")
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("session: create dest dir: %w", err)
	}

	for _, tgzPath := range tgzPaths {
		if err := extractTGZ(tgzPath, destDir); err != nil {
			return fmt.Errorf("session: import tgz %s: %w", filepath.Base(tgzPath), err)
		}
	}

	// Store all original paths
	var originalPaths []string
	for _, p := range tgzPaths {
		originalPaths = append(originalPaths, p)
	}

	// Determine imported path relative to session dir if possible
	importedPath := destDir
	if rel, err := filepath.Rel(s.Dir, destDir); err == nil && !strings.HasPrefix(rel, "..") {
		importedPath = rel
	}

	s.Config.Source = Source{
		Type:         SourceTGZ,
		OriginalPath: strings.Join(originalPaths, ","),
		ImportedPath: importedPath,
	}
	return s.Save()
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func copyDirRecursive(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)

		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	info, err := sf.Stat()
	if err != nil {
		return err
	}

	df, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer df.Close()

	_, err = io.Copy(df, sf)
	return err
}

func extractZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		target := filepath.Join(destDir, f.Name)

		// Guard against zip-slip
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}

		rc, err := f.Open()
		if err != nil {
			return err
		}

		wf, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}

		_, copyErr := io.Copy(wf, rc)
		rc.Close()
		wf.Close()
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func extractTGZ(tgzPath, destDir string) error {
	f, err := os.Open(tgzPath)
	if err != nil {
		return fmt.Errorf("open tgz: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar next: %w", err)
		}

		target := filepath.Join(destDir, hdr.Name)

		// Guard against tar-slip
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			wf, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(wf, tr); err != nil {
				wf.Close()
				return err
			}
			wf.Close()
		}
	}
	return nil
}
