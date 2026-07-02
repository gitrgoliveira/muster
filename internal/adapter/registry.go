package adapter

import (
	"fmt"

	"github.com/gitrgoliveira/muster/internal/core"
)

// Registry maps AgentID to its Adapter implementation.
// M2 registers only claude; additional adapters are added in M5.
type Registry struct {
	adapters map[core.AgentID]Adapter
}

// NewRegistry creates an empty Registry.
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[core.AgentID]Adapter)}
}

// NewRegistryWithDefaults creates a Registry with the standard M2 adapters registered.
// The caller provides the adapters to register (keeps this package free of adapter/claude import).
func NewRegistryWithDefaults(adapters ...Adapter) *Registry {
	r := NewRegistry()
	for _, a := range adapters {
		r.Register(a)
	}
	return r
}

// Register adds an adapter to the registry. Panics on duplicate ID
// (registrations happen at startup and duplicates are a programming error).
func (r *Registry) Register(a Adapter) {
	id := a.ID()
	if _, exists := r.adapters[id]; exists {
		panic(fmt.Sprintf("adapter %q already registered", id))
	}
	r.adapters[id] = a
}

// Get returns the adapter for the given AgentID, or false if not registered.
func (r *Registry) Get(id core.AgentID) (Adapter, bool) {
	a, ok := r.adapters[id]
	return a, ok
}

// All returns a snapshot of all registered adapters.
func (r *Registry) All() []Adapter {
	out := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		out = append(out, a)
	}
	return out
}
