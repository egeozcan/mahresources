package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// fakeSimilarExecutor answers every [similar] query with a fixed result and
// records the calls. lookup, when set, answers the [using] resource lookup.
type fakeSimilarExecutor struct {
	result *QueryResult
	err    error
	lookup map[string]QueryResultItem // [using] id → resource
	calls  []usingCall
}

func (f *fakeSimilarExecutor) exec(_ context.Context, query string, opts QueryOptions) (*QueryResult, error) {
	f.calls = append(f.calls, usingCall{query, opts})
	if f.err != nil {
		return nil, f.err
	}
	if query == usingLookupQuery("resource") {
		if item, ok := f.lookup[opts.Params["id"]]; ok {
			return &QueryResult{EntityType: "resource", Mode: "flat", Items: []QueryResultItem{item}}, nil
		}
		return &QueryResult{Mode: "flat"}, nil
	}
	return f.result, nil
}

func intp(v int) *int { return &v }

func simItem(id uint, name, hash string, d *int) QueryResultItem {
	return QueryResultItem{
		EntityType:         "resource",
		EntityID:           id,
		Entity:             &models.Resource{ID: id, Name: name, Hash: hash},
		SimilarityDistance: d,
	}
}

func similarCtx() MetaShortcodeContext {
	return MetaShortcodeContext{
		EntityType: "resource",
		EntityID:   42,
		Entity:     &models.Resource{ID: 42, Name: "Beach sunset"},
	}
}

func renderSimilar(t *testing.T, attrs map[string]string, ctx MetaShortcodeContext, f *fakeSimilarExecutor) string {
	t.Helper()
	if attrs == nil {
		attrs = map[string]string{}
	}
	return RenderSimilarShortcode(context.Background(), Shortcode{Name: "similar", Attrs: attrs}, ctx, f.exec)
}

func flat(items ...QueryResultItem) *QueryResult {
	return &QueryResult{EntityType: "resource", Mode: "flat", Items: items}
}

func TestSimilarQueryAndOptions(t *testing.T) {
	f := &fakeSimilarExecutor{result: flat()}
	renderSimilar(t, nil, similarCtx(), f)
	require.Len(t, f.calls, 1)
	assert.Equal(t, "type = resource AND SIMILAR TO resource(42) ORDER BY distance ASC", f.calls[0].query)
	assert.Equal(t, QueryOptions{Limit: 8, ScopeGroupID: 0, WantSimilarityDistance: true}, f.calls[0].opts)

	for in, want := range map[string]string{"6": " WITHIN 6", "0": " WITHIN 0", "11": " WITHIN 11"} {
		f := &fakeSimilarExecutor{result: flat()}
		renderSimilar(t, map[string]string{"max-distance": in}, similarCtx(), f)
		require.Len(t, f.calls, 1)
		assert.Equal(t, "type = resource AND SIMILAR TO resource(42)"+want+" ORDER BY distance ASC", f.calls[0].query)
	}

	f = &fakeSimilarExecutor{result: flat()}
	renderSimilar(t, map[string]string{"limit": "24"}, similarCtx(), f)
	assert.Equal(t, 24, f.calls[0].opts.Limit)
}

