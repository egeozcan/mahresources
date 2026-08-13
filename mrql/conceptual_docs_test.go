package mrql

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// docs-site/docs/features/mrql.md documents the same fields as the reference
// page, but as tables with a declared Type column. Nothing checked it, and it
// drifted: `category` and `noteType` were typed `string` while holding numeric
// IDs, and four queryable fields were missing outright. The type column is the
// valuable half — a reader who believes `category` is a string writes
// `category = "Photos"`, which is accepted and silently matches nothing.

const conceptualPath = "../docs-site/docs/features/mrql.md"

// documentedTypes are the type names the page uses, mapped to FieldType.
var documentedTypes = map[string]FieldType{
	"string":   FieldString,
	"number":   FieldNumber,
	"datetime": FieldDateTime,
	"relation": FieldRelation,
}

// derivedFieldTypes are fields whose documented type deliberately describes
// their query semantics rather than their storage. `shared` is backed by a
// nullable share-token column (FieldString) but only accepts booleans, so
// documenting it as "string" would be actively misleading.
var derivedFieldTypes = map[string]string{
	"shared": "boolean",
}

// The name cell may document an alias pair, as in "`groups` / `group`".
var fieldRowRE = regexp.MustCompile("^\\| ((?:`[^`]+`(?: / )?)+) \\| ([a-z/]+) \\|")
var backtickedName = regexp.MustCompile("`([^`]+)`")

// fieldTable returns the field rows of the table introduced by heading.
func fieldTable(t *testing.T, page, heading string) map[string]string {
	t.Helper()
	i := strings.Index(page, heading)
	if i < 0 {
		t.Fatalf("%s: no %q section", conceptualPath, heading)
	}
	rows := map[string]string{}
	for _, line := range strings.Split(page[i+len(heading):], "\n") {
		if strings.HasPrefix(line, "**") && len(rows) > 0 {
			break // the next section's heading
		}
		m := fieldRowRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		for _, n := range backtickedName.FindAllStringSubmatch(m[1], -1) {
			rows[n[1]] = m[2]
		}
	}
	if len(rows) == 0 {
		t.Fatalf("%s: no field rows under %q", conceptualPath, heading)
	}
	return rows
}

func TestConceptualPageDocumentsEveryFieldWithTheRightType(t *testing.T) {
	data, err := os.ReadFile(conceptualPath)
	if err != nil {
		t.Fatalf("reading %s: %v", conceptualPath, err)
	}
	page := string(data)

	cases := []struct {
		heading string
		fields  []FieldDef
	}{
		{"**Common fields** (available on all entity types):", commonFields},
		{"**Resource-only fields:**", resourceFields},
		{"**Note-only fields:**", noteFields},
		{"**Group-only fields:**", groupFields},
	}

	for _, tc := range cases {
		t.Run(tc.heading, func(t *testing.T) {
			rows := fieldTable(t, page, tc.heading)

			for _, f := range tc.fields {
				documented, ok := rows[f.Name]
				if !ok {
					t.Errorf("field %q is queryable but has no row under %q in %s",
						f.Name, tc.heading, conceptualPath)
					continue
				}
				if want, isDerived := derivedFieldTypes[f.Name]; isDerived {
					if documented != want {
						t.Errorf("field %q documents type %q, want the derived type %q",
							f.Name, documented, want)
					}
					continue
				}
				got, known := documentedTypes[documented]
				if !known {
					continue // e.g. meta.<key>'s "string/number"
				}
				if got != f.Type {
					t.Errorf("field %q is documented as %q but is %s in fields.go",
						f.Name, documented, typeName(f.Type))
				}
			}

			for name := range rows {
				if strings.HasPrefix(name, "meta.") {
					continue
				}
				if _, ok := lookupInSet(tc.fields, name); !ok {
					t.Errorf("%s documents %q under %q, but no such field exists there",
						conceptualPath, name, tc.heading)
				}
			}
		})
	}
}

func lookupInSet(fields []FieldDef, name string) (FieldDef, bool) {
	for _, f := range fields {
		if f.Name == name {
			return f, true
		}
	}
	return FieldDef{}, false
}

func typeName(t FieldType) string {
	for name, ft := range documentedTypes {
		if ft == t {
			return name
		}
	}
	return "meta"
}
