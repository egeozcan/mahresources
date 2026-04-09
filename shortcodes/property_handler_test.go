package shortcodes

import (
	"encoding/json"
	"testing"
	"time"

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