func TestSimilarMarkup(t *testing.T) {
	f := &fakeSimilarExecutor{result: flat(
		simItem(17, "Beach sunset (2)", `ab&c"d`, intp(1)),
		simItem(18, "Dunes", "h18", intp(5)),
	)}
	out := renderSimilar(t, nil, similarCtx(), f)

	assert.Equal(t, 1, strings.Count(out, "<figure "), out)
	assert.Contains(t, out, `<figure class="similar-strip my-2" data-similar-for="42">`)
	assert.Contains(t, out, `<figcaption class="text-xs font-semibold mb-1">Similar resources</figcaption>`)
	assert.Equal(t, 1, strings.Count(out, `<ul class="flex flex-wrap gap-2" role="list">`))
	assert.Equal(t, 2, strings.Count(out, "<li "))
	assert.Less(t, strings.Index(out, "id=17"), strings.Index(out, "id=18"), "items keep the executor's order")

	assert.Contains(t, out, `<a href="/resource?id=17" class="block rounded focus-visible:outline focus-visible:outline-2 focus-visible:outline-amber-500">`)
	assert.Contains(t, out, `src="/v1/resource/preview?id=17&amp;width=96&amp;height=96&amp;v=ab%26c%22d"`)
	assert.Contains(t, out, `width="96" height="96"`)
	assert.Contains(t, out, `loading="lazy" decoding="async"`)
	assert.Contains(t, out, `class="rounded object-cover h-24 w-24 bg-stone-100"`)
	assert.Contains(t, out, `alt="Beach sunset (2), near-certain duplicate, distance 1"`)
	assert.Contains(t, out, `<a href="/resource/compare?r1=42&amp;r2=17" class="text-xs underline inline-flex items-center min-h-6 px-1 rounded focus-visible:outline focus-visible:outline-2 focus-visible:outline-amber-500">Compare<span class="sr-only"> Beach sunset (2) with Beach sunset</span></a>`)
	assert.Contains(t, out, `alt="Dunes, similar, distance 5"`)

	// The rendered box matches the requested pixels: Tailwind's h-48 is 192px.
	for size, want := range map[string]struct{ px, class string }{"sm": {"48", "h-12 w-12"}, "lg": {"200", "h-[200px] w-[200px]"}} {
		f := &fakeSimilarExecutor{result: flat(simItem(17, "x", "h", intp(1)))}
		out := renderSimilar(t, map[string]string{"size": size}, similarCtx(), f)
		assert.Contains(t, out, want.class, size)
		want := want.px
		assert.Contains(t, out, `width="`+want+`" height="`+want+`"`, size)
		assert.Contains(t, out, "&amp;width="+want+"&amp;height="+want+"&amp;", size)
	}

	f = &fakeSimilarExecutor{result: flat(simItem(17, "x", "h", intp(1)))}
	out = renderSimilar(t, map[string]string{"label": "Look <alikes>"}, similarCtx(), f)
	assert.Contains(t, out, `>Look &lt;alikes&gt;</figcaption>`)
	f = &fakeSimilarExecutor{result: flat(simItem(17, "x", "h", intp(1)))}
	out = renderSimilar(t, map[string]string{"label": "   "}, similarCtx(), f)
	assert.Contains(t, out, `>Similar resources</figcaption>`)

	// A context entity with no name falls back to its ID.
	ctx := similarCtx()
	ctx.Entity = &models.Resource{ID: 42}
	f = &fakeSimilarExecutor{result: flat(simItem(17, "Beach sunset (2)", "h", intp(1)))}
	out = renderSimilar(t, nil, ctx, f)
	assert.Contains(t, out, `<span class="sr-only"> Beach sunset (2) with resource 42</span>`)
}

func TestSimilarTiers(t *testing.T) {
	for _, d := range []int{0, 2} {
		f := &fakeSimilarExecutor{result: flat(simItem(17, "A", "h", intp(d)))}
		out := renderSimilar(t, nil, similarCtx(), f)
		assert.Contains(t, out, fmt.Sprintf(`<span class="text-[11px] font-medium rounded px-1 text-rose-800 bg-rose-100" aria-hidden="true">Duplicate · %d</span>`, d))
		assert.Contains(t, out, fmt.Sprintf(`alt="A, near-certain duplicate, distance %d"`, d))
	}
	f := &fakeSimilarExecutor{result: flat(simItem(17, "A", "h", intp(3)))}
	out := renderSimilar(t, nil, similarCtx(), f)
	assert.Contains(t, out, `<span class="text-[11px] font-medium rounded px-1 text-stone-700 bg-stone-100" aria-hidden="true">Similar · 3</span>`)
	assert.Contains(t, out, `alt="A, similar, distance 3"`)

	// The shortcode and the built-in panel share one cut-off.
	for d := 0; d <= 11; d++ {
		dd := d
		badge, _ := similarTierLabel(d)
		panel := (&models.Resource{SimilarityDistance: &dd}).SimilarityTier()
		assert.Equal(t, strings.ToLower(badge), panel, "distance %d", d)
	}
}

