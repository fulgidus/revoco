// Package processors provides data processing modules for YouTube Music.
package processors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/youtubemusic/metadata"
)

// MusicProcessor handles the YouTube Music processing pipeline.
type MusicProcessor struct{}

// NewMusicProcessor creates a new music processor.
func NewMusicProcessor() *MusicProcessor {
	return &MusicProcessor{}
}

func (p *MusicProcessor) ID() string   { return "youtube-music-processor" }
func (p *MusicProcessor) Name() string { return "Music Processor" }
func (p *MusicProcessor) Description() string {
	return "Process YouTube Music data: playlists, liked songs, uploads, subscriptions"
}

// ConfigSchema returns the configuration options for this processor.
func (p *MusicProcessor) ConfigSchema() []core.ConfigOption {
	return []core.ConfigOption{
		{
			ID:          "include_playlists",
			Name:        "Include Playlists",
			Description: "Process playlist data",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "include_liked_songs",
			Name:        "Include Liked Songs",
			Description: "Process liked/thumbs-up songs",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "include_uploads",
			Name:        "Include Uploads",
			Description: "Process uploaded music files",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "include_subscriptions",
			Name:        "Include Subscriptions",
			Description: "Process channel subscriptions",
			Type:        "bool",
			Default:     true,
		},
		{
			ID:          "local_library_path",
			Name:        "Local Library Path",
			Description: "Path to local music library for track matching",
			Type:        "string",
		},
	}
}

