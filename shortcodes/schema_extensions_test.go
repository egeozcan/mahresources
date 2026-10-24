package shortcodes

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"mahresources/mrql"
)

func lintSchema(t *testing.T, schema string, opts ...MetaSchemaLintOptions) []LintIssue {
	t.Helper()
	var o MetaSchemaLintOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	return LintMetaSchema(schema, o)
}

func TestLintMetaSchema(t *testing.T) {
	plugin := MetaSchemaLintOptions{DisplayTypes: []string{"plugin:gallery:image-grid"}}
	cases := []struct {
		name     string
		schema   string
		opts     MetaSchemaLintOptions
		severity string // "" means no issue
		contains string
	}{
		{"clean", `{"type":"object","properties":{"a":{"type":"string","title":"A"}}}`, plugin, "", ""},
		{"non-string x-display", `{"type":"object","properties":{"a":{"type":"object","x-display":5}}}`, plugin, SeverityWarning, `"x-display" must be a string`},
		{"null x-display", `{"type":"object","properties":{"a":{"type":"object","x-display":null}}}`, plugin, SeverityWarning, `"x-display" must be a string`},
		{"builtin on string", `{"type":"object","properties":{"a":{"type":"string","x-display":"geo"}}}`, plugin, SeverityWarning, `x-display "geo" is for an object property`},
		{"builtin on type list with object", `{"type":"object","properties":{"a":{"type":["object","null"],"x-display":"dimensions","properties":{"width":{"type":"number"},"height":{"type":"number"}},"required":["width","height"]}}}`, plugin, "", ""},
		{"geo missing keys", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{"type":"number"}}}}}`, plugin, SeverityWarning, `latitude and longitude`},
		{"geo lat/lng", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{"type":"number"},"lng":{"type":"number"}},"required":["lat","lng"]}}}`, plugin, "", ""},
		{"url needs host", `{"type":"object","properties":{"a":{"type":"object","x-display":"url","properties":{"href":{"type":"string"}}}}}`, plugin, SeverityWarning, `href and host`},
		{"url hostname", `{"type":"object","properties":{"a":{"type":"object","x-display":"url","properties":{"href":{"type":"string"},"hostname":{"type":"string"}},"required":["href","hostname"]}}}`, plugin, "", ""},
		{"daterange missing end", `{"type":"object","properties":{"a":{"type":"object","x-display":"daterange","properties":{"start":{"type":"string"}}}}}`, plugin, SeverityWarning, `start and end`},
		{"geo not required", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"latitude":{"type":"number"},"longitude":{"type":"number"}},"required":["latitude"]}}}`, plugin, SeverityWarning, `required`},
		{"renderer without declared properties", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo"}}}`, plugin, "", ""},
		{"raw opt-out", `{"type":"object","properties":{"a":{"type":"object","x-display":"raw"}}}`, plugin, "", ""},
		{"none opt-out", `{"type":"object","properties":{"a":{"type":"string","x-display":"none"}}}`, plugin, "", ""},
		{"unknown x-display", `{"type":"object","properties":{"a":{"type":"object","x-display":"map"}}}`, plugin, SeverityWarning, `suppresses shape detection`},
		{"malformed plugin x-display", `{"type":"object","properties":{"a":{"type":"object","x-display":"plugin:gallery"}}}`, plugin, SeverityWarning, `plugin:<plugin>:<type>`},
		{"known plugin x-display", `{"type":"object","properties":{"a":{"type":"object","x-display":"plugin:gallery:image-grid"}}}`, plugin, "", ""},
		{"unlisted plugin x-display", `{"type":"object","properties":{"a":{"type":"object","x-display":"plugin:gallery:other"}}}`, plugin, SeverityWarning, `no enabled plugin`},
		{"unlisted plugin x-display with truncated list", `{"type":"object","properties":{"a":{"type":"object","x-display":"plugin:gallery:other"}}}`, MetaSchemaLintOptions{DisplayTypesTruncated: true}, "", ""},
		{"labeled enum with type key", `{"type":"object","properties":{"s":{"oneOf":[{"const":"a","title":"A","type":"string"},{"const":"b","title":"B"}]}}}`, plugin, SeverityWarning, `labeled enum`},
		{"labeled enum clean", `{"type":"object","properties":{"s":{"oneOf":[{"const":"a","title":"A","x-color":"#fff"},{"const":"b","title":"B"}]}}}`, plugin, "", ""},
		{"oneOf of object variants is not a labeled enum", `{"type":"object","properties":{"s":{"oneOf":[{"type":"string"},{"type":"number"}]}}}`, plugin, "", ""},
		{"dotted key", `{"type":"object","properties":{"a.b":{"type":"string"}}}`, plugin, SeverityWarning, `contains a dot`},
		{"hyphen key", `{"type":"object","properties":{"due-date":{"type":"string"}}}`, plugin, SeverityWarning, `letters, digits and underscores`},
		{"external ref", `{"type":"object","properties":{"a":{"$ref":"https://example.com/s.json"}}}`, plugin, SeverityWarning, `local`},
		{"local ref", `{"type":"object","properties":{"a":{"$ref":"#/$defs/x"}},"$defs":{"x":{"type":"string"}}}`, plugin, "", ""},
		{"nested in items and defs", `{"type":"object","properties":{"l":{"type":"array","items":{"type":"object","properties":{"p":{"type":"string","x-display":"geo"}}}}}}`, plugin, SeverityWarning, `x-display "geo" is for an object property`},
		{"nested in defs", `{"type":"object","$defs":{"x":{"type":"object","properties":{"a b":{"type":"string"}}}}}`, plugin, SeverityWarning, `letters, digits and underscores`},
		{"nested in allOf", `{"type":"object","allOf":[{"properties":{"a-b":{"type":"string"}}}]}`, plugin, SeverityWarning, `letters, digits and underscores`},
		{"geo coordinates typed string", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{"type":"string"},"lng":{"type":"string"}},"required":["lat","lng"]}}}`, plugin, SeverityWarning, `typed number`},
		{"dimensions mixed lat typed", `{"type":"object","properties":{"a":{"type":"object","x-display":"dimensions","properties":{"width":{"type":["string","null"]},"height":{"type":"number"}},"required":["width","height"]}}}`, plugin, SeverityWarning, `typed number`},
		{"geo cross-paired keys", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"latitude":{"type":"number"},"lng":{"type":"number"}}}}}`, plugin, SeverityWarning, `latitude and longitude`},
		{"url hrefs typed number", `{"type":"object","properties":{"a":{"type":"object","x-display":"url","properties":{"href":{"type":"integer"},"host":{"type":"string"}},"required":["href","host"]}}}`, plugin, SeverityWarning, `typed string`},
		{"untyped fields fit", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{},"lng":{"description":"east"}},"required":["lat","lng"]}}}`, plugin, "", ""},
		{"anyOf is ambiguous", `{"type":"object","properties":{"a":{"x-display":"geo","anyOf":[{"properties":{"lat":{"type":"number"}}},{"type":"string"}]}}}`, plugin, "", ""},
		{"external ref is not guessed", `{"type":"object","properties":{"a":{"x-display":"geo","$ref":"https://example.com/g.json"}}}`, plugin, SeverityWarning, `not local`},
		{"unresolvable ref is not guessed", `{"type":"object","properties":{"a":{"x-display":"geo","$ref":"#/$defs/missing","properties":{"zzz":{}}}}}`, plugin, "", ""},
		{"x-display beside ref to a non-object", `{"type":"object","$defs":{"s":{"type":"string"}},"properties":{"a":{"x-display":"geo","$ref":"#/$defs/s"}}}`, plugin, SeverityWarning, `is for an object property`},
		{"not json", `{`, plugin, "", ""},
		{"empty schema", ``, plugin, "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			issues := lintSchema(t, c.schema, c.opts)
			if c.severity == "" {
				if len(issues) != 0 {
					t.Fatalf("want no issues, got %+v", issues)
				}
				return
			}
			for _, issue := range issues {
				if issue.Severity == c.severity && strings.Contains(issue.Message, c.contains) {
					return
				}
			}
			t.Fatalf("want a %s containing %q, got %+v", c.severity, c.contains, issues)
		})
	}
}

// A shape split across allOf entries or supplied by a local $ref is one effective
// schema in the editors, so the lint must not report its keys as missing. Each
// case is written for draft-04 (id, definitions), draft-07 (definitions) and
// 2020-12 ($defs).
func TestLintMetaSchemaResolvesComposedShapes(t *testing.T) {
	drafts := []struct{ name, schemaURI, defs string }{
		{"draft-04", "http://json-schema.org/draft-04/schema#", "definitions"},
		{"draft-07", "http://json-schema.org/draft-07/schema#", "definitions"},
		{"2020-12", "https://json-schema.org/draft/2020-12/schema", "$defs"},
	}
	// %[1]s is the schema URI, %[2]s the definitions keyword.
	cases := []struct{ name, schema string }{
		{"lat local, lng through allOf", `{"$schema":"%[1]s","type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{"type":"number"}},"allOf":[{"properties":{"lng":{"type":"number"}}}],"required":["lat","lng"]}}}`},
		{"coordinates through $ref", `{"$schema":"%[1]s","type":"object","%[2]s":{"coords":{"type":"object","properties":{"latitude":{"type":"number"},"longitude":{"type":"number"}},"required":["latitude","longitude"]}},"properties":{"a":{"$ref":"#/%[2]s/coords","x-display":"geo"}}}`},
		{"coordinates through allOf of $ref", `{"$schema":"%[1]s","type":"object","%[2]s":{"coords":{"properties":{"width":{"type":"integer"},"height":{"type":"integer"}},"required":["width","height"]}},"properties":{"a":{"type":"object","x-display":"dimensions","allOf":[{"$ref":"#/%[2]s/coords"}]}}}`},
		{"x-display on the referenced schema", `{"$schema":"%[1]s","type":"object","%[2]s":{"g":{"type":"object","x-display":"geo","properties":{"lat":{"type":"number"},"lng":{"type":"number"}},"required":["lat","lng"]}},"properties":{"a":{"$ref":"#/%[2]s/g"}}}`},
		{"required through allOf", `{"$schema":"%[1]s","type":"object","properties":{"a":{"type":"object","x-display":"daterange","properties":{"start":{"type":"string"},"end":{"type":"string"}},"allOf":[{"required":["start","end"]}]}}}`},
		{"type array with object", `{"$schema":"%[1]s","type":"object","properties":{"a":{"type":["object","null"],"x-display":"url","properties":{"href":{"type":"string"},"hostname":{"type":"string"}},"required":["href","hostname"]}}}`},
	}
	for _, d := range drafts {
		for _, c := range cases {
			t.Run(d.name+"/"+c.name, func(t *testing.T) {
				schema := fmt.Sprintf(c.schema, d.schemaURI, d.defs)
				if issues := LintMetaSchema(schema, MetaSchemaLintOptions{}); len(issues) != 0 {
					t.Fatalf("want no issues, got %+v", issues)
				}
			})
		}
	}
}

