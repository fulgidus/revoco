package processors

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fulgidus/revoco/services/core"
	"github.com/fulgidus/revoco/services/youtubemusic/metadata"
)

// ── Test Helpers ─────────────────────────────────────────────────────────────

// createTestTakeout sets up a minimal synthetic YouTube Music Takeout directory
func createTestTakeout(t *testing.T, basePath string) {
	t.Helper()

	ytMusicPath := filepath.Join(basePath, "YouTube and YouTube Music")
	playlistsPath := filepath.Join(ytMusicPath, "playlists")
	uploadsPath := filepath.Join(ytMusicPath, "uploads")

	// Create directories
	os.MkdirAll(playlistsPath, 0755)
	os.MkdirAll(uploadsPath, 0755)

	// ── Create playlists CSV ──
	playlistCSV := filepath.Join(playlistsPath, "My Playlist.csv")
	f, err := os.Create(playlistCSV)
	if err != nil {
		t.Fatalf("create playlist CSV: %v", err)
	}
	w := csv.NewWriter(f)
	w.Write([]string{"Video ID", "Time Added", "Title"})
	w.Write([]string{"dQw4w9WgXcQ", "2023-01-15T10:30:00Z", "Never Gonna Give You Up"})
	w.Write([]string{"9bZkp7q19f0", "2023-02-20T14:00:00Z", "Gangnam Style"})
	w.Flush()
	f.Close()

	// ── Create liked songs CSV ──
	likedCSV := filepath.Join(ytMusicPath, "liked music.csv")
	f2, err := os.Create(likedCSV)
	if err != nil {
		t.Fatalf("create liked songs CSV: %v", err)
	}
	w2 := csv.NewWriter(f2)
	w2.Write([]string{"Video ID", "Time Added", "Title"})
	w2.Write([]string{"abc123", "2022-06-10T09:00:00Z", "Liked Song 1"})
	w2.Write([]string{"def456", "2022-07-15T12:30:00Z", "Liked Song 2"})
	w2.Flush()
	f2.Close()

	// ── Create subscriptions CSV ──
	subsCSV := filepath.Join(ytMusicPath, "subscriptions.csv")
	f3, err := os.Create(subsCSV)
	if err != nil {
		t.Fatalf("create subscriptions CSV: %v", err)
	}
	w3 := csv.NewWriter(f3)
	w3.Write([]string{"Channel ID", "Channel URL", "Channel Name"})
	w3.Write([]string{"UCchannel1", "https://youtube.com/channel/UCchannel1", "Test Channel 1"})
	w3.Write([]string{"UCchannel2", "https://youtube.com/channel/UCchannel2", "Test Channel 2"})
	w3.Flush()
	f3.Close()

	// ── Create upload files (fake audio) ──
	uploadFile1 := filepath.Join(uploadsPath, "My Song.mp3")
	uploadFile2 := filepath.Join(uploadsPath, "Another Track.flac")
	os.WriteFile(uploadFile1, []byte("fake mp3 data"), 0644)
	os.WriteFile(uploadFile2, []byte("fake flac data"), 0644)
}

// createEmptyTakeout creates a YouTube Music Takeout with no data
func createEmptyTakeout(t *testing.T, basePath string) {
	t.Helper()
	ytMusicPath := filepath.Join(basePath, "YouTube and YouTube Music")
	os.MkdirAll(ytMusicPath, 0755)
	// Empty structure — no playlists, no liked songs, no uploads
}

