package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRelations records every request and answers from a fixed list.
type fakeRelations struct {
	entries []GroupRelationEntry
	err     error
	calls   []RelationsRequest
}

func (f *fakeRelations) list(_ context.Context, req RelationsRequest) ([]GroupRelationEntry, error) {
	f.calls = append(f.calls, req)
	return f.entries, f.err
}

func relationsGroup(id uint) MetaShortcodeContext {
	return MetaShortcodeContext{EntityType: "group", EntityID: id, Entity: struct{ Name string }{"Carrier"}}
}

func renderRelations(t *testing.T, f *fakeRelations, input string, ctx MetaShortcodeContext) string {
	t.Helper()
	reqCtx := context.Background()
	if f != nil {
		reqCtx = WithRelationLister(reqCtx, f.list)
	}
	return Process(reqCtx, input, ctx, nil, nil)
}

var twoTypes = []GroupRelationEntry{
	{Direction: "out", Type: "Depends on", Name: "db", GroupID: 12, GroupName: "Auth Service", GroupCategory: "Service"},
	{Direction: "out", Type: "Depends on", GroupID: 13, GroupName: "Queue"},
	{Direction: "out", Type: "Owned by", GroupID: 14, GroupName: "Platform Team", GroupCategory: "Team"},
}

func TestRelations_InlineMarkup(t *testing.T) {
	f := &fakeRelations{entries: twoTypes}
	got := renderRelations(t, f, `[relations]`, relationsGroup(7))
	want := `<div class="relations-shortcode" data-direction="out"><ul class="relations-shortcode__types" role="list">` +
		`<li class="relations-shortcode__type"><span class="relations-shortcode__label">Depends on</span><ul class="relations-shortcode__list" role="list">` +
		`<li class="relations-shortcode__entry"><a href="/group?id=12">Auth Service</a> <span class="relations-shortcode__category">(Service)</span></li>` +
		`<li class="relations-shortcode__entry"><a href="/group?id=13">Queue</a></li>` +
		`</ul></li>` +
		`<li class="relations-shortcode__type"><span class="relations-shortcode__label">Owned by</span><ul class="relations-shortcode__list" role="list">` +
		`<li class="relations-shortcode__entry"><a href="/group?id=14">Platform Team</a> <span class="relations-shortcode__category">(Team)</span></li>` +
		`</ul></li></ul></div>`
	assert.Equal(t, want, got)
	require.Len(t, f.calls, 1)
	assert.Equal(t, RelationsRequest{GroupID: 7, Direction: "out", Limit: 20}, f.calls[0])
}

func TestRelations_IncomingLabelAndFallbackNames(t *testing.T) {
	f := &fakeRelations{entries: []GroupRelationEntry{
		{Direction: "in", Type: "Blocks", GroupID: 3, GroupName: "  "},
		{Direction: "in", Type: "", GroupID: 4, GroupName: "Four"},
	}}
	got := renderRelations(t, f, `[relations direction="in"]`, relationsGroup(7))
	assert.Contains(t, got, `data-direction="in"`)
	assert.Contains(t, got, `<span class="relations-shortcode__label">Incoming: Blocks</span>`)
	assert.Contains(t, got, `<a href="/group?id=3">Group 3</a>`, "a blank name falls back so the link keeps an accessible name")
	assert.Contains(t, got, `<span class="relations-shortcode__label">Incoming: Relation</span>`)
	assert.Equal(t, "in", f.calls[0].Direction)

	out := renderRelations(t, &fakeRelations{entries: twoTypes}, `[relations direction="out"]`, relationsGroup(7))
	assert.NotContains(t, out, "Incoming:")
}

func TestRelations_EscapesEveryField(t *testing.T) {
	evil := `<script>x</script>"`
	f := &fakeRelations{entries: []GroupRelationEntry{
		{Direction: "out", Type: evil, Name: evil, Description: evil, GroupID: 5, GroupName: evil, GroupCategory: evil},
	}}
	inline := renderRelations(t, f, `[relations]`, relationsGroup(7))
	assert.NotContains(t, inline, "<script>")
	assert.Equal(t, 3, strings.Count(inline, "&lt;script&gt;x&lt;/script&gt;&#34;"), inline)

	block := renderRelations(t, f, `[relations]<i>[item path="Name"]|[item path="Description"]|[item path="Type"]|[item path="GroupName"]|[item path="GroupCategory"]</i>[/relations]`, relationsGroup(7))
	assert.NotContains(t, block, "<script>")
	assert.Equal(t, 6, strings.Count(block, "&lt;script&gt;x&lt;/script&gt;&#34;"), block)
}

