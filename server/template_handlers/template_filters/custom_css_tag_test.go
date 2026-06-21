package template_filters

import (
	"strings"
	"testing"

	"github.com/flosch/pongo2/v4"
	"mahresources/models"
)

func renderCustomCSS(t *testing.T, src string, ctx pongo2.Context) string {
	t.Helper()
	tpl, err := pongo2.FromString(src)
	if err != nil {
		t.Fatalf("failed to compile template %q: %v", src, err)
	}
	out, err := tpl.Execute(ctx)
	if err != nil {
		t.Fatalf("failed to execute template %q: %v", src, err)
	}
	return out
}

func grp(id uint, catID uint, css string) *models.Group {
	g := &models.Group{ID: id}
	if catID != 0 {
		g.Category = &models.Category{ID: catID, CustomCSS: css}
	}
	return g
}

// A single entity emits exactly one <style> block carrying its category's CustomCSS verbatim.
func TestCustomCSS_SingleEntity(t *testing.T) {
	out := renderCustomCSS(t, `{% custom_css group %}`, pongo2.Context{
		"group": grp(7, 3, ".hdr{color:red}"),
	})
	want := `<style data-mr-custom-css="group:3">.hdr{color:red}</style>`
	if out != want {
		t.Fatalf("single entity: got %q, want %q", out, want)
	}
}

// CSS is emitted UNESCAPED — selectors with '>' and quoted content must survive (pongo2 would
// HTML-escape these via {{ }}, which is exactly why a dedicated tag exists).
func TestCustomCSS_RawUnescaped(t *testing.T) {
	raw := `a > b::after { content: "›" } .x{--q:'"'}`
	out := renderCustomCSS(t, `{% custom_css group %}`, pongo2.Context{
		"group": grp(1, 1, raw),
	})
	if !strings.Contains(out, raw) {
		t.Fatalf("expected raw CSS preserved, got %q", out)
	}
	if strings.Contains(out, "&gt;") || strings.Contains(out, "&quot;") || strings.Contains(out, "&#34;") {
		t.Fatalf("CSS was HTML-escaped: %q", out)
	}
}

// A collection emits one block per DISTINCT category (deduped), skips entities whose category has
// no CustomCSS, and skips entities with no category — all without panicking.
func TestCustomCSS_CollectionDedup(t *testing.T) {
	out := renderCustomCSS(t, `{% custom_css groups %}`, pongo2.Context{
		"groups": []*models.Group{
			grp(1, 3, ".a{}"),
			grp(2, 3, ".a{}"), // same category 3 -> deduped
			grp(3, 5, ".b{}"),
			grp(4, 9, ""),     // category present but empty CSS -> skipped
			grp(5, 0, ".c{}"), // no category -> skipped
		},
	})
	if got := strings.Count(out, "<style"); got != 2 {
		t.Fatalf("expected 2 deduped <style> blocks, got %d in %q", got, out)
	}
	if !strings.Contains(out, `data-mr-custom-css="group:3"`) || !strings.Contains(out, ".a{}") {
		t.Fatalf("missing category 3 block: %q", out)
	}
	if !strings.Contains(out, `data-mr-custom-css="group:5"`) || !strings.Contains(out, ".b{}") {
		t.Fatalf("missing category 5 block: %q", out)
	}
	if strings.Contains(out, ".c{}") {
		t.Fatalf("category-less entity should be skipped: %q", out)
	}
}

// An empty collection, a nil value, and a value of an unsupported type all render to nothing.
func TestCustomCSS_EmptyAndNil(t *testing.T) {
	cases := map[string]pongo2.Context{
		"empty slice": {"groups": []*models.Group{}},
		"nil":         {"groups": nil},
		"missing key": {},
	}
	for name, ctx := range cases {
		if out := renderCustomCSS(t, `{% custom_css groups %}`, ctx); out != "" {
			t.Fatalf("%s: expected empty output, got %q", name, out)
		}
	}
}

// Dedup is shared across multiple custom_css tags on the same render (page-level seen set), so a
// category already emitted for the detail entity is not repeated for the surrounding collection.
func TestCustomCSS_SharedDedupAcrossTags(t *testing.T) {
	out := renderCustomCSS(t, `{% custom_css group %}{% custom_css groups %}`, pongo2.Context{
		"group":  grp(1, 3, ".a{}"),
		"groups": []*models.Group{grp(2, 3, ".a{}"), grp(3, 4, ".d{}")},
	})
	if got := strings.Count(out, "<style"); got != 2 {
		t.Fatalf("expected 2 blocks (category 3 emitted once, category 4 once), got %d in %q", got, out)
	}
}
