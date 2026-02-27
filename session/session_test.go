package session

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// override baseDir for tests so we don't touch ~/.revoco
func init() {
	// We'll use t.Setenv in each test to redirect HOME
}

// withTempHome sets HOME to a temp directory for the duration of the test.
func withTempHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	return dir
}

// ── Create / Load ────────────────────────────────────────────────────────────

func TestCreateAndLoad(t *testing.T) {
	withTempHome(t)

	s, err := Create("test-session")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if s.Config.Name != "test-session" {
		t.Errorf("name: got %q, want %q", s.Config.Name, "test-session")
	}
	if s.Config.Status != StatusIdle {
		t.Errorf("status: got %q, want %q", s.Config.Status, StatusIdle)
	}
	if s.Config.OutputDir != "output" {
		t.Errorf("output_dir: got %q, want %q", s.Config.OutputDir, "output")
	}

	// Verify directories were created
	for _, sub := range []string{"output", "recovered"} {
		p := filepath.Join(s.Dir, sub)
		info, err := os.Stat(p)
		if err != nil {
			t.Errorf("subdir %s not created: %v", sub, err)
		} else if !info.IsDir() {
			t.Errorf("%s is not a directory", sub)
		}
	}

	// Verify config.json exists and is valid
	data, err := os.ReadFile(s.ConfigPath())
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}
	if cfg.Name != "test-session" {
		t.Errorf("config name: got %q", cfg.Name)
	}

	// Load
	loaded, err := Load("test-session")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Config.Name != "test-session" {
		t.Errorf("loaded name: got %q", loaded.Config.Name)
	}
}

func TestCreateDuplicate(t *testing.T) {
	withTempHome(t)

	if _, err := Create("dup"); err != nil {
		t.Fatal(err)
	}
	_, err := Create("dup")
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
}

func TestCreateEmptyName(t *testing.T) {
	withTempHome(t)

	_, err := Create("")
	if err == nil {
		t.Fatal("expected error for empty name, got nil")
	}
}

func TestCreateInvalidChars(t *testing.T) {
	withTempHome(t)

	for _, name := range []string{"a/b", "a\\b", "a:b", "a*b", "a?b", `a"b`, "a<b", "a>b", "a|b"} {
		_, err := Create(name)
		if err == nil {
			t.Errorf("expected error for name %q, got nil", name)
		}
	}
}

func TestLoadNonexistent(t *testing.T) {
	withTempHome(t)

	_, err := Load("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent session, got nil")
	}
}

// ── List ─────────────────────────────────────────────────────────────────────

func TestList(t *testing.T) {
	withTempHome(t)

	// Empty list initially
	names, err := List()
	if err != nil {
		t.Fatalf("List empty: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(names))
	}

	// Create a few sessions
	for _, n := range []string{"charlie", "alice", "bob"} {
		if _, err := Create(n); err != nil {
			t.Fatal(err)
		}
	}

	names, err = List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 sessions, got %d", len(names))
	}
	// Should be sorted alphabetically
	expected := []string{"alice", "bob", "charlie"}
	for i, name := range names {
		if name != expected[i] {
			t.Errorf("names[%d]: got %q, want %q", i, name, expected[i])
		}
	}
}