// Process runs the YouTube Music processing pipeline.
func (p *MusicProcessor) Process(ctx context.Context, cfg core.ProcessorConfig, events chan<- core.ProgressEvent) (*core.ProcessResult, error) {
	defer close(events)

	emit := func(phase int, label string, done, total int, msg string) {
		select {
		case events <- core.ProgressEvent{
			Phase:   phase,
			Label:   label,
			Done:    done,
			Total:   total,
			Message: msg,
		}:
		case <-ctx.Done():
		}
	}

	settings := cfg.Settings
	if settings == nil {
		settings = make(map[string]any)
	}

	includePlaylists := getBool(settings, "include_playlists", true)
	includeLikedSongs := getBool(settings, "include_liked_songs", true)
	includeUploads := getBool(settings, "include_uploads", true)
	includeSubs := getBool(settings, "include_subscriptions", true)
	localLibPath := getString(settings, "local_library_path", "")

	// Setup logging
	logDir := cfg.SessionDir
	if logDir == "" {
		logDir = cfg.WorkDir
	}
	os.MkdirAll(logDir, 0o755)

	logPath := filepath.Join(logDir, "process.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer logFile.Close()
	logger := log.New(logFile, "", log.LstdFlags)
	logger.Printf("=== YouTube Music processing started (source=%s) ===", cfg.SourceDir)

	// Find YouTube Music directory
	ytMusicPath, err := detectYouTubeMusicDir(cfg.SourceDir)
	if err != nil {
		return nil, err
	}
	emit(0, "Setup", 1, 1, fmt.Sprintf("Found: %s", filepath.Base(ytMusicPath)))

	result := &core.ProcessResult{
		Stats:    make(map[string]int),
		Metadata: make(map[string]any),
		LogPath:  logPath,
	}

	library := &metadata.MusicLibrary{}

	// ── Phase 1: Scan for playlists ─────────────────────────────────────────
	if includePlaylists {
		emit(1, "Scanning playlists", 0, 0, "")
		playlists, err := p.scanPlaylists(ctx, ytMusicPath, emit, logger)
		if err != nil {
			logger.Printf("[Phase 1] Error scanning playlists: %v", err)
		}
		library.Playlists = playlists
		result.Stats["playlists"] = len(playlists)
		totalTracks := 0
		for _, pl := range playlists {
			totalTracks += len(pl.Tracks)
		}
		result.Stats["playlist_tracks"] = totalTracks
		emit(1, "Playlists done", len(playlists), len(playlists),
			fmt.Sprintf("%d playlists, %d tracks", len(playlists), totalTracks))
		logger.Printf("[Phase 1] playlists=%d tracks=%d", len(playlists), totalTracks)
	} else {
		emit(1, "Playlists skipped", 1, 1, "")
		logger.Printf("[Phase 1] Skipped")
	}

	// ── Phase 2: Scan liked songs ───────────────────────────────────────────
	if includeLikedSongs {
		emit(2, "Scanning liked songs", 0, 0, "")
		likedSongs, err := p.scanLikedSongs(ctx, ytMusicPath, logger)
		if err != nil {
			logger.Printf("[Phase 2] Error scanning liked songs: %v", err)
		}
		library.LikedSongs = likedSongs
		result.Stats["liked_songs"] = len(likedSongs)
		emit(2, "Liked songs done", len(likedSongs), len(likedSongs),
			fmt.Sprintf("%d liked songs", len(likedSongs)))
		logger.Printf("[Phase 2] liked_songs=%d", len(likedSongs))
	} else {
		emit(2, "Liked songs skipped", 1, 1, "")
		logger.Printf("[Phase 2] Skipped")
	}

	// ── Phase 3: Scan uploads ───────────────────────────────────────────────
	if includeUploads {
		emit(3, "Scanning uploads", 0, 0, "")
		uploads, err := p.scanUploads(ctx, ytMusicPath, logger)
		if err != nil {
			logger.Printf("[Phase 3] Error scanning uploads: %v", err)
		}
		library.Uploads = uploads
		result.Stats["uploads"] = len(uploads)
		emit(3, "Uploads done", len(uploads), len(uploads),
			fmt.Sprintf("%d uploads", len(uploads)))
		logger.Printf("[Phase 3] uploads=%d", len(uploads))
	} else {
		emit(3, "Uploads skipped", 1, 1, "")
		logger.Printf("[Phase 3] Skipped")
	}

	// ── Phase 4: Scan subscriptions ─────────────────────────────────────────
	if includeSubs {
		emit(4, "Scanning subscriptions", 0, 0, "")
		subs, err := p.scanSubscriptions(ctx, ytMusicPath, logger)
		if err != nil {
			logger.Printf("[Phase 4] Error scanning subscriptions: %v", err)
		}
		library.Subscriptions = subs
		result.Stats["subscriptions"] = len(subs)
		emit(4, "Subscriptions done", len(subs), len(subs),
			fmt.Sprintf("%d subscriptions", len(subs)))
		logger.Printf("[Phase 4] subscriptions=%d", len(subs))
	} else {
		emit(4, "Subscriptions skipped", 1, 1, "")
		logger.Printf("[Phase 4] Skipped")
	}

	// ── Phase 5: Local library matching (if configured) ─────────────────────
	if localLibPath != "" {
		emit(5, "Matching local library", 0, 0, "")
		matched, err := p.matchLocalLibrary(ctx, library, localLibPath, logger, func(done, total int) {
			emit(5, "Matching tracks", done, total, "")
		})
		if err != nil {
			logger.Printf("[Phase 5] Error matching library: %v", err)
		}
		result.Stats["local_matches"] = matched
		emit(5, "Local matching done", matched, matched,
			fmt.Sprintf("%d tracks matched to local files", matched))
		logger.Printf("[Phase 5] local_matches=%d", matched)
	} else {
		emit(5, "Local matching skipped", 1, 1, "No local library configured")
		logger.Printf("[Phase 5] Skipped - no local library path")
	}

	// ── Phase 6: Write output ───────────────────────────────────────────────
	emit(6, "Writing output", 0, 1, "")

	// Write the full library as JSON
	libraryPath := filepath.Join(cfg.WorkDir, "library.json")
	libraryData, _ := json.MarshalIndent(library, "", "  ")
	os.WriteFile(libraryPath, libraryData, 0o644)

	result.Metadata["library"] = library
	result.Metadata["library_path"] = libraryPath

	emit(6, "Output done", 1, 1, fmt.Sprintf("Wrote %s", filepath.Base(libraryPath)))
	logger.Printf("[Phase 6] Wrote library.json")
	logger.Printf("=== YouTube Music processing complete ===")

	// Build ProcessedItems for output modules
	result.Items = p.buildProcessedItems(library, cfg.WorkDir)

	return result, nil
}

func (p *MusicProcessor) scanPlaylists(ctx context.Context, basePath string, emit func(int, string, int, int, string), logger *log.Logger) ([]metadata.Playlist, error) {
	var playlists []metadata.Playlist

	// Look for playlist files in typical Takeout locations
	playlistDirs := []string{
		filepath.Join(basePath, "playlists"),
		filepath.Join(basePath, "Playlists"),
		filepath.Join(basePath, "playlist"),
	}

	for _, dir := range playlistDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}

		for i, entry := range entries {
			select {
			case <-ctx.Done():
				return playlists, ctx.Err()
			default:
			}

			if entry.IsDir() {
				// Each subdirectory might be a playlist
				pl, err := p.parsePlaylistDir(filepath.Join(dir, entry.Name()))
				if err == nil && len(pl.Tracks) > 0 {
					playlists = append(playlists, *pl)
				}
			} else if strings.HasSuffix(strings.ToLower(entry.Name()), ".csv") {
				pl, err := metadata.ParsePlaylistCSV(filepath.Join(dir, entry.Name()))
				if err == nil {
					pl.Title = strings.TrimSuffix(entry.Name(), ".csv")
					playlists = append(playlists, *pl)
				}
			}

			emit(1, "Scanning playlists", i+1, len(entries), "")
		}
	}

	return playlists, nil
}

