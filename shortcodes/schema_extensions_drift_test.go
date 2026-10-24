package shortcodes

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The facts in schema_extensions.go are copies of what the TypeScript editors
// implement. The contract file below is the Go side written out; the vitest
// test src/schema-editor/schema-extensions-contract.test.ts imports the real
// selectors and checks their behaviour against it, so a change on either side
// fails one of the two tests and the message names both files.

const (
	metaSchemasDoc     = "../docs-site/docs/features/meta-schemas.md"
	schemaExtensionsGo = "shortcodes/schema_extensions.go"
	contractFile       = "testdata/schema_extensions_contract.json"
	contractTest       = "src/schema-editor/schema-extensions-contract.test.ts"
)

func readSource(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

type contractRenderer struct {
	Name       string            `json:"name"`
	Shapes     [][]string        `json:"shapes"`
	FieldTypes map[string]string `json:"fieldTypes"`
}

type schemaExtensionsContract struct {
	Renderers   []contractRenderer `json:"renderers"`
	OptOut      []string           `json:"optOut"`
	Formats     []string           `json:"formats"`
	ComplexKeys []string           `json:"complexKeys"`
}

func currentContract() []byte {
	c := schemaExtensionsContract{OptOut: DisplayOptOutValues, Formats: SchemaFormatsWithUI(), ComplexKeys: LabeledEnumComplexKeys}
	for _, r := range BuiltinDisplayRenderers() {
		c.Renderers = append(c.Renderers, contractRenderer{Name: r.Name, Shapes: r.Shapes, FieldTypes: r.FieldTypes})
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}

// TestSchemaExtensionsContractIsCurrent fails when the committed contract is
// not what the Go side says. Regenerate it with UPDATE_SCHEMA_CONTRACT=1.
func TestSchemaExtensionsContractIsCurrent(t *testing.T) {
	want := currentContract()
	if os.Getenv("UPDATE_SCHEMA_CONTRACT") == "1" {
		if err := os.WriteFile(contractFile, want, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := os.ReadFile(contractFile)
	if err != nil {
		t.Fatalf("read %s: %v; regenerate with UPDATE_SCHEMA_CONTRACT=1 go test ./shortcodes -run TestSchemaExtensionsContractIsCurrent", contractFile, err)
	}
	if string(got) != string(want) {
		t.Fatalf("%s is stale against %s; regenerate with UPDATE_SCHEMA_CONTRACT=1 go test --tags 'json1 fts5' ./shortcodes -run TestSchemaExtensionsContractIsCurrent, then run `npx vitest run %s` to see whether the TypeScript still behaves as the contract says", contractFile, schemaExtensionsGo, contractTest)
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
