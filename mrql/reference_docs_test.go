package mrql

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The MRQL reference page is the single source the installable agent skill is
// generated from (cmd/skills-gen), so a field added to fields.go that never
// reaches the page silently ships an incomplete skill. These tests fail the
// build instead.

const referencePath = "../docs-site/docs/features/mrql-reference.md"

func readReference(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(referencePath)
	if err != nil {
		t.Fatalf("reading %s: %v", referencePath, err)
	}
	return string(data)
}

// fieldsSection returns the "**<label>:**" bullet listing an entity's fields.
func fieldsSection(t *testing.T, page, label string) string {
	t.Helper()
	marker := "**" + label + ":**"
	i := strings.Index(page, marker)
	if i < 0 {
		t.Fatalf("%s: no %q bullet in the Fields section", referencePath, marker)
	}
	rest := page[i+len(marker):]
	if j := strings.Index(rest, "\n\n"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func TestReferenceDocumentsEveryField(t *testing.T) {
	page := readReference(t)

	cases := []struct {
		label  string
		fields []FieldDef
	}{
		{"Common to all types", commonFields},
		{"Resources only", resourceFields},
		{"Notes only", noteFields},
		{"Groups only", groupFields},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			section := fieldsSection(t, page, tc.label)
			for _, f := range tc.fields {
				// "group" is documented as an alias of "groups" rather than as
				// its own list entry.
				if f.Name == "group" {
					if !strings.Contains(section, "alias `group`") {
						t.Errorf("field %q: expected the `groups` entry to note the alias", f.Name)
					}
					continue
				}
				if !strings.Contains(section, "`"+f.Name+"`") {
					t.Errorf("field %q is queryable but missing from the %q list in %s",
						f.Name, tc.label, referencePath)
				}
			}
		})
	}
}

var backtickedRE = regexp.MustCompile("`([A-Za-z][A-Za-z0-9_.]*)`")

func TestReferenceInventsNoFields(t *testing.T) {
	page := readReference(t)

	// Names that legitimately appear in a fields bullet without being a
	// FieldDef: pseudo-fields, the meta prefix, and the alias note.
	allowed := map[string]bool{
		"meta.<key>": true, "meta": true, "TEXT": true, "type": true,
		"SCOPE": true, "group": true,
	}

	// Each section is checked against exactly its own set. LookupField alone
	// would be too weak in both directions: it answers "valid on this entity",
	// which is true of a common field listed under "Resources only" and, because
	// common fields are checked first, of a resource-only field listed under
	// "Common to all types".
	cases := []struct {
		label      string
		entityType EntityType
		common     bool
	}{
		{"Common to all types", EntityResource, true},
		{"Resources only", EntityResource, false},
		{"Notes only", EntityNote, false},
		{"Groups only", EntityGroup, false},
	}

	for _, tc := range cases {
		t.Run(tc.label, func(t *testing.T) {
			section := fieldsSection(t, page, tc.label)
			for _, m := range backtickedRE.FindAllStringSubmatch(section, -1) {
				name := m[1]
				if allowed[name] || strings.HasPrefix(name, "meta.") {
					continue
				}
				if tc.common {
					if !IsCommonField(name) {
						t.Errorf("%s lists %q as common to all types, but it is not a common field",
							referencePath, name)
					}
					continue
				}
				if IsCommonField(name) {
					t.Errorf("%s lists the common field %q under %q, where it reads as entity-specific",
						referencePath, name, tc.label)
					continue
				}
				if _, ok := LookupField(tc.entityType, name); !ok {
					t.Errorf("%s documents %q in the %q list, but it is not a queryable field",
						referencePath, name, tc.label)
				}
			}
		})
	}
}

func TestReferenceGuardrailNumbersMatchLimits(t *testing.T) {
	page := readReference(t)

	// The guardrail table is what a caller sizes a query against; a constant
	// changed in limits.go without the table is a documented lie.
	for _, want := range []struct {
		label string
		value int
	}{
		{"Query text", MaxQueryBytes},
		{"Lexer tokens", MaxQueryTokens},
		{"Nested `NOT` / boolean parentheses", MaxExpressionDepth},
		{"Values in one `IN` / `NOT IN` list", MaxINListValues},
	} {
		row := guardrailRow(t, page, want.label)
		if got := digitsOnly(row); got != fmt.Sprint(want.value) {
			t.Errorf("guardrail %q documents %q, want %d", want.label, got, want.value)
		}
	}
}

func guardrailRow(t *testing.T, page, label string) string {
	t.Helper()
	for _, line := range strings.Split(page, "\n") {
		if strings.HasPrefix(line, "| "+label+" |") {
			return line
		}
	}
	t.Fatalf("%s: no guardrail row for %q", referencePath, label)
	return ""
}

var guardrailNumberRE = regexp.MustCompile(`\d[\d,]*`)

// digitsOnly reads the maximum out of a guardrail row, dropping the thousands
// separators and any unit suffix ("32,768 bytes").
func digitsOnly(row string) string {
	cells := strings.Split(row, "|")
	if len(cells) < 3 {
		return ""
	}
	m := guardrailNumberRE.FindString(cells[2])
	return strings.ReplaceAll(m, ",", "")
}

// TestSkillReferenceIsGenerated guards the other half of the contract: the
// checked-in skill file must actually be the transformed reference, not a copy
// someone edited by hand. CI regenerates and diffs; this catches the local case
// where the header was deleted or the file was hand-edited.
func TestSkillReferenceIsGenerated(t *testing.T) {
	path := filepath.Join("..", "skills", "mahresources-mrql", "references", "language.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	body := string(data)
	if !strings.HasPrefix(body, "<!--\nGENERATED FILE. DO NOT EDIT.") {
		t.Fatalf("%s lost its generated-file header; run 'npm run skills-gen'", path)
	}
	if !strings.Contains(body, "## CLI Reference") {
		t.Errorf("%s is missing the generated CLI section", path)
	}
	if strings.Contains(body, "](./") || strings.Contains(body, "](../") {
		t.Errorf("%s still has relative doc links, which do not resolve once the skill is installed elsewhere", path)
	}
}
