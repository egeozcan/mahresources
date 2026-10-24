package shortcodes_test

import (
	"os"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/shortcodes"
)

// TestBuiltinDocMRQLExamplesParse guards against documenting invalid MRQL in the
// built-in shortcode examples. Every query an example would run, [mrql]
// queries, [conditional] sources and [elseif] guards alike, gets the verdict
// rendering would give it (application_context.CheckTemplateMRQL, with the
// example's own params, limit and buckets): a user copying an example
// verbatim must not hit an MRQL error, on SQLite or PostgreSQL.
func TestBuiltinDocMRQLExamplesParse(t *testing.T) {
	for _, env := range []application_context.MRQLStaticEnv{{FTS: true}, {Postgres: true, FTS: true}} {
		checked := 0
		validate := func(query string, opts shortcodes.MRQLCheckOptions) error {
			checked++
			return application_context.CheckTemplateMRQL(query, opts, env)
		}
		for _, d := range shortcodes.BuiltinDocs() {
			for _, ex := range d.Examples {
				for _, issue := range shortcodes.Lint(ex.Code, shortcodes.LintOptions{Known: shortcodes.KnownFromBuiltins(), ValidateMRQL: validate}) {
					if strings.HasPrefix(issue.Message, "MRQL error") {
						t.Errorf("shortcode %q example %q (postgres=%t): %s", d.Name, ex.Title, env.Postgres, issue.Message)
					}
				}
			}
		}
		if checked < 5 {
			t.Fatalf("only %d example queries were checked", checked)
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
