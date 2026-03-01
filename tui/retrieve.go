package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	core "github.com/fulgidus/revoco/connectors"
	_ "github.com/fulgidus/revoco/connectors/googledrive" // Register Google Drive connector
	_ "github.com/fulgidus/revoco/connectors/local"       // Register local connectors
	"github.com/fulgidus/revoco/session"
)

// ── Messages ─────────────────────────────────────────────────────────────────

type retrieveProgressMsg struct {
	connectorID string
	done        int
	total       int
}

type retrieveCompleteMsg struct {
	connectorID string
	items       int
	err         error
}

type retrieveAllCompleteMsg struct {
	items     int
	errors    []error
	cancelled bool
}

// ── Model ────────────────────────────────────────────────────────────────────

// RetrieveModel is the data retrieval progress screen.
type RetrieveModel struct {
	session *session.Session
	width   int
	height  int
	err     error

	// Retrieval state per connector
	connectors []retrieveConnectorState

	// Overall state
	running        bool
	complete       bool
	cancelled      bool
	retrievedCount int
	spinner        spinner.Model

	// Cancellation - using pointer so it persists across value copies
	cancelFunc *context.CancelFunc
}

type retrieveConnectorState struct {
	config   core.ConnectorConfig
	done     int
	total    int
	complete bool
	err      error
}

// NewRetrieveModel creates the retrieval progress screen.
func NewRetrieveModel(sess *session.Session) RetrieveModel {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("205"))

	// Pre-allocate cancel func pointer for sharing across value copies
	var cancelFunc context.CancelFunc

	m := RetrieveModel{
		session:    sess,
		spinner:    sp,
		cancelFunc: &cancelFunc,
	}

	// Initialize connector states
	for _, cfg := range sess.GetInputConnectors() {
		m.connectors = append(m.connectors, retrieveConnectorState{
			config: cfg,
		})
	}

	return m
}

// Init implements tea.Model.
func (m RetrieveModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model.
func (m RetrieveModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case retrieveProgressMsg:
		for i := range m.connectors {
			if m.connectors[i].config.InstanceID == msg.connectorID {
				m.connectors[i].done = msg.done
				m.connectors[i].total = msg.total
				break
			}
		}
		return m, nil

	case retrieveCompleteMsg:
		for i := range m.connectors {
			if m.connectors[i].config.InstanceID == msg.connectorID {
				m.connectors[i].complete = true
				m.connectors[i].err = msg.err
				break
			}
		}
		// Check if all complete
		allDone := true
		for _, cs := range m.connectors {
			if !cs.complete {
				allDone = false
				break
			}
		}
		if allDone {
			return m, func() tea.Msg {
				return retrieveAllCompleteMsg{items: 0}
			}
		}
		return m, nil

	case retrieveAllCompleteMsg:
		m.running = false
		m.complete = true
		m.cancelled = msg.cancelled
		m.retrievedCount = msg.items
		if msg.cancelled {
			m.err = fmt.Errorf("retrieval cancelled by user")
		} else if len(msg.errors) > 0 {
			// Combine errors into a summary
			errSummary := fmt.Sprintf("%d error(s) during retrieval", len(msg.errors))
			for _, e := range msg.errors {
				errSummary += "\n  - " + e.Error()
			}
			m.err = fmt.Errorf("%s", errSummary)
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "enter":
			if !m.running && !m.complete {
				// Start retrieval
				m.running = true
				return m, m.startRetrieval()
			}
		case "esc", "q":
			if m.running {
				// Cancel running retrieval
				if m.cancelFunc != nil && *m.cancelFunc != nil {
					(*m.cancelFunc)()
				}
				return m, nil
			}
			// Not running - go back to dashboard
			return m, func() tea.Msg {
				return SwitchScreenMsg{
					To:      ScreenDashboard,
					Session: m.session,
				}
			}
		}
	}

	return m, nil
}

