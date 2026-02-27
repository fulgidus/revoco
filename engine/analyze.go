package engine

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// AnalysisResult is returned by Analyze and shown on the pre-flight screen.
type AnalysisResult struct {
	TotalMedia   int
	TotalJSON    int
	MatchRate    float64 // 0.0–1.0
	TotalBytes   int64
	Albums       []string
	EarliestDate time.Time
	LatestDate   time.Time
	MotionPhotos int
	Videos       int
}

// Analyze performs a quick (read-only) scan of a Takeout source directory and
// returns statistics for the pre-flight screen. It accepts an optional progress
// callback that is called with (done, total) file counts as the walk proceeds.
func Analyze(sourceDir string, progress func(done, total int)) (*AnalysisResult, error) {
	gfotoPath, err := detectGooglePhotosDir(sourceDir)
	if err != nil {
		return nil, err
	}

	// ── First pass: collect all entries ──────────────────────────────────────
	type entry struct {
		path    string
		size    int64
		isJSON  bool
		isVideo bool
		isMP    bool
		ts      int64 // unix seconds from JSON, 0 if unknown
		dir     string
	}

	var entries []entry
	albumSet := make(map[string]struct{})

	err = filepath.WalkDir(gfotoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}

		base := d.Name()
		lower := strings.ToLower(base)
		ext := strings.ToLower(filepath.Ext(lower))
		dir := filepath.Base(filepath.Dir(path))

		e := entry{
			path: path,
			size: info.Size(),
			dir:  dir,
		}

		switch {
		case strings.HasSuffix(lower, ".json"):
			if skipJSONs[strings.ToLower(base)] {
				return nil
			}
			e.isJSON = true
			_, _, photoTS, creationTS := extractJSONMeta(path)
			if photoTS > 0 {
				e.ts = photoTS
			} else {
				e.ts = creationTS
			}
		case ext == ".mp" || ext == ".cover":
			e.isMP = true
		case isVideoExt(ext):
			e.isVideo = true
		}

		entries = append(entries, e)
		return nil
	})
	if err != nil {
		return nil, err
	}

	// ── Second pass: compute stats ────────────────────────────────────────────
	total := len(entries)
	res := &AnalysisResult{}
	var earliest, latest int64

	for i, e := range entries {
		if progress != nil {
			progress(i, total)
		}

		if e.isJSON {
			res.TotalJSON++
			if e.ts > 0 {
				if earliest == 0 || e.ts < earliest {
					earliest = e.ts
				}
				if e.ts > latest {
					latest = e.ts
				}
			}
			continue
		}

		res.TotalMedia++
		res.TotalBytes += e.size

		if e.isMP {
			res.MotionPhotos++
		} else if e.isVideo {
			res.Videos++
		}

		// Count albums: any directory that is not the Photos from <year> pattern
		dir := e.dir
		if dir != "" && !isYearFolder(dir) {
			albumSet[dir] = struct{}{}
		}
	}

	if progress != nil {
		progress(total, total)
	}

	// Albums list
	res.Albums = make([]string, 0, len(albumSet))
	for name := range albumSet {
		res.Albums = append(res.Albums, name)
	}

	// Match rate: approximate as TotalJSON / TotalMedia (capped at 1)
	if res.TotalMedia > 0 && res.TotalJSON > 0 {
		m := float64(res.TotalJSON) / float64(res.TotalMedia)
		if m > 1.0 {
			m = 1.0
		}
		res.MatchRate = m
	}

	// Dates
	if earliest > 0 {
		res.EarliestDate = time.Unix(earliest, 0).UTC()
	}
	if latest > 0 {
		res.LatestDate = time.Unix(latest, 0).UTC()
	}

	return res, nil
}

// isYearFolder returns true for "Photos from YYYY"-style folder names that
// are not real albums.
func isYearFolder(name string) bool {
	lower := strings.ToLower(name)
	// Italian "Foto del YYYY", English "Photos from YYYY", Spanish "Fotos de YYYY"
	if strings.HasPrefix(lower, "photos from ") ||
		strings.HasPrefix(lower, "foto del ") ||
		strings.HasPrefix(lower, "fotos de ") ||
		strings.HasPrefix(lower, "photos de ") {
		return true
	}
	// Also bare 4-digit year folders like "2023"
	if len(name) == 4 {
		allDigits := true
		for _, c := range name {
			if c < '0' || c > '9' {
				allDigits = false
				break
			}
		}
		return allDigits
	}
	// Check if path stat is dir — the caller already filtered to non-dirs
	_ = os.Stat // keep import used if needed
	return false
}
