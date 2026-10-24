package shortcodes

import (
	"strings"
	"testing"
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
		{"non-string x-display", `{"type":"object","properties":{"a":{"type":"object","x-display":5}}}`, plugin, SeverityError, `"x-display" must be a string`},
		{"null x-display", `{"type":"object","properties":{"a":{"type":"object","x-display":null}}}`, plugin, SeverityError, `"x-display" must be a string`},
		{"builtin on string", `{"type":"object","properties":{"a":{"type":"string","x-display":"geo"}}}`, plugin, SeverityError, `"geo" needs an object`},
		{"builtin on type list with object", `{"type":"object","properties":{"a":{"type":["object","null"],"x-display":"dimensions","properties":{"width":{"type":"number"},"height":{"type":"number"}},"required":["width","height"]}}}`, plugin, "", ""},
		{"geo missing keys", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{"type":"number"}}}}}`, plugin, SeverityError, `latitude and longitude`},
		{"geo lat/lng", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"lat":{"type":"number"},"lng":{"type":"number"}},"required":["lat","lng"]}}}`, plugin, "", ""},
		{"url needs host", `{"type":"object","properties":{"a":{"type":"object","x-display":"url","properties":{"href":{"type":"string"}}}}}`, plugin, SeverityError, `href and host`},
		{"url hostname", `{"type":"object","properties":{"a":{"type":"object","x-display":"url","properties":{"href":{"type":"string"},"hostname":{"type":"string"}},"required":["href","hostname"]}}}`, plugin, "", ""},
		{"daterange missing end", `{"type":"object","properties":{"a":{"type":"object","x-display":"daterange","properties":{"start":{"type":"string"}}}}}`, plugin, SeverityError, `start and end`},
		{"geo not required", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo","properties":{"latitude":{"type":"number"},"longitude":{"type":"number"}},"required":["latitude"]}}}`, plugin, SeverityWarning, `required`},
		{"forced renderer without declared properties", `{"type":"object","properties":{"a":{"type":"object","x-display":"geo"}}}`, plugin, "", ""},
		{"raw opt-out", `{"type":"object","properties":{"a":{"type":"object","x-display":"raw"}}}`, plugin, "", ""},
		{"none opt-out", `{"type":"object","properties":{"a":{"type":"string","x-display":"none"}}}`, plugin, "", ""},
		{"unknown x-display", `{"type":"object","properties":{"a":{"type":"object","x-display":"map"}}}`, plugin, SeverityWarning, `unknown x-display "map"`},
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
		{"nested in items and defs", `{"type":"object","properties":{"l":{"type":"array","items":{"type":"object","properties":{"p":{"type":"string","x-display":"geo"}}}}}}`, plugin, SeverityError, `"geo" needs an object`},
		{"nested in defs", `{"type":"object","$defs":{"x":{"type":"object","properties":{"a b":{"type":"string"}}}}}`, plugin, SeverityWarning, `letters, digits and underscores`},
		{"nested in allOf", `{"type":"object","allOf":[{"properties":{"a-b":{"type":"string"}}}]}`, plugin, SeverityWarning, `letters, digits and underscores`},
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
	ref := SchemaExtensionReference(nil)
	for _, want := range []string{"x-display", "geo", "daterange", "dimensions", "url", "raw", "x-color", "oneOf"} {
		if !strings.Contains(ref, want) {
			t.Errorf("reference lacks %q", want)
		}
	}
	if len(ref) > 2500 {
		t.Errorf("reference is %d bytes, want about 2 KB", len(ref))
	}
	if strings.Contains(ref, "plugin:gallery") {
		t.Errorf("reference names a plugin type nobody enabled")
	}
	with := SchemaExtensionReference([]string{"plugin:gallery:image-grid", "plugin:b:c"})
	for _, want := range []string{"plugin:gallery:image-grid", "plugin:b:c"} {
		if !strings.Contains(with, want) {
			t.Errorf("reference lacks the enabled type %q", want)
		}
	}
	if SchemaExtensionReference([]string{"plugin:b:c"}) != SchemaExtensionReference([]string{"plugin:b:c"}) {
		t.Error("reference is not deterministic")
	}
}
