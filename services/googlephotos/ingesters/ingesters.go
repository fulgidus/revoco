// Package ingesters provides data import modules for Google Photos Takeout.
package ingesters

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/core"
)

// ── Folder Ingester ──────────────────────────────────────────────────────────

// FolderIngester imports data from an extracted folder.
type FolderIngester struct{}

// NewFolder creates a new folder ingester.
func NewFolder() *FolderIngester {
	return &FolderIngester{}
}

func (f *FolderIngester) ID() string                    { return "google-photos-folder" }
func (f *FolderIngester) Name() string                  { return "Folder" }
func (f *FolderIngester) Description() string           { return "Import from an extracted Takeout folder" }
func (f *FolderIngester) SupportedExtensions() []string { return nil }

func (f *FolderIngester) CanIngest(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	if !info.IsDir() {
		return false
	}
	// Check if it contains a Google Photos folder
	return hasGooglePhotosDir(path)
}

func (f *FolderIngester) Ingest(ctx context.Context, sourcePath, destDir string, progress core.ProgressFunc) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	// Count total files first
	var total int
	filepath.WalkDir(sourcePath, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			total++
		}
		return nil
	})

	done := 0
	err := filepath.WalkDir(sourcePath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		rel, err := filepath.Rel(sourcePath, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		if err := copyFile(path, target); err != nil {
			return err
		}
		done++
		if progress != nil {
			progress(done, total)
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("copy folder: %w", err)
	}
	return destDir, nil
}

// ── ZIP Ingester ─────────────────────────────────────────────────────────────

// ZipIngester imports data from ZIP archives.
type ZipIngester struct{}

// NewZip creates a new ZIP ingester.
func NewZip() *ZipIngester {
	return &ZipIngester{}
}

func (z *ZipIngester) ID() string                    { return "google-photos-zip" }
func (z *ZipIngester) Name() string                  { return "ZIP Archive" }
func (z *ZipIngester) Description() string           { return "Import from .zip Takeout archives" }
func (z *ZipIngester) SupportedExtensions() []string { return []string{".zip"} }

func (z *ZipIngester) CanIngest(path string) bool {
	return strings.HasSuffix(strings.ToLower(path), ".zip")
}

func (z *ZipIngester) Ingest(ctx context.Context, sourcePath, destDir string, progress core.ProgressFunc) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	r, err := zip.OpenReader(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	total := len(r.File)
	for i, f := range r.File {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		target := filepath.Join(destDir, f.Name)

		// Guard against zip-slip
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return "", err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return "", err
		}

		if err := extractZipFile(f, target); err != nil {
			return "", err
		}

		if progress != nil {
			progress(i+1, total)
		}
	}
	return destDir, nil
}

// IngestMulti extracts multiple ZIP files to the same destination.
func (z *ZipIngester) IngestMulti(ctx context.Context, sourcePaths []string, destDir string, progress core.ProgressFunc) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	// Count total files across all archives
	var total int
	for _, p := range sourcePaths {
		r, err := zip.OpenReader(p)
		if err != nil {
			return "", fmt.Errorf("open zip %s: %w", filepath.Base(p), err)
		}
		total += len(r.File)
		r.Close()
	}

	done := 0
	for _, sourcePath := range sourcePaths {
		r, err := zip.OpenReader(sourcePath)
		if err != nil {
			return "", fmt.Errorf("open zip %s: %w", filepath.Base(sourcePath), err)
		}

		for _, f := range r.File {
			select {
			case <-ctx.Done():
				r.Close()
				return "", ctx.Err()
			default:
			}

			target := filepath.Join(destDir, f.Name)
			if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
				continue
			}

			if f.FileInfo().IsDir() {
				os.MkdirAll(target, f.Mode())
				continue
			}

			os.MkdirAll(filepath.Dir(target), 0o755)
			if err := extractZipFile(f, target); err != nil {
				r.Close()
				return "", err
			}

			done++
			if progress != nil {
				progress(done, total)
			}
		}
		r.Close()
	}
	return destDir, nil
}

