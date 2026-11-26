package shortcodes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"mahresources/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// calcSC builds an inline [calc] shortcode from alternating attribute names and
// values: calcSC("expr", "1 + 2", "decimals", "2").
func calcSC(kv ...string) Shortcode {
	attrs := map[string]string{}
	for i := 0; i+1 < len(kv); i += 2 {
		attrs[kv[i]] = kv[i+1]
	}
	return Shortcode{Name: "calc", Attrs: attrs}
}

func calcMeta(t *testing.T, meta map[string]any) MetaShortcodeContext {
	t.Helper()
	return MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: makeMetaJSON(t, meta)}
}

func isCalcMarker(s string) bool {
	return strings.HasPrefix(s, `<span class="shortcode-error`) && strings.Contains(s, "⚠ calc")
}

func TestCalcArithmetic(t *testing.T) {
	ctx := calcMeta(t, map[string]any{"a": 5})
	cases := []struct{ expr, want string }{
		{"2 + 3 * 4", "14"},
		{"(2 + 3) * 4", "20"},
		{"-meta.a", "-5"},
		{"--2", "2"},
		{"+-+2", "-2"},
		{"7 % 3", "1"},
		{"-7 % 3", "-1"},
		{"10 - 4 - 3", "3"},
		{"100 / 10 / 5", "2"},
		{"meta.a*2", "10"},
		{"  1\t+\n1 ", "2"},
		{"0.5 * 3", "1.5"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, RenderCalcShortcode(calcSC("expr", c.expr), ctx), c.expr)
	}
}

func TestCalcPitchExample(t *testing.T) {
	expr := "meta.qty * meta.unit_price * (1 - coalesce(meta.discount, 0))"
	ctx := calcMeta(t, map[string]any{"qty": 3, "unit_price": 12.5, "discount": 0.1})
	assert.Equal(t, "33.75", RenderCalcShortcode(calcSC("expr", expr, "decimals", "2", "default", "-"), ctx))

	ctx = calcMeta(t, map[string]any{"qty": 3, "unit_price": 12.5})
	assert.Equal(t, "37.50", RenderCalcShortcode(calcSC("expr", expr, "decimals", "2", "default", "-"), ctx))

	for _, meta := range []map[string]any{{"unit_price": 12.5}, {"qty": "abc", "unit_price": 12.5}} {
		assert.Equal(t, "-", RenderCalcShortcode(calcSC("expr", expr, "decimals", "2", "default", "-"), calcMeta(t, meta)))
	}
}

func TestCalcOperandConversion(t *testing.T) {
	numeric := map[string]any{"a": "12.50", "b": " 3 ", "c": "-1e2", "d": ".5"}
	ctx := calcMeta(t, numeric)
	assert.Equal(t, "12.5", RenderCalcShortcode(calcSC("expr", "meta.a"), ctx))
	assert.Equal(t, "3", RenderCalcShortcode(calcSC("expr", "meta.b"), ctx))
	assert.Equal(t, "-100", RenderCalcShortcode(calcSC("expr", "meta.c"), ctx))
	assert.Equal(t, "0.5", RenderCalcShortcode(calcSC("expr", "meta.d"), ctx))
	atCap := calcMeta(t, map[string]any{"n": strings.Repeat("0", calcMaxNumericString-1) + "7"})
	assert.Equal(t, "7", RenderCalcShortcode(calcSC("expr", "meta.n"), atCap))

	bad := map[string]any{
		"comma": "12,5", "word": "abc", "hex": "0x10", "nan": "NaN", "inf": "Inf",
		"under": "1_000", "empty": "", "yes": true, "nil": nil,
		"obj": map[string]any{}, "arr": []any{}, "huge": "1e999",
		// Numeric, but longer than any float64 needs: not read at all.
		"long": "1" + strings.Repeat("0", calcMaxNumericString),
	}
	ctx = calcMeta(t, bad)
	for key := range bad {
		assert.Equal(t, "n/a", RenderCalcShortcode(calcSC("expr", "meta."+key+" + 1", "default", "n/a"), ctx), key)
		assert.Equal(t, "", RenderCalcShortcode(calcSC("expr", "meta."+key), ctx), key)
	}
	assert.Equal(t, "n/a", RenderCalcShortcode(calcSC("expr", "meta.missing", "default", "n/a"), ctx))
	assert.Equal(t, "n/a", RenderCalcShortcode(calcSC("expr", "meta.obj.deeper", "default", "n/a"), ctx))

	nested := calcMeta(t, map[string]any{"price": map[string]any{"net": 10}})
	assert.Equal(t, "10", RenderCalcShortcode(calcSC("expr", "meta.price.net"), nested))
}