func (p *MusicProcessor) parsePlaylistDir(path string) (*metadata.Playlist, error) {
	playlist := &metadata.Playlist{
		Title: filepath.Base(path),
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if strings.HasSuffix(strings.ToLower(entry.Name()), ".csv") {
			pl, err := metadata.ParsePlaylistCSV(filepath.Join(path, entry.Name()))
			if err == nil {
				playlist.Tracks = append(playlist.Tracks, pl.Tracks...)
			}
		}
	}

	return playlist, nil
}

func (p *MusicProcessor) scanLikedSongs(ctx context.Context, basePath string, logger *log.Logger) ([]metadata.LikedSong, error) {
	likedPaths := []string{
		filepath.Join(basePath, "liked videos.csv"),
		filepath.Join(basePath, "liked music.csv"),
		filepath.Join(basePath, "Mi piace.csv"),
		filepath.Join(basePath, "playlists", "Liked Music.csv"),
		filepath.Join(basePath, "playlists", "Mi piace.csv"),
	}

	for _, path := range likedPaths {
		if _, err := os.Stat(path); err == nil {
			songs, err := metadata.ParseLikedSongsCSV(path)
			if err == nil {
				return songs, nil
			}
		}
	}

	return nil, nil
}

func (p *MusicProcessor) scanUploads(ctx context.Context, basePath string, logger *log.Logger) ([]metadata.Upload, error) {
	var uploads []metadata.Upload

	uploadDirs := []string{
		filepath.Join(basePath, "uploads"),
		filepath.Join(basePath, "Uploads"),
		filepath.Join(basePath, "music uploads"),
	}

	for _, dir := range uploadDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			continue
		}

		filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}

			ext := strings.ToLower(filepath.Ext(path))
			if isAudioExt(ext) {
				info, _ := d.Info()
				upload := metadata.Upload{
					Title:    strings.TrimSuffix(d.Name(), ext),
					FilePath: path,
				}
				if info != nil {
					upload.FileSize = info.Size()
					upload.UploadedAt = info.ModTime()
				}
				uploads = append(uploads, upload)
			}
			return nil
		})
	}

	return uploads, nil
}

func (p *MusicProcessor) scanSubscriptions(ctx context.Context, basePath string, logger *log.Logger) ([]metadata.Subscription, error) {
	subPaths := []string{
		filepath.Join(basePath, "subscriptions.csv"),
		filepath.Join(basePath, "subscriptions", "subscriptions.csv"),
	}

	for _, path := range subPaths {
		if _, err := os.Stat(path); err == nil {
			subs, err := metadata.ParseSubscriptionsCSV(path)
			if err == nil {
				return subs, nil
			}
		}
	}

	return nil, nil
}

func (p *MusicProcessor) matchLocalLibrary(ctx context.Context, library *metadata.MusicLibrary, libPath string, logger *log.Logger, progress core.ProgressFunc) (int, error) {
	// Build index of local files by name
	localFiles := make(map[string]string) // lowercase name -> full path

	filepath.WalkDir(libPath, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if isAudioExt(ext) {
			name := strings.ToLower(strings.TrimSuffix(d.Name(), ext))
			localFiles[name] = path
		}
		return nil
	})

	matched := 0
	total := 0
	for _, pl := range library.Playlists {
		total += len(pl.Tracks)
	}
	total += len(library.LikedSongs)

	done := 0
	// Match playlist tracks
	for i := range library.Playlists {
		for j := range library.Playlists[i].Tracks {
			track := &library.Playlists[i].Tracks[j]
			if localPath := findLocalMatch(track.Title, track.Artist, localFiles); localPath != "" {
				track.Album = localPath // Store local path in Album field for now
				matched++
			}
			done++
			if progress != nil {
				progress(done, total)
			}
		}
	}

	// Match liked songs
	for i := range library.LikedSongs {
		song := &library.LikedSongs[i]
		if localPath := findLocalMatch(song.Title, song.Artist, localFiles); localPath != "" {
			song.Album = localPath
			matched++
		}
		done++
		if progress != nil {
			progress(done, total)
		}
	}

	return matched, nil
}