// A shape or type check warns only when the outcome is certain. When the
// annotated node is a branch of a composition, a referenced definition, or sits
// under a node whose composition can add to it, the effective schema depends on
// context the lint does not model, so it stays silent.
func TestLintMetaSchemaSkipsWhatItCannotDecide(t *testing.T) {
	geo := `"x-display":"geo","properties":{"lat":{"type":"number"}}`
	cases := map[string]string{
		"annotated allOf branch":    `{"type":"object","properties":{"a":{"allOf":[{` + geo + `},{"properties":{"lng":{"type":"number"}},"required":["lat","lng"]}]}}}`,
		"annotated anyOf branch":    `{"type":"object","properties":{"a":{"anyOf":[{` + geo + `},{"type":"string"}]}}}`,
		"annotated oneOf branch":    `{"type":"object","properties":{"a":{"oneOf":[{` + geo + `},{"type":"string"}]}}}`,
		"referenced definition":     `{"type":"object","$defs":{"g":{` + geo + `}},"properties":{"a":{"$ref":"#/$defs/g","properties":{"lng":{"type":"number"}}}}}`,
		"ancestor with allOf":       `{"type":"object","allOf":[{"properties":{"a":{"properties":{"lng":{"type":"number"}}}}}],"properties":{"a":{` + geo + `}}}`,
		"ancestor with $ref":        `{"type":"object","$defs":{"base":{"properties":{"a":{"properties":{"lng":{"type":"number"}}}}}},"$ref":"#/$defs/base","properties":{"a":{` + geo + `}}}`,
		"ancestor property in then": `{"type":"object","if":{},"then":{"properties":{"a":{` + geo + `}}}}`,
	}
	for name, schema := range cases {
		t.Run(name, func(t *testing.T) {
			if issues := LintMetaSchema(schema, MetaSchemaLintOptions{}); len(issues) != 0 {
				t.Fatalf("want no issues, got %+v", issues)
			}
		})
	}
}