// ── TGZ Ingester ─────────────────────────────────────────────────────────────

// TGZIngester imports data from tar.gz archives.
type TGZIngester struct{}

// NewTGZ creates a new TGZ ingester.
func NewTGZ() *TGZIngester {
	return &TGZIngester{}
}

func (t *TGZIngester) ID() string                    { return "google-photos-tgz" }
func (t *TGZIngester) Name() string                  { return "TGZ Archive" }
func (t *TGZIngester) Description() string           { return "Import from .tgz/.tar.gz Takeout archives" }
func (t *TGZIngester) SupportedExtensions() []string { return []string{".tgz", ".tar.gz"} }

func (t *TGZIngester) CanIngest(path string) bool {
	lower := strings.ToLower(path)
	return strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".tar.gz")
}

func (t *TGZIngester) Ingest(ctx context.Context, sourcePath, destDir string, progress core.ProgressFunc) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	f, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open tgz: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var done int
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("tar next: %w", err)
		}

		target := filepath.Join(destDir, hdr.Name)
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
			continue
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return "", err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return "", err
			}
			wf, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(wf, tr); err != nil {
				wf.Close()
				return "", err
			}
			wf.Close()
			done++
			if progress != nil {
				progress(done, done) // Total unknown for tar streams
			}
		}
	}
	return destDir, nil
}

// IngestMulti extracts multiple TGZ files to the same destination.
func (t *TGZIngester) IngestMulti(ctx context.Context, sourcePaths []string, destDir string, progress core.ProgressFunc) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create dest dir: %w", err)
	}

	var done int
	for _, sourcePath := range sourcePaths {
		f, err := os.Open(sourcePath)
		if err != nil {
			return "", fmt.Errorf("open tgz %s: %w", filepath.Base(sourcePath), err)
		}

		gz, err := gzip.NewReader(f)
		if err != nil {
			f.Close()
			return "", fmt.Errorf("gzip reader %s: %w", filepath.Base(sourcePath), err)
		}

		tr := tar.NewReader(gz)
		for {
			select {
			case <-ctx.Done():
				gz.Close()
				f.Close()
				return "", ctx.Err()
			default:
			}

			hdr, err := tr.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				gz.Close()
				f.Close()
				return "", fmt.Errorf("tar next: %w", err)
			}

			target := filepath.Join(destDir, hdr.Name)
			if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(destDir)+string(os.PathSeparator)) {
				continue
			}

			switch hdr.Typeflag {
			case tar.TypeDir:
				os.MkdirAll(target, os.FileMode(hdr.Mode))
			case tar.TypeReg:
				os.MkdirAll(filepath.Dir(target), 0o755)
				wf, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
				if err != nil {
					gz.Close()
					f.Close()
					return "", err
				}
				if _, err := io.Copy(wf, tr); err != nil {
					wf.Close()
					gz.Close()
					f.Close()
					return "", err
				}
				wf.Close()
				done++
				if progress != nil {
					progress(done, done)
				}
			}
		}
		gz.Close()
		f.Close()
	}
	return destDir, nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

var googlePhotosVariants = []string{
	"Google Foto",   // Italian
	"Google Photos", // English
	"Google Fotos",  // Spanish/Portuguese
}

func hasGooglePhotosDir(path string) bool {
	// Check if path itself is a Google Photos folder
	baseName := filepath.Base(path)
	for _, variant := range googlePhotosVariants {
		if strings.EqualFold(baseName, variant) {
			return true
		}
	}

	// Search for Google Photos subfolder (up to 3 levels)
	var found bool
	filepath.WalkDir(path, func(p string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(path, p)
		if len(strings.Split(rel, string(os.PathSeparator))) > 3 {
			return filepath.SkipDir
		}
		for _, variant := range googlePhotosVariants {
			if strings.EqualFold(d.Name(), variant) {
				found = true
				return filepath.SkipAll
			}
		}
		return nil
	})
	return found
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

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
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

func extractZipFile(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	wf, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer wf.Close()

	_, err = io.Copy(wf, rc)
	return err
}
