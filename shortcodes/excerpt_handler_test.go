package shortcodes

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"mahresources/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// excerptNote is a note context whose Description is desc.
func excerptNote(desc string) MetaShortcodeContext {
	return MetaShortcodeContext{EntityType: "note", EntityID: 42, Entity: &models.Note{ID: 42, Name: "Sunset <notes>", Description: desc}}
}

// excerptRender runs template through Process against ctx, with no executor.
func excerptRender(template string, ctx MetaShortcodeContext) string {
	return Process(context.Background(), template, ctx, nil, nil)
}

func TestExcerptParses(t *testing.T) {
	scs := Parse(`[excerpt words="5" more="Read more"]`)
	require.Len(t, scs, 1)
	assert.Equal(t, "excerpt", scs[0].Name)
	assert.Equal(t, map[string]string{"words": "5", "more": "Read more"}, scs[0].Attrs)
}

func TestExcerptStripsMarkdownAndHTML(t *testing.T) {
	cases := []struct{ name, desc, want string }{
		{"markdown", "# Title\n\n**Bold** and [a link](http://x) and `code`", "Title Bold and a link and code"},
		{"html", `<script>alert(1)</script><b>hi</b> <img src=x onerror=y alt="alt text"> Tom & <Jerry>`, "hi Tom &amp;"},
		{"escaped text", "Tom & Jerry say 5 < 6 \"quoted\" 'single'", "Tom &amp; Jerry say 5 &lt; 6 &#34;quoted&#34; &#39;single&#39;"},
		{"html comment and style", "a <!-- hidden --> b <style>.x{}</style> c", "a b c"},
		{"blocks become spaces", "<p>one</p><p>two</p><ul><li>three</li><li>four</li></ul>", "one two three four"},
		{"entities", "caf&eacute; &amp; &lt;b&gt;", "café &amp; &lt;b&gt;"},
		{"whitespace collapses", "a\n\n\tb\u00a0\u00a0c   d", "a b c d"},
		{"gfm", "~~gone~~ kept\n\n| h1 | h2 |\n|----|----|\n| c1 | c2 |\n\n- [x] done", "gone kept h1 h2 c1 c2 done"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, excerptRender(`[excerpt words="200"]`, excerptNote(c.desc)), c.name)
	}
}

func TestExcerptDropsHiddenHTMLSubtrees(t *testing.T) {
	for _, c := range []struct{ name, desc, want string }{
		{"hidden span", `Before <span hidden>Unpublished draft</span> after`, "Before after"},
		{"nested spans", `Before <span HIDDEN="false">draft <span>nested</span> still hidden</span> after`, "Before after"},
		{"void element", `Before <img hidden src="x.png" alt="Draft"> after`, "Before after"},
		{"raw text in hidden subtree", `Before <span hidden><script>"</span>"</script> still hidden</span> after`, "Before after"},
		{"does not interpret CSS or ARIA", `Before <span style="display:none">styled</span> <span aria-hidden="true">ARIA</span> after`, "Before styled ARIA after"},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, excerptRender(`[excerpt]`, excerptNote(c.desc)))
		})
	}
}

func TestExcerptRemovesShortcodesWithoutRunningThem(t *testing.T) {
	executor := func(context.Context, string, QueryOptions) (*QueryResult, error) {
		t.Fatal("an embedded shortcode ran a query")
		return nil, nil
	}
	ctx := excerptNote(`intro [mrql query='type = note'] mid [details summary="s"]secret[/details] [plugin:x:y a="1"] end [/meta]`)
	assert.Equal(t, "intro mid end", Process(context.Background(), `[excerpt]`, ctx, nil, executor))

	// An unclosed block opener goes; its body is the entity's own text and stays.
	assert.Equal(t, "lead body", excerptRender(`[excerpt]`, excerptNote(`lead [details]body`)))
}

