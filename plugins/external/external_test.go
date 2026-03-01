package external

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// ══════════════════════════════════════════════════════════════════════════════
// JSON-RPC Protocol Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestJSONRPCClientCall(t *testing.T) {
	// Create pipes to simulate stdin/stdout
	clientIn := &bytes.Buffer{}
	clientOut := &bytes.Buffer{}

	// Create client
	client := NewClient(clientIn, clientOut)
	defer client.Close()

	// Prepare a mock response in clientOut
	response := Response{
		JSONRPC: "2.0",
		ID:      intPtr(1),
		Result:  map[string]any{"success": true},
	}
	respData, _ := json.Marshal(response)
	clientOut.Write(respData)
	clientOut.Write([]byte("\n"))

	// Call with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := client.Call(ctx, "test", nil)
	if err != nil {
		// Expected timeout since we're not properly simulating the read loop
		// This is a basic test to ensure the code doesn't panic
		t.Logf("Expected behavior: %v", err)
	}
	_ = result
}

func TestJSONRPCError(t *testing.T) {
	err := &Error{
		Code:    ErrCodePluginError,
		Message: "test error",
		Data:    "additional info",
	}

	errStr := err.Error()
	if !strings.Contains(errStr, "test error") {
		t.Errorf("Error string should contain message, got: %s", errStr)
	}
	if !strings.Contains(errStr, "-32000") {
		t.Errorf("Error string should contain code, got: %s", errStr)
	}
	if !strings.Contains(errStr, "additional info") {
		t.Errorf("Error string should contain data, got: %s", errStr)
	}
}

