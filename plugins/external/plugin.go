package external

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/plugins"
)

// ExternalPlugin represents a loaded external plugin.
type ExternalPlugin struct {
	manager  *ProcessManager
	manifest *PluginManifest
	info     *plugins.PluginInfo
	dir      string

	// Current process (nil when not loaded)
	process *Process
}

// ══════════════════════════════════════════════════════════════════════════════
// Plugin Loading
// ══════════════════════════════════════════════════════════════════════════════

// LoadExternalPlugin loads an external plugin from a directory.
func LoadExternalPlugin(manager *ProcessManager, dir string) (*ExternalPlugin, error) {
	// Find and read manifest
	manifestPath, err := FindPluginManifest(dir)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}

	var manifest PluginManifest
	ext := filepath.Ext(manifestPath)
	if ext == ".json" {
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("failed to parse manifest: %w", err)
		}
	} else {
		// YAML - use yaml package
		// For now, just support JSON
		return nil, fmt.Errorf("YAML manifests not yet supported, use plugin.json")
	}

	// Build plugin info
	info := &plugins.PluginInfo{
		ID:          manifest.ID,
		Name:        manifest.Name,
		Description: manifest.Description,
		Version:     manifest.Version,
		Type:        plugins.PluginType(manifest.Type),
		Tier:        plugins.PluginTierExternal,
		Source:      plugins.PluginSourceUser,
		Path:        dir,
		State:       plugins.PluginStateUnloaded,
	}

	// Convert capabilities
	for _, cap := range manifest.Capabilities {
		info.Capabilities = append(info.Capabilities, core.ConnectorCapability(cap))
	}

	// Convert data types
	for _, dt := range manifest.DataTypes {
		info.DataTypes = append(info.DataTypes, core.DataType(dt))
	}

	// Convert config schema
	for _, opt := range manifest.ConfigOptions {
		info.ConfigSchema = append(info.ConfigSchema, plugins.ConfigOption{
			ID:          opt.ID,
			Name:        opt.Name,
			Description: opt.Description,
			Type:        opt.Type,
			Default:     opt.Default,
			Options:     opt.Options,
			Required:    opt.Required,
			Sensitive:   opt.Sensitive,
		})
	}

	// Convert binary dependencies
	for _, dep := range manifest.BinaryDeps {
		info.Dependencies = append(info.Dependencies, plugins.BinaryDependency{
			Binary:     dep.Name,
			Check:      fmt.Sprintf("%s %s", dep.Command, dep.VersionFlag),
			MinVersion: dep.MinVersion,
			Install:    dep.InstallCommands,
		})
	}

	// Note: Selector can be provided via ConfigOptions or as structured data
	// For simple selectors, the manifest can include a basic extension filter
	// Complex selectors should be configured at runtime

	info.RequiresAuth = manifest.RequiresAuth
	info.AuthType = manifest.AuthType

	return &ExternalPlugin{
		manager:  manager,
		manifest: &manifest,
		info:     info,
		dir:      dir,
	}, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Plugin Interface Implementation
// ══════════════════════════════════════════════════════════════════════════════

// Info returns the plugin metadata.
func (p *ExternalPlugin) Info() plugins.PluginInfo {
	if p.info == nil {
		return plugins.PluginInfo{}
	}
	return *p.info
}

