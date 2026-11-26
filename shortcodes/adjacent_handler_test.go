package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAdjacent records every lookup and answers from a fixed result.
type fakeAdjacent struct {
	result AdjacentResult
	err    error
	calls  []AdjacentQuery
}

func (f *fakeAdjacent) resolve(_ context.Context, q AdjacentQuery) (AdjacentResult, error) {
	f.calls = append(f.calls, q)
	return f.result, f.err
}

func adjacentCtx(entityType string, id uint) MetaShortcodeContext {
	return MetaShortcodeContext{EntityType: entityType, EntityID: id}
}

func renderAdjacent(t *testing.T, f *fakeAdjacent, input string, ctx MetaShortcodeContext) string {
	t.Helper()
	reqCtx := context.Background()
	if f != nil {
		reqCtx = WithAdjacentResolver(reqCtx, f.resolve)
	}
	return Process(reqCtx, input, ctx, nil, nil)
}

func TestAdjacent_BothLinksExactMarkup(t *testing.T) {
	f := &fakeAdjacent{result: AdjacentResult{
		Prev: &AdjacentLink{ID: 12, Name: "Page 1"},
		Next: &AdjacentLink{ID: 14, Name: "Page 3"},
	}}
	got := renderAdjacent(t, f, `[adjacent]`, adjacentCtx("resource", 13))
	want := `<nav class="shortcode-adjacent flex flex-wrap justify-between gap-2" aria-label="Previous and next">` +
		`<a href="/resource?id=12" rel="prev" class="text-amber-700 hover:text-amber-900 underline"><span aria-hidden="true">←</span> Previous: Page 1</a>` +
		`<a href="/resource?id=14" rel="next" class="ml-auto text-amber-700 hover:text-amber-900 underline">Next: Page 3 <span aria-hidden="true">→</span></a>` +
		`</nav>`
	assert.Equal(t, want, got)
	require.Len(t, f.calls, 1)
	assert.Equal(t, AdjacentQuery{EntityType: "resource", EntityID: 13}, f.calls[0])
}

func TestAdjacent_OnlyOneNeighbourAndNone(t *testing.T) {
	prevOnly := renderAdjacent(t, &fakeAdjacent{result: AdjacentResult{Prev: &AdjacentLink{ID: 2, Name: "A"}}}, `[adjacent]`, adjacentCtx("note", 3))
	assert.Contains(t, prevOnly, `rel="prev"`)
	assert.NotContains(t, prevOnly, `rel="next"`)
	assert.Contains(t, prevOnly, `href="/note?id=2"`)

	nextOnly := renderAdjacent(t, &fakeAdjacent{result: AdjacentResult{Next: &AdjacentLink{ID: 4, Name: "B"}}}, `[adjacent]`, adjacentCtx("group", 3))
	assert.NotContains(t, nextOnly, `rel="prev"`)
	assert.Contains(t, nextOnly, `<a href="/group?id=4" rel="next" class="ml-auto `, "a lone Next link stays right-aligned")

	none := renderAdjacent(t, &fakeAdjacent{}, `x[adjacent]y`, adjacentCtx("note", 3))
	assert.Equal(t, "xy", none, "no neighbours renders no empty landmark")
}

func TestAdjacent_EscapingAndFallbacks(t *testing.T) {
	f := &fakeAdjacent{result: AdjacentResult{
		Prev: &AdjacentLink{ID: 12, Name: `<script>alert(1)</script>`},
		Next: &AdjacentLink{ID: 14, Name: "  \t "},
	}}
	got := renderAdjacent(t, f, `[adjacent label='Say "hi"']`, adjacentCtx("resource", 13))
	assert.NotContains(t, got, "<script>")
	assert.Contains(t, got, "Previous: &lt;script&gt;alert(1)&lt;/script&gt;")
	assert.Contains(t, got, `aria-label="Say &#34;hi&#34;"`)
	assert.Contains(t, got, "Next: Resource 14 ")
}

func TestAdjacent_UnavailableWithoutResolver(t *testing.T) {
	got := renderAdjacent(t, nil, `[adjacent]`, adjacentCtx("note", 3))
	assert.Equal(t, "<!-- mr:adjacent unavailable in this context -->", got)
}

func TestAdjacent_InvalidInputNeverCallsTheResolver(t *testing.T) {
	cases := map[string]struct {
		input string
		ctx   MetaShortcodeContext
		want  string
	}{
		"bad series":       {`[adjacent series="yes"]`, adjacentCtx("resource", 3), `series must be &#34;true&#34; or &#34;false&#34;`},
		"series on group":  {`[adjacent series="true"]`, adjacentCtx("group", 3), `series=&#34;true&#34; works only on resources`},
		"series on note":   {`[adjacent series="true"]`, adjacentCtx("note", 3), `series=&#34;true&#34; works only on resources`},
		"carrier category": {`[adjacent]`, adjacentCtx("category", 3), ""},
		"carrier noteType": {`[adjacent]`, adjacentCtx("noteType", 3), ""},
		"no entity id":     {`[adjacent]`, adjacentCtx("note", 0), ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := &fakeAdjacent{result: AdjacentResult{Next: &AdjacentLink{ID: 9, Name: "n"}}}
			got := renderAdjacent(t, f, c.input, c.ctx)
			if c.want == "" {
				assert.Equal(t, "", got)
			} else {
				assert.Contains(t, got, `class="shortcode-error`)
				assert.Contains(t, got, c.want)
			}
			assert.Empty(t, f.calls, "the resolver must not be called")
		})
	}

	// series="false" is the default spelled out, and series="true" on a resource
	// asks for series mode.
	f := &fakeAdjacent{}
	renderAdjacent(t, f, `[adjacent series="false"][adjacent series="true"]`, adjacentCtx("resource", 5))
	require.Len(t, f.calls, 2)
	assert.False(t, f.calls[0].Series)
	assert.True(t, f.calls[1].Series)
}

