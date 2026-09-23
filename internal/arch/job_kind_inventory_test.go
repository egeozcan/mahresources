package arch

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/jobs"
)

// This is the release inventory, not a list inferred from registrations. Adding a
// user-facing submission path must extend this list and prove its adapter in the
// same change. The summary-export Kind joins when its adapter is integrated.
func TestJobKindInventory(t *testing.T) {
	expected := map[string]uint{
		"remote-download":            1,
		"deferred-download":          1,
		"group-export":               1,
		"group-import-parse":         1,
		"group-import-apply":         1,
		"resource-reduction-compute": 1,
		"similarity-recompute":       1,
		"plugin-action":              1, // registered, scheduled, and closure-backed actions
		"plugin-command":             1,
		"plugin-command-import":      1,
	}

	var ctx application_context.MahresourcesContext
	service := jobs.NewService()
	ctx.SetJobService(service)
	registrations := service.Registrations()
	if len(registrations) != len(expected) {
		t.Errorf("registered %d Kinds, inventory has %d; registrations: %+v", len(registrations), len(expected), registrations)
	}
	for _, registration := range registrations {
		kind, version := registration.Definition.Kind, registration.Definition.KindVersion
		wantVersion, listed := expected[kind]
		if !listed || wantVersion != version {
			t.Errorf("unlisted adapter %s v%d", kind, version)
		}
	}
	for kind, version := range expected {
		adapter, present := service.AdapterFor(kind, version)
		if !present {
			t.Errorf("%s v%d has no runtime adapter", kind, version)
			continue
		}
		if !jobs.HasReplayCodec(service, kind, version) {
			t.Errorf("%s v%d has no replay codec", kind, version)
		}
		if _, present := adapter.(jobs.CommandFilterAdapter); !present {
			t.Errorf("%s v%d has no exact command selector", kind, version)
		}
	}
}

// JobOptions.Source is the legacy queue's open extension point. A new source
// would otherwise silently render as a download while having no canonical Kind.
// Keep the source expressions constrained and review every new queue caller.
func TestQueueSubmissionSourcesHaveAJobKind(t *testing.T) {
	root := moduleRoot(t)
	dir := filepath.Join(root, "application_context")
	allowedSources := map[string]bool{
		"pluginCommandJobSource":                    true,
		"pluginImportJobSource":                     true,
		"maintenanceJobSource":                      true,
		"download_queue.JobSourceGroupExport":       true,
		"download_queue.JobSourceGroupImportParse":  true,
		"download_queue.JobSourceGroupImportApply":  true,
		"download_queue.JobSourceResourceReduction": true,
	}
	allowedCallers := map[string]bool{
		"job_export_adapter.go":      true,
		"job_import_adapter.go":      true,
		"job_maintenance_adapter.go": true,
		"job_reduction_adapter.go":   true,
		"job_queue_bridge.go":        true,
		"plugin_command_runtime.go":  true,
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var sources, submissions int
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch n := node.(type) {
			case *ast.CompositeLit:
				selector, ok := n.Type.(*ast.SelectorExpr)
				if !ok || selector.Sel.Name != "JobOptions" {
					break
				}
				for _, field := range n.Elts {
					entry, ok := field.(*ast.KeyValueExpr)
					if !ok || sourceKey(entry.Key) != "Source" {
						continue
					}
					sources++
					if source := sourceKey(entry.Value); !allowedSources[source] {
						t.Errorf("%s: unlisted JobOptions.Source %q", name, source)
					}
				}
			case *ast.CallExpr:
				selector, ok := n.Fun.(*ast.SelectorExpr)
				if !ok {
					break
				}
				switch selector.Sel.Name {
				case "SubmitJob", "SubmitJobWithOptions", "SubmitManagedJob":
					submissions++
					if !allowedCallers[name] {
						t.Errorf("%s: new queue submission requires inventory and canonical publication proof", name)
					}
				}
			}
			return true
		})
	}
	if sources < 12 || submissions < 8 {
		t.Errorf("source inventory stopped matching queue paths: %d source declarations, %d submissions", sources, submissions)
	}
}

func sourceKey(expr ast.Expr) string {
	switch n := expr.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.SelectorExpr:
		return sourceKey(n.X) + "." + n.Sel.Name
	default:
		return "<dynamic>"
	}
}