func TestCalcNonFinite(t *testing.T) {
	ctx := calcMeta(t, map[string]any{"a": 4, "b": 0, "big": 1e308})
	for _, expr := range []string{"meta.a / 0", "meta.a % 0", "meta.big * 10", "meta.a / meta.b"} {
		assert.Equal(t, "-", RenderCalcShortcode(calcSC("expr", expr, "default", "-"), ctx), expr)
	}
	assert.Equal(t, "0", RenderCalcShortcode(calcSC("expr", "coalesce(meta.a / meta.b, 0)"), ctx))
	assert.Equal(t, "-", RenderCalcShortcode(calcSC("expr", "meta.big * 10 - meta.big * 10", "default", "-"), ctx))
}

func TestCalcFunctions(t *testing.T) {
	ctx := calcMeta(t, map[string]any{"a": 7})
	cases := []struct{ expr, want string }{
		{"min(3, 1, 2)", "1"},
		{"max(3, 1, 2)", "3"},
		{"min(4)", "4"},
		{"abs(-2)", "2"},
		{"round(2.5)", "3"},
		{"round(-2.5)", "-3"},
		{"round(2.675, 2)", "2.68"},
		{"round(1.005, 2)", "1.01"},
		{"round(1, 11)", "?"},
		{"round(1, 1.5)", "?"},
		{"round(1, -1)", "?"},
		{"coalesce(meta.missing, meta.a, 1)", "7"},
		// The later argument would fail, but is never reached.
		{"coalesce(meta.a, meta.nothing.here)", "7"},
		{"coalesce(meta.x, meta.y)", "?"},
		{"max(meta.missing, 1)", "?"},
		{"min(1, 2, 3, 4, 5, 6, 7, 8)", "1"},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, RenderCalcShortcode(calcSC("expr", c.expr, "default", "?"), ctx), c.expr)
	}
	for _, expr := range []string{"abs(1, 2)", "round()", "round(1, 2, 3)", "min()", "min(1, 2, 3, 4, 5, 6, 7, 8, 9)", "coalesce()"} {
		assert.True(t, isCalcMarker(RenderCalcShortcode(calcSC("expr", expr), ctx)), expr)
	}
}

func TestCalcFormatting(t *testing.T) {
	ctx := calcMeta(t, map[string]any{"big": 1e20, "neg": -1234.0})
	cases := []struct {
		expr, decimals, format, want string
	}{
		{"0.1 + 0.2", "", "", "0.3"},
		{"1 / 3", "", "", "0.3333333333"},
		{"10", "2", "", "10.00"},
		{"-0.001", "2", "", "0.00"},
		{"-0.001", "", "", "-0.001"},
		{"-0.00000000001", "", "", "0"},
		{"0 * -1", "", "", "0"},
		{"2.5", "0", "", "3"},
		{"1234567.891", "2", "grouped", "1,234,567.89"},
		{"meta.neg", "", "grouped", "-1,234"},
		{"999", "", "grouped", "999"},
		{"1000", "", "grouped", "1,000"},
		{"0.256", "", "percent", "25.6%"},
		{"0.256", "0", "percent", "26%"},
		{"1234.5", "", "number", "1234.5"},
		{"meta.big", "", "", "100000000000000000000"},
		{"meta.big", "", "grouped", "100,000,000,000,000,000,000"},
		{"9.9999999999999", "", "", "10"},
		{"0.995", "2", "", "1.00"},
	}
	for _, c := range cases {
		sc := calcSC("expr", c.expr)
		if c.decimals != "" {
			sc.Attrs["decimals"] = c.decimals
		}
		if c.format != "" {
			sc.Attrs["format"] = c.format
		}
		got := RenderCalcShortcode(sc, ctx)
		assert.Equal(t, c.want, got, "%s decimals=%q format=%q", c.expr, c.decimals, c.format)
		assert.NotContains(t, got, "e+", c.expr)
	}
}

type calcCredentialEntity struct {
	ID         uint
	ShareToken int
}

