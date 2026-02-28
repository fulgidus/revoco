// Package engine contains the core processing logic for revoco.
// This file implements Phase 1: file discovery and JSON-to-media matching.
package engine

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// skipJSONs are album-level metadata files that are not per-photo JSON.
var skipJSONs = map[string]bool{
	"metadati.json":                          true,
	"metadata.json":                          true,
	"commenti_album_condivisi.json":          true,
	"titoli-memoria-generati-da-utente.json": true,
	"user-generated-memory-titles.json":      true,
	"shared-album-comments.json":             true,
}

// supplementalSuffixes are the truncated variants Google produces for
// .supplemental-metadata.json filenames (longest first for greedy matching).
var supplementalSuffixes = []string{
	".supplemental-metadata",
	".supplemental-metadat",
	".supplemental-metada",
	".supplemental-metad",
	".supplemental-meta",
	".supplemental-met",
	".supplemental-me",
	".supplemental-m",
	".supplemental-",
	".supplemental",
	".supplementa",
	".supplement",
	".supplemen",
	".suppleme",
	".supplem",
	".supple",
	".suppl",
	".supp",
	".sup",
	".su",
	".s",
}

// IndexResult holds the output of Phase 1.
type IndexResult struct {
	// MediaFiles maps media path -> its matched JSON path (empty if unmatched)
	MediaFiles map[string]string
	// OrphanJSONs are JSON files with no matching media
	OrphanJSONs []OrphanJSON
	// Stats
	TotalMedia   int
	TotalJSON    int
	TotalMatched int
	TotalOrphans int
}

// OrphanJSON is a JSON metadata file with no corresponding media file.
type OrphanJSON struct {
	Path         string
	Title        string
	URL          string
	PhotoTakenTS int64
	CreationTS   int64
	SourceFolder string
}

// IndexFiles walks the Google Photos subdirectory, classifies all files, and
// matches JSON metadata to their corresponding media files.
// The context allows cancellation during the matching phase.
func IndexFiles(ctx context.Context, gfotoPath string, progress func(done, total int)) (*IndexResult, error) {
	// Step 1: Collect all files, classify into media and JSON lists
	var mediaFiles []string
	var jsonFiles []string

	err := filepath.WalkDir(gfotoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		// Check for cancellation periodically during walk
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			return nil
		}
		base := d.Name()
		lower := strings.ToLower(base)

		if strings.HasSuffix(lower, ".json") {
			if !skipJSONs[strings.ToLower(base)] {
				jsonFiles = append(jsonFiles, path)
			}
		} else {
			mediaFiles = append(mediaFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Step 2: Build a basename -> []path index for media files
	mediaByName := make(map[string][]string, len(mediaFiles))
	for _, p := range mediaFiles {
		base := filepath.Base(p)
		mediaByName[base] = append(mediaByName[base], p)
	}

	result := &IndexResult{
		MediaFiles: make(map[string]string, len(mediaFiles)),
		TotalMedia: len(mediaFiles),
		TotalJSON:  len(jsonFiles),
	}
	// Pre-populate all media as unmatched
	for _, p := range mediaFiles {
		result.MediaFiles[p] = ""
	}

	total := len(jsonFiles)
	// Step 3: Match each JSON to its media file
	for i, jsonPath := range jsonFiles {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if progress != nil {
			progress(i, total)
		}

		title, url, photoTS, creationTS := extractJSONMeta(jsonPath)
		dir := filepath.Dir(jsonPath)
		base := filepath.Base(jsonPath)

		matched := false

		// Strategy 1: same directory + title field
		if title != "" {
			candidate := filepath.Join(dir, title)
			if _, ok := result.MediaFiles[candidate]; ok {
				result.MediaFiles[candidate] = jsonPath
				matched = true
			}
		}

		// Strategy 2: same directory + strip supplemental suffix from JSON name
		if !matched {
			stem := strings.TrimSuffix(base, ".json")
			for _, suf := range supplementalSuffixes {
				if strings.HasSuffix(stem, suf) {
					stripped := strings.TrimSuffix(stem, suf)
					candidate := filepath.Join(dir, stripped)
					if _, ok := result.MediaFiles[candidate]; ok {
						result.MediaFiles[candidate] = jsonPath
						matched = true
						break
					}
				}
			}
		}

		// Strategy 2b: same directory + JSON named "mediafile.ext.json" → "mediafile.ext"
		// This handles the common Google Takeout pattern where JSON is simply filename + .json
		if !matched {
			stem := strings.TrimSuffix(base, ".json") // e.g., "IMG_123.jpg.json" → "IMG_123.jpg"
			candidate := filepath.Join(dir, stem)
			if _, ok := result.MediaFiles[candidate]; ok {
				result.MediaFiles[candidate] = jsonPath
				matched = true
			}
		}

		// Strategy 3: global basename search by title
		if !matched && title != "" {
			if candidates, ok := mediaByName[title]; ok && len(candidates) == 1 {
				result.MediaFiles[candidates[0]] = jsonPath
				matched = true
			}
		}

		if !matched {
			result.OrphanJSONs = append(result.OrphanJSONs, OrphanJSON{
				Path:         jsonPath,
				Title:        title,
				URL:          url,
				PhotoTakenTS: photoTS,
				CreationTS:   creationTS,
				SourceFolder: filepath.Base(dir),
			})
			result.TotalOrphans++
		} else {
			result.TotalMatched++
		}
	}

	if progress != nil {
		progress(total, total)
	}

	return result, nil
}

// extractJSONMeta reads title, url, and timestamps from a JSON metadata file.
func extractJSONMeta(path string) (title, url string, photoTS, creationTS int64) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var v struct {
		Title          string `json:"title"`
		URL            string `json:"url"`
		PhotoTakenTime struct {
			Timestamp string `json:"timestamp"`
		} `json:"photoTakenTime"`
		CreationTime struct {
			Timestamp string `json:"timestamp"`
		} `json:"creationTime"`
	}
	if err := json.Unmarshal(data, &v); err != nil {
		return
	}
	title = v.Title
	url = v.URL
	if ts, err := strconv.ParseInt(v.PhotoTakenTime.Timestamp, 10, 64); err == nil {
		photoTS = ts
	}
	if ts, err := strconv.ParseInt(v.CreationTime.Timestamp, 10, 64); err == nil {
		creationTS = ts
	}
	return
}
