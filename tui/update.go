// Package tui provides a Bubble Tea TUI for revoco.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/fulgidus/revoco/cmd"
	"github.com/fulgidus/revoco/plugins"
)

// ══════════════════════════════════════════════════════════════════════════════
// Update Check Messages
// ══════════════════════════════════════════════════════════════════════════════

// UpdateCheckMsg is sent when update check completes.
type UpdateCheckMsg struct {
	// RevocoUpdate is the new version if available, empty if up-to-date
	RevocoUpdate string
	// PluginUpdates contains plugin IDs that have updates available
	PluginUpdates []PluginUpdateInfo
	// Error is set if the check failed
	Error error
}

// PluginUpdateInfo holds update info for a single plugin.
type PluginUpdateInfo struct {
	ID             string
	CurrentVersion string
	LatestVersion  string
}

// SelfUpdateStartMsg is sent when self-update begins.
type SelfUpdateStartMsg struct{}

// SelfUpdateProgressMsg is sent during self-update.
type SelfUpdateProgressMsg struct {
	Stage   string // "downloading", "verifying", "installing"
	Percent int    // 0-100
}

// SelfUpdateCompleteMsg is sent when self-update finishes.
type SelfUpdateCompleteMsg struct {
	Success    bool
	NewVersion string
	Error      error
}

// ══════════════════════════════════════════════════════════════════════════════
// Update Check Commands
// ══════════════════════════════════════════════════════════════════════════════

// CheckForUpdatesCmd returns a command that checks for both revoco and plugin updates.
func CheckForUpdatesCmd() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		msg := UpdateCheckMsg{}

		// Check for revoco updates
		revocoUpdate, err := checkRevocoUpdate(ctx)
		if err != nil {
			// Don't fail entirely, just note the error
			msg.Error = fmt.Errorf("revoco update check: %w", err)
		} else {
			msg.RevocoUpdate = revocoUpdate
		}

		// Check for plugin updates
		pluginUpdates, err := checkPluginUpdates(ctx)
		if err != nil {
			if msg.Error != nil {
				msg.Error = fmt.Errorf("%v; plugin update check: %w", msg.Error, err)
			} else {
				msg.Error = fmt.Errorf("plugin update check: %w", err)
			}
		} else {
			msg.PluginUpdates = pluginUpdates
		}

		return msg
	}
}

// checkRevocoUpdate checks GitHub for a newer version of revoco.
// Returns empty string if up-to-date, or the new version string if available.
func checkRevocoUpdate(ctx context.Context) (string, error) {
	const (
		githubAPI   = "https://api.github.com"
		githubOwner = "fulgidus"
		githubRepo  = "revoco"
	)

	url := fmt.Sprintf("%s/repos/%s/%s/releases/latest", githubAPI, githubOwner, githubRepo)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github.v3+json")
	req.Header.Set("User-Agent", "revoco-tui")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("no releases found")
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse release: %w", err)
	}

	currentVersion := cmd.GetVersion()
	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentClean := strings.TrimPrefix(currentVersion, "v")

	if currentClean == latestVersion || currentClean == "dev" {
		return "", nil // Up to date or dev version
	}

	return latestVersion, nil
}

