package template_presets

import (
	"testing"

	"mahresources/shortcodes"
)

func TestAllPresetsParseAndAreShortcodeClean(t *testing.T) {
	presets, err := All()
	if err != nil {
		t.Fatalf("All() error: %v", err)
	}
	if len(presets) < 4 {
		t.Fatalf("expected at least 4 presets, got %d", len(presets))
	}

	validCarrier := map[string]bool{"category": true, "resourceCategory": true, "noteType": true}
	known := shortcodes.KnownFromBuiltins()
	seen := map[string]bool{}

	for _, p := range presets {
		if p.SchemaVersion != 1 {
			t.Errorf("preset %q: schemaVersion = %d, want 1", p.Name, p.SchemaVersion)
		}
		if p.Name == "" || p.Title == "" {
			t.Errorf("preset %q: name/title must be non-empty", p.Name)
		}
		if seen[p.Name] {
			t.Errorf("duplicate preset name %q", p.Name)
		}
		seen[p.Name] = true
		if !validCarrier[p.Carrier] {
			t.Errorf("preset %q: invalid carrier %q", p.Name, p.Carrier)
		}

		// Every slot's shortcodes must lint without errors — presets double as a
		// regression fixture for the shortcode language.
		for slot, content := range p.Slots {
			for _, issue := range shortcodes.Lint(content, shortcodes.LintOptions{Known: known}) {
				if issue.Severity == shortcodes.SeverityError {
					t.Errorf("preset %q slot %q: lint error: %s", p.Name, slot, issue.Message)
				}
			}
		}
	}
}
