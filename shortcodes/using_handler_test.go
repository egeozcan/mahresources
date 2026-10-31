package shortcodes

import (
	"context"
	"errors"
	"html"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// usingEntity is a target with the fields [using id-prop=] reads.
type usingEntity struct {
	ID      uint
	Name    string
	OwnerId *uint
}

// fakeUsingExecutor serves the [using] lookup query from a fixed set of
// visible entities, keyed by type and ID, and records every call. An entity
// that is not in the set comes back as an empty result, which is what the real
// executor returns both for a missing ID and for one outside the principal's
// subtree.
type fakeUsingExecutor struct {
	visible map[string]map[string]QueryResultItem
	calls   []usingCall
	err     error
	result  *QueryResult // when set, returned for every call regardless of query
}

type usingCall struct {
	query string
	opts  QueryOptions
}

func (f *fakeUsingExecutor) exec(_ context.Context, query string, opts QueryOptions) (*QueryResult, error) {
	f.calls = append(f.calls, usingCall{query, opts})
	if f.err != nil {
		return nil, f.err
	}
	if f.result != nil {
		return f.result, nil
	}
	for typ, byID := range f.visible {
		if query != usingLookupQuery(typ) {
			continue
		}
		if item, ok := byID[opts.Params["id"]]; ok {
			return &QueryResult{EntityType: typ, Mode: "flat", Items: []QueryResultItem{item}}, nil
		}
	}
	return &QueryResult{Mode: "flat"}, nil
}

func newUsingFixture() *fakeUsingExecutor {
	root := uint(1)
	return &fakeUsingExecutor{visible: map[string]map[string]QueryResultItem{
		"group": {
			"1": {EntityType: "group", EntityID: 1, Entity: &usingEntity{ID: 1, Name: "Root"}, Meta: []byte(`{"status":"archived"}`), ScopeGroupID: 1, ParentGroupID: unresolvedScopeSentinel, RootGroupID: 1},
			"7": {EntityType: "group", EntityID: 7, Entity: &usingEntity{ID: 7, Name: "Project", OwnerId: &root}, Meta: []byte(`{"status":"active"}`), ScopeGroupID: 7, ParentGroupID: 1, RootGroupID: 1},
		},
		"resource": {
			"42": {EntityType: "resource", EntityID: 42, Entity: &usingEntity{ID: 42, Name: "photo.jpg"}, Meta: []byte(`{}`), ScopeGroupID: 7, ParentGroupID: 1, RootGroupID: 1},
		},
	}}
}

// noteCtx is a note owned by group 7, whose Meta references group 7 and an
// unknown group 99.
func noteCtx() MetaShortcodeContext {
	owner := uint(7)
	return MetaShortcodeContext{
		EntityType:    "note",
		EntityID:      3,
		Entity:        &usingEntity{ID: 3, Name: "Meeting", OwnerId: &owner},
		Meta:          []byte(`{"client":7,"clientStr":"7","missing":99,"frac":7.5,"neg":-7,"list":[7],"obj":{"id":7}}`),
		ScopeGroupID:  7,
		ParentGroupID: 1,
		RootGroupID:   1,
	}
}

func renderUsing(t *testing.T, f *fakeUsingExecutor, ctx MetaShortcodeContext, src string) string {
	t.Helper()
	return Process(context.Background(), src, ctx, nil, f.exec)
}

func TestUsingSourcesResolveTarget(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"literal id", `[using type="group" id="7"][property path="Name"][/using]`, "Project"},
		{"id-path number", `[using type="group" id-path="client"][property path="Name"][/using]`, "Project"},
		{"id-path numeric string", `[using type="group" id-path="clientStr"][property path="Name"][/using]`, "Project"},
		{"id-path nested", `[using type="group" id-path="obj.id"][property path="Name"][/using]`, "Project"},
		{"id-prop", `[using type="group" id-prop="OwnerId"][property path="Name"][/using]`, "Project"},
		{"rel owner of a note", `[using rel="owner"][property path="Name"][/using]`, "Project"},
		{"rel root", `[using rel="root"][property path="Name"][/using]`, "Root"},
		{"resource", `[using type="resource" id="42"][property path="Name"][/using]`, "photo.jpg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderUsing(t, newUsingFixture(), noteCtx(), tc.src)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestUsingRelOwnerOfAGroupIsItsParent(t *testing.T) {
	// A group's scope ID is itself, so its owner is the parent scope, exactly
	// as [link to="owner"] reads it.
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7, ScopeGroupID: 7, ParentGroupID: 1, RootGroupID: 1}
	got := renderUsing(t, newUsingFixture(), ctx, `[using rel="owner"][property path="Name"][/using]`)
	assert.Equal(t, "Root", got)
}