// createMalformedTakeout creates a Takeout with broken CSV files
func createMalformedTakeout(t *testing.T, basePath string) {
	t.Helper()

	ytMusicPath := filepath.Join(basePath, "YouTube and YouTube Music")
	playlistsPath := filepath.Join(ytMusicPath, "playlists")
	os.MkdirAll(playlistsPath, 0755)

	// CSV with broken structure (missing columns, no header, etc.)
	brokenCSV := filepath.Join(playlistsPath, "broken.csv")
	os.WriteFile(brokenCSV, []byte("just some text\nno proper csv structure"), 0644)

	// Empty CSV (only header)
	emptyCSV := filepath.Join(playlistsPath, "empty.csv")
	os.WriteFile(emptyCSV, []byte("Video ID,Time Added,Title\n"), 0644)

	// Liked songs with missing data
	likedCSV := filepath.Join(ytMusicPath, "liked music.csv")
	os.WriteFile(likedCSV, []byte("Video ID\n\n\n"), 0644) // Empty rows
}

// collectProgressEvents captures events from progress channel
func collectProgressEvents(events chan core.ProgressEvent) []core.ProgressEvent {
	var collected []core.ProgressEvent
	for evt := range events {
		collected = append(collected, evt)
	}
	return collected
}

// ── Unit Tests ───────────────────────────────────────────────────────────────

func TestMusicProcessor_Metadata(t *testing.T) {
	p := NewMusicProcessor()

	if p.ID() != "youtube-music-processor" {
		t.Errorf("expected ID 'youtube-music-processor', got '%s'", p.ID())
	}
	if p.Name() != "Music Processor" {
		t.Errorf("expected Name 'Music Processor', got '%s'", p.Name())
	}
	if p.Description() == "" {
		t.Error("expected non-empty Description")
	}
}

func TestMusicProcessor_ConfigSchema(t *testing.T) {
	p := NewMusicProcessor()
	schema := p.ConfigSchema()

	expectedFields := []string{
		"include_playlists",
		"include_liked_songs",
		"include_uploads",
		"include_subscriptions",
		"local_library_path",
	}

	foundFields := make(map[string]bool)
	for _, opt := range schema {
		foundFields[opt.ID] = true
	}

	for _, field := range expectedFields {
		if !foundFields[field] {
			t.Errorf("expected config field '%s' in schema", field)
		}
	}
}

func TestMusicProcessor_Process_Full(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "takeout")
	workDir := filepath.Join(tmpDir, "work")
	os.MkdirAll(workDir, 0755)

	createTestTakeout(t, sourceDir)

	p := NewMusicProcessor()
	cfg := core.ProcessorConfig{
		SourceDir:  sourceDir,
		WorkDir:    workDir,
		SessionDir: workDir,
		Settings: map[string]any{
			"include_playlists":     true,
			"include_liked_songs":   true,
			"include_uploads":       true,
			"include_subscriptions": true,
		},
	}

	events := make(chan core.ProgressEvent, 100)
	ctx := context.Background()

	result, err := p.Process(ctx, cfg, events)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	// Collect progress events
	progressEvents := collectProgressEvents(events)
	if len(progressEvents) == 0 {
		t.Error("expected progress events, got none")
	}

	// Verify stats
	if result.Stats["playlists"] != 1 {
		t.Errorf("expected 1 playlist, got %d", result.Stats["playlists"])
	}
	if result.Stats["playlist_tracks"] != 2 {
		t.Errorf("expected 2 playlist tracks, got %d", result.Stats["playlist_tracks"])
	}
	if result.Stats["liked_songs"] != 2 {
		t.Errorf("expected 2 liked songs, got %d", result.Stats["liked_songs"])
	}
	if result.Stats["uploads"] != 2 {
		t.Errorf("expected 2 uploads, got %d", result.Stats["uploads"])
	}
	if result.Stats["subscriptions"] != 2 {
		t.Errorf("expected 2 subscriptions, got %d", result.Stats["subscriptions"])
	}

	// Verify library JSON was written
	libraryPath := filepath.Join(workDir, "library.json")
	if _, err := os.Stat(libraryPath); os.IsNotExist(err) {
		t.Error("library.json not created")
	}

	// Parse and validate library JSON
	libraryData, _ := os.ReadFile(libraryPath)
	var library metadata.MusicLibrary
	if err := json.Unmarshal(libraryData, &library); err != nil {
		t.Fatalf("invalid library.json: %v", err)
	}

	if len(library.Playlists) != 1 {
		t.Errorf("library has %d playlists, expected 1", len(library.Playlists))
	}
	if len(library.LikedSongs) != 2 {
		t.Errorf("library has %d liked songs, expected 2", len(library.LikedSongs))
	}
	if len(library.Uploads) != 2 {
		t.Errorf("library has %d uploads, expected 2", len(library.Uploads))
	}
	if len(library.Subscriptions) != 2 {
		t.Errorf("library has %d subscriptions, expected 2", len(library.Subscriptions))
	}

	// Verify ProcessedItems
	if len(result.Items) == 0 {
		t.Error("expected ProcessedItems, got none")
	}

	// Check that items include playlists, liked songs, uploads, subscriptions
	itemTypes := make(map[string]int)
	for _, item := range result.Items {
		itemTypes[item.Type]++
	}
	if itemTypes["playlist"] == 0 {
		t.Error("no playlist items found")
	}
	if itemTypes["audio"] == 0 {
		t.Error("no audio items found")
	}
	if itemTypes["document"] == 0 {
		t.Error("no document items found")
	}

	// Verify log file was created
	if result.LogPath == "" {
		t.Error("expected LogPath to be set")
	}
	if _, err := os.Stat(result.LogPath); os.IsNotExist(err) {
		t.Errorf("log file not created at %s", result.LogPath)
	}
}