func TestCalcProperty(t *testing.T) {
	res := &models.Resource{ID: 9, Name: "pic", Width: 1920, Height: 1080}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 9, Entity: res}
	assert.Equal(t, "2073600", RenderCalcShortcode(calcSC("expr", "property.Width * property.Height"), ctx))
	assert.Equal(t, "2.07", RenderCalcShortcode(calcSC("expr", "property.Width * property.Height / 1000000", "decimals", "2"), ctx))
	assert.Equal(t, "-", RenderCalcShortcode(calcSC("expr", "property.OwnerId + 1", "default", "-"), ctx))
	assert.Equal(t, "-", RenderCalcShortcode(calcSC("expr", "property.Name", "default", "-"), ctx))
	assert.Equal(t, "-", RenderCalcShortcode(calcSC("expr", "property.CreatedAt", "default", "-"), ctx))
	assert.Equal(t, "-", RenderCalcShortcode(calcSC("expr", "property.Tags.0", "default", "-"), ctx))

	owner := uint(4)
	res.OwnerId = &owner
	assert.Equal(t, "5", RenderCalcShortcode(calcSC("expr", "property.OwnerId + 1"), ctx))

	cred := MetaShortcodeContext{Entity: calcCredentialEntity{ID: 1, ShareToken: 42}}
	assert.Equal(t, "-", RenderCalcShortcode(calcSC("expr", "property.ShareToken", "default", "-"), cred))
	cred.credentialsVisible = true
	assert.Equal(t, "42", RenderCalcShortcode(calcSC("expr", "property.ShareToken"), cred))

	assert.Equal(t, "-", RenderCalcShortcode(calcSC("expr", "property.Width", "default", "-"), MetaShortcodeContext{}))
}

func TestCalcAuthorErrorsRenderMarker(t *testing.T) {
	ctx := calcMeta(t, map[string]any{"a": 1})
	exprs := map[string]string{
		"too long":         strings.Repeat("1+", 256) + "1",
		"too many tokens":  strings.Repeat("1+", 64) + "1",
		"too deep":         strings.Repeat("(", 33) + "1" + strings.Repeat(")", 33),
		"deep unary":       strings.Repeat("-", 33) + "1",
		"bare identifier":  "qty * 2",
		"unknown root":     "entity.x",
		"unknown function": "sqrt(4)",
		"trailing op":      "1 +",
		"empty parens":     "()",
		"no segment":       "meta.",
		"hyphenated key":   "meta.unit-price",
		"trailing dot":     "meta.a.",
		"leading dot num":  ".5",
		"exponent literal": "1e5",
		"long literal":     strings.Repeat("1", 33),
		"unclosed paren":   "(1 + 2",
		"stray close":      "1 + 2)",
		"bad char":         "1 & 2",
		"quote":            `1 + "2"`,
		"two numbers":      "1 2",
		"numeric meta seg": "meta.0",
		"too many refs":    strings.TrimSuffix(strings.Repeat("meta.a+", 33), "+"),
	}
	require.Less(t, len(exprs["too many refs"]), 512)
	for name, expr := range exprs {
		got := RenderCalcShortcode(calcSC("expr", expr), ctx)
		assert.True(t, isCalcMarker(got), "%s: %q rendered %q", name, expr, got)
	}

	// The limits are inclusive.
	assert.Equal(t, "1", RenderCalcShortcode(calcSC("expr", strings.Repeat("(", 32)+"1"+strings.Repeat(")", 32)), ctx))
	assert.Equal(t, "65", RenderCalcShortcode(calcSC("expr", strings.Repeat("1+", 63)+"2"), ctx))
	assert.Equal(t, "32", RenderCalcShortcode(calcSC("expr", strings.TrimSuffix(strings.Repeat("meta.a+", 32), "+")), ctx))

	for _, attrs := range [][]string{
		{"expr", "1", "decimals", "x"},
		{"expr", "1", "decimals", "11"},
		{"expr", "1", "decimals", "-1"},
		{"expr", "1", "decimals", "1.5"},
		{"expr", "1", "format", "currency"},
	} {
		assert.True(t, isCalcMarker(RenderCalcShortcode(calcSC(attrs...), ctx)), "%v", attrs)
	}

	got := RenderCalcShortcode(calcSC("expr", "<script>alert(1)</script>"), ctx)
	assert.True(t, isCalcMarker(got))
	assert.NotContains(t, got, "<script>")
	assert.Contains(t, got, "&lt;script&gt;")

	// A blank expression renders nothing, like [meta] with no path.
	assert.Equal(t, "", RenderCalcShortcode(calcSC("expr", "  ", "default", "-"), ctx))
	assert.Equal(t, "", RenderCalcShortcode(calcSC(), ctx))
}

func TestCalcDefaultIsEscaped(t *testing.T) {
	got := RenderCalcShortcode(calcSC("expr", "meta.none", "default", "<b>x</b>"), calcMeta(t, map[string]any{}))
	assert.Equal(t, "&lt;b&gt;x&lt;/b&gt;", got)
	ctx := calcMeta(t, map[string]any{})
	ctx.UntrustedSource = true
	assert.Equal(t, "&lt;b&gt;x&lt;/b&gt;", RenderCalcShortcode(calcSC("expr", "meta.none", "default", "<b>x</b>"), ctx))
}