// Load initializes the plugin (starts the process).
func (p *ExternalPlugin) Load(ctx context.Context) error {
	// Check binary dependencies
	if len(p.info.Dependencies) > 0 {
		checker := plugins.NewDependencyChecker()
		missing := checker.MissingDependencies(p.info.Dependencies)
		if len(missing) > 0 {
			p.info.State = plugins.PluginStateMissingDeps
			p.info.StateError = fmt.Sprintf("missing: %s", missing[0].Binary)
			return &plugins.DependencyError{
				Binary:  missing[0].Binary,
				Message: "binary not found",
				Install: missing[0].Install,
			}
		}
	}

	// Run setup commands if needed
	if len(p.manifest.SetupCommands) > 0 {
		if err := RunPluginSetup(ctx, p.dir, p.manifest.SetupCommands); err != nil {
			p.info.State = plugins.PluginStateError
			p.info.StateError = fmt.Sprintf("setup failed: %v", err)
			return err
		}
	}

	// Build command arguments
	args := make([]string, len(p.manifest.Args))
	copy(args, p.manifest.Args)

	// Start the process
	process, err := p.manager.Start(ctx, p.info.ID, p.manifest.Command, args, nil, p.dir)
	if err != nil {
		p.info.State = plugins.PluginStateError
		p.info.StateError = err.Error()
		return err
	}
	p.process = process

	// Call initialize on the plugin
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	client := process.Client()
	var result InitializeResult
	err = client.CallWithResult(initCtx, MethodInitialize, &InitializeParams{
		WorkDir:  p.dir,
		CacheDir: filepath.Join(p.dir, ".cache"),
	}, &result)
	if err != nil {
		p.manager.Stop(p.info.ID)
		p.process = nil
		p.info.State = plugins.PluginStateError
		p.info.StateError = fmt.Sprintf("initialization failed: %v", err)
		return err
	}

	if !result.Success {
		p.manager.Stop(p.info.ID)
		p.process = nil
		p.info.State = plugins.PluginStateError
		p.info.StateError = result.Message
		return fmt.Errorf("plugin initialization failed: %s", result.Message)
	}

	p.info.State = plugins.PluginStateReady
	return nil
}

// Unload releases resources held by the plugin (stops the process).
func (p *ExternalPlugin) Unload() error {
	if p.process != nil {
		if err := p.manager.Stop(p.info.ID); err != nil {
			return err
		}
		p.process = nil
	}
	p.info.State = plugins.PluginStateUnloaded
	return nil
}

// Reload reloads the plugin.
func (p *ExternalPlugin) Reload(ctx context.Context) error {
	if err := p.Unload(); err != nil {
		return err
	}
	return p.Load(ctx)
}

// ══════════════════════════════════════════════════════════════════════════════
// Internal Helpers
// ══════════════════════════════════════════════════════════════════════════════

// Client returns the JSON-RPC client for this plugin.
func (p *ExternalPlugin) Client() *Client {
	if p.process == nil {
		return nil
	}
	return p.process.Client()
}

// IsLoaded returns true if the plugin process is running.
func (p *ExternalPlugin) IsLoaded() bool {
	return p.process != nil && p.process.IsRunning()
}

// Selector returns the default selector for this plugin.
func (p *ExternalPlugin) Selector() *plugins.Selector {
	return p.info.DefaultSelector
}

// Call is a helper method to call a method on the plugin.
func (p *ExternalPlugin) Call(ctx context.Context, method string, params any) (any, error) {
	client := p.Client()
	if client == nil {
		return nil, fmt.Errorf("plugin not loaded")
	}
	return client.Call(ctx, method, params)
}

// CallWithResult is a helper method to call a method and unmarshal the result.
func (p *ExternalPlugin) CallWithResult(ctx context.Context, method string, params any, result any) error {
	client := p.Client()
	if client == nil {
		return fmt.Errorf("plugin not loaded")
	}
	return client.CallWithResult(ctx, method, params, result)
}

// ══════════════════════════════════════════════════════════════════════════════
// Data Conversion
// ══════════════════════════════════════════════════════════════════════════════

// DataItemToExternal converts a core.DataItem to the external protocol format.
func DataItemToExternal(item *core.DataItem) *DataItem {
	if item == nil {
		return nil
	}
	return &DataItem{
		ID:           item.ID,
		Type:         string(item.Type),
		Path:         item.Path,
		RemoteID:     item.RemoteID,
		SourceConnID: item.SourceConnID,
		Metadata:     item.Metadata,
		Size:         item.Size,
		Checksum:     item.Checksum,
	}
}

// ExternalToDataItem converts an external protocol DataItem to core.DataItem.
func ExternalToDataItem(item *DataItem) *core.DataItem {
	if item == nil {
		return nil
	}
	return &core.DataItem{
		ID:           item.ID,
		Type:         core.DataType(item.Type),
		Path:         item.Path,
		RemoteID:     item.RemoteID,
		SourceConnID: item.SourceConnID,
		Metadata:     item.Metadata,
		Size:         item.Size,
		Checksum:     item.Checksum,
	}
}
