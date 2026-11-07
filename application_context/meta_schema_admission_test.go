package application_context

import (
	"context"
	"errors"
	"fmt"
	"mahresources/shortcodes"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func schemaWithMinimums(n int, literal string) string {
	var b strings.Builder
	b.WriteString(`{"type":"object","properties":{`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		fmt.Fprintf(&b, `"p%d":{"type":"number","minimum":%s}`, i, literal)
	}
	b.WriteString(`}}`)
	return b.String()
}

// jsonschema keeps every numeric constraint as an exact big.Rat, so 500 minimums
// of 1e-1000000 retained about 200 MB while json.Unmarshal let the value through
// as zero. The numbers are refused before compilation.
func TestValidateMetaSchemaRefusesHugeExponents(t *testing.T) {
	schema := schemaWithMinimums(500, "1e-1000000")
	if len(schema) > 40_000 {
		t.Fatalf("fixture is %d bytes; want a small schema", len(schema))
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	err := ValidateMetaSchema(schema)
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Fatal("schema with 1e-1000000 minimums was accepted")
	}
	if !strings.Contains(err.Error(), "number") {
		t.Errorf("error should name the number problem: %v", err)
	}
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 32<<20 {
		t.Errorf("refusal allocated %d bytes; the check must run before compilation", grown)
	}
}

func TestValidateMetaSchemaNumberBoundaries(t *testing.T) {
	long := func(n int) string { return "1." + strings.Repeat("1", n-1) }
	cases := []struct {
		name, literal string
		ok            bool
	}{
		{"plain", "5", true},
		{"negative fraction", "-12.5", true},
		{"exponent 400", "1e400", true},
		{"exponent negative 400", "1e-400", true},
		{"exponent 401", "1e401", false},
		{"exponent negative 401", "1E-401", false},
		{"explicit plus sign", "2e+401", false},
		{"exponent too long to parse", "1e99999999999999999999", false},
		{"100 digits", long(100), true},
		{"101 digits", long(101), false},
		{"leading zeros are not significant", "0." + strings.Repeat("0", 200) + "5", true},
	}
	for _, c := range cases {
		err := ValidateMetaSchema(schemaWithMinimums(1, c.literal))
		if c.ok && err != nil {
			t.Errorf("%s: refused: %v", c.name, err)
		}
		if !c.ok && err == nil {
			t.Errorf("%s: accepted", c.name)
		}
	}
}

func TestValidateMetaSchemaTotalDigits(t *testing.T) {
	// 99 digits each: 101 numbers are 9,999 digits, 102 are 10,098.
	lit := "1." + strings.Repeat("1", 98)
	if err := ValidateMetaSchema(schemaWithMinimums(101, lit)); err != nil {
		t.Errorf("9,999 digits refused: %v", err)
	}
	if err := ValidateMetaSchema(schemaWithMinimums(102, lit)); err == nil {
		t.Error("10,098 digits accepted")
	}
}

func TestValidateMetaSchemaParallelRefusalsStayBounded(t *testing.T) {
	schema := schemaWithMinimums(500, "1e-1000000")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ValidateMetaSchema(schema) == nil {
				t.Error("accepted")
			}
		}()
	}
	wg.Wait()
	runtime.ReadMemStats(&after)
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 64<<20 {
		t.Errorf("six parallel refusals allocated %d bytes", grown)
	}
}

func TestValidateMetaSchemaStructureBounds(t *testing.T) {
	deep := strings.Repeat(`{"properties":{"a":`, 200) + `{}` + strings.Repeat(`}}`, 200)
	if err := ValidateMetaSchema(deep); err == nil {
		t.Error("200-deep schema accepted")
	}
	// A pattern past the regular expression allowance is refused before compile.
	pat := `{"type":"string","pattern":"` + strings.Repeat("a{1000}", 100) + `"}`
	if err := ValidateMetaSchema(pat); err == nil {
		t.Error("oversized pattern accepted")
	}
	// Ordinary schemas still validate.
	ok := `{"type":"object","properties":{"n":{"type":"number","minimum":0,"maximum":1e9},"s":{"type":"string","pattern":"^[a-z]+$"}}}`
	if err := ValidateMetaSchema(ok); err != nil {
		t.Errorf("ordinary schema refused: %v", err)
	}
}

// Review r1: a member named "pattern" inside const/enum/default/examples is
// data, not a regular expression.
func TestValidateMetaSchemaPatternDataIsNotAPattern(t *testing.T) {
	long := strings.Repeat("a", 3000)
	for _, s := range []string{
		`{"const":{"pattern":"` + long + `{1000}{1000}"}}`,
		`{"enum":[{"pattern":"` + strings.Repeat("a{1000}", 100) + `"}]}`,
		`{"type":"object","default":{"pattern":"` + strings.Repeat("a{1000}", 100) + `"}}`,
		`{"examples":[{"pattern":"` + strings.Repeat("a{1000}", 100) + `"}]}`,
	} {
		if err := ValidateMetaSchema(s); err != nil {
			t.Errorf("data refused: %v\n%.80s", err, s)
		}
	}
	// The same text at a schema position is refused, including through $defs
	// and a local $ref target.
	bad := strings.Repeat("a{1000}", 100)
	for _, s := range []string{
		`{"properties":{"a":{"pattern":"` + bad + `"}}}`,
		`{"$defs":{"x":{"pattern":"` + bad + `"}}}`,
		`{"patternProperties":{"` + bad + `":{}}}`,
		`{"allOf":[{"items":{"pattern":"` + bad + `"}}]}`,
	} {
		if err := ValidateMetaSchema(s); err == nil {
			t.Errorf("accepted: %.80s", s)
		}
	}
}

