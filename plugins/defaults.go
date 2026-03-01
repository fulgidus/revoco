// Package plugins provides a dynamic plugin system for revoco.
//
// This file handles embedding and extraction of default plugins that are
// bundled with the revoco binary and auto-extracted on first run.
package plugins

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
)

// defaultPlugins embeds the default plugins shipped with revoco.
// These are extracted to the user's plugin directory on first run.
//
//go:embed defaults/connectors/*.lua defaults/processors/*.lua defaults/outputs/*.lua
var defaultPlugins embed.FS

// markerFileName is the file used to track whether defaults have been extracted.
const markerFileName = ".defaults-extracted"

// DefaultPluginsVersion is the version of the default plugins bundle.
// Increment this when default plugins are updated to trigger re-extraction.
const DefaultPluginsVersion = "1"

// ExtractDefaultPlugins extracts bundled default plugins to the user's plugin directory.
// It only extracts if this is the first run (marker file doesn't exist) or if
// the plugin version has changed.
func ExtractDefaultPlugins(destDir string) error {
	if destDir == "" {
		dirs := DefaultPluginDirs()
		if len(dirs) == 0 {
			return fmt.Errorf("could not determine plugin directory")
		}
		destDir = dirs[0]
	}

	// Check if extraction is needed
	if !needsExtraction(destDir) {
		return nil
	}

	log.Printf("[plugins] Extracting default plugins to %s", destDir)

	// Walk the embedded filesystem and extract files
	err := fs.WalkDir(defaultPlugins, "defaults", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root "defaults" directory itself
		if path == "defaults" {
			return nil
		}

		// Calculate destination path (strip "defaults/" prefix)
		relPath, err := filepath.Rel("defaults", path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}
		destPath := filepath.Join(destDir, relPath)

		if d.IsDir() {
			// Create directory
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			return nil
		}

		// Read embedded file
		content, err := defaultPlugins.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		// Check if file already exists and skip if it does (don't overwrite user modifications)
		if _, err := os.Stat(destPath); err == nil {
			log.Printf("[plugins] Skipping existing file: %s", relPath)
			return nil
		}

		// Write file
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}

		log.Printf("[plugins] Extracted: %s", relPath)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to extract default plugins: %w", err)
	}

	// Write marker file
	if err := writeMarkerFile(destDir); err != nil {
		log.Printf("[plugins] Warning: failed to write marker file: %v", err)
	}

	return nil
}

// needsExtraction checks if default plugins need to be extracted.
func needsExtraction(destDir string) bool {
	markerPath := filepath.Join(destDir, markerFileName)

	// Read existing marker
	content, err := os.ReadFile(markerPath)
	if os.IsNotExist(err) {
		// First run - needs extraction
		return true
	}
	if err != nil {
		// Error reading - try to extract anyway
		return true
	}

	// Check version
	if string(content) != DefaultPluginsVersion {
		// Version mismatch - needs extraction
		return true
	}

	return false
}

// writeMarkerFile writes the marker file indicating defaults have been extracted.
func writeMarkerFile(destDir string) error {
	markerPath := filepath.Join(destDir, markerFileName)

	// Ensure directory exists
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	return os.WriteFile(markerPath, []byte(DefaultPluginsVersion), 0644)
}

// ListDefaultPlugins returns information about the embedded default plugins.
func ListDefaultPlugins() ([]string, error) {
	var plugins []string

	err := fs.WalkDir(defaultPlugins, "defaults", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if !d.IsDir() && filepath.Ext(path) == ".lua" {
			relPath, _ := filepath.Rel("defaults", path)
			plugins = append(plugins, relPath)
		}

		return nil
	})

	return plugins, err
}

// GetDefaultPluginContent returns the content of an embedded default plugin.
func GetDefaultPluginContent(name string) ([]byte, error) {
	path := filepath.Join("defaults", name)
	return defaultPlugins.ReadFile(path)
}

// ResetDefaultPlugins removes the marker file, causing defaults to be
// re-extracted on next run. Existing plugins are NOT removed.
func ResetDefaultPlugins(destDir string) error {
	if destDir == "" {
		dirs := DefaultPluginDirs()
		if len(dirs) == 0 {
			return fmt.Errorf("could not determine plugin directory")
		}
		destDir = dirs[0]
	}

	markerPath := filepath.Join(destDir, markerFileName)
	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove marker file: %w", err)
	}

	return nil
}

// ForceExtractDefaultPlugins extracts default plugins, overwriting existing files.
// Use with caution - this will overwrite any user modifications to default plugins.
func ForceExtractDefaultPlugins(destDir string) error {
	if destDir == "" {
		dirs := DefaultPluginDirs()
		if len(dirs) == 0 {
			return fmt.Errorf("could not determine plugin directory")
		}
		destDir = dirs[0]
	}

	// Remove marker to force extraction
	if err := ResetDefaultPlugins(destDir); err != nil {
		return err
	}

	log.Printf("[plugins] Force extracting default plugins to %s", destDir)

	// Walk the embedded filesystem and extract files
	err := fs.WalkDir(defaultPlugins, "defaults", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root "defaults" directory itself
		if path == "defaults" {
			return nil
		}

		// Calculate destination path (strip "defaults/" prefix)
		relPath, err := filepath.Rel("defaults", path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}
		destPath := filepath.Join(destDir, relPath)

		if d.IsDir() {
			// Create directory
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			return nil
		}

		// Read embedded file
		content, err := defaultPlugins.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read embedded file %s: %w", path, err)
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %w", err)
		}

		// Write file (overwrite existing)
		if err := os.WriteFile(destPath, content, 0644); err != nil {
			return fmt.Errorf("failed to write file %s: %w", destPath, err)
		}

		log.Printf("[plugins] Extracted: %s", relPath)
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to extract default plugins: %w", err)
	}

	// Write marker file
	if err := writeMarkerFile(destDir); err != nil {
		log.Printf("[plugins] Warning: failed to write marker file: %v", err)
	}

	return nil
}
