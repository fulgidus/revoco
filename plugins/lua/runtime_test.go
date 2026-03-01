package lua

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	glua "github.com/yuin/gopher-lua"
)

func TestRuntime_CreateState(t *testing.T) {
	r := NewRuntime()
	L := r.CreateState()
	defer L.Close()

	// Test basic Lua execution
	if err := L.DoString(`x = 1 + 1`); err != nil {
		t.Fatalf("basic math failed: %v", err)
	}

	// Verify sandboxing - dangerous functions should be nil
	if err := L.DoString(`
		assert(dofile == nil, "dofile should be nil")
		assert(loadfile == nil, "loadfile should be nil")
		assert(load == nil, "load should be nil")
	`); err != nil {
		t.Fatalf("sandbox check failed: %v", err)
	}

	// Test that safe libs are available
	if err := L.DoString(`
		-- string lib
		assert(string.upper("hello") == "HELLO", "string lib missing")
		-- table lib
		local t = {1, 2, 3}
		assert(#t == 3, "table lib missing")
		-- math lib
		assert(math.abs(-5) == 5, "math lib missing")
	`); err != nil {
		t.Fatalf("safe libs check failed: %v", err)
	}
}

func TestRuntime_LoadPlugin(t *testing.T) {
	// Create a temp plugin file
	tmpDir := t.TempDir()
	pluginPath := filepath.Join(tmpDir, "test-processor.lua")

	pluginContent := `
Plugin = {
	id = "test-processor",
	name = "Test Processor",
	description = "A test processor for unit tests",
	version = "1.0.0",
	type = "processor",
}

DefaultSelector = {
	extensions = {".txt", ".md"},
}

function Process(item, config)
	item.metadata = item.metadata or {}
	item.metadata.processed = true
	return item
end
`
	if err := os.WriteFile(pluginPath, []byte(pluginContent), 0644); err != nil {
		t.Fatalf("failed to write plugin file: %v", err)
	}

	r := NewRuntime()
	ctx := context.Background()

	plugin, err := r.LoadPlugin(ctx, pluginPath)
	if err != nil {
		t.Fatalf("failed to load plugin: %v", err)
	}

	// Verify plugin info
	info := plugin.Info()
	if info.ID != "test-processor" {
		t.Errorf("expected ID 'test-processor', got '%s'", info.ID)
	}
	if info.Name != "Test Processor" {
		t.Errorf("expected Name 'Test Processor', got '%s'", info.Name)
	}
	if info.Type != "processor" {
		t.Errorf("expected Type 'processor', got '%s'", info.Type)
	}

	// Verify selector
	if info.DefaultSelector == nil {
		t.Fatal("expected DefaultSelector to be set")
	}
	if len(info.DefaultSelector.Extensions) != 2 {
		t.Errorf("expected 2 extensions, got %d", len(info.DefaultSelector.Extensions))
	}
}

func TestRuntime_LoadConnectorPlugin(t *testing.T) {
	tmpDir := t.TempDir()
	pluginPath := filepath.Join(tmpDir, "test-connector.lua")

	pluginContent := `
Plugin = {
	id = "test-connector",
	name = "Test Connector",
	description = "A test connector",
	version = "1.0.0",
	type = "connector",
	capabilities = {"read", "list"},
	data_types = {"document"},
}

ConfigSchema = {
	{
		id = "path",
		name = "Path",
		description = "The path to read from",
		type = "path",
		required = true,
	},
}

local items = {}

function Initialize(config)
	-- Store config for later use
	return true
end

function List()
	return {
		{id = "1", type = "document", path = "/test/file1.txt"},
		{id = "2", type = "document", path = "/test/file2.txt"},
	}
end

function Read(item)
	return "content of " .. item.id
end
`
	if err := os.WriteFile(pluginPath, []byte(pluginContent), 0644); err != nil {
		t.Fatalf("failed to write plugin file: %v", err)
	}

	r := NewRuntime()
	ctx := context.Background()

	plugin, err := r.LoadPlugin(ctx, pluginPath)
	if err != nil {
		t.Fatalf("failed to load plugin: %v", err)
	}

	// Load the plugin
	if err := plugin.Load(ctx); err != nil {
		t.Fatalf("failed to load plugin state: %v", err)
	}
	defer plugin.Unload()

	// Create connector adapter
	connector := NewLuaConnector(plugin)

	// Verify connector info
	if connector.ID() != "test-connector" {
		t.Errorf("expected ID 'test-connector', got '%s'", connector.ID())
	}

	caps := connector.Capabilities()
	if len(caps) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(caps))
	}

	// Test HasFunction
	if !plugin.HasFunction("List") {
		t.Error("expected plugin to have List function")
	}
	if !plugin.HasFunction("Read") {
		t.Error("expected plugin to have Read function")
	}
	if plugin.HasFunction("NonExistent") {
		t.Error("expected plugin to NOT have NonExistent function")
	}
}

