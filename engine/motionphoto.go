package engine

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

// ConvertMotionPhotos finds .MP and .COVER files in destMap and converts them
// to .mp4 using ffmpeg (stream copy, no re-encode).
func ConvertMotionPhotos(destMap map[string]string, dryRun bool, progress func(done, total int)) (converted int, errs []error) {
	var targets []string
	for _, dest := range destMap {
		ext := strings.ToLower(filepath.Ext(dest))
		if ext == ".mp" || ext == ".cover" {
			targets = append(targets, dest)
		}
	}

	total := len(targets)
	for i, src := range targets {
		if progress != nil {
			progress(i, total)
		}

		outPath := strings.TrimSuffix(src, filepath.Ext(src)) + ".mp4"

		if dryRun {
			converted++
			continue
		}

		cmd := exec.Command("ffmpeg",
			"-y",
			"-i", src,
			"-map", "0:0",
			"-c:v", "copy",
			"-movflags", "+faststart",
			outPath,
		)
		if err := cmd.Run(); err != nil {
			errs = append(errs, fmt.Errorf("ffmpeg %s: %w", src, err))
		} else {
			converted++
		}
	}

	if progress != nil {
		progress(total, total)
	}
	return
}
