package arch

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"mahresources/models"
)

// A Custom* field on a carrier is only worth anything once four separate places
// know about it: the edit form that lets an author fill it, the two front-end
// slot maps that drive the live preview and the bundle copy/export tools, and a
// render site that actually emits it. None of those is a compile-time
// dependency, so a new field can be added, saved, exported and still never
// appear anywhere — which is exactly how CustomListHeader shipped with no CLI
// flag. These tests are the backstop.

type slotCarrier struct {
	model      any
	form       string // create form template
	tplVar     string // the pongo2 variable the form binds
	entityType string // templatePreview.js `only` value
}

var slotCarriers = []slotCarrier{
	{models.Category{}, "createCategory.tpl", "category", "group"},
	{models.ResourceCategory{}, "createResourceCategory.tpl", "resourceCategory", "resource"},
	{models.NoteType{}, "createNoteType.tpl", "noteType", "note"},
}

func customSlotFields(model any) []string {
	t := reflect.TypeOf(model)
	var out []string
	for i := 0; i < t.NumField(); i++ {
		if f := t.Field(i); strings.HasPrefix(f.Name, "Custom") && f.Type.Kind() == reflect.String {
			out = append(out, f.Name)
		}
	}
	return out
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", rel))
	if err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return string(b)
}

// TestEveryCustomSlotHasAnEditor fails when a carrier gains a Custom* field that
// its create form does not offer an editor for — a field that can only ever be
// set through the API.
func TestEveryCustomSlotHasAnEditor(t *testing.T) {
	for _, c := range slotCarriers {
		form := readRepoFile(t, filepath.Join("templates", c.form))
		for _, field := range customSlotFields(c.model) {
			if !strings.Contains(form, `name="`+field+`"`) {
				t.Errorf("%s: no editor input named %q", c.form, field)
			}
			if !strings.Contains(form, "value="+c.tplVar+"."+field+" ") {
				t.Errorf("%s: editor for %s does not bind %s.%s", c.form, field, c.tplVar, field)
			}
		}
	}
}

// TestEveryCustomSlotIsInTheFrontEndSlotMaps covers the live preview pane's slot
// picker and the bundle copy/export/import map. A slot missing from the first
// cannot be previewed; a slot missing from the second is silently dropped by
// "Copy from…", by an export, and by a whole-template generation.
func TestEveryCustomSlotIsInTheFrontEndSlotMaps(t *testing.T) {
	preview := readRepoFile(t, "src/components/templatePreview.js")
	bundle := readRepoFile(t, "src/components/templateBundle.js")

	// { name: 'CustomX', label: '…' , only: 'resource' }
	entry := regexp.MustCompile(`\{ name: '(Custom\w+)'[^}]*\}`)
	previewSlots := map[string]string{}
	for _, m := range entry.FindAllStringSubmatch(preview, -1) {
		only := ""
		if i := strings.Index(m[0], "only: '"); i >= 0 {
			rest := m[0][i+len("only: '"):]
			only = rest[:strings.Index(rest, "'")]
		}
		previewSlots[m[1]] = only
	}

	for _, c := range slotCarriers {
		for _, field := range customSlotFields(c.model) {
			only, listed := previewSlots[field]
			if !listed {
				t.Errorf("templatePreview.js SLOTS is missing %s", field)
			} else if only != "" && only != c.entityType {
				// A field present on two carriers must not be pinned to one.
				t.Errorf("templatePreview.js pins %s to only:'%s', but %T declares it too", field, only, c.model)
			}
			if !strings.Contains(bundle, "'"+field+"',") {
				t.Errorf("templateBundle.js SLOT_FIELDS is missing %s", field)
			}
		}
	}
}

// TestEveryCustomSlotIsRendered is the one that proves a slot is not a dead
// column. A slot is rendered either by a template ({% process_shortcodes %} or
// an x-html read in the lightbox) or by the JSON pre-render in server/routes.go.
func TestEveryCustomSlotIsRendered(t *testing.T) {
	var sources []string
	root := filepath.Join("..", "..", "templates")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".tpl") {
			b, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			sources = append(sources, string(b))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk templates: %v", err)
	}
	sources = append(sources, readRepoFile(t, "server/routes.go"))
	all := strings.Join(sources, "\n")

	for _, c := range slotCarriers {
		for _, field := range customSlotFields(c.model) {
			// The create forms mention every field; exclude them by requiring a
			// mention that is not an editor input binding.
			hits := strings.Count(all, "."+field)
			editors := strings.Count(all, "value="+c.tplVar+"."+field+" ")
			if hits-editors < 1 {
				t.Errorf("%T.%s is never rendered: no template or JSON pre-render reads it", c.model, field)
			}
		}
	}
}

// TestEveryCustomSlotSurvivesExportImport closes the last place a slot can be
// half-wired. The archive manifest is a stable public contract, so a slot the
// export does not write is a slot that silently vanishes when a group is moved
// between instances — and nothing about that fails to compile, because both
// halves are plain struct-field copies. Checked at source level: the manifest
// must declare the field, and both the export and the import must copy it.
func TestEveryCustomSlotSurvivesExportImport(t *testing.T) {
	manifest := readRepoFile(t, "archive/manifest.go")
	export := readRepoFile(t, "groupio/export_context.go")
	imprt := readRepoFile(t, "groupio/apply_import.go")

	for _, c := range slotCarriers {
		for _, field := range customSlotFields(c.model) {
			// gofmt aligns these struct fields and literals, so the checks
			// have to tolerate runs of whitespace.
			decl := regexp.MustCompile(`\b` + field + `\s+string\s+` + "`" + `json:`)
			if !decl.MatchString(manifest) {
				t.Errorf("archive/manifest.go declares no %s field; an archive would drop it", field)
			}
			written := regexp.MustCompile(`\b` + field + `:\s`)
			if !written.MatchString(export) {
				t.Errorf("groupio/export_context.go never writes %s into the manifest", field)
			}
			read := regexp.MustCompile(`\.` + field + `\s*=\s*def\.` + field + `\b`)
			if !read.MatchString(imprt) {
				t.Errorf("groupio/apply_import.go never reads %s back out of the manifest", field)
			}
		}
	}
}
