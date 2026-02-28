// Package outputs provides output modules specific to YouTube Music.
package outputs

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/youtubemusic/metadata"
)

// ── JSON Output ──────────────────────────────────────────────────────────────

// JSONOutput exports YouTube Music data to JSON files.
type JSONOutput struct {
	destDir string
	pretty  bool
}

// NewJSON creates a new JSON output.
func NewJSON() *JSONOutput {
	return &JSONOutput{pretty: true}
}

func (o *JSONOutput) ID() string          { return "ytmusic-json" }
func (o *JSONOutput) Name() string        { return "JSON Export" }
func (o *JSONOutput) Description() string { return "Export playlists and library data to JSON files" }

func (o *JSONOutput) SupportedItemTypes() []string {
	return []string{"playlist", "document"}
}

func (o *JSONOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for JSON files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "pretty",
			Name:        "Pretty Print",
			Description: "Format JSON with indentation",
			Type:        "bool",
			Default:     true,
		},
	}
}

func (o *JSONOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["pretty"].(bool); ok {
		o.pretty = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *JSONOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	destPath := filepath.Join(o.destDir, item.DestRelPath)
	os.MkdirAll(filepath.Dir(destPath), 0o755)

	// Ensure it has .json extension
	if !strings.HasSuffix(destPath, ".json") {
		destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".json"
	}

	var data []byte
	var err error
	if o.pretty {
		data, err = json.MarshalIndent(item.Metadata, "", "  ")
	} else {
		data, err = json.Marshal(item.Metadata)
	}
	if err != nil {
		return err
	}

	return os.WriteFile(destPath, data, 0o644)
}

func (o *JSONOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *JSONOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── CSV Output ───────────────────────────────────────────────────────────────

// CSVOutput exports YouTube Music data to CSV files.
type CSVOutput struct {
	destDir string
}

// NewCSV creates a new CSV output.
func NewCSV() *CSVOutput {
	return &CSVOutput{}
}

func (o *CSVOutput) ID() string          { return "ytmusic-csv" }
func (o *CSVOutput) Name() string        { return "CSV Export" }
func (o *CSVOutput) Description() string { return "Export playlists to CSV files" }

func (o *CSVOutput) SupportedItemTypes() []string {
	return []string{"playlist"}
}

func (o *CSVOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for CSV files",
			Type:        "string",
			Required:    true,
		},
	}
}

func (o *CSVOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}
	return os.MkdirAll(o.destDir, 0o755)
}

func (o *CSVOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "playlist" {
		return nil
	}

	destPath := filepath.Join(o.destDir, item.DestRelPath)
	destPath = strings.TrimSuffix(destPath, filepath.Ext(destPath)) + ".csv"
	os.MkdirAll(filepath.Dir(destPath), 0o755)

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	defer w.Flush()

	// Write header
	w.Write([]string{"Position", "Video ID", "Title", "Artist", "Album", "Duration", "Added At"})

	// Extract playlist data
	if pl, ok := item.Metadata["playlist"].(metadata.Playlist); ok {
		for _, track := range pl.Tracks {
			w.Write([]string{
				fmt.Sprintf("%d", track.PlaylistPos),
				track.VideoID,
				track.Title,
				track.Artist,
				track.Album,
				fmt.Sprintf("%d", track.Duration),
				track.AddedAt.Format("2006-01-02 15:04:05"),
			})
		}
	} else if songs, ok := item.Metadata["songs"].([]metadata.LikedSong); ok {
		for i, song := range songs {
			w.Write([]string{
				fmt.Sprintf("%d", i+1),
				song.VideoID,
				song.Title,
				song.Artist,
				song.Album,
				fmt.Sprintf("%d", song.Duration),
				song.LikedAt.Format("2006-01-02 15:04:05"),
			})
		}
	}

	return nil
}

func (o *CSVOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *CSVOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── M3U Output ───────────────────────────────────────────────────────────────

// M3UOutput exports playlists to M3U format.
type M3UOutput struct {
	destDir       string
	useLocalPaths bool
	localLibrary  string
}

// NewM3U creates a new M3U output.
func NewM3U() *M3UOutput {
	return &M3UOutput{}
}

func (o *M3UOutput) ID() string          { return "ytmusic-m3u" }
func (o *M3UOutput) Name() string        { return "M3U Playlists" }
func (o *M3UOutput) Description() string { return "Export playlists to M3U format for media players" }

func (o *M3UOutput) SupportedItemTypes() []string {
	return []string{"playlist"}
}

func (o *M3UOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for M3U files",
			Type:        "string",
			Required:    true,
		},
		{
			ID:          "use_local_paths",
			Name:        "Use Local Paths",
			Description: "Use matched local file paths instead of YouTube URLs",
			Type:        "bool",
			Default:     false,
		},
		{
			ID:          "local_library",
			Name:        "Local Library",
			Description: "Path to local music library for path resolution",
			Type:        "string",
		},
	}
}

