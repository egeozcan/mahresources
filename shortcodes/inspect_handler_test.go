package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type inspectOwner struct {
	ID   uint
	Name string
}

type inspectEntity struct {
	ID          uint
	Name        string
	Description string
	CreatedAt   time.Time
	FileSize    int64
	OwnerId     *uint
	Owner       *inspectOwner
	Tags        []string
	ShareToken  *string
	Meta        json.RawMessage
	hidden      string
}

func inspectCtx(meta string) MetaShortcodeContext {
	return MetaShortcodeContext{EntityType: "group", EntityID: 42, Meta: json.RawMessage(meta)}
}

func inspectOn(input string, ctx MetaShortcodeContext) string {
	return Process(WithInspect(context.Background()), input, ctx, nil, nil)
}

// Every surface without the flag renders [inspect] as nothing at all: no
// comment, no marker, byte-for-byte the template without it.
func TestInspectRendersNothingWithoutTheFlag(t *testing.T) {
	ctx := inspectCtx(`{"project":{"name":"Apollo"},"steps":[{"title":"a"},{"title":"b"}],"on":true}`)
	exec := mockExecutor(&QueryResult{EntityType: "resource", Mode: "flat", Items: []QueryResultItem{
		{EntityType: "resource", EntityID: 1, Entity: testEntity{ID: 1, Name: "Photo A"}, Meta: []byte(`{"k":1}`)},
	}}, nil)

	cases := []struct{ with, without string }{
		{`<i id="x">[inspect]</i>`, `<i id="x"></i>`},
		{`[inspect path="project"]`, ``},
		{`[conditional path="on" eq="true"]A[inspect]B[/conditional]`, `[conditional path="on" eq="true"]AB[/conditional]`},
		{`[each path="steps"]<[inspect][item path="title"]>[/each]`, `[each path="steps"]<[item path="title"]>[/each]`},
		{`[mrql query="type = resource"][inspect][property path="Name"][/mrql]`, `[mrql query="type = resource"][property path="Name"][/mrql]`},
		{`[details summary="S"][inspect]body[/details]`, `[details summary="S"]body[/details]`},
	}
	for _, tc := range cases {
		got := Process(context.Background(), tc.with, ctx, nil, exec)
		want := Process(context.Background(), tc.without, ctx, nil, exec)
		assert.Equal(t, want, got, tc.with)
		assert.NotContains(t, got, "inspect", tc.with)
	}
}

// A [details] body sealed on a display page carries the raw [inspect]; the
// deferred endpoint renders that body later with a context that has no flag.
func TestInspectInsideSealedDetailsBodyRendersNothingLater(t *testing.T) {
	var calls []string
	reqCtx := WithDeferredSigner(context.Background(), fakeSigner(&calls))
	ctx := inspectCtx(`{"a":1}`)
	out := Process(reqCtx, `[details summary="S"][inspect]body[/details]`, ctx, nil, nil)
	assert.NotContains(t, out, "inspect-shortcode")
	require.Len(t, calls, 1)
	assert.Contains(t, calls[0], "[inspect]", "the sealed body is the raw source")

	body := strings.TrimPrefix(calls[0], "group|")
	assert.Equal(t, "body", Process(context.Background(), body, ctx, nil, nil))
}

func TestInspectUntrustedSourceRendersNothing(t *testing.T) {
	ctx := inspectCtx(`{"a":1}`)
	ctx.UntrustedSource = true
	assert.Equal(t, "x", inspectOn(`x[inspect]`, ctx))
}