func TestListSessions(t *testing.T) {
	withTempHome(t)

	if _, err := Create("s1"); err != nil {
		t.Fatal(err)
	}
	if _, err := Create("s2"); err != nil {
		t.Fatal(err)
	}

	sessions, err := ListSessions()
	if err != nil {
		t.Fatalf("ListSessions: %v", err)
	}
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

// ── Rename ───────────────────────────────────────────────────────────────────

func TestRename(t *testing.T) {
	withTempHome(t)

	if _, err := Create("old-name"); err != nil {
		t.Fatal(err)
	}

	if err := Rename("old-name", "new-name"); err != nil {
		t.Fatalf("Rename: %v", err)
	}

	// Old name should not exist
	_, err := Load("old-name")
	if err == nil {
		t.Error("old name should not be loadable after rename")
	}

	// New name should exist with updated config
	s, err := Load("new-name")
	if err != nil {
		t.Fatalf("Load new-name: %v", err)
	}
	if s.Config.Name != "new-name" {
		t.Errorf("config name: got %q, want %q", s.Config.Name, "new-name")
	}
}

func TestRenameToExisting(t *testing.T) {
	withTempHome(t)

	if _, err := Create("a"); err != nil {
		t.Fatal(err)
	}
	if _, err := Create("b"); err != nil {
		t.Fatal(err)
	}

	err := Rename("a", "b")
	if err == nil {
		t.Fatal("expected error when renaming to existing name")
	}
}

func TestRenameNonexistent(t *testing.T) {
	withTempHome(t)

	err := Rename("ghost", "new")
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}

func TestRenameEmptyName(t *testing.T) {
	withTempHome(t)

	if _, err := Create("src"); err != nil {
		t.Fatal(err)
	}
	err := Rename("src", "")
	if err == nil {
		t.Fatal("expected error for empty new name")
	}
}

func TestRenameInvalidChars(t *testing.T) {
	withTempHome(t)

	if _, err := Create("src"); err != nil {
		t.Fatal(err)
	}
	err := Rename("src", "a/b")
	if err == nil {
		t.Fatal("expected error for invalid chars in new name")
	}
}

// ── Remove ───────────────────────────────────────────────────────────────────

func TestRemove(t *testing.T) {
	withTempHome(t)

	if _, err := Create("doomed"); err != nil {
		t.Fatal(err)
	}

	if err := Remove("doomed"); err != nil {
		t.Fatalf("Remove: %v", err)
	}

	_, err := Load("doomed")
	if err == nil {
		t.Error("session should not be loadable after removal")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	withTempHome(t)

	err := Remove("ghost")
	if err == nil {
		t.Fatal("expected error for nonexistent session")
	}
}

// ── Session paths ────────────────────────────────────────────────────────────

func TestOutputPath(t *testing.T) {
	withTempHome(t)

	s, err := Create("paths-test")
	if err != nil {
		t.Fatal(err)
	}

	// Default relative output dir
	out := s.OutputPath()
	expected := filepath.Join(s.Dir, "output")
	if out != expected {
		t.Errorf("OutputPath: got %q, want %q", out, expected)
	}

	// Absolute output dir
	s.Config.OutputDir = "/tmp/custom-output"
	out = s.OutputPath()
	if out != "/tmp/custom-output" {
		t.Errorf("OutputPath absolute: got %q, want %q", out, "/tmp/custom-output")
	}
}

func TestSourcePath(t *testing.T) {
	withTempHome(t)

	s, err := Create("source-test")
	if err != nil {
		t.Fatal(err)
	}

	// No source set → empty
	if p := s.SourcePath(); p != "" {
		t.Errorf("expected empty SourcePath, got %q", p)
	}

	// Imported relative path
	s.Config.Source.ImportedPath = "source"
	expected := filepath.Join(s.Dir, "source")
	if p := s.SourcePath(); p != expected {
		t.Errorf("SourcePath relative: got %q, want %q", p, expected)
	}

	// External path (no import)
	s.Config.Source.ImportedPath = ""
	s.Config.Source.OriginalPath = "/mnt/photos/takeout"
	if p := s.SourcePath(); p != "/mnt/photos/takeout" {
		t.Errorf("SourcePath external: got %q", p)
	}
}

func TestLogPath(t *testing.T) {
	withTempHome(t)

	s, err := Create("log-test")
	if err != nil {
		t.Fatal(err)
	}

	lp := s.LogPath("process.log")
	expected := filepath.Join(s.Dir, "process.log")
	if lp != expected {
		t.Errorf("LogPath: got %q, want %q", lp, expected)
	}
}

// ── SetExternalSource ────────────────────────────────────────────────────────

func TestSetExternalSource(t *testing.T) {
	withTempHome(t)

	s, err := Create("ext-src")
	if err != nil {
		t.Fatal(err)
	}

	// Create a real directory to point at
	srcDir := t.TempDir()

	if err := s.SetExternalSource(srcDir); err != nil {
		t.Fatalf("SetExternalSource: %v", err)
	}

	if s.Config.Source.Type != SourceFolder {
		t.Errorf("type: got %q, want %q", s.Config.Source.Type, SourceFolder)
	}
	if s.Config.Source.OriginalPath != srcDir {
		t.Errorf("original_path: got %q", s.Config.Source.OriginalPath)
	}
	if s.Config.Source.ImportedPath != "" {
		t.Errorf("imported_path should be empty, got %q", s.Config.Source.ImportedPath)
	}
}

func TestSetExternalSourceNotDir(t *testing.T) {
	withTempHome(t)

	s, err := Create("ext-src-file")
	if err != nil {
		t.Fatal(err)
	}

	// Point at a file, not a dir
	f := filepath.Join(t.TempDir(), "file.txt")
	if err := os.WriteFile(f, []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = s.SetExternalSource(f)
	if err == nil {
		t.Fatal("expected error when pointing at a file")
	}
}

// ── Import folder ────────────────────────────────────────────────────────────

func TestImportFolder(t *testing.T) {
	withTempHome(t)

	s, err := Create("import-folder")
	if err != nil {
		t.Fatal(err)
	}

	// Create source with some files
	srcDir := t.TempDir()
	os.MkdirAll(filepath.Join(srcDir, "subdir"), 0o755)
	os.WriteFile(filepath.Join(srcDir, "photo.jpg"), []byte("jpeg-data"), 0o644)
	os.WriteFile(filepath.Join(srcDir, "subdir", "meta.json"), []byte("{}"), 0o644)

	if err := s.ImportFolder(srcDir); err != nil {
		t.Fatalf("ImportFolder: %v", err)
	}

	// Verify files were copied
	imported := filepath.Join(s.Dir, "source")
	if _, err := os.Stat(filepath.Join(imported, "photo.jpg")); err != nil {
		t.Error("photo.jpg not imported")
	}
	if _, err := os.Stat(filepath.Join(imported, "subdir", "meta.json")); err != nil {
		t.Error("subdir/meta.json not imported")
	}

	if s.Config.Source.Type != SourceFolder {
		t.Errorf("type: got %q", s.Config.Source.Type)
	}
	if s.Config.Source.ImportedPath != "source" {
		t.Errorf("imported_path: got %q", s.Config.Source.ImportedPath)
	}
}

// ── Import zip ───────────────────────────────────────────────────────────────

func TestImportZip(t *testing.T) {
	withTempHome(t)

	s, err := Create("import-zip")
	if err != nil {
		t.Fatal(err)
	}

	// Create a test zip
	zipPath := filepath.Join(t.TempDir(), "test.zip")
	createTestZip(t, zipPath, map[string]string{
		"photo.jpg":        "jpeg-data",
		"subdir/meta.json": "{}",
	})

	if err := s.ImportZip(zipPath); err != nil {
		t.Fatalf("ImportZip: %v", err)
	}

	imported := filepath.Join(s.Dir, "source")
	if _, err := os.Stat(filepath.Join(imported, "photo.jpg")); err != nil {
		t.Error("photo.jpg not extracted from zip")
	}
	if _, err := os.Stat(filepath.Join(imported, "subdir", "meta.json")); err != nil {
		t.Error("subdir/meta.json not extracted from zip")
	}

	if s.Config.Source.Type != SourceZip {
		t.Errorf("type: got %q", s.Config.Source.Type)
	}
}

// ── Import tgz ───────────────────────────────────────────────────────────────

func TestImportTGZ(t *testing.T) {
	withTempHome(t)

	s, err := Create("import-tgz")
	if err != nil {
		t.Fatal(err)
	}

	// Create a test .tgz
	tgzPath := filepath.Join(t.TempDir(), "test.tgz")
	createTestTGZ(t, tgzPath, map[string]string{
		"photo.jpg":        "jpeg-data",
		"subdir/meta.json": "{}",
	})

	if err := s.ImportTGZ(tgzPath); err != nil {
		t.Fatalf("ImportTGZ: %v", err)
	}

	imported := filepath.Join(s.Dir, "source")
	if _, err := os.Stat(filepath.Join(imported, "photo.jpg")); err != nil {
		t.Error("photo.jpg not extracted from tgz")
	}
	if _, err := os.Stat(filepath.Join(imported, "subdir", "meta.json")); err != nil {
		t.Error("subdir/meta.json not extracted from tgz")
	}

	if s.Config.Source.Type != SourceTGZ {
		t.Errorf("type: got %q", s.Config.Source.Type)
	}
}

// ── Save / reload round-trip ─────────────────────────────────────────────────

func TestSaveReload(t *testing.T) {
	withTempHome(t)

	s, err := Create("save-test")
	if err != nil {
		t.Fatal(err)
	}

	s.Config.Status = StatusProcessing
	s.Config.LastPhaseCompleted = 5
	s.Config.UseMove = true
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	reloaded, err := Load("save-test")
	if err != nil {
		t.Fatalf("Load after save: %v", err)
	}

	if reloaded.Config.Status != StatusProcessing {
		t.Errorf("status: got %q", reloaded.Config.Status)
	}
	if reloaded.Config.LastPhaseCompleted != 5 {
		t.Errorf("phase: got %d", reloaded.Config.LastPhaseCompleted)
	}
	if !reloaded.Config.UseMove {
		t.Error("use_move should be true")
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func createTestZip(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for name, content := range files {
		fw, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func createTestTGZ(t *testing.T, path string, files map[string]string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)

	// Collect and sort directories to create them first
	dirs := map[string]bool{}
	for name := range files {
		dir := filepath.Dir(name)
		if dir != "." {
			dirs[dir] = true
		}
	}
	for dir := range dirs {
		hdr := &tar.Header{
			Name:     dir + "/",
			Typeflag: tar.TypeDir,
			Mode:     0o755,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
	}

	for name, content := range files {
		hdr := &tar.Header{
			Name:     name,
			Size:     int64(len(content)),
			Mode:     0o644,
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
}
