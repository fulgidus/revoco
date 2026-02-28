package core

import (
	"fmt"
	"sync"
)

// ══════════════════════════════════════════════════════════════════════════════
// Connector Registry
// ══════════════════════════════════════════════════════════════════════════════

// ConnectorFactory creates a new instance of a connector.
type ConnectorFactory func() Connector

// ConnectorInfo holds metadata about a registered connector type.
type ConnectorInfo struct {
	ID           string
	Name         string
	Description  string
	Capabilities []ConnectorCapability
	DataTypes    []DataType
	RequiresAuth bool
	AuthType     string
	Factory      ConnectorFactory
}

// Registry manages available connector types.
type Registry struct {
	mu         sync.RWMutex
	connectors map[string]*ConnectorInfo
	processors map[string]Processor
}

// globalRegistry is the default registry instance.
var globalRegistry = NewRegistry()

// NewRegistry creates a new empty registry.
func NewRegistry() *Registry {
	return &Registry{
		connectors: make(map[string]*ConnectorInfo),
		processors: make(map[string]Processor),
	}
}

// ── Connector Registration ────────────────────────────────────────────────────

// RegisterConnector adds a connector type to the registry.
func (r *Registry) RegisterConnector(factory ConnectorFactory) error {
	if factory == nil {
		return fmt.Errorf("registry: factory cannot be nil")
	}

	// Create a temporary instance to extract metadata
	conn := factory()
	if conn == nil {
		return fmt.Errorf("registry: factory returned nil connector")
	}

	id := conn.ID()
	if id == "" {
		return fmt.Errorf("registry: connector ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.connectors[id]; exists {
		return fmt.Errorf("registry: connector %q already registered", id)
	}

	r.connectors[id] = &ConnectorInfo{
		ID:           id,
		Name:         conn.Name(),
		Description:  conn.Description(),
		Capabilities: conn.Capabilities(),
		DataTypes:    conn.SupportedDataTypes(),
		RequiresAuth: conn.RequiresAuth(),
		AuthType:     conn.AuthType(),
		Factory:      factory,
	}

	return nil
}

// UnregisterConnector removes a connector type from the registry.
func (r *Registry) UnregisterConnector(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.connectors, id)
}

// GetConnectorInfo returns info about a registered connector type.
func (r *Registry) GetConnectorInfo(id string) (*ConnectorInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	info, ok := r.connectors[id]
	return info, ok
}

// CreateConnector creates a new instance of a registered connector type.
func (r *Registry) CreateConnector(id string) (Connector, error) {
	r.mu.RLock()
	info, ok := r.connectors[id]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("registry: unknown connector %q", id)
	}

	return info.Factory(), nil
}

// ListConnectors returns info about all registered connectors.
func (r *Registry) ListConnectors() []*ConnectorInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]*ConnectorInfo, 0, len(r.connectors))
	for _, info := range r.connectors {
		list = append(list, info)
	}
	return list
}

// ListConnectorsByCapability returns connectors that have a specific capability.
func (r *Registry) ListConnectorsByCapability(cap ConnectorCapability) []*ConnectorInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*ConnectorInfo
	for _, info := range r.connectors {
		for _, c := range info.Capabilities {
			if c == cap {
				list = append(list, info)
				break
			}
		}
	}
	return list
}

// ListConnectorsByDataType returns connectors that support a specific data type.
func (r *Registry) ListConnectorsByDataType(dt DataType) []*ConnectorInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*ConnectorInfo
	for _, info := range r.connectors {
		for _, t := range info.DataTypes {
			if t == dt {
				list = append(list, info)
				break
			}
		}
	}
	return list
}

// ListInputConnectors returns all connectors that can read data.
func (r *Registry) ListInputConnectors() []*ConnectorInfo {
	return r.ListConnectorsByCapability(CapabilityRead)
}

// ListOutputConnectors returns all connectors that can write data.
func (r *Registry) ListOutputConnectors() []*ConnectorInfo {
	return r.ListConnectorsByCapability(CapabilityWrite)
}

// ── Processor Registration ────────────────────────────────────────────────────

// RegisterProcessor adds a processor to the registry.
func (r *Registry) RegisterProcessor(p Processor) error {
	if p == nil {
		return fmt.Errorf("registry: processor cannot be nil")
	}

	id := p.ID()
	if id == "" {
		return fmt.Errorf("registry: processor ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.processors[id]; exists {
		return fmt.Errorf("registry: processor %q already registered", id)
	}

	r.processors[id] = p
	return nil
}

// UnregisterProcessor removes a processor from the registry.
func (r *Registry) UnregisterProcessor(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.processors, id)
}