// [calc] reads only the context it was handed: no form of it reaches the
// query executor or a plugin, so a subtree-confined reader sees nothing through
// it that the page had not already loaded for them.
func TestCalcNeverQueriesOrCallsPlugins(t *testing.T) {
	executor := func(context.Context, string, QueryOptions) (*QueryResult, error) {
		t.Fatal("[calc] called the query executor")
		return nil, nil
	}
	renderer := func(string, Shortcode, MetaShortcodeContext) (string, error) {
		t.Fatal("[calc] called the plugin renderer")
		return "", nil
	}
	tpl := `[calc expr="meta.qty * meta.unit_price" decimals="2"]|` +
		`[calc expr="property.Width * property.Height"]|` +
		`[calc expr="coalesce(meta.none, item, 1)"]|` +
		`[calc expr="nope"]|[calc expr="1" format="bad"]|` +
		`[each path="lines"][calc expr="item.qty * item.price" decimals="2"];[/each]`
	ctx := MetaShortcodeContext{
		EntityType: "resource", EntityID: 1,
		Entity: &models.Resource{ID: 1, Width: 2, Height: 3},
		Meta:   json.RawMessage(`{"qty":3,"unit_price":12.5,"lines":[{"qty":2,"price":1.25}]}`),
	}
	out := Process(context.Background(), tpl, ctx, renderer, executor)
	parts := strings.Split(out, "|")
	require.Len(t, parts, 6, out)
	assert.Equal(t, "37.50", parts[0])
	assert.Equal(t, "6", parts[1])
	assert.Equal(t, "1", parts[2])
	assert.True(t, isCalcMarker(parts[3]))
	assert.True(t, isCalcMarker(parts[4]))
	assert.Equal(t, "2.50;", parts[5])
}

func TestCalcReportsComputedNumberToObserver(t *testing.T) {
	var seen []string
	reqCtx := WithScalarFormatObserver(context.Background(), func(s string) { seen = append(seen, s) })
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"qty":3,"unit_price":12.5,"discount":0.1}`)}
	out := Process(reqCtx, `[calc expr="meta.qty * meta.unit_price * (1 - coalesce(meta.discount, 0))" decimals="2"]`, ctx, nil, nil)
	assert.Equal(t, "33.75", out)
	assert.Equal(t, []string{"33.75"}, seen)

	seen = nil
	out = Process(reqCtx, `[calc expr="meta.missing" default="not set"]`, ctx, nil, nil)
	assert.Equal(t, "not set", out)
	assert.Empty(t, seen)
}

func TestCalcInsideEach(t *testing.T) {
	meta := json.RawMessage(`{"rate":2,"lines":[{"qty":2,"price":1.25},{"qty":1,"price":4}],"nums":[1,2.5],` +
		`"groups":[{"w":1},{"w":2}],"rows":[{"v":1},{"v":10}]}`)
	ctx := MetaShortcodeContext{Meta: meta}
	run := func(tpl string) string { return Process(context.Background(), tpl, ctx, nil, nil) }

	assert.Equal(t, "2.50;4.00;", run(`[each path="lines"][calc expr="item.qty * item.price" decimals="2"];[/each]`))
	assert.Equal(t, "2;5;", run(`[each path="nums"][calc expr="item * 2"];[/each]`))
	assert.Equal(t, "5;8;", run(`[each path="lines"][calc expr="item.qty * item.price * meta.rate"];[/each]`))
	assert.Equal(t, "?", run(`[calc expr="item.x" default="?"]`))
	assert.Equal(t, "?", run(`[calc expr="item" default="?"]`))
	// A nested [each] binds its own element (inner paths are absolute Meta
	// paths, as for [item]), and the outer [calc] binds the outer one.
	assert.Equal(t, "1(10;100;)2(10;100;)", run(`[each path="groups"][calc expr="item.w"]([each path="rows"][calc expr="item.v * 10"];[/each])[/each]`))
	// Inside another block within the branch the element is not visible, but
	// the entity still is.
	assert.Equal(t, "?2|?2|", run(`[each path="lines"][conditional path="rate" eq="2"][calc expr="item.qty" default="?"][calc expr="meta.rate"][/conditional]|[/each]`))
	// An object element is not a number.
	assert.Equal(t, "?", run(`[each path="lines" limit="1"][calc expr="item" default="?"][/each]`))
}

func TestCalcParserNamesTheReferences(t *testing.T) {
	prog, err := parseCalcExpr("meta.a.b + property.Tags.0 * coalesce(item, item.x) - meta.c")
	require.NoError(t, err)
	var got []string
	for _, r := range prog.refs {
		got = append(got, r.root+":"+r.path)
	}
	assert.Equal(t, []string{"meta:a.b", "property:Tags.0", "item:", "item:x", "meta:c"}, got)
}