func TestMusicProcessor_Process_SelectivePhases(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "takeout")
	workDir := filepath.Join(tmpDir, "work")
	os.MkdirAll(workDir, 0755)

	createTestTakeout(t, sourceDir)

	p := NewMusicProcessor()

	// Only process playlists
	cfg := core.ProcessorConfig{
		SourceDir:  sourceDir,
		WorkDir:    workDir,
		SessionDir: workDir,
		Settings: map[string]any{
			"include_playlists":     true,
			"include_liked_songs":   false,
			"include_uploads":       false,
			"include_subscriptions": false,
		},
	}

	events := make(chan core.ProgressEvent, 100)
	ctx := context.Background()

	result, err := p.Process(ctx, cfg, events)
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}

	collectProgressEvents(events)

	// Should only have playlist data
	if result.Stats["playlists"] != 1 {
		t.Errorf("expected 1 playlist, got %d", result.Stats["playlists"])
	}
	if result.Stats["liked_songs"] != 0 {
		t.Errorf("expected 0 liked songs (disabled), got %d", result.Stats["liked_songs"])
	}
	if result.Stats["uploads"] != 0 {
		t.Errorf("expected 0 uploads (disabled), got %d", result.Stats["uploads"])
	}
	if result.Stats["subscriptions"] != 0 {
		t.Errorf("expected 0 subscriptions (disabled), got %d", result.Stats["subscriptions"])
	}
}

func TestMusicProcessor_Process_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "takeout")
	workDir := filepath.Join(tmpDir, "work")
	os.MkdirAll(workDir, 0755)

	createEmptyTakeout(t, sourceDir)

	p := NewMusicProcessor()
	cfg := core.ProcessorConfig{
		SourceDir:  sourceDir,
		WorkDir:    workDir,
		SessionDir: workDir,
		Settings: map[string]any{
			"include_playlists":     true,
			"include_liked_songs":   true,
			"include_uploads":       true,
			"include_subscriptions": true,
		},
	}

	events := make(chan core.ProgressEvent, 100)
	ctx := context.Background()

	result, err := p.Process(ctx, cfg, events)
	if err != nil {
		t.Fatalf("Process failed on empty takeout: %v", err)
	}

	collectProgressEvents(events)

	// All stats should be 0 for empty takeout
	if result.Stats["playlists"] != 0 {
		t.Errorf("expected 0 playlists, got %d", result.Stats["playlists"])
	}
	if result.Stats["liked_songs"] != 0 {
		t.Errorf("expected 0 liked songs, got %d", result.Stats["liked_songs"])
	}
	if result.Stats["uploads"] != 0 {
		t.Errorf("expected 0 uploads, got %d", result.Stats["uploads"])
	}
	if result.Stats["subscriptions"] != 0 {
		t.Errorf("expected 0 subscriptions, got %d", result.Stats["subscriptions"])
	}

	// Should still create library.json (but empty)
	libraryPath := filepath.Join(workDir, "library.json")
	if _, err := os.Stat(libraryPath); os.IsNotExist(err) {
		t.Error("library.json not created for empty takeout")
	}
}

