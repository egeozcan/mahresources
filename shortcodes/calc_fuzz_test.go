package shortcodes

import (
	"encoding/json"
	"html"
	"regexp"
	"strings"
	"testing"
)

var calcNumberOutput = regexp.MustCompile(`^-?[0-9,]+(\.[0-9]+)?%?$`)

// FuzzCalcExpr feeds arbitrary expressions and attributes to [calc]. Whatever
// the input, it must not panic, and it renders one of three things: the error
// marker, the escaped default, or a plain number.
func FuzzCalcExpr(f *testing.F) {
	seeds := []string{
		"2 + 3 * 4", "(2 + 3) * 4", "-meta.a", "--2", "7 % 3", "-7 % 3", "10 - 4 - 3",
		"meta.qty * meta.unit_price * (1 - coalesce(meta.discount, 0))",
		"min(3, 1, 2)", "max(1)", "abs(-2)", "round(2.675, 2)", "round(1, 11)", "coalesce(meta.x, 0)",
		"meta.a / 0", "meta.big * 10", "item.qty * item.price", "property.ID * 2",
		strings.Repeat("(", 40) + "1" + strings.Repeat(")", 40),
		strings.Repeat("-", 40) + "1",
		strings.Repeat("9", 40), strings.Repeat("1.", 30), "meta.", "meta.unit-price", "()", "1 +", "qty",
		"<script>", "1e5", ".5", "round(1, 1.5)",
	}
	for _, s := range seeds {
		f.Add(s, "", "", "-")
		f.Add(s, "2", "grouped", "<b>x</b>")
		f.Add(s, "0", "percent", "")
	}
	meta := json.RawMessage(`{"a":5,"qty":3,"unit_price":12.5,"discount":0.1,"big":1e308,"x":"abc","lines":[{"qty":1,"price":2}]}`)
	f.Fuzz(func(t *testing.T, expr, decimals, format, def string) {
		ctx := MetaShortcodeContext{Meta: meta, Entity: struct{ ID uint }{ID: 3}}
		attrs := map[string]string{"expr": expr, "default": def}
		if decimals != "" {
			attrs["decimals"] = decimals
		}
		if format != "" {
			attrs["format"] = format
		}
		out := RenderCalcShortcode(Shortcode{Name: "calc", Attrs: attrs}, ctx)
		switch {
		case out == "":
		case isCalcMarker(out):
		case out == html.EscapeString(def):
		case calcNumberOutput.MatchString(out):
		default:
			t.Fatalf("expr %q decimals %q format %q default %q rendered %q", expr, decimals, format, def, out)
		}
		// Inside [each] it takes the splice path, with the same contract.
		elem := renderCalc(ctx, Shortcode{Name: "calc", Attrs: attrs}, map[string]any{"qty": 2.0, "price": 1.5}, true)
		if elem != "" && !isCalcMarker(elem) && elem != html.EscapeString(def) && !calcNumberOutput.MatchString(elem) {
			t.Fatalf("in [each], expr %q rendered %q", expr, elem)
		}
	})
}
