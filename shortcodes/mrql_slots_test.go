package shortcodes

import (
	"sort"
	"strings"
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

func TestViewAllURLInlineScopeAlreadyInText(t *testing.T) {
	// The executor bakes any applied scope into EffectiveQuery, so viewAllURL
	// only URL-encodes it. A SCOPE clause in the text is carried verbatim.
	assert.Equal(t, "/mrql?q=type+%3D+%22resource%22+SCOPE+7",
		viewAllURL(&QueryResult{EffectiveQuery: `type = "resource" SCOPE 7`}))
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

// TestSlotTagSpansAreWhatTheHandlersConsume pins SlotTagSpans to the runtime
// by removing the reported spans and comparing with what each handler leaves.
func TestSlotTagSpansAreWhatTheHandlersConsume(t *testing.T) {
	cut := func(s string, spans [][2]int) string {
		var b strings.Builder
		prev := 0
		for _, sp := range spans {
			b.WriteString(s[prev:sp[0]])
			prev = sp[1]
		}
		return b.String() + s[prev:]
	}
	for _, inner := range []string{
		`[header]H[/header]I[footer]F[/footer][else]E`,
		`[header]A[/header][header]B[/header]I`,
		`[header]A[else]B[/header]I`,
		`[header class="x"]A[/header]I[footer]F`,
		`I[footer]F[/footer][header]H[/header]`,
	} {
		spans := SlotTagSpans("mrql", inner)
		s := parseMRQLSlots(inner)
		want := cut(inner, spans)
		got := s.Header + s.Item + s.Footer + s.Else
		// Order differs in the cut text; compare as multisets of bytes.
		if sortedBytes(want) != sortedBytes(got) {
			t.Errorf("mrql %q: spans leave %q, handler keeps %q", inner, want, got)
		}
	}
	for _, inner := range []string{`A[elseif x="1"]B[else]C`, `A[else]B[else]C`, `[lazy]a[else]b[/lazy]A`} {
		var want string
		for _, b := range SplitBranches(inner) {
			want += b.Content
		}
		if got := cut(inner, SlotTagSpans("conditional", inner)); got != want {
			t.Errorf("conditional %q: spans leave %q, handler keeps %q", inner, got, want)
		}
	}
	for _, inner := range []string{`A[else]B[else]C`, `[lazy]a[else]b[/lazy]A`, `A`} {
		a, b := SplitElse(inner)
		if got := cut(inner, SlotTagSpans("each", inner)); got != a+b {
			t.Errorf("each %q: spans leave %q, handler keeps %q", inner, got, a+b)
		}
	}
}

func sortedBytes(s string) string {
	b := []byte(s)
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	return string(b)
}