// Keywords beside $ref are ignored up to draft-07 and apply from 2019-09, so
// the field's effective type depends on $schema; with none, the lint is silent.
func TestLintMetaSchemaReadsRefSiblingsByDraft(t *testing.T) {
	build := func(schemaURI, defs string) string {
		head := ""
		if schemaURI != "" {
			head = `"$schema":"` + schemaURI + `",`
		}
		return `{` + head + `"type":"object","` + defs + `":{"num":{"type":"number"}},"properties":{"a":{"type":"object","x-display":"dimensions","properties":{"width":{"$ref":"#/` + defs + `/num","type":"string"},"height":{"type":"number"}},"required":["width","height"]}}}`
	}
	for _, uri := range []string{"http://json-schema.org/draft-04/schema#", "http://json-schema.org/draft-06/schema#", "http://json-schema.org/draft-07/schema#"} {
		if issues := LintMetaSchema(build(uri, "definitions"), MetaSchemaLintOptions{}); len(issues) != 0 {
			t.Errorf("%s: the sibling type is ignored, want no issues, got %+v", uri, issues)
		}
	}
	for _, uri := range []string{"https://json-schema.org/draft/2019-09/schema", "https://json-schema.org/draft/2020-12/schema"} {
		issues := LintMetaSchema(build(uri, "$defs"), MetaSchemaLintOptions{})
		if len(issues) != 1 || !strings.Contains(issues[0].Message, "typed number") {
			t.Errorf("%s: the sibling type applies and no value fits, got %+v", uri, issues)
		}
	}
	for _, uri := range []string{"", "https://example.com/my-dialect"} {
		if issues := LintMetaSchema(build(uri, "$defs"), MetaSchemaLintOptions{}); len(issues) != 0 {
			t.Errorf("%q: unknown semantics, want no issues, got %+v", uri, issues)
		}
	}
}