func TestExcerptPreservesMarkdownLabelsNamedLikeShortcodes(t *testing.T) {
	for _, c := range []struct{ name, desc, want string }{
		{"inline link", "See [link](https://example.org)", "See link"},
		{"inline image", "Before ![excerpt](image.png) after", "Before after"},
		{"inline link with trailing space", "See [link ](https://example.org)", "See link"},
		{"inline image with trailing space", "Before ![excerpt ](image.png) after", "Before after"},
		{"full reference", "See [link][target].\n\n[target]: https://example.org", "See link."},
		{"reference definition with trailing space", "See [link].\n\n[link ]: https://example.org", "See link."},
		{"nested full reference", "See [a [nested] label][link].\n\n[link]: https://example.org", "See a [nested] label."},
		{"collapsed reference", "See [link][].\n\n[link]: https://example.org", "See link."},
		{"shortcut reference", "See [link].\n\n[link]: https://example.org", "See link."},
		{"shortcut image", "Before ![excerpt] after\n\n[excerpt]: image.png", "Before after"},
		{"full reference image", "Before ![excerpt][link] after\n\n[link]: image.png", "Before after"},
		{"not a reference", "before [link] after\n\n`[link]: x`", "before after : x"},
		{"not a block opener", "[details](https://example.org) kept [/details]", "details kept"},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, excerptRender(`[excerpt words="200"]`, excerptNote(c.desc)))
		})
	}
}

func TestExcerptRemovesShortcodesInsideCode(t *testing.T) {
	assert.Equal(t, "before code here x", excerptRender(`[excerpt]`, excerptNote("before `code [property path=\"Name\"] here` x")))
	assert.Equal(t, "top code bottom", excerptRender(`[excerpt]`, excerptNote("top\n\n```\ncode [meta path=\"a\"]\n```\n\nbottom")))
}

func TestExcerptRemovesTemplateCommentsAndDoesNotRecurse(t *testing.T) {
	assert.Equal(t, "shown tail", excerptRender(`[excerpt]`, excerptNote(`shown [comment]hidden[/comment] tail`)))
	// An [excerpt] in the Description is deleted, not expanded.
	assert.Equal(t, "self ref", excerptRender(`[excerpt]`, excerptNote(`self [excerpt words="2"] ref`)))
}

func TestExcerptMentions(t *testing.T) {
	assert.Equal(t, "see Sunset photo now", excerptRender(`[excerpt]`, excerptNote("see @[resource:5:Sunset photo] now")))
	// The name comes out verbatim: not read as Markdown or HTML.
	assert.Equal(t, "a*b*_c_&lt;d&gt;", excerptRender(`[excerpt]`, excerptNote("@[resource:5:a*b*_c_<d>]")))
	// Sentinel runes already in the text are removed, so they cannot pull a
	// name into the wrong place.
	assert.Equal(t, "x0 y Name z", excerptRender(`[excerpt]`, excerptNote("x\uE0000\uE001 y @[note:9:Name] z")))
	// An invalid mention stays as written.
	assert.Equal(t, "@[note:0:Zero]", excerptRender(`[excerpt]`, excerptNote("@[note:0:Zero]")))
}

func TestExcerptMentionSentinelsCannotBeForgedByEntities(t *testing.T) {
	encoded := "&#57344;0&#57345;"
	desc := "<div>before " + strings.Repeat(encoded, 450) + " after <!-- @[note:1:HIDDEN " +
		strings.Repeat("a ", 4000) + "] --></div>"
	require.Less(t, len(desc), excerptSourceCap)
	assert.Equal(t, "before "+strings.Repeat("0", 450)+" after", excerptRender(`[excerpt words="200"]`, excerptNote(desc)))
}