func TestUsingBodyReadsTheTarget(t *testing.T) {
	src := `[using rel="owner"][meta path="status" inline="true"]|[conditional path="status" eq="active"]on[else]off[/conditional]|[link][/using]`
	got := renderUsing(t, newUsingFixture(), noteCtx(), src)
	assert.Equal(t, "active|on|/group?id=7", got)
}

func TestUsingLookupBindsIDAndSearchesGlobally(t *testing.T) {
	f := newUsingFixture()
	renderUsing(t, f, noteCtx(), `[using type="group" id-path="client"]x[/using]`)
	require.Len(t, f.calls, 1)
	call := f.calls[0]
	// The ID is a bound parameter, never part of the query text.
	assert.Equal(t, "type = group AND id = $id", call.query)
	assert.Equal(t, map[string]string{"id": "7"}, call.opts.Params)
	assert.Equal(t, 1, call.opts.Limit)
	// Global: the carrier's group (7) must not filter an explicit reference.
	assert.Equal(t, uint(0), call.opts.ScopeGroupID)
}

func TestUsingNoTargetRendersElse(t *testing.T) {
	ownerless := MetaShortcodeContext{EntityType: "note", EntityID: 3, Entity: &usingEntity{ID: 3}, Meta: []byte(`{}`),
		ScopeGroupID: unresolvedScopeSentinel, ParentGroupID: unresolvedScopeSentinel, RootGroupID: unresolvedScopeSentinel}
	cases := []struct {
		name string
		ctx  MetaShortcodeContext
		src  string
		hits int // executor calls expected
	}{
		{"unknown id", noteCtx(), `[using type="group" id="99"]T[else]E[/using]`, 1},
		{"unknown id from meta", noteCtx(), `[using type="group" id-path="missing"]T[else]E[/using]`, 1},
		{"absent meta path", noteCtx(), `[using type="group" id-path="nope"]T[else]E[/using]`, 0},
		{"fractional", noteCtx(), `[using type="group" id-path="frac"]T[else]E[/using]`, 0},
		{"negative", noteCtx(), `[using type="group" id-path="neg"]T[else]E[/using]`, 0},
		{"array", noteCtx(), `[using type="group" id-path="list"]T[else]E[/using]`, 0},
		{"object", noteCtx(), `[using type="group" id-path="obj"]T[else]E[/using]`, 0},
		{"nil id-prop", ownerless, `[using type="group" id-prop="OwnerId"]T[else]E[/using]`, 0},
		{"unknown id-prop", noteCtx(), `[using type="group" id-prop="Nope"]T[else]E[/using]`, 0},
		{"ownerless rel", ownerless, `[using rel="owner"]T[else]E[/using]`, 0},
		{"wrong type for id", noteCtx(), `[using type="resource" id="7"]T[else]E[/using]`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newUsingFixture()
			assert.Equal(t, "E", renderUsing(t, f, tc.ctx, tc.src))
			assert.Len(t, f.calls, tc.hits)
		})
	}
}

func TestUsingElseRendersAgainstCarrier(t *testing.T) {
	got := renderUsing(t, newUsingFixture(), noteCtx(), `[using type="group" id="99"]T[else][property path="Name"][/using]`)
	assert.Equal(t, "Meeting", got)
}