func (m RetrieveModel) startRetrieval() tea.Cmd {
	sess := m.session
	connectors := m.connectors
	cancelFuncPtr := m.cancelFunc

	// Create cancellable context and store cancel func via pointer
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	if cancelFuncPtr != nil {
		*cancelFuncPtr = cancel
	}

	return func() tea.Msg {
		defer cancel()

		dataDir := sess.DataDir()
		parallelRetrieval := sess.Config.Connectors.ParallelRetrieval

		var wg sync.WaitGroup
		var mu sync.Mutex
		var allItems []core.DataItem
		var errors []error
		var cancelled bool

		// Process each input connector
		retrieveFunc := func(cs retrieveConnectorState) {
			defer wg.Done()

			// Check cancellation early
			select {
			case <-ctx.Done():
				cancelled = true
				return
			default:
			}

			// Create connector instance from registry
			conn, err := core.CreateConnector(cs.config.ConnectorID)
			if err != nil {
				mu.Lock()
				errors = append(errors, fmt.Errorf("create connector %s: %w", cs.config.Name, err))
				mu.Unlock()
				return
			}

			// Check if it's a reader
			reader, ok := conn.(core.ConnectorReader)
			if !ok {
				mu.Lock()
				errors = append(errors, fmt.Errorf("connector %s does not support reading", cs.config.Name))
				mu.Unlock()
				return
			}

			// Initialize the connector
			if err := reader.Initialize(ctx, cs.config); err != nil {
				if ctx.Err() != nil {
					cancelled = true
					return
				}
				mu.Lock()
				errors = append(errors, fmt.Errorf("initialize connector %s: %w", cs.config.Name, err))
				mu.Unlock()
				return
			}
			defer reader.Close()

			// List items from connector
			items, err := reader.List(ctx, nil)
			if err != nil {
				if ctx.Err() != nil {
					cancelled = true
					return
				}
				mu.Lock()
				errors = append(errors, fmt.Errorf("list items from %s: %w", cs.config.Name, err))
				mu.Unlock()
				return
			}

			// Retrieve each item
			for i, item := range items {
				select {
				case <-ctx.Done():
					cancelled = true
					return
				default:
				}

				// Determine destination path - use item.Path for proper filename/structure
				// item.Path contains the relative path with folder structure and proper filename
				// item.ID is just an identifier (e.g., Google Drive file ID) which isn't a valid filename
				itemPath := item.Path
				if itemPath == "" {
					// Fallback to ID if Path is empty (shouldn't happen but be safe)
					itemPath = item.ID
				}
				destPath := filepath.Join(dataDir, cs.config.InstanceID, itemPath)

				// Read the item to destination
				if err := reader.ReadTo(ctx, item, destPath, cs.config.ImportMode); err != nil {
					if ctx.Err() != nil {
						cancelled = true
						return
					}
					// Log error but continue
					mu.Lock()
					errors = append(errors, fmt.Errorf("retrieve %s from %s: %w", item.ID, cs.config.Name, err))
					mu.Unlock()
					continue
				}

				// Update item path
				item.Path = destPath

				mu.Lock()
				allItems = append(allItems, item)
				mu.Unlock()

				// Progress is tracked via items count
				_ = i // Progress would be sent via channel in real impl
			}
		}

		if parallelRetrieval {
			// Run all connectors in parallel
			for _, cs := range connectors {
				wg.Add(1)
				go retrieveFunc(cs)
			}
		} else {
			// Run connectors sequentially
			for _, cs := range connectors {
				wg.Add(1)
				retrieveFunc(cs)
			}
		}

		wg.Wait()

		// Check if cancelled
		if ctx.Err() != nil || cancelled {
			return retrieveAllCompleteMsg{
				items:     len(allItems),
				cancelled: true,
			}
		}

		// Update session stats
		if sess.Config.Connectors.Stats == nil {
			sess.Config.Connectors.Stats = &core.DataStats{
				ByType:      make(map[core.DataType]int),
				ByConnector: make(map[string]int),
			}
		}
		stats := sess.Config.Connectors.Stats
		stats.TotalItems = len(allItems)
		for _, item := range allItems {
			stats.ByType[item.Type]++
			stats.ByConnector[item.SourceConnID]++
			stats.TotalSize += item.Size
		}

		// Save session
		_ = sess.Save()

		return retrieveAllCompleteMsg{
			items:  len(allItems),
			errors: errors,
		}
	}
}

// View implements tea.Model.
func (m RetrieveModel) View() string {
	var sb strings.Builder

	sb.WriteString(titleStyle.Render("Data Retrieval"))
	sb.WriteString("\n")
	sb.WriteString(subtitleStyle.Render("Retrieving data from input connectors"))
	sb.WriteString("\n\n")

	if m.err != nil {
		sb.WriteString(dangerStyle.Render("Error: " + m.err.Error()))
		sb.WriteString("\n\n")
	}

	if len(m.connectors) == 0 {
		sb.WriteString(descStyle.Render("No input connectors configured"))
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("esc back"))
		return sb.String()
	}

	// Show each connector's progress
	for _, cs := range m.connectors {
		sb.WriteString(labelStyle.Render(cs.config.Name))
		sb.WriteString("\n")

		if cs.err != nil {
			sb.WriteString(dangerStyle.Render("  Error: " + cs.err.Error()))
		} else if cs.complete {
			sb.WriteString(successStyle.Render("  Complete"))
		} else if m.running {
			sb.WriteString(m.spinner.View())
			sb.WriteString(" ")
			if cs.total > 0 {
				pct := float64(cs.done) / float64(cs.total) * 100
				sb.WriteString(fmt.Sprintf("%.0f%% (%d/%d)", pct, cs.done, cs.total))
			} else {
				sb.WriteString("Scanning...")
			}
		} else {
			sb.WriteString(descStyle.Render("  Ready"))
		}
		sb.WriteString("\n\n")
	}

	// Actions
	if m.complete {
		if m.cancelled {
			sb.WriteString(warningStyle.Render(fmt.Sprintf("Retrieval cancelled. %d items retrieved before cancellation.", m.retrievedCount)))
		} else {
			sb.WriteString(successStyle.Render(fmt.Sprintf("Retrieval complete! %d items retrieved.", m.retrievedCount)))
		}
		sb.WriteString("\n\n")
		sb.WriteString(helpStyle.Render("esc back to dashboard"))
	} else if m.running {
		sb.WriteString(helpStyle.Render("esc/q cancel  •  Retrieving data..."))
	} else {
		sb.WriteString(helpStyle.Render("enter start retrieval  esc cancel"))
	}

	return sb.String()
}
