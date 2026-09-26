package arch

// Forced colors (Windows High Contrast, or forced-colors: active in any
// browser) repaints every colour from the user's system palette and drops
// box-shadow entirely. Tailwind's ring utilities are box-shadow, and a focus
// background, border or text colour is repainted to the same system colour as
// everything around it, so a control that suppresses its outline and shows
// focus any of those ways shows no focus at all there (WCAG 2.4.7), for exactly
// the people who asked their system for more contrast.
//
// The outline is the one indicator forced colors keeps, and it repaints even a
// transparent one. So focus is hidden with a transparent outline rather than
// with none: Tailwind v4's hidden-outline utility, which is `outline-style:
// none` normally and a 2px transparent outline under forced colors, or
// `outline: 2px solid transparent` in hand-written CSS. The utility the app
// used for this in Tailwind v3 became a plain `outline-style: none` in v4,
// which is how every control that paired it with a ring lost its indicator.
//
// One suppression stays legitimate: a rule scoped to `:not(:focus-visible)`,
// whose element paints a real outline under `:focus-visible`, so keyboard
// focus is still drawn.
//
// The forbidden class name is assembled rather than spelled out, because
// Tailwind reads every .go file for class names and would emit it from here.

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	noneOutlineClass = "outline-" + "none"
	// The class with any variant prefix (focus:, focus-visible:, dark:…), as a
	// whole token.
	noneOutlineClassPattern = regexp.MustCompile(`(^|[^A-Za-z0-9_-])` + regexp.QuoteMeta(noneOutlineClass) + `($|[^A-Za-z0-9_-])`)
	// A hand-written declaration that removes the outline.
	noneOutlineDeclaration = regexp.MustCompile(`(?i)\boutline\s*:\s*(none|0(px)?)\s*(!important\s*)?([;}"'` + "`" + `]|$)|\boutline-style\s*:\s*none\b|\boutline-width\s*:\s*0(px)?\b`)
)

// focusOutlineRoots are the trees that author markup, class names or CSS the
// application renders: templates, the frontend source and its web components,
// the bundled plugins and their e2e copies, the hand-written stylesheets, and
// the Go and JSON that emit markup.
var focusOutlineRoots = []string{
	"templates", "src", "plugins", "e2e/test-plugins", "public", "index.css",
	"server/template_presets", "shortcodes",
}

var focusOutlineExtensions = map[string]bool{
	".tpl": true, ".html": true, ".js": true, ".ts": true, ".lua": true,
	".css": true, ".json": true, ".go": true,
}

// focusOutlineSkipped reports files that are built output or tests rather
// than authored UI.
func focusOutlineSkipped(rel string) bool {
	switch {
	case strings.HasPrefix(rel, "public/dist/"), rel == "public/tailwind.css":
		return true
	case strings.HasSuffix(rel, "_test.go"), strings.HasSuffix(rel, ".test.ts"), strings.HasSuffix(rel, ".test.js"):
		return true
	}
	return false
}

// hiddenFocusOutlines returns the line and text of every place in body that
// removes an outline in a way forced colors cannot repaint.
func hiddenFocusOutlines(body string) []string {
	var found []string
	lineOf := func(offset int) int { return strings.Count(body[:offset], "\n") + 1 }
	for _, match := range noneOutlineClassPattern.FindAllStringIndex(body, -1) {
		// Report the whole class token, variants included.
		start := strings.LastIndexAny(body[:match[0]+1], " \t\n\"'`<>=") + 1
		end := match[1]
		if next := strings.IndexAny(body[match[0]+1:], " \t\n\"'`<>="); next >= 0 {
			end = match[0] + 1 + next
		}
		found = append(found, lineText(lineOf(match[0]), body[start:end]))
	}
	for _, match := range noneOutlineDeclaration.FindAllStringIndex(body, -1) {
		if scopedToFocusNotVisible(body, match[0]) {
			continue
		}
		found = append(found, lineText(lineOf(match[0]), body[match[0]:match[1]]))
	}
	return found
}

