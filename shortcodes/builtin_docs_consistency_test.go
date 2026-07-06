package shortcodes_test

import (
	"os"
	"strings"
	"testing"

	"mahresources/mrql"
	"mahresources/shortcodes"
)

// TestBuiltinDocMRQLExamplesParse guards against documenting invalid MRQL in the
// built-in shortcode examples. Every [mrql query='…'] (and any [conditional
// mrql='…']) example embedded in BuiltinDocs must be a syntactically valid MRQL
// query — a user copying an example verbatim must not hit a parse error.
func TestBuiltinDocMRQLExamplesParse(t *testing.T) {
	for _, d := range shortcodes.BuiltinDocs() {
		for _, ex := range d.Examples {
			for _, sc := range shortcodes.Parse(ex.Code) {
				var query string
				switch sc.Name {
				case "mrql":
					query = sc.Attrs["query"]
				case "conditional":
					query = sc.Attrs["mrql"]
				}
				if query == "" {
					continue
				}
				if _, err := mrql.Parse(query); err != nil {
					t.Errorf("shortcode %q example %q: embedded MRQL %q does not parse: %v",
						d.Name, ex.Title, query, err)
				}
			}
		}
	}
}

// referencePanels are the in-app category / resource-category / note-type editor
// help panels. Their hand-written "Shortcodes" section must list every built-in
// shortcode so it cannot silently drift behind BuiltinDocs (the source of truth
// that also drives the editor autocomplete, hover docs, and linter).
var referencePanels = []string{
	"../templates/createCategory.tpl",
	"../templates/createResourceCategory.tpl",
	"../templates/createNoteType.tpl",
}

func TestReferencePanelsCoverAllBuiltins(t *testing.T) {
	for _, path := range referencePanels {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		for _, d := range shortcodes.BuiltinDocs() {
			token := "[" + d.Name
			if !strings.Contains(text, token) {
				t.Errorf("%s: reference panel does not document built-in shortcode %q (looked for %q)",
					path, d.Name, token)
			}
		}
	}
}

func TestDocsSiteShortcodesPageCoversAllBuiltins(t *testing.T) {
	body, err := os.ReadFile("../docs-site/docs/features/shortcodes.md")
	if err != nil {
		t.Fatalf("read docs-site shortcode page: %v", err)
	}
	text := string(body)
	for _, d := range shortcodes.BuiltinDocs() {
		token := "| `[" + d.Name + "]` |"
		if !strings.Contains(text, token) {
			t.Errorf("docs-site shortcode page does not list built-in shortcode %q in the Built-in Shortcodes table (looked for %q)",
				d.Name, token)
		}
	}
}
