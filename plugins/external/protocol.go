// Package external provides the external plugin runtime for revoco.
//
// External plugins communicate via JSON-RPC 2.0 over stdin/stdout,
// allowing plugins to be written in any language.
package external

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"sync/atomic"
)

// ══════════════════════════════════════════════════════════════════════════════
// JSON-RPC 2.0 Protocol Types
// ══════════════════════════════════════════════════════════════════════════════

// Request represents a JSON-RPC 2.0 request.
type Request struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"` // nil for notifications
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

// Response represents a JSON-RPC 2.0 response.
type Response struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"`
	Result  any    `json:"result,omitempty"`
	Error   *Error `json:"error,omitempty"`
}

// Error represents a JSON-RPC 2.0 error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Standard JSON-RPC 2.0 error codes
const (
	ErrCodeParse          = -32700 // Invalid JSON
	ErrCodeInvalidRequest = -32600 // Invalid request object
	ErrCodeMethodNotFound = -32601 // Method not found
	ErrCodeInvalidParams  = -32602 // Invalid method parameters
	ErrCodeInternal       = -32603 // Internal error

	// Custom error codes (application-defined, must be -32000 to -32099)
	ErrCodePluginError    = -32000 // Plugin-specific error
	ErrCodeTimeout        = -32001 // Operation timed out
	ErrCodeNotInitialized = -32002 // Plugin not initialized
	ErrCodeAuthRequired   = -32003 // Authentication required
)

// Error implements the error interface.
func (e *Error) Error() string {
	if e.Data != nil {
		return fmt.Sprintf("JSON-RPC error %d: %s (%v)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("JSON-RPC error %d: %s", e.Code, e.Message)
}

// NewError creates a new JSON-RPC error.
func NewError(code int, message string, data any) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// JSON-RPC Client
// ══════════════════════════════════════════════════════════════════════════════

// Client handles JSON-RPC communication with an external process.
type Client struct {
	stdin  io.Writer
	stdout *bufio.Reader

	mu        sync.Mutex
	nextID    atomic.Int64
	pending   map[int]chan *Response
	pendingMu sync.Mutex

	// For receiving notifications from the plugin
	notifications chan *Request

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewClient creates a new JSON-RPC client.
func NewClient(stdin io.Writer, stdout io.Reader) *Client {
	ctx, cancel := context.WithCancel(context.Background())

	c := &Client{
		stdin:         stdin,
		stdout:        bufio.NewReader(stdout),
		pending:       make(map[int]chan *Response),
		notifications: make(chan *Request, 100),
		ctx:           ctx,
		cancel:        cancel,
		done:          make(chan struct{}),
	}

	// Start the reader goroutine
	go c.readLoop()

	return c
}

// Close shuts down the client.
func (c *Client) Close() error {
	c.cancel()
	<-c.done
	return nil
}

// Notifications returns the channel for receiving notifications from the plugin.
func (c *Client) Notifications() <-chan *Request {
	return c.notifications
}

// readLoop continuously reads responses from stdout.
func (c *Client) readLoop() {
	defer close(c.done)
	defer close(c.notifications)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				// Log error but don't crash
			}
			return
		}

		// Try to parse as response first
		var resp Response
		if err := json.Unmarshal(line, &resp); err == nil && resp.JSONRPC == "2.0" {
			if resp.ID != nil {
				// This is a response to a request
				c.pendingMu.Lock()
				if ch, ok := c.pending[*resp.ID]; ok {
					ch <- &resp
					delete(c.pending, *resp.ID)
				}
				c.pendingMu.Unlock()
			}
			continue
		}

		// Try to parse as notification
		var req Request
		if err := json.Unmarshal(line, &req); err == nil && req.JSONRPC == "2.0" && req.ID == nil {
			select {
			case c.notifications <- &req:
			default:
				// Notification channel full, drop
			}
		}
	}
}

// Call sends a request and waits for a response.
func (c *Client) Call(ctx context.Context, method string, params any) (any, error) {
	id := int(c.nextID.Add(1))

	req := Request{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}

	// Create response channel
	respCh := make(chan *Response, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	// Send request
	if err := c.send(&req); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	// Wait for response
	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	}
}

// Notify sends a notification (no response expected).
func (c *Client) Notify(method string, params any) error {
	req := Request{
		JSONRPC: "2.0",
		ID:      nil, // Notifications have no ID
		Method:  method,
		Params:  params,
	}
	return c.send(&req)
}

// send writes a request to stdin.
func (c *Client) send(req *Request) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Write with newline delimiter
	data = append(data, '\n')
	_, err = c.stdin.Write(data)
	return err
}

// ══════════════════════════════════════════════════════════════════════════════
// Helper Functions
// ══════════════════════════════════════════════════════════════════════════════

// CallWithResult calls a method and unmarshals the result into the given target.
func (c *Client) CallWithResult(ctx context.Context, method string, params any, result any) error {
	raw, err := c.Call(ctx, method, params)
	if err != nil {
		return err
	}

	if result == nil {
		return nil
	}

	// Re-marshal and unmarshal to convert to the target type
	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Errorf("failed to marshal result: %w", err)
	}

	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("failed to unmarshal result: %w", err)
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Protocol Methods
// ══════════════════════════════════════════════════════════════════════════════

// These are the standard methods that external plugins must implement.

const (
	// Lifecycle methods
	MethodInitialize = "initialize"
	MethodShutdown   = "shutdown"

	// Info methods
	MethodGetInfo         = "getInfo"
	MethodGetCapabilities = "getCapabilities"
	MethodGetConfigSchema = "getConfigSchema"

	// Connector methods
	MethodList           = "list"
	MethodRead           = "read"
	MethodWrite          = "write"
	MethodDelete         = "delete"
	MethodTestConnection = "testConnection"

	// Processor methods
	MethodCanProcess   = "canProcess"
	MethodProcess      = "process"
	MethodProcessBatch = "processBatch"

	// Output methods
	MethodCanOutput   = "canOutput"
	MethodExport      = "export"
	MethodExportBatch = "exportBatch"
	MethodFinalize    = "finalize"
)

// InitializeParams are the parameters for the initialize method.
type InitializeParams struct {
	Config   map[string]any `json:"config"`
	WorkDir  string         `json:"workDir"`
	CacheDir string         `json:"cacheDir"`
}

// InitializeResult is the result of the initialize method.
type InitializeResult struct {
	Success bool   `json:"success"`
	Message string `json:"message,omitempty"`
}

// PluginInfo is returned by getInfo.
type PluginInfo struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Version      string   `json:"version"`
	Type         string   `json:"type"` // connector, processor, output
	Capabilities []string `json:"capabilities,omitempty"`
	DataTypes    []string `json:"dataTypes,omitempty"`
	RequiresAuth bool     `json:"requiresAuth,omitempty"`
	AuthType     string   `json:"authType,omitempty"`
}

// DataItem represents an item being processed.
type DataItem struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"`
	Path         string         `json:"path"`
	RemoteID     string         `json:"remoteId,omitempty"`
	SourceConnID string         `json:"sourceConnId,omitempty"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	Size         int64          `json:"size,omitempty"`
	Checksum     string         `json:"checksum,omitempty"`
}

// ProcessResult is returned by process methods.
type ProcessResult struct {
	Item    *DataItem `json:"item,omitempty"`
	Skipped bool      `json:"skipped,omitempty"`
	Error   string    `json:"error,omitempty"`
}

// ConfigOption describes a configuration option.
type ConfigOption struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`
	Default     any      `json:"default,omitempty"`
	Options     []string `json:"options,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Sensitive   bool     `json:"sensitive,omitempty"`
}
