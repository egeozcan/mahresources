package arch

// Template guards for the 2026-07-29 UI bug hunt (docs/todo.md, Phase 3).
//
// Several of that hunt's findings are one defect repeated across entities, and
// each of them is decidable from the template source alone. They are written
// here, in Go, rather than as Playwright specs, because `.github/workflows/
// ci.yml` runs `go test` and does not run the browser suite — a guard written
// as a spec gates nothing.
//
// layering_test.go's walker skips templates/ (it is looking for Go imports), so
// this file has its own walk.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// templateFiles returns every .tpl under templates/, keyed by its path relative
// to the module root (slash-separated), with its contents.
func templateFiles(t *testing.T) map[string]string {
	t.Helper()

	root := moduleRoot(t)
	dir := filepath.Join(root, "templates")
	out := map[string]string{}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".tpl") {
			return nil
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = string(body)
		return nil
	})
	if err != nil {
		t.Fatalf("walking templates/: %v", err)
	}
	if len(out) < 100 {
		t.Fatalf("found only %d templates; the walk is broken, not the templates", len(out))
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// -- {# … #} comments ---------------------------------------------------------

// TestNoMultiLinePongoComment is not in the plan's guard list, and it should
// have been. Pongo2's comment token is matched on a single line: a `{#` whose
// `#}` is on a later line is not a comment, it is a parse error, and every page
// in the application answers ERR_EMPTY_RESPONSE. Batch 8 of the hunt wrote eight
// of them in one pass and took the whole app down; docs/lessons.md records it
// and nothing enforced it.
//
// The rule has to be narrow enough to leave comments usable — several later
// batches wrote correct single-line ones, and there are hundreds in the tree.
// Spanning a newline is the entire defect, so that is the entire rule.
func TestNoMultiLinePongoComment(t *testing.T) {
	files := templateFiles(t)

	total := 0
	for _, name := range sortedKeys(files) {
		body := files[name]
		for idx := 0; ; {
			open := strings.Index(body[idx:], "{#")
			if open < 0 {
				break
			}
			open += idx
			total++
			closeIdx := strings.Index(body[open+2:], "#}")
			if closeIdx < 0 {
				t.Errorf("%s:%d: `{#` is never closed.\n"+
					"\tAn unterminated pongo2 comment is a parse error, and every page\n"+
					"\tthat extends this template returns an empty response.",
					name, lineOf(body, open))
				break
			}
			closeIdx += open + 2
			if strings.Contains(body[open:closeIdx], "\n") {
				t.Errorf("%s:%d: `{# … #}` comment spans a newline.\n"+
					"\tPongo2 matches a comment on one line. A multi-line one is a parse\n"+
					"\terror that takes down every page extending this template with\n"+
					"\tERR_EMPTY_RESPONSE. Split it into one `{# … #}` per line.",
					name, lineOf(body, open))
			}
			idx = closeIdx + 2
		}
	}

	// Positive control: without it, a walk that found no comments at all would
	// satisfy every assertion above forever.
	if total < 100 {
		t.Errorf("only %d `{#` tokens found across templates/; expected hundreds. "+
			"The scan is not looking at the comments it claims to guard.", total)
	}
}

// -- {% empty %} on list pages -------------------------------------------------

// listTemplatesWithoutAServerSideLoop documents the list templates that
// legitimately have no `{% for %}` over their collection, so the sweep below
// cannot be satisfied vacuously by one of them.
//
// The timeline views render a bar chart from `/v1/<entity>/timeline` in the
// browser (partials/timeline.tpl); there is no server-rendered collection, and
// an empty chart is the correct rendering of no data. Their only `{% for %}` is
// the popular-tags list in the sidebar, which the sweep already excludes.
var listTemplatesWithoutAServerSideLoop = map[string]string{
	"templates/listCategoriesTimeline.tpl": "chart rendered client-side from /v1/categories/timeline",
	"templates/listGroupsTimeline.tpl":     "chart rendered client-side from /v1/groups/timeline",
	"templates/listNotesTimeline.tpl":      "chart rendered client-side from /v1/notes/timeline",
	"templates/listQueriesTimeline.tpl":    "chart rendered client-side from /v1/queries/timeline",
	"templates/listResourcesTimeline.tpl":  "chart rendered client-side from /v1/resources/timeline",
	"templates/listTagsTimeline.tpl":       "chart rendered client-side from /v1/tags/timeline",
}

// templateTag matches a pongo2 statement tag and captures its keyword.
var templateTag = regexp.MustCompile(`\{%-?\s*(\w+)`)

// blockStack walks a template's `{% block %}`/`{% endblock %}` nesting and calls
// visit for every statement tag with the stack of enclosing block names.
func blockStack(body string, visit func(keyword string, offset int, stack []string)) {
	var stack []string
	for _, m := range templateTag.FindAllStringSubmatchIndex(body, -1) {
		keyword := body[m[2]:m[3]]
		switch keyword {
		case "block":
			// The block's name is the next word after the keyword.
			rest := body[m[3]:]
			end := strings.Index(rest, "%}")
			if end < 0 {
				end = len(rest)
			}
			stack = append(stack, strings.TrimSpace(strings.TrimSuffix(rest[:end], "-")))
		case "endblock":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
		visit(keyword, m[0], stack)
	}
}

func inBlock(stack []string, name string) bool {
	for _, b := range stack {
		if b == name {
			return true
		}
	}
	return false
}

// TestListTemplatesHaveAnEmptyBranch catches findings 54, 68, 77, 126 and 146 of
// the hunt: a list page whose collection is empty rendered nothing at all — no
// heading, no "no results", no hint that a filter was responsible — because the
// `{% for %}` had no `{% empty %}` branch.
//
// Only loops outside the `sidebar` block count. The sidebar's loops are over
// popular tags and filter option lists, where "nothing to show" correctly
// renders nothing.
func TestListTemplatesHaveAnEmptyBranch(t *testing.T) {
	files := templateFiles(t)

	swept := 0
	for _, name := range sortedKeys(files) {
		if !strings.HasPrefix(name, "templates/list") {
			continue
		}
		body := files[name]

		type openFor struct {
			offset  int
			hasElse bool
		}
		var stack []*openFor
		var loops []*openFor

		blockStack(body, func(keyword string, offset int, blocks []string) {
			if inBlock(blocks, "sidebar") {
				return
			}
			switch keyword {
			case "for":
				f := &openFor{offset: offset}
				stack = append(stack, f)
				loops = append(loops, f)
			case "empty":
				if len(stack) > 0 {
					stack[len(stack)-1].hasElse = true
				}
			case "endfor":
				if len(stack) > 0 {
					stack = stack[:len(stack)-1]
				}
			}
		})

		if len(loops) == 0 {
			if _, ok := listTemplatesWithoutAServerSideLoop[name]; !ok {
				t.Errorf("%s renders no server-side collection loop and is not in "+
					"listTemplatesWithoutAServerSideLoop.\n"+
					"\tEither it grew a client-rendered list (add it, with the reason) or\n"+
					"\tthis sweep has stopped seeing its loop — in which case the guard is\n"+
					"\tpassing vacuously.", name)
			}
			continue
		}
		if reason, ok := listTemplatesWithoutAServerSideLoop[name]; ok {
			t.Errorf("%s is allowlisted as %q but does have a server-side loop.\n"+
				"\tRemove the allowlist entry; the loop needs an {%% empty %%} branch\n"+
				"\tlike every other list page.", name, reason)
		}
		swept += len(loops)
		for _, f := range loops {
			if !f.hasElse {
				t.Errorf("%s:%d: `{%% for %%}` over a collection has no `{%% empty %%}` branch.\n"+
					"\tWith no rows the page renders chrome and nothing else, so a reader\n"+
					"\tcannot tell an empty database from a filter that matched nothing.\n"+
					"\tUse {%% include \"/partials/listEmpty.tpl\" %%}.", name, lineOf(body, f.offset))
			}
		}
	}

	// Positive control. The sweep is a negative assertion over templates it
	// discovers for itself; if the discovery breaks, every assertion above holds
	// for free.
	if swept < 15 {
		t.Errorf("only %d list-page loops were examined; expected at least 15. "+
			"The sweep is not finding the loops it claims to guard.", swept)
	}
}

// -- breadcrumbs ---------------------------------------------------------------

var homeURLLiteral = regexp.MustCompile(`"HomeUrl":\s*"([^"]*)"`)
var homeURLTemplateArg = regexp.MustCompile(`HomeUrl=['"]([^'"]*)['"]`)

// TestBreadcrumbHomeURLIsAbsolute catches finding 45 and the two latent siblings
// found beside it. `group_template_context.go` passed `"HomeUrl": "groups"` —
// relative — so the breadcrumb's Home link resolved against whatever path the
// reader was on: `/groups` from `/group?id=N` (correct by luck) and
// `/group/groups` from `/group/tree` (a 404). The two other providers had the
// same literal and were harmless only because of their URL depth.
//
// This is a source scan rather than a rendered-page assertion on purpose: a new
// provider with a relative value is caught before anyone renders the page it
// belongs to.
func TestBreadcrumbHomeURLIsAbsolute(t *testing.T) {
	root := moduleRoot(t)

	found := 0
	check := func(file string, line int, value string) {
		found++
		if !strings.HasPrefix(value, "/") && !strings.HasPrefix(value, "http") {
			t.Errorf("%s:%d: breadcrumb HomeUrl %q is relative.\n"+
				"\tpartials/breadcrumb.tpl emits it as href verbatim, so it resolves\n"+
				"\tagainst the current path: %q reads as /groups from /group?id=N and\n"+
				"\tas /group/%s from /group/tree. Write it absolute.",
				file, line, value, value, value)
		}
	}

	// Go providers, where the value is authored.
	err := filepath.Walk(filepath.Join(root, "server"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		rel, _ := filepath.Rel(root, path)
		for _, m := range homeURLLiteral.FindAllStringSubmatchIndex(string(body), -1) {
			check(filepath.ToSlash(rel), lineOf(string(body), m[0]), string(body)[m[2]:m[3]])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking server/: %v", err)
	}

	// Templates, where an include could pass a literal directly.
	for _, name := range sortedKeys(templateFiles(t)) {
		body := templateFiles(t)[name]
		for _, m := range homeURLTemplateArg.FindAllStringSubmatchIndex(body, -1) {
			check(name, lineOf(body, m[0]), body[m[2]:m[3]])
		}
	}

	if found == 0 {
		t.Error("no HomeUrl values found at all; the scan is broken, not the breadcrumbs")
	}
}

// -- types.JSON rendered bare --------------------------------------------------

// jsonFieldNames reads models/ and returns the set of struct field names whose
// declared type is types.JSON. Deriving the list rather than hardcoding it means
// a model that grows a new JSON column is covered the day it is added.
func jsonFieldNames(t *testing.T) map[string]string {
	t.Helper()

	root := moduleRoot(t)
	fset := token.NewFileSet()
	out := map[string]string{}

	err := filepath.Walk(filepath.Join(root, "models"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		ast.Inspect(f, func(n ast.Node) bool {
			st, ok := n.(*ast.StructType)
			if !ok || st.Fields == nil {
				return true
			}
			for _, field := range st.Fields.List {
				sel, ok := field.Type.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "JSON" {
					continue
				}
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != "types" {
					continue
				}
				for _, name := range field.Names {
					out[name.Name] = filepath.ToSlash(rel)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking models/: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("no types.JSON fields found in models/; the scan is broken, not the models")
	}
	return out
}

// outputTag matches a pongo2 `{{ … }}` output expression.
var outputTag = regexp.MustCompile(`\{\{([^}]*)\}\}`)

// pongoComment matches a single-line `{# … #}`. TestNoMultiLinePongoComment is
// what makes "single-line" safe to assume here.
var pongoComment = regexp.MustCompile(`\{#[^\n]*?#\}`)

// bareJSONFieldExceptions allows an expression whose final path segment is the
// name of a types.JSON field on *some* model, but not on the model this template
// is actually rendering. Matching is by name only — templates carry no type
// information — so a name shared between a JSON column and a plain one needs an
// entry here. Each entry MUST say why, and TestNoTemplateRendersATypesJSONFieldBare
// fails if an entry stops matching, so a stale exception cannot linger.
var bareJSONFieldExceptions = map[string]string{
	// models.TemplatePartial.Content is a plain `string` (the template body).
	// The name collides with models.NoteBlock.Content, which is types.JSON.
	"templates/displayTemplatePartial.tpl:templatePartial.Content": "TemplatePartial.Content is a string, not types.JSON",
}

// TestNoTemplateRendersATypesJSONFieldBare catches finding 26: `/log?id=N`
// rendered `<types.JSON Value>` where the details payload should be.
//
// types.JSON is a slice type, and pongo2 only consults fmt.Stringer for structs,
// so `{{ log.Details }}` prints Go's reflect wrapper. The value has to go
// through `|json`, through `.String`, or through a sub-path — all three of which
// this test accepts. Only the bare field is a defect.
//
// Compound expressions (anything with a filter, a call, or internal whitespace)
// are skipped: the defect always looks like a plain path, and reading the others
// would need an expression parser this guard does not have.
func TestNoTemplateRendersATypesJSONFieldBare(t *testing.T) {
	jsonFields := jsonFieldNames(t)
	files := templateFiles(t)

	checked := 0
	usedExceptions := map[string]bool{}
	for _, name := range sortedKeys(files) {
		body := files[name]
		// Comments are not output. displayLog.tpl's own comment quotes the
		// defective expression to explain why the fix is there, and reading it
		// as code would fail this test against the fix for the bug it names.
		scanned := pongoComment.ReplaceAllStringFunc(body, func(c string) string {
			return strings.Repeat(" ", len(c))
		})
		for _, m := range outputTag.FindAllStringSubmatchIndex(scanned, -1) {
			expr := strings.TrimSpace(scanned[m[2]:m[3]])
			if strings.ContainsAny(expr, "|( \t") {
				continue
			}
			path := strings.Split(expr, ".")
			if len(path) < 2 {
				continue // a bare context variable, not a model field access
			}
			last := path[len(path)-1]
			checked++
			model, ok := jsonFields[last]
			if !ok {
				continue
			}
			key := name + ":" + expr
			if _, allowed := bareJSONFieldExceptions[key]; allowed {
				usedExceptions[key] = true
				continue
			}
			t.Errorf("%s:%d: `{{ %s }}` renders a types.JSON field bare.\n"+
				"\t%s declares %s as types.JSON, which is a slice type; pongo2 only\n"+
				"\tconsults fmt.Stringer for structs, so this prints\n"+
				"\t\"<types.JSON Value>\" to the reader. Use `|json`, `.String`, or a\n"+
				"\tsub-path.", name, lineOf(scanned, m[0]), expr, model, last)
		}
	}
	if checked == 0 {
		t.Error("no dotted output expressions examined; the scan is broken, not the templates")
	}
	for key, reason := range bareJSONFieldExceptions {
		if !usedExceptions[key] {
			t.Errorf("bareJSONFieldExceptions[%q] (%s) matched nothing.\n"+
				"\tThe expression it excuses is gone, so the exception is now a hole\n"+
				"\tin the guard rather than a documented one. Remove it.", key, reason)
		}
	}
}

// -- bulk editor / list container ----------------------------------------------

// listContainerSelector mirrors LIST_CONTAINER_SELECTOR in
// src/utils/listContainer.js. Kept as three separate needles rather than the
// literal selector string because the test greps template markup, not CSS.
var listContainerHooks = []string{"data-list-container", "list-container", "items-container"}

// TestBulkEditorPagesExposeAListContainer catches finding 9, the highest-severity
// false-failure in the hunt: every bulk operation on /resources/details reported
// "Bulk operation failed" *after* the write had already succeeded, because the
// page had none of the hooks findListContainer() looks for, so the refresh threw.
//
// The coupling is worth stating, because it is what made the obvious fix wrong:
// `.list-container` and `.items-container` are layout classes (grid and flex
// column), so a view whose layout is neither cannot simply take one — giving the
// details table a `.list-container` breaks the table. `data-list-container` is
// the opt-in hook that carries no styling, and it is the one a new view should
// use.
func TestBulkEditorPagesExposeAListContainer(t *testing.T) {
	files := templateFiles(t)

	withEditor := 0
	for _, name := range sortedKeys(files) {
		body := files[name]
		if !strings.Contains(body, "bulkEditor") {
			continue
		}
		if strings.HasPrefix(name, "templates/partials/") {
			continue // the bulk editor partials themselves
		}
		withEditor++
		if !containsAny(body, listContainerHooks) {
			t.Errorf("%s includes a bulkEditor partial but exposes no list container.\n"+
				"\tsrc/utils/listContainer.js resolves the element to morph with\n"+
				"\t'[data-list-container], .list-container, .items-container'. Without\n"+
				"\tone, every bulk operation on this page succeeds on the server and\n"+
				"\tthen reports \"Bulk operation failed\" to the reader.\n"+
				"\tPrefer `data-list-container` — the two classes carry layout.", name)
		}
	}
	if withEditor < 5 {
		t.Errorf("only %d templates include a bulkEditor partial; expected at least 5. "+
			"The sweep is not finding the pages it claims to guard.", withEditor)
	}
}

func containsAny(body string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(body, n) {
			return true
		}
	}
	return false
}

func lineOf(body string, offset int) int {
	if offset > len(body) {
		offset = len(body)
	}
	return strings.Count(body[:offset], "\n") + 1
}

// -- tag chips on a detail page ------------------------------------------------

var cardInclude = regexp.MustCompile(`\{%\s*include\s+(?:"[^"]*partials/(resource|note|group)\.tpl"|partial\("(resource|note|group)"\))([^%]*)%\}`)

// TestDetailPagesGiveCardPartialsATagBaseURL catches finding 71.
//
// A tag chip in partials/resource.tpl links to `withQuery("tags", id)` unless the
// caller supplies `tagBaseUrl`, and withQuery appends to the URL the reader is
// on. On a list page that is right — /resources?tags=79 is exactly the filter.
// On a *detail* page it produced `/resource?id=88&tags=79`: a link back to the
// same page, carrying a parameter that page ignores, so clicking a tag appeared
// to do nothing.
//
// The scope is display*.tpl and dashboard.tpl, i.e. every template that renders
// a card somewhere other than its own list. Partials under templates/partials/
// are excluded deliberately: pongo2's `include ... with` extends the parent
// context rather than replacing it, so partials/relation.tpl inherits the
// tagBaseUrl seeAll.tpl passed, and requiring them to repeat it would be wrong
// as well as noisy.
func TestDetailPagesGiveCardPartialsATagBaseURL(t *testing.T) {
	files := templateFiles(t)

	checked := 0
	for _, name := range sortedKeys(files) {
		base := strings.TrimPrefix(name, "templates/")
		if !strings.HasPrefix(base, "display") && base != "dashboard.tpl" {
			continue
		}
		body := files[name]
		for _, m := range cardInclude.FindAllStringSubmatch(body, -1) {
			partial := m[1]
			if partial == "" {
				partial = m[2]
			}
			args := m[3]
			checked++
			if !strings.Contains(args, "tagBaseUrl") {
				t.Errorf("%s includes the %q card partial without tagBaseUrl.\n"+
					"\tIts tag chips then use withQuery(), which appends to the current URL —\n"+
					"\tso on a detail page a chip links back to that same page with a\n"+
					"\tparameter it ignores, and clicking a tag does nothing.\n"+
					"\tPass tagBaseUrl=\"/%ss\".", name, partial, partial)
			}
		}
	}

	// Positive control: the sweep must actually find card includes on detail
	// pages, or every assertion above holds because the regex stopped matching.
	if checked < 4 {
		t.Errorf("only %d card-partial includes found on detail pages; expected at least 4. "+
			"The scan is not finding the includes it claims to guard.", checked)
	}

	// And the other half of the control: the *list* pages must still be able to
	// omit it, because there withQuery() is the correct behaviour. If this stops
	// holding, the rule above has been applied too widely.
	listIncludes := 0
	for _, name := range sortedKeys(files) {
		if !strings.HasPrefix(name, "templates/list") {
			continue
		}
		for _, m := range cardInclude.FindAllStringSubmatch(files[name], -1) {
			if !strings.Contains(m[3], "tagBaseUrl") {
				listIncludes++
			}
		}
	}
	if listIncludes == 0 {
		t.Error("no list page includes a card partial without tagBaseUrl any more. " +
			"On a list page withQuery() is correct — it builds the page's own filter — " +
			"so this rule has been applied where it does not belong.")
	}
}

// -- colour contrast on solid buttons ------------------------------------------

// lowContrastOnWhiteText lists the Tailwind background utilities that fail the
// WCAG AA 4.5:1 minimum against white text, with the measured ratio.
//
// The app's own primary button (partials/form/createFormSubmit.tpl) is
// `text-white bg-amber-700`, which passes at 5.05:1. Three buttons had drifted
// to `bg-amber-600` — /account, /admin/users and the template bundle "Apply"
// control — and /admin/users is where axe caught it, the first time that page
// entered the accessibility sweep at all (Batch 13 added it).
//
// The shades are spelled without their `bg-` prefix on purpose. Tailwind v4
// scans the whole project for utility candidates, so writing the full class name
// here would make the build emit CSS for a colour no template uses — this file
// would generate the very rule it exists to forbid.
var lowContrastOnWhiteText = map[string]string{
	"amber-600":  "3.19:1 against white; use amber-700 (5.05:1), the app's own primary button",
	"amber-500":  "2.15:1 against white",
	"amber-400":  "1.62:1 against white",
	"yellow-500": "1.72:1 against white",
	"yellow-400": "1.35:1 against white",
	"lime-500":   "1.86:1 against white",
}

// classAttr matches a literal class attribute value.
var classAttr = regexp.MustCompile(`class="([^"]*)"`)

// TestNoWhiteTextOnALowContrastBackground is the static half of a colour-contrast
// check. axe measures the rendered page and is the authority, but it only sees
// pages that are in the sweep and states that a run happens to reach — which is
// exactly how a 3.19:1 submit button sat on /admin/users unnoticed. This reads
// every template, so a page nobody audits is still covered.
func TestNoWhiteTextOnALowContrastBackground(t *testing.T) {
	files := templateFiles(t)

	checked := 0
	for _, name := range sortedKeys(files) {
		body := files[name]
		for _, m := range classAttr.FindAllStringSubmatchIndex(body, -1) {
			classes := body[m[2]:m[3]]
			if !strings.Contains(classes, "text-white") {
				continue
			}
			checked++
			for shade, why := range lowContrastOnWhiteText {
				// Word-boundary match: a `hover:` or `focus:` variant is not the
				// resting colour axe measures, and a `/50` opacity modifier is a
				// different colour again.
				if !hasClass(classes, "bg-"+shade) {
					continue
				}
				t.Errorf("%s:%d: `text-white` on `bg-%s` — %s.\n"+
					"\tWCAG AA needs 4.5:1 for body-sized text. The app's primary button\n"+
					"\t(partials/form/createFormSubmit.tpl) pairs text-white with amber-700.",
					name, lineOf(body, m[0]), shade, why)
			}
		}
	}

	// Positive control: white text has to exist somewhere, or this guard is
	// scanning nothing.
	if checked < 10 {
		t.Errorf("only %d class attributes carry text-white; expected dozens. "+
			"The scan is not finding the markup it claims to guard.", checked)
	}
}

// hasClass reports whether the space-separated class list contains name as a
// whole token — not as part of `hover:name`, `focus:name` or `name/50`.
func hasClass(classes, name string) bool {
	for _, tok := range strings.Fields(classes) {
		if tok == name {
			return true
		}
	}
	return false
}

// nativeConfirmCall matches a call to the browser's own confirm(), in a template
// or in a frontend module. It deliberately does not match `$store.confirmDialog`
// or `askToConfirm(`, which are the replacements, and the leading boundary keeps
// it off `confirmDialog(` and off the entity picker's own `confirm()` commit
// method (`$store.entityPicker.confirm()`), which is not a dialog at all.
// The trailing `[^)\s]` requires a non-empty first argument. A confirmation
// always carries a message, so this distinguishes a real call from a zero-argument
// method *definition* — `$store.entityPicker.confirm()` is the picker's own commit
// method and not a dialog at all, and it must not be swept up here.
var nativeConfirmCall = regexp.MustCompile(`(^|[^.\w$])(window\.)?confirm\s*\(\s*[^)\s]`)

// blockComment and fullLineComment strip the places a guard like this otherwise
// trips over its own subject matter: a doc comment that *describes* confirm() is
// not a call to it, and this file's own explanatory prose is the single most
// likely thing to fail the sweep first.
// `pongoComment` is declared above and reused here: it is single-line by
// construction, which TestNoMultiLinePongoComment is what makes safe to assume.
var (
	blockComment    = regexp.MustCompile(`(?s)/\*.*?\*/`)
	fullLineComment = regexp.MustCompile(`(?m)^[ \t]*(//|\*).*$`)
	htmlComment     = regexp.MustCompile(`(?s)<!--.*?-->`)
)

// withoutComments blanks comments while preserving byte offsets, so lineOf still
// reports the right line for whatever survives.
//
// Only *whole-line* `//` comments are stripped, never trailing ones: a line such
// as `const u = 'http://x'; confirm(msg)` would lose its real call to a naive
// strip-to-end-of-line, turning a false positive into a false negative.
func withoutComments(body string) string {
	blank := func(s string) string {
		out := []byte(s)
		for i := range out {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
		return string(out)
	}
	for _, re := range []*regexp.Regexp{pongoComment, htmlComment, blockComment, fullLineComment} {
		body = re.ReplaceAllStringFunc(body, blank)
	}
	return body
}

// srcFiles returns every .js/.ts under src/, keyed by module-root-relative path.
func srcFiles(t *testing.T) map[string]string {
	t.Helper()

	root := moduleRoot(t)
	dir := filepath.Join(root, "src")
	out := map[string]string{}

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".js") && !strings.HasSuffix(path, ".ts") {
			return nil
		}
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		out[filepath.ToSlash(rel)] = string(body)
		return nil
	})
	if err != nil {
		t.Fatalf("walking src/: %v", err)
	}
	if len(out) < 50 {
		t.Fatalf("found only %d source files; the walk is broken, not the sources", len(out))
	}
	return out
}

// TestNoNativeConfirm pins the replacement of window.confirm by the in-app
// alertdialog (docs/deferred-work.md item 1).
//
// window.confirm is modal to the *browser*, not to the page. That bought three
// properties for free — it cannot be missed, it cannot be dismissed by accident,
// and it traps focus by construction — and the in-app dialog re-earns all three.
// The reason to forbid the native call rather than merely to have replaced it is
// that the replacement is invisible at a call site: `confirm(msg)` keeps working
// forever, so the next destructive feature would reintroduce the class without
// anything noticing.
//
// This is a Go guard and not a Playwright spec because ci.yml runs `go test` and,
// as of the e2e-browser job, only the accessibility and regression directories of
// the browser suite.
func TestNoNativeConfirm(t *testing.T) {
	// The dialog's own implementation is the one place allowed to name it — and
	// today does not, but an explicit allow-list is cheaper than a guard that
	// fires on its own replacement the first time someone adds a fallback.
	allowed := map[string]bool{
		"src/components/confirmDialog.js":      true,
		"src/components/confirmDialog.test.ts": true,
	}

	scanned := 0
	for _, group := range []map[string]string{templateFiles(t), srcFiles(t)} {
		for _, name := range sortedKeys(group) {
			if allowed[name] {
				continue
			}
			scanned++
			body := withoutComments(group[name])
			for _, m := range nativeConfirmCall.FindAllStringSubmatchIndex(body, -1) {
				t.Errorf("%s:%d: native confirm() — use $store.confirmDialog.ask() in a template,\n"+
					"\tor askToConfirm() from src/components/confirmDialog.js in a module.\n"+
					"\tA native confirm cannot be styled, cannot be labelled as an alertdialog,\n"+
					"\tand returns synchronously, which is the shape the async replacement broke.",
					name, lineOf(body, m[0]))
			}
		}
	}

	if scanned < 100 {
		t.Errorf("only %d files scanned; the sweep is not reading the tree it claims to guard", scanned)
	}
}

// TestNoInlineOnsubmitConfirm forbids the other spelling of the same thing.
//
// Four sites used `onsubmit="return confirm(...)"`, which is not merely a native
// confirm: it is a native confirm that no component can see, so the selection
// guards, the message placeholders and the focus handling all skip it. The
// regexp above already matches these, but a dedicated failure names the actual
// mistake instead of pointing at the word `confirm`.
func TestNoInlineOnsubmitConfirm(t *testing.T) {
	files := templateFiles(t)

	for _, name := range sortedKeys(files) {
		body := withoutComments(files[name])
		for _, m := range inlineOnsubmit.FindAllStringSubmatchIndex(body, -1) {
			t.Errorf("%s:%d: inline onsubmit= handler — route the form through\n"+
				"\tx-data=\"confirmAction({message: '…'})\" x-bind=\"events\" instead.\n"+
				"\tAn inline handler is invisible to the component that owns the\n"+
				"\tconfirmation, so it cannot resolve {count}/{s}/{winner} and cannot\n"+
				"\tbe blocked by the empty-selection guard.",
				name, lineOf(body, m[0]))
		}
	}
}

var inlineOnsubmit = regexp.MustCompile(`\sonsubmit\s*=`)

// TestConfirmDialogIsReachableFromEveryPage pins the two wiring facts the whole
// replacement rests on: the partial is in the universal overlay layer, and the
// store that drives it is registered.
//
// Without either, askToConfirm() fails closed and every destructive control in
// the app silently stops working — which is the safe direction, but is still the
// entire feature being off.
func TestConfirmDialogIsReachableFromEveryPage(t *testing.T) {
	files := templateFiles(t)

	base, ok := files["templates/layouts/base.tpl"]
	if !ok {
		t.Fatal("templates/layouts/base.tpl not found; every page layout extends it")
	}
	if !strings.Contains(base, "partials/confirmDialog.tpl") {
		t.Error("templates/layouts/base.tpl does not include partials/confirmDialog.tpl.\n" +
			"\tIt belongs inside the .overlays container, which sits at z-index 41 —\n" +
			"\tabove the .header stacking context at 40 — and is present on every page.")
	}

	dialog, ok := files["templates/partials/confirmDialog.tpl"]
	if !ok {
		t.Fatal("templates/partials/confirmDialog.tpl not found")
	}
	for _, needle := range []string{`role="alertdialog"`, `aria-modal="true"`, "aria-labelledby", "aria-describedby"} {
		if !strings.Contains(dialog, needle) {
			t.Errorf("partials/confirmDialog.tpl is missing %s.\n"+
				"\tA confirm is an alertdialog, not a dialog: it interrupts to demand a\n"+
				"\tdecision, and the description has to be announced with it.", needle)
		}
	}
	// The backdrop must not dismiss. That is the one accident window.confirm
	// could not have, and re-earning it is the point of the replacement.
	if strings.Contains(withoutComments(dialog), "click.self=") {
		t.Error("partials/confirmDialog.tpl has a @click.self dismissal on its backdrop.\n" +
			"\tA stray click must not discard a destructive confirm.")
	}

	main, err := os.ReadFile(filepath.Join(moduleRoot(t), "src", "main.js"))
	if err != nil {
		t.Fatalf("reading src/main.js: %v", err)
	}
	if !strings.Contains(string(main), "registerConfirmDialogStore(Alpine)") {
		t.Error("src/main.js does not call registerConfirmDialogStore(Alpine); " +
			"the dialog's markup would render but never open.")
	}
}

// dateFilterLayout captures the Go time layout passed to pongo2's `date` filter.
var dateFilterLayout = regexp.MustCompile(`\|\s*date\s*:\s*"([^"]*)"`)

// TestMachineReadableDatesCarryAZone is the real guard behind finding 118, and
// behind the observation in docs/deferred-work.md item 8 that the runtime guard
// for it is vacuous.
//
// The defect: `date:"2006-01-02T15:04:05Z"` looks like RFC3339 and is not. In a Go
// layout a trailing bare `Z` is a *literal character*, while `Z07:00` is the zone.
// pongo2's date filter is a bare t.Format(param) with no zone conversion, so the
// buggy layout printed the value's local wall clock and stapled a "Z" onto it —
// every timestamp then parsed as that many hours into the future.
//
// Why this is a source sweep and not (only) a rendered-output assertion:
//
//   - On a UTC host the buggy and fixed layouts emit *identical bytes*, so no
//     assertion over rendered output can tell them apart. CI hosts run UTC. The
//     existing runtime test therefore passed against the unfixed code on exactly
//     the machine that matters, and only ever failed on its author's UTC+3 laptop.
//   - It was also one-sided: `drift > time.Minute` cannot fire west of Greenwich,
//     where the bogus stamp resolves into the past instead.
//   - And it can skip. The dashboard provider fans out over five goroutines, and
//     the api_tests DSN uses `cache=private`, which hands each pooled connection a
//     brand-new empty in-memory database — so the activity feed often renders no
//     <time> at all and the test t.Skips, which CI scores as a pass.
//
// The layout is decidable from the template source, on any host, with no database
// and no rendering. That is the whole reason to assert it here.
func TestMachineReadableDatesCarryAZone(t *testing.T) {
	files := templateFiles(t)

	checked := 0
	for _, name := range sortedKeys(files) {
		body := withoutComments(files[name])
		for _, m := range dateFilterLayout.FindAllStringSubmatchIndex(body, -1) {
			layout := body[m[2]:m[3]]
			checked++

			// Only layouts that carry a time-of-day claim to identify an instant. A
			// date-only layout ("Jan 02, 2006") is a human-facing label and has no
			// zone to get wrong.
			if !strings.Contains(layout, "15:04") {
				continue
			}
			// A trailing literal Z with no offset directive anywhere in the layout.
			if !strings.HasSuffix(layout, "Z") {
				continue
			}
			if strings.Contains(layout, "Z07:00") || strings.Contains(layout, "Z0700") {
				continue
			}

			t.Errorf("%s:%d: date layout %q ends in a bare literal Z.\n"+
				"\tIn a Go layout `Z` is the character Z; the zone directive is `Z07:00`.\n"+
				"\tThis stamps local wall-clock time and calls it UTC, so the value parses\n"+
				"\tas the UTC offset into the future. Use \"2006-01-02T15:04:05Z07:00\".",
				name, lineOf(body, m[0]), layout)
		}
	}

	// Positive control: this guard is worthless if the walk finds no date filters.
	if checked < 10 {
		t.Errorf("only %d `|date:` layouts found across templates/; expected dozens. "+
			"The scan is not reading the templates it claims to guard.", checked)
	}
}

// -- deferred-work item 8 -----------------------------------------------------

// templateOpenTag returns the `<name …>` open tag in body that contains marker,
// or "" if there is none.
//
// The scan is quote-aware, for the reason findOpenTag in
// server/api_tests/ws5_keyboard_names_headings_test.go documents: a `[^>]*`
// regex stops inside an attribute value the moment an Alpine expression contains
// a `>` (`:aria-expanded="results.length > 0"`), so every attribute after it
// looks absent. An absence check that truncates reports a defect that is not
// there, which is the worse of the two failure modes.
func templateOpenTag(body, name, marker string) string {
	needle := "<" + name
	for i := 0; i < len(body); {
		start := strings.Index(body[i:], needle)
		if start < 0 {
			return ""
		}
		start += i

		end, quote := -1, byte(0)
		for j := start + len(needle); j < len(body) && end < 0; j++ {
			switch c := body[j]; {
			case quote != 0:
				if c == quote {
					quote = 0
				}
			case c == '"' || c == '\'':
				quote = c
			case c == '>':
				end = j
			}
		}
		if end < 0 {
			return ""
		}
		if tag := body[start : end+1]; strings.Contains(tag, marker) {
			return tag
		}
		i = end + 1
	}
	return ""
}

// soleActionControls are the controls whose *only* affordance was a hover
// reveal. Each is identified by its handler, which is what makes it that
// control; matching on the class list would match the thing under test.
var soleActionControls = []struct {
	file, marker, what string
}{
	{
		"templates/partials/blockEditor.tpl",
		`@click="removeResource(resId)"`,
		"the block editor's gallery remove",
	},
	{
		"templates/partials/lightbox.tpl",
		`$store.lightbox.editingSlotIndex = idx`,
		"the lightbox quick-tag Add",
	},
	{
		"templates/partials/lightbox.tpl",
		`$store.lightbox.clearQuickTagSlot(idx)`,
		"the lightbox quick-tag Clear",
	},
}

// TestSoleActionControlsAreNotHoverOnly is deferred-work item 8's third finding,
// and the sequel to finding 99 (server/api_tests/ws5_keyboard_names_headings_test.go,
// TestSavedQueryDelete_IsNotHoverOnly). These three buttons were `opacity-0
// group-hover:opacity-100`: a touch device has no hover, and no :focus-visible
// before the tap, so the only way to remove a gallery item or to configure a
// quick-tag slot was drawn at zero opacity.
//
// The eleven other users of that idiom, all in displayResource.tpl, are
// deliberately outside this table: they are copy-to-clipboard buttons sitting
// beside the value they copy, so a hidden one hides nothing and blocks nothing.
// The rule being pinned is "not the sole path", not "never hover-revealed".
func TestSoleActionControlsAreNotHoverOnly(t *testing.T) {
	files := templateFiles(t)

	for _, c := range soleActionControls {
		body, ok := files[c.file]
		if !ok {
			t.Fatalf("%s not found", c.file)
		}
		tag := templateOpenTag(body, "button", c.marker)
		if tag == "" {
			t.Fatalf("%s: no <button> carrying %s — this test measured nothing", c.file, c.marker)
		}
		flat := strings.Join(strings.Fields(tag), " ")

		if strings.Contains(tag, "opacity-0") {
			t.Errorf("%s:%d: %s is opacity-0 until hover, so on touch it does not exist.\ntag: %s",
				c.file, lineOf(body, strings.Index(body, tag)), c.what, flat)
		}
		if !strings.Contains(tag, "touch-reachable") {
			t.Errorf("%s:%d: %s does not carry .touch-reachable, the class that pins its resting paint\n"+
				"\t(public/index.css). Three call sites share it so they cannot drift apart again.\ntag: %s",
				c.file, lineOf(body, strings.Index(body, tag)), c.what, flat)
		}
		if !strings.Contains(tag, "aria-label") {
			t.Errorf("%s:%d: %s lost its accessible name.\ntag: %s",
				c.file, lineOf(body, strings.Index(body, tag)), c.what, flat)
		}
	}

	css, err := os.ReadFile(filepath.Join(moduleRoot(t), "public", "index.css"))
	if err != nil {
		t.Fatalf("reading public/index.css: %v", err)
	}
	if !strings.Contains(string(css), ".touch-reachable") {
		t.Error("public/index.css does not define .touch-reachable, so the class the three call sites " +
			"name resolves to nothing and they are unstyled, not always-painted.")
	}
}

// TestTextareasKeepTheirNativeRole is deferred-work item 8's second finding.
//
// Three mention textareas declared role="combobox". That override replaces the
// implicit textbox role and takes aria-multiline="true" with it, so a multi-line
// field stopped announcing as one. Adding aria-multiline back is not available:
// ARIA 1.2 has no multiline combobox, the attribute is not in axe's allowed set
// for role=combobox, and aria-allowed-attr is tagged wcag2a/wcag412 with impact
// critical — the fix would have been a worse violation than the defect.
//
// So the rule is the general one: never override a textarea's role. A textbox
// already allows aria-activedescendant, aria-autocomplete and aria-multiline,
// and aria-controls, aria-owns and aria-haspopup are global, which is every
// attribute the mention pattern needs.
//
// aria-expanded is checked in the same sweep because it is the attribute that
// cannot come along: it is neither global nor allowed on a textbox, so leaving
// it behind on a de-roled textarea trades one aria-allowed-attr failure for
// another. The state it carried is announced by mentionTextarea.js's live
// region instead.
func TestTextareasKeepTheirNativeRole(t *testing.T) {
	files := templateFiles(t)

	seen := 0
	for _, name := range sortedKeys(files) {
		body := withoutComments(files[name])
		for i := 0; i < len(body); {
			start := strings.Index(body[i:], "<textarea")
			if start < 0 {
				break
			}
			start += i
			tag := templateOpenTag(body[start:], "textarea", "")
			if tag == "" {
				break
			}
			i = start + len(tag)
			seen++

			flat := strings.Join(strings.Fields(tag), " ")
			if strings.Contains(tag, "role=") {
				t.Errorf("%s:%d: a <textarea> overrides its role. That discards the implicit\n"+
					"\taria-multiline=\"true\", and no ARIA 1.2 role restores it — role=combobox\n"+
					"\trejects aria-multiline outright.\ntag: %s", name, lineOf(body, start), flat)
			}
			if strings.Contains(tag, "aria-expanded") {
				t.Errorf("%s:%d: a <textarea> declares aria-expanded, which is not global and not\n"+
					"\tallowed on role=textbox (aria-allowed-attr, wcag2a/wcag412). Announce the\n"+
					"\topen/closed state through a live region instead.\ntag: %s", name, lineOf(body, start), flat)
			}
		}
	}

	// Positive control: the sweep has to be finding textareas at all.
	if seen < 10 {
		t.Errorf("only %d <textarea> open tags found; the scan is broken, not the templates", seen)
	}
}

// TestMentionOptionsAreOutOfTheTabOrder is deferred-work item 8's first finding.
//
// mentionDropdown.tpl renders each suggestion as a <button role="option">. A
// button is natively focusable, so without tabindex="-1" the options sat in the
// tab order — while the owning textarea drives the list with
// aria-activedescendant and never yields focus. Tab therefore walked into the
// listbox and DOM focus diverged from the option ARIA reported as active. axe
// does not catch this, which is why it survived a sweep that keeps KNOWN_ISSUES
// empty.
//
// The rule is scoped to <button>, not to every role="option": templatePreviewPane.tpl
// and blockEditor.tpl's block picker are roving-tabindex listboxes on <li>, where
// tabindex="0" on the active option is correct. Requiring the attribute to be
// *declared* rather than to equal "-1" keeps a future roving <button> listbox
// legal while still failing the omission, which is the defect.
func TestMentionOptionsAreOutOfTheTabOrder(t *testing.T) {
	files := templateFiles(t)

	seen := 0
	for _, name := range sortedKeys(files) {
		body := withoutComments(files[name])
		for i := 0; i < len(body); {
			start := strings.Index(body[i:], "<button")
			if start < 0 {
				break
			}
			start += i
			tag := templateOpenTag(body[start:], "button", "")
			if tag == "" {
				break
			}
			i = start + len(tag)

			if !strings.Contains(tag, `role="option"`) {
				continue
			}
			seen++
			if !strings.Contains(tag, "tabindex") {
				t.Errorf("%s:%d: a <button role=\"option\"> declares no tabindex, so it is in the tab\n"+
					"\torder. Under an aria-activedescendant listbox that lets DOM focus and\n"+
					"\taria-activedescendant point at different things. adminExport.tpl and\n"+
					"\tadminImport.tpl are the house pattern: tabindex=\"-1\".\ntag: %s",
					name, lineOf(body, start), strings.Join(strings.Fields(tag), " "))
			}
		}
	}

	if seen == 0 {
		t.Error("no <button role=\"option\"> in the tree; partials/form/mentionDropdown.tpl renders one, " +
			"so the scan is broken rather than the templates being clean.")
	}
}