func TestExcerptRestoreMentionsOnlyRestoresEmittedSentinelsOnce(t *testing.T) {
	marker, nonce, names := excerptMentionSentinels("@[note:1:Alpha]")
	otherMarker, _, _ := excerptMentionSentinels("@[note:2:Beta]")
	require.NotEqual(t, marker, otherMarker, "each conversion needs a fresh nonce")
	assert.Equal(t, strings.Trim(otherMarker, "\uE000\uE001"), excerptRestoreMentions(otherMarker, nonce, names), "different conversion")
	assert.Equal(t, nonce+"7 b", excerptRestoreMentions("\uE000"+nonce+"7\uE001 b", nonce, names), "out of range")
	assert.Equal(t, nonce+"00 b", excerptRestoreMentions("\uE000"+nonce+"00\uE001 b", nonce, names), "non-canonical index")
	assert.Equal(t, nonce+"0 b", excerptRestoreMentions("\uE000"+nonce+"0 b", nonce, names), "no closer")
	assert.Equal(t, "a b", excerptRestoreMentions("a\uE001 b", nonce, names), "stray closer")
	assert.Equal(t, "Alpha b", excerptRestoreMentions(marker+marker+" b", nonce, names), "a marker restores at most once")
}

func TestExcerptTruncation(t *testing.T) {
	cases := []struct{ words, desc, want string }{
		{"3", "one two three four", "one two three…"},
		{"3", "one two three", "one two three"},
		{"3", "a, b, c, d", "a, b, c…"},
		{"1", "— dash", "—…"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, excerptRender(`[excerpt words="`+c.words+`"]`, excerptNote(c.desc)), c.desc)
	}
}

func TestExcerptRuneCap(t *testing.T) {
	got := excerptRender(`[excerpt words="10"]`, excerptNote(strings.Repeat("漢", 500)))
	assert.Equal(t, strings.Repeat("漢", 10*excerptRunesPerWord)+"…", got)
}

func TestExcerptRuneCapBacksUpOverPartialCharacters(t *testing.T) {
	for _, c := range []struct{ name, prefix, cluster, suffix, want string }{
		{"combining accent", strings.Repeat("a", 11), "e\u0301", "x", strings.Repeat("a", 11) + "…"},
		{"enclosing mark", strings.Repeat("a", 11), "e\u20dd", "x", strings.Repeat("a", 11) + "…"},
		{"joiner at cut", strings.Repeat("a", 11), "👩‍💻", "x", strings.Repeat("a", 11) + "…"},
		{"joiner before cut", strings.Repeat("a", 10), "👩‍💻", "x", strings.Repeat("a", 10) + "…"},
		{"variation selector", strings.Repeat("a", 11), "✌️", "x", strings.Repeat("a", 11) + "…"},
		{"emoji modifier", strings.Repeat("a", 11), "👍🏿", "x", strings.Repeat("a", 11) + "…"},
		{"completed accent at cut", strings.Repeat("a", 10), "e\u0301", "x", strings.Repeat("a", 10) + "e\u0301…"},
		{"cluster bigger than cap", "", "e" + strings.Repeat("\u0301", 12), "x", "…"},
	} {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.want, excerptRender(`[excerpt words="1"]`, excerptNote(c.prefix+c.cluster+c.suffix)))
		})
	}
}

func TestExcerptWordsAttribute(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("w ", 300))
	countWords := func(out string) int { return len(strings.Fields(strings.TrimSuffix(out, "…"))) }
	for _, v := range []string{"abc", "0", "-1", "", " 2.5 "} {
		assert.Equal(t, excerptDefaultWords, countWords(excerptRender(`[excerpt words="`+v+`"]`, excerptNote(long))), v)
	}
	assert.Equal(t, excerptMaxWords, countWords(excerptRender(`[excerpt words="500"]`, excerptNote(long))))
	assert.Equal(t, 7, countWords(excerptRender(`[excerpt words=" 7 "]`, excerptNote(long))))
}

