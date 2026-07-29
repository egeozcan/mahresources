package block_types

import (
	"sort"
	"sync"
)

var (
	registry = make(map[string]BlockType)
	mu       sync.RWMutex
)

// RegisterBlockType registers a block type in the global registry.
// Block types typically call this in their init() function.
// If a block type with the same name is already registered, it will be replaced.
func RegisterBlockType(bt BlockType) {
	mu.Lock()
	defer mu.Unlock()
	registry[bt.Type()] = bt
}

// UnregisterBlockType removes a block type from the registry.
// No-op if the type name is not registered.
func UnregisterBlockType(typeName string) {
	mu.Lock()
	defer mu.Unlock()
	delete(registry, typeName)
}

// GetBlockType returns a registered block type by its type name, or nil if not found.
func GetBlockType(typeName string) BlockType {
	mu.RLock()
	defer mu.RUnlock()
	return registry[typeName]
}

// GetAllBlockTypes returns all registered block types, ordered by type name.
//
// The order is part of the contract: this backs the "+ Add Block" listbox, and
// ranging the registry map gave a different rotation on every request. That
// broke muscle memory, made "the 2nd item" mean a different block each time,
// and — because the picker's roving tabindex is keyed on position — left
// keyboard users able to insert only whichever type randomly landed first.
func GetAllBlockTypes() []BlockType {
	mu.RLock()
	defer mu.RUnlock()
	types := make([]BlockType, 0, len(registry))
	for _, bt := range registry {
		types = append(types, bt)
	}
	sort.Slice(types, func(i, j int) bool { return types[i].Type() < types[j].Type() })
	return types
}