// Many individually admitted patterns add up.
func TestValidateMetaSchemaAggregateRegexWork(t *testing.T) {
	p := strings.Repeat("a{1000}", 19) // ~19,000 instructions each: under the single-pattern allowance
	var props []string
	for i := 0; i < 20; i++ {
		props = append(props, fmt.Sprintf(`"p%d":{"type":"string","pattern":"%s%d"}`, i, p, i)) // distinct sources: each counts once
	}
	s := `{"type":"object","properties":{` + strings.Join(props, ",") + `}}`
	err := ValidateMetaSchema(s)
	if err == nil || !strings.Contains(err.Error(), "add up") {
		t.Errorf("aggregate not refused: %v", err)
	}
}

func TestValidateMetaSchemaContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ValidateMetaSchemaContext(ctx, `{"type":"object"}`); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v", err)
	}
}

// Round 2: the compiler's own regexp engine is metered, so these all see the
// patterns the compiler really compiles.
func TestValidateMetaSchemaRegexEngineSeesWhatTheCompilerCompiles(t *testing.T) {
	bad := strings.Repeat("a{1000}", 100)
	refs := strings.Repeat(`{"$ref":"#"},`, 10000)
	cases := map[string]string{
		"number beyond float64 beside oversized regex": `{"minimum":1e400,"pattern":"` + bad + `"}`,
		"encoded pointer into default":                 `{"$ref":"#/default/%78","default":{"x":{"pattern":"` + bad + `"}}}`,
		"self-document absolute ref":                   `{"$id":"https://example.test/s","$ref":"https://example.test/s#/default/x","default":{"x":{"pattern":"` + bad + `"}}}`,
		"nested resource":                              `{"$defs":{"n":{"$id":"https://example.test/n","properties":{"a":{"pattern":"` + bad + `"}}}}}`,
		"dynamic ref":                                  `{"$schema":"https://json-schema.org/draft/2020-12/schema","$dynamicRef":"#anchor","$defs":{"a":{"$dynamicAnchor":"anchor","pattern":"` + bad + `"}}}`,
		"10,000 refs then an oversized target":         `{"allOf":[` + refs + `{"$ref":"#/default"}],"default":{"pattern":"` + bad + `"}}`,
	}
	for name, s := range cases {
		if err := ValidateMetaSchema(s); err == nil {
			t.Errorf("%s: accepted", name)
		} else if !strings.Contains(err.Error(), "complex") && !strings.Contains(err.Error(), "add up") {
			t.Errorf("%s: refused for another reason: %v", name, err)
		}
	}
}

func TestValidateMetaSchemaInactiveDialectKeywordsStaySavable(t *testing.T) {
	bad := strings.Repeat("a{1000}", 100)
	s := `{"$schema":"http://json-schema.org/draft-07/schema#","prefixItems":[{"pattern":"` + bad + `"}]}`
	if err := ValidateMetaSchema(s); err != nil {
		t.Errorf("unknown keyword in draft-07 refused: %v", err)
	}
}

func TestValidateMetaSchemaInvalidJSONFailsClosed(t *testing.T) {
	if err := ValidateMetaSchema(`{"type":`); err == nil || !strings.Contains(err.Error(), "not valid JSON") {
		t.Errorf("err = %v", err)
	}
}

// Round 3: ten distinct patterns of 19,000 \pL each, under $defs nothing
// references, retained about a gigabyte of rune tables while staying under the
// instruction count.
func TestValidateMetaSchemaUnicodeClassPayloadIsBounded(t *testing.T) {
	var defs []string
	for i := 0; i < 10; i++ {
		const classes = 10000
		p := strings.Repeat(`\\pL`, classes) + fmt.Sprint(i) // JSON text; decodes to 3 bytes per class
		if decoded := 3*classes + len(fmt.Sprint(i)); decoded >= shortcodes.MaxRegexpSource {
			t.Fatalf("prerequisite: %d decoded bytes is not under the %d byte source gate", decoded, shortcodes.MaxRegexpSource)
		}
		defs = append(defs, fmt.Sprintf(`"d%d":{"type":"string","pattern":"%s"}`, i, p))
	}
	s := `{"type":"object","$defs":{` + strings.Join(defs, ",") + `}}`
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	err := ValidateMetaSchema(s)
	runtime.ReadMemStats(&after)
	if err == nil {
		t.Fatal("accepted")
	}
	if grown := after.TotalAlloc - before.TotalAlloc; grown > 128<<20 {
		t.Errorf("allocated %d bytes", grown)
	}
}
