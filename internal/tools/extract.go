// Package tools provides runtime lookup and extraction of bundled binaries.
//
// When built with -tags with_bundled_tools, exiftool and ffmpeg blobs are
// embedded in the binary via internal/bundled. On first use, they are
// extracted to ~/.cache/revoco/bin/ (or %LOCALAPPDATA%\revoco\bin on Windows)
// and made executable.
//
// Without the build tag (dev builds), FindTool falls back to PATH only.
package tools

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/fulgidus/revoco/internal/bundled"
)

var (
	cacheDir     string
	cacheDirOnce sync.Once
	cacheDirErr  error

	exiftoolOnce sync.Once
	exiftoolPath string
	exiftoolErr  error

	ffmpegOnce sync.Once
	ffmpegPath string
	ffmpegErr  error
)

// FindTool returns the absolute path to the named tool ("exiftool" or "ffmpeg").
// It first tries the embedded binary (if bundled), then falls back to PATH.
func FindTool(name string) (string, error) {
	switch name {
	case "exiftool":
		exiftoolOnce.Do(func() {
			exiftoolPath, exiftoolErr = resolveTool("exiftool", bundled.ExifTool)
		})
		return exiftoolPath, exiftoolErr
	case "ffmpeg":
		ffmpegOnce.Do(func() {
			ffmpegPath, ffmpegErr = resolveTool("ffmpeg", bundled.FFmpeg)
		})
		return ffmpegPath, ffmpegErr
	default:
		return exec.LookPath(name)
	}
}

// resolveTool extracts blob to the cache dir (if non-nil) and returns its path.
// Falls back to PATH if no blob is available.
func resolveTool(name string, blob []byte) (string, error) {
	if len(blob) > 0 {
		path, err := extractTool(name, blob)
		if err == nil {
			return path, nil
		}
		// extraction failed — fall through to PATH
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s not found in PATH and no bundled binary available", name)
	}
	return path, nil
}

// extractTool writes blob to ~/.cache/revoco/bin/<name> and chmod 0755.
// Returns the absolute path.
func extractTool(name string, blob []byte) (string, error) {
	dir, err := getCacheDir()
	if err != nil {
		return "", err
	}

	binName := name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}

	dest := filepath.Join(dir, binName)

	// If already extracted and same size, skip write.
	if info, err := os.Stat(dest); err == nil && info.Size() == int64(len(blob)) {
		return dest, nil
	}

	if err := os.WriteFile(dest, blob, 0o755); err != nil {
		return "", fmt.Errorf("extract %s: %w", name, err)
	}
	return dest, nil
}

// getCacheDir returns (and creates) the per-user cache directory for revoco.
func getCacheDir() (string, error) {
	cacheDirOnce.Do(func() {
		var base string
		switch runtime.GOOS {
		case "windows":
			base = os.Getenv("LOCALAPPDATA")
			if base == "" {
				cacheDirErr = errors.New("LOCALAPPDATA not set")
				return
			}
		default:
			cache, err := os.UserCacheDir()
			if err != nil {
				cacheDirErr = fmt.Errorf("user cache dir: %w", err)
				return
			}
			base = cache
		}
		dir := filepath.Join(base, "revoco", "bin")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			cacheDirErr = fmt.Errorf("create cache dir %s: %w", dir, err)
			return
		}
		cacheDir = dir
	})
	return cacheDir, cacheDirErr
}