func TestBuiltins_Revoco(t *testing.T) {
	r := NewRuntime()
	L := r.CreateState()
	defer L.Close()

	// Load the revoco module
	if err := L.DoString(`local revoco = require("revoco")`); err != nil {
		t.Fatalf("failed to require revoco: %v", err)
	}

	// Test path functions
	if err := L.DoString(`
		local revoco = require("revoco")
		
		-- Test join
		local joined = revoco.join("a", "b", "c")
		assert(joined == "a/b/c" or joined == "a\\b\\c", "join failed: " .. joined)
		
		-- Test basename
		assert(revoco.basename("/path/to/file.txt") == "file.txt", "basename failed")
		
		-- Test dirname
		local dir = revoco.dirname("/path/to/file.txt")
		assert(dir == "/path/to" or dir == "\\path\\to", "dirname failed: " .. dir)
		
		-- Test extname
		assert(revoco.extname("file.txt") == ".txt", "extname failed")
	`); err != nil {
		t.Fatalf("path functions test failed: %v", err)
	}

	// Test time functions
	if err := L.DoString(`
		local revoco = require("revoco")
		
		-- Test now
		local now = revoco.now()
		assert(type(now) == "number", "now should return number")
		assert(now > 0, "now should be positive")
	`); err != nil {
		t.Fatalf("time functions test failed: %v", err)
	}

	// Test JSON functions
	if err := L.DoString(`
		local revoco = require("revoco")
		
		-- Test encode/decode
		local data = {name = "test", value = 123}
		local encoded = revoco.json.encode(data)
		local decoded = revoco.json.decode(encoded)
		assert(decoded.name == "test", "json roundtrip failed")
		assert(decoded.value == 123, "json roundtrip failed")
	`); err != nil {
		t.Fatalf("JSON functions test failed: %v", err)
	}

	// Test hash functions
	if err := L.DoString(`
		local revoco = require("revoco")
		
		-- Test md5
		local md5 = revoco.hash.md5("hello")
		assert(#md5 == 32, "md5 should be 32 chars")
		
		-- Test sha256
		local sha256 = revoco.hash.sha256("hello")
		assert(#sha256 == 64, "sha256 should be 64 chars")
	`); err != nil {
		t.Fatalf("hash functions test failed: %v", err)
	}
}

func TestBuiltins_FileOps(t *testing.T) {
	tmpDir := t.TempDir()

	r := NewRuntime()
	L := r.CreateState()
	defer L.Close()

	// Set up test path
	testFile := filepath.Join(tmpDir, "test.txt")
	L.SetGlobal("TEST_DIR", glua.LString(tmpDir))
	L.SetGlobal("TEST_FILE", glua.LString(testFile))

	// Test file operations
	if err := L.DoString(`
		local revoco = require("revoco")
		
		-- Test writeFile
		local ok, err = revoco.writeFile(TEST_FILE, "hello world")
		assert(ok, "writeFile failed: " .. tostring(err))
		
		-- Test exists
		assert(revoco.exists(TEST_FILE), "file should exist")
		assert(not revoco.exists(TEST_FILE .. ".nonexistent"), "file should not exist")
		
		-- Test isDir
		assert(revoco.isDir(TEST_DIR), "should be a directory")
		assert(not revoco.isDir(TEST_FILE), "should not be a directory")
		
		-- Test readFile
		local content, err = revoco.readFile(TEST_FILE)
		assert(content == "hello world", "readFile content mismatch: " .. tostring(content))
	`); err != nil {
		t.Fatalf("file operations test failed: %v", err)
	}
}

func TestExec_AllowedBinaries(t *testing.T) {
	r := NewRuntime()
	L := r.CreateState()
	defer L.Close()

	// Test that disallowed binaries are blocked
	if err := L.DoString(`
		local revoco = require("revoco")
		
		-- Try to execute a disallowed binary
		local result = revoco.exec("rm", {"-rf", "/"})
		assert(result == nil, "should not be able to execute rm")
	`); err != nil {
		t.Fatalf("exec security test failed: %v", err)
	}
}
