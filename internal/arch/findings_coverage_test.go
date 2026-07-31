package arch

// Phase 3 of the 2026-07-29 UI bug hunt (docs/todo.md): "every confirmed finding
// has a regressions/ spec or a Go test naming it."
//
// That line sat in the plan's final-verification checklist as something a person
// would eyeball at the end. This makes it a test, for the reason the whole Phase
// 3 exists: a promise nobody checks is a comment. Run against the ledger as
// written, it found three findings recorded as FIXED with no test anywhere — and
// one of those three turned out to have no *fix* either (60/65, the relation
// category-mismatch message, which docs/todo.md said was rewritten in Batch 12
// and was not).
//
// The audit reads docs/todo.md rather than a hand-maintained list, so it cannot
// drift from the ledger it is auditing.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// ledgerRow matches a verification-ledger line: `| 78 | low | ux | … |`.
var ledgerRow = regexp.MustCompile(`^\|\s*(\d{1,3})\s*\|\s*(high|med|low)\s*\|`)

// findingProse matches "finding 78", "findings 16/92", "Findings 26, 27 and 37".
var findingProse = regexp.MustCompile(`[Ff]indings?\s+((?:\d{1,3}\s*(?:[/,]|and)?\s*)+)`)

// findingHeader matches the house header-list form used at the top of the
// per-workstream test files:
//
//	//	104 — the export page prints a raw Go duration
//	 *  44/52 — the tree root picker is capped at 50
var findingHeader = regexp.MustCompile(`(?m)^\s*(?://|\*|#)?\s*(\d{1,3}(?:\s*[/,]\s*\d{1,3})*)\s*[—-]\s`)

var digits = regexp.MustCompile(`\d{1,3}`)

// TestEveryFixedFindingIsNamedByATest reads the ledger in docs/todo.md and
// requires each finding carrying a **FIXED** note to be named by at least one
// test file.
//
// It checks that a finding is *named*, not that it is covered — no static check
// can tell those apart, and pretending otherwise would be the "guard that
// advertises coverage it does not have" failure this batch is about. What it
// does buy is that a finding cannot be marked fixed and then have its test
// deleted, renamed out of recognition, or never written; and, as it happens, it
// is also how a fix that was recorded but never made was found.
func TestEveryFixedFindingIsNamedByATest(t *testing.T) {
	root := moduleRoot(t)

	fixed := fixedFindings(t, root)
	if len(fixed) < 100 {
		t.Fatalf("found only %d findings marked FIXED in docs/todo.md; the ledger parse is "+
			"broken, not the coverage", len(fixed))
	}

	named := findingsNamedInTests(t, root)
	if len(named) < 100 {
		t.Fatalf("found only %d finding references across the test suite; the scan is "+
			"broken, not the coverage", len(named))
	}

	var missing []int
	for _, n := range fixed {
		if !named[n] {
			missing = append(missing, n)
		}
	}
	sort.Ints(missing)

	if len(missing) > 0 {
		t.Errorf("%d findings are recorded as FIXED in docs/todo.md and named by no test: %v\n"+
			"\tEach needs a Go test or a spec under e2e/tests/regressions/ that names it —\n"+
			"\t\"finding N\" in prose, or the house header form \"N — one-line description\"\n"+
			"\tat the top of the file. If a finding genuinely cannot be tested, say so in\n"+
			"\tthe file that comes closest and name the number there, so the reason is\n"+
			"\twritten down where the next person will look.", len(missing), missing)
	}
}

