package shortcodes

import (
	"context"
	"encoding/json"
	"html"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPillsShortcode(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group", EntityID: 12,
		Meta:       json.RawMessage(`{"task":{"priority":2}}`),
		MetaSchema: `{"type":"object","properties":{"task":{"properties":{"priority":{"$ref":"#/$defs/priority"}}}},"$defs":{"priority":{"oneOf":[{"const":1,"title":"Low"},{"const":2,"title":"High"}]}}}`,
	}
	for _, options := range []string{"", ` options='[{"value":1,"label":"Low"},{"value":2,"label":"High"}]'`} {
		source := `[pills path="task.priority" editable="true"` + options + `]`
		result := Process(context.Background(), source, ctx, nil, nil)
		assert.Contains(t, result, `data-pills="true"`)
		assert.Contains(t, result, `data-editable="true"`)
		assert.Contains(t, result, `data-value="2"`)
		assert.Contains(t, result, `data-entity-type="group" data-entity-id="12"`)
		assert.Contains(t, html.UnescapeString(result), `"const":1,"title":"Low"`)
		if options != "" {
			assert.Contains(t, html.UnescapeString(result), `data-options="[{"value":1,"label":"Low"},{"value":2,"label":"High"}]"`)
		}
		assert.Empty(t, Lint(source, LintOptions{Known: KnownFromBuiltins()}))
	}
	ctx.ForceReadOnly = true
	assert.Contains(t, Process(context.Background(), `[pills path="task.priority" editable="true"]`, ctx, nil, nil), `data-editable="false"`)
	assert.Empty(t, Process(context.Background(), `[pills]`, ctx, nil, nil))
}

func TestPillsOptionsAreEscaped(t *testing.T) {
	result := Process(context.Background(), `[pills path="priority" options='[{"value":"x","label":"<img src=x onerror=alert(1)>"}]']`, MetaShortcodeContext{}, nil, nil)
	assert.NotContains(t, result, `<img`)
	assert.Contains(t, result, `&lt;img`)
	assert.Contains(t, result, `&#34;label&#34;`)
}