func TestRelations_AttributePassThrough(t *testing.T) {
	cases := []struct {
		input string
		want  RelationsRequest
	}{
		{`[relations type="  Depends on "]`, RelationsRequest{GroupID: 7, Type: "Depends on", Direction: "out", Limit: 20}},
		{`[relations direction=" in "]`, RelationsRequest{GroupID: 7, Direction: "in", Limit: 20}},
		{`[relations limit=""]`, RelationsRequest{GroupID: 7, Direction: "out", Limit: 20}},
		{`[relations limit="abc"]`, RelationsRequest{GroupID: 7, Direction: "out", Limit: 20}},
		{`[relations limit="0"]`, RelationsRequest{GroupID: 7, Direction: "out", Limit: 20}},
		{`[relations limit="-3"]`, RelationsRequest{GroupID: 7, Direction: "out", Limit: 20}},
		{`[relations limit="250"]`, RelationsRequest{GroupID: 7, Direction: "out", Limit: 100}},
		{`[relations limit="7"]`, RelationsRequest{GroupID: 7, Direction: "out", Limit: 7}},
	}
	for _, c := range cases {
		f := &fakeRelations{}
		renderRelations(t, f, c.input, relationsGroup(7))
		require.Len(t, f.calls, 1, c.input)
		assert.Equal(t, c.want, f.calls[0], c.input)
	}
}

func TestRelations_InvalidDirectionRunsNoLookup(t *testing.T) {
	for _, dir := range []string{"both", "sideways", ""} {
		f := &fakeRelations{entries: twoTypes}
		got := renderRelations(t, f, fmt.Sprintf(`[relations direction=%q]`, dir), relationsGroup(7))
		assert.Contains(t, got, "shortcode-error", dir)
		assert.Contains(t, got, `direction must be &#34;out&#34; or &#34;in&#34;`, dir)
		assert.Empty(t, f.calls, dir)
	}
}

func TestRelations_OnlyGroupsHaveRelations(t *testing.T) {
	for _, ctx := range []MetaShortcodeContext{
		{EntityType: "resource", EntityID: 3},
		{EntityType: "note", EntityID: 3},
		{EntityType: "category", EntityID: 3},
		{EntityType: "group", EntityID: 0},
	} {
		f := &fakeRelations{entries: twoTypes}
		got := renderRelations(t, f, `[relations]`, ctx)
		assert.Equal(t, "<!-- mr:relations: only groups have relations -->", got, ctx.EntityType)
		assert.Empty(t, f.calls)
	}
}

func TestRelations_UnavailableWithoutLister(t *testing.T) {
	assert.Equal(t, "<!-- mr:relations unavailable in this context -->", renderRelations(t, nil, `[relations]`, relationsGroup(7)))
	explicitNil := Process(WithRelationLister(context.Background(), nil), `[relations]`, relationsGroup(7), nil, nil)
	assert.Equal(t, "<!-- mr:relations unavailable in this context -->", explicitNil)

	// The surface's handle cannot confine the viewer: the lister says so
	// rather than answering from an unscoped read.
	f := &fakeRelations{err: fmt.Errorf("wrapped: %w", ErrRelationsUnavailable)}
	assert.Equal(t, "<!-- mr:relations unavailable in this context -->", renderRelations(t, f, `[relations]`, relationsGroup(7)))
}

func TestRelations_EmptyResult(t *testing.T) {
	assert.Equal(t, "ab", renderRelations(t, &fakeRelations{}, `a[relations]b`, relationsGroup(7)))
	assert.Equal(t, "ab", renderRelations(t, &fakeRelations{}, `a[relations]<i>[item path="Name"]</i>[/relations]b`, relationsGroup(7)))
	got := renderRelations(t, &fakeRelations{}, `[relations]x[else]<p>None for [property path="Name"].</p>[/relations]`, relationsGroup(7))
	assert.Equal(t, "<p>None for Carrier.</p>", got)
}

func TestRelations_BlockBody(t *testing.T) {
	f := &fakeRelations{entries: twoTypes}
	got := renderRelations(t, f,
		`[relations]<b>[item index="true"]. [item path="Name" default="(unnamed)"] [item path="GroupName"] [item path="GroupURL"] [item path="Direction"] [item path="GroupID"] [property path="Name"]</b>[/relations]`,
		relationsGroup(7))
	assert.Contains(t, got, `<li class="relations-shortcode__entry"><b>1. db Auth Service /group?id=12 out 12 Carrier</b></li>`)
	assert.Contains(t, got, `<li class="relations-shortcode__entry"><b>2. (unnamed) Queue /group?id=13 out 13 Carrier</b></li>`)
	assert.Contains(t, got, `<span class="relations-shortcode__label">Owned by</span><ul class="relations-shortcode__list" role="list"><li class="relations-shortcode__entry"><b>3. (unnamed) Platform Team /group?id=14 out 14 Carrier</b></li>`)
	assert.False(t, f.calls[0].Details, "a body that never names the far group's details does not load them")
}

