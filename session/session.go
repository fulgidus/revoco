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
