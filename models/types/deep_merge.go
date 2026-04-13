package types

// DeepMergeJSON recursively merges incoming into base. Incoming keys
// overwrite base keys. When both base and incoming values for the same
// key are map[string]any, they are merged recursively. Otherwise,
// the incoming value wins. Neither input is mutated; a new map is returned.
func DeepMergeJSON(base, incoming map[string]any) map[string]any {
	if base == nil {
		return incoming
	}
	if incoming == nil {
		return base
	}
	result := make(map[string]any, len(base)+len(incoming))
	for k, v := range base {
		result[k] = v
	}
	for k, v := range incoming {
		if vMap, ok := v.(map[string]any); ok {
			if bMap, ok := result[k].(map[string]any); ok {
				result[k] = DeepMergeJSON(bMap, vMap)
				continue
			}
		}
		result[k] = v
	}
	return result
}