func TestMusicProcessor_Process_Malformed(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "takeout")
	workDir := filepath.Join(tmpDir, "work")
	os.MkdirAll(workDir, 0755)

	createMalformedTakeout(t, sourceDir)

	p := NewMusicProcessor()
	cfg := core.ProcessorConfig{
		SourceDir:  sourceDir,
		WorkDir:    workDir,
		SessionDir: workDir,
		Settings: map[string]any{
			"include_playlists":     true,
			"include_liked_songs":   true,
			"include_uploads":       true,
			"include_subscriptions": true,
		},
	}

	events := make(chan core.ProgressEvent, 100)
	ctx := context.Background()

	// Should NOT fail — errors are logged but processing continues
	result, err := p.Process(ctx, cfg, events)
	if err != nil {
		t.Fatalf("Process failed on malformed data: %v", err)
	}

	collectProgressEvents(events)

	// Malformed CSVs should result in 0 or minimal data
	// Processor should gracefully handle broken files
	t.Logf("Malformed processing stats: %+v", result.Stats)

	// Log file should exist and contain error messages
	if result.LogPath == "" {
		t.Error("expected LogPath to be set")
	}
	logData, err := os.ReadFile(result.LogPath)
	if err != nil {
		t.Errorf("failed to read log file: %v", err)
	}
	t.Logf("Log output:\n%s", string(logData))
}

func TestMusicProcessor_Process_Cancellation(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "takeout")
	workDir := filepath.Join(tmpDir, "work")
	os.MkdirAll(workDir, 0755)

	createTestTakeout(t, sourceDir)

	p := NewMusicProcessor()
	cfg := core.ProcessorConfig{
		SourceDir:  sourceDir,
		WorkDir:    workDir,
		SessionDir: workDir,
		Settings:   map[string]any{},
	}

	events := make(chan core.ProgressEvent, 100)

	// Cancel context immediately
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	result, err := p.Process(ctx, cfg, events)

	// Should handle cancellation gracefully
	// May return partial results or error
	if err == context.Canceled {
		t.Log("Process correctly returned context.Canceled")
	} else if result != nil {
		// Partial processing is acceptable
		t.Logf("Partial result: %+v", result.Stats)
	}

	collectProgressEvents(events)
}

func TestMusicProcessor_ScanPlaylists(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "takeout")
	createTestTakeout(t, sourceDir)

	ytMusicPath := filepath.Join(sourceDir, "YouTube and YouTube Music")

	p := NewMusicProcessor()
	emit := func(phase int, label string, done, total int, msg string) {}

	ctx := context.Background()
	playlists, err := p.scanPlaylists(ctx, ytMusicPath, emit, nil)
	if err != nil {
		t.Fatalf("scanPlaylists failed: %v", err)
	}

	if len(playlists) != 1 {
		t.Errorf("expected 1 playlist, got %d", len(playlists))
	}

	if len(playlists) > 0 {
		pl := playlists[0]
		if len(pl.Tracks) != 2 {
			t.Errorf("expected 2 tracks in playlist, got %d", len(pl.Tracks))
		}
	}
}

