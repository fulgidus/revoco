package core

import (
	"fmt"
	"sync"
)

var (
	// Global registry instances
	serviceRegistry = &Registry[Service]{items: make(map[string]Service)}
	outputRegistry  = &Registry[Output]{items: make(map[string]Output)}
	authRegistry    = &Registry[AuthProvider]{items: make(map[string]AuthProvider)}
)

// Registry holds registered modules of a specific type.
type Registry[T any] struct {
	mu    sync.RWMutex
	items map[string]T
}

// Register adds a module to the registry.
func (r *Registry[T]) Register(id string, item T) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.items[id]; exists {
		return fmt.Errorf("module %q already registered", id)
	}
	r.items[id] = item
	return nil
}

// Get retrieves a module by ID.
func (r *Registry[T]) Get(id string) (T, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	item, ok := r.items[id]
	return item, ok
}

// List returns all registered modules.
func (r *Registry[T]) List() []T {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]T, 0, len(r.items))
	for _, item := range r.items {
		result = append(result, item)
	}
	return result
}

// IDs returns all registered module IDs.
func (r *Registry[T]) IDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]string, 0, len(r.items))
	for id := range r.items {
		result = append(result, id)
	}
	return result
}

// ── Service Registry ─────────────────────────────────────────────────────────

// RegisterService registers a service globally.
func RegisterService(s Service) error {
	return serviceRegistry.Register(s.ID(), s)
}

// GetService retrieves a service by ID.
func GetService(id string) (Service, bool) {
	return serviceRegistry.Get(id)
}

// ListServices returns all registered services.
func ListServices() []Service {
	return serviceRegistry.List()
}

// ServiceIDs returns all registered service IDs.
func ServiceIDs() []string {
	return serviceRegistry.IDs()
}

// ── Output Registry ──────────────────────────────────────────────────────────

// RegisterOutput registers an output module globally.
func RegisterOutput(o Output) error {
	return outputRegistry.Register(o.ID(), o)
}

// GetOutput retrieves an output module by ID.
func GetOutput(id string) (Output, bool) {
	return outputRegistry.Get(id)
}

// ListOutputs returns all registered output modules.
func ListOutputs() []Output {
	return outputRegistry.List()
}

// OutputIDs returns all registered output module IDs.
func OutputIDs() []string {
	return outputRegistry.IDs()
}

// ── Auth Provider Registry ───────────────────────────────────────────────────

// RegisterAuthProvider registers an auth provider globally.
func RegisterAuthProvider(a AuthProvider) error {
	return authRegistry.Register(a.ID(), a)
}

// GetAuthProvider retrieves an auth provider by ID.
func GetAuthProvider(id string) (AuthProvider, bool) {
	return authRegistry.Get(id)
}

// ListAuthProviders returns all registered auth providers.
func ListAuthProviders() []AuthProvider {
	return authRegistry.List()
}

// AuthProviderIDs returns all registered auth provider IDs.
func AuthProviderIDs() []string {
	return authRegistry.IDs()
}
