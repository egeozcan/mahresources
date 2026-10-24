package shortcodes

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The facts in schema_extensions.go are copies of what the TypeScript editors
// implement. These tests read that TypeScript and the user docs, anchored on
// identifiers rather than line numbers, so a change on either side fails here
// and names both files.

const (
	displayRenderersTS = "../src/schema-editor/display-renderers.ts"
	displayModeTS      = "../src/schema-editor/modes/display-mode.ts"
	formModeTS         = "../src/schema-editor/modes/form-mode.ts"
	schemaCoreTS       = "../src/schema-editor/schema-core.ts"
	metaShortcodeTS    = "../src/webcomponents/meta-shortcode.ts"
	metaSchemasDoc     = "../docs-site/docs/features/meta-schemas.md"
	schemaExtensionsGo = "shortcodes/schema_extensions.go"
)

func readSource(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func sameSet(a, b []string) bool {
	return strings.Join(sortedCopy(a), ",") == strings.Join(sortedCopy(b), ",")
}

func quotedList(t *testing.T, list, source string) []string {
	t.Helper()
	var out []string
	for _, m := range regexp.MustCompile(`'([^']+)'`).FindAllStringSubmatch(list, -1) {
		out = append(out, m[1])
	}
	if len(out) == 0 {
		t.Fatalf("%s: no quoted values in %q", source, list)
	}
	return out
}

// functionBody returns the text of `function name(` up to the next top-level
// closing brace.
func functionBody(t *testing.T, src, name, file string) string {
	t.Helper()
	i := strings.Index(src, "function "+name+"(")
	if i < 0 {
		t.Fatalf("%s: function %s not found; update %s", file, name, schemaExtensionsGo)
	}
	rest := src[i:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		t.Fatalf("%s: function %s has no closing brace at column 0", file, name)
	}
	return rest[:end]
}

func TestSchemaExtensionsRenderersMatchTypeScript(t *testing.T) {
	src := readSource(t, displayRenderersTS)
	block := regexp.MustCompile(`(?s)export const builtinRenderers: BuiltinRenderer\[\] = \[(.*?)\n\];`).FindStringSubmatch(src)
	if block == nil {
		t.Fatalf("%s: builtinRenderers registry not found; update %s", displayRenderersTS, schemaExtensionsGo)
	}
	var names, selectors []string
	for _, m := range regexp.MustCompile(`builtin\('([a-z]+)',\s*(\w+),`).FindAllStringSubmatch(block[1], -1) {
		names = append(names, m[1])
		selectors = append(selectors, m[2])
	}
	var goNames []string
	for _, r := range BuiltinDisplayRenderers() {
		goNames = append(goNames, r.Name)
	}
	if strings.Join(names, ",") != strings.Join(goNames, ",") {
		t.Fatalf("builtin renderers (in order) are %v in %s but %v in BuiltinDisplayRenderers (%s)", names, displayRenderersTS, goNames, schemaExtensionsGo)
	}
	for i, r := range BuiltinDisplayRenderers() {
		body := functionBody(t, src, selectors[i], displayRenderersTS)
		var tsKeys []string
		for _, m := range regexp.MustCompile(`typeof o\.(\w+)`).FindAllStringSubmatch(body, -1) {
			tsKeys = append(tsKeys, m[1])
		}
		var goKeys []string
		for _, shape := range r.Shapes {
			goKeys = append(goKeys, shape...)
		}
		seen := map[string]bool{}
		var tsUnique, goUnique []string
		for _, k := range tsKeys {
			if !seen[k] {
				seen[k] = true
				tsUnique = append(tsUnique, k)
			}
		}
		seen = map[string]bool{}
		for _, k := range goKeys {
			if !seen[k] {
				seen[k] = true
				goUnique = append(goUnique, k)
			}
		}
		if !sameSet(tsUnique, goUnique) {
			t.Errorf("renderer %q reads keys %v in %s (%s) but its Shapes in %s hold %v", r.Name, tsUnique, displayRenderersTS, selectors[i], schemaExtensionsGo, goUnique)
		}
	}
}

func TestSchemaExtensionsOptOutMatchesBothConsumers(t *testing.T) {
	display := readSource(t, displayModeTS)
	m := regexp.MustCompile(`if \(xd && ((?:xd !== '\w+'(?: && )?)+)\)`).FindStringSubmatch(display)
	if m == nil {
		t.Fatalf("%s: the forced-renderer guard `if (xd && xd !== ...)` was not found; update DisplayOptOutValues in %s", displayModeTS, schemaExtensionsGo)
	}
	tsOptOut := quotedList(t, m[1], displayModeTS)
	if !sameSet(tsOptOut, DisplayOptOutValues) {
		t.Errorf("opt-out values are %v in %s but %v in DisplayOptOutValues (%s)", tsOptOut, displayModeTS, DisplayOptOutValues, schemaExtensionsGo)
	}
	// Auto-detection runs only when x-display is unset, so an opt-out value
	// (which no renderer is registered under) suppresses it in the panel...
	if !strings.Contains(display, "if (!xd && typeof val === 'object'") {
		t.Errorf("%s: auto shape detection is no longer guarded by `!xd`, so the opt-out %v may not suppress it (see %s)", displayModeTS, DisplayOptOutValues, schemaExtensionsGo)
	}
	// ...and in [meta], which looks the value up among the built-ins.
	meta := readSource(t, metaShortcodeTS)
	if !strings.Contains(meta, "if (!xDisplay && value != null && typeof value === 'object'") ||
		!strings.Contains(meta, "const renderer = getBuiltinRenderer(xDisplay);") {
		t.Errorf("%s: [meta] no longer guards auto-detection on `!xDisplay` and looks forced renderers up with getBuiltinRenderer, so it may disagree with %s about %v (see %s)", metaShortcodeTS, displayModeTS, DisplayOptOutValues, schemaExtensionsGo)
	}
	for _, r := range BuiltinDisplayRenderers() {
		for _, v := range DisplayOptOutValues {
			if r.Name == v {
				t.Errorf("opt-out value %q is also a built-in renderer name (%s)", v, schemaExtensionsGo)
			}
		}
	}
}

func TestSchemaExtensionsComplexKeysMatchTypeScript(t *testing.T) {
	src := readSource(t, schemaCoreTS)
	body := functionBody(t, src, "isLabeledEnum", schemaCoreTS)
	m := regexp.MustCompile(`const complexKeys = new Set\(\[([^\]]*)\]\)`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("%s: isLabeledEnum's complexKeys set was not found; update LabeledEnumComplexKeys in %s", schemaCoreTS, schemaExtensionsGo)
	}
	ts := quotedList(t, m[1], schemaCoreTS)
	if !sameSet(ts, LabeledEnumComplexKeys) {
		t.Errorf("complexKeys is %v in %s but %v in LabeledEnumComplexKeys (%s)", ts, schemaCoreTS, LabeledEnumComplexKeys, schemaExtensionsGo)
	}
}

func TestSchemaExtensionsFormatsMatchTypeScript(t *testing.T) {
	re := regexp.MustCompile(`\.format === '([\w-]+)'`)
	for _, file := range []string{formModeTS, displayModeTS} {
		var ts []string
		seen := map[string]bool{}
		for _, m := range re.FindAllStringSubmatch(readSource(t, file), -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				ts = append(ts, m[1])
			}
		}
		if !sameSet(ts, SchemaFormatsWithUI()) {
			t.Errorf("formats with their own UI are %v in %s but %v in SchemaFormatsWithUI (%s)", ts, file, SchemaFormatsWithUI(), schemaExtensionsGo)
		}
	}
}

