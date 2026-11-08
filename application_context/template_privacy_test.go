package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"
)

var placeholderShape = regexp.MustCompile(`^⟦[a-z2-7]{2}·(text|url|email) \d+ · (1 line|multi-line) · ~(10|20|50|100|200|500|1000|1000\+) chars⟧$`)

func TestTemplatePrivacyPlaceholderFormat(t *testing.T) {
	p := NewTemplatePrivacy("k7")
	got := p.Text("Quarterly review of the Moonbase project")
	if !placeholderShape.MatchString(got) {
		t.Fatalf("placeholder %q does not match the P3 format", got)
	}
	if got != "⟦k7·text 1 · 1 line · ~50 chars⟧" {
		t.Fatalf("got %q", got)
	}
	if again := p.Text("Quarterly review of the Moonbase project"); again != got {
		t.Fatalf("equal values must share a Placeholder: %q vs %q", again, got)
	}
	second := p.Text("line one\nline two")
	if second != "⟦k7·text 2 · multi-line · ~20 chars⟧" {
		t.Fatalf("got %q", second)
	}
	if p.Text("") != "" {
		t.Fatal("the empty string stays empty")
	}
	if got := p.Text(strings.Repeat("x", 1500)); !strings.Contains(got, "~1000+ chars") {
		t.Fatalf("long value: %q", got)
	}
}

func TestTemplatePrivacyKinds(t *testing.T) {
	p := NewTemplatePrivacy("ab")
	cases := map[string]string{
		"https://example.com/private/path": "url",
		"someone@example.org":              "email",
		"plain words":                      "text",
	}
	for value, kind := range cases {
		if got := p.Text(value); !strings.HasPrefix(got, "⟦ab·"+kind+" ") {
			t.Errorf("%q: got %q, want kind %s", value, got, kind)
		}
	}
	date := p.Text("2024-05-06")
	if !regexp.MustCompile(`^⟦ab·date \d+⟧$`).MatchString(date) {
		t.Fatalf("a date-shaped string is a date Placeholder: %q", date)
	}
	instant := time.Date(2024, 5, 6, 7, 8, 9, 0, time.UTC)
	d := p.Date(instant)
	if !regexp.MustCompile(`^⟦ab·date \d+⟧$`).MatchString(d) || p.Date(instant) != d {
		t.Fatalf("Date: %q", d)
	}
	if p.Date(instant.Add(time.Hour)) == d {
		t.Fatal("different instants must not share a Placeholder")
	}
}

func TestTemplatePrivacyLegendKeepsIssueOrder(t *testing.T) {
	p := NewTemplatePrivacy("zz")
	a := p.Text("alpha value")
	b := p.Text("beta value")
	legend := p.Legend()
	if len(legend) != 2 || legend[0].Placeholder != a || legend[0].Value != "alpha value" || legend[1].Placeholder != b || legend[1].Value != "beta value" {
		t.Fatalf("legend %+v", legend)
	}
}

func TestTemplatePrivacyTextAlwaysTreatsInputAsRaw(t *testing.T) {
	p := NewTemplatePrivacy("qq")
	first := p.Text("secret thing")
	// A database string that equals an issued Placeholder is a value like any other.
	second := p.Text(first)
	if second == first {
		t.Fatalf("a string equal to a Placeholder must get its own Placeholder, got %q", second)
	}
	legend := p.Legend()
	if len(legend) != 2 || legend[1].Value != first {
		t.Fatalf("legend %+v", legend)
	}
}

func TestTemplatePrivacyDeterministicTag(t *testing.T) {
	a := TemplatePrivacyTag("user:1", "group", 7)
	b := TemplatePrivacyTag("user:1", "group", 7)
	if a != b || len(a) != 2 {
		t.Fatalf("tag must be stable and two characters: %q %q", a, b)
	}
}

func TestTemplatePrivacyKeepIsReadable(t *testing.T) {
	p := NewTemplatePrivacy("kk")
	if p.Keep("Book") != "Book" {
		t.Fatal("Keep returns the string unchanged")
	}
	if len(p.Legend()) != 0 {
		t.Fatal("Keep must not issue a Placeholder")
	}
}

func TestTemplatePrivacyMetaMasksStringLeavesAndKeepsKeys(t *testing.T) {
	p := NewTemplatePrivacy("mm")
	raw := []byte(`{"title":"Moonbase plans","count":3,"ok":true,"nested":{"note":"deep secret"},"tags":["red thing","blue thing"],"none":null}`)
	out := p.Meta(raw, "")
	var decoded map[string]any
	if err := json.Unmarshal(out, &decoded); err != nil {
		t.Fatalf("masked meta is not JSON: %v: %s", err, out)
	}
	if strings.Contains(string(out), "Moonbase") || strings.Contains(string(out), "deep secret") || strings.Contains(string(out), "red thing") {
		t.Fatalf("a value leaked: %s", out)
	}
	for _, key := range []string{"title", "count", "ok", "nested", "tags", "none"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("key %q was dropped", key)
		}
	}
	if decoded["count"].(float64) != 3 || decoded["ok"] != true {
		t.Fatalf("numbers and booleans pass through: %v", decoded)
	}
}

