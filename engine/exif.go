package engine

import (
	"bufio"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/fulgidus/revoco/filedate"
	"github.com/fulgidus/revoco/internal/tools"
	"github.com/fulgidus/revoco/metadata"
)

// ExifTool manages a persistent exiftool -stay_open process to avoid
// per-file process spawn overhead.
type ExifTool struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
}

// NewExifTool starts a persistent exiftool process.
func NewExifTool() (*ExifTool, error) {
	exiftoolBin, err := tools.FindTool("exiftool")
	if err != nil {
		return nil, fmt.Errorf("exiftool not available: %w", err)
	}
	cmd := exec.Command(exiftoolBin, "-stay_open", "true", "-@", "-")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("exiftool stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("exiftool stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start exiftool: %w", err)
	}
	return &ExifTool{
		cmd:    cmd,
		stdin:  stdin,
		stdout: bufio.NewScanner(stdout),
	}, nil
}

// Close shuts down the persistent exiftool process.
func (e *ExifTool) Close() error {
	fmt.Fprintln(e.stdin, "-stay_open")
	fmt.Fprintln(e.stdin, "false")
	fmt.Fprintln(e.stdin, "-execute")
	e.stdin.Close()
	return e.cmd.Wait()
}

// Execute sends a batch of arguments to exiftool and waits for completion.
// Arguments are one per line; exiftool reads until it sees "-execute".
func (e *ExifTool) Execute(args []string) error {
	for _, arg := range args {
		fmt.Fprintln(e.stdin, arg)
	}
	fmt.Fprintln(e.stdin, "-execute")

	// Read output until "{ready}" sentinel
	for e.stdout.Scan() {
		line := e.stdout.Text()
		if strings.HasPrefix(line, "{ready}") {
			return nil
		}
	}
	if err := e.stdout.Err(); err != nil {
		return fmt.Errorf("exiftool output: %w", err)
	}
	return nil
}

// ApplyEXIFFromJSON applies metadata from a Takeout JSON file to its media file.
func (e *ExifTool) ApplyEXIFFromJSON(mediaPath, jsonPath string) error {
	photo, err := metadata.ParseFile(jsonPath)
	if err != nil {
		return fmt.Errorf("parse json %s: %w", jsonPath, err)
	}

	ts := photo.PhotoTakenTime.Unix()
	if ts == 0 {
		ts = photo.CreationTime.Unix()
	}
	if ts == 0 {
		return nil // no timestamp at all — skip
	}

	exifDate := time.Unix(ts, 0).Local().Format("2006:01:02 15:04:05")
	isVideo := isVideoExt(filepath.Ext(mediaPath))

	args := []string{
		"-overwrite_original",
		"-DateTimeOriginal=" + exifDate,
		"-CreateDate=" + exifDate,
		"-ModifyDate=" + exifDate,
	}

	if isVideo {
		args = append(args,
			"-MediaCreateDate="+exifDate,
			"-MediaModifyDate="+exifDate,
			"-TrackCreateDate="+exifDate,
			"-TrackModifyDate="+exifDate,
		)
	}

	// GPS
	geo := photo.BestGeo()
	if !geo.IsZero() {
		latRef := "N"
		lonRef := "E"
		lat := geo.Latitude
		lon := geo.Longitude
		if lat < 0 {
			latRef = "S"
			lat = math.Abs(lat)
		}
		if lon < 0 {
			lonRef = "W"
			lon = math.Abs(lon)
		}
		args = append(args,
			fmt.Sprintf("-GPSLatitude=%.8f", lat),
			"-GPSLatitudeRef="+latRef,
			fmt.Sprintf("-GPSLongitude=%.8f", lon),
			"-GPSLongitudeRef="+lonRef,
			fmt.Sprintf("-GPSAltitude=%.2f", geo.Altitude),
			"-GPSAltitudeRef=0",
		)
	}

	// Description
	if photo.Description != "" {
		args = append(args, "-ImageDescription="+photo.Description)
	}

	// People as keywords
	for _, kw := range photo.Keywords() {
		args = append(args, "-Keywords+="+kw, "-Subject+="+kw)
	}

	args = append(args, mediaPath)

	if err := e.Execute(args); err != nil {
		return fmt.Errorf("exiftool apply %s: %w", mediaPath, err)
	}

	// Set filesystem mtime
	os.Chtimes(mediaPath, time.Unix(ts, 0), time.Unix(ts, 0)) //nolint:errcheck
	return nil
}

// ApplyDateFromFilename extracts a date from the filename and applies it via exiftool.
func (e *ExifTool) ApplyDateFromFilename(mediaPath string) (bool, error) {
	base := filepath.Base(mediaPath)
	t, _ := filedate.Extract(base)
	if t.IsZero() {
		return false, nil
	}

	exifDate := t.Format("2006:01:02 15:04:05")
	isVideo := isVideoExt(filepath.Ext(mediaPath))

	args := []string{
		"-overwrite_original",
		"-DateTimeOriginal=" + exifDate,
		"-CreateDate=" + exifDate,
		"-ModifyDate=" + exifDate,
	}
	if isVideo {
		args = append(args,
			"-MediaCreateDate="+exifDate,
			"-MediaModifyDate="+exifDate,
			"-TrackCreateDate="+exifDate,
			"-TrackModifyDate="+exifDate,
		)
	}
	args = append(args, mediaPath)

	if err := e.Execute(args); err != nil {
		return false, fmt.Errorf("exiftool filename-date %s: %w", mediaPath, err)
	}

	os.Chtimes(mediaPath, t, t) //nolint:errcheck
	return true, nil
}

var videoExts = map[string]bool{
	".mp4": true, ".mov": true, ".avi": true, ".3gp": true,
	".mkv": true, ".wmv": true, ".flv": true, ".webm": true,
	".m4v": true, ".mpg": true, ".mpeg": true,
}

func isVideoExt(ext string) bool {
	return videoExts[strings.ToLower(ext)]
}

// ApplyEXIFBatch applies metadata to a batch of files using the persistent exiftool process.
// For files with JSON: uses JSON metadata. For files without JSON: falls back to filename date.
func ApplyEXIFBatch(
	destMap map[string]string, // src -> dest
	mediaJSON map[string]string, // src -> json path
	dryRun bool,
	progress func(done, total int),
) (applied, dateFallback, errCount int, errs []error) {
	if dryRun {
		for range destMap {
			applied++
		}
		return
	}

	et, err := NewExifTool()
	if err != nil {
		// exiftool not available — skip silently
		return 0, 0, len(destMap), []error{fmt.Errorf("exiftool not available: %w", err)}
	}
	defer et.Close()

	total := len(destMap)
	done := 0

	// Build src list for deterministic ordering
	srcs := make([]string, 0, total)
	for src := range destMap {
		srcs = append(srcs, src)
	}

	for _, src := range srcs {
		dest := destMap[src]
		jsonPath := mediaJSON[src]

		var applyErr error
		if jsonPath != "" {
			applyErr = et.ApplyEXIFFromJSON(dest, jsonPath)
			if applyErr == nil {
				applied++
			}
		} else {
			ok, applyErr2 := et.ApplyDateFromFilename(dest)
			if applyErr2 == nil && ok {
				dateFallback++
			}
			applyErr = applyErr2
		}

		if applyErr != nil {
			errCount++
			errs = append(errs, applyErr)
		}

		done++
		if progress != nil {
			progress(done, total)
		}
	}

	return
}