func TestUsingNoTargetWithoutElseRendersNothing(t *testing.T) {
	got := renderUsing(t, newUsingFixture(), noteCtx(), `a[using type="group" id="99"]T[/using]b`)
	assert.Equal(t, "ab", got)
}

func TestUsingHiddenAndMissingRenderAlike(t *testing.T) {
	// Group 7 exists but is withheld from this executor, as the scoped
	// executor withholds an out-of-subtree entity; 99 does not exist at all.
	f := newUsingFixture()
	delete(f.visible["group"], "7")
	hidden := renderUsing(t, f, noteCtx(), `[using type="group" id="7"]T[else]E[/using]`)
	missing := renderUsing(t, f, noteCtx(), `[using type="group" id="99"]T[else]E[/using]`)
	assert.Equal(t, missing, hidden)
}

func TestUsingAttributeMistakesRenderMarker(t *testing.T) {
	cases := map[string]string{
		`[using]x[/using]`: "needs one of",
		`[using type="group" id="1" rel="owner"]x[/using]`: "exactly one of",
		`[using id="1"]x[/using]`:                          "type must be",
		`[using type="tag" id="1"]x[/using]`:               "type must be",
		`[using type="group" id="abc"]x[/using]`:           "positive whole number",
		`[using type="group" id="0"]x[/using]`:             "positive whole number",
		`[using type="group" id="-1"]x[/using]`:            "positive whole number",
	}
	for src, want := range cases {
		f := newUsingFixture()
		got := renderUsing(t, f, noteCtx(), src)
		assert.Contains(t, got, "shortcode-error", src)
		assert.Contains(t, html.UnescapeString(got), want, src)
		assert.Empty(t, f.calls, src)
	}
}

func TestUsingInlineFormIsAnError(t *testing.T) {
	got := renderUsing(t, newUsingFixture(), noteCtx(), `[using type="group" id="7"]`)
	assert.Contains(t, got, "requires a closing [/using] tag")
}

func TestUsingQuerySource(t *testing.T) {
	f := newUsingFixture()
	f.result = &QueryResult{Mode: "flat", Items: []QueryResultItem{
		{EntityType: "note", EntityID: 5, Entity: &usingEntity{ID: 5, Name: "First"}},
		{EntityType: "note", EntityID: 6, Entity: &usingEntity{ID: 6, Name: "Second"}},
	}}
	got := renderUsing(t, f, noteCtx(), `[using query="type = note AND name = $n ORDER BY created" param-n="Index" scope="parent"][property path="Name"][/using]`)
	assert.Equal(t, "First", got)
	require.Len(t, f.calls, 1)
	assert.Equal(t, "type = note AND name = $n ORDER BY created", f.calls[0].query)
	assert.Equal(t, map[string]string{"n": "Index"}, f.calls[0].opts.Params)
	assert.Equal(t, 1, f.calls[0].opts.Limit)
	assert.Equal(t, uint(1), f.calls[0].opts.ScopeGroupID, "scope=parent resolves against the carrier")
}

func TestUsingQueryDefaultsToEntityScope(t *testing.T) {
	f := newUsingFixture()
	f.result = &QueryResult{Mode: "flat"}
	renderUsing(t, f, noteCtx(), `[using query="type = note"]x[/using]`)
	require.Len(t, f.calls, 1)
	assert.Equal(t, uint(7), f.calls[0].opts.ScopeGroupID)
}

func TestUsingGroupedResultIsAnError(t *testing.T) {
	f := newUsingFixture()
	f.result = &QueryResult{Mode: "bucketed", Groups: []QueryResultGroup{{Items: []QueryResultItem{{EntityType: "note", EntityID: 5}}}}}
	got := renderUsing(t, f, noteCtx(), `[using query="type = note GROUP BY name"]x[else]E[/using]`)
	assert.Contains(t, got, "shortcode-error")
}

func TestUsingExecutorErrorIsShown(t *testing.T) {
	f := newUsingFixture()
	f.err = errors.New("budget exceeded")
	got := renderUsing(t, f, noteCtx(), `[using type="group" id="7"]x[else]E[/using]`)
	assert.Contains(t, got, "budget exceeded")
	assert.NotContains(t, got, "E")
}