// Only keywords the resolver models may decide a shape or type warning. Any
// other keyword on the node or in its $ref/allOf chain can change what is
// valid or rendered, so the check stays silent.
func TestLintMetaSchemaAllowlistsModelledKeywords(t *testing.T) {
	node := func(extra string) string {
		return `{"type":"object","$defs":{"d":{` + extra + `}},"properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{"type":"number"}},` + extra + `}}}`
	}
	skip := map[string]string{
		"dependentSchemas":      `"dependentSchemas":{"lat":{"properties":{"lng":{"type":"number"}},"required":["lng"]}}`,
		"dependencies":          `"dependencies":{"lat":{"properties":{"lng":{"type":"number"}}}}`,
		"dependentRequired":     `"dependentRequired":{"lat":["lng"]}`,
		"if/then":               `"if":{"required":["lat"]},"then":{"properties":{"lng":{"type":"number"}}}`,
		"not":                   `"not":{"required":["zzz"]}`,
		"anyOf":                 `"anyOf":[{"properties":{"lng":{"type":"number"}}}]`,
		"oneOf":                 `"oneOf":[{"properties":{"lng":{"type":"number"}}}]`,
		"patternProperties":     `"patternProperties":{"^l":{"type":"number"}}`,
		"additionalProperties":  `"additionalProperties":{"type":"number"}`,
		"unevaluatedProperties": `"unevaluatedProperties":{"type":"number"}`,
		"propertyNames":         `"propertyNames":{"maxLength":3}`,
	}
	for name, extra := range skip {
		t.Run("skip "+name, func(t *testing.T) {
			if issues := LintMetaSchema(node(extra), MetaSchemaLintOptions{}); len(issues) != 0 {
				t.Fatalf("want no issues, got %+v", issues)
			}
		})
	}
	t.Run("skip keyword in the $ref chain", func(t *testing.T) {
		schema := `{"type":"object","$defs":{"d":{"dependentSchemas":{"lat":{"properties":{"lng":{"type":"number"}}}}}},"properties":{"a":{"$ref":"#/$defs/d","x-display":"geo","properties":{"lat":{"type":"number"}}}}}`
		if issues := LintMetaSchema(schema, MetaSchemaLintOptions{}); len(issues) != 0 {
			t.Fatalf("want no issues, got %+v", issues)
		}
	})
	t.Run("skip keyword in an allOf entry", func(t *testing.T) {
		schema := `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{"type":"number"}},"allOf":[{"dependentRequired":{"lat":["lng"]}}]}}}`
		if issues := LintMetaSchema(schema, MetaSchemaLintOptions{}); len(issues) != 0 {
			t.Fatalf("want no issues, got %+v", issues)
		}
	})
	t.Run("modelled keywords and annotations still warn", func(t *testing.T) {
		extra := `"title":"Place","description":"d","default":{},"examples":[{}],"x-note":1,"$comment":"c","additionalProperties":false,"readOnly":true,"deprecated":false`
		issues := LintMetaSchema(node(extra), MetaSchemaLintOptions{})
		if len(issues) != 1 || !strings.Contains(issues[0].Message, "latitude and longitude") {
			t.Fatalf("want the missing-keys warning, got %+v", issues)
		}
	})
}