func TestExcerptSourceCap(t *testing.T) {
	huge := strings.Repeat("word ", (1<<20)/5)
	got := excerptRender(`[excerpt words="200"]`, excerptNote(huge))
	assert.True(t, strings.HasSuffix(got, "…"))
	assert.LessOrEqual(t, utf8.RuneCountInString(got), excerptMaxWords*excerptRunesPerWord+1)

	// The cut lands inside a [comment]: the unclosed opener hides the rest of
	// the capped prefix, so neither the comment body nor the tail leaks.
	head := strings.Repeat("a", excerptSourceCap-10-len("[comment]"))
	desc := head + " x [comment]" + strings.Repeat("s", 100) + "[/comment] tail"
	got = excerptRender(`[excerpt words="200"]`, excerptNote(desc))
	assert.NotContains(t, got, "sss")
	assert.NotContains(t, got, "tail")
	assert.True(t, strings.HasSuffix(got, "…"), "a capped source always ends with an ellipsis")

	// A short visible text from a capped source still gets the ellipsis.
	short := "only words\n\n<!--" + strings.Repeat("x", excerptSourceCap) + "-->"
	assert.Equal(t, "only words…", excerptRender(`[excerpt]`, excerptNote(short)))

	// The cut never splits a rune.
	multi := strings.Repeat("é", excerptSourceCap)
	assert.True(t, utf8.ValidString(excerptRender(`[excerpt words="1"]`, excerptNote(multi))))
}

func TestExcerptScratchBudget(t *testing.T) {
	m := newRenderMeter(WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1 << 20}))
	require.True(t, m.chargeScratch(maxScratchBytes-100))
	reqCtx := context.WithValue(context.Background(), renderMeterKey{}, m)
	sc := Shortcode{Name: "excerpt", Attrs: map[string]string{}}
	assert.Equal(t, "", RenderExcerptShortcode(reqCtx, sc, excerptNote("some text that is long enough to charge")))
	assert.ErrorIs(t, m.Err(), ErrRenderBudgetExceeded)
}

func TestExcerptPath(t *testing.T) {
	// No Entity: path reads Meta alone.
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 3, Meta: makeMetaJSON(t, map[string]any{
		"intro": map[string]any{"text": "**Hello** there"},
		"n":     5,
		"obj":   map[string]any{"a": "b"},
	})}
	assert.Equal(t, "Hello there", excerptRender(`[excerpt path="intro.text"]`, ctx))
	for _, p := range []string{"n", "obj", "missing", "intro"} {
		assert.Equal(t, "", excerptRender(`[excerpt path="`+p+`"]`, ctx), p)
		assert.Equal(t, "none", excerptRender(`[excerpt path="`+p+`" default="none"]`, ctx), p)
	}
	assert.Equal(t, "&lt;b&gt;x&lt;/b&gt;", excerptRender(`[excerpt path="n" default="<b>x</b>"]`, ctx))
}

func TestExcerptField(t *testing.T) {
	token := "secret-token"
	note := &models.Note{ID: 7, Name: "**Named** note", Description: "desc", ShareToken: &token}
	ctx := MetaShortcodeContext{EntityType: "note", EntityID: 7, Entity: note}
	assert.Equal(t, "Named note", excerptRender(`[excerpt field="Name"]`, ctx))
	for _, f := range []string{"Owner.Name", "ID", "Missing", "shareToken", "0"} {
		assert.Equal(t, "", excerptRender(`[excerpt field="`+f+`"]`, ctx), f)
	}
	assert.Equal(t, "", excerptRender(`[excerpt field="ShareToken"]`, ctx), "a credential is withheld by default")
	visible := Process(WithCredentialFieldsVisible(context.Background(), true), `[excerpt field="ShareToken"]`, ctx, nil, nil)
	assert.Equal(t, "secret-token", visible)

	assert.Equal(t, "", excerptRender(`[excerpt]`, MetaShortcodeContext{EntityType: "note", EntityID: 7}), "no entity")
}

func TestExcerptPathAndFieldTogether(t *testing.T) {
	got := excerptRender(`[excerpt path="a" field="Name"]`, excerptNote("x"))
	assert.Contains(t, got, "shortcode-error")
	assert.Contains(t, got, "⚠ excerpt")
}

