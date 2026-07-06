package shortcodes

import (
	"context"
	"encoding/json"
	"net/url"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/types"

	"github.com/stretchr/testify/assert"
)

type testEntity struct {
	ID          uint
	Name        string
	Description string
	CreatedAt   time.Time
	Tags        []string
	Meta        json.RawMessage
}

func TestPropertyShortcodeStringField(t *testing.T) {
	entity := testEntity{ID: 1, Name: "My Resource"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "My Resource", result)
}

func TestPropertyShortcodeHTMLEscaped(t *testing.T) {
	entity := testEntity{ID: 1, Name: `<script>alert("xss")</script>`}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "&lt;script&gt;alert(&#34;xss&#34;)&lt;/script&gt;", result)
	assert.NotContains(t, result, "<script>")
}

func TestPropertyShortcodeRawAttribute(t *testing.T) {
	entity := testEntity{ID: 1, Description: "<b>bold</b> text"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Description", "raw": "true"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "<b>bold</b> text", result)
}

func TestPropertyShortcodeUintField(t *testing.T) {
	entity := testEntity{ID: 42, Name: "Test"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "ID"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "42", result)
}

func TestPropertyShortcodeSliceField(t *testing.T) {
	entity := testEntity{ID: 1, Tags: []string{"photo", "landscape"}}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Tags"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "photo, landscape", result)
}

func TestPropertyShortcodeSliceHTMLEscaped(t *testing.T) {
	entity := testEntity{ID: 1, Tags: []string{"<b>bold</b>", "normal"}}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Tags"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "&lt;b&gt;bold&lt;/b&gt;, normal", result)
}

func TestPropertyShortcodeTimeField(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	entity := testEntity{ID: 1, CreatedAt: ts}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Contains(t, result, "2026")
}

func TestPropertyShortcodeMissingPath(t *testing.T) {
	entity := testEntity{ID: 1, Name: "Test"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "", result)
}

func TestPropertyShortcodeInvalidField(t *testing.T) {
	entity := testEntity{ID: 1, Name: "Test"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "NonExistent"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "", result)
}

func TestPropertyShortcodeNilEntity(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: nil}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "", result)
}

func TestPropertyShortcodePointerEntity(t *testing.T) {
	entity := &testEntity{ID: 1, Name: "Pointer Entity"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "Pointer Entity", result)
}

// --- Phase 2: dot-path traversal, format, default ---

type traversalOwner struct {
	Name string
	ID   uint
}

type traversalTag struct {
	Name string
}

type traversalEntity struct {
	ID        uint
	Name      string
	Owner     *traversalOwner
	Tags      []*traversalTag
	CreatedAt time.Time
	FileSize  int64
	Count     int
	URL       *types.URL
}

func TestPropertyShortcodeDotPath(t *testing.T) {
	entity := traversalEntity{Owner: &traversalOwner{Name: "Alice"}}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Owner.Name"}}
	assert.Equal(t, "Alice", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDotPathNilPointer(t *testing.T) {
	entity := traversalEntity{Owner: nil}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Owner.Name"}}
	assert.Equal(t, "", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDotPathMissingSegment(t *testing.T) {
	entity := traversalEntity{Owner: &traversalOwner{Name: "Alice"}}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Owner.Nope"}}
	assert.Equal(t, "", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeSliceIndex(t *testing.T) {
	entity := traversalEntity{Tags: []*traversalTag{{Name: "photo"}, {Name: "landscape"}}}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Tags.1.Name"}}
	assert.Equal(t, "landscape", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeSliceIndexOutOfRange(t *testing.T) {
	entity := traversalEntity{Tags: []*traversalTag{{Name: "photo"}}}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Tags.5.Name"}}
	assert.Equal(t, "", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDefault(t *testing.T) {
	entity := traversalEntity{Owner: nil}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Owner.Name", "default": "Unassigned"}}
	assert.Equal(t, "Unassigned", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDefaultNotUsedWhenPresent(t *testing.T) {
	entity := traversalEntity{Name: "Real"}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name", "default": "Fallback"}}
	assert.Equal(t, "Real", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDefaultHTMLEscaped(t *testing.T) {
	ctx := MetaShortcodeContext{Entity: traversalEntity{}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name", "default": "<b>x</b>"}}
	assert.Equal(t, "&lt;b&gt;x&lt;/b&gt;", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDefaultRaw(t *testing.T) {
	ctx := MetaShortcodeContext{Entity: traversalEntity{}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name", "default": "<b>x</b>", "raw": "true"}}
	assert.Equal(t, "<b>x</b>", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeFormatDate(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	ctx := MetaShortcodeContext{Entity: traversalEntity{CreatedAt: ts}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt", "format": "date"}}
	assert.Equal(t, "2026-04-09", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeFormatDateTime(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	ctx := MetaShortcodeContext{Entity: traversalEntity{CreatedAt: ts}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt", "format": "datetime"}}
	assert.Equal(t, "2026-04-09 12:30", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeFormatTime(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	ctx := MetaShortcodeContext{Entity: traversalEntity{CreatedAt: ts}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt", "format": "time"}}
	assert.Equal(t, "12:30", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeLayoutWinsOverFormat(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	ctx := MetaShortcodeContext{Entity: traversalEntity{CreatedAt: ts}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt", "format": "date", "layout": "Jan 2, 2006"}}
	assert.Equal(t, "Apr 9, 2026", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeFormatFilesize(t *testing.T) {
	ctx := MetaShortcodeContext{Entity: traversalEntity{FileSize: 1536}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "FileSize", "format": "filesize"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Contains(t, result, "1.5")
	assert.Contains(t, result, "KB")
}

func TestPropertyShortcodeFormatUnknownPassesThrough(t *testing.T) {
	ctx := MetaShortcodeContext{Entity: traversalEntity{Name: "Hello"}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name", "format": "bogus"}}
	assert.Equal(t, "Hello", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeFormatDateOnNonTimePassesThrough(t *testing.T) {
	ctx := MetaShortcodeContext{Entity: traversalEntity{Name: "Hello"}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name", "format": "date"}}
	assert.Equal(t, "Hello", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeURLStringer(t *testing.T) {
	parsed, err := url.Parse("https://example.com/profile?tab=social#links")
	assert.NoError(t, err)
	u := types.URL(*parsed)

	ctx := MetaShortcodeContext{Entity: traversalEntity{URL: &u}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "URL"}}
	assert.Equal(t, "https://example.com/profile?tab=social#links", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeProcessRealGroupURL(t *testing.T) {
	parsed, err := url.Parse("https://example.com/profile?tab=social#links")
	assert.NoError(t, err)
	u := types.URL(*parsed)
	group := models.Group{ID: 7, URL: &u}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7, Entity: group}

	result := Process(
		context.Background(),
		`<a href="[property path="URL"]">[property path="URL"]</a>`,
		ctx,
		nil,
		nil,
	)

	assert.Equal(t, `<a href="https://example.com/profile?tab=social#links">https://example.com/profile?tab=social#links</a>`, result)
}
