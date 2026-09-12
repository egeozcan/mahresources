package shortcodes

import (
	"context"
	"errors"
	"testing"
)

func TestProcessWithDiagnosticsKeepsLegacyOutputAndNestedErrors(t *testing.T) {
	renderer := func(_ string, sc Shortcode, _ MetaShortcodeContext) (string, error) {
		if sc.Name == "plugin:test:outer" {
			return "[plugin:test:inner]", nil
		}
		return "", errors.New("failed render")
	}
	input := "[plugin:test:outer]"
	got := ProcessWithDiagnostics(context.Background(), input, MetaShortcodeContext{}, renderer, nil)
	if got.HTML != Process(context.Background(), input, MetaShortcodeContext{}, renderer, nil) || len(got.Errors) == 0 {
		t.Fatalf("lost output/error: %+v", got)
	}
	neutral := func(string, Shortcode, MetaShortcodeContext) (string, error) { return "", ErrPluginUnavailable }
	if got := ProcessWithDiagnostics(context.Background(), input, MetaShortcodeContext{}, neutral, nil); len(got.Errors) != 0 {
		t.Fatal(got.Errors)
	}
}