func TestRelations_ValuesAreNeverTemplateSource(t *testing.T) {
	f := &fakeRelations{entries: []GroupRelationEntry{{Direction: "out", Type: "T", Name: `[property path="Name"]`, GroupID: 1, GroupName: "G"}}}
	got := renderRelations(t, f, `[relations]<i>[item path="Name"]</i>[/relations]`, relationsGroup(7))
	assert.Contains(t, got, `<i>[property path=&#34;Name&#34;]</i>`)
	assert.NotContains(t, got, "Carrier")
}

func TestRelations_FarGroupDetailsLoadOnlyWhenNamed(t *testing.T) {
	entries := []GroupRelationEntry{{
		Direction: "out", Type: "Depends on", GroupID: 12, GroupName: "Auth",
		GroupDescription: "Signs tokens", GroupMeta: json.RawMessage(`{"status":"green","cost":3}`),
	}}
	for _, body := range []string{
		`[item path="GroupMeta.status"]`,
		`[item path="GroupDescription"]`,
		`[calc expr="item.GroupMeta.cost * 2"]`,
	} {
		f := &fakeRelations{entries: entries}
		got := renderRelations(t, f, `[relations]<i>`+body+`</i>[/relations]`, relationsGroup(7))
		require.Len(t, f.calls, 1, body)
		assert.True(t, f.calls[0].Details, body)
		switch {
		case strings.Contains(body, "status"):
			assert.Contains(t, got, "<i>green</i>")
		case strings.Contains(body, "Description"):
			assert.Contains(t, got, "<i>Signs tokens</i>")
		default:
			assert.Contains(t, got, "<i>6</i>")
		}
	}
	// The inline form and the [else] branch never read them.
	f := &fakeRelations{}
	renderRelations(t, f, `[relations]x[else][meta path="GroupMeta"][/relations]`, relationsGroup(7))
	renderRelations(t, f, `[relations type="GroupMeta"]`, relationsGroup(7))
	require.Len(t, f.calls, 2)
	assert.False(t, f.calls[0].Details)
	assert.False(t, f.calls[1].Details)
}

func TestRelations_Errors(t *testing.T) {
	f := &fakeRelations{err: errors.New("pq: relation \"secret_table\" does not exist")}
	got := renderRelations(t, f, `[relations]`, relationsGroup(7))
	assert.Contains(t, got, "could not load relations")
	assert.NotContains(t, got, "secret_table")

	f = &fakeRelations{err: fmt.Errorf("%w: inline query budget exceeded (3 per page); refine templates or raise -mrql-page-query-budget", ErrRelationsBudget)}
	got = renderRelations(t, f, `[relations]`, relationsGroup(7))
	assert.Contains(t, got, "inline query budget exceeded (3 per page)")
	assert.NotContains(t, got, "relations: budget")

	f = &fakeRelations{err: fmt.Errorf("query: %w", context.DeadlineExceeded)}
	assert.Contains(t, renderRelations(t, f, `[relations]`, relationsGroup(7)), "lookup timed out")
}

func TestRelations_StoppedRenderAndReloadFace(t *testing.T) {
	// The render is cancelled while the lookup runs: nothing is written.
	render, cancel := context.WithCancel(context.Background())
	defer cancel()
	lister := func(context.Context, RelationsRequest) ([]GroupRelationEntry, error) {
		cancel()
		return twoTypes, nil
	}
	assert.Empty(t, Process(WithRelationLister(render, lister), `[relations]`, relationsGroup(7), nil, nil))

	f := &fakeRelations{entries: twoTypes}
	got := Process(WithRelationLister(context.Background(), f.list), `[reload][relations][/reload]`, relationsGroup(7), nil, nil)
	assert.Contains(t, got, "[relations] cannot be used inside a [reload] button face")
	assert.Empty(t, f.calls)
}

func TestRelations_TooltipRendersNamesWithoutLinks(t *testing.T) {
	f := &fakeRelations{entries: twoTypes}
	reqCtx := WithRelationLister(WithinTooltip(context.Background()), f.list)
	got := Process(reqCtx, `[relations]`, relationsGroup(7), nil, nil)
	assert.NotContains(t, got, "<a ")
	assert.Contains(t, got, `<li class="relations-shortcode__entry">Auth Service <span class="relations-shortcode__category">(Service)</span></li>`)
}
