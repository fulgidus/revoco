// Package ingesters provides data import modules for Google Tasks.
package ingesters

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/core"
	coreingesters "github.com/fulgidus/revoco/services/core/ingesters"
)

// NewTasksIngesters creates the standard set of ingesters for Tasks.
func NewTasksIngesters() []core.Ingester {
	return coreingesters.NewServiceIngesters("tasks", detectTasksDir)
}

// detectTasksDir checks if a path contains Tasks data.
func detectTasksDir(path string) bool {
	return hasTasksDir(path, []string{
		"Tasks",
		"Attività", // Italian
	})
}

// hasTasksDir checks for Tasks directory (case-insensitive).
func hasTasksDir(path string, variants []string) bool {
	baseName := filepath.Base(path)
	for _, variant := range variants {
		if strings.EqualFold(baseName, variant) {
			return true
		}
	}

	// Search subdirectories up to 3 levels deep
	var found bool
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(path, p)
		depth := len(strings.Split(rel, string(os.PathSeparator)))
		if depth > 3 {
			return filepath.SkipDir
		}
		for _, variant := range variants {
			if strings.EqualFold(d.Name(), variant) {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found
}
