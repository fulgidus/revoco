package engine

import (
	"bytes"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/internal/tools"
)

// ConvertMotionPhotos finds .MP and .COVER files in destMap and converts them
// to .mp4 using ffmpeg (stream copy, no re-encode).
func ConvertMotionPhotos(destMap map[string]string, dryRun bool, logger *log.Logger, progress func(done, total int)) (converted int, errs []error) {
	var targets []string
	for _, dest := range destMap {
		ext := strings.ToLower(filepath.Ext(dest))
		if ext == ".mp" || ext == ".cover" {
			targets = append(targets, dest)
		}
	}

	total := len(targets)
	if total == 0 {
		logger.Printf("[MotionPhoto] No motion photos found to convert")
		return
	}

	if dryRun {
		logger.Printf("[MotionPhoto] DRY-RUN mode - simulating conversion of %d files", total)
		for i, src := range targets {
			outPath := strings.TrimSuffix(src, filepath.Ext(src)) + ".mp4"
			logger.Printf("[MotionPhoto] [DRY-RUN] Would convert: %s -> %s", filepath.Base(src), filepath.Base(outPath))
			converted++
			if progress != nil {
				progress(i+1, total)
			}
		}
		return
	}

	logger.Printf("[MotionPhoto] Starting conversion of %d motion photos", total)
	for i, src := range targets {
		if progress != nil {
			progress(i, total)
		}

		outPath := strings.TrimSuffix(src, filepath.Ext(src)) + ".mp4"
		baseName := filepath.Base(src)

		ffmpegBin, ffErr := tools.FindTool("ffmpeg")
		if ffErr != nil {
			errs = append(errs, fmt.Errorf("ffmpeg not available: %w", ffErr))
			logger.Printf("[MotionPhoto] ERROR: ffmpeg not available: %v", ffErr)
			continue
		}

		cmd := exec.Command(ffmpegBin,
			"-y",
			"-i", src,
			"-map", "0:0",
			"-c:v", "copy",
			"-movflags", "+faststart",
			outPath,
		)

		// Capture stderr to route to logger instead of console
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf

		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Errorf("ffmpeg %s: %w", src, err))
			logger.Printf("[MotionPhoto] ERROR converting %s: %v", baseName, err)
			// Log ffmpeg stderr output for debugging
			if stderrBuf.Len() > 0 {
				logger.Printf("[MotionPhoto] ffmpeg output: %s", strings.TrimSpace(stderrBuf.String()))
			}
		} else {
			converted++
			logger.Printf("[MotionPhoto] Converted: %s -> %s", baseName, filepath.Base(outPath))
		}
	}

	if progress != nil {
		progress(total, total)
	}

	logger.Printf("[MotionPhoto] Complete: converted=%d, errors=%d", converted, len(errs))
	return
}
