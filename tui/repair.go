package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/session"
)

// ── Messages ─────────────────────────────────────────────────────────────────

type repairProgressMsg struct {
	connectorID string
	done        int
	total       int
}

type repairCompleteMsg struct {
	repaired int
	failed   int
	err      error
}

// ── Model ────────────────────────────────────────────────────────────────────

// RepairModel is the fallback-based repair screen.
type RepairModel struct {
	session *session.Session
	width   int
	height  int
	err     error

	// Fallback connectors available
	fallbacks []core.ConnectorConfig

	// Repair state
	running  bool
	complete bool
	repaired int
	failed   int
	done     int
	total    int
	spinner  spinner.Model
}

// NewRepairModel creates the repair screen.
func NewRepairModel(sess *session.Session) RepairModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	return RepairModel{
		session:   sess,
		fallbacks: sess.GetFallbackConnectors(),
		spinner:   sp,
	}
}

// Init implements tea.Model.
func (m RepairModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model.
func (m RepairModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case repairProgressMsg:
		m.done = msg.done
		m.total = msg.total
		return m, nil

	case repairCompleteMsg:
		m.running = false
		m.complete = true
		m.repaired = msg.repaired
		m.failed = msg.failed
		if msg.err != nil {
			m.err = msg.err
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if !m.running && !m.complete {
				m.running = true
				return m, m.startRepair()
			}
		case "esc", "q":
			if !m.running {
				return m, func() tea.Msg {
					return SwitchScreenMsg{
						To:      ScreenDashboard,
						Session: m.session,
					}
				}
			}
		}
	}

	return m, nil
}

func (m RepairModel) startRepair() tea.Cmd {
	sess := m.session
	fallbacks := m.fallbacks

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		// Load missing items from the session's missing-files.json
		missingPath := filepath.Join(sess.Dir, sess.Config.Recover.InputJSON)
		missingData, err := os.ReadFile(missingPath)
		if err != nil {
			// No missing file, nothing to repair
			return repairCompleteMsg{repaired: 0, failed: 0, err: nil}
		}

		var missingItems []core.DataItem
		if err := json.Unmarshal(missingData, &missingItems); err != nil {
			return repairCompleteMsg{repaired: 0, failed: 0, err: fmt.Errorf("parse missing items: %w", err)}
		}

		if len(missingItems) == 0 {
			return repairCompleteMsg{repaired: 0, failed: 0}
		}

		repaired := 0
		failed := 0
		recoveredDir := filepath.Join(sess.Dir, sess.Config.Recover.OutputDir)

		// Try each fallback connector
		for _, fbCfg := range fallbacks {
			conn, err := core.CreateConnector(fbCfg.ConnectorID)
			if err != nil {
				continue
			}

			// Check if it supports repair or at least reading
			repairer, isRepairer := conn.(core.ConnectorRepairer)
			reader, isReader := conn.(core.ConnectorReader)

			if !isRepairer && !isReader {
				continue
			}

			// Initialize
			if isRepairer {
				if err := repairer.Initialize(ctx, fbCfg); err != nil {
					continue
				}
				defer repairer.Close()
			} else if isReader {
				if err := reader.Initialize(ctx, fbCfg); err != nil {
					continue
				}
				defer reader.Close()
			}

			// Try to repair each missing item
			for i := range missingItems {
				item := &missingItems[i]
				if item.Path != "" {
					// Already repaired
					continue
				}

				select {
				case <-ctx.Done():
					return repairCompleteMsg{repaired: repaired, failed: failed, err: ctx.Err()}
				default:
				}

				// Use item's original path for proper filename, fallback to ID if empty
				itemPath := item.Path
				if itemPath == "" {
					itemPath = item.ID
				}
				destPath := filepath.Join(recoveredDir, itemPath)

				if isRepairer {
					// Use dedicated repair method
					canRepair, err := repairer.CanRepair(ctx, *item)
					if err != nil || !canRepair {
						continue
					}

					if err := repairer.Repair(ctx, *item, destPath); err != nil {
						failed++
						continue
					}
				} else if isReader {
					// Fall back to ReadTo
					if err := reader.ReadTo(ctx, *item, destPath, core.ImportModeCopy); err != nil {
						failed++
						continue
					}
				}

				item.Path = destPath
				repaired++
			}
		}

		// Update stats
		if sess.Config.Connectors.Stats != nil {
			sess.Config.Connectors.Stats.Missing = len(missingItems) - repaired
		}

		// Save repaired items back
		repairedData, _ := json.MarshalIndent(missingItems, "", "  ")
		_ = os.WriteFile(missingPath, repairedData, 0o644)

		_ = sess.Save()

		return repairCompleteMsg{repaired: repaired, failed: failed}
	}
}

// View implements tea.Model.
func (m RepairModel) View() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Repair Missing Data"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Using fallback connectors to recover missing items"))
	sb.WriteString("\n\n")

	if m.err != nil {
		sb.WriteString(dangerStyle.Render("Error: " + m.err.Error()))
		sb.WriteString("\n\n")
	}

	// Show fallback connectors
	sb.WriteString(labelStyle.Render("Available Fallbacks"))
	sb.WriteString("\n")
	if len(m.fallbacks) == 0 {
		sb.WriteString(descStyle.Render("No fallback connectors configured"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("esc back"))
		return sb.String()
	}

	for _, fb := range m.fallbacks {
		sb.WriteString(fmt.Sprintf("  - %s (%s)\n", fb.Name, fb.ConnectorID))
	}
	sb.WriteString("\n")

	// Show missing items count
	stats := m.session.Config.Connectors.Stats
	if stats != nil && stats.Missing > 0 {
		sb.WriteString(fmt.Sprintf("Missing items: %d\n", stats.Missing))
		sb.WriteString(fmt.Sprintf("Repairable:    %d\n", stats.Repairable))
		sb.WriteString("\n")
	} else {
		sb.WriteString(descStyle.Render("No missing items detected (run Analyze first)"))
		sb.WriteString("\n\n")
	}

	// Progress
	if m.running {
		sb.WriteString(m.spinner.View())
		sb.WriteString(" ")
		if m.total > 0 {
			pct := float64(m.done) / float64(m.total) * 100
			sb.WriteString(fmt.Sprintf("Repairing... %.0f%% (%d/%d)", pct, m.done, m.total))
		} else {
			sb.WriteString("Starting repair...")
		}
		sb.WriteString("\n\n")
	}

	// Results
	if m.complete {
		sb.WriteString(successStyle.Render(fmt.Sprintf("Repair complete! Repaired: %d, Failed: %d", m.repaired, m.failed)))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("esc back to dashboard"))
	} else if !m.running {
		sb.WriteString(helpStyle.Render("enter start repair  esc cancel"))
	}

	return sb.String()
}
