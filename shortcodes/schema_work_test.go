package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func schemaCtx(schema string) MetaShortcodeContext {
	return MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: json.RawMessage(`{}`), MetaSchema: schema}
}

func budgeted() context.Context {
	return WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 256 << 10, ParsedSourceBytes: 512 << 10})
}

// A schema whose $defs.node.child refers to node used to overflow the stack, a
// fatal error no recover catches.
func TestMetaSelfReferencingSchemaDoesNotCrash(t *testing.T) {
	schema := `{"type":"object","properties":{"tree":{"$ref":"#/$defs/node"}},
	  "$defs":{"node":{"type":"object","properties":{"name":{"type":"string"},"child":{"$ref":"#/$defs/node"}}}}}`
	for _, in := range []string{`[meta path="tree"]`, `[meta path="tree.child"]`, `[meta path="tree.child.child.name"]`} {
		got, err := ProcessChecked(budgeted(), in, schemaCtx(schema), nil, nil)
		require.NoError(t, err, in)
		assert.Contains(t, got, "<meta-shortcode", in)
	}
	// The same schema on an unbudgeted render (a page).
	got := Process(context.Background(), `[meta path="tree"]`, schemaCtx(schema), nil, nil)
	assert.Contains(t, got, "<meta-shortcode")
	// A direct leaf-reference cycle: a refers to b refers to a.
	cyc := `{"type":"object","properties":{"x":{"$ref":"#/$defs/a"}},"$defs":{"a":{"$ref":"#/$defs/b"},"b":{"$ref":"#/$defs/a"}}}`
	assert.NotPanics(t, func() { Process(context.Background(), `[meta path="x"]`, schemaCtx(cyc), nil, nil) })
}

// fanSchema builds levels of $defs where each level has n properties that all
// reference the next level: n^levels expansions, none cyclic.
func fanSchema(n, levels int, leafDescription string) string {
	defs := map[string]any{}
	for l := 0; l < levels; l++ {
		props := map[string]any{}
		for i := 0; i < n; i++ {
			if l == levels-1 {
				props[fmt.Sprintf("p%d", i)] = map[string]any{"type": "string", "description": leafDescription}
			} else {
				props[fmt.Sprintf("p%d", i)] = map[string]any{"$ref": fmt.Sprintf("#/$defs/l%d", l+1)}
			}
		}
		defs[fmt.Sprintf("l%d", l)] = map[string]any{"type": "object", "properties": props}
	}
	root := map[string]any{"type": "object", "properties": map[string]any{"root": map[string]any{"$ref": "#/$defs/l0"}}, "$defs": defs}
	b, _ := json.Marshal(root)
	return string(b)
}

func TestMetaAcyclicFanOutStopsAtWorkBudget(t *testing.T) {
	schema := fanSchema(20, 5, "d")
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := ProcessChecked(budgeted(), `[meta path="root"]`, schemaCtx(schema), nil, nil)
	runtime.ReadMemStats(&after)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
	assert.Less(t, int64(after.TotalAlloc-before.TotalAlloc), int64(256<<20))

	// A page (no budget) renders without a slice rather than crashing or hanging.
	got := Process(context.Background(), `[meta path="root"]`, schemaCtx(schema), nil, nil)
	assert.Contains(t, got, `data-schema=""`)
}