func TestUsingWithoutExecutorIsUnavailable(t *testing.T) {
	got := Process(context.Background(), `[using type="group" id="7"]x[else]E[/using]`, noteCtx(), nil, nil)
	assert.Equal(t, "<!-- mr:using unavailable in this context -->", got)
}

func TestUsingNests(t *testing.T) {
	// The inner rel resolves against the outer target (group 7), not the note.
	src := `[using rel="owner"][property path="Name"]>[using rel="owner"][property path="Name"][/using][/using]`
	assert.Equal(t, "Project>Root", renderUsing(t, newUsingFixture(), noteCtx(), src))
}

func TestUsingInsideEach(t *testing.T) {
	ctx := noteCtx()
	ctx.Meta = []byte(`{"rows":["a","b"]}`)
	f := newUsingFixture()
	src := `[each path="rows"][item]:[using rel="owner"][property path="Name"][/using];[/each]`
	assert.Equal(t, "a:Project;b:Project;", renderUsing(t, f, ctx, src))
	assert.Len(t, f.calls, 2, "one lookup per element, as an [mrql] in the branch would run")
}

func TestUsingItemInsideRendersNothing(t *testing.T) {
	ctx := noteCtx()
	ctx.Meta = []byte(`{"rows":["a"]}`)
	src := `[each path="rows"][using rel="owner"]<[item]>[/using][/each]`
	assert.Equal(t, "<>", renderUsing(t, newUsingFixture(), ctx, src))
}

func TestUsingChildCarriesReadOnlyAndOuter(t *testing.T) {
	item := newUsingFixture().visible["group"]["7"]
	parent := noteCtx()
	parent.ForceReadOnly = true

	child := contextFromItem(item, parent)
	assert.True(t, child.ForceReadOnly)

	got := renderUsing(t, newUsingFixture(), parent, `[using rel="owner"][meta path="status" editable="true"][/using]`)
	assert.Contains(t, got, `data-editable="false"`)
	assert.Contains(t, got, `data-entity-id="7"`)
}

func TestUsingSetsOuter(t *testing.T) {
	var seen *MetaShortcodeContext
	renderer := func(_ string, _ Shortcode, ctx MetaShortcodeContext) (string, error) {
		seen = ctx.outer
		return "", nil
	}
	src := `[using rel="owner"][plugin:p:x][/using]`
	Process(context.Background(), src, noteCtx(), renderer, newUsingFixture().exec)
	require.NotNil(t, seen)
	assert.Equal(t, uint(3), seen.EntityID)
	assert.Nil(t, seen.outer)
}

func TestUsingDeferredBlockSignsTarget(t *testing.T) {
	var signedType string
	var signedID uint
	signer := func(entityType string, entityID uint, body string) string {
		signedType, signedID = entityType, entityID
		return "tok"
	}
	reqCtx := WithDeferredSigner(context.Background(), signer)
	out := Process(reqCtx, `[using rel="owner"][lazy][property path="Name"][/lazy][/using]`, noteCtx(), nil, newUsingFixture().exec)
	assert.True(t, strings.Contains(out, "lazy-shortcode"), out)
	assert.Equal(t, "group", signedType)
	assert.Equal(t, uint(7), signedID)
}

func TestEntityIDFromValue(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want uint
		ok   bool
	}{
		{float64(7), 7, true},
		{"12", 12, true},
		{" 12 ", 12, true},
		{float64(0), 0, false},
		{float64(1.5), 0, false},
		{float64(1 << 60), 0, false},
		{"1e3", 0, false},
		{"0x10", 0, false},
		{true, 0, false},
		{nil, 0, false},
		{[]any{float64(1)}, 0, false},
	} {
		got, ok := entityIDFromValue(tc.in)
		assert.Equal(t, tc.ok, ok, "%v", tc.in)
		assert.Equal(t, tc.want, got, "%v", tc.in)
	}
}
