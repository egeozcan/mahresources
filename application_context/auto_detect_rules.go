package application_context

import (
	"encoding/json"
	"fmt"
	"math"
)

// AutoDetectRule represents a parsed auto-detect rule for a ResourceCategory.
type AutoDetectRule struct {
	ContentTypes  []string   `json:"contentTypes"`
	Width         *RangeRule `json:"width,omitempty"`
	Height        *RangeRule `json:"height,omitempty"`
	AspectRatio   *RangeRule `json:"aspectRatio,omitempty"`
	FileSize      *RangeRule `json:"fileSize,omitempty"`
	PixelCount    *RangeRule `json:"pixelCount,omitempty"`
	BytesPerPixel *RangeRule `json:"bytesPerPixel,omitempty"`
	Priority      int        `json:"priority,omitempty"`
}

// RangeRule represents a numeric range with optional min and max bounds.
type RangeRule struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

var knownFields = map[string]bool{
	"contentTypes": true, "width": true, "height": true,
	"aspectRatio": true, "fileSize": true, "pixelCount": true,
	"bytesPerPixel": true, "priority": true,
}

// ValidateAutoDetectRules validates the JSON string for auto-detect rules.
// Empty string is valid (no rules). Returns an error describing the problem.
func ValidateAutoDetectRules(rules string) error {
	if rules == "" {
		return nil
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(rules), &raw); err != nil {
		return fmt.Errorf("invalid JSON in auto-detect rules: %w", err)
	}

	for key := range raw {
		if !knownFields[key] {
			return fmt.Errorf("unknown field %q in auto-detect rules", key)
		}
	}

	var rule AutoDetectRule
	if err := json.Unmarshal([]byte(rules), &rule); err != nil {
		return fmt.Errorf("invalid auto-detect rules structure: %w", err)
	}

	if len(rule.ContentTypes) == 0 {
		return fmt.Errorf("contentTypes is required and must be a non-empty array")
	}

	if rawPriority, ok := raw["priority"]; ok {
		var f float64
		if err := json.Unmarshal(rawPriority, &f); err != nil {
			return fmt.Errorf("priority must be an integer")
		}
		if f != math.Trunc(f) {
			return fmt.Errorf("priority must be an integer, got %v", f)
		}
	}

	rangeFields := map[string]*RangeRule{
		"width": rule.Width, "height": rule.Height, "aspectRatio": rule.AspectRatio,
		"fileSize": rule.FileSize, "pixelCount": rule.PixelCount, "bytesPerPixel": rule.BytesPerPixel,
	}
	for name, r := range rangeFields {
		if r != nil && r.Min == nil && r.Max == nil {
			return fmt.Errorf("%s must have at least one of min or max", name)
		}
	}

	return nil
}