func TestSimilarEscapesNamesAndDoesNotExpand(t *testing.T) {
	evil := `"<b>[meta path="x"]`
	ctx := similarCtx()
	ctx.Entity = &models.Resource{ID: 42, Name: `<i>T</i>`}
	f := &fakeSimilarExecutor{result: flat(simItem(17, evil, "h", intp(1)))}
	out := Process(context.Background(), "[similar]", ctx, nil, f.exec)

	assert.Len(t, f.calls, 1, "a name is never expanded as a shortcode")
	assert.NotContains(t, out, "<b>")
	assert.NotContains(t, out, "<i>")
	assert.Contains(t, out, `alt="&#34;&lt;b&gt;[meta path=&#34;x&#34;], near-certain duplicate, distance 1"`)
	assert.Contains(t, out, `<span class="sr-only"> &#34;&lt;b&gt;[meta path=&#34;x&#34;] with &lt;i&gt;T&lt;/i&gt;</span>`)

	f = &fakeSimilarExecutor{result: flat(simItem(17, "", "h", intp(4)))}
	out = renderSimilar(t, nil, similarCtx(), f)
	assert.Contains(t, out, `alt="Resource 17, similar, distance 4"`)
	assert.Contains(t, out, `<span class="sr-only"> Resource 17 with Beach sunset</span>`)
}

func TestSimilarNilDistance(t *testing.T) {
	f := &fakeSimilarExecutor{result: flat(simItem(17, "Gone pair", "h", nil))}
	out := renderSimilar(t, nil, similarCtx(), f)
	assert.Contains(t, out, `alt="Gone pair"`)
	assert.NotContains(t, out, "aria-hidden")
	assert.Contains(t, out, "/resource/compare?r1=42&amp;r2=17")
}

func TestSimilarItemsAndLimit(t *testing.T) {
	f := &fakeSimilarExecutor{result: flat()}
	assert.Equal(t, "", renderSimilar(t, nil, similarCtx(), f))
	f = &fakeSimilarExecutor{result: nil}
	assert.Equal(t, "", renderSimilar(t, nil, similarCtx(), f))
	f = &fakeSimilarExecutor{result: &QueryResult{Mode: "bucketed"}}
	assert.Equal(t, "", renderSimilar(t, nil, similarCtx(), f))

	f = &fakeSimilarExecutor{result: flat(
		simItem(1, "a", "h", intp(1)), simItem(2, "b", "h", intp(1)), simItem(3, "c", "h", intp(1)),
	)}
	out := renderSimilar(t, map[string]string{"limit": "2"}, similarCtx(), f)
	assert.Equal(t, 2, strings.Count(out, "<li "))
	assert.NotContains(t, out, "id=3")

	note := QueryResultItem{EntityType: "note", EntityID: 5, Entity: &models.Note{ID: 5, Name: "n"}}
	f = &fakeSimilarExecutor{result: flat(note, simItem(2, "b", "h", intp(1)))}
	out = renderSimilar(t, nil, similarCtx(), f)
	assert.Equal(t, 1, strings.Count(out, "<li "))
	assert.NotContains(t, out, "id=5")

	// Only non-resource items: nothing to show.
	f = &fakeSimilarExecutor{result: flat(note)}
	assert.Equal(t, "", renderSimilar(t, nil, similarCtx(), f))
}

const similarWrongEntity = `[similar] works only on a resource; use [using type="resource" …] to target one`

func TestSimilarWrongContext(t *testing.T) {
	for _, ctx := range []MetaShortcodeContext{
		{EntityType: "note", EntityID: 3},
		{EntityType: "group", EntityID: 3},
		{EntityType: "resource", EntityID: 0},
	} {
		f := &fakeSimilarExecutor{result: flat()}
		out := renderSimilar(t, nil, ctx, f)
		assert.Equal(t, shortcodeErrorMarker("similar", similarWrongEntity), out)
		assert.Empty(t, f.calls)
	}
}

