package shortcodes

import (
	"context"
	"encoding/json"
	"html"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDateTimeShortcode(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "note", EntityID: 12,
		Meta:       json.RawMessage(`{"event":{"start":"not a date"}}`),
		MetaSchema: `{"type":"object","properties":{"event":{"type":"object","properties":{"start":{"type":"string","format":"date-time","default":"2026-09-05T14:00:00Z"}}}}}`,
	}
	source := `[datetime path="event.start" editable="true" layout="January 2, 2006" input-layout="2006-01-02T15:04:05Z07:00"]`
	result := Process(context.Background(), source, ctx, nil, nil)
	assert.Contains(t, result, `<meta-shortcode`)
	assert.Contains(t, result, `data-date-time="true"`)
	assert.Contains(t, result, `data-editable="true"`)
	assert.Contains(t, result, `data-layout="January 2, 2006"`)
	assert.Contains(t, result, `data-input-layout="2006-01-02T15:04:05Z07:00"`)
	assert.Contains(t, html.UnescapeString(result), `"default":"2026-09-05T14:00:00Z"`)
	assert.Contains(t, html.UnescapeString(result), `"not a date"`)
	assert.Empty(t, Lint(source, LintOptions{Known: KnownFromBuiltins()}))

	ctx.ForceReadOnly = true
	assert.Contains(t, Process(context.Background(), source, ctx, nil, nil), `data-editable="false"`)
	assert.Empty(t, Process(context.Background(), `[datetime]`, ctx, nil, nil))
}

func TestDateTimeEscapesValuesAndLayouts(t *testing.T) {
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"when":"<img src=x onerror=alert(1)>"}`)}
	result := Process(context.Background(), `[datetime path="when" layout='" onmouseover="bad' input-layout='" onclick="bad']`, ctx, nil, nil)
	assert.NotContains(t, result, `<img`)
	assert.NotContains(t, result, `" onmouseover="bad`)
	assert.NotContains(t, result, `" onclick="bad`)
	assert.Contains(t, result, `\u003cimg`)
}
