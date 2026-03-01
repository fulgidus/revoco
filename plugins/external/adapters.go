package external

import (
	"bytes"
	"context"
	"fmt"
	"io"

	core "github.com/fulgidus/revoco/connectors"
	"github.com/fulgidus/revoco/plugins"
	svccore "github.com/fulgidus/revoco/services/core"
)

// ══════════════════════════════════════════════════════════════════════════════
// External Connector Adapter
// ══════════════════════════════════════════════════════════════════════════════

// ExternalConnector adapts an ExternalPlugin to the connector interfaces.
type ExternalConnector struct {
	plugin *ExternalPlugin
	config core.ConnectorConfig
}

// NewExternalConnector creates a connector adapter for an external plugin.
func NewExternalConnector(plugin *ExternalPlugin) *ExternalConnector {
	return &ExternalConnector{plugin: plugin}
}

// Plugin returns the underlying plugin.
func (c *ExternalConnector) Plugin() *ExternalPlugin {
	return c.plugin
}

// ── Connector interface ───────────────────────────────────────────────────────

func (c *ExternalConnector) ID() string {
	return c.plugin.info.ID
}

func (c *ExternalConnector) Name() string {
	return c.plugin.info.Name
}

func (c *ExternalConnector) Description() string {
	return c.plugin.info.Description
}

func (c *ExternalConnector) Capabilities() []core.ConnectorCapability {
	return c.plugin.info.Capabilities
}

func (c *ExternalConnector) SupportedDataTypes() []core.DataType {
	return c.plugin.info.DataTypes
}

func (c *ExternalConnector) RequiresAuth() bool {
	return c.plugin.info.RequiresAuth
}

func (c *ExternalConnector) AuthType() string {
	return c.plugin.info.AuthType
}

func (c *ExternalConnector) ConfigSchema() []core.ConfigOption {
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

func (c *ExternalConnector) ValidateConfig(cfg core.ConnectorConfig) error {
	// Call plugin's validateConfig if available
	ctx := context.Background()
	_, err := c.plugin.Call(ctx, "validateConfig", map[string]any{
		"config": cfg.Settings,
	})
	if err != nil {
		// If method not found, consider config valid
		if rpcErr, ok := err.(*Error); ok && rpcErr.Code == ErrCodeMethodNotFound {
			return nil
		}
		return err
	}
	return nil
}

func (c *ExternalConnector) FallbackOptions() []core.FallbackOption {
	// External plugins don't support fallback options yet
	return nil
}

// ── ConnectorReader interface ─────────────────────────────────────────────────

func (c *ExternalConnector) Initialize(ctx context.Context, cfg core.ConnectorConfig) error {
	c.config = cfg

	var result InitializeResult
	err := c.plugin.CallWithResult(ctx, MethodInitialize, &InitializeParams{
		Config: cfg.Settings,
	}, &result)
	if err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("initialization failed: %s", result.Message)
	}

	return nil
}

func (c *ExternalConnector) List(ctx context.Context, progress core.ProgressFunc) ([]core.DataItem, error) {
	var result []DataItem
	err := c.plugin.CallWithResult(ctx, MethodList, nil, &result)
	if err != nil {
		return nil, err
	}

	// Convert to core.DataItem
	items := make([]core.DataItem, 0, len(result))
	for _, item := range result {
		items = append(items, *ExternalToDataItem(&item))
	}

	return items, nil
}

func (c *ExternalConnector) Read(ctx context.Context, item core.DataItem) (io.ReadCloser, error) {
	// Call plugin's read method
	params := map[string]any{
		"item": DataItemToExternal(&item),
	}

	var result struct {
		Content string `json:"content"`
		Base64  bool   `json:"base64,omitempty"`
	}

	err := c.plugin.CallWithResult(ctx, MethodRead, params, &result)
	if err != nil {
		return nil, err
	}

	// Decode content
	var data []byte
	if result.Base64 {
		// Base64 decode for binary data
		// TODO: implement base64 decoding
		data = []byte(result.Content)
	} else {
		data = []byte(result.Content)
	}

	return io.NopCloser(bytes.NewReader(data)), nil
}

func (c *ExternalConnector) ReadTo(ctx context.Context, item core.DataItem, destPath string, mode core.ImportMode) error {
	// Call plugin's readTo method
	params := map[string]any{
		"item":     DataItemToExternal(&item),
		"destPath": destPath,
		"mode":     string(mode),
	}

	_, err := c.plugin.Call(ctx, "readTo", params)
	if err != nil {
		// If readTo not implemented, fall back to Read + write
		if rpcErr, ok := err.(*Error); ok && rpcErr.Code == ErrCodeMethodNotFound {
			reader, err := c.Read(ctx, item)
			if err != nil {
				return err
			}
			defer reader.Close()

			// Use a temporary approach - external plugins should implement readTo
			return fmt.Errorf("readTo not implemented and fallback not available")
		}
		return err
	}

	return nil
}

