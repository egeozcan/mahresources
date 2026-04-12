package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcessNoShortcodes(t *testing.T) {
	result := Process(context.Background(), "<div>hello</div>", MetaShortcodeContext{}, nil, nil)
	assert.Equal(t, "<div>hello</div>", result)
}

func TestProcessMetaShortcode(t *testing.T) {
	meta := map[string]any{"name": "test"}
	metaJSON, _ := json.Marshal(meta)

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
	}

	result := Process(context.Background(), `before [meta path="name"] after`, ctx, nil, nil)
	assert.Contains(t, result, "before ")
	assert.Contains(t, result, "<meta-shortcode")
	assert.Contains(t, result, " after")
	assert.NotContains(t, result, "[meta")
}

func TestProcessMixedHTMLAndShortcodes(t *testing.T) {
	meta := map[string]any{"a": 1, "b": 2}
	metaJSON, _ := json.Marshal(meta)

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
	}

	input := `<div class="flex gap-2">[meta path="a"]<span>sep</span>[meta path="b"]</div>`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Contains(t, result, `<div class="flex gap-2">`)
	assert.Contains(t, result, `<span>sep</span>`)
	assert.Contains(t, result, `data-path="a"`)
	assert.Contains(t, result, `data-path="b"`)
}

func TestProcessPluginShortcode(t *testing.T) {
	renderer := func(name string, sc Shortcode, ctx MetaShortcodeContext) (string, error) {
		return "<div>plugin output</div>", nil
	}

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
	}

	result := Process(context.Background(), `[plugin:test:widget size="large"]`, ctx, renderer, nil)
	assert.Equal(t, "<div>plugin output</div>", result)
}

func TestProcessPluginShortcodeError(t *testing.T) {
	renderer := func(name string, sc Shortcode, ctx MetaShortcodeContext) (string, error) {
		return "", fmt.Errorf("render error")
	}

	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
	}

	// On error, the original shortcode text is preserved
	result := Process(context.Background(), `[plugin:test:widget]`, ctx, renderer, nil)
	assert.Equal(t, `[plugin:test:widget]`, result)
}

func TestProcessWithNilExecutor(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       []byte(`{}`),
	}
	result := Process(context.Background(), "<p>hello</p>", ctx, nil, nil)
	assert.Equal(t, "<p>hello</p>", result)
}

func TestProcessBlockConditionalTrue(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"active"}`),
	}
	input := `before[conditional path="status" eq="active"]<b>yes</b>[/conditional]after`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, "before<b>yes</b>after", result)
}

func TestProcessBlockConditionalFalse(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"inactive"}`),
	}
	input := `[conditional path="status" eq="active"]<b>yes</b>[/conditional]`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, "", result)
}

func TestProcessBlockConditionalElse(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"draft"}`),
	}
	input := `[conditional path="status" eq="active"]yes[else]no[/conditional]`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, "no", result)
}

func TestProcessBlockWithNestedSelfClosing(t *testing.T) {
	meta := map[string]any{"status": "active", "name": "test"}
	metaJSON, _ := json.Marshal(meta)
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
	}
	input := `[conditional path="status" eq="active"][meta path="name"][/conditional]`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Contains(t, result, "<meta-shortcode")
	assert.Contains(t, result, `data-path="name"`)
}

func TestProcessBlockPluginGetsInnerContent(t *testing.T) {
	var receivedInner string
	var receivedIsBlock bool
	renderer := func(name string, sc Shortcode, ctx MetaShortcodeContext) (string, error) {
		receivedInner = sc.InnerContent
		receivedIsBlock = sc.IsBlock
		return "<div>" + sc.InnerContent + "</div>", nil
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	input := `[plugin:test:wrap]hello world[/plugin:test:wrap]`
	result := Process(context.Background(), input, ctx, renderer, nil)
	assert.Equal(t, "hello world", receivedInner)
	assert.True(t, receivedIsBlock)
	assert.Equal(t, "<div>hello world</div>", result)
}

func TestProcessBlockDepthLimit(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"a":"1"}`),
	}
	inner := "deep"
	for i := 0; i < 12; i++ {
		inner = fmt.Sprintf(`[conditional path="a" eq="1"]%s[/conditional]`, inner)
	}
	result := Process(context.Background(), inner, ctx, nil, nil)
	assert.Contains(t, result, "deep")
}
