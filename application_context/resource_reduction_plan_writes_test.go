package application_context

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// The plan document has several independent writers, and everything the plan
// carries beyond its Clusters (the settings the compute ran with, the record of
// the last apply) survives only because each writer decodes the whole plan and
// writes the whole plan back. A new site that built a plan from scratch would drop
// them without any test failing. So every write of the row's "plan" column goes
// through a known site, and adding one means deciding here how it preserves them.
func TestEveryPlanWriteGoesThroughAKnownSite(t *testing.T) {
	known := map[string]string{
		"storeReductionPlan":                "compute's landing: rebuilds the Clusters, carries LastApply over from the plan it replaces",
		"mutatePlan":                        "every bookkeeping write: decodes and re-encodes the whole plan",
		"overrideReductionClusterOnce":      "a review decision: decodes and re-encodes the whole plan",
		"bulkOverrideReductionClustersOnce": "a bulk decision: decodes and re-encodes the whole plan",
		"claimReductionApplyBatch":          "the batch claim: decodes the plan, sets LastApply running, re-encodes",
		"OverwritePlanForTest":              "test seam",
	}

	files, err := filepath.Glob("*.go")
	require.NoError(t, err)

	found := map[string][]string{}
	for _, file := range files {
		if filepath.Ext(file) != ".go" || len(file) > 8 && file[len(file)-8:] == "_test.go" {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		require.NoError(t, err)
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				kv, ok := n.(*ast.KeyValueExpr)
				if !ok {
					return true
				}
				lit, ok := kv.Key.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				if value, err := strconv.Unquote(lit.Value); err == nil && value == "plan" {
					found[fn.Name.Name] = append(found[fn.Name.Name], file)
				}
				return true
			})
		}
	}

	var sites []string
	for name := range found {
		sites = append(sites, name)
	}
	sort.Strings(sites)
	for _, name := range sites {
		_, allowed := known[name]
		assert.True(t, allowed, "%s (%v) writes the plan column and is not a known site", name, found[name])
	}
	for name := range known {
		_, present := found[name]
		assert.True(t, present, "%s is listed as a plan write site but no longer writes the plan; remove it", name)
	}
}

// A record still running past the ceiling is one whose process died or whose batch
// was abandoned, and the page says so.
func TestDescribeLastApplyReadsAnOldRunningRecordAsInterrupted(t *testing.T) {
	assert.Nil(t, describeLastApply(nil))

	fresh := describeLastApply(&models.ReductionLastApply{StartedAt: time.Now().Add(-time.Minute)})
	assert.True(t, fresh.Running)
	assert.False(t, fresh.Interrupted)

	old := describeLastApply(&models.ReductionLastApply{StartedAt: time.Now().Add(-ReductionApplyCeiling - time.Minute)})
	assert.True(t, old.Running)
	assert.True(t, old.Interrupted)

	done := time.Now()
	finished := describeLastApply(&models.ReductionLastApply{StartedAt: time.Now().Add(-3 * ReductionApplyCeiling), FinishedAt: &done})
	assert.False(t, finished.Running)
	assert.False(t, finished.Interrupted, "a finished apply is never interrupted, however old")
}