func TestSimilarInvalidAttrs(t *testing.T) {
	const (
		dist  = "[similar] max-distance must be a whole number from 0 to 11"
		limit = "[similar] limit must be a whole number from 1 to 24"
		size  = `[similar] size must be "sm", "md" or "lg"`
	)
	cases := []struct {
		attr, value, want string
	}{
		{"size", "xl", size},
		{"max-distance", "12", dist},
		{"max-distance", "-1", dist},
		{"max-distance", "abc", dist},
		{"max-distance", "2.5", dist},
		{"max-distance", "+3", dist},
		{"limit", "0", limit},
		{"limit", "25", limit},
		{"limit", "x", limit},
	}
	for _, c := range cases {
		_, problem := parseSimilarAttrs(map[string]string{c.attr: c.value})
		assert.Equal(t, c.want, problem, "%s=%q", c.attr, c.value)
		f := &fakeSimilarExecutor{result: flat()}
		out := renderSimilar(t, map[string]string{c.attr: c.value}, similarCtx(), f)
		assert.Equal(t, shortcodeErrorMarker("similar", c.want), out)
		assert.Empty(t, f.calls)
	}
	_, problem := parseSimilarAttrs(map[string]string{"max-distance": " 3 ", "limit": "1", "size": "lg", "label": ""})
	assert.Empty(t, problem)
}

func TestSimilarThroughProcess(t *testing.T) {
	out := Process(context.Background(), "[similar]", similarCtx(), nil, nil)
	assert.Equal(t, "<!-- mr:similar unavailable in this context -->", out)

	f := &fakeSimilarExecutor{err: NewQueryUnavailable("")}
	out = Process(context.Background(), "[similar]", similarCtx(), nil, f.exec)
	assert.Equal(t, unavailableComment(f.err), out)

	f = &fakeSimilarExecutor{err: errors.New("inline query budget exceeded")}
	out = Process(context.Background(), "[similar]", similarCtx(), nil, f.exec)
	assert.Contains(t, out, `class="mrql-results mrql-error`)
	assert.Contains(t, out, "inline query budget exceeded")
}

func TestSimilarUnderUsing(t *testing.T) {
	cover := QueryResultItem{EntityType: "resource", EntityID: 99, Entity: &models.Resource{ID: 99, Name: "Cover"}}
	group := MetaShortcodeContext{EntityType: "group", EntityID: 7, Entity: &usingEntity{ID: 7, Name: "Album"}, Meta: []byte(`{"cover":99}`)}
	tpl := `[using type="resource" id-path="cover"][similar][/using]`

	f := &fakeSimilarExecutor{
		lookup: map[string]QueryResultItem{"99": cover},
		result: flat(simItem(17, "Twin", "h", intp(1))),
	}
	out := Process(context.Background(), tpl, group, nil, f.exec)
	require.Len(t, f.calls, 2)
	assert.Equal(t, usingLookupQuery("resource"), f.calls[0].query)
	assert.Equal(t, "type = resource AND SIMILAR TO resource(99) ORDER BY distance ASC", f.calls[1].query)
	assert.Contains(t, out, `data-similar-for="99"`)
	assert.Contains(t, out, `<span class="sr-only"> Twin with Cover</span>`)
	assert.Contains(t, out, "/resource/compare?r1=99&amp;r2=17")

	// A hidden or missing cover: the lookup is empty and [similar] never runs.
	f = &fakeSimilarExecutor{lookup: map[string]QueryResultItem{}, result: flat(simItem(17, "Twin", "h", intp(1)))}
	out = Process(context.Background(), tpl, group, nil, f.exec)
	assert.Len(t, f.calls, 1)
	assert.Equal(t, "", out)
}

// The classes the handler writes are literal strings in this package, so the
// stylesheet scan finds them. This pins that each is spelled out whole.
func TestSimilarClassesAreLiteral(t *testing.T) {
	f := &fakeSimilarExecutor{result: flat(simItem(17, "A", "h", intp(1)), simItem(18, "B", "h", intp(5)))}
	out := renderSimilar(t, map[string]string{"size": "sm"}, similarCtx(), f)
	classAttr := regexp.MustCompile(`class="([^"]*)"`)
	for _, m := range classAttr.FindAllStringSubmatch(out, -1) {
		for _, cls := range strings.Fields(m[1]) {
			assert.NotContains(t, cls, "%", "class %q is built at runtime", cls)
		}
	}
	assert.Contains(t, out, "h-12 w-12")
}
