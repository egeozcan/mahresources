package shortcodes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseMRQLSlotsBasic(t *testing.T) {
	inner := `[header]<h4>Open ({count})</h4>[/header]<li>[property path="Name"]</li>[footer]<p>live</p>[/footer][else]<p>none</p>`
	s := parseMRQLSlots(inner)

	assert.True(t, s.HasHeader)
	assert.Equal(t, `<h4>Open ({count})</h4>`, s.Header)
	assert.True(t, s.HasFooter)
	assert.Equal(t, `<p>live</p>`, s.Footer)
	assert.True(t, s.HasElse)
	assert.Equal(t, `<p>none</p>`, s.Else)
	assert.Equal(t, `<li>[property path="Name"]</li>`, s.Item)
}

func TestParseMRQLSlotsNoSlots(t *testing.T) {
	inner := `<span>[property path="Name"]</span>`
	s := parseMRQLSlots(inner)
	assert.False(t, s.HasHeader)
	assert.False(t, s.HasFooter)
	assert.False(t, s.HasElse)
	assert.Equal(t, inner, s.Item)
}

func TestParseMRQLSlotsOnlyElse(t *testing.T) {
	inner := `<li>[property path="Name"]</li>[else]<p>empty</p>`
	s := parseMRQLSlots(inner)
	assert.False(t, s.HasHeader)
	assert.True(t, s.HasElse)
	assert.Equal(t, `<p>empty</p>`, s.Else)
	assert.Equal(t, `<li>[property path="Name"]</li>`, s.Item)
}

func TestParseMRQLSlotsSkipsNestedBlockTags(t *testing.T) {
	// A [header] inside a nested [each] block must be left inside the item
	// template, not hoisted out as the mrql header.
	inner := `[each path="tags"][header]nested[/header][item][/each]<li>x</li>`
	s := parseMRQLSlots(inner)
	assert.False(t, s.HasHeader, "nested [header] must not be extracted")
	assert.Contains(t, s.Item, `[each path="tags"][header]nested[/header][item][/each]`)
}

func TestParseMRQLSlotsFirstOccurrenceOnly(t *testing.T) {
	inner := `[header]one[/header][header]two[/header]<li>x</li>`
	s := parseMRQLSlots(inner)
	assert.Equal(t, "one", s.Header)
	// The second header stays in the item template.
	assert.Contains(t, s.Item, `[header]two[/header]`)
}

func TestMentionsTotal(t *testing.T) {
	assert.True(t, parseMRQLSlots(`[header]{total} items[/header]<li>x</li>`).mentionsTotal())
	assert.True(t, parseMRQLSlots(`<li>x</li>[footer]{total}[/footer]`).mentionsTotal())
	assert.True(t, parseMRQLSlots(`<li>x</li>[else]none of {total}`).mentionsTotal())
	assert.False(t, parseMRQLSlots(`[header]{count} items[/header]<li>x</li>`).mentionsTotal())
}

func TestViewAllURLSaved(t *testing.T) {
	assert.Equal(t, "/mrql?saved=42", viewAllURL(&QueryResult{SavedID: 42, EffectiveQuery: "resources"}))
}

func TestViewAllURLInline(t *testing.T) {
	assert.Equal(t, "/mrql?q=resources+where+tag+%3D+%27x%27",
		viewAllURL(&QueryResult{EffectiveQuery: "resources where tag = 'x'"}))
}

func TestViewAllURLInlineWithScope(t *testing.T) {
	// Applied scope with no explicit SCOPE in the text → SCOPE appended.
	assert.Equal(t, "/mrql?q=resources+SCOPE+7",
		viewAllURL(&QueryResult{EffectiveQuery: "resources", LinkScopeGroupID: 7}))
}

func TestViewAllURLEmpty(t *testing.T) {
	assert.Equal(t, "", viewAllURL(&QueryResult{}))
}

func TestSubstitutePlaceholders(t *testing.T) {
	total := int64(57)
	result := &QueryResult{
		Mode:           "flat",
		Items:          make([]QueryResultItem, 3),
		Total:          &total,
		EffectiveQuery: "resources",
	}
	out := substitutePlaceholders(`{count} of {total} → {link-all}`, result)
	assert.Equal(t, "3 of 57 → /mrql?q=resources", out)
}

func TestSubstitutePlaceholdersTotalFallsBackToCount(t *testing.T) {
	result := &QueryResult{Mode: "flat", Items: make([]QueryResultItem, 4)}
	// No Total computed → {total} renders the count.
	assert.Equal(t, "4 / 4", substitutePlaceholders(`{count} / {total}`, result))
}