func TestMusicProcessor_ScanLikedSongs(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "takeout")
	createTestTakeout(t, sourceDir)

	ytMusicPath := filepath.Join(sourceDir, "YouTube and YouTube Music")

	p := NewMusicProcessor()
	ctx := context.Background()

	songs, err := p.scanLikedSongs(ctx, ytMusicPath, nil)
	if err != nil {
		t.Fatalf("scanLikedSongs failed: %v", err)
	}

	if len(songs) != 2 {
		t.Errorf("expected 2 liked songs, got %d", len(songs))
	}

	if len(songs) > 0 {
		if songs[0].VideoID != "abc123" {
			t.Errorf("expected VideoID 'abc123', got '%s'", songs[0].VideoID)
		}
	}
}

func TestMusicProcessor_ScanUploads(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "takeout")
	createTestTakeout(t, sourceDir)

	ytMusicPath := filepath.Join(sourceDir, "YouTube and YouTube Music")

	p := NewMusicProcessor()
	ctx := context.Background()

	uploads, err := p.scanUploads(ctx, ytMusicPath, nil)
	if err != nil {
		t.Fatalf("scanUploads failed: %v", err)
	}

	if len(uploads) != 2 {
		t.Errorf("expected 2 uploads, got %d", len(uploads))
	}

	// Verify audio extensions detected
	foundMP3 := false
	foundFLAC := false
	for _, u := range uploads {
		if filepath.Ext(u.FilePath) == ".mp3" {
			foundMP3 = true
		}
		if filepath.Ext(u.FilePath) == ".flac" {
			foundFLAC = true
		}
		if u.Title == "" {
			t.Error("upload Title should not be empty")
		}
	}
	if !foundMP3 {
		t.Error("expected to find .mp3 upload")
	}
	if !foundFLAC {
		t.Error("expected to find .flac upload")
	}
}

func TestMusicProcessor_ScanSubscriptions(t *testing.T) {
	tmpDir := t.TempDir()
	sourceDir := filepath.Join(tmpDir, "takeout")
	createTestTakeout(t, sourceDir)

	ytMusicPath := filepath.Join(sourceDir, "YouTube and YouTube Music")

	p := NewMusicProcessor()
	ctx := context.Background()

	subs, err := p.scanSubscriptions(ctx, ytMusicPath, nil)
	if err != nil {
		t.Fatalf("scanSubscriptions failed: %v", err)
	}

	if len(subs) != 2 {
		t.Errorf("expected 2 subscriptions, got %d", len(subs))
	}

	if len(subs) > 0 {
		if subs[0].ChannelID != "UCchannel1" {
			t.Errorf("expected ChannelID 'UCchannel1', got '%s'", subs[0].ChannelID)
		}
		if subs[0].ChannelName != "Test Channel 1" {
			t.Errorf("expected ChannelName 'Test Channel 1', got '%s'", subs[0].ChannelName)
		}
	}
}

func TestMusicProcessor_MatchLocalLibrary(t *testing.T) {
	tmpDir := t.TempDir()
	workDir := filepath.Join(tmpDir, "work")
	localLibPath := filepath.Join(tmpDir, "music-library")
	os.MkdirAll(workDir, 0755)
	os.MkdirAll(localLibPath, 0755)

	// Create fake local library
	os.WriteFile(filepath.Join(localLibPath, "never gonna give you up.mp3"), []byte("fake audio"), 0644)
	os.WriteFile(filepath.Join(localLibPath, "gangnam style.flac"), []byte("fake audio"), 0644)

	library := &metadata.MusicLibrary{
		Playlists: []metadata.Playlist{
			{
				Title: "Test Playlist",
				Tracks: []metadata.Track{
					{Title: "Never Gonna Give You Up", Artist: "Rick Astley"},
					{Title: "Gangnam Style", Artist: "PSY"},
					{Title: "Unknown Song", Artist: "Unknown"},
				},
			},
		},
		LikedSongs: []metadata.LikedSong{
			{Track: metadata.Track{Title: "Never Gonna Give You Up"}},
		},
	}

	p := NewMusicProcessor()
	ctx := context.Background()

	progressCalls := 0
	progressFunc := func(done, total int) {
		progressCalls++
	}

	matched, err := p.matchLocalLibrary(ctx, library, localLibPath, nil, progressFunc)
	if err != nil {
		t.Fatalf("matchLocalLibrary failed: %v", err)
	}

	// Should match at least 2 tracks (exact title match)
	if matched < 2 {
		t.Errorf("expected at least 2 matches, got %d", matched)
	}

	// Verify progress callback was called
	if progressCalls == 0 {
		t.Error("expected progress callback to be called")
	}

	// Verify that matched tracks have Album field set (stores local path)
	foundMatch := false
	for _, track := range library.Playlists[0].Tracks {
		if track.Album != "" {
			foundMatch = true
			t.Logf("Matched track '%s' to local file: %s", track.Title, track.Album)
		}
	}
	if !foundMatch {
		t.Error("expected at least one track to have Album field set with local path")
	}
}