func lineText(line int, text string) string {
	return "line " + strconv.Itoa(line) + ": " + strings.TrimSpace(text)
}

// scopedToFocusNotVisible reports whether the declaration at offset sits in a
// rule every one of whose selectors is limited to :not(:focus-visible).
func scopedToFocusNotVisible(body string, offset int) bool {
	open := strings.LastIndex(body[:offset], "{")
	if open < 0 || strings.Contains(body[open:offset], "}") {
		return false
	}
	start := strings.LastIndexAny(body[:open], "{}") + 1
	selectors := strings.Split(body[start:open], ",")
	for _, selector := range selectors {
		if !strings.Contains(selector, ":not(:focus-visible)") {
			return false
		}
	}
	return len(selectors) > 0
}

func TestFocusIsNeverHiddenFromForcedColors(t *testing.T) {
	root := moduleRoot(t)
	scanned := 0
	var offenders []string
	for _, entry := range focusOutlineRoots {
		start := filepath.Join(root, filepath.FromSlash(entry))
		err := filepath.Walk(start, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			rel, rerr := filepath.Rel(root, path)
			if rerr != nil {
				return rerr
			}
			rel = filepath.ToSlash(rel)
			if info.IsDir() {
				if info.Name() == "node_modules" {
					return filepath.SkipDir
				}
				// A nested checkout (a worktree, a vendored repo) is another
				// branch's files, not this one's.
				if path != start {
					if _, gerr := os.Stat(filepath.Join(path, ".git")); gerr == nil {
						return filepath.SkipDir
					}
				}
				return nil
			}
			if !focusOutlineExtensions[filepath.Ext(path)] || focusOutlineSkipped(rel) {
				return nil
			}
			body, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			scanned++
			for _, hit := range hiddenFocusOutlines(string(body)) {
				offenders = append(offenders, rel+" "+hit)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", entry, err)
		}
	}
	if scanned < 200 {
		t.Fatalf("scanned only %d files; the walk is broken, not the sources", scanned)
	}
	if len(offenders) > 0 {
		t.Errorf("%d places remove the focus outline in a way forced colors cannot repaint, so the control shows no focus there. "+
			"Use the hidden-outline utility (a transparent outline under forced colors) in class names, or "+
			"`outline: 2px solid transparent` in CSS:\n  %s", len(offenders), strings.Join(offenders, "\n  "))
	}
}

func TestHiddenFocusOutlineDetector(t *testing.T) {
	none := noneOutlineClass
	cases := []struct {
		name  string
		body  string
		found int
	}{
		{"a focus variant beside a ring", `<button class="focus:` + none + ` focus:ring-2">`, 1},
		{"a focus-visible variant", `class="focus-visible:` + none + ` focus-visible:ring-2"`, 1},
		{"the bare class", `class="border-0 ` + none + `"`, 1},
		{"a longer utility that only starts the same", `class="` + none + `-ish"`, 0},
		{"the transparent outline", `class="focus:outline-hidden focus:ring-2"`, 0},
		{"a declaration on focus", ".field:focus {\n  outline: none;\n  box-shadow: 0 0 0 2px red;\n}", 1},
		{"a zero outline", ".field:focus{outline:0}", 1},
		{"a none outline style", ".field:focus { outline-style: none; }", 1},
		{"a transparent declaration", ".field:focus { outline: 2px solid transparent; }", 0},
		{"a pointer-only suppression", ".toggle:focus:not(:focus-visible) {\n  outline: none;\n}", 0},
		{"a pointer-only suppression beside another selector", ".a:hover,\n.b:focus:not(:focus-visible) { outline: none; }", 1},
		{"an inline style", `<input style="outline: none">`, 1},
	}
	for _, tc := range cases {
		if got := hiddenFocusOutlines(tc.body); len(got) != tc.found {
			t.Errorf("%s: found %d (%v), want %d", tc.name, len(got), got, tc.found)
		}
	}
}