func TestInspectMetaRows(t *testing.T) {
	long := strings.Repeat("é", 100)
	ctx := inspectCtx(`{"zeta":1.5,"project":{"name":"Apollo","stage":null},"tags":["a","b"],"on":true,"long":"` + long + `","a.b":"dot","q\"":"quote"}`)
	out := inspectOn(`[inspect]`, ctx)

	require.Contains(t, out, `<details class="inspect-shortcode">`)
	assert.Contains(t, out, `<summary>Inspect: group #42</summary>`)

	// Sorted depth-first: a.b, long, on, project, project.name, project.stage, q", tags, zeta.
	paths := regexp.MustCompile(`<th scope="row"><code>([^<]*)</code>`).FindAllStringSubmatch(metaTable(t, out), -1)
	var got []string
	for _, p := range paths {
		got = append(got, html.UnescapeString(p[1]))
	}
	assert.Equal(t, []string{"a.b", "long", "on", "project", "project.name", "project.stage", `q"`, "tags", "zeta"}, got)

	assert.Contains(t, out, `<code>project</code></th><td>object (2 keys)</td><td></td>`)
	assert.Contains(t, out, `<code>project.name</code></th><td>string</td><td>&#34;Apollo&#34;</td>`)
	assert.Contains(t, out, `<code>project.stage</code></th><td>null</td>`)
	assert.Contains(t, out, `<code>on</code></th><td>boolean</td><td>true</td>`)
	assert.Contains(t, out, `<code>zeta</code></th><td>number</td><td>1.5</td>`)
	assert.Contains(t, out, `<code>tags</code></th><td>array (2 items)</td><td></td>`)
	assert.Contains(t, out, `value="&#91;each path=&#34;tags&#34;]&#91;item]&#91;/each]"`)
	assert.Contains(t, out, `value="&#91;meta path=&#34;project.name&#34;]"`)

	// 80 runes then an ellipsis.
	assert.Contains(t, out, `&#34;`+strings.Repeat("é", 80)+`…&#34;`)
	assert.NotContains(t, out, strings.Repeat("é", 81))

	// A key no path= can name gets no input.
	assert.Contains(t, out, `<code>a.b</code></th><td>string</td><td>&#34;dot&#34;</td><td>not addressable by path=</td>`)
	assert.Contains(t, out, `<code>q&#34;</code></th><td>string</td><td>&#34;quote&#34;</td><td>not addressable by path=</td>`)
}

// metaTable returns the Meta table's markup alone.
func metaTable(t *testing.T, out string) string {
	t.Helper()
	start := strings.Index(out, `<caption>Meta</caption>`)
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(out[start:], `</table>`)
	require.Greater(t, end, 0)
	return out[start : start+end]
}

func TestInspectPathNarrowsMeta(t *testing.T) {
	ctx := inspectCtx(`{"project":{"name":"Apollo","lead":{"id":3}},"other":1}`)
	out := inspectOn(`[inspect path="project"]`, ctx)
	assert.Contains(t, out, "· Meta at “project”")
	table := metaTable(t, out)
	assert.Contains(t, table, `<code>project.name</code>`)
	assert.Contains(t, table, `<code>project.lead.id</code>`)
	assert.NotContains(t, table, `<code>other</code>`)
	assert.NotContains(t, table, `<code>project</code>`)

	scalar := inspectOn(`[inspect path="project.name"]`, ctx)
	assert.Contains(t, metaTable(t, scalar), `<code>project.name</code></th><td>string</td>`)

	missing := inspectOn(`[inspect path="nope"]`, ctx)
	assert.Contains(t, missing, `No Meta value at path &#34;nope&#34;.`)
	assert.NotContains(t, missing, `<caption>Meta</caption>`)

	empty := inspectOn(`[inspect]`, inspectCtx(``))
	assert.Contains(t, empty, `This entity has no Meta.`)
	assert.Contains(t, inspectOn(`[inspect]`, inspectCtx(`{}`)), `This entity has no Meta.`)
}

func TestInspectMetaDepthIsBounded(t *testing.T) {
	ctx := inspectCtx(`{"a":{"b":{"c":{"d":{"e":{"f":{"g":1}}}}}}}`)
	out := inspectOn(`[inspect]`, ctx)
	assert.Contains(t, out, `<code>a.b.c.d.e.f</code>`)
	assert.NotContains(t, out, `<code>a.b.c.d.e.f.g</code>`)
}

func TestInspectEscapesValuesAndSnippets(t *testing.T) {
	ctx := inspectCtx(`{"a":"<b>[mrql query='x']"}`)
	out := inspectOn(`[inspect]`, ctx)
	assert.NotContains(t, out, "<b>")
	assert.NotContains(t, out, "[mrql")
	assert.NotContains(t, out, "[meta")
	assert.Contains(t, out, `&lt;b&gt;&#91;mrql query=&#39;x&#39;]`)

	m := regexp.MustCompile(`<input [^>]*value="([^"]*)"`).FindStringSubmatch(out)
	require.NotNil(t, m)
	assert.Equal(t, `[meta path="a"]`, html.UnescapeString(m[1]))

	// The panel is never parsed again: rendering it once more changes nothing.
	assert.Empty(t, Parse(out))
}

