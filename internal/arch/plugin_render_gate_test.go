package arch

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Plugin host functions (mah.db.*) run against the unscoped DB handle, so a
// group-confined principal that reaches plugin code can read and write outside
// its subtree. server/authz_policy.go enforces that for URL paths under
// /v1/plugins/ and /plugins/, but plugin Lua also runs from inside template and
// API rendering, which is not a plugin path: {% plugin_slot %},
// {% process_shortcodes %} and {% custom_css %} execute on ordinary content
// pages, and the JSON, MRQL, deferred-render and template-preview handlers each
// build a renderer of their own.
//
// That is how the hole appeared in the first place: the rule was enforced in one
// place and the other seams were added later without it. A confined user could
// write [plugin:...] into a description of an entity it owns and trigger plugin
// Lua simply by viewing it.
//
// This test is the guard. Every seam that invokes plugin Lua must gate on
// auth.PluginCodeAllowed, and a file may not gain a second invocation without a
// second gate. It is deliberately a source-level check rather than a behavioural
// one: the failure mode being prevented is a NEW seam that forgets the rule, and
// no behavioural test covers a seam nobody has written yet.
func TestPluginRenderSeamsAreGated(t *testing.T) {
	root := moduleRoot(t)
	serverDir := filepath.Join(root, "server")

	// Calls that cause plugin Lua to execute during a render.
	invocations := []string{"RenderShortcode(", "RenderSlot("}
	const gate = "auth.PluginCodeAllowed("

	var checked int

	err := filepath.Walk(serverDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		srcBytes, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		// Strip comments before counting, so prose mentioning the gate (these
		// seams all carry a comment explaining why they gate) cannot stand in for
		// an actual call, and deleting a real condition while leaving its comment
		// behind still fails.
		src := stripGoComments(string(srcBytes))

		calls := 0
		for _, inv := range invocations {
			calls += strings.Count(src, inv)
		}
		if calls == 0 {
			return nil
		}
		checked++

		gates := strings.Count(src, gate)
		if gates < calls {
			rel, _ := filepath.Rel(root, path)
			t.Errorf("%s invokes plugin Lua %d time(s) but gates only %d of them.\n"+
				"Every RenderShortcode/RenderSlot seam must be guarded by auth.PluginCodeAllowed(reqCtx),\n"+
				"or a group-confined principal reaches plugin code running on the unscoped DB handle.",
				rel, calls, gates)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking server/: %v", err)
	}

	// If this drops to zero the test has silently stopped testing anything,
	// which is the same failure it exists to prevent.
	if checked == 0 {
		t.Fatal("found no plugin render seams under server/; the guard is no longer checking anything")
	}
}

// stripGoComments removes comments from Go source using the parser, so a
// substring check cannot be satisfied by a comment. It deliberately keeps string
// literals: a seam is identified by the call it makes, not by its prose.
func stripGoComments(src string) string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "src.go", src, parser.ParseComments)
	if err != nil {
		// Unparseable source is not this test's problem; fall back to the raw
		// text rather than masking a compile error as a gating failure.
		return src
	}
	out := []byte(src)
	base := fset.File(file.Pos()).Base()
	for _, group := range file.Comments {
		start := int(group.Pos()) - base
		end := int(group.End()) - base
		if start < 0 || end > len(out) || start >= end {
			continue
		}
		for i := start; i < end; i++ {
			if out[i] != '\n' {
				out[i] = ' '
			}
		}
	}
	return string(out)
}