// Key-count bounds cannot change which keys or types are declared, so they do
// not hide a certain warning.
func TestLintMetaSchemaKeyCountBoundsDoNotHideWarnings(t *testing.T) {
	for _, bounds := range []string{`"minProperties":2`, `"maxProperties":2`, `"minProperties":1,"maxProperties":4`} {
		schema := `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{"type":"string"},"lng":{"type":"string"}},"required":["lat","lng"],"additionalProperties":false,` + bounds + `}}}`
		issues := LintMetaSchema(schema, MetaSchemaLintOptions{})
		if len(issues) != 1 || !strings.Contains(issues[0].Message, "typed number") {
			t.Errorf("%s: want the type warning, got %+v", bounds, issues)
		}
	}
}

// A closed object with minProperties at or above its declared count can only be
// valid with every declared key present, so each is effectively required.
func TestLintMetaSchemaClosedMinPropertiesImpliesRequired(t *testing.T) {
	node := func(extra string) string {
		return `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{"type":"number"},"lng":{"type":"number"}},` + extra + `}}}`
	}
	for _, extra := range []string{
		`"additionalProperties":false,"minProperties":2`,
		`"additionalProperties":false,"minProperties":3`,
	} {
		if issues := LintMetaSchema(node(extra), MetaSchemaLintOptions{}); len(issues) != 0 {
			t.Errorf("%s: every key is effectively required, want no issues, got %+v", extra, issues)
		}
	}
	for _, extra := range []string{
		`"additionalProperties":false,"minProperties":1`,
		`"minProperties":2`,
		`"additionalProperties":true,"minProperties":2`,
		`"title":"Place"`,
	} {
		issues := LintMetaSchema(node(extra), MetaSchemaLintOptions{})
		if len(issues) != 1 || !strings.Contains(issues[0].Message, "required") {
			t.Errorf("%q: want the required warning, got %+v", extra, issues)
		}
	}
}

// Dependency and conditional keywords on an ancestor can reach into a property
// and add keys to it, so nothing below such an ancestor is decided.
func TestLintMetaSchemaSkipsBelowAncestorsThatCanAddKeys(t *testing.T) {
	geo := `"p":{"type":"object","x-display":"geo","properties":{"lat":{"type":"number"}}}`
	for name, ancestor := range map[string]string{
		"dependentSchemas":      `"dependentSchemas":{"mode":{"properties":{"p":{"properties":{"lng":{"type":"number"}}}}}}`,
		"dependencies":          `"dependencies":{"mode":{"properties":{"p":{"properties":{"lng":{"type":"number"}}}}}}`,
		"if/then/else":          `"if":{"required":["mode"]},"then":{"properties":{"p":{"properties":{"lng":{"type":"number"}}}}}`,
		"patternProperties":     `"patternProperties":{"^p$":{"properties":{"lng":{"type":"number"}}}}`,
		"additionalProperties":  `"additionalProperties":{"properties":{"lng":{"type":"number"}}}`,
		"unevaluatedProperties": `"unevaluatedProperties":{"properties":{"lng":{"type":"number"}}}`,
	} {
		schema := `{"type":"object","properties":{"mode":{"type":"string"},` + geo + `},` + ancestor + `}`
		if issues := LintMetaSchema(schema, MetaSchemaLintOptions{}); len(issues) != 0 {
			t.Errorf("%s: want no issues, got %+v", name, issues)
		}
	}
	plain := `{"type":"object","required":["p"],"additionalProperties":false,"properties":{` + geo + `}}`
	if issues := LintMetaSchema(plain, MetaSchemaLintOptions{}); len(issues) != 1 {
		t.Errorf("a plain ancestor must not hide the warning, got %+v", issues)
	}
}

