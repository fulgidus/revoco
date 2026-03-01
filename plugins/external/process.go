// Package external provides the external plugin runtime for revoco.
package external

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// Process Manager
// ══════════════════════════════════════════════════════════════════════════════

// ProcessManager manages external plugin processes.
type ProcessManager struct {
	mu        sync.RWMutex
	processes map[string]*Process

	// Default settings
	startTimeout    time.Duration
	shutdownTimeout time.Duration
}

// NewProcessManager creates a new process manager.
func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		processes:       make(map[string]*Process),
		startTimeout:    30 * time.Second,
		shutdownTimeout: 10 * time.Second,
	}
}

// SetStartTimeout sets the timeout for starting processes.
func (pm *ProcessManager) SetStartTimeout(d time.Duration) {
	pm.startTimeout = d
}

// SetShutdownTimeout sets the timeout for shutting down processes.
func (pm *ProcessManager) SetShutdownTimeout(d time.Duration) {
	pm.shutdownTimeout = d
}

// Start launches a new plugin process.
func (pm *ProcessManager) Start(ctx context.Context, pluginID string, command string, args []string, env map[string]string, workDir string) (*Process, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Check if already running
	if proc, ok := pm.processes[pluginID]; ok && proc.IsRunning() {
		return proc, nil
	}

	// Create the process
	proc, err := NewProcess(pluginID, command, args, env, workDir)
	if err != nil {
		return nil, err
	}

	// Start with timeout
	startCtx, cancel := context.WithTimeout(ctx, pm.startTimeout)
	defer cancel()

	if err := proc.Start(startCtx); err != nil {
		return nil, err
	}

	pm.processes[pluginID] = proc
	return proc, nil
}

// Stop terminates a plugin process.
func (pm *ProcessManager) Stop(pluginID string) error {
	pm.mu.Lock()
	proc, ok := pm.processes[pluginID]
	if !ok {
		pm.mu.Unlock()
		return nil
	}
	delete(pm.processes, pluginID)
	pm.mu.Unlock()

	return proc.Stop(pm.shutdownTimeout)
}

// Get returns a running process by plugin ID.
func (pm *ProcessManager) Get(pluginID string) (*Process, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	proc, ok := pm.processes[pluginID]
	if !ok || !proc.IsRunning() {
		return nil, false
	}
	return proc, true
}