// TestEveryRejectedFindingIsPinnedByATest is the same audit for the other
// direction, and it is the one Batch 14 added.
//
// A rejection is a claim — "the reported behaviour is not a defect" — and it can
// stop being true without anyone touching the ledger. The campaign's standing
// rule is that a rejected finding gets a test as well, pinning the rejection so
// that it cannot quietly become wrong; such a test passes in *both* directions
// for the assertions that describe the probe's mistake, and fails in one for the
// assertion that describes the behaviour the report claimed was broken.
//
// Run against the ledger as written this found one gap: finding 79 (the grid
// selection checkboxes desynchronising from the bulk-selection store) was
// rejected in Batch 4 and named by nothing. The rejection rested entirely on a
// paragraph of prose in docs/todo.md, so a real desynchronisation would have had
// to be re-found from scratch.
func TestEveryRejectedFindingIsPinnedByATest(t *testing.T) {
	root := moduleRoot(t)

	rejected := ledgerFindings(t, root, func(line string) bool {
		return (strings.Contains(line, "REJECTED") || strings.Contains(line, "NOT FIXED")) &&
			!strings.Contains(line, "**FIXED")
	})
	if len(rejected) < 5 {
		t.Fatalf("found only %d rejected findings in docs/todo.md; the ledger parse is "+
			"broken, not the coverage", len(rejected))
	}

	named := findingsNamedInTests(t, root)

	var missing []int
	for _, n := range rejected {
		if !named[n] {
			missing = append(missing, n)
		}
	}
	sort.Ints(missing)

	if len(missing) > 0 {
		t.Errorf("%d findings are recorded as REJECTED in docs/todo.md and pinned by no test: %v\n"+
			"\tA rejection is a claim that can stop being true. Each needs a test that\n"+
			"\tasserts the behaviour the report said was broken, so the rejection cannot\n"+
			"\tquietly become wrong — and, where the rejection is a probe artefact,\n"+
			"\treproduces the artefact so the next reader of that evidence does not\n"+
			"\tre-open the finding.", len(missing), missing)
	}
}

// fixedFindings returns the ledger row numbers whose status carries a FIXED note.
func fixedFindings(t *testing.T, root string) []int {
	t.Helper()
	return ledgerFindings(t, root, func(line string) bool { return strings.Contains(line, "**FIXED") })
}

// ledgerFindings returns the row numbers of every verification-ledger line the
// predicate accepts.
func ledgerFindings(t *testing.T, root string, keep func(line string) bool) []int {
	t.Helper()

	body, err := os.ReadFile(filepath.Join(root, "docs", "todo.md"))
	if err != nil {
		t.Fatalf("reading docs/todo.md: %v", err)
	}

	var out []int
	for _, line := range strings.Split(string(body), "\n") {
		m := ledgerRow.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		if !keep(line) {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(m[1], "%d", &n); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// findingsNamedInTests scans every test file in the repository and returns the
// set of finding numbers referenced.
func findingsNamedInTests(t *testing.T, root string) map[int]bool {
	t.Helper()

	named := map[int]bool{}
	files := 0

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name == "node_modules" || name == ".git" || name == "docs-site" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		base := info.Name()
		if !strings.HasSuffix(base, "_test.go") &&
			!strings.HasSuffix(base, ".spec.ts") &&
			!strings.HasSuffix(base, ".test.ts") {
			return nil
		}
		// Never let the audit satisfy itself. This file is a _test.go and its own
		// prose names the findings it has caught, so without this skip the way to
		// close a coverage gap is to write about the gap here — the guard would
		// certify a finding as covered on the strength of its own commentary. It
		// went green exactly that way once, before this line existed.
		if base == "findings_coverage_test.go" {
			return nil
		}
		src, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		files++
		text := string(src)
		for _, m := range findingProse.FindAllStringSubmatch(text, -1) {
			for _, d := range digits.FindAllString(m[1], -1) {
				var n int
				if _, err := fmt.Sscanf(d, "%d", &n); err == nil {
					named[n] = true
				}
			}
		}
		for _, m := range findingHeader.FindAllStringSubmatch(text, -1) {
			for _, d := range digits.FindAllString(m[1], -1) {
				var n int
				if _, err := fmt.Sscanf(d, "%d", &n); err == nil {
					named[n] = true
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repository: %v", err)
	}
	if files < 500 {
		t.Fatalf("found only %d test files; the walk is broken, not the coverage", files)
	}
	return named
}
