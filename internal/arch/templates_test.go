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
