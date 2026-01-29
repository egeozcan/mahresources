package block_types

import "sync"

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

// GetBlockType returns a registered block type by its type name, or nil if not found.
func GetBlockType(typeName string) BlockType {
	mu.RLock()
	defer mu.RUnlock()
	return registry[typeName]
}

// GetAllBlockTypes returns all registered block types.
// The order of returned types is not guaranteed.
func GetAllBlockTypes() []BlockType {
	mu.RLock()
	defer mu.RUnlock()
	types := make([]BlockType, 0, len(registry))
	for _, bt := range registry {
		types = append(types, bt)
	}
	return types
}