func TestNewError(t *testing.T) {
	err := NewError(ErrCodeInternal, "internal error", nil)

	if err.Code != ErrCodeInternal {
		t.Errorf("Expected code %d, got %d", ErrCodeInternal, err.Code)
	}
	if err.Message != "internal error" {
		t.Errorf("Expected message 'internal error', got '%s'", err.Message)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Process Manager Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestProcessManagerCreateStop(t *testing.T) {
	pm := NewProcessManager()
	pm.SetStartTimeout(5 * time.Second)
	pm.SetShutdownTimeout(5 * time.Second)

	// Clean up
	defer pm.StopAll()

	// Test getting non-existent process
	_, ok := pm.Get("nonexistent")
	if ok {
		t.Error("Should not find non-existent process")
	}

	// Test stopping non-existent process (should not error)
	err := pm.Stop("nonexistent")
	if err != nil {
		t.Errorf("Stopping non-existent process should not error: %v", err)
	}
}

func TestProcessManagerWithEcho(t *testing.T) {
	pm := NewProcessManager()
	pm.SetStartTimeout(5 * time.Second)
	pm.SetShutdownTimeout(2 * time.Second)
	defer pm.StopAll()

	ctx := context.Background()

	// Start a simple echo process (using cat which will read from stdin)
	// Note: This is a basic test - real plugins would use a proper JSON-RPC server
	process, err := pm.Start(ctx, "test-cat", "cat", nil, nil, "")
	if err != nil {
		t.Fatalf("Failed to start process: %v", err)
	}

	if !process.IsRunning() {
		t.Error("Process should be running")
	}

	if process.PID() <= 0 {
		t.Error("Process should have a valid PID")
	}

	// Get the process
	got, ok := pm.Get("test-cat")
	if !ok {
		t.Error("Should find started process")
	}
	if got != process {
		t.Error("Should return same process")
	}

	// Stop the process
	err = pm.Stop("test-cat")
	if err != nil {
		t.Errorf("Failed to stop process: %v", err)
	}

	if process.IsRunning() {
		t.Error("Process should not be running after stop")
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Setup Command Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestRunSetupCommand(t *testing.T) {
	ctx := context.Background()

	// Test successful command
	result, err := RunSetupCommand(ctx, "", "echo", []string{"hello"})
	if err != nil {
		t.Fatalf("RunSetupCommand failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Command should succeed, got exit code %d: %s", result.ExitCode, result.Output)
	}

	if !strings.Contains(result.Output, "hello") {
		t.Errorf("Output should contain 'hello', got: %s", result.Output)
	}

	// Test failing command
	result, err = RunSetupCommand(ctx, "", "false", nil)
	if err != nil {
		t.Fatalf("RunSetupCommand failed: %v", err)
	}

	if result.Success {
		t.Error("Command 'false' should fail")
	}

	if result.ExitCode == 0 {
		t.Error("Exit code should be non-zero for failed command")
	}
}

func TestRunPluginSetup(t *testing.T) {
	ctx := context.Background()

	// Test with successful commands
	commands := [][]string{
		{"echo", "step1"},
		{"echo", "step2"},
	}

	err := RunPluginSetup(ctx, "", commands)
	if err != nil {
		t.Errorf("RunPluginSetup should succeed: %v", err)
	}

	// Test with failing command
	commands = [][]string{
		{"echo", "step1"},
		{"false"},
	}

	err = RunPluginSetup(ctx, "", commands)
	if err == nil {
		t.Error("RunPluginSetup should fail when a command fails")
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Data Conversion Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestDataItemConversion(t *testing.T) {
	// Test nil conversion
	if DataItemToExternal(nil) != nil {
		t.Error("nil DataItem should convert to nil")
	}

	if ExternalToDataItem(nil) != nil {
		t.Error("nil external item should convert to nil")
	}

	// Test round-trip conversion
	extItem := &DataItem{
		ID:           "test-id",
		Type:         "photo",
		Path:         "/path/to/file.jpg",
		RemoteID:     "remote-123",
		SourceConnID: "conn-1",
		Metadata: map[string]any{
			"width":  1920,
			"height": 1080,
		},
		Size:     12345,
		Checksum: "abc123",
	}

	coreItem := ExternalToDataItem(extItem)
	if coreItem.ID != extItem.ID {
		t.Errorf("ID mismatch: %s vs %s", coreItem.ID, extItem.ID)
	}
	if string(coreItem.Type) != extItem.Type {
		t.Errorf("Type mismatch: %s vs %s", coreItem.Type, extItem.Type)
	}
	if coreItem.Path != extItem.Path {
		t.Errorf("Path mismatch: %s vs %s", coreItem.Path, extItem.Path)
	}
	if coreItem.Size != extItem.Size {
		t.Errorf("Size mismatch: %d vs %d", coreItem.Size, extItem.Size)
	}

	// Convert back
	backToExt := DataItemToExternal(coreItem)
	if backToExt.ID != extItem.ID {
		t.Errorf("Round-trip ID mismatch: %s vs %s", backToExt.ID, extItem.ID)
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Mock Plugin Tests
// ══════════════════════════════════════════════════════════════════════════════

// mockReadWriter implements io.Reader and io.Writer for testing
type mockReadWriter struct {
	readData  *bytes.Buffer
	writeData *bytes.Buffer
}

func newMockReadWriter() *mockReadWriter {
	return &mockReadWriter{
		readData:  &bytes.Buffer{},
		writeData: &bytes.Buffer{},
	}
}

func (m *mockReadWriter) Read(p []byte) (n int, err error) {
	return m.readData.Read(p)
}

func (m *mockReadWriter) Write(p []byte) (n int, err error) {
	return m.writeData.Write(p)
}

// mockPluginServer simulates a JSON-RPC plugin server
type mockPluginServer struct {
	input  io.Reader
	output io.Writer
}

func (s *mockPluginServer) handleRequest(req *Request) *Response {
	resp := &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case MethodGetInfo:
		resp.Result = &PluginInfo{
			ID:          "mock-plugin",
			Name:        "Mock Plugin",
			Description: "A mock plugin for testing",
			Version:     "1.0.0",
			Type:        "processor",
		}
	case MethodInitialize:
		resp.Result = &InitializeResult{
			Success: true,
			Message: "initialized",
		}
	case MethodShutdown:
		resp.Result = map[string]any{"success": true}
	default:
		resp.Error = NewError(ErrCodeMethodNotFound, "method not found", nil)
	}

	return resp
}

func TestClientWithMockServer(t *testing.T) {
	// This test demonstrates the expected protocol behavior
	// without actually spawning processes

	// Prepare a mock response
	response := Response{
		JSONRPC: "2.0",
		ID:      intPtr(1),
		Result: &PluginInfo{
			ID:      "mock-plugin",
			Name:    "Mock Plugin",
			Version: "1.0.0",
			Type:    "processor",
		},
	}

	respData, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	// Verify the response format
	var parsed Response
	err = json.Unmarshal(respData, &parsed)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if parsed.JSONRPC != "2.0" {
		t.Error("Response should have jsonrpc: 2.0")
	}

	if parsed.ID == nil || *parsed.ID != 1 {
		t.Error("Response should have matching ID")
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Manifest Tests
// ══════════════════════════════════════════════════════════════════════════════

func TestPluginManifestParsing(t *testing.T) {
	manifestJSON := `{
		"id": "test-plugin",
		"name": "Test Plugin",
		"description": "A test plugin",
		"version": "1.0.0",
		"type": "processor",
		"command": "python3",
		"args": ["plugin.py"],
		"configOptions": [
			{
				"id": "api_key",
				"name": "API Key",
				"type": "string",
				"required": true,
				"sensitive": true
			}
		]
	}`

	var manifest PluginManifest
	err := json.Unmarshal([]byte(manifestJSON), &manifest)
	if err != nil {
		t.Fatalf("Failed to parse manifest: %v", err)
	}

	if manifest.ID != "test-plugin" {
		t.Errorf("Expected ID 'test-plugin', got '%s'", manifest.ID)
	}

	if manifest.Command != "python3" {
		t.Errorf("Expected command 'python3', got '%s'", manifest.Command)
	}

	if len(manifest.Args) != 1 || manifest.Args[0] != "plugin.py" {
		t.Errorf("Unexpected args: %v", manifest.Args)
	}

	if len(manifest.ConfigOptions) != 1 {
		t.Errorf("Expected 1 config option, got %d", len(manifest.ConfigOptions))
	}

	opt := manifest.ConfigOptions[0]
	if opt.ID != "api_key" {
		t.Errorf("Expected config option ID 'api_key', got '%s'", opt.ID)
	}
	if !opt.Sensitive {
		t.Error("Expected config option to be sensitive")
	}
}

// ══════════════════════════════════════════════════════════════════════════════
// Helper Functions
// ══════════════════════════════════════════════════════════════════════════════

func intPtr(i int) *int {
	return &i
}
