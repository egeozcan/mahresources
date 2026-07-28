// Package arch holds architecture tests: rules about how packages may depend on
// each other, enforced by the test suite rather than by convention.
//
// CLAUDE.md describes the intended layering as
//
//	server/ (HTTP) -> application_context/ (business logic) -> models/ (GORM)
//
// That claim used to be false. server/interfaces/ held the contracts that
// application_context implements, so the business layer imported a package
// nested under the HTTP layer, and the two directories depended on each other.
// Those contracts now live in the top-level contracts/ package, and the tests
// below keep it that way. A layering rule nobody checks is a comment.
package arch

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const modulePath = "mahresources"

// directories that hold no Go source we care about, or a second copy of the tree
var skipDirs = map[string]bool{
	".git": true, ".worktrees": true, "node_modules": true,
	"docs-site": true, "e2e": true, "public": true, "files": true,
	"docs": true, "templates": true, "plugins": true, "test-assets": true,
}

// pkgImports maps a package directory (slash-separated, relative to the module
// root, "." for the root package) to the set of intra-module packages it imports.
func pkgImports(t *testing.T) map[string]map[string]bool {
	t.Helper()

	root := moduleRoot(t)
	out := map[string]map[string]bool{}
	fset := token.NewFileSet()

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
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		// ImportsOnly keeps this fast and immune to build-tag variation: a file
		// excluded from a build still declares dependencies we want to police.
		f, perr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if perr != nil {
			return nil // unparseable file is not this test's problem
		}

		rel, rerr := filepath.Rel(root, filepath.Dir(path))
		if rerr != nil {
			return rerr
		}
		dir := filepath.ToSlash(rel)

		for _, spec := range f.Imports {
			imp := strings.Trim(spec.Path.Value, `"`)
			if imp != modulePath && !strings.HasPrefix(imp, modulePath+"/") {
				continue // third-party or stdlib
			}
			if out[dir] == nil {
				out[dir] = map[string]bool{}
			}
			out[dir][imp] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking module: %v", err)
	}
	if len(out) == 0 {
		t.Fatal("found no intra-module imports; the walk is broken, not the architecture")
	}
	return out
}

func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find go.mod above the test directory")
		}
		dir = parent
	}
}

// under reports whether pkgDir is prefix, or nested inside it.
func under(pkgDir, prefix string) bool {
	return pkgDir == prefix || strings.HasPrefix(pkgDir, prefix+"/")
}

// TestServerIsALeaf is the rule that the contracts/ extraction bought us.
//
// server/ is the outermost layer. Nothing beneath it may reach back up. The
// only packages allowed to import it are the composition roots that wire the
// application together (the root main package and cmd/...) and the internal/
// harnesses that boot a real server to benchmark it.
func TestServerIsALeaf(t *testing.T) {
	for dir, imports := range pkgImports(t) {
		if under(dir, "server") || dir == "." || under(dir, "cmd") || under(dir, "internal") {
			continue
		}
		for _, imp := range sorted(imports) {
			if strings.HasPrefix(imp, modulePath+"/server") {
				t.Errorf("%s imports %s\n"+
					"\tOnly composition roots (main, cmd/..., internal/...) may depend on server/.\n"+
					"\tIf a lower layer needs a type or an interface from the HTTP layer, that\n"+
					"\tcontract belongs in contracts/ instead.", dir, imp)
			}
		}
	}
}

// TestModelsAreSelfContained keeps the bottom layer at the bottom. models/ is
// imported by 15 packages; letting it reach upward would make the dependency
// graph a cycle in everything but name.
func TestModelsAreSelfContained(t *testing.T) {
	allowed := func(imp string) bool {
		return imp == modulePath+"/constants" ||
			imp == modulePath+"/models" ||
			strings.HasPrefix(imp, modulePath+"/models/")
	}
	for dir, imports := range pkgImports(t) {
		if !under(dir, "models") {
			continue
		}
		for _, imp := range sorted(imports) {
			if !allowed(imp) {
				t.Errorf("%s imports %s\n"+
					"\tmodels/ may only depend on constants/ and other models/ packages.",
					dir, imp)
			}
		}
	}
}

// TestContractsStayBelowTheLayersThatUseThem guards the package this whole
// refactor created. contracts/ describes what the business layer offers the
// HTTP layer; the moment it imports either of them, it stops being a boundary
// and becomes another knot.
func TestContractsStayBelowTheLayersThatUseThem(t *testing.T) {
	allowed := func(imp string) bool {
		return imp == modulePath+"/constants" ||
			imp == modulePath+"/models" ||
			strings.HasPrefix(imp, modulePath+"/models/")
	}
	for dir, imports := range pkgImports(t) {
		if !under(dir, "contracts") {
			continue
		}
		for _, imp := range sorted(imports) {
			if !allowed(imp) {
				t.Errorf("%s imports %s\n"+
					"\tcontracts/ may only depend on models/ and constants/. It is the\n"+
					"\tboundary between application_context/ and server/, so it cannot\n"+
					"\tdepend on either of them.", dir, imp)
			}
		}
	}
}

// TestGroupioStaysBelowApplicationContext pins the seam the groupio extraction
// created. groupio/ owns group import/export and is consumed by
// application_context through a facade; the moment it reaches back up, the
// extraction has been undone and the facade's whole point — that the service
// receives a *gorm.DB per call rather than capturing context state — is
// available to be bypassed.
func TestGroupioStaysBelowApplicationContext(t *testing.T) {
	forbidden := []string{
		modulePath + "/application_context",
		modulePath + "/server",
		modulePath + "/contracts",
	}
	for dir, imports := range pkgImports(t) {
		if !under(dir, "groupio") {
			continue
		}
		for _, imp := range sorted(imports) {
			for _, bad := range forbidden {
				if imp == bad || strings.HasPrefix(imp, bad+"/") {
					t.Errorf("%s imports %s\n"+
						"\tgroupio/ sits below application_context/ and server/. It may depend on\n"+
						"\tarchive/, models/, download_queue/ and constants/ only.", dir, imp)
				}
			}
		}
	}
}

func sorted(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
