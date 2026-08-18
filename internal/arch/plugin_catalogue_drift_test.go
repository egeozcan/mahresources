package arch

// A catalogue is only worth having while it is true. plugin_system refuses a
// mah.on or mah.inject name that is not in it, so an entry the host stopped
// dispatching silently disables nothing (the hook simply never fires again,
// which is the original bug wearing a badge), and a name the host dispatches
// that is missing from the catalogue refuses a plugin that would have worked.
//
// Neither direction is decidable from inside plugin_system: the events are
// dispatched from application_context and the slots are declared in the
// templates. So the catalogues are checked here, against those two sources,
// rather than against a hand-copied second list.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"mahresources/plugin_system"
)

// The shape of a lifecycle event name. Deliberately looser than the catalogue —
// it has to match a typo (`after_create_note`) and a genuinely new event
// (`before_series_create`) as readily as a correct one, or the scan would only
// ever find what it already knows.
var hookEventLiteral = regexp.MustCompile(`^(?:before|after)_[a-z]+_[a-z]+$`)

// dispatchFuncs are the calls that put an event name on the wire. A file is
// scanned only if it mentions one of them, which keeps an unrelated string
// elsewhere in the tree from being read as an event while still catching a
// dispatch site added outside application_context.
var dispatchFuncs = []string{
	"RunBeforePluginHooks", "RunAfterPluginHooks",
	"RunBeforeHooks", "RunAfterHooks",
}