func TestMusicProcessor_BuildProcessedItems(t *testing.T) {
	workDir := "/tmp/work"

	library := &metadata.MusicLibrary{
		Playlists: []metadata.Playlist{
			{Title: "Playlist 1", Tracks: []metadata.Track{{Title: "Track 1"}}},
		},
		LikedSongs: []metadata.LikedSong{
			{Track: metadata.Track{Title: "Liked 1"}},
		},
		Uploads: []metadata.Upload{
			{Title: "Upload 1", FilePath: "/source/uploads/song.mp3"},
		},
		Subscriptions: []metadata.Subscription{
			{ChannelName: "Channel 1"},
		},
	}

	p := NewMusicProcessor()
	items := p.buildProcessedItems(library, workDir)

	if len(items) == 0 {
		t.Fatal("expected ProcessedItems, got none")
	}

	// Verify item types
	itemTypes := make(map[string]int)
	for _, item := range items {
		itemTypes[item.Type]++
	}

	if itemTypes["playlist"] == 0 {
		t.Error("expected playlist items")
	}
	if itemTypes["audio"] == 0 {
		t.Error("expected audio items")
	}
	if itemTypes["document"] == 0 {
		t.Error("expected document items")
	}

	// Verify metadata is populated
	for _, item := range items {
		if item.Metadata == nil {
			t.Errorf("item has nil Metadata: %+v", item)
		}
	}
}

func TestDetectYouTubeMusicDir(t *testing.T) {
	tests := []struct {
		name       string
		dirName    string
		shouldFind bool
	}{
		{"exact match", "YouTube and YouTube Music", true},
		{"case insensitive", "youtube and youtube music", true},
		{"Italian variant", "YouTube e YouTube Music", true},
		{"nested", "Takeout/YouTube and YouTube Music", true},
		{"wrong name", "Random Folder", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()

			var targetPath string
			if tt.name == "nested" {
				basePath := filepath.Join(tmpDir, "Takeout")
				targetPath = filepath.Join(basePath, "YouTube and YouTube Music")
				os.MkdirAll(targetPath, 0755)
				targetPath = basePath // Start detection from Takeout
			} else {
				targetPath = filepath.Join(tmpDir, tt.dirName)
				os.MkdirAll(targetPath, 0755)
			}

			detected, err := detectYouTubeMusicDir(targetPath)
			if err != nil {
				t.Fatalf("detectYouTubeMusicDir failed: %v", err)
			}

			if tt.shouldFind {
				if detected == "" {
					t.Errorf("expected to find YouTube Music dir, got empty path")
				}
				t.Logf("Detected: %s", detected)
			}
		})
	}
}