// StopAll terminates all plugin processes.
func (pm *ProcessManager) StopAll() error {
	pm.mu.Lock()
	processes := make([]*Process, 0, len(pm.processes))
	for _, proc := range pm.processes {
		processes = append(processes, proc)
	}
	pm.processes = make(map[string]*Process)
	pm.mu.Unlock()

	var errs []error
	for _, proc := range processes {
		if err := proc.Stop(pm.shutdownTimeout); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to stop %d processes", len(errs))
	}
	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Process
// ══════════════════════════════════════════════════════════════════════════════

// Process represents a running external plugin process.
type Process struct {
	pluginID string
	command  string
	args     []string
	env      map[string]string
	workDir  string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	client *Client

	mu       sync.RWMutex
	running  bool
	exitCode int
	exitErr  error

	// Stderr capture for debugging
	stderrBuf *strings.Builder
	stderrMu  sync.Mutex
}

// NewProcess creates a new process (but doesn't start it).
func NewProcess(pluginID string, command string, args []string, env map[string]string, workDir string) (*Process, error) {
	// Resolve command path
	cmdPath, err := exec.LookPath(command)
	if err != nil {
		return nil, fmt.Errorf("command not found: %s: %w", command, err)
	}

	return &Process{
		pluginID:  pluginID,
		command:   cmdPath,
		args:      args,
		env:       env,
		workDir:   workDir,
		stderrBuf: &strings.Builder{},
	}, nil
}

// Start launches the process.
func (p *Process) Start(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.running {
		return nil
	}

	// Build command
	p.cmd = exec.CommandContext(ctx, p.command, p.args...)

	// Set working directory
	if p.workDir != "" {
		p.cmd.Dir = p.workDir
	}

	// Build environment
	p.cmd.Env = os.Environ()
	for k, v := range p.env {
		p.cmd.Env = append(p.cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	// Set up stdio
	var err error
	p.stdin, err = p.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdin pipe: %w", err)
	}

	p.stdout, err = p.cmd.StdoutPipe()
	if err != nil {
		p.stdin.Close()
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	p.stderr, err = p.cmd.StderrPipe()
	if err != nil {
		p.stdin.Close()
		p.stdout.Close()
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the process
	if err := p.cmd.Start(); err != nil {
		p.stdin.Close()
		p.stdout.Close()
		p.stderr.Close()
		return fmt.Errorf("failed to start process: %w", err)
	}

	p.running = true

	// Create JSON-RPC client
	p.client = NewClient(p.stdin, p.stdout)

	// Start stderr reader
	go p.readStderr()

	// Start process monitor
	go p.monitor()

	return nil
}

// Stop terminates the process gracefully.
func (p *Process) Stop(timeout time.Duration) error {
	p.mu.Lock()
	if !p.running {
		p.mu.Unlock()
		return nil
	}
	client := p.client
	p.mu.Unlock()

	// Try graceful shutdown first
	if client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), timeout/2)
		_, _ = client.Call(ctx, MethodShutdown, nil)
		cancel()
		client.Close()
	}

	// Wait for process to exit
	done := make(chan struct{})
	go func() {
		p.cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Process exited gracefully
	case <-time.After(timeout / 2):
		// Force kill
		p.mu.Lock()
		if p.cmd.Process != nil {
			p.cmd.Process.Signal(syscall.SIGTERM)
		}
		p.mu.Unlock()

		select {
		case <-done:
		case <-time.After(timeout / 4):
			// Really force kill
			p.mu.Lock()
			if p.cmd.Process != nil {
				p.cmd.Process.Kill()
			}
			p.mu.Unlock()
			<-done
		}
	}

	p.mu.Lock()
	p.running = false
	if p.stdin != nil {
		p.stdin.Close()
	}
	if p.stdout != nil {
		p.stdout.Close()
	}
	if p.stderr != nil {
		p.stderr.Close()
	}
	p.mu.Unlock()

	return nil
}

// Client returns the JSON-RPC client for this process.
func (p *Process) Client() *Client {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.client
}

// IsRunning returns true if the process is running.
func (p *Process) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// ExitCode returns the exit code (only valid after process exits).
func (p *Process) ExitCode() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exitCode
}

// ExitError returns any error from the process exit.
func (p *Process) ExitError() error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.exitErr
}

// Stderr returns the captured stderr output.
func (p *Process) Stderr() string {
	p.stderrMu.Lock()
	defer p.stderrMu.Unlock()
	return p.stderrBuf.String()
}

// PID returns the process ID.
func (p *Process) PID() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return p.cmd.Process.Pid
	}
	return 0
}

// readStderr reads stderr and buffers it for debugging.
func (p *Process) readStderr() {
	scanner := bufio.NewScanner(p.stderr)
	for scanner.Scan() {
		line := scanner.Text()
		p.stderrMu.Lock()
		// Limit stderr buffer size
		if p.stderrBuf.Len() < 1024*1024 { // 1MB limit
			p.stderrBuf.WriteString(line)
			p.stderrBuf.WriteString("\n")
		}
		p.stderrMu.Unlock()
	}
}

// monitor watches for process exit.
func (p *Process) monitor() {
	err := p.cmd.Wait()

	p.mu.Lock()
	p.running = false
	p.exitErr = err

	if exitErr, ok := err.(*exec.ExitError); ok {
		p.exitCode = exitErr.ExitCode()
	} else if err == nil {
		p.exitCode = 0
	} else {
		p.exitCode = -1
	}

	if p.client != nil {
		p.client.Close()
	}
	p.mu.Unlock()
}

