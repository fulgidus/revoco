// Package metadata defines types for YouTube Music Takeout data.
package metadata

import (
	"encoding/csv"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

// Playlist represents a YouTube Music playlist from Takeout.
type Playlist struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Visibility  string    `json:"visibility"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	Tracks      []Track   `json:"tracks"`
}

// Track represents a single track/song.
type Track struct {
	VideoID     string    `json:"video_id"`
	Title       string    `json:"title"`
	Artist      string    `json:"artist"`
	Album       string    `json:"album"`
	Duration    int       `json:"duration_seconds"`
	AddedAt     time.Time `json:"added_at"`
	PlaylistPos int       `json:"playlist_position"`
}

// LikedSong represents a liked/thumbs-up track.
type LikedSong struct {
	Track
	LikedAt time.Time `json:"liked_at"`
}

// Upload represents a user-uploaded music file.
type Upload struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	Artist     string    `json:"artist"`
	Album      string    `json:"album"`
	Duration   int       `json:"duration_seconds"`
	UploadedAt time.Time `json:"uploaded_at"`
	FilePath   string    `json:"file_path,omitempty"` // Path to the actual file if included
	FileSize   int64     `json:"file_size,omitempty"`
}

// Subscription represents a channel subscription.
type Subscription struct {
	ChannelID    string    `json:"channel_id"`
	ChannelName  string    `json:"channel_name"`
	ChannelURL   string    `json:"channel_url"`
	SubscribedAt time.Time `json:"subscribed_at,omitempty"`
}

// WatchHistory represents a watched/listened item (if history is included).
type WatchHistory struct {
	VideoID   string    `json:"video_id"`
	Title     string    `json:"title"`
	Channel   string    `json:"channel"`
	WatchedAt time.Time `json:"watched_at"`
}

// MusicLibrary holds all parsed YouTube Music data.
type MusicLibrary struct {
	Playlists     []Playlist     `json:"playlists"`
	LikedSongs    []LikedSong    `json:"liked_songs"`
	Uploads       []Upload       `json:"uploads"`
	Subscriptions []Subscription `json:"subscriptions"`
	WatchHistory  []WatchHistory `json:"watch_history,omitempty"`
}

// ── Parsing Functions ────────────────────────────────────────────────────────

// ParsePlaylistCSV reads a playlist from YouTube Music CSV export format.
func ParsePlaylistCSV(path string) (*Playlist, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	if len(records) < 2 {
		return &Playlist{}, nil
	}

	// First row is headers: Video ID, Time Added, (possibly more columns)
	playlist := &Playlist{
		Title: strings.TrimSuffix(strings.TrimPrefix(path, "Playlist-"), ".csv"),
	}

	for i, record := range records[1:] {
		if len(record) < 1 {
			continue
		}
		track := Track{
			VideoID:     record[0],
			PlaylistPos: i + 1,
		}
		if len(record) > 1 {
			track.AddedAt, _ = time.Parse(time.RFC3339, record[1])
		}
		if len(record) > 2 {
			track.Title = record[2]
		}
		playlist.Tracks = append(playlist.Tracks, track)
	}

	return playlist, nil
}

// ParseLikedSongsCSV reads liked songs from CSV.
func ParseLikedSongsCSV(path string) ([]LikedSong, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var songs []LikedSong
	for _, record := range records[1:] { // Skip header
		if len(record) < 1 {
			continue
		}
		song := LikedSong{
			Track: Track{VideoID: record[0]},
		}
		if len(record) > 1 {
			song.LikedAt, _ = time.Parse(time.RFC3339, record[1])
			song.AddedAt = song.LikedAt
		}
		if len(record) > 2 {
			song.Title = record[2]
		}
		songs = append(songs, song)
	}
	return songs, nil
}

// ParseSubscriptionsCSV reads subscriptions from CSV.
func ParseSubscriptionsCSV(path string) ([]Subscription, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var subs []Subscription
	for _, record := range records[1:] {
		if len(record) < 3 {
			continue
		}
		sub := Subscription{
			ChannelID:   record[0],
			ChannelURL:  record[1],
			ChannelName: record[2],
		}
		subs = append(subs, sub)
	}
	return subs, nil
}

// ParseWatchHistoryJSON reads watch history from the JSON format.
func ParseWatchHistoryJSON(path string) ([]WatchHistory, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rawHistory []struct {
		Title     string `json:"title"`
		TitleURL  string `json:"titleUrl"`
		Time      string `json:"time"`
		Subtitles []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"subtitles"`
	}

	if err := json.Unmarshal(data, &rawHistory); err != nil {
		return nil, err
	}

	var history []WatchHistory
	for _, item := range rawHistory {
		wh := WatchHistory{
			Title: strings.TrimPrefix(item.Title, "Watched "),
		}

		// Extract video ID from URL
		if strings.Contains(item.TitleURL, "watch?v=") {
			parts := strings.Split(item.TitleURL, "watch?v=")
			if len(parts) > 1 {
				wh.VideoID = strings.Split(parts[1], "&")[0]
			}
		}

		// Parse time
		wh.WatchedAt, _ = time.Parse(time.RFC3339, item.Time)

		// Channel name from subtitles
		if len(item.Subtitles) > 0 {
			wh.Channel = item.Subtitles[0].Name
		}

		history = append(history, wh)
	}

	return history, nil
}

// ── Export Functions ─────────────────────────────────────────────────────────

// ToM3U generates M3U playlist content for a playlist.
func (p *Playlist) ToM3U() string {
	var sb strings.Builder
	sb.WriteString("#EXTM3U\n")
	sb.WriteString("#PLAYLIST:" + p.Title + "\n")

	for _, track := range p.Tracks {
		// #EXTINF:duration,Artist - Title
		duration := track.Duration
		if duration == 0 {
			duration = -1
		}
		title := track.Title
		if track.Artist != "" {
			title = track.Artist + " - " + track.Title
		}
		sb.WriteString("#EXTINF:" + strconv.Itoa(duration) + "," + title + "\n")

		// YouTube URL as fallback
		sb.WriteString("https://music.youtube.com/watch?v=" + track.VideoID + "\n")
	}

	return sb.String()
}

// ToCSV generates CSV content for a playlist.
func (p *Playlist) ToCSV() string {
	var sb strings.Builder
	sb.WriteString("position,video_id,title,artist,album,duration,added_at\n")

	for _, track := range p.Tracks {
		sb.WriteString(strconv.Itoa(track.PlaylistPos) + ",")
		sb.WriteString(escapeCSV(track.VideoID) + ",")
		sb.WriteString(escapeCSV(track.Title) + ",")
		sb.WriteString(escapeCSV(track.Artist) + ",")
		sb.WriteString(escapeCSV(track.Album) + ",")
		sb.WriteString(strconv.Itoa(track.Duration) + ",")
		sb.WriteString(track.AddedAt.Format(time.RFC3339) + "\n")
	}

	return sb.String()
}

func escapeCSV(s string) string {
	if strings.ContainsAny(s, ",\"\n") {
		return "\"" + strings.ReplaceAll(s, "\"", "\"\"") + "\""
	}
	return s
}