func TestInspectPropertiesNeverReadThroughAnAssociation(t *testing.T) {
	token := "secret-share-token"
	ownerID := uint(9)
	ctx := inspectCtx(`{"k":1}`)
	ctx.Entity = &inspectEntity{
		ID: 42, Name: "Root", Description: "Desc[comment]hidden note[/comment]", CreatedAt: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
		FileSize: 2048, OwnerId: &ownerID, Owner: &inspectOwner{ID: 9, Name: "Outside Owner"},
		Tags: []string{"tag-one"}, ShareToken: &token, Meta: json.RawMessage(`{"k":1}`), hidden: "unexported",
	}
	reqCtx := WithCredentialFieldsVisible(WithInspect(context.Background()), true)
	out := Process(reqCtx, `[inspect]`, ctx, nil, nil)

	assert.Contains(t, out, `<caption>Properties</caption>`)
	assert.Contains(t, out, `<code>Name</code></th><td>string</td><td>&#34;Root&#34;</td>`)
	assert.Contains(t, out, `value="&#91;property path=&#34;Name&#34;]"`)
	assert.Contains(t, out, `<code>CreatedAt</code></th><td>time</td><td>2026-01-02T03:04:05Z</td>`)
	assert.Contains(t, out, `value="&#91;property path=&#34;CreatedAt&#34; format=&#34;date&#34;]"`)
	assert.Contains(t, out, `value="&#91;property path=&#34;FileSize&#34; format=&#34;filesize&#34;]"`)
	assert.Contains(t, out, `<code>OwnerId</code></th><td>integer</td><td>9</td>`)
	assert.Contains(t, out, `<code>Owner</code></th><td>relation</td><td></td><td>association: values not shown</td>`)
	assert.Contains(t, out, `<code>Tags</code></th><td>list</td><td></td>`)
	assert.Contains(t, out, `&#34;Desc&#34;`, "template comments are stripped from Description")

	assert.NotContains(t, out, "Outside Owner")
	assert.NotContains(t, out, "tag-one")
	assert.NotContains(t, out, "hidden note")
	assert.NotContains(t, out, "ShareToken")
	assert.NotContains(t, out, token)
	assert.NotContains(t, out, "unexported")
	assert.NotContains(t, out, `<code>Meta</code>`)
}

func TestInspectRowBudget(t *testing.T) {
	var b strings.Builder
	b.WriteString("{")
	for i := 0; i < 300; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `"k%03d":%d`, i, i)
	}
	b.WriteString("}")
	out := inspectOn(`[inspect]`, inspectCtx(b.String()))
	assert.Equal(t, maxInspectRows, strings.Count(out, `<th scope="row">`))
	assert.Contains(t, out, `100 more paths not shown. Narrow with path=.`)
}

func TestInspectPanelCap(t *testing.T) {
	out := inspectOn(strings.Repeat(`[inspect]`, 12), inspectCtx(`{"a":1}`))
	assert.Equal(t, maxInspectPanels, strings.Count(out, `<details class="inspect-shortcode">`))
	assert.Equal(t, 1, strings.Count(out, `Further &#91;inspect] panels omitted (limit 10 per preview).`))
	assert.True(t, strings.HasSuffix(out, `per preview).</p>`), "nothing follows the omission note")
}

func TestInspectInsideReloadFace(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42}
	out := Process(WithInspect(context.Background()), `[reload][inspect][/reload]`, ctx, nil, nil)
	assert.Contains(t, out, "[inspect] cannot be used inside a [reload] button face")
	assert.NotContains(t, out, "inspect-shortcode")

	// Without the flag, not even the marker.
	out = Process(context.Background(), `[reload][inspect][/reload]`, ctx, nil, nil)
	assert.NotContains(t, out, "inspect")
}

func TestInspectInsideUsingSaysSo(t *testing.T) {
	f := newUsingFixture()
	reqCtx := WithInspect(context.Background())
	out := Process(reqCtx, `[using type="group" id="7"][inspect][/using]`, noteCtx(), nil, f.exec)
	assert.Contains(t, out, `<summary>Inspect: group #7 · inside &#91;using]</summary>`)
	assert.Contains(t, out, `<code>status</code></th><td>string</td><td>&#34;active&#34;</td>`)
}