func TestAdjacent_Errors(t *testing.T) {
	budget := fmt.Errorf("%w: inline query budget exceeded (1 per page); refine templates or raise -mrql-page-query-budget", ErrAdjacentBudget)
	got := renderAdjacent(t, &fakeAdjacent{err: budget}, `[adjacent]`, adjacentCtx("note", 3))
	assert.Contains(t, got, "inline query budget exceeded (1 per page); refine templates or raise -mrql-page-query-budget")
	assert.NotContains(t, got, "adjacent: budget")

	timeout := fmt.Errorf("seek: %w", context.DeadlineExceeded)
	got = renderAdjacent(t, &fakeAdjacent{err: timeout}, `[adjacent]`, adjacentCtx("note", 3))
	assert.Contains(t, got, "lookup timed out")

	got = renderAdjacent(t, &fakeAdjacent{err: errors.New(`near "SELECT": syntax error`)}, `[adjacent]`, adjacentCtx("note", 3))
	assert.Contains(t, got, "lookup failed")
	assert.NotContains(t, got, "SELECT")
}

func TestAdjacent_FollowsTheSwitchedEntity(t *testing.T) {
	// Inside [using] the resolver is asked about the [using] target.
	exec := newUsingFixture()
	f := &fakeAdjacent{}
	reqCtx := WithAdjacentResolver(context.Background(), f.resolve)
	Process(reqCtx, `[using type="resource" id="42"][adjacent][/using]`, noteCtx(), nil, exec.exec)
	require.Len(t, f.calls, 1)
	assert.Equal(t, AdjacentQuery{EntityType: "resource", EntityID: 42}, f.calls[0])

	// And inside an [mrql] item, about that item.
	f = &fakeAdjacent{}
	reqCtx = WithAdjacentResolver(context.Background(), f.resolve)
	mrqlExec := func(context.Context, string, QueryOptions) (*QueryResult, error) {
		return &QueryResult{EntityType: "note", Mode: "flat", Items: []QueryResultItem{
			{EntityType: "note", EntityID: 77, Entity: &usingEntity{ID: 77, Name: "n77"}, Meta: []byte(`{}`)},
		}}, nil
	}
	Process(reqCtx, `[mrql query="type = note" format="custom"][adjacent][/mrql]`, noteCtx(), nil, mrqlExec)
	require.Len(t, f.calls, 1)
	assert.Equal(t, AdjacentQuery{EntityType: "note", EntityID: 77}, f.calls[0])
}

func TestAdjacent_RefusedInsideAReloadFace(t *testing.T) {
	f := &fakeAdjacent{result: AdjacentResult{Next: &AdjacentLink{ID: 9, Name: "n"}}}
	got := renderAdjacent(t, f, `[reload][adjacent][/reload]`, adjacentCtx("note", 3))
	assert.NotContains(t, got, "<nav", "a <button> takes phrasing content only")
	assert.Contains(t, got, "cannot be used inside a [reload] button face")
	assert.Empty(t, f.calls)
}

// Every <nav> one render emits has its own name, whether the template repeats
// [adjacent] or a per-card slot renders it once per card with the shared page
// context.
func TestAdjacent_LandmarkNamesAreUniquePerRender(t *testing.T) {
	f := &fakeAdjacent{result: AdjacentResult{Next: &AdjacentLink{ID: 13, Name: "Page 3"}}}
	reqCtx := WithAdjacentResolver(context.Background(), f.resolve)
	page := Process(reqCtx, `[adjacent][adjacent][adjacent label="Chapters"]`, adjacentCtx("resource", 12), nil, nil)
	card := Process(reqCtx, `[adjacent][adjacent label="Chapters"]`, adjacentCtx("resource", 40), nil, nil)

	assert.Contains(t, page, `aria-label="Previous and next">`)
	assert.Contains(t, page, `aria-label="Previous and next (2)">`)
	assert.Contains(t, page, `aria-label="Chapters">`)
	assert.Contains(t, card, `aria-label="Previous and next (3)">`)
	assert.Contains(t, card, `aria-label="Chapters (2)">`)

	// An author's own numbered label is never handed out again.
	mixed := Process(WithAdjacentResolver(context.Background(), f.resolve),
		`[adjacent label="Chapters"][adjacent label="Chapters (2)"][adjacent label="Chapters"]`, adjacentCtx("resource", 12), nil, nil)
	assert.Contains(t, mixed, `aria-label="Chapters">`)
	assert.Contains(t, mixed, `aria-label="Chapters (2)">`)
	assert.Contains(t, mixed, `aria-label="Chapters (3)">`)

	// A new render starts the names again.
	fresh := renderAdjacent(t, f, `[adjacent]`, adjacentCtx("resource", 12))
	assert.Contains(t, fresh, `aria-label="Previous and next">`)
}
