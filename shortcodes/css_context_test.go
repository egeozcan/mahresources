package shortcodes

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestStylesheetBareOpenersUseSelectorCharacters(t *testing.T) {
	const selectors = `form[action]{x:1}.w[tabs]{x:2}a0[excerpt]{x:3}a-[pivot]{x:4}a_[relations]{x:5}*[link]{x:6}.a[x][property]{x:7}svg|[meta ]{x:8}`
	const value = `[property path="Name"]`
	const style = `<STYLE data-value="a[item]" data-test=">">` + selectors + ` .v{--name:` + value + `}</STYLE>`
	template := `<script>const text = 'not a <style>a[item]';</script>` + style + `<p>a[item]` + value + `!</p>`
	want := strings.ReplaceAll(strings.ReplaceAll(template, value, "Item"), "a[item]", "a")
	got := Process(context.Background(), template, MetaShortcodeContext{Entity: testEntity{Name: "Item"}}, nil, nil)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStylesheetContextFollowsShortcodeBodies(t *testing.T) {
	const template = `<style>[each path="xs"].a[item]{--text:'[item path="name"]'}[/each]</style>`
	meta := MetaShortcodeContext{Meta: []byte(`{"xs":[{"name":"test"}]}`)}
	got := Process(context.Background(), template, meta, nil, nil)
	if want := `<style>.a[item]{--text:'test'}</style>`; got != want {
		t.Errorf("got %q, want %q", got, want)
	}

	// The outer source uses the whole budget. The stylesheet inherits that
	// accounting and cannot start a fresh allowance for the shortcode body.
	ctx := WithRenderBudget(context.Background(), RenderBudget{ParsedSourceBytes: len(template)})
	if _, err := ProcessChecked(ctx, template, meta, nil, nil); !errors.Is(err, ErrRenderBudgetExceeded) {
		t.Errorf("nested stylesheet must share the render budget, got %v", err)
	}
}

func TestLintTreatsStylesheetBareOpenersAsSelectors(t *testing.T) {
	const css = `form[action]{x:1}a0[action]{x:2}a-[action]{x:3}a_[action]{x:4}*[action]{x:5}[data-x][tabs]{x:6}svg|[pivot ]{x:7}`
	for _, tc := range []struct {
		src     string
		cssMode bool
	}{{css, true}, {"<style>" + css + "</style>", false}} {
		if issues := Lint(tc.src, LintOptions{Known: KnownFromBuiltins(), CSSMode: tc.cssMode}); len(issues) != 0 {
			t.Errorf("lint diagnosed selectors in %q: %+v", tc.src, issues)
		}
		interpolation := strings.Replace(tc.src, "x:1", `x:[property path="Name"]`, 1)
		if issues := Lint(interpolation, LintOptions{Known: KnownFromBuiltins(), CSSMode: tc.cssMode}); len(issues) == 0 {
			t.Error("lint failed to warn about an actual CSS interpolation")
		}
	}
}
