package shortcodes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func metaWithArray(name string, n int) MetaShortcodeContext {
	arr := make([]int, n)
	for i := range arr {
		arr[i] = i
	}
	raw, _ := json.Marshal(map[string]any{name: arr})
	return MetaShortcodeContext{Meta: raw}
}

// Without a budget ProcessChecked is Process: same bytes, no error.
func TestProcessCheckedWithoutBudgetMatchesProcess(t *testing.T) {
	ctx := metaWithArray("v", 5)
	in := `<ul>[each path="v"]<li>[item]</li>[/each]</ul> [property path="Name"] plain`
	want := Process(context.Background(), in, ctx, nil, nil)
	got, err := ProcessChecked(context.Background(), in, ctx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, want, got)
	assert.Contains(t, got, "<li>3</li>")
}

func TestRenderBudgetOutputBytesLiteral(t *testing.T) {
	bctx := WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 100})
	in := strings.Repeat("a", 90) + "[meta path=\"x\"]" + strings.Repeat("b", 90)
	_, err := ProcessChecked(bctx, in, MetaShortcodeContext{}, nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))

	ok, err := ProcessChecked(bctx, strings.Repeat("a", 90), MetaShortcodeContext{}, nil, nil)
	require.NoError(t, err)
	assert.Len(t, ok, 90)
}

func TestRenderBudgetParsedSourceBytes(t *testing.T) {
	bctx := WithRenderBudget(context.Background(), RenderBudget{ParsedSourceBytes: 50})
	_, err := ProcessChecked(bctx, strings.Repeat("x", 51), MetaShortcodeContext{}, nil, nil)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
	_, err = ProcessChecked(bctx, strings.Repeat("x", 50), MetaShortcodeContext{}, nil, nil)
	assert.NoError(t, err)
}

// Parsed source counts every partial parsed during the render, not only the
// top-level template.
func TestRenderBudgetCountsPartialSource(t *testing.T) {
	big := strings.Repeat("p", 400)
	resolver := func(name string) (string, bool) { return big, true }
	bctx := WithPartialResolver(WithRenderBudget(context.Background(), RenderBudget{ParsedSourceBytes: 1000}), resolver)
	in := `[partial name="a"][partial name="a"][partial name="a"]`
	_, err := ProcessChecked(bctx, in, MetaShortcodeContext{}, nil, nil)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded), "3 x 400 bytes of partial source exceed 1000")
}

// Five nested [each] over 100 elements would render 10^10 iterations. The
// budget must stop it quickly and in bounded memory.
func TestRenderBudgetNestedEach(t *testing.T) {
	ctx := metaWithArray("v", 100)
	in := `[each path="v"][each path="v"][each path="v"][each path="v"][each path="v"]x[/each][/each][/each][/each][/each]`
	// 100 elements need limit= raised past the default 100? the default is 100.
	bctx := WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 256 << 10, ParsedSourceBytes: 512 << 10})

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	_, err := ProcessChecked(bctx, in, ctx, nil, nil)
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
	assert.Less(t, elapsed, 5*time.Second)
	assert.Less(t, int64(after.TotalAlloc-before.TotalAlloc), int64(256<<20), "allocation stays bounded")
}

func TestRenderCancelledContextStopsLongEach(t *testing.T) {
	ctx := metaWithArray("v", 100)
	in := `[each path="v"][each path="v"][each path="v"]x[/each][/each][/each]`
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	_, err := ProcessChecked(cctx, in, ctx, nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
	assert.True(t, errors.Is(err, context.Canceled))
	assert.Less(t, time.Since(start), time.Second)
}

// A cancelled context also stops a long parse.
func TestParseTokenLoopChecksContext(t *testing.T) {
	in := strings.Repeat(`[meta path="x"]`, 30000)
	cctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := ProcessChecked(cctx, in, MetaShortcodeContext{}, nil, nil)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
}

// A budgeted parse refuses an adversarial run of tags before retaining the whole
// match collection.
func TestRenderBudgetParseTokenCap(t *testing.T) {
	in := strings.Repeat("[/meta]", 200000) // 1.4 MB would also exceed bytes, so give a roomy byte budget
	bctx := WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 10 << 20, ParsedSourceBytes: 10 << 20})
	_, err := ProcessChecked(bctx, in, MetaShortcodeContext{}, nil, nil)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
}

func TestRenderBudgetAttributeCap(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[meta ")
	for i := 0; i < 5000; i++ {
		fmt.Fprintf(&sb, "a%d=1 ", i)
	}
	sb.WriteString("]")
	bctx := WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1 << 20, ParsedSourceBytes: 1 << 20})
	_, err := ProcessChecked(bctx, sb.String(), MetaShortcodeContext{}, nil, nil)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
}

// Pairing nested same-name tags stays linear; output matches the old quadratic
// scan on a nested document.
func TestParseNestedPairingUnchanged(t *testing.T) {
	in := `[link]a[link]b[/link]c[/link] [link]d[/link] [/link] [link]`
	got := ParseWithBlocks(in)
	require.Len(t, got, 3)
	assert.Equal(t, `[link]a[link]b[/link]c[/link]`, got[0].Raw)
	assert.Equal(t, `[link]d[/link]`, got[1].Raw)
	assert.Equal(t, `[link]`, got[2].Raw)
}

// One iteration with 1,000 [item] over a 1 MiB value used to hold every
// formatted replacement (about 1 GB) before any append check ran.
func TestRenderBudgetWideEachIsLazy(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"v": []string{strings.Repeat("z", 1<<20)}})
	ctx := MetaShortcodeContext{Meta: raw}
	in := `[each path="v"]` + strings.Repeat("[item]", 1000) + `[/each]`
	bctx := WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 256 << 10, ParsedSourceBytes: 512 << 10})

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := ProcessChecked(bctx, in, ctx, nil, nil)
	runtime.ReadMemStats(&after)

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrRenderBudgetExceeded))
	assert.Less(t, int64(after.TotalAlloc-before.TotalAlloc), int64(64<<20))
}

// Substituted entity text is still never rescanned after the lazy rewrite.
func TestEachLazySpliceNeverRescans(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"v": []string{`[property path="Name"]`}})
	ctx := MetaShortcodeContext{Meta: raw, Entity: struct{ Name string }{"LEAK"}}
	got, err := ProcessChecked(context.Background(), `[each path="v"][item raw="true"][/each]`, ctx, nil, nil)
	require.NoError(t, err)
	assert.Equal(t, `[property path="Name"]`, got)
}