// dispatchedHookEvents returns every event-shaped string literal in the
// non-test Go files that dispatch hooks.
//
// Literals rather than call arguments, because note_context.go picks its event
// with a variable — `hookEvent := "before_note_create"`, reassigned to
// `"before_note_update"` for an edit — so reading only the argument expression
// would miss four of the thirty. Parsed rather than grepped, so prose in a
// comment cannot stand in for a dispatch.
func dispatchedHookEvents(t *testing.T) map[string]string {
	t.Helper()

	root := moduleRoot(t)
	fset := token.NewFileSet()
	found := map[string]string{}
	var scanned int

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path != root && (skipDirs[info.Name()] || strings.HasPrefix(info.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		src := string(body)
		var dispatches bool
		for _, fn := range dispatchFuncs {
			if strings.Contains(src, fn+"(") {
				dispatches = true
				break
			}
		}
		if !dispatches {
			return nil
		}
		scanned++

		f, perr := parser.ParseFile(fset, path, src, 0)
		if perr != nil {
			t.Fatalf("parsing %s: %v", path, perr)
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			val, uerr := strconv.Unquote(lit.Value)
			if uerr != nil || !scanSeesEventLiteral(val) {
				return true
			}
			found[val] = filepath.ToSlash(rel)
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking module: %v", err)
	}
	if scanned == 0 {
		t.Fatal("no file dispatches hooks; this scan is broken, not the catalogue")
	}
	if len(found) == 0 {
		t.Fatal("found no dispatched events; this scan is broken, not the catalogue")
	}
	return found
}

func TestHookEventCatalogueMatchesWhatTheHostDispatches(t *testing.T) {
	dispatched := dispatchedHookEvents(t)

	catalogued := map[string]bool{}
	for _, e := range plugin_system.AllHookEvents {
		catalogued[e] = true
	}

	var missing, unused []string
	for event, where := range dispatched {
		if !catalogued[event] {
			missing = append(missing, event+" ("+where+")")
		}
	}
	for event := range catalogued {
		if _, ok := dispatched[event]; !ok {
			unused = append(unused, event)
		}
	}
	sort.Strings(missing)
	sort.Strings(unused)

	if len(missing) > 0 {
		t.Errorf("the host dispatches events that plugin_system.AllHookEvents does not list, so mah.on refuses "+
			"a plugin that would have worked: %s", strings.Join(missing, ", "))
	}
	if len(unused) > 0 {
		t.Errorf("plugin_system.AllHookEvents lists events nothing dispatches, so mah.on accepts a name that "+
			"can never fire: %s", strings.Join(unused, ", "))
	}
}

// The slot literals a page actually renders. templateFiles walks templates/;
// the tag is parsed by server/template_handlers/template_filters/plugin_slot.go,
// which takes a single string token, so a literal is the only form there is.
var pluginSlotTag = regexp.MustCompile(`{%\s*plugin_slot\s+"([^"]+)"\s*%}`)

func TestInjectionSlotCatalogueMatchesTheTemplates(t *testing.T) {
	declared := map[string]string{}
	for path, body := range templateFiles(t) {
		for _, slot := range slotNamesIn(body) {
			declared[slot] = path
		}
	}
	if len(declared) == 0 {
		t.Fatal("found no plugin_slot tags; this scan is broken, not the catalogue")
	}

	catalogued := map[string]bool{}
	for _, s := range plugin_system.AllInjectionSlots {
		catalogued[s] = true
	}

	var missing, unrendered []string
	for slot, where := range declared {
		if !catalogued[slot] {
			missing = append(missing, slot+" ("+where+")")
		}
	}
	for slot := range catalogued {
		if _, ok := declared[slot]; !ok {
			unrendered = append(unrendered, slot)
		}
	}
	sort.Strings(missing)
	sort.Strings(unrendered)

	if len(missing) > 0 {
		t.Errorf("templates declare slots that plugin_system.AllInjectionSlots does not list, so mah.inject "+
			"refuses a plugin that would have rendered: %s", strings.Join(missing, ", "))
	}
	if len(unrendered) > 0 {
		t.Errorf("plugin_system.AllInjectionSlots lists slots no template renders, so mah.inject accepts a "+
			"name that is still a silent no-op: %s", strings.Join(unrendered, ", "))
	}
}

// scanSeesEventLiteral is how the scan above decides a string literal is an
// event name. The test below constrains that decision rather than the way it
// happens to be made today, so a filter that stops being a regexp re-points
// this one line instead of a table of call sites.
func scanSeesEventLiteral(s string) bool {
	return hookEventLiteral.MatchString(s)
}

// The scan is the only thing standing between a new event and the catalogue, so
// its filter has to admit every name the host could plausibly dispatch, not
// only the names it already dispatches. A literal the filter does not match is
// a literal the guard never compares: the event ships uncatalogued and green,
// IsHookEvent then refuses the plugin that asks for it, and the host's own
// dispatch is skipped with a log line for company. That is the exact failure
// this file exists to prevent, arriving through the guard meant to prevent it.
//
// A filter that allows a one-word entity is defeated by every entity in this
// tree whose name is two, and each of these has CRUD paths that could grow a
// hook tomorrow: NoteBlock, ResourceVersion, GroupRelation, ResourceCategory,
// SavedMRQLQuery. The last one is four words, so "allow three" is no fix
// either.
//
// Only the accepting direction is pinned here. Over-loosening is already
// checked, by TestHookEventCatalogueMatchesWhatTheHostDispatches on a clean
// tree: a filter loose enough to sweep up an ordinary literal in one of the
// files it reads reports that literal as an event the catalogue is missing, and
// goes red at once.
func TestHookEventScanSeesAMultiWordEntity(t *testing.T) {
	for _, event := range []string{
		"before_note_block_create",
		"after_note_block_delete",
		"after_resource_version_create",
		"before_group_relation_create",
		"after_resource_category_update",
		"before_saved_mrql_query_update",
	} {
		if !scanSeesEventLiteral(event) {
			t.Errorf("the scan does not recognise %q as an event name, so a dispatch of it added without "+
				"a catalogue entry passes this guard", event)
		}
	}

	// Still a filter. "id" and "name" are ordinary literals in the files the
	// scan reads; "resource_create" is an event name with its prefix lost.
	for _, notAnEvent := range []string{"id", "name", "resource_create"} {
		if scanSeesEventLiteral(notAnEvent) {
			t.Errorf("the scan reads %q as an event name, so the catalogue is asked to list a string "+
				"nothing dispatches", notAnEvent)
		}
	}
}

// slotNamesIn returns the slot names a template body declares. The catalogue
// check above and the test below both read it, so a scan taught to see a
// spelling it was missing cannot be taught it in only one of the two places.
func slotNamesIn(body string) []string {
	var out []string
	for _, m := range pluginSlotTag.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

// pongo2 opens a string token on either quote character and closes it on the
// one it opened with (lexer.go stateString, reached from l.accept("\"'")), so
// {% plugin_slot 'head' %} parses and renders exactly like the double-quoted
// form. The scan reads only double quotes, so a slot declared the other legal
// way is invisible to it: the catalogue never learns the name, mah.inject
// refuses it, and RenderSlot warns about a slot the template really does
// declare.
//
// Every tag in templates/ is double-quoted today, which is why the catalogue
// check is green and why scanning the tree cannot find this. It is the defect
// above wearing different clothes, on the other half of the same file: a form
// the host accepts and the scan cannot see.
func TestSlotScanSeesASingleQuotedTag(t *testing.T) {
	got := slotNamesIn(`{% plugin_slot 'group_detail_sidebar' %}`)
	if len(got) != 1 || got[0] != "group_detail_sidebar" {
		t.Errorf("the scan read %q from a single-quoted plugin_slot tag, so a slot declared that way is "+
			"missing from this guard while the page renders it", got)
	}

	// The form every template uses today must keep working.
	if got := slotNamesIn(`{% plugin_slot "head" %}`); len(got) != 1 || got[0] != "head" {
		t.Errorf("the scan stopped reading the double-quoted form: %q", got)
	}
}
