package shortcodes

import (
	"context"
	"encoding/json"
	"math"
	"net/url"
	"reflect"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/types"

	"github.com/stretchr/testify/assert"
)

type testEntity struct {
	ID          uint
	Name        string
	Description string
	CreatedAt   time.Time
	Tags        []string
	Meta        json.RawMessage
}

func TestPropertyShortcodeStringField(t *testing.T) {
	entity := testEntity{ID: 1, Name: "My Resource"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "My Resource", result)
}

func TestPropertyShortcodeHTMLEscaped(t *testing.T) {
	entity := testEntity{ID: 1, Name: `<script>alert("xss")</script>`}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "&lt;script&gt;alert(&#34;xss&#34;)&lt;/script&gt;", result)
	assert.NotContains(t, result, "<script>")
}

func TestPropertyShortcodeRawAttribute(t *testing.T) {
	entity := testEntity{ID: 1, Description: "<b>bold</b> text"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Description", "raw": "true"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "<b>bold</b> text", result)
}

func TestPropertyShortcodeUintField(t *testing.T) {
	entity := testEntity{ID: 42, Name: "Test"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 42, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "ID"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "42", result)
}

func TestPropertyShortcodeSliceField(t *testing.T) {
	entity := testEntity{ID: 1, Tags: []string{"photo", "landscape"}}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Tags"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "photo, landscape", result)
}

func TestPropertyShortcodeSliceHTMLEscaped(t *testing.T) {
	entity := testEntity{ID: 1, Tags: []string{"<b>bold</b>", "normal"}}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Tags"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "&lt;b&gt;bold&lt;/b&gt;, normal", result)
}

func TestPropertyShortcodeTimeField(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	entity := testEntity{ID: 1, CreatedAt: ts}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Contains(t, result, "2026")
}

func TestPropertyShortcodeMissingPath(t *testing.T) {
	entity := testEntity{ID: 1, Name: "Test"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "", result)
}

func TestPropertyShortcodeInvalidField(t *testing.T) {
	entity := testEntity{ID: 1, Name: "Test"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "NonExistent"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "", result)
}

func TestPropertyShortcodeNilEntity(t *testing.T) {
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: nil}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "", result)
}

