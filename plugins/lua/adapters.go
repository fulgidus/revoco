package lua

import (
	"bytes"
	"context"
	"fmt"
	"io"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/plugins"
	svccore "github.com/fulgidus/revoco/services/core"
	lua "github.com/yuin/gopher-lua"
)

// ══════════════════════════════════════════════════════════════════════════════
// Lua Connector Adapter
// ══════════════════════════════════════════════════════════════════════════════

// LuaConnector adapts a LuaPlugin to the connector interfaces.
type LuaConnector struct {
	plugin *LuaPlugin
	config core.ConnectorConfig
}

// NewLuaConnector creates a connector adapter for a Lua plugin.
func NewLuaConnector(plugin *LuaPlugin) *LuaConnector {
	return &LuaConnector{plugin: plugin}
}

// Plugin returns the underlying plugin.
func (c *LuaConnector) Plugin() *LuaPlugin {
	return c.plugin
}

// ── Connector interface ───────────────────────────────────────────────────────

func (c *LuaConnector) ID() string {
	return c.plugin.info.ID
}

func (c *LuaConnector) Name() string {
	return c.plugin.info.Name
}

func (c *LuaConnector) Description() string {
	return c.plugin.info.Description
}

func (c *LuaConnector) Capabilities() []core.ConnectorCapability {
	return c.plugin.info.Capabilities
}

func (c *LuaConnector) SupportedDataTypes() []core.DataType {
	return c.plugin.info.DataTypes
}

func (c *LuaConnector) RequiresAuth() bool {
	return c.plugin.info.RequiresAuth
}

func (c *LuaConnector) AuthType() string {
	return c.plugin.info.AuthType
}