func TestTemplatePrivacyMetaKeepsEnumAndConstOfItsOwnField(t *testing.T) {
	p := NewTemplatePrivacy("ee")
	schema := `{
	  "type":"object",
	  "properties":{
	    "status":{"type":"string","enum":["draft","published"]},
	    "kind":{"const":"article"},
	    "other":{"type":"string"},
	    "nested":{"type":"object","properties":{"level":{"enum":["high","low"]}}},
	    "refd":{"$ref":"#/$defs/choice"},
	    "either":{"anyOf":[{"enum":["alpha","beta"]},{"type":"null"}]},
	    "list":{"type":"array","items":{"enum":["one","two"]}}
	  },
	  "$defs":{"choice":{"enum":["yes","no"]}}
	}`
	raw := []byte(`{"status":"draft","kind":"article","other":"draft","nested":{"level":"high"},"refd":"yes","either":"alpha","list":["one","three"],"unknown":"draft"}`)
	var out map[string]any
	if err := json.Unmarshal(p.Meta(raw, schema), &out); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"status": "draft", "kind": "article"} {
		if out[key] != want {
			t.Errorf("%s: want %q kept, got %v", key, want, out[key])
		}
	}
	if out["nested"].(map[string]any)["level"] != "high" || out["refd"] != "yes" || out["either"] != "alpha" {
		t.Errorf("enum through nested, $ref and anyOf must stay readable: %v", out)
	}
	list := out["list"].([]any)
	if list[0] != "one" || list[1] == "three" {
		t.Errorf("array items: %v", list)
	}
	if out["other"] == "draft" || out["unknown"] == "draft" {
		t.Errorf("a value equal to an enum of ANOTHER field, or on an unresolved path, is masked: %v", out)
	}
}

func TestTemplatePrivacyContainsIssued(t *testing.T) {
	p := NewTemplatePrivacy("ci")
	ph := p.Text("a hidden value")
	if got, ok := p.ContainsIssued("before " + ph + " after"); !ok || got != ph {
		t.Fatalf("got %q %v", got, ok)
	}
	if _, ok := p.ContainsIssued("no placeholder here ⟦ci·text 99 · 1 line · ~10 chars⟧"); ok {
		t.Fatal("a Placeholder this run never issued is not an issued Placeholder")
	}
	alias := p.URLAlias("https://example.com/secret")
	if !strings.HasPrefix(alias, "https://placeholder.invalid/ci-url-") {
		t.Fatalf("alias %q", alias)
	}
	if _, ok := p.ContainsIssued(`<a href="` + alias + `">x</a>`); !ok {
		t.Fatal("the URL alias spelling is recognised")
	}
	legend := p.Legend()
	last := legend[len(legend)-1]
	if last.Alias != alias || last.Value != "https://example.com/secret" || !strings.HasPrefix(last.Placeholder, "⟦ci·url ") {
		t.Fatalf("legend lists both spellings: %+v", last)
	}
}

func TestTemplatePrivacyLimitsAreSticky(t *testing.T) {
	p := NewTemplatePrivacy("lm")
	for i := 0; i < templatePrivacyMaxValues; i++ {
		if got := p.Text("value number " + strings.Repeat("x", i%7) + string(rune('A'+i%26)) + time.Duration(i).String()); strings.Contains(got, "withheld") {
			t.Fatalf("value %d withheld too early", i)
		}
	}
	if p.Err() != nil {
		t.Fatalf("limit not reached yet: %v", p.Err())
	}
	got := p.Text("one value too many")
	if got != "⟦lm·withheld⟧" {
		t.Fatalf("past the limit Text returns the fixed withheld token, got %q", got)
	}
	if !errors.Is(p.Err(), ErrTemplatePrivacyLimit) {
		t.Fatalf("Err must be sticky: %v", p.Err())
	}
	if err := p.CheckOutbound(context.Background(), []byte(`{"a":"b"}`)); !errors.Is(err, ErrTemplatePrivacyLeak) {
		t.Fatalf("CheckOutbound refuses outright while Err is set: %v", err)
	}
}

func TestTemplatePrivacyOversizedValueIsAnOverflow(t *testing.T) {
	p := NewTemplatePrivacy("ov")
	got := p.Text(strings.Repeat("a", templatePrivacyMaxValueBytes+1))
	if got != "⟦ov·withheld⟧" || !errors.Is(p.Err(), ErrTemplatePrivacyLimit) {
		t.Fatalf("got %q err %v", got, p.Err())
	}
}

func TestTemplatePrivacyTotalBytesLimit(t *testing.T) {
	p := NewTemplatePrivacy("tb")
	chunk := templatePrivacyMaxValueBytes
	n := templatePrivacyMaxLegendBytes/chunk + 1
	var last string
	for i := 0; i < n; i++ {
		last = p.Text(string(rune('a'+i)) + strings.Repeat("z", chunk-1))
	}
	if last != "⟦tb·withheld⟧" || p.Err() == nil {
		t.Fatalf("the legend byte budget must be enforced, last=%q err=%v", last, p.Err())
	}
}

func TestTemplatePrivacyContextHelpers(t *testing.T) {
	if TemplatePrivacyFrom(context.Background()) != nil {
		t.Fatal("no privacy on a bare context")
	}
	p := NewTemplatePrivacy("cx")
	if TemplatePrivacyFrom(WithTemplatePrivacy(context.Background(), p)) != p {
		t.Fatal("round trip")
	}
	if TemplatePrivacyFrom(WithTemplatePrivacy(context.Background(), nil)) != nil {
		t.Fatal("a nil privacy is off")
	}
}