func TestMetaRepeatedLargeDescriptionStopsAtByteBudget(t *testing.T) {
	// 16 KiB description under three levels of twenty repeated references.
	schema := fanSchema(20, 3, strings.Repeat("x", 16<<10))
	_, err := ProcessChecked(budgeted(), `[meta path="root"]`, schemaCtx(schema), nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
}

func TestMetaOrdinarySchemaSliceUnchanged(t *testing.T) {
	schema := `{"type":"object","properties":{"addr":{"$ref":"#/$defs/addr"}},"$defs":{"addr":{"type":"object","properties":{"city":{"type":"string"}}}}}`
	got, err := ProcessChecked(budgeted(), `[meta path="addr"]`, schemaCtx(schema), nil, nil)
	require.NoError(t, err)
	assert.Contains(t, got, "city")
}

func TestSchemaWorkDepthCapAndContext(t *testing.T) {
	w := newSchemaWork(nil)
	assert.True(t, w.deep(schemaMaxDepth+1))
	assert.Error(t, w.Err())

	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	w = newSchemaWork(cctx)
	for i := 0; i < 200 && w.charge(1); i++ {
	}
	assert.ErrorIs(t, w.Err(), context.Canceled)
}

// A composition fan-out met while walking the path ends before leaf expansion.
func TestMetaCompositionFanOutEndsEarly(t *testing.T) {
	defs := map[string]any{}
	const n, levels = 30, 8
	for l := 0; l < levels; l++ {
		var branches []any
		for i := 0; i < n; i++ {
			if l == levels-1 {
				branches = append(branches, map[string]any{"properties": map[string]any{"p": map[string]any{"type": "string"}}})
			} else {
				branches = append(branches, map[string]any{"$ref": fmt.Sprintf("#/$defs/l%d", l+1)})
			}
		}
		defs[fmt.Sprintf("l%d", l)] = map[string]any{"allOf": branches}
	}
	root := map[string]any{"type": "object", "properties": map[string]any{"root": map[string]any{"$ref": "#/$defs/l0"}}, "$defs": defs}
	b, _ := json.Marshal(root)
	_, err := ProcessChecked(budgeted(), `[meta path="root.p"]`, schemaCtx(string(b)), nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
}

func TestLintMetaSchemaFanOutReportsOneIssue(t *testing.T) {
	// Each of the 20 properties per level is an x-display=url object that
	// references the next level, so effective-schema expansion fans out.
	defs := map[string]any{}
	const n, levels = 20, 5
	for l := 0; l < levels; l++ {
		props := map[string]any{}
		var all []any
		for i := 0; i < n; i++ {
			if l < levels-1 {
				all = append(all, map[string]any{"$ref": fmt.Sprintf("#/$defs/l%d", l+1)})
			}
			props[fmt.Sprintf("p%d", i)] = map[string]any{"type": "string", "description": strings.Repeat("d", 2000)}
		}
		node := map[string]any{"type": "object", "properties": props}
		if len(all) > 0 {
			node["allOf"] = all
		}
		defs[fmt.Sprintf("l%d", l)] = node
	}
	root := map[string]any{
		"type":       "object",
		"properties": map[string]any{"u": map[string]any{"$ref": "#/$defs/l0", "x-display": "url"}},
		"$defs":      defs,
	}
	b, _ := json.Marshal(root)
	issues := LintMetaSchema(string(b), MetaSchemaLintOptions{})
	require.Len(t, issues, 1)
	assert.Equal(t, SeverityError, issues[0].Severity)
	assert.Contains(t, issues[0].Message, "too complex")
}

func TestLintMetaSchemaBytesBudget(t *testing.T) {
	// Many properties whose names are long enough that their warnings alone
	// pass the byte allowance.
	props := map[string]any{}
	for i := 0; i < 3000; i++ {
		props[fmt.Sprintf("bad.key.%d.%s", i, strings.Repeat("k", 400))] = map[string]any{"type": "string"}
	}
	b, _ := json.Marshal(map[string]any{"type": "object", "properties": props})
	issues := LintMetaSchema(string(b), MetaSchemaLintOptions{})
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too complex")
}

func TestLintMetaSchemaCancelled(t *testing.T) {
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	props := map[string]any{}
	for i := 0; i < 500; i++ {
		props[fmt.Sprintf("p%d", i)] = map[string]any{"type": "string"}
	}
	b, _ := json.Marshal(map[string]any{"type": "object", "properties": props})
	issues := LintMetaSchema(string(b), MetaSchemaLintOptions{Context: cctx})
	require.Len(t, issues, 1)
	assert.Contains(t, issues[0].Message, "too complex")
}