// GetProcessor returns a registered processor by ID.
func (r *Registry) GetProcessor(id string) (Processor, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.processors[id]
	return p, ok
}

// ListProcessors returns all registered processors.
func (r *Registry) ListProcessors() []Processor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	list := make([]Processor, 0, len(r.processors))
	for _, p := range r.processors {
		list = append(list, p)
	}
	return list
}

// ListProcessorsForDataType returns processors that handle a specific data type.
func (r *Registry) ListProcessorsForDataType(dt DataType) []Processor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []Processor
	for _, p := range r.processors {
		if p.Scope() != ProcessorScopeDataType {
			continue
		}
		for _, t := range p.SupportedDataTypes() {
			if t == dt {
				list = append(list, p)
				break
			}
		}
	}
	return list
}

// ListProcessorsForConnector returns processors specific to a connector.
func (r *Registry) ListProcessorsForConnector(connectorID string) []Processor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []Processor
	for _, p := range r.processors {
		if p.Scope() != ProcessorScopeConnector {
			continue
		}
		for _, c := range p.SupportedConnectors() {
			if c == connectorID {
				list = append(list, p)
				break
			}
		}
	}
	return list
}

// GetApplicableProcessors returns all processors that can handle a given item.
func (r *Registry) GetApplicableProcessors(item DataItem) []Processor {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []Processor
	for _, p := range r.processors {
		if p.CanProcess(item) {
			list = append(list, p)
		}
	}
	return list
}

// ══════════════════════════════════════════════════════════════════════════════
// Global Registry Functions
// ══════════════════════════════════════════════════════════════════════════════

// Global returns the global registry instance.
func Global() *Registry {
	return globalRegistry
}

// RegisterConnector registers a connector in the global registry.
func RegisterConnector(factory ConnectorFactory) error {
	return globalRegistry.RegisterConnector(factory)
}

// CreateConnector creates a connector instance from the global registry.
func CreateConnector(id string) (Connector, error) {
	return globalRegistry.CreateConnector(id)
}

// GetConnectorInfo returns connector info from the global registry.
func GetConnectorInfo(id string) (*ConnectorInfo, bool) {
	return globalRegistry.GetConnectorInfo(id)
}

// ListConnectors returns all connectors from the global registry.
func ListConnectors() []*ConnectorInfo {
	return globalRegistry.ListConnectors()
}

// RegisterProcessor registers a processor in the global registry.
func RegisterProcessor(p Processor) error {
	return globalRegistry.RegisterProcessor(p)
}

// GetProcessor returns a processor from the global registry.
func GetProcessor(id string) (Processor, bool) {
	return globalRegistry.GetProcessor(id)
}

// ListProcessors returns all processors from the global registry.
func ListProcessors() []Processor {
	return globalRegistry.ListProcessors()
}

// ══════════════════════════════════════════════════════════════════════════════
// Validation Helpers
// ══════════════════════════════════════════════════════════════════════════════

// HasCapability checks if a connector has a specific capability.
func HasCapability(conn Connector, cap ConnectorCapability) bool {
	for _, c := range conn.Capabilities() {
		if c == cap {
			return true
		}
	}
	return false
}

// SupportsDataType checks if a connector supports a specific data type.
func SupportsDataType(conn Connector, dt DataType) bool {
	for _, t := range conn.SupportedDataTypes() {
		if t == dt {
			return true
		}
	}
	return false
}

// CanRead checks if a connector can read/retrieve data.
func CanRead(conn Connector) bool {
	return HasCapability(conn, CapabilityRead)
}

// CanWrite checks if a connector can write/push data.
func CanWrite(conn Connector) bool {
	return HasCapability(conn, CapabilityWrite)
}

// ValidateRoles checks if a connector can fulfill the given roles.
func ValidateRoles(conn Connector, roles ConnectorRoles) error {
	if roles.IsInput && !CanRead(conn) {
		return fmt.Errorf("connector %q cannot read data (required for input role)", conn.ID())
	}
	if roles.IsOutput && !CanWrite(conn) {
		return fmt.Errorf("connector %q cannot write data (required for output role)", conn.ID())
	}
	if roles.IsFallback && !HasCapability(conn, CapabilityRepair) && !CanRead(conn) {
		return fmt.Errorf("connector %q cannot repair or read data (required for fallback role)", conn.ID())
	}
	return nil
}

// ValidateRole checks if a connector can fulfill a given legacy role.
// Deprecated: Use ValidateRoles instead.
func ValidateRole(conn Connector, role ConnectorRole) error {
	return ValidateRoles(conn, RolesFromLegacy(role))
}