func TestExcerptMoreLink(t *testing.T) {
	long := "one two three four"
	assert.Equal(t, "one two three four", excerptRender(`[excerpt words="10" more="Read more"]`, excerptNote(long)), "no link when nothing was cut")
	assert.Equal(t, "one two…", excerptRender(`[excerpt words="2" more="  "]`, excerptNote(long)), "a blank label is no label")

	want := `one two… <a href="/note?id=42" class="underline" aria-label="Read more: Sunset &lt;notes&gt;">Read more</a>`
	assert.Equal(t, want, excerptRender(`[excerpt words="2" more="Read more"]`, excerptNote(long)))

	for typ, href := range map[string]string{"resource": "/resource?id=9", "group": "/group?id=9"} {
		ctx := MetaShortcodeContext{EntityType: typ, EntityID: 9, Entity: &models.Resource{ID: 9, Name: "", Description: long}}
		got := excerptRender(`[excerpt words="1" more="More"]`, ctx)
		assert.Contains(t, got, `href="`+href+`"`, typ)
		assert.Contains(t, got, `aria-label="More: `+typ+` 9"`, "a blank name falls back to the type and ID")
	}

	for _, ctx := range []MetaShortcodeContext{
		{EntityType: "tag", EntityID: 9, Entity: &models.Note{Description: long}},
		{EntityType: "note", EntityID: 0, Entity: &models.Note{Description: long}},
	} {
		assert.Equal(t, "one…", excerptRender(`[excerpt words="1" more="More"]`, ctx), "no resolvable target, no link")
	}

	// A read-only render keeps the link: it is navigation, not an edit.
	ro := excerptNote(long)
	ro.ForceReadOnly = true
	assert.Equal(t, want, excerptRender(`[excerpt words="2" more="Read more"]`, ro))

	// The public share server serves no detail pages, so no link there.
	share := Process(WithPublicSharePage(context.Background()), `[excerpt words="2" more="Read more"]`, excerptNote(long), nil, nil)
	assert.Equal(t, "one two…", share)

	// A button face cannot contain a link.
	face := Process(context.Background(), `[reload][excerpt words="2" more="Read more"][/reload]`, excerptNote(long), nil, nil)
	assert.Contains(t, face, "one two…")
	assert.NotContains(t, face, "<a ")
}

func TestExcerptUntrustedSourceIsByteIdentical(t *testing.T) {
	desc := "<b>bold</b> & **md** @[note:1:<x>] words beyond the limit here"
	trusted := excerptNote(desc)
	untrusted := excerptNote(desc)
	untrusted.UntrustedSource = true
	tpl := `[excerpt words="4" more="More"]`
	assert.Equal(t, excerptRender(tpl, trusted), excerptRender(tpl, untrusted))
}

func TestExcerptInsideMRQLAndUsing(t *testing.T) {
	items := []QueryResultItem{
		{EntityType: "note", EntityID: 1, Entity: &models.Note{ID: 1, Name: "n1", Description: "first note text"}},
		{EntityType: "note", EntityID: 2, Entity: &models.Note{ID: 2, Name: "n2", Description: "second note text"}},
	}
	executor := func(context.Context, string, QueryOptions) (*QueryResult, error) {
		return &QueryResult{EntityType: "note", Mode: "flat", Items: items}, nil
	}
	outer := excerptNote("outer text")
	got := Process(context.Background(), `[mrql query="type = note"]<p>[excerpt words="2"]</p>[/mrql]`, outer, nil, executor)
	assert.Contains(t, got, "<p>first note…</p>")
	assert.Contains(t, got, "<p>second note…</p>")
	assert.NotContains(t, got, "outer")

	got = Process(context.Background(), `[using query="type = note"][excerpt words="1"][/using]`, outer, nil, executor)
	assert.Equal(t, "first…", got)
}

func TestExcerptIsABareValueInAnAttribute(t *testing.T) {
	got := excerptRender(`<span title="[excerpt words='3']">x</span>`, excerptNote(`say "hi" & <go> now`))
	assert.Equal(t, `<span title="say &#34;hi&#34; &amp;…">x</span>`, got)
}