func findLocalMatch(title, artist string, localFiles map[string]string) string {
	// Try exact title match
	titleLower := strings.ToLower(title)
	if path, ok := localFiles[titleLower]; ok {
		return path
	}

	// Try artist - title
	if artist != "" {
		combined := strings.ToLower(artist + " - " + title)
		if path, ok := localFiles[combined]; ok {
			return path
		}
	}

	// Try partial match
	for name, path := range localFiles {
		if strings.Contains(name, titleLower) || strings.Contains(titleLower, name) {
			return path
		}
	}

	return ""
}

func (p *MusicProcessor) buildProcessedItems(library *metadata.MusicLibrary, workDir string) []core.ProcessedItem {
	var items []core.ProcessedItem

	// Add playlists as items
	for _, pl := range library.Playlists {
		items = append(items, core.ProcessedItem{
			SourcePath:    "",
			ProcessedPath: filepath.Join(workDir, "playlists", pl.Title+".json"),
			DestRelPath:   filepath.Join("playlists", pl.Title+".json"),
			Type:          "playlist",
			Metadata: map[string]any{
				"title":       pl.Title,
				"track_count": len(pl.Tracks),
				"playlist":    pl,
			},
		})
	}

	// Add liked songs as a single item
	if len(library.LikedSongs) > 0 {
		items = append(items, core.ProcessedItem{
			SourcePath:    "",
			ProcessedPath: filepath.Join(workDir, "liked_songs.json"),
			DestRelPath:   "liked_songs.json",
			Type:          "playlist",
			Metadata: map[string]any{
				"title":       "Liked Songs",
				"track_count": len(library.LikedSongs),
				"songs":       library.LikedSongs,
			},
		})
	}

	// Add uploads as items
	for _, upload := range library.Uploads {
		if upload.FilePath != "" {
			items = append(items, core.ProcessedItem{
				SourcePath:    upload.FilePath,
				ProcessedPath: upload.FilePath,
				DestRelPath:   filepath.Join("uploads", filepath.Base(upload.FilePath)),
				Type:          "audio",
				Metadata: map[string]any{
					"title":  upload.Title,
					"artist": upload.Artist,
					"upload": upload,
				},
			})
		}
	}

	// Add subscriptions
	if len(library.Subscriptions) > 0 {
		items = append(items, core.ProcessedItem{
			SourcePath:    "",
			ProcessedPath: filepath.Join(workDir, "subscriptions.json"),
			DestRelPath:   "subscriptions.json",
			Type:          "document",
			Metadata: map[string]any{
				"count":         len(library.Subscriptions),
				"subscriptions": library.Subscriptions,
			},
		})
	}

	return items
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func getBool(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

func getString(m map[string]any, key string, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

var youTubeMusicVariants = []string{
	"YouTube Music",
	"YouTube e YouTube Music",
	"YouTube and YouTube Music",
}

func detectYouTubeMusicDir(sourceDir string) (string, error) {
	baseName := filepath.Base(sourceDir)
	for _, variant := range youTubeMusicVariants {
		if strings.EqualFold(baseName, variant) {
			return sourceDir, nil
		}
	}

	var found string
	filepath.WalkDir(sourceDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(sourceDir, path)
		depth := len(strings.Split(rel, string(os.PathSeparator)))
		if depth > 3 {
			return filepath.SkipDir
		}
		for _, variant := range youTubeMusicVariants {
			if strings.EqualFold(d.Name(), variant) {
				found = path
				return filepath.SkipAll
			}
		}
		return nil
	})

	if found != "" {
		return found, nil
	}

	// If no specific folder found, use the source dir itself
	return sourceDir, nil
}

func isAudioExt(ext string) bool {
	audioExts := map[string]bool{
		".mp3": true, ".m4a": true, ".flac": true, ".wav": true,
		".ogg": true, ".opus": true, ".aac": true, ".wma": true,
	}
	return audioExts[ext]
}
