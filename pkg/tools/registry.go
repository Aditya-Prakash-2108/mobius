package tools

import (
	"fmt"
	"sort"
	"sync"
)

// Registry manages the set of available tools for the agent harness.
// Registry must not be copied after it is first used.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry initializes an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *Registry) Register(tool Tool) {
	name := tool.Name()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	r.tools[name] = tool
}

// Get retrieves a tool by its unique name.
func (r *Registry) Get(name string) (Tool, error) {
	if name == "" {
		return nil, fmt.Errorf("tool name is required")
	}
	if r == nil {
		return nil, fmt.Errorf("tool '%s' not found in registry", name)
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool '%s' not found in registry", name)
	}
	return tool, nil
}

// List returns all registered tools sorted alphabetically by name.
func (r *Registry) List() []Tool {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Tool, 0, len(r.tools))
	for _, tool := range r.tools {
		result = append(result, tool)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result
}

// NewRegistrySubset returns a registry containing only the named tools.
// A nil base, a nil name list, or an empty name list returns an empty
// registry. Names that are not in base are skipped.
func NewRegistrySubset(base *Registry, names []string) *Registry {
	out := NewRegistry()
	if base == nil || len(names) == 0 {
		return out
	}
	base.mu.RLock()
	defer base.mu.RUnlock()
	for _, name := range names {
		tool, ok := base.tools[name]
		if !ok {
			continue
		}
		out.tools[name] = tool
	}
	return out
}

// NewDefaultRegistry creates a registry pre-loaded with all standard tools.
func NewDefaultRegistry(workDir string) *Registry {
	r := NewRegistry()
	r.Register(NewViewFileTool(workDir))
	r.Register(NewWriteTool(workDir))
	r.Register(NewListTool(workDir))
	r.Register(NewBashTool(workDir))
	r.Register(NewEditFileTool(workDir))
	r.Register(NewGrepTool(workDir))
	return r
}