func (c *ExternalConnector) Close() error {
	ctx := context.Background()
	_, err := c.plugin.Call(ctx, "close", nil)
	if err != nil {
		// Ignore method not found
		if rpcErr, ok := err.(*Error); ok && rpcErr.Code == ErrCodeMethodNotFound {
			return nil
		}
		return err
	}
	return nil
}

// ── ConnectorWriter interface ─────────────────────────────────────────────────

func (c *ExternalConnector) Write(ctx context.Context, item core.DataItem, reader io.Reader) error {
	// Read content
	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}

	params := map[string]any{
		"item":    DataItemToExternal(&item),
		"content": string(content),
	}

	_, err = c.plugin.Call(ctx, MethodWrite, params)
	return err
}

func (c *ExternalConnector) WriteFrom(ctx context.Context, item core.DataItem, sourcePath string) error {
	params := map[string]any{
		"item":       DataItemToExternal(&item),
		"sourcePath": sourcePath,
	}

	_, err := c.plugin.Call(ctx, "writeFrom", params)
	if err != nil {
		if rpcErr, ok := err.(*Error); ok && rpcErr.Code == ErrCodeMethodNotFound {
			return fmt.Errorf("writeFrom not implemented")
		}
		return err
	}
	return nil
}

func (c *ExternalConnector) WriteBatch(ctx context.Context, items []core.DataItem, getReader func(core.DataItem) (io.Reader, error), progress core.ProgressFunc) error {
	// External plugins process one at a time for now
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

func (c *ExternalConnector) Delete(ctx context.Context, item core.DataItem) error {
	params := map[string]any{
		"item": DataItemToExternal(&item),
	}

	_, err := c.plugin.Call(ctx, MethodDelete, params)
	return err
}

// ── ConnectorTester interface ─────────────────────────────────────────────────

func (c *ExternalConnector) TestConnection(ctx context.Context, cfg core.ConnectorConfig) error {
	params := map[string]any{
		"config": cfg.Settings,
	}

	var result struct {
		Success bool   `json:"success"`
		Message string `json:"message,omitempty"`
	}

	err := c.plugin.CallWithResult(ctx, MethodTestConnection, params, &result)
	if err != nil {
		return err
	}

	if !result.Success {
		return fmt.Errorf("connection test failed: %s", result.Message)
	}

	return nil
}

// ══════════════════════════════════════════════════════════════════════════════
// External Processor Adapter
// ══════════════════════════════════════════════════════════════════════════════

// ExternalProcessor adapts an ExternalPlugin to the Processor interface.
type ExternalProcessor struct {
	plugin *ExternalPlugin
}

// NewExternalProcessor creates a processor adapter for an external plugin.
func NewExternalProcessor(plugin *ExternalPlugin) *ExternalProcessor {
	return &ExternalProcessor{plugin: plugin}
}

// Plugin returns the underlying plugin.
func (p *ExternalProcessor) Plugin() *ExternalPlugin {
	return p.plugin
}

func (p *ExternalProcessor) ID() string {
	return p.plugin.info.ID
}

func (p *ExternalProcessor) Name() string {
	return p.plugin.info.Name
}

func (p *ExternalProcessor) Description() string {
	return p.plugin.info.Description
}

func (p *ExternalProcessor) ConfigSchema() []plugins.ConfigOption {
	return p.plugin.info.ConfigSchema
}

func (p *ExternalProcessor) DefaultSelector() *plugins.Selector {
	return p.plugin.info.DefaultSelector
}

func (p *ExternalProcessor) CanProcess(item *core.DataItem) bool {
	// First check selector
	if p.plugin.info.DefaultSelector != nil {
		matcher, err := plugins.NewSelectorMatcher(p.plugin.info.DefaultSelector)
		if err == nil && !matcher.Match(item) {
			return false
		}
	}

	// Then check plugin's canProcess
	ctx := context.Background()
	params := map[string]any{
		"item": DataItemToExternal(item),
	}

	var result bool
	err := p.plugin.CallWithResult(ctx, MethodCanProcess, params, &result)
	if err != nil {
		// If method not found, assume can process
		if rpcErr, ok := err.(*Error); ok && rpcErr.Code == ErrCodeMethodNotFound {
			return true
		}
		return false
	}

	return result
}

func (p *ExternalProcessor) Process(ctx context.Context, item *core.DataItem, config map[string]any) (*core.DataItem, error) {
	params := map[string]any{
		"item":   DataItemToExternal(item),
		"config": config,
	}

	var result ProcessResult
	err := p.plugin.CallWithResult(ctx, MethodProcess, params, &result)
	if err != nil {
		return nil, err
	}

	if result.Error != "" {
		return nil, fmt.Errorf("%s", result.Error)
	}

	if result.Skipped {
		return nil, nil
	}

	return ExternalToDataItem(result.Item), nil
}

func (p *ExternalProcessor) ProcessBatch(ctx context.Context, items []*core.DataItem, config map[string]any, progress plugins.ProgressFunc) ([]*core.DataItem, error) {
	// Try batch method first
	extItems := make([]*DataItem, len(items))
	for i, item := range items {
		extItems[i] = DataItemToExternal(item)
	}

	params := map[string]any{
		"items":  extItems,
		"config": config,
	}

	var results []ProcessResult
	err := p.plugin.CallWithResult(ctx, MethodProcessBatch, params, &results)
	if err != nil {
		// If batch not supported, fall back to processing one at a time
		if rpcErr, ok := err.(*Error); ok && rpcErr.Code == ErrCodeMethodNotFound {
			return p.processBatchSequential(ctx, items, config, progress)
		}
		return nil, err
	}

	// Convert results
	processed := make([]*core.DataItem, 0, len(results))
	for _, result := range results {
		if result.Error != "" {
			return nil, fmt.Errorf("%s", result.Error)
		}
		if !result.Skipped && result.Item != nil {
			processed = append(processed, ExternalToDataItem(result.Item))
		}
	}

	return processed, nil
}

func (p *ExternalProcessor) processBatchSequential(ctx context.Context, items []*core.DataItem, config map[string]any, progress plugins.ProgressFunc) ([]*core.DataItem, error) {
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
// External Output Adapter
// ══════════════════════════════════════════════════════════════════════════════

// ExternalOutput adapts an ExternalPlugin to the Output interface.
type ExternalOutput struct {
	plugin *ExternalPlugin
}

// NewExternalOutput creates an output adapter for an external plugin.
func NewExternalOutput(plugin *ExternalPlugin) *ExternalOutput {
	return &ExternalOutput{plugin: plugin}
}

// Plugin returns the underlying plugin.
func (o *ExternalOutput) Plugin() *ExternalPlugin {
	return o.plugin
}

func (o *ExternalOutput) ID() string {
	return o.plugin.info.ID
}

func (o *ExternalOutput) Name() string {
	return o.plugin.info.Name
}

func (o *ExternalOutput) Description() string {
	return o.plugin.info.Description
}

func (o *ExternalOutput) ConfigSchema() []svccore.ConfigOption {
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

func (o *ExternalOutput) Selector() *plugins.Selector {
	return o.plugin.info.DefaultSelector
}

// SupportedItemTypes returns the item types this output can handle.
func (o *ExternalOutput) SupportedItemTypes() []string {
	var types []string
	for _, dt := range o.plugin.info.DataTypes {
		types = append(types, string(dt))
	}
	if len(types) == 0 {
		return []string{"photo", "video", "audio", "note", "document"}
	}
	return types
}

// Initialize prepares the output for use.
func (o *ExternalOutput) Initialize(ctx context.Context, cfg svccore.OutputConfig) error {
	var result InitializeResult
	err := o.plugin.CallWithResult(ctx, MethodInitialize, &InitializeParams{
		Config: cfg.Settings,
	}, &result)
	if err != nil {
		// If method not found, consider initialized
		if rpcErr, ok := err.(*Error); ok && rpcErr.Code == ErrCodeMethodNotFound {
			return nil
		}
		return err
	}

	if !result.Success {
		return fmt.Errorf("initialization failed: %s", result.Message)
	}

	return nil
}

// Export sends a single item to the destination.
func (o *ExternalOutput) Export(ctx context.Context, item svccore.ProcessedItem) error {
	params := map[string]any{
		"item": map[string]any{
			"sourcePath":    item.SourcePath,
			"processedPath": item.ProcessedPath,
			"destRelPath":   item.DestRelPath,
			"type":          item.Type,
			"metadata":      item.Metadata,
		},
	}

	_, err := o.plugin.Call(ctx, MethodExport, params)
	return err
}

// ExportBatch sends multiple items to the destination.
func (o *ExternalOutput) ExportBatch(ctx context.Context, items []svccore.ProcessedItem, progress svccore.ProgressFunc) error {
	// Try batch method first
	extItems := make([]map[string]any, len(items))
	for i, item := range items {
		extItems[i] = map[string]any{
			"sourcePath":    item.SourcePath,
			"processedPath": item.ProcessedPath,
			"destRelPath":   item.DestRelPath,
			"type":          item.Type,
			"metadata":      item.Metadata,
		}
	}

	params := map[string]any{
		"items": extItems,
	}

	_, err := o.plugin.Call(ctx, MethodExportBatch, params)
	if err != nil {
		// If batch not supported, fall back to one at a time
		if rpcErr, ok := err.(*Error); ok && rpcErr.Code == ErrCodeMethodNotFound {
			return o.exportBatchSequential(ctx, items, progress)
		}
		return err
	}

	return nil
}

func (o *ExternalOutput) exportBatchSequential(ctx context.Context, items []svccore.ProcessedItem, progress svccore.ProgressFunc) error {
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
func (o *ExternalOutput) Finalize(ctx context.Context) error {
	_, err := o.plugin.Call(ctx, MethodFinalize, nil)
	if err != nil {
		// If method not found, consider finalized
		if rpcErr, ok := err.(*Error); ok && rpcErr.Code == ErrCodeMethodNotFound {
			return nil
		}
		return err
	}
	return nil
}

func (o *ExternalOutput) CanOutput(item *core.DataItem) bool {
	// First check selector
	if o.plugin.info.DefaultSelector != nil {
		matcher, err := plugins.NewSelectorMatcher(o.plugin.info.DefaultSelector)
		if err == nil && !matcher.Match(item) {
			return false
		}
	}

	// Then check plugin's canOutput
	ctx := context.Background()
	params := map[string]any{
		"item": DataItemToExternal(item),
	}

	var result bool
	err := o.plugin.CallWithResult(ctx, MethodCanOutput, params, &result)
	if err != nil {
		// If method not found, assume can output
		if rpcErr, ok := err.(*Error); ok && rpcErr.Code == ErrCodeMethodNotFound {
			return true
		}
		return false
	}

	return result
}

// ══════════════════════════════════════════════════════════════════════════════
// Plugin Interface Implementations
// ══════════════════════════════════════════════════════════════════════════════

// ExternalConnectorPlugin wraps an ExternalPlugin as a ConnectorPlugin.
type ExternalConnectorPlugin struct {
	*ExternalPlugin
	connector *ExternalConnector
}

// NewExternalConnectorPlugin creates a ConnectorPlugin from an ExternalPlugin.
func NewExternalConnectorPlugin(plugin *ExternalPlugin) *ExternalConnectorPlugin {
	return &ExternalConnectorPlugin{
		ExternalPlugin: plugin,
		connector:      NewExternalConnector(plugin),
	}
}

func (p *ExternalConnectorPlugin) AsConnector() core.Connector {
	return p.connector
}

func (p *ExternalConnectorPlugin) AsReader() (core.ConnectorReader, bool) {
	for _, cap := range p.info.Capabilities {
		if cap == core.CapabilityRead {
			return p.connector, true
		}
	}
	return nil, false
}

func (p *ExternalConnectorPlugin) AsWriter() (core.ConnectorWriter, bool) {
	for _, cap := range p.info.Capabilities {
		if cap == core.CapabilityWrite {
			return p.connector, true
		}
	}
	return nil, false
}

func (p *ExternalConnectorPlugin) AsTester() (core.ConnectorTester, bool) {
	// External plugins always support TestConnection
	return p.connector, true
}

// ExternalProcessorPlugin wraps an ExternalPlugin as a ProcessorPlugin.
type ExternalProcessorPlugin struct {
	*ExternalPlugin
	processor *ExternalProcessor
}

// NewExternalProcessorPlugin creates a ProcessorPlugin from an ExternalPlugin.
func NewExternalProcessorPlugin(plugin *ExternalPlugin) *ExternalProcessorPlugin {
	return &ExternalProcessorPlugin{
		ExternalPlugin: plugin,
		processor:      NewExternalProcessor(plugin),
	}
}

func (p *ExternalProcessorPlugin) AsProcessor() plugins.Processor {
	return p.processor
}

func (p *ExternalProcessorPlugin) Selector() *plugins.Selector {
	return p.info.DefaultSelector
}

// ExternalOutputPlugin wraps an ExternalPlugin as an OutputPlugin.
type ExternalOutputPlugin struct {
	*ExternalPlugin
	output *ExternalOutput
}

// NewExternalOutputPlugin creates an OutputPlugin from an ExternalPlugin.
func NewExternalOutputPlugin(plugin *ExternalPlugin) *ExternalOutputPlugin {
	return &ExternalOutputPlugin{
		ExternalPlugin: plugin,
		output:         NewExternalOutput(plugin),
	}
}

func (p *ExternalOutputPlugin) AsOutput() svccore.Output {
	return p.output
}

func (p *ExternalOutputPlugin) Selector() *plugins.Selector {
	return p.info.DefaultSelector
}
