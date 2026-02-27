package engine

import (
	"io/fs"
	"path/filepath"
	"regexp"
)

// albumKind classifies a Takeout subfolder.
type albumKind int

const (
	albumKindNamed      albumKind = iota // becomes a subdirectory in dest
	albumKindChronologi                  // dissolved into root
	albumKindUnnamed                     // dissolved into root
)

var (
	chronoPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^Foto da \d{4}$`),
		regexp.MustCompile(`^Photos from \d{4}$`),
		regexp.MustCompile(`^Fotos de \d{4}$`),
	}
	unnamedPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^Senza nome`),
		regexp.MustCompile(`^Untitled`),
		regexp.MustCompile(`^Sin título`),
	}
)

func classifyFolder(name string) albumKind {
	for _, re := range chronoPatterns {
		if re.MatchString(name) {
			return albumKindChronologi
		}
	}
	for _, re := range unnamedPatterns {
		if re.MatchString(name) {
			return albumKindUnnamed
		}
	}
	return albumKindNamed
}

// AlbumsResult holds Phase 2 output: which album (if any) each media file belongs to.
type AlbumsResult struct {
	// MediaAlbum maps media path -> album name ("" = root / no album)
	MediaAlbum  map[string]string
	NamedAlbums []string
}

// AssignAlbums classifies Takeout subfolders and assigns album membership to each media file.
func AssignAlbums(gfotoPath string, mediaFiles map[string]string) (*AlbumsResult, error) {
	// Find all depth-1 subdirectories
	albumKinds := make(map[string]albumKind)
	err := filepath.WalkDir(gfotoPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() || path == gfotoPath {
			return nil
		}
		rel, _ := filepath.Rel(gfotoPath, path)
		// Only direct children
		if filepath.Dir(rel) != "." {
			return nil
		}
		albumKinds[d.Name()] = classifyFolder(d.Name())
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Collect named albums
	var namedAlbums []string
	for name, kind := range albumKinds {
		if kind == albumKindNamed {
			namedAlbums = append(namedAlbums, name)
		}
	}

	// Assign each media file to an album (or root)
	mediaAlbum := make(map[string]string, len(mediaFiles))
	for mediaPath := range mediaFiles {
		parent := filepath.Dir(mediaPath)
		folderName := filepath.Base(parent)
		kind, ok := albumKinds[folderName]
		if ok && kind == albumKindNamed {
			mediaAlbum[mediaPath] = folderName
		} else {
			mediaAlbum[mediaPath] = ""
		}
	}

	return &AlbumsResult{
		MediaAlbum:  mediaAlbum,
		NamedAlbums: namedAlbums,
	}, nil
}