func TestPropertyShortcodePointerEntity(t *testing.T) {
	entity := &testEntity{ID: 1, Name: "Pointer Entity"}
	ctx := MetaShortcodeContext{EntityType: "resource", EntityID: 1, Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Equal(t, "Pointer Entity", result)
}

// --- Phase 2: dot-path traversal, format, default ---

type traversalOwner struct {
	Name string
	ID   uint
}

type traversalTag struct {
	Name string
}

type traversalEntity struct {
	ID        uint
	Name      string
	Owner     *traversalOwner
	Tags      []*traversalTag
	CreatedAt time.Time
	FileSize  int64
	Count     int
	URL       *types.URL
}

func TestPropertyShortcodeDotPath(t *testing.T) {
	entity := traversalEntity{Owner: &traversalOwner{Name: "Alice"}}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Owner.Name"}}
	assert.Equal(t, "Alice", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDotPathNilPointer(t *testing.T) {
	entity := traversalEntity{Owner: nil}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Owner.Name"}}
	assert.Equal(t, "", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDotPathMissingSegment(t *testing.T) {
	entity := traversalEntity{Owner: &traversalOwner{Name: "Alice"}}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Owner.Nope"}}
	assert.Equal(t, "", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeSliceIndex(t *testing.T) {
	entity := traversalEntity{Tags: []*traversalTag{{Name: "photo"}, {Name: "landscape"}}}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Tags.1.Name"}}
	assert.Equal(t, "landscape", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeSliceIndexOutOfRange(t *testing.T) {
	entity := traversalEntity{Tags: []*traversalTag{{Name: "photo"}}}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Tags.5.Name"}}
	assert.Equal(t, "", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDefault(t *testing.T) {
	entity := traversalEntity{Owner: nil}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Owner.Name", "default": "Unassigned"}}
	assert.Equal(t, "Unassigned", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDefaultNotUsedWhenPresent(t *testing.T) {
	entity := traversalEntity{Name: "Real"}
	ctx := MetaShortcodeContext{Entity: entity}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name", "default": "Fallback"}}
	assert.Equal(t, "Real", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDefaultHTMLEscaped(t *testing.T) {
	ctx := MetaShortcodeContext{Entity: traversalEntity{}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name", "default": "<b>x</b>"}}
	assert.Equal(t, "&lt;b&gt;x&lt;/b&gt;", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeDefaultRaw(t *testing.T) {
	ctx := MetaShortcodeContext{Entity: traversalEntity{}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name", "default": "<b>x</b>", "raw": "true"}}
	assert.Equal(t, "<b>x</b>", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeFormatDate(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	ctx := MetaShortcodeContext{Entity: traversalEntity{CreatedAt: ts}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt", "format": "date"}}
	assert.Equal(t, "2026-04-09", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeFormatDateTime(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	ctx := MetaShortcodeContext{Entity: traversalEntity{CreatedAt: ts}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt", "format": "datetime"}}
	assert.Equal(t, "2026-04-09 12:30", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeFormatTime(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	ctx := MetaShortcodeContext{Entity: traversalEntity{CreatedAt: ts}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt", "format": "time"}}
	assert.Equal(t, "12:30", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeLayoutWinsOverFormat(t *testing.T) {
	ts := time.Date(2026, 4, 9, 12, 30, 0, 0, time.UTC)
	ctx := MetaShortcodeContext{Entity: traversalEntity{CreatedAt: ts}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "CreatedAt", "format": "date", "layout": "Jan 2, 2006"}}
	assert.Equal(t, "Apr 9, 2026", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeFormatFilesize(t *testing.T) {
	ctx := MetaShortcodeContext{Entity: traversalEntity{FileSize: 1536}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "FileSize", "format": "filesize"}}
	result := RenderPropertyShortcode(sc, ctx)
	assert.Contains(t, result, "1.5")
	assert.Contains(t, result, "KB")
}

func TestPropertyShortcodeFormatUnknownPassesThrough(t *testing.T) {
	ctx := MetaShortcodeContext{Entity: traversalEntity{Name: "Hello"}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name", "format": "bogus"}}
	assert.Equal(t, "Hello", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeFormatDateOnNonTimePassesThrough(t *testing.T) {
	ctx := MetaShortcodeContext{Entity: traversalEntity{Name: "Hello"}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "Name", "format": "date"}}
	assert.Equal(t, "Hello", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeURLStringer(t *testing.T) {
	parsed, err := url.Parse("https://example.com/profile?tab=social#links")
	assert.NoError(t, err)
	u := types.URL(*parsed)

	ctx := MetaShortcodeContext{Entity: traversalEntity{URL: &u}}
	sc := Shortcode{Name: "property", Attrs: map[string]string{"path": "URL"}}
	assert.Equal(t, "https://example.com/profile?tab=social#links", RenderPropertyShortcode(sc, ctx))
}

func TestPropertyShortcodeProcessRealGroupURL(t *testing.T) {
	parsed, err := url.Parse("https://example.com/profile?tab=social#links")
	assert.NoError(t, err)
	u := types.URL(*parsed)
	group := models.Group{ID: 7, URL: &u}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 7, Entity: group}

	result := Process(
		context.Background(),
		`<a href="[property path="URL"]">[property path="URL"]</a>`,
		ctx,
		nil,
		nil,
	)

	assert.Equal(t, `<a href="https://example.com/profile?tab=social#links">https://example.com/profile?tab=social#links</a>`, result)
}

// float64 cannot represent math.MaxInt64: it rounds up to 2^63. A `>` guard is
// therefore false at exactly 2^63 and int64(f) saturates, reporting a byte count
// one less than it was given rather than declining to format.
func TestAsInt64RejectsOutOfRangeFloats(t *testing.T) {
	cases := []struct {
		name string
		in   float64
		want bool
	}{
		{"2^63 is out of range", math.Pow(2, 63), false},
		{"just under 2^63 is in range", math.Pow(2, 63) - 1024, true},
		{"-2^63 is representable", -math.Pow(2, 63), true},
		{"below -2^63 is out of range", -math.Pow(2, 64), false},
		{"fractions are not counts", 2.5, false},
		{"NaN", math.NaN(), false},
		{"+Inf", math.Inf(1), false},
		{"-Inf", math.Inf(-1), false},
		{"an ordinary count", 2048, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, ok := asInt64(reflect.ValueOf(tc.in)); ok != tc.want {
				t.Errorf("asInt64(%v) ok = %v, want %v", tc.in, ok, tc.want)
			}
		})
	}
}

// [property] reads Go struct fields, where a string is genuinely a string. The
// JSON-aware time parsing that [item] and [meta inline] need must not reach it:
// a group named "2026-08-22" under format="time" rendered its name and would
// otherwise start rendering "00:00".
func TestPropertyDoesNotParseStringsAsTimes(t *testing.T) {
	type ent struct{ Name string }
	e := &ent{Name: "2026-08-22"}
	for _, attrs := range []map[string]string{
		{"path": "Name", "format": "time"},
		{"path": "Name", "format": "date"},
		{"path": "Name", "format": "datetime"},
		{"path": "Name", "layout": "Jan 2, 2006"},
	} {
		got := RenderPropertyShortcode(Shortcode{Name: "property", Attrs: attrs}, MetaShortcodeContext{Entity: e})
		if got != "2026-08-22" {
			t.Errorf("[property %v] = %q, want the field verbatim", attrs, got)
		}
	}
}

// The JSON entry point is the one that parses, and only when a time format was
// actually requested.
func TestFormatJSONScalarParsesOnlyOnRequest(t *testing.T) {
	if got := formatJSONScalar("2026-08-22T10:30:00Z", "date", ""); got != "2026-08-22" {
		t.Errorf(`format="date" = %q, want "2026-08-22"`, got)
	}
	if got := formatJSONScalar("2026-08-22T10:30:00Z", "", ""); got != "2026-08-22T10:30:00Z" {
		t.Errorf("no format asked for = %q, want the string verbatim", got)
	}
	if got := formatJSONScalar("release-2026-08-22", "date", ""); got != "release-2026-08-22" {
		t.Errorf("unparseable string = %q, want it verbatim", got)
	}
	// A zone-less string is read as UTC; there is no zone in the data to do
	// better with, so a zone-bearing layout appends Z.
	if got := formatJSONScalar("2026-08-22 10:30:00", "", "2006-01-02T15:04:05Z07:00"); got != "2026-08-22T10:30:00Z" {
		t.Errorf("zone-less input = %q", got)
	}
}

// An MRQL aggregated row is a GORM map scan, so a timestamp column arrives as a
// string on SQLite and as a time.Time on Postgres. format="date" has to mean the
// same thing on both engines — and the same thing it means on [item] and on
// [meta inline="true"], whose values are JSON-shaped too.
func TestFormatScalarValueFormatsBothEngineShapesAlike(t *testing.T) {
	stamp := time.Date(2026, 8, 22, 10, 30, 0, 0, time.UTC)

	assert.Equal(t, "2026-08-22", formatScalarValue("2026-08-22T10:30:00Z", "date", ""))
	assert.Equal(t, "2026-08-22", formatScalarValue(stamp, "date", ""))
	assert.Equal(t, "10:30", formatScalarValue("2026-08-22T10:30:00Z", "time", ""))
	assert.Equal(t, "10:30", formatScalarValue(stamp, "time", ""))
	assert.Equal(t, "Aug 22, 2026", formatScalarValue("2026-08-22", "", "Jan 2, 2006"))
	assert.Equal(t, "Aug 22, 2026", formatScalarValue(stamp, "", "Jan 2, 2006"))

	// A string that is not a time passes through even when a time format was
	// asked for, and nothing is parsed when no time format was asked for.
	assert.Equal(t, "report", formatScalarValue("report", "date", ""))
	assert.Equal(t, "2026-08-22T10:30:00Z", formatScalarValue("2026-08-22T10:30:00Z", "", ""))

	// The non-time formatting [mrql value=] already had is unchanged.
	assert.Equal(t, "", formatScalarValue(nil, "", ""))
	assert.Equal(t, "42", formatScalarValue(int64(42), "", ""))
	assert.Equal(t, "2.0 KB", formatScalarValue(float64(2048), "filesize", ""))
}

// JSON decodes every number to float64, so "%g" put every integer-looking Meta
// field at or above one million into scientific notation: {"n":1234567} rendered
// "1.234567e+06". It reaches the three shortcodes whose values are JSON-shaped.
func TestJSONNumbersRenderInPlainDecimal(t *testing.T) {
	// [item]
	item := renderItemValue(
		Shortcode{Name: "item", Attrs: map[string]string{"path": "n"}},
		map[string]any{"n": float64(1234567)}, 1)
	assert.Equal(t, "1234567", item)

	// [meta inline="true"]
	inline := RenderMetaShortcode(
		Shortcode{Name: "meta", Attrs: map[string]string{"path": "n", "inline": "true"}},
		MetaShortcodeContext{Meta: json.RawMessage(`{"n":1234567}`)})
	assert.Equal(t, "1234567", inline)

	// [mrql value=…] on an aggregate
	assert.Equal(t, "1234567", formatScalarValue(float64(1234567), "", ""))
	assert.Equal(t, "1234567.5", formatScalarValue(float64(1234567.5), "", ""))

	// Small and negative values keep their plain form too.
	assert.Equal(t, "0", formatScalarValue(float64(0), "", ""))
	assert.Equal(t, "-1234567", formatScalarValue(float64(-1234567), "", ""))
	assert.Equal(t, "2.5", formatScalarValue(2.5, "", ""))

	// Past the point where plain decimal is hundreds of digits, exponent form is
	// the readable one — the same bounds encoding/json switches at.
	assert.Equal(t, "1e+21", formatScalarValue(1e21, "", ""))
	assert.Equal(t, "1e-07", formatScalarValue(1e-7, "", ""))
	assert.Equal(t, "100000000000000000000", formatScalarValue(1e20, "", ""))
	assert.Equal(t, "0.000001", formatScalarValue(1e-6, "", ""))
}