// ══════════════════════════════════════════════════════════════════════════════
// Setup Command Execution
// ══════════════════════════════════════════════════════════════════════════════

// SetupResult contains the result of running a setup command.
type SetupResult struct {
	Success  bool
	Output   string
	ExitCode int
	Error    error
}

// RunSetupCommand runs a setup command for a plugin (e.g., pip install, npm install).
func RunSetupCommand(ctx context.Context, workDir string, command string, args []string) (*SetupResult, error) {
	cmd := exec.CommandContext(ctx, command, args...)
	if workDir != "" {
		cmd.Dir = workDir
	}

	output, err := cmd.CombinedOutput()

	result := &SetupResult{
		Output: string(output),
	}

	if err != nil {
		result.Error = err
		if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
		} else {
			result.ExitCode = -1
		}
		return result, nil
	}

	result.Success = true
	result.ExitCode = 0
	return result, nil
}

// RunPluginSetup runs the setup commands defined in a plugin manifest.
func RunPluginSetup(ctx context.Context, pluginDir string, setupCommands [][]string) error {
	for _, cmdParts := range setupCommands {
		if len(cmdParts) == 0 {
			continue
		}

		command := cmdParts[0]
		var args []string
		if len(cmdParts) > 1 {
			args = cmdParts[1:]
		}

		result, err := RunSetupCommand(ctx, pluginDir, command, args)
		if err != nil {
			return fmt.Errorf("failed to run setup command %q: %w", command, err)
		}

		if !result.Success {
			return fmt.Errorf("setup command %q failed (exit %d): %s", command, result.ExitCode, result.Output)
		}
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Plugin Discovery
// ══════════════════════════════════════════════════════════════════════════════

// PluginManifest describes an external plugin.
type PluginManifest struct {
	// Basic info
	ID          string `json:"id" yaml:"id"`
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	Version     string `json:"version" yaml:"version"`
	Type        string `json:"type" yaml:"type"` // connector, processor, output

	// Execution
	Command string   `json:"command" yaml:"command"` // The command to run
	Args    []string `json:"args" yaml:"args"`       // Arguments to the command

	// Setup
	SetupCommands [][]string `json:"setupCommands" yaml:"setupCommands"` // Commands to run on first setup

	// Dependencies
	BinaryDeps []BinaryDep `json:"binaryDeps" yaml:"binaryDeps"` // Required binaries

	// Configuration
	ConfigOptions []ConfigOption `json:"configOptions" yaml:"configOptions"`

	// Capabilities (for connectors)
	Capabilities []string `json:"capabilities" yaml:"capabilities"`
	DataTypes    []string `json:"dataTypes" yaml:"dataTypes"`

	// Auth (for connectors)
	RequiresAuth bool   `json:"requiresAuth" yaml:"requiresAuth"`
	AuthType     string `json:"authType" yaml:"authType"`

	// Selector (for processors)
	Selector string `json:"selector" yaml:"selector"`
}

// BinaryDep describes a required binary dependency.
type BinaryDep struct {
	Name            string            `json:"name" yaml:"name"`
	Command         string            `json:"command" yaml:"command"`
	VersionFlag     string            `json:"versionFlag" yaml:"versionFlag"`
	MinVersion      string            `json:"minVersion" yaml:"minVersion"`
	InstallCommands map[string]string `json:"installCommands" yaml:"installCommands"` // os -> command
}

// FindPluginManifest looks for a plugin manifest in a directory.
func FindPluginManifest(pluginDir string) (string, error) {
	candidates := []string{
		"plugin.json",
		"plugin.yaml",
		"plugin.yml",
	}

	for _, name := range candidates {
		path := filepath.Join(pluginDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("no plugin manifest found in %s", pluginDir)
}