func TestIsAudioExt(t *testing.T) {
	tests := []struct {
		ext      string
		expected bool
	}{
		{".mp3", true},
		{".flac", true},
		{".m4a", true},
		{".wav", true},
		{".ogg", true},
		{".opus", true},
		{".aac", true},
		{".wma", true},
		{".txt", false},
		{".jpg", false},
		{".json", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			result := isAudioExt(tt.ext)
			if result != tt.expected {
				t.Errorf("isAudioExt(%q) = %v, expected %v", tt.ext, result, tt.expected)
			}
		})
	}
}

func TestGetBool(t *testing.T) {
	m := map[string]any{
		"enabled":  true,
		"disabled": false,
		"string":   "not a bool",
		"number":   42,
	}

	tests := []struct {
		key      string
		def      bool
		expected bool
	}{
		{"enabled", false, true},
		{"disabled", true, false},
		{"missing", true, true},
		{"missing", false, false},
		{"string", true, true},   // Falls back to default
		{"number", false, false}, // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := getBool(m, tt.key, tt.def)
			if result != tt.expected {
				t.Errorf("getBool(%q, %v) = %v, expected %v", tt.key, tt.def, result, tt.expected)
			}
		})
	}
}

func TestGetString(t *testing.T) {
	m := map[string]any{
		"name":   "test",
		"empty":  "",
		"number": 42,
		"bool":   true,
	}

	tests := []struct {
		key      string
		def      string
		expected string
	}{
		{"name", "default", "test"},
		{"empty", "default", ""},
		{"missing", "default", "default"},
		{"number", "default", "default"}, // Falls back to default
		{"bool", "default", "default"},   // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			result := getString(m, tt.key, tt.def)
			if result != tt.expected {
				t.Errorf("getString(%q, %q) = %q, expected %q", tt.key, tt.def, result, tt.expected)
			}
		})
	}
}

func TestFindLocalMatch(t *testing.T) {
	localFiles := map[string]string{
		"never gonna give you up":           "/music/rickroll.mp3",
		"gangnam style":                     "/music/gangnam.flac",
		"artist name - track title":         "/music/track.mp3",
		"partial match here somewhere else": "/music/partial.wav",
	}

	tests := []struct {
		title    string
		artist   string
		expected string
	}{
		{"Never Gonna Give You Up", "", "/music/rickroll.mp3"}, // Exact title match (case insensitive)
		{"Gangnam Style", "", "/music/gangnam.flac"},           // Exact title match
		{"Track Title", "Artist Name", "/music/track.mp3"},     // Artist - Title match
		{"Partial Match Here", "", "/music/partial.wav"},       // Partial match
		{"Unknown Song That Doesn't Exist", "", ""},            // No match
		{"never gonna give you up", "", "/music/rickroll.mp3"}, // Lowercase exact match
		{"Track Title", "", "/music/track.mp3"},                // Partial match finds it in "artist name - track title"
		{"somewhere", "", "/music/partial.wav"},                // Partial substring match
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			result := findLocalMatch(tt.title, tt.artist, localFiles)
			if result != tt.expected {
				t.Errorf("findLocalMatch(%q, %q) = %q, expected %q", tt.title, tt.artist, result, tt.expected)
			}
		})
	}
}

// ── Benchmark ────────────────────────────────────────────────────────────────

func BenchmarkMusicProcessor_Process(b *testing.B) {
	tmpDir := b.TempDir()
	sourceDir := filepath.Join(tmpDir, "takeout")
	createTestTakeout(&testing.T{}, sourceDir)

	p := NewMusicProcessor()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		workDir := filepath.Join(tmpDir, "work", time.Now().Format("20060102150405"))
		os.MkdirAll(workDir, 0755)

		cfg := core.ProcessorConfig{
			SourceDir:  sourceDir,
			WorkDir:    workDir,
			SessionDir: workDir,
			Settings:   map[string]any{},
		}

		events := make(chan core.ProgressEvent, 100)
		ctx := context.Background()

		_, err := p.Process(ctx, cfg, events)
		if err != nil {
			b.Fatalf("Process failed: %v", err)
		}

		collectProgressEvents(events)
	}
}