func (c *LuaConnector) ConfigSchema() []core.ConfigOption {
	// Convert plugin ConfigOption to core ConfigOption
	var schema []core.ConfigOption
	for _, opt := range c.plugin.info.ConfigSchema {
		schema = append(schema, core.ConfigOption{
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
	return schema
}

func (c *LuaConnector) ValidateConfig(cfg core.ConnectorConfig) error {
	if !c.plugin.HasFunction("ValidateConfig") {
		return nil // No validation function, accept all
	}

	configTbl := c.plugin.ConfigToLua(cfg.Settings)
	results, err := c.plugin.CallFunction("ValidateConfig", configTbl)
	if err != nil {
		return err
	}

	// Check for error return
	if len(results) >= 2 && results[1] != lua.LNil {
		if errStr, ok := results[1].(lua.LString); ok {
			return fmt.Errorf("%s", string(errStr))
		}
	}

	return nil
}

func (c *LuaConnector) FallbackOptions() []core.FallbackOption {
	// Lua plugins don't support fallback options yet
	return nil
}

// ── ConnectorReader interface ─────────────────────────────────────────────────

func (c *LuaConnector) Initialize(ctx context.Context, cfg core.ConnectorConfig) error {
	c.config = cfg

	if !c.plugin.HasFunction("Initialize") {
		return nil // No init function needed
	}

	configTbl := c.plugin.ConfigToLua(cfg.Settings)
	results, err := c.plugin.CallFunction("Initialize", configTbl)
	if err != nil {
		return err
	}

	// Check for error return
	if len(results) >= 1 && results[0] != lua.LNil && results[0] != lua.LTrue {
		if errStr, ok := results[0].(lua.LString); ok {
			return fmt.Errorf("%s", string(errStr))
		}
	}

	return nil
}

func (c *LuaConnector) List(ctx context.Context, progress core.ProgressFunc) ([]core.DataItem, error) {
	if !c.plugin.HasFunction("List") {
		return nil, fmt.Errorf("plugin does not implement List")
	}

	results, err := c.plugin.CallFunction("List")
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, nil
	}

	// Check for error
	if len(results) >= 2 && results[1] != lua.LNil {
		if errStr, ok := results[1].(lua.LString); ok {
			return nil, fmt.Errorf("%s", string(errStr))
		}
	}

	// Convert result table to items
	resultTbl, ok := results[0].(*lua.LTable)
	if !ok {
		return nil, fmt.Errorf("List must return a table")
	}

	var items []core.DataItem
	resultTbl.ForEach(func(_, value lua.LValue) {
		if itemTbl, ok := value.(*lua.LTable); ok {
			item := c.plugin.LuaToDataItem(itemTbl)
			if item != nil {
				items = append(items, *item)
			}
		}
	})

	return items, nil
}

func (c *LuaConnector) Read(ctx context.Context, item core.DataItem) (io.ReadCloser, error) {
	if !c.plugin.HasFunction("Read") {
		return nil, fmt.Errorf("plugin does not implement Read")
	}

	itemTbl := c.plugin.DataItemToLua(&item)
	results, err := c.plugin.CallFunction("Read", itemTbl)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("Read returned no data")
	}

	// Check for error
	if len(results) >= 2 && results[1] != lua.LNil {
		if errStr, ok := results[1].(lua.LString); ok {
			return nil, fmt.Errorf("%s", string(errStr))
		}
	}

	// Result should be string (content)
	if content, ok := results[0].(lua.LString); ok {
		return io.NopCloser(bytes.NewReader([]byte(string(content)))), nil
	}

	return nil, fmt.Errorf("Read must return string content")
}

func (c *LuaConnector) ReadTo(ctx context.Context, item core.DataItem, destPath string, mode core.ImportMode) error {
	if c.plugin.HasFunction("ReadTo") {
		// Use plugin's ReadTo if available
		itemTbl := c.plugin.DataItemToLua(&item)
		results, err := c.plugin.CallFunction("ReadTo", itemTbl, lua.LString(destPath), lua.LString(string(mode)))
		if err != nil {
			return err
		}

		// Check for error
		if len(results) >= 2 && results[1] != lua.LNil {
			if errStr, ok := results[1].(lua.LString); ok {
				return fmt.Errorf("%s", string(errStr))
			}
		}
		return nil
	}

	// Fall back to Read + write to file
	reader, err := c.Read(ctx, item)
	if err != nil {
		return err
	}
	defer reader.Close()

	// Use revoco.writeFile equivalent
	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	// Write using Lua's writeFile
	results, err := c.plugin.CallFunction("revoco.writeFile", lua.LString(destPath), lua.LString(content))
	if err != nil {
		// Fall back to direct file write if function doesn't exist
		return fmt.Errorf("ReadTo fallback not implemented: %w", err)
	}

	if len(results) >= 2 && results[1] != lua.LNil {
		if errStr, ok := results[1].(lua.LString); ok {
			return fmt.Errorf("%s", string(errStr))
		}
	}

	return nil
}

func (c *LuaConnector) Close() error {
	if c.plugin.HasFunction("Close") {
		_, err := c.plugin.CallFunction("Close")
		return err
	}
	return nil
}

// ── ConnectorWriter interface ─────────────────────────────────────────────────

func (c *LuaConnector) Write(ctx context.Context, item core.DataItem, reader io.Reader) error {
	if !c.plugin.HasFunction("Write") {
		return fmt.Errorf("plugin does not implement Write")
	}

	// Read content
	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	itemTbl := c.plugin.DataItemToLua(&item)
	results, err := c.plugin.CallFunction("Write", itemTbl, lua.LString(content))
	if err != nil {
		return err
	}

	// Check for error
	if len(results) >= 2 && results[1] != lua.LNil {
		if errStr, ok := results[1].(lua.LString); ok {
			return fmt.Errorf("%s", string(errStr))
		}
	}

	return nil
}

func (c *LuaConnector) WriteFrom(ctx context.Context, item core.DataItem, sourcePath string) error {
	if c.plugin.HasFunction("WriteFrom") {
		itemTbl := c.plugin.DataItemToLua(&item)
		results, err := c.plugin.CallFunction("WriteFrom", itemTbl, lua.LString(sourcePath))
		if err != nil {
			return err
		}

		if len(results) >= 2 && results[1] != lua.LNil {
			if errStr, ok := results[1].(lua.LString); ok {
				return fmt.Errorf("%s", string(errStr))
			}
		}
		return nil
	}

	// Fall back to reading file and calling Write
	return fmt.Errorf("WriteFrom not implemented and no fallback available")
}

func (c *LuaConnector) WriteBatch(ctx context.Context, items []core.DataItem, getReader func(core.DataItem) (io.Reader, error), progress core.ProgressFunc) error {
	// Lua plugins don't support batching, write one at a time
	for i, item := range items {
		reader, err := getReader(item)
		if err != nil {
			return err
		}

		if err := c.Write(ctx, item, reader); err != nil {
			return err
		}

		if progress != nil {
			progress(i+1, len(items))
		}
	}
	return nil
}

func (c *LuaConnector) Delete(ctx context.Context, item core.DataItem) error {
	if !c.plugin.HasFunction("Delete") {
		return fmt.Errorf("plugin does not implement Delete")
	}

	itemTbl := c.plugin.DataItemToLua(&item)
	results, err := c.plugin.CallFunction("Delete", itemTbl)
	if err != nil {
		return err
	}

	if len(results) >= 2 && results[1] != lua.LNil {
		if errStr, ok := results[1].(lua.LString); ok {
			return fmt.Errorf("%s", string(errStr))
		}
	}

	return nil
}

// ── ConnectorTester interface ─────────────────────────────────────────────────

func (c *LuaConnector) TestConnection(ctx context.Context, cfg core.ConnectorConfig) error {
	if !c.plugin.HasFunction("TestConnection") {
		// If no test function, try Initialize as a test
		return c.Initialize(ctx, cfg)
	}

	configTbl := c.plugin.ConfigToLua(cfg.Settings)
	results, err := c.plugin.CallFunction("TestConnection", configTbl)
	if err != nil {
		return err
	}

	if len(results) >= 1 {
		// Check if first result is false or an error string
		if results[0] == lua.LFalse {
			if len(results) >= 2 {
				if errStr, ok := results[1].(lua.LString); ok {
					return fmt.Errorf("%s", string(errStr))
				}
			}
			return fmt.Errorf("connection test failed")
		}
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Lua Processor Adapter
// ══════════════════════════════════════════════════════════════════════════════

// LuaProcessor adapts a LuaPlugin to the Processor interface.
type LuaProcessor struct {
	plugin *LuaPlugin
}

// NewLuaProcessor creates a processor adapter for a Lua plugin.
func NewLuaProcessor(plugin *LuaPlugin) *LuaProcessor {
	return &LuaProcessor{plugin: plugin}
}

// Plugin returns the underlying plugin.
func (p *LuaProcessor) Plugin() *LuaPlugin {
	return p.plugin
}

func (p *LuaProcessor) ID() string {
	return p.plugin.info.ID
}

func (p *LuaProcessor) Name() string {
	return p.plugin.info.Name
}

func (p *LuaProcessor) Description() string {
	return p.plugin.info.Description
}

func (p *LuaProcessor) ConfigSchema() []plugins.ConfigOption {
	return p.plugin.info.ConfigSchema
}

func (p *LuaProcessor) DefaultSelector() *plugins.Selector {
	return p.plugin.info.DefaultSelector
}

func (p *LuaProcessor) CanProcess(item *core.DataItem) bool {
	// First check selector using SelectorMatcher
	if p.plugin.info.DefaultSelector != nil {
		matcher, err := plugins.NewSelectorMatcher(p.plugin.info.DefaultSelector)
		if err == nil && !matcher.Match(item) {
			return false
		}
	}

	// Then check plugin's CanProcess if available
	if p.plugin.HasFunction("CanProcess") {
		itemTbl := p.plugin.DataItemToLua(item)
		results, err := p.plugin.CallFunction("CanProcess", itemTbl)
		if err != nil {
			return false
		}

		if len(results) > 0 {
			if b, ok := results[0].(lua.LBool); ok {
				return bool(b)
			}
		}
	}

	return true
}

func (p *LuaProcessor) Process(ctx context.Context, item *core.DataItem, config map[string]any) (*core.DataItem, error) {
	if !p.plugin.HasFunction("Process") {
		return nil, fmt.Errorf("plugin does not implement Process")
	}

	itemTbl := p.plugin.DataItemToLua(item)
	configTbl := p.plugin.ConfigToLua(config)

	results, err := p.plugin.CallFunction("Process", itemTbl, configTbl)
	if err != nil {
		return nil, err
	}

	if len(results) == 0 {
		return nil, fmt.Errorf("Process returned no result")
	}

	// Check for error
	if len(results) >= 2 && results[1] != lua.LNil {
		if errStr, ok := results[1].(lua.LString); ok {
			return nil, fmt.Errorf("%s", string(errStr))
		}
	}

	// Convert result
	if resultTbl, ok := results[0].(*lua.LTable); ok {
		return p.plugin.LuaToDataItem(resultTbl), nil
	}

	// If nil returned, item was filtered out
	if results[0] == lua.LNil {
		return nil, nil
	}

	return nil, fmt.Errorf("Process must return a table or nil")
}

func (p *LuaProcessor) ProcessBatch(ctx context.Context, items []*core.DataItem, config map[string]any, progress plugins.ProgressFunc) ([]*core.DataItem, error) {
	// Check for batch function
	if p.plugin.HasFunction("ProcessBatch") {
		// Build items table
		itemsTbl := p.plugin.state.NewTable()
		for i, item := range items {
			itemsTbl.RawSetInt(i+1, p.plugin.DataItemToLua(item))
		}

		configTbl := p.plugin.ConfigToLua(config)
		results, err := p.plugin.CallFunction("ProcessBatch", itemsTbl, configTbl)
		if err != nil {
			return nil, err
		}

		if len(results) == 0 {
			return nil, fmt.Errorf("ProcessBatch returned no result")
		}

		// Check for error
		if len(results) >= 2 && results[1] != lua.LNil {
			if errStr, ok := results[1].(lua.LString); ok {
				return nil, fmt.Errorf("%s", string(errStr))
			}
		}

		// Convert results
		resultsTbl, ok := results[0].(*lua.LTable)
		if !ok {
			return nil, fmt.Errorf("ProcessBatch must return a table")
		}

		var processed []*core.DataItem
		resultsTbl.ForEach(func(_, value lua.LValue) {
			if itemTbl, ok := value.(*lua.LTable); ok {
				processed = append(processed, p.plugin.LuaToDataItem(itemTbl))
			}
		})

		return processed, nil
	}

	// Fall back to processing one at a time
	var processed []*core.DataItem
	for i, item := range items {
		result, err := p.Process(ctx, item, config)
		if err != nil {
			return nil, err
		}
		if result != nil {
			processed = append(processed, result)
		}
		if progress != nil {
			progress(i+1, len(items), fmt.Sprintf("Processing %s", item.ID))
		}
	}

	return processed, nil
}

// ══════════════════════════════════════════════════════════════════════════════
// Lua Output Adapter
// ══════════════════════════════════════════════════════════════════════════════

// LuaOutput adapts a LuaPlugin to the Output interface.
type LuaOutput struct {
	plugin *LuaPlugin
}

// NewLuaOutput creates an output adapter for a Lua plugin.
func NewLuaOutput(plugin *LuaPlugin) *LuaOutput {
	return &LuaOutput{plugin: plugin}
}

// Plugin returns the underlying plugin.
func (o *LuaOutput) Plugin() *LuaPlugin {
	return o.plugin
}

func (o *LuaOutput) ID() string {
	return o.plugin.info.ID
}

func (o *LuaOutput) Name() string {
	return o.plugin.info.Name
}

func (o *LuaOutput) Description() string {
	return o.plugin.info.Description
}

func (o *LuaOutput) ConfigSchema() []svccore.ConfigOption {
	var schema []svccore.ConfigOption
	for _, opt := range o.plugin.info.ConfigSchema {
		schema = append(schema, svccore.ConfigOption{
			ID:          opt.ID,
			Name:        opt.Name,
			Description: opt.Description,
			Type:        opt.Type,
			Default:     opt.Default,
			Options:     opt.Options,
			Required:    opt.Required,
		})
	}
	return schema
}

func (o *LuaOutput) Selector() *plugins.Selector {
	return o.plugin.info.DefaultSelector
}

// SupportedItemTypes returns the item types this output can handle.
func (o *LuaOutput) SupportedItemTypes() []string {
	var types []string
	for _, dt := range o.plugin.info.DataTypes {
		types = append(types, string(dt))
	}
	if len(types) == 0 {
		// Default to all types if not specified
		return []string{"photo", "video", "audio", "note", "document"}
	}
	return types
}

// Initialize prepares the output for use.
func (o *LuaOutput) Initialize(ctx context.Context, cfg svccore.OutputConfig) error {
	if !o.plugin.HasFunction("Initialize") {
		return nil
	}

	configTbl := o.plugin.ConfigToLua(cfg.Settings)
	results, err := o.plugin.CallFunction("Initialize", configTbl)
	if err != nil {
		return err
	}

	if len(results) >= 1 && results[0] != lua.LNil && results[0] != lua.LTrue {
		if errStr, ok := results[0].(lua.LString); ok {
			return fmt.Errorf("%s", string(errStr))
		}
	}

	return nil
}

// Export sends a single item to the destination.
func (o *LuaOutput) Export(ctx context.Context, item svccore.ProcessedItem) error {
	if !o.plugin.HasFunction("Export") && !o.plugin.HasFunction("Output") {
		return fmt.Errorf("plugin does not implement Export or Output")
	}

	// Convert ProcessedItem to a Lua table
	itemTbl := o.plugin.state.NewTable()
	itemTbl.RawSetString("source_path", lua.LString(item.SourcePath))
	itemTbl.RawSetString("processed_path", lua.LString(item.ProcessedPath))
	itemTbl.RawSetString("dest_rel_path", lua.LString(item.DestRelPath))
	itemTbl.RawSetString("type", lua.LString(item.Type))
	if item.Metadata != nil {
		metaTbl := o.plugin.state.NewTable()
		for k, v := range item.Metadata {
			metaTbl.RawSetString(k, goValueToLua(o.plugin.state, v))
		}
		itemTbl.RawSetString("metadata", metaTbl)
	}

	// Try Export first, then fall back to Output
	fnName := "Export"
	if !o.plugin.HasFunction("Export") {
		fnName = "Output"
	}

	results, err := o.plugin.CallFunction(fnName, itemTbl)
	if err != nil {
		return err
	}

	if len(results) >= 1 && results[0] != lua.LNil && results[0] != lua.LTrue {
		if errStr, ok := results[0].(lua.LString); ok {
			return fmt.Errorf("%s", string(errStr))
		}
	}

	return nil
}

// ExportBatch sends multiple items to the destination.
func (o *LuaOutput) ExportBatch(ctx context.Context, items []svccore.ProcessedItem, progress svccore.ProgressFunc) error {
	// Check for batch function
	if o.plugin.HasFunction("ExportBatch") {
		itemsTbl := o.plugin.state.NewTable()
		for i, item := range items {
			itemTbl := o.plugin.state.NewTable()
			itemTbl.RawSetString("source_path", lua.LString(item.SourcePath))
			itemTbl.RawSetString("processed_path", lua.LString(item.ProcessedPath))
			itemTbl.RawSetString("dest_rel_path", lua.LString(item.DestRelPath))
			itemTbl.RawSetString("type", lua.LString(item.Type))
			if item.Metadata != nil {
				metaTbl := o.plugin.state.NewTable()
				for k, v := range item.Metadata {
					metaTbl.RawSetString(k, goValueToLua(o.plugin.state, v))
				}
				itemTbl.RawSetString("metadata", metaTbl)
			}
			itemsTbl.RawSetInt(i+1, itemTbl)
		}

		results, err := o.plugin.CallFunction("ExportBatch", itemsTbl)
		if err != nil {
			return err
		}

		if len(results) >= 1 && results[0] != lua.LNil && results[0] != lua.LTrue {
			if errStr, ok := results[0].(lua.LString); ok {
				return fmt.Errorf("%s", string(errStr))
			}
		}

		return nil
	}

	// Fall back to exporting one at a time
	for i, item := range items {
		if err := o.Export(ctx, item); err != nil {
			return err
		}
		if progress != nil {
			progress(i+1, len(items))
		}
	}

	return nil
}

// Finalize completes the export process.
func (o *LuaOutput) Finalize(ctx context.Context) error {
	if !o.plugin.HasFunction("Finalize") {
		return nil
	}

	results, err := o.plugin.CallFunction("Finalize")
	if err != nil {
		return err
	}

	if len(results) >= 1 && results[0] != lua.LNil && results[0] != lua.LTrue {
		if errStr, ok := results[0].(lua.LString); ok {
			return fmt.Errorf("%s", string(errStr))
		}
	}

	return nil
}

func (o *LuaOutput) CanOutput(item *core.DataItem) bool {
	// First check selector using SelectorMatcher
	if o.plugin.info.DefaultSelector != nil {
		matcher, err := plugins.NewSelectorMatcher(o.plugin.info.DefaultSelector)
		if err == nil && !matcher.Match(item) {
			return false
		}
	}

	// Then check plugin's CanOutput if available
	if o.plugin.HasFunction("CanOutput") {
		itemTbl := o.plugin.DataItemToLua(item)
		results, err := o.plugin.CallFunction("CanOutput", itemTbl)
		if err != nil {
			return false
		}

		if len(results) > 0 {
			if b, ok := results[0].(lua.LBool); ok {
				return bool(b)
			}
		}
	}

	return true
}

// ══════════════════════════════════════════════════════════════════════════════
// Plugin Interface Implementations
// ══════════════════════════════════════════════════════════════════════════════

// Ensure LuaPlugin implements ConnectorPlugin when it's a connector.
type LuaConnectorPlugin struct {
	*LuaPlugin
	connector *LuaConnector
}

func NewLuaConnectorPlugin(plugin *LuaPlugin) *LuaConnectorPlugin {
	return &LuaConnectorPlugin{
		LuaPlugin: plugin,
		connector: NewLuaConnector(plugin),
	}
}

func (p *LuaConnectorPlugin) AsConnector() core.Connector {
	return p.connector
}

func (p *LuaConnectorPlugin) AsReader() (core.ConnectorReader, bool) {
	for _, cap := range p.info.Capabilities {
		if cap == core.CapabilityRead {
			return p.connector, true
		}
	}
	return nil, false
}

func (p *LuaConnectorPlugin) AsWriter() (core.ConnectorWriter, bool) {
	for _, cap := range p.info.Capabilities {
		if cap == core.CapabilityWrite {
			return p.connector, true
		}
	}
	return nil, false
}

func (p *LuaConnectorPlugin) AsTester() (core.ConnectorTester, bool) {
	if p.HasFunction("TestConnection") {
		return p.connector, true
	}
	return nil, false
}

// Ensure LuaPlugin implements ProcessorPlugin when it's a processor.
type LuaProcessorPlugin struct {
	*LuaPlugin
	processor *LuaProcessor
}

func NewLuaProcessorPlugin(plugin *LuaPlugin) *LuaProcessorPlugin {
	return &LuaProcessorPlugin{
		LuaPlugin: plugin,
		processor: NewLuaProcessor(plugin),
	}
}

func (p *LuaProcessorPlugin) AsProcessor() plugins.Processor {
	return p.processor
}

func (p *LuaProcessorPlugin) Selector() *plugins.Selector {
	return p.info.DefaultSelector
}

// Ensure LuaPlugin implements OutputPlugin when it's an output.
type LuaOutputPlugin struct {
	*LuaPlugin
	output *LuaOutput
}

func NewLuaOutputPlugin(plugin *LuaPlugin) *LuaOutputPlugin {
	return &LuaOutputPlugin{
		LuaPlugin: plugin,
		output:    NewLuaOutput(plugin),
	}
}

func (p *LuaOutputPlugin) AsOutput() svccore.Output {
	return p.output
}

func (p *LuaOutputPlugin) Selector() *plugins.Selector {
	return p.info.DefaultSelector
}