func (o *M3UOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}

	if v, ok := cfg.Settings["use_local_paths"].(bool); ok {
		o.useLocalPaths = v
	}
	if v, ok := cfg.Settings["local_library"].(string); ok {
		o.localLibrary = v
	}

	return os.MkdirAll(o.destDir, 0o755)
}

func (o *M3UOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "playlist" {
		return nil
	}

	title, _ := item.Metadata["title"].(string)
	if title == "" {
		title = "playlist"
	}

	destPath := filepath.Join(o.destDir, sanitizeFilename(title)+".m3u")
	os.MkdirAll(filepath.Dir(destPath), 0o755)

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString("#EXTM3U\n")
	f.WriteString(fmt.Sprintf("#PLAYLIST:%s\n\n", title))

	if pl, ok := item.Metadata["playlist"].(metadata.Playlist); ok {
		for _, track := range pl.Tracks {
			o.writeTrack(f, track.VideoID, track.Title, track.Artist, track.Duration, track.Album)
		}
	} else if songs, ok := item.Metadata["songs"].([]metadata.LikedSong); ok {
		for _, song := range songs {
			o.writeTrack(f, song.VideoID, song.Title, song.Artist, song.Duration, song.Album)
		}
	}

	return nil
}

func (o *M3UOutput) writeTrack(f *os.File, videoID, title, artist string, duration int, localPath string) {
	displayTitle := title
	if artist != "" {
		displayTitle = artist + " - " + title
	}

	dur := duration
	if dur == 0 {
		dur = -1
	}

	f.WriteString(fmt.Sprintf("#EXTINF:%d,%s\n", dur, displayTitle))

	// Use local path if available and enabled
	if o.useLocalPaths && localPath != "" && strings.HasPrefix(localPath, "/") {
		f.WriteString(localPath + "\n")
	} else {
		f.WriteString(fmt.Sprintf("https://music.youtube.com/watch?v=%s\n", videoID))
	}
}

func (o *M3UOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *M3UOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── Spotify Import Output ────────────────────────────────────────────────────

// SpotifyOutput generates files formatted for Spotify playlist import tools.
type SpotifyOutput struct {
	destDir string
}

// NewSpotify creates a new Spotify import output.
func NewSpotify() *SpotifyOutput {
	return &SpotifyOutput{}
}

func (o *SpotifyOutput) ID() string   { return "ytmusic-spotify" }
func (o *SpotifyOutput) Name() string { return "Spotify Import" }
func (o *SpotifyOutput) Description() string {
	return "Export playlists in format suitable for Spotify import tools"
}

func (o *SpotifyOutput) SupportedItemTypes() []string {
	return []string{"playlist"}
}

func (o *SpotifyOutput) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "dest_dir",
			Name:        "Destination",
			Description: "Output directory for Spotify import files",
			Type:        "string",
			Required:    true,
		},
	}
}

func (o *SpotifyOutput) Initialize(ctx context.Context, cfg core.OutputConfig) error {
	o.destDir = cfg.DestDir
	if o.destDir == "" {
		if d, ok := cfg.Settings["dest_dir"].(string); ok {
			o.destDir = d
		}
	}
	if o.destDir == "" {
		return fmt.Errorf("destination directory not specified")
	}
	return os.MkdirAll(o.destDir, 0o755)
}

func (o *SpotifyOutput) Export(ctx context.Context, item core.ProcessedItem) error {
	if item.Type != "playlist" {
		return nil
	}

	title, _ := item.Metadata["title"].(string)
	if title == "" {
		title = "playlist"
	}

	// Generate a simple text file with track names (for tools like Soundiiz, TuneMyMusic, etc.)
	destPath := filepath.Join(o.destDir, sanitizeFilename(title)+".txt")

	f, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer f.Close()

	f.WriteString(fmt.Sprintf("# Playlist: %s\n", title))
	f.WriteString("# Format: Artist - Title\n\n")

	if pl, ok := item.Metadata["playlist"].(metadata.Playlist); ok {
		for _, track := range pl.Tracks {
			if track.Artist != "" {
				f.WriteString(fmt.Sprintf("%s - %s\n", track.Artist, track.Title))
			} else {
				f.WriteString(track.Title + "\n")
			}
		}
	} else if songs, ok := item.Metadata["songs"].([]metadata.LikedSong); ok {
		for _, song := range songs {
			if song.Artist != "" {
				f.WriteString(fmt.Sprintf("%s - %s\n", song.Artist, song.Title))
			} else {
				f.WriteString(song.Title + "\n")
			}
		}
	}

	return nil
}

func (o *SpotifyOutput) ExportBatch(ctx context.Context, items []core.ProcessedItem, progress core.ProgressFunc) error {
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (o *SpotifyOutput) Finalize(ctx context.Context) error {
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func sanitizeFilename(name string) string {
	// Replace characters that are problematic in filenames
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(name)
}

// Register all outputs
func init() {
	_ = core.RegisterOutput(NewJSON())
	_ = core.RegisterOutput(NewCSV())
	_ = core.RegisterOutput(NewM3U())
	_ = core.RegisterOutput(NewSpotify())
}