// docRows returns the table rows (lines starting with `|`) after the heading.
func docRows(t *testing.T, doc, heading string) []string {
	t.Helper()
	i := strings.Index(doc, heading)
	if i < 0 {
		t.Fatalf("%s: heading %q not found; update schema_extensions_drift_test.go", metaSchemasDoc, heading)
	}
	var rows []string
	for _, line := range strings.Split(doc[i+len(heading):], "\n") {
		if strings.HasPrefix(line, "|") {
			rows = append(rows, line)
		} else if len(rows) > 0 {
			break
		}
	}
	if len(rows) == 0 {
		t.Fatalf("%s: no table after %q", metaSchemasDoc, heading)
	}
	return rows
}

func TestSchemaExtensionsMatchDocs(t *testing.T) {
	doc := readSource(t, metaSchemasDoc)
	rowLabels := map[string]string{"url": "**URL / Location**", "geo": "**GeoLocation**", "daterange": "**Date Range**", "dimensions": "**Dimensions**"}

	shapeRows := docRows(t, doc, "### Built-in Shape Detection")
	for _, r := range BuiltinDisplayRenderers() {
		var row string
		for _, line := range shapeRows {
			if strings.Contains(line, rowLabels[r.Name]) {
				row = line
			}
		}
		if row == "" {
			t.Errorf("%s: the shape table has no row %q for renderer %q (%s)", metaSchemasDoc, rowLabels[r.Name], r.Name, schemaExtensionsGo)
			continue
		}
		for _, shape := range r.Shapes {
			for _, k := range shape {
				if !strings.Contains(row, "`"+k+"`") {
					t.Errorf("%s: the %q shape row does not mention key `%s` that %s requires", metaSchemasDoc, r.Name, k, schemaExtensionsGo)
				}
			}
		}
	}

	valueRows := strings.Join(docRows(t, doc, "**Built-in values:**"), "\n")
	for _, r := range BuiltinDisplayRenderers() {
		if !strings.Contains(valueRows, "`\""+r.Name+"\"`") {
			t.Errorf("%s: the built-in x-display values table lacks `\"%s\"` (%s)", metaSchemasDoc, r.Name, schemaExtensionsGo)
		}
	}
	for _, v := range DisplayOptOutValues {
		if !strings.Contains(valueRows, "`\""+v+"\"`") {
			t.Errorf("%s: the built-in x-display values table lacks the opt-out `\"%s\"` (%s)", metaSchemasDoc, v, schemaExtensionsGo)
		}
	}

	typeRows := strings.Join(docRows(t, doc, "### Type-Aware Rendering"), "\n")
	for _, f := range SchemaFormatsWithUI() {
		if !strings.Contains(typeRows, "\""+f+"\"") {
			t.Errorf("%s: the type-aware rendering table never mentions format \"%s\" (%s SchemaFormatsWithUI)", metaSchemasDoc, f, schemaExtensionsGo)
		}
	}
}
