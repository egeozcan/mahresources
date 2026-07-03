package shortcodes

import (
	"reflect"
	"testing"
)

func TestCollectShortcodeParams(t *testing.T) {
	got := collectShortcodeParams(map[string]string{
		"saved":       "report",
		"param-tag":   "x",
		"param-since": "-7d",
		"limit":       "10",
	})
	want := map[string]string{"tag": "x", "since": "-7d"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestCollectShortcodeParams_None(t *testing.T) {
	if got := collectShortcodeParams(map[string]string{"query": "type = resource"}); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	// A bare "param-" (empty name) is ignored.
	if got := collectShortcodeParams(map[string]string{"param-": "x"}); got != nil {
		t.Fatalf("expected nil for empty param name, got %v", got)
	}
}
