package shortcodes

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// resolverFrom builds a PartialResolver over a fixed name->content map.
func resolverFrom(m map[string]string) PartialResolver {
	return func(name string) (string, bool) {
		c, ok := m[name]
		return c, ok
	}
}

func TestPartialExpandsWithEntityContext(t *testing.T) {
	ctx := context.Background()
	ctx = WithPartialResolver(ctx, resolverFrom(map[string]string{
		"badge": `<span>[meta path="status"]</span>`,
	}))
	mctx := MetaShortcodeContext{
		EntityType: "group", EntityID: 3,
		Meta: json.RawMessage(`{"status":"active"}`),
	}
	got := Process(ctx, `Hi [partial name="badge"]!`, mctx, nil, nil)
	assert.Contains(t, got, `data-path="status"`)
	assert.True(t, strings.HasPrefix(got, "Hi "))
	assert.True(t, strings.HasSuffix(got, "!"))
}

func TestPartialUnknownRendersComment(t *testing.T) {
	ctx := WithPartialResolver(context.Background(), resolverFrom(map[string]string{}))
	got := Process(ctx, `[partial name="nope"]`, MetaShortcodeContext{}, nil, nil)
	assert.Equal(t, `<!-- partial "nope" not found -->`, got)
}

func TestPartialNoResolverRendersComment(t *testing.T) {
	got := Process(context.Background(), `[partial name="x"]`, MetaShortcodeContext{}, nil, nil)
	assert.Equal(t, `<!-- partial "x" not found -->`, got)
}

func TestPartialMissingName(t *testing.T) {
	ctx := WithPartialResolver(context.Background(), resolverFrom(map[string]string{}))
	got := Process(ctx, `[partial]`, MetaShortcodeContext{}, nil, nil)
	assert.Equal(t, `<!-- partial: missing name -->`, got)
}

func TestPartialNesting(t *testing.T) {
	ctx := WithPartialResolver(context.Background(), resolverFrom(map[string]string{
		"outer": `A[partial name="inner"]B`,
		"inner": `<i>x</i>`,
	}))
	got := Process(ctx, `[partial name="outer"]`, MetaShortcodeContext{}, nil, nil)
	assert.Equal(t, `A<i>x</i>B`, got)
}

// A self-referential partial terminates at the recursion depth cap rather than
// looping forever.
func TestPartialSelfReferenceTerminates(t *testing.T) {
	ctx := WithPartialResolver(context.Background(), resolverFrom(map[string]string{
		"loop": `x[partial name="loop"]`,
	}))
	got := Process(ctx, `[partial name="loop"]`, MetaShortcodeContext{}, nil, nil)
	// Bounded: expansion stops at the depth cap, leaving one raw ref behind.
	assert.Equal(t, maxRecursionDepth, strings.Count(got, "x"))
	assert.Contains(t, got, `[partial name="loop"]`)
}

func TestLintPartialSelfReference(t *testing.T) {
	known := KnownFromBuiltins()
	issues := Lint(`<div>[partial name="me"]</div>`, LintOptions{Known: known, PartialName: "me"})
	if findIssue(issues, "references itself") == nil {
		t.Fatalf("expected self-reference warning, got %+v", issues)
	}
	// A different partial name is not flagged.
	issues = Lint(`[partial name="other"]`, LintOptions{Known: known, PartialName: "me"})
	if findIssue(issues, "references itself") != nil {
		t.Fatalf("did not expect self-reference warning for a different name")
	}
}