// A property name is addressed as meta.<name> in MRQL, so the lint follows the
// parser: a name warns exactly when meta.<name> does not parse as that field.
func TestLintMetaSchemaPropertyNamesFollowTheMRQLParser(t *testing.T) {
	addressable := func(key string) bool {
		q, err := mrql.Parse("meta." + key + " = 1")
		if err != nil {
			return false
		}
		cmp, ok := q.Where.(*mrql.ComparisonExpr)
		return ok && len(cmp.Field.Parts) == 2 && cmp.Field.Parts[1].Value == key
	}
	known := map[string]bool{ // key -> addressable
		"rating": true, "_x": true, "Type": true, "type": true, "having": true, "within": true, "count": true, "rating2": true,
		"1st": false, "9": false, "limit": false, "and": false, "NOT": false, "null": false, "order": true,
		"due-date": false, "a b": false, "na\u00efve": false,
	}
	for key, want := range known {
		if got := addressable(key); got != want {
			t.Errorf("fixture: %q addressable=%t in the parser, test expects %t", key, got, want)
		}
		doc, _ := json.Marshal(map[string]any{"type": "object", "properties": map[string]any{key: map[string]any{"type": "string"}}})
		warned := len(LintMetaSchema(string(doc), MetaSchemaLintOptions{})) > 0
		if warned == addressable(key) {
			t.Errorf("%q: warned=%t but addressable=%t", key, warned, addressable(key))
		}
	}
}

func TestLintMetaSchemaTreatsEmptyXDisplayAsUnset(t *testing.T) {
	for _, schema := range []string{
		`{"type":"object","properties":{"a":{"type":"object","x-display":""}}}`,
		`{"type":"object","properties":{"a":{"type":"string","x-display":"","properties":{"lat":{}}}}}`,
	} {
		if issues := LintMetaSchema(schema, MetaSchemaLintOptions{}); len(issues) != 0 {
			t.Errorf("%s: want no issues, got %+v", schema, issues)
		}
	}
}

func TestLintMetaSchemaNeverReportsAnError(t *testing.T) {
	for _, schema := range []string{
		`{"properties":{"a":{"x-display":5},"b":{"type":"string","x-display":"geo"},"c":{"x-display":"geo","properties":{"x":{}}}}}`,
	} {
		for _, issue := range LintMetaSchema(schema, MetaSchemaLintOptions{}) {
			if issue.Severity != SeverityWarning {
				t.Errorf("%+v: no annotation breaks a page, so every issue is a warning", issue)
			}
		}
	}
}

func TestLintMetaSchemaIsDeterministic(t *testing.T) {
	schema := `{"type":"object","properties":{"b-1":{"type":"string"},"a-1":{"type":"string"},"c.1":{"type":"string"}}}`
	first := LintMetaSchema(schema, MetaSchemaLintOptions{})
	if len(first) != 3 {
		t.Fatalf("want 3 warnings, got %+v", first)
	}
	for i := 0; i < 20; i++ {
		again := LintMetaSchema(schema, MetaSchemaLintOptions{})
		for j := range first {
			if first[j] != again[j] {
				t.Fatalf("run %d differs at %d: %+v vs %+v", i, j, first[j], again[j])
			}
		}
	}
}

func TestSchemaExtensionReference(t *testing.T) {
	ref := SchemaExtensionReference()
	for _, want := range []string{"x-display", "geo", "daterange", "dimensions", "url", "raw", "x-color", "oneOf"} {
		if !strings.Contains(ref, want) {
			t.Errorf("reference lacks %q", want)
		}
	}
	if len(ref) > 2500 {
		t.Errorf("reference is %d bytes, want about 2 KB", len(ref))
	}
	for _, banned := range []string{"throw", "crash"} {
		if strings.Contains(ref, banned) {
			t.Errorf("reference mentions %q; nothing here breaks a page", banned)
		}
	}
}