func TestInspectInsideMRQLBlockShowsTheItem(t *testing.T) {
	exec := mockExecutor(&QueryResult{EntityType: "resource", Mode: "flat", Items: []QueryResultItem{
		{EntityType: "resource", EntityID: 5, Entity: testEntity{ID: 5, Name: "Photo A"}, Meta: []byte(`{"width":640}`)},
	}}, nil)
	out := Process(WithInspect(context.Background()), `[mrql query="type = resource"][inspect][/mrql]`, inspectCtx(`{"own":1}`), nil, exec)
	assert.Contains(t, out, `<summary>Inspect: resource #5</summary>`)
	assert.Contains(t, out, `<code>width</code>`)
	assert.NotContains(t, out, `<code>own</code>`)
}

func TestInspectEachShowsTheFirstElementOnce(t *testing.T) {
	ctx := inspectCtx(`{"steps":[{"title":"First"},{"title":"Second"}]}`)
	out := inspectOn(`[each path="steps"][inspect]<li>[item path="title"]</li>[/each]`, ctx)

	assert.Equal(t, 1, strings.Count(out, `<details class="inspect-shortcode">`))
	assert.Contains(t, out, `<caption>Current &#91;each] element</caption>`)
	assert.Contains(t, out, `<code>(position)</code></th><td>position</td><td>1</td><td><input type="text" readonly class="inspect-shortcode__snippet" value="&#91;item index=&#34;true&#34;]"`)
	assert.Contains(t, out, `<code>title</code></th><td>string</td><td>&#34;First&#34;</td>`)
	assert.Contains(t, out, `value="&#91;item path=&#34;title&#34;]"`)
	assert.Contains(t, out, `<li>First</li>`)
	assert.Contains(t, out, `<li>Second</li>`)

	without := Process(context.Background(), `[each path="steps"][inspect]<li>[item path="title"]</li>[/each]`, ctx, nil, nil)
	assert.Equal(t, `<li>First</li><li>Second</li>`, without)
}

func TestInspectEachScalarElement(t *testing.T) {
	out := inspectOn(`[each path="xs"][inspect][item][/each]`, inspectCtx(`{"xs":["a","b"]}`))
	assert.Contains(t, out, `<code>(element)</code></th><td>string</td><td>&#34;a&#34;</td><td><input type="text" readonly class="inspect-shortcode__snippet" value="&#91;item]"`)
	assert.True(t, strings.HasSuffix(out, "ab"))
}

// Nested inside another block within the loop, [inspect] is not top-level and
// shows the entity context only, as [item] there renders empty.
func TestInspectNestedInsideEachBlockShowsEntityOnly(t *testing.T) {
	ctx := inspectCtx(`{"on":true,"steps":[{"title":"First"}]}`)
	out := inspectOn(`[each path="steps"][conditional path="on" eq="true"][inspect][/conditional][/each]`, ctx)
	assert.Equal(t, 1, strings.Count(out, `<details class="inspect-shortcode">`))
	assert.NotContains(t, out, `Current &#91;each] element`)
}

func TestInspectAccessibleStructure(t *testing.T) {
	ctx := inspectCtx(`{"project":{"name":"Apollo"}}`)
	ctx.Entity = &inspectEntity{ID: 42, Name: "Root"}
	out := inspectOn(`[each path="project.list"][/each][inspect]`, ctx)

	assert.Contains(t, out, `<details class="inspect-shortcode"><summary>`)
	tables := strings.Count(out, "<table>")
	assert.Equal(t, 2, tables)
	assert.Equal(t, tables, strings.Count(out, "<table><caption>"))
	assert.Equal(t, tables*4, strings.Count(out, `<th scope="col">`))
	assert.Equal(t, strings.Count(out, "<tr><th"), strings.Count(out, `<tr><th scope="row">`)+tables)
	assert.Equal(t, tables, strings.Count(out, `role="region"`))
	assert.Equal(t, tables, strings.Count(out, `tabindex="0"`))
	assert.NotRegexp(t, `<h[1-6]`, out, "no headings, so no level skips in the host slot")

	inputs := regexp.MustCompile(`<input [^>]*>`).FindAllString(out, -1)
	require.NotEmpty(t, inputs)
	for _, in := range inputs {
		assert.Contains(t, in, " readonly ")
		assert.Regexp(t, `aria-label="[^"]+"`, in)
	}
}