// checkPluginUpdates checks for available plugin updates.
func checkPluginUpdates(ctx context.Context) ([]PluginUpdateInfo, error) {
	installer := plugins.NewInstaller()
	updates, err := installer.CheckUpdates(ctx)
	if err != nil {
		return nil, err
	}

	var result []PluginUpdateInfo
	for _, u := range updates {
		result = append(result, PluginUpdateInfo{
			ID:             u.ID,
			CurrentVersion: u.CurrentVersion,
			LatestVersion:  u.LatestVersion,
		})
	}

	return result, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Update State
// ══════════════════════════════════════════════════════════════════════════════

// UpdateState holds the current state of update information for the TUI.
type UpdateState struct {
	// Checking is true while an update check is in progress
	Checking bool

	// RevocoUpdateAvailable is the new version if available
	RevocoUpdateAvailable string

	// PluginUpdatesAvailable holds plugins with available updates
	PluginUpdatesAvailable []PluginUpdateInfo

	// CheckError holds any error from the last check
	CheckError error

	// LastChecked is when the last check was performed
	LastChecked time.Time

	// Updating is true while a self-update is in progress
	Updating bool

	// UpdateStage describes the current update stage
	UpdateStage string

	// UpdateProgress is 0-100 during update
	UpdateProgress int
}

// HasUpdates returns true if any updates are available.
func (s *UpdateState) HasUpdates() bool {
	return s.RevocoUpdateAvailable != "" || len(s.PluginUpdatesAvailable) > 0
}

// UpdateBadge returns a short string for display in the header.
func (s *UpdateState) UpdateBadge() string {
	if s.Checking {
		return "[checking...]"
	}
	if s.Updating {
		return fmt.Sprintf("[updating %d%%]", s.UpdateProgress)
	}
	if s.RevocoUpdateAvailable != "" {
		return fmt.Sprintf("[v%s available]", s.RevocoUpdateAvailable)
	}
	if len(s.PluginUpdatesAvailable) > 0 {
		return fmt.Sprintf("[%d plugin updates]", len(s.PluginUpdatesAvailable))
	}
	return ""
}

// StatusLine returns a longer status line for display.
func (s *UpdateState) StatusLine() string {
	if s.Checking {
		return "Checking for updates..."
	}
	if s.Updating {
		return fmt.Sprintf("Updating: %s (%d%%)", s.UpdateStage, s.UpdateProgress)
	}
	if s.CheckError != nil {
		return fmt.Sprintf("Update check failed: %v", s.CheckError)
	}

	var parts []string
	if s.RevocoUpdateAvailable != "" {
		parts = append(parts, fmt.Sprintf("revoco v%s available (press 'u' to update)", s.RevocoUpdateAvailable))
	}
	if len(s.PluginUpdatesAvailable) > 0 {
		ids := make([]string, len(s.PluginUpdatesAvailable))
		for i, p := range s.PluginUpdatesAvailable {
			ids[i] = p.ID
		}
		parts = append(parts, fmt.Sprintf("plugin updates: %s", strings.Join(ids, ", ")))
	}

	if len(parts) > 0 {
		return strings.Join(parts, " | ")
	}
	return ""
}

// ══════════════════════════════════════════════════════════════════════════════
// Update Confirmation Model
// ══════════════════════════════════════════════════════════════════════════════

// UpdateConfirmModel is a modal for confirming self-update.
type UpdateConfirmModel struct {
	currentVersion string
	newVersion     string
	confirmed      bool
	cancelled      bool
	width          int
	height         int
}

// NewUpdateConfirmModel creates a new update confirmation modal.
func NewUpdateConfirmModel(currentVersion, newVersion string) UpdateConfirmModel {
	return UpdateConfirmModel{
		currentVersion: currentVersion,
		newVersion:     newVersion,
	}
}

// Init implements tea.Model.
func (m UpdateConfirmModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m UpdateConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "y", "Y", "enter":
			m.confirmed = true
			return m, nil
		case "n", "N", "esc", "q":
			m.cancelled = true
			return m, nil
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

// View implements tea.Model.
func (m UpdateConfirmModel) View() string {
	return fmt.Sprintf(`
  Update revoco?

  Current version: v%s
  New version:     v%s

  [y]es  [n]o
`, m.currentVersion, m.newVersion)
}

// IsConfirmed returns true if user confirmed the update.
func (m UpdateConfirmModel) IsConfirmed() bool {
	return m.confirmed
}

// IsCancelled returns true if user cancelled.
func (m UpdateConfirmModel) IsCancelled() bool {
	return m.cancelled
}
