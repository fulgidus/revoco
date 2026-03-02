package tui

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/session"
)

// ── Messages ─────────────────────────────────────────────────────────────────

type pushProgressMsg struct {
	connectorID string
	done        int
	total       int
}

type pushCompleteMsg struct {
	connectorID string
	pushed      int
	err         error
}

type pushAllCompleteMsg struct{}

// ── Model ────────────────────────────────────────────────────────────────────

// PushModel is the output-to-destinations screen.
type PushModel struct {
	session *session.Session
	width   int
	height  int
	err     error

	// Output connectors
	outputs []pushConnectorState

	// State
	running  bool
	complete bool
	spinner  spinner.Model
}

type pushConnectorState struct {
	config   core.ConnectorConfig
	done     int
	total    int
	pushed   int
	complete bool
	err      error
}

// NewPushModel creates the push/output screen.
func NewPushModel(sess *session.Session) PushModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	m := PushModel{
		session: sess,
		spinner: sp,
	}

	for _, cfg := range sess.GetOutputConnectors() {
		m.outputs = append(m.outputs, pushConnectorState{
			config: cfg,
		})
	}

	return m
}

// Init implements tea.Model.
func (m PushModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model.
func (m PushModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case pushProgressMsg:
		for i := range m.outputs {
			if m.outputs[i].config.InstanceID == msg.connectorID {
				m.outputs[i].done = msg.done
				m.outputs[i].total = msg.total
				break
			}
		}
		return m, nil

	case pushCompleteMsg:
		for i := range m.outputs {
			if m.outputs[i].config.InstanceID == msg.connectorID {
				m.outputs[i].complete = true
				m.outputs[i].pushed = msg.pushed
				m.outputs[i].err = msg.err
				break
			}
		}
		// Check if all complete
		allDone := true
		for _, cs := range m.outputs {
			if !cs.complete {
				allDone = false
				break
			}
		}
		if allDone {
			return m, func() tea.Msg {
				return pushAllCompleteMsg{}
			}
		}
		return m, nil

	case pushAllCompleteMsg:
		m.running = false
		m.complete = true
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if !m.running && !m.complete {
				m.running = true
				return m, m.startPush()
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

func (m PushModel) startPush() tea.Cmd {
	sess := m.session
	outputs := m.outputs

	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
		defer cancel()

		// Get processed data from session's output directory
		outputDir := sess.OutputPath()
		var items []core.DataItem

		// Scan output directory for files to push
		_ = filepath.WalkDir(outputDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}

			relPath, _ := filepath.Rel(outputDir, path)
			info, _ := d.Info()

			items = append(items, core.DataItem{
				ID:   relPath,
				Path: path,
				Type: detectDataTypeFromPath(path),
				Size: info.Size(),
			})
			return nil
		})

		if len(items) == 0 {
			return pushAllCompleteMsg{}
		}

		var wg sync.WaitGroup
		var mu sync.Mutex
		results := make(map[string]int)

		// Push to each output connector
		for _, outState := range outputs {
			wg.Add(1)
			go func(cfg core.ConnectorConfig) {
				defer wg.Done()

				conn, err := core.CreateConnector(cfg.ConnectorID)
				if err != nil {
					return
				}

				writer, ok := conn.(core.ConnectorWriter)
				if !ok {
					return
				}

				if err := writer.Initialize(ctx, cfg); err != nil {
					return
				}
				defer writer.Close()

				pushed := 0
				for _, item := range items {
					select {
					case <-ctx.Done():
						return
					default:
					}

					// Open the source file
					f, err := os.Open(item.Path)
					if err != nil {
						continue
					}

					err = writer.Write(ctx, item, f)
					f.Close()

					if err == nil {
						pushed++
					}
				}

				mu.Lock()
				results[cfg.InstanceID] = pushed
				mu.Unlock()
			}(outState.config)
		}

		wg.Wait()

		return pushAllCompleteMsg{}
	}
}

// detectDataTypeFromPath determines data type from file extension
func detectDataTypeFromPath(path string) core.DataType {
	ext := strings.ToLower(filepath.Ext(path))

	photoExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true,
		".webp": true, ".heic": true, ".heif": true, ".raw": true,
	}
	if photoExts[ext] {
		return core.DataTypePhoto
	}

	videoExts := map[string]bool{
		".mp4": true, ".mov": true, ".avi": true, ".mkv": true,
		".webm": true, ".m4v": true,
	}
	if videoExts[ext] {
		return core.DataTypeVideo
	}

	audioExts := map[string]bool{
		".mp3": true, ".m4a": true, ".wav": true, ".flac": true,
		".aac": true, ".ogg": true,
	}
	if audioExts[ext] {
		return core.DataTypeAudio
	}

	return core.DataTypeUnknown
}

// View implements tea.Model.
func (m PushModel) View() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Push to Outputs"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Sending processed data to output connectors"))
	sb.WriteString("\n\n")

	if m.err != nil {
		sb.WriteString(dangerStyle.Render("Error: " + m.err.Error()))
		sb.WriteString("\n\n")
	}

	if len(m.outputs) == 0 {
		sb.WriteString(descStyle.Render("No output connectors configured"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("esc back"))
		return sb.String()
	}

	// Show each output connector
	for _, os := range m.outputs {
		sb.WriteString(labelStyle.Render(os.config.Name))
		sb.WriteString("\n")

		if os.err != nil {
			sb.WriteString(dangerStyle.Render("  Error: " + os.err.Error()))
		} else if os.complete {
			sb.WriteString(successStyle.Render(fmt.Sprintf("  Complete (%d items pushed)", os.pushed)))
		} else if m.running {
			sb.WriteString(m.spinner.View())
			sb.WriteString(" ")
			if os.total > 0 {
				pct := float64(os.done) / float64(os.total) * 100
				sb.WriteString(fmt.Sprintf("%.0f%% (%d/%d)", pct, os.done, os.total))
			} else {
				sb.WriteString("Preparing...")
			}
		} else {
			sb.WriteString(descStyle.Render("  Ready"))
		}
		sb.WriteString("\n\n")
	}

	// Actions
	if m.complete {
		sb.WriteString(successStyle.Render("Push complete!"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("esc back to dashboard"))
	} else if m.running {
		sb.WriteString(helpStyle.Render("Pushing data..."))
	} else {
		sb.WriteString(helpStyle.Render("enter start push  esc cancel"))
	}

	return sb.String()
}
