// Package template_presets embeds the starter template bundles served to the
// category-template edit forms' "start from preset" picker. Each JSON file is a
// bundle in the same schemaVersion-1 shape the Export/Import tools use, so the
// preset picker routes through the exact same client-side import path.
package template_presets

import (
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

//go:embed *.json
var presetFS embed.FS

// Preset is a starter template bundle (schemaVersion 1 shape).
type Preset struct {
	SchemaVersion int               `json:"schemaVersion"`
	Name          string            `json:"name"`
	Title         string            `json:"title"`
	Carrier       string            `json:"carrier"`
	Description   string            `json:"description"`
	Slots         map[string]string `json:"slots"`
	MetaSchema    string            `json:"metaSchema"`
	SectionConfig string            `json:"sectionConfig"`
}

// All returns every embedded preset, sorted by name for stable ordering.
func All() ([]Preset, error) {
	entries, err := presetFS.ReadDir(".")
	if err != nil {
		return nil, err
	}
	presets := make([]Preset, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, readErr := presetFS.ReadFile(e.Name())
		if readErr != nil {
			return nil, readErr
		}
		var p Preset
		if err := json.Unmarshal(data, &p); err != nil {
			return nil, fmt.Errorf("preset %s: %w", e.Name(), err)
		}
		presets = append(presets, p)
	}
	sort.Slice(presets, func(i, j int) bool { return presets[i].Name < presets[j].Name })
	return presets, nil
}
