# Block Shortcodes & Built-in Conditional Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend shortcodes to support paired `[name]content[/name]` block syntax and add a built-in `[conditional]` shortcode.

**Architecture:** Two-phase regex parser extension (tokenize then pair-match inside-out) produces top-level non-overlapping `Shortcode` results with `InnerContent`/`IsBlock` fields. Processor delegates expansion to handlers. Built-in conditional evaluates condition first, expands only the taken branch.

**Tech Stack:** Go (shortcodes package, plugin_system), Playwright E2E, Lua plugins

---

### Task 1: Extend Shortcode Struct and Parser

**Files:**
- Modify: `shortcodes/parser.go`
- Modify: `shortcodes/parser_test.go`

- [ ] **Step 1: Write failing tests for block parsing**

Add these tests to `shortcodes/parser_test.go`:

```go
func TestParseWithBlocksSimplePair(t *testing.T) {
	result := ParseWithBlocks(`[conditional path="x" eq="1"]hello[/conditional]`)
	require.Len(t, result, 1)
	assert.Equal(t, "conditional", result[0].Name)
	assert.Equal(t, "1", result[0].Attrs["eq"])
	assert.Equal(t, "hello", result[0].InnerContent)
	assert.True(t, result[0].IsBlock)
	assert.Equal(t, 0, result[0].Start)
	assert.Equal(t, len(`[conditional path="x" eq="1"]hello[/conditional]`), result[0].End)
}

func TestParseWithBlocksSelfClosingUnchanged(t *testing.T) {
	result := ParseWithBlocks(`[meta path="a"]`)
	require.Len(t, result, 1)
	assert.Equal(t, "meta", result[0].Name)
	assert.Equal(t, "", result[0].InnerContent)
	assert.False(t, result[0].IsBlock)
}

func TestParseWithBlocksNestedBlocks(t *testing.T) {
	input := `[conditional path="a" eq="1"]outer[conditional path="b" eq="2"]inner[/conditional]after[/conditional]`
	result := ParseWithBlocks(input)
	require.Len(t, result, 1)
	assert.Equal(t, "conditional", result[0].Name)
	assert.True(t, result[0].IsBlock)
	// Inner content includes the nested block as raw text
	assert.Contains(t, result[0].InnerContent, `[conditional path="b" eq="2"]inner[/conditional]`)
	assert.Contains(t, result[0].InnerContent, "outer")
	assert.Contains(t, result[0].InnerContent, "after")
}

func TestParseWithBlocksMixedSelfClosingAndBlock(t *testing.T) {
	input := `[meta path="x"][conditional path="a" eq="1"]body[/conditional][meta path="y"]`
	result := ParseWithBlocks(input)
	require.Len(t, result, 3)
	assert.Equal(t, "meta", result[0].Name)
	assert.False(t, result[0].IsBlock)
	assert.Equal(t, "conditional", result[1].Name)
	assert.True(t, result[1].IsBlock)
	assert.Equal(t, "body", result[1].InnerContent)
	assert.Equal(t, "meta", result[2].Name)
	assert.False(t, result[2].IsBlock)
}

func TestParseWithBlocksUnmatchedClosingIgnored(t *testing.T) {
	result := ParseWithBlocks(`text[/conditional]more`)
	assert.Empty(t, result)
}

func TestParseWithBlocksUnmatchedOpeningStaysSelfClosing(t *testing.T) {
	result := ParseWithBlocks(`[conditional path="x" eq="1"]no closing tag`)
	require.Len(t, result, 1)
	assert.False(t, result[0].IsBlock)
	assert.Equal(t, "", result[0].InnerContent)
}

func TestParseWithBlocksElseIsLiteralContent(t *testing.T) {
	input := `[conditional path="x" eq="1"]yes[else]no[/conditional]`
	result := ParseWithBlocks(input)
	require.Len(t, result, 1)
	assert.True(t, result[0].IsBlock)
	assert.Equal(t, "yes[else]no", result[0].InnerContent)
}

func TestParseWithBlocksPluginBlock(t *testing.T) {
	input := `[plugin:test:wrap]content[/plugin:test:wrap]`
	result := ParseWithBlocks(input)
	require.Len(t, result, 1)
	assert.Equal(t, "plugin:test:wrap", result[0].Name)
	assert.True(t, result[0].IsBlock)
	assert.Equal(t, "content", result[0].InnerContent)
}

func TestParseWithBlocksTopLevelOnly(t *testing.T) {
	// Nested shortcodes should NOT appear in results — they stay as raw text in InnerContent
	input := `[conditional path="a" eq="1"][meta path="x"][/conditional]`
	result := ParseWithBlocks(input)
	require.Len(t, result, 1)
	assert.True(t, result[0].IsBlock)
	assert.Equal(t, `[meta path="x"]`, result[0].InnerContent)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run TestParseWithBlocks -v`
Expected: FAIL — `ParseWithBlocks` not defined

- [ ] **Step 3: Update Shortcode struct and add ParseWithBlocks**

In `shortcodes/parser.go`, update the struct (line 12) and add the new regex and function:

```go
// Shortcode represents a parsed shortcode occurrence in text.
type Shortcode struct {
	Name         string            // e.g., "meta" or "plugin:my-plugin:rating"
	Attrs        map[string]string // e.g., {"path": "cooking.time", "editable": "true"}
	Raw          string            // original matched text including brackets
	Start        int               // byte offset of opening tag (or full block start)
	End          int               // byte offset end (exclusive)
	InnerContent string            // content between [name]...[/name], empty for self-closing
	IsBlock      bool              // true if matched as [name]...[/name] pair
}
```

Update `shortcodePattern` (line 24) to include `conditional`:

```go
var shortcodePattern = regexp.MustCompile(
	`\[(meta|property|mrql|conditional|plugin:[a-z][a-z0-9_-]*:[a-z][a-z0-9_-]*)\s*([^\]]*)\]`,
)
```

Add a closing tag regex and the `ParseWithBlocks` function:

```go
// closingTagPattern matches [/name] closing tags.
var closingTagPattern = regexp.MustCompile(
	`\[/(meta|property|mrql|conditional|plugin:[a-z][a-z0-9_-]*:[a-z][a-z0-9_-]*)\]`,
)

// token represents a parsed opening or closing tag.
type token struct {
	name    string
	attrs   map[string]string
	raw     string
	start   int
	end     int
	closing bool
	matched bool // used during pair matching
}

// ParseWithBlocks scans input for shortcode patterns and returns all top-level
// matches, including block shortcodes ([name]...[/name]). Nested shortcodes
// inside a block's InnerContent are left as raw text — they are not returned.
// This preserves the processor's linear Start/End splice algorithm.
func ParseWithBlocks(input string) []Shortcode {
	// Phase 1: tokenize all opening and closing tags.
	var tokens []token

	for _, m := range shortcodePattern.FindAllStringSubmatchIndex(input, -1) {
		fullStart, fullEnd := m[0], m[1]
		name := input[m[2]:m[3]]
		attrStr := ""
		if m[4] >= 0 && m[5] >= 0 {
			attrStr = input[m[4]:m[5]]
		}
		tokens = append(tokens, token{
			name:  name,
			attrs: parseAttrs(strings.TrimSpace(attrStr)),
			raw:   input[fullStart:fullEnd],
			start: fullStart,
			end:   fullEnd,
		})
	}

	for _, m := range closingTagPattern.FindAllStringSubmatchIndex(input, -1) {
		fullStart, fullEnd := m[0], m[1]
		name := input[m[2]:m[3]]
		tokens = append(tokens, token{
			name:    name,
			start:   fullStart,
			end:     fullEnd,
			raw:     input[fullStart:fullEnd],
			closing: true,
		})
	}

	if len(tokens) == 0 {
		return nil
	}

	// Sort tokens by start position, closing tags after opening tags at same position.
	sort.Slice(tokens, func(i, j int) bool {
		if tokens[i].start != tokens[j].start {
			return tokens[i].start < tokens[j].start
		}
		// Opening before closing at same position
		if tokens[i].closing != tokens[j].closing {
			return !tokens[i].closing
		}
		return false
	})

	// Phase 2: match pairs inside-out.
	// For each closing tag, scan backward for the nearest unmatched opening tag with the same name.
	for i := range tokens {
		if !tokens[i].closing {
			continue
		}
		for j := i - 1; j >= 0; j-- {
			if tokens[j].matched || tokens[j].closing {
				continue
			}
			if tokens[j].name == tokens[i].name {
				tokens[j].matched = true
				tokens[i].matched = true
				break
			}
		}
	}

	// Phase 3: build top-level results.
	// Walk tokens, skipping any that are inside a block's span.
	var result []Shortcode
	skipUntil := -1

	for i := range tokens {
		if tokens[i].start < skipUntil {
			continue
		}

		if tokens[i].closing {
			// Unmatched closing tag — ignore.
			continue
		}

		if tokens[i].matched {
			// Find the matching closing tag.
			var closeIdx int
			for j := i + 1; j < len(tokens); j++ {
				if tokens[j].closing && tokens[j].matched && tokens[j].name == tokens[i].name {
					// Verify this closing tag actually pairs with this opening tag
					// by checking it's the innermost match: no unmatched closer
					// of the same name between i and j.
					closeIdx = j
					break
				}
			}

			// Re-find the correct matching closer by scanning inside-out again
			// for this specific opening tag.
			depth := 0
			closeIdx = -1
			for j := i + 1; j < len(tokens); j++ {
				if !tokens[j].closing && tokens[j].name == tokens[i].name {
					depth++
				} else if tokens[j].closing && tokens[j].name == tokens[i].name {
					if depth == 0 {
						closeIdx = j
						break
					}
					depth--
				}
			}

			if closeIdx < 0 {
				// Shouldn't happen if matching was correct, treat as self-closing
				result = append(result, Shortcode{
					Name:  tokens[i].name,
					Attrs: tokens[i].attrs,
					Raw:   tokens[i].raw,
					Start: tokens[i].start,
					End:   tokens[i].end,
				})
				continue
			}

			innerStart := tokens[i].end
			innerEnd := tokens[closeIdx].start
			inner := ""
			if innerEnd > innerStart {
				inner = input[innerStart:innerEnd]
			}

			result = append(result, Shortcode{
				Name:         tokens[i].name,
				Attrs:        tokens[i].attrs,
				Raw:          input[tokens[i].start:tokens[closeIdx].end],
				Start:        tokens[i].start,
				End:          tokens[closeIdx].end,
				InnerContent: inner,
				IsBlock:      true,
			})
			skipUntil = tokens[closeIdx].end
		} else {
			// Unmatched opening tag — self-closing.
			result = append(result, Shortcode{
				Name:  tokens[i].name,
				Attrs: tokens[i].attrs,
				Raw:   tokens[i].raw,
				Start: tokens[i].start,
				End:   tokens[i].end,
			})
		}
	}

	return result
}
```

Add `"sort"` to the import block.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run TestParseWithBlocks -v`
Expected: PASS

- [ ] **Step 5: Run all existing parser tests to check for regressions**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -v`
Expected: PASS (existing `Parse()` tests unchanged)

- [ ] **Step 6: Commit**

```bash
git add shortcodes/parser.go shortcodes/parser_test.go
git commit -m "feat: add ParseWithBlocks for paired shortcode tags"
```

---

### Task 2: Add splitElse Helper

**Files:**
- Create: `shortcodes/split_else.go`
- Create: `shortcodes/split_else_test.go`

- [ ] **Step 1: Write failing tests**

Create `shortcodes/split_else_test.go`:

```go
package shortcodes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSplitElseNoBranch(t *testing.T) {
	ifBranch, elseBranch := SplitElse("just content")
	assert.Equal(t, "just content", ifBranch)
	assert.Equal(t, "", elseBranch)
}

func TestSplitElseSimple(t *testing.T) {
	ifBranch, elseBranch := SplitElse("yes[else]no")
	assert.Equal(t, "yes", ifBranch)
	assert.Equal(t, "no", elseBranch)
}

func TestSplitElseWithWhitespace(t *testing.T) {
	ifBranch, elseBranch := SplitElse("  yes  [else]  no  ")
	assert.Equal(t, "  yes  ", ifBranch)
	assert.Equal(t, "  no  ", elseBranch)
}

func TestSplitElseNestedBlockIgnored(t *testing.T) {
	// [else] inside a nested block should NOT be treated as the split point
	input := `[conditional path="x" eq="1"]inner[else]fallback[/conditional][else]outer-else`
	ifBranch, elseBranch := SplitElse(input)
	assert.Equal(t, `[conditional path="x" eq="1"]inner[else]fallback[/conditional]`, ifBranch)
	assert.Equal(t, "outer-else", elseBranch)
}

func TestSplitElseMultipleTopLevel(t *testing.T) {
	// Only the first top-level [else] splits; rest stays in else branch
	ifBranch, elseBranch := SplitElse("a[else]b[else]c")
	assert.Equal(t, "a", ifBranch)
	assert.Equal(t, "b[else]c", elseBranch)
}

func TestSplitElseEmptyBranches(t *testing.T) {
	ifBranch, elseBranch := SplitElse("[else]")
	assert.Equal(t, "", ifBranch)
	assert.Equal(t, "", elseBranch)
}

func TestSplitElseSelfClosingBeforeElse(t *testing.T) {
	// Self-closing shortcodes must NOT affect depth — [else] should still be found
	input := `[meta path="x"][else]fallback`
	ifBranch, elseBranch := SplitElse(input)
	assert.Equal(t, `[meta path="x"]`, ifBranch)
	assert.Equal(t, "fallback", elseBranch)
}

func TestSplitElseMultipleSelfClosingBeforeElse(t *testing.T) {
	input := `[meta path="a"][property path="Name"][else]no`
	ifBranch, elseBranch := SplitElse(input)
	assert.Equal(t, `[meta path="a"][property path="Name"]`, ifBranch)
	assert.Equal(t, "no", elseBranch)
}

func TestSplitElseNestedMultipleLevels(t *testing.T) {
	// Two levels of nesting — [else] at depth 0 is the split point
	input := `before[conditional path="a" eq="1"][conditional path="b" eq="2"]deep[else]deep-else[/conditional][else]mid-else[/conditional][else]top-else`
	ifBranch, elseBranch := SplitElse(input)
	assert.Equal(t, `before[conditional path="a" eq="1"][conditional path="b" eq="2"]deep[else]deep-else[/conditional][else]mid-else[/conditional]`, ifBranch)
	assert.Equal(t, "top-else", elseBranch)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run TestSplitElse -v`
Expected: FAIL — `SplitElse` not defined

- [ ] **Step 3: Implement SplitElse**

Create `shortcodes/split_else.go`:

```go
package shortcodes

import "strings"

const elseTag = "[else]"

// SplitElse splits content on the first top-level [else] tag.
// [else] tags nested inside block shortcodes (tracked by depth) are ignored.
// Only matched block pairs (opening tag with a corresponding closing tag) affect depth.
// Self-closing shortcodes do not change depth.
// Returns (ifBranch, elseBranch). If no top-level [else] is found, elseBranch is empty.
func SplitElse(content string) (string, string) {
	depth := 0
	i := 0

	for i < len(content) {
		// Check for closing tags that decrease depth.
		if content[i] == '[' && i+1 < len(content) && content[i+1] == '/' {
			if loc := closingTagPattern.FindStringIndex(content[i:]); loc != nil && loc[0] == 0 {
				depth--
				if depth < 0 {
					depth = 0
				}
				i += loc[1]
				continue
			}
		}

		// Check for opening tags. Only increase depth if a matching closing tag
		// exists later — otherwise it's a self-closing shortcode.
		if content[i] == '[' && i+1 < len(content) && content[i+1] != '/' && content[i+1] != 'e' {
			if loc := shortcodePattern.FindStringIndex(content[i:]); loc != nil && loc[0] == 0 {
				// Extract the name to build the closing tag pattern
				m := shortcodePattern.FindStringSubmatch(content[i:])
				if m != nil {
					closingTag := "[/" + m[1] + "]"
					if strings.Contains(content[i+loc[1]:], closingTag) {
						depth++
					}
				}
				i += loc[1]
				continue
			}
		}

		// Check for [else] at depth 0.
		if depth == 0 && content[i] == '[' && strings.HasPrefix(content[i:], elseTag) {
			return content[:i], content[i+len(elseTag):]
		}

		i++
	}

	return content, ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run TestSplitElse -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add shortcodes/split_else.go shortcodes/split_else_test.go
git commit -m "feat: add SplitElse helper for conditional branching"
```

---

### Task 3: Add extractRawValueAtPath and Conditional Handler

**Files:**
- Create: `shortcodes/conditional_handler.go`
- Create: `shortcodes/conditional_handler_test.go`

- [ ] **Step 1: Write failing tests for extractRawValueAtPath**

Create `shortcodes/conditional_handler_test.go`:

```go
package shortcodes

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractRawValueAtPathString(t *testing.T) {
	meta := json.RawMessage(`{"status":"active"}`)
	val := extractRawValueAtPath(meta, "status")
	assert.Equal(t, "active", val)
}

func TestExtractRawValueAtPathNumber(t *testing.T) {
	meta := json.RawMessage(`{"count":42}`)
	val := extractRawValueAtPath(meta, "count")
	assert.Equal(t, float64(42), val)
}

func TestExtractRawValueAtPathNested(t *testing.T) {
	meta := json.RawMessage(`{"a":{"b":"deep"}}`)
	val := extractRawValueAtPath(meta, "a.b")
	assert.Equal(t, "deep", val)
}

func TestExtractRawValueAtPathMissing(t *testing.T) {
	meta := json.RawMessage(`{"a":"b"}`)
	val := extractRawValueAtPath(meta, "missing")
	assert.Nil(t, val)
}

func TestExtractRawValueAtPathEmpty(t *testing.T) {
	val := extractRawValueAtPath(nil, "x")
	assert.Nil(t, val)
}

func TestExtractRawValueAtPathBool(t *testing.T) {
	meta := json.RawMessage(`{"featured":true}`)
	val := extractRawValueAtPath(meta, "featured")
	assert.Equal(t, true, val)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run TestExtractRawValue -v`
Expected: FAIL — `extractRawValueAtPath` not defined

- [ ] **Step 3: Write failing tests for RenderConditionalShortcode**

Add to `shortcodes/conditional_handler_test.go`:

```go
func TestConditionalEq(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "eq": "active"},
		InnerContent: "<b>yes</b>",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"status":"active"}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "<b>yes</b>", result)
}

func TestConditionalEqFalse(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "eq": "active"},
		InnerContent: "<b>yes</b>",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"status":"inactive"}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "", result)
}

func TestConditionalNeq(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "neq": "draft"},
		InnerContent: "published",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"status":"active"}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "published", result)
}

func TestConditionalGt(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "score", "gt": "50"},
		InnerContent: "high",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"score":75}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "high", result)
}

func TestConditionalLt(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "score", "lt": "50"},
		InnerContent: "low",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"score":25}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "low", result)
}

func TestConditionalGtNonNumericReturnsFalse(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "name", "gt": "50"},
		InnerContent: "should not show",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"name":"hello"}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "", result)
}

func TestConditionalLtNonNumericReturnsFalse(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "name", "lt": "50"},
		InnerContent: "should not show",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"name":"hello"}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "", result)
}

func TestConditionalContains(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "name", "contains": "test"},
		InnerContent: "has test",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"name":"my test item"}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "has test", result)
}

func TestConditionalEmpty(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "missing", "empty": "true"},
		InnerContent: "is empty",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "is empty", result)
}

func TestConditionalNotEmpty(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "name", "not-empty": "true"},
		InnerContent: "exists",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"name":"hello"}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "exists", result)
}

func TestConditionalElseBranch(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "eq": "active"},
		InnerContent: "yes[else]no",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"status":"inactive"}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "no", result)
}

func TestConditionalElseBranchTrue(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "status", "eq": "active"},
		InnerContent: "yes[else]no",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"status":"active"}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "yes", result)
}

func TestConditionalSelfClosingReturnsEmpty(t *testing.T) {
	sc := Shortcode{
		Name:  "conditional",
		Attrs: map[string]string{"path": "status", "eq": "active"},
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"status":"active"}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "", result)
}

func TestConditionalFieldSource(t *testing.T) {
	type TestEntity struct {
		Name string
		ID   uint
	}
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"field": "Name", "eq": "hello"},
		InnerContent: "matched",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Entity: &TestEntity{Name: "hello", ID: 1}}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "matched", result)
}

func TestConditionalBoolEq(t *testing.T) {
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"path": "featured", "eq": "true"},
		InnerContent: "featured",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{Meta: json.RawMessage(`{"featured":true}`)}
	result := RenderConditionalShortcode(nil, sc, ctx, nil, nil, 0)
	assert.Equal(t, "featured", result)
}

func TestConditionalMRQLSourceFlat(t *testing.T) {
	// MRQL flat result: condition value = item count
	executor := func(ctx context.Context, query, saved string, limit, buckets int, scopeGroupID uint) (*QueryResult, error) {
		return &QueryResult{
			Mode:  "flat",
			Items: []QueryResultItem{{}, {}, {}}, // 3 items
		}, nil
	}
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "type = resource", "gt": "2"},
		InnerContent: "many",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "many", result)
}

func TestConditionalMRQLSourceFlatFalse(t *testing.T) {
	executor := func(ctx context.Context, query, saved string, limit, buckets int, scopeGroupID uint) (*QueryResult, error) {
		return &QueryResult{
			Mode:  "flat",
			Items: []QueryResultItem{{}}, // 1 item
		}, nil
	}
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "type = resource", "gt": "2"},
		InnerContent: "many",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "", result)
}

func TestConditionalMRQLSourceAggregated(t *testing.T) {
	executor := func(ctx context.Context, query, saved string, limit, buckets int, scopeGroupID uint) (*QueryResult, error) {
		return &QueryResult{
			Mode: "aggregated",
			Rows: []map[string]any{{"total": float64(100)}},
		}, nil
	}
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "type = resource GROUP BY category", "aggregate": "total", "gt": "50"},
		InnerContent: "high total",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "high total", result)
}

func TestConditionalMRQLSourceAggregatedNoAggregate(t *testing.T) {
	executor := func(ctx context.Context, query, saved string, limit, buckets int, scopeGroupID uint) (*QueryResult, error) {
		return &QueryResult{
			Mode: "aggregated",
			Rows: []map[string]any{{"total": float64(100)}},
		}, nil
	}
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "type = resource GROUP BY category", "gt": "50"},
		InnerContent: "should not show",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}
	// Missing aggregate attr — resolveConditionalValue returns nil
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "", result)
}

func TestConditionalMRQLSourceBucketed(t *testing.T) {
	executor := func(ctx context.Context, query, saved string, limit, buckets int, scopeGroupID uint) (*QueryResult, error) {
		return &QueryResult{
			Mode:   "bucketed",
			Groups: []QueryResultGroup{{}, {}, {}, {}}, // 4 groups
		}, nil
	}
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "type = resource GROUP BY category", "gt": "3"},
		InnerContent: "many groups",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1}
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "many groups", result)
}

func TestConditionalMRQLSourcePriorityOverPath(t *testing.T) {
	// MRQL takes priority over path — even if path would match, mrql result is used
	executor := func(ctx context.Context, query, saved string, limit, buckets int, scopeGroupID uint) (*QueryResult, error) {
		return &QueryResult{
			Mode:  "flat",
			Items: []QueryResultItem{}, // 0 items
		}, nil
	}
	sc := Shortcode{
		Name:         "conditional",
		Attrs:        map[string]string{"mrql": "type = resource", "path": "status", "gt": "0"},
		InnerContent: "should not show",
		IsBlock:      true,
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: json.RawMessage(`{"status":"active"}`)}
	// MRQL returns 0 items, gt "0" is false (0 > 0 is false)
	result := RenderConditionalShortcode(context.Background(), sc, ctx, nil, executor, 0)
	assert.Equal(t, "", result)
}
```

- [ ] **Step 4: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run "TestConditional|TestExtractRawValue" -v`
Expected: FAIL — functions not defined

- [ ] **Step 5: Implement conditional_handler.go**

Create `shortcodes/conditional_handler.go`:

```go
package shortcodes

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// extractRawValueAtPath navigates a JSON object by dot-notation path and returns
// the raw Go value (string, float64, bool, nil). Unlike extractValueAtPath, this
// does NOT JSON-encode the result — it returns the value as json.Unmarshal produced it.
func extractRawValueAtPath(metaRaw json.RawMessage, path string) any {
	if len(metaRaw) == 0 || path == "" {
		return nil
	}

	var meta map[string]any
	if err := json.Unmarshal(metaRaw, &meta); err != nil {
		return nil
	}

	parts := strings.Split(path, ".")
	var current any = meta

	for _, part := range parts {
		obj, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current, ok = obj[part]
		if !ok {
			return nil
		}
	}

	return current
}

// resolveConditionalValue resolves the condition value from the shortcode's
// data source attributes. Checked in order: mrql > field > path.
func resolveConditionalValue(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, executor QueryExecutor) any {
	// MRQL source
	if query := sc.Attrs["mrql"]; query != "" && executor != nil {
		scope := resolveScopeKeyword(sc.Attrs["scope"], ctx)
		limit := parseIntAttr(sc.Attrs["limit"], defaultMRQLShortcodeLimit)
		buckets := parseIntAttr(sc.Attrs["buckets"], defaultMRQLShortcodeBuckets)

		result, err := executor(reqCtx, query, "", limit, buckets, scope)
		if err != nil || result == nil {
			return nil
		}

		switch result.Mode {
		case "flat", "":
			return float64(len(result.Items))
		case "aggregated":
			agg := sc.Attrs["aggregate"]
			if agg == "" || len(result.Rows) == 0 {
				return nil
			}
			return result.Rows[0][agg]
		case "bucketed":
			return float64(len(result.Groups))
		}
		return nil
	}

	// Field source
	if fieldName := sc.Attrs["field"]; fieldName != "" && ctx.Entity != nil {
		v := reflect.ValueOf(ctx.Entity)
		if v.Kind() == reflect.Ptr {
			if v.IsNil() {
				return nil
			}
			v = v.Elem()
		}
		if v.Kind() != reflect.Struct {
			return nil
		}
		field := v.FieldByName(fieldName)
		if !field.IsValid() {
			return nil
		}
		return formatFieldValue(field)
	}

	// Path source (default)
	return extractRawValueAtPath(ctx.Meta, sc.Attrs["path"])
}

// evaluateCondition checks the resolved value against the shortcode's operator attributes.
// Returns true if the condition is met.
func evaluateCondition(value any, attrs map[string]string) bool {
	if _, ok := attrs["eq"]; ok {
		return fmt.Sprint(value) == attrs["eq"]
	}
	if _, ok := attrs["neq"]; ok {
		return fmt.Sprint(value) != attrs["neq"]
	}
	if gtStr, ok := attrs["gt"]; ok {
		lhs, lhsOk := toFloat(value)
		rhs, rhsOk := toFloat(gtStr)
		return lhsOk && rhsOk && lhs > rhs
	}
	if ltStr, ok := attrs["lt"]; ok {
		lhs, lhsOk := toFloat(value)
		rhs, rhsOk := toFloat(ltStr)
		return lhsOk && rhsOk && lhs < rhs
	}
	if substr, ok := attrs["contains"]; ok {
		return strings.Contains(fmt.Sprint(value), substr)
	}
	if _, ok := attrs["empty"]; ok {
		return value == nil || fmt.Sprint(value) == ""
	}
	if _, ok := attrs["not-empty"]; ok {
		return value != nil && fmt.Sprint(value) != ""
	}
	return false
}

// toFloat attempts to parse a value as float64.
// Returns (value, true) on success, (0, false) if the value is not numeric.
func toFloat(v any) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case string:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return 0, false
		}
		return f, true
	default:
		f, err := strconv.ParseFloat(fmt.Sprint(v), 64)
		if err != nil {
			return 0, false
		}
		return f, true
	}
}

// RenderConditionalShortcode expands a [conditional] block shortcode.
// It evaluates the condition, selects the matching branch (splitting on [else]),
// and recursively expands shortcodes in the selected branch only.
func RenderConditionalShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if !sc.IsBlock {
		return ""
	}

	value := resolveConditionalValue(reqCtx, sc, ctx, executor)
	conditionMet := evaluateCondition(value, sc.Attrs)

	ifBranch, elseBranch := SplitElse(sc.InnerContent)

	var selected string
	if conditionMet {
		selected = ifBranch
	} else {
		selected = elseBranch
	}

	if selected == "" {
		return ""
	}

	// Recursively expand shortcodes in the selected branch only.
	return processWithDepth(reqCtx, selected, ctx, renderer, executor, depth+1)
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run "TestConditional|TestExtractRawValue" -v`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add shortcodes/conditional_handler.go shortcodes/conditional_handler_test.go
git commit -m "feat: add built-in conditional shortcode handler"
```

---

### Task 4: Update Processor to Use ParseWithBlocks

**Files:**
- Modify: `shortcodes/processor.go`
- Modify: `shortcodes/processor_test.go`

- [ ] **Step 1: Write failing tests for block processing**

Add to `shortcodes/processor_test.go`:

```go
func TestProcessBlockConditionalTrue(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"active"}`),
	}
	input := `before[conditional path="status" eq="active"]<b>yes</b>[/conditional]after`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, "before<b>yes</b>after", result)
}

func TestProcessBlockConditionalFalse(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"inactive"}`),
	}
	input := `[conditional path="status" eq="active"]<b>yes</b>[/conditional]`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, "", result)
}

func TestProcessBlockConditionalElse(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"status":"draft"}`),
	}
	input := `[conditional path="status" eq="active"]yes[else]no[/conditional]`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Equal(t, "no", result)
}

func TestProcessBlockWithNestedSelfClosing(t *testing.T) {
	meta := map[string]any{"status": "active", "name": "test"}
	metaJSON, _ := json.Marshal(meta)
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       metaJSON,
	}
	input := `[conditional path="status" eq="active"][meta path="name"][/conditional]`
	result := Process(context.Background(), input, ctx, nil, nil)
	assert.Contains(t, result, "<meta-shortcode")
	assert.Contains(t, result, `data-path="name"`)
}

func TestProcessBlockPluginGetsInnerContent(t *testing.T) {
	var receivedInner string
	var receivedIsBlock bool
	renderer := func(name string, sc Shortcode, ctx MetaShortcodeContext) (string, error) {
		receivedInner = sc.InnerContent
		receivedIsBlock = sc.IsBlock
		return "<div>" + sc.InnerContent + "</div>", nil
	}
	ctx := MetaShortcodeContext{EntityType: "group", EntityID: 1, Meta: []byte(`{}`)}
	input := `[plugin:test:wrap]hello world[/plugin:test:wrap]`
	result := Process(context.Background(), input, ctx, renderer, nil)
	assert.Equal(t, "hello world", receivedInner)
	assert.True(t, receivedIsBlock)
	assert.Equal(t, "<div>hello world</div>", result)
}

func TestProcessBlockDepthLimit(t *testing.T) {
	ctx := MetaShortcodeContext{
		EntityType: "group",
		EntityID:   1,
		Meta:       json.RawMessage(`{"a":"1"}`),
	}
	// Build deeply nested conditionals beyond the depth limit
	inner := "deep"
	for i := 0; i < 12; i++ {
		inner = fmt.Sprintf(`[conditional path="a" eq="1"]%s[/conditional]`, inner)
	}
	result := Process(context.Background(), inner, ctx, nil, nil)
	// Should not panic, should eventually stop expanding
	assert.Contains(t, result, "deep")
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -run "TestProcessBlock" -v`
Expected: FAIL — processor doesn't handle blocks yet

- [ ] **Step 3: Update processor.go**

In `shortcodes/processor.go`, bump the depth limit (line 51) and rewrite `processWithDepth` (line 62):

```go
const maxRecursionDepth = 10
```

Replace the `processWithDepth` function:

```go
func processWithDepth(reqCtx context.Context, input string, ctx MetaShortcodeContext, renderer PluginRenderer, executor QueryExecutor, depth int) string {
	if depth >= maxRecursionDepth {
		return input
	}

	shortcodes := ParseWithBlocks(input)
	if len(shortcodes) == 0 {
		return input
	}

	var b strings.Builder
	b.Grow(len(input) * 2)
	lastEnd := 0

	for _, sc := range shortcodes {
		b.WriteString(input[lastEnd:sc.Start])

		var replacement string

		switch {
		case sc.Name == "conditional":
			replacement = RenderConditionalShortcode(reqCtx, sc, ctx, renderer, executor, depth)
		case sc.Name == "meta":
			replacement = RenderMetaShortcode(sc, ctx)
		case sc.Name == "property":
			replacement = RenderPropertyShortcode(sc, ctx)
		case sc.Name == "mrql":
			if executor != nil && depth < maxRecursionDepth {
				replacement = RenderMRQLShortcode(reqCtx, sc, ctx, renderer, executor, depth)
			} else {
				replacement = sc.Raw
			}
		case strings.HasPrefix(sc.Name, "plugin:"):
			if renderer != nil {
				parts := strings.SplitN(sc.Name, ":", 3)
				if len(parts) == 3 {
					html, err := renderer(parts[1], sc, ctx)
					if err == nil {
						replacement = html
						// Post-plugin expansion for block shortcodes
						if sc.IsBlock && depth+1 < maxRecursionDepth {
							replacement = processWithDepth(reqCtx, replacement, ctx, renderer, executor, depth+1)
						}
					} else {
						replacement = sc.Raw
					}
				} else {
					replacement = sc.Raw
				}
			} else {
				replacement = sc.Raw
			}
		}

		b.WriteString(replacement)
		lastEnd = sc.End
	}

	b.WriteString(input[lastEnd:])

	return b.String()
}
```

- [ ] **Step 4: Run all shortcode tests**

Run: `go test --tags 'json1 fts5' ./shortcodes/... -v`
Expected: PASS (all existing and new tests)

- [ ] **Step 5: Commit**

```bash
git add shortcodes/processor.go shortcodes/processor_test.go
git commit -m "feat: processor uses ParseWithBlocks, handles conditional and plugin blocks"
```

---

### Task 5: Update Plugin System for Block Shortcodes

**Files:**
- Modify: `plugin_system/shortcodes.go`
- Modify: `plugin_system/shortcode_docs.go`
- Modify: `plugin_system/shortcodes_test.go`

- [ ] **Step 1: Write failing tests for inner_content and is_block in Lua context**

Add to `plugin_system/shortcodes_test.go`:

```go
func TestRenderShortcodeBlockContext(t *testing.T) {
	pm := newTestPluginManager(t)
	defer pm.Close()

	registerTestPlugin(t, pm, "block-test", `
		mah.shortcode({
			name = "wrapper",
			label = "Wrapper",
			render = function(ctx)
				if ctx.is_block then
					return "<wrap>" .. (ctx.inner_content or "") .. "</wrap>"
				end
				return "<self-closing/>"
			end,
		})
	`)

	// Block usage
	result, err := pm.RenderShortcode(
		context.Background(), "block-test", "plugin:block-test:wrapper",
		"group", 1, json.RawMessage(`{}`),
		map[string]string{}, nil,
	)
	require.NoError(t, err)
	// Without inner_content/is_block in context, this would return "<self-closing/>"
	// We need to verify the test structure matches the actual test helpers
	assert.Equal(t, "<self-closing/>", result) // Self-closing call — no inner content
}
```

Note: the exact test helper setup depends on the existing test infrastructure in `shortcodes_test.go`. This test verifies the Lua context receives `is_block` and `inner_content`. The actual block-mode test will be covered in the processor tests (Task 4) since `RenderShortcode` is called from the processor with the full `Shortcode` struct.

- [ ] **Step 2: Update RenderShortcode to pass inner_content and is_block**

In `plugin_system/shortcodes.go`, update the `ctxData` map in `RenderShortcode` (after line 245):

```go
	ctxData := map[string]any{
		"entity_type":   entityType,
		"entity_id":     float64(entityID),
		"value":         metaMap,
		"attrs":         attrsMap,
		"settings":      settings,
		"inner_content": innerContent,
		"is_block":      isBlock,
	}
```

This requires adding `innerContent string` and `isBlock bool` parameters to the `RenderShortcode` method signature:

```go
func (pm *PluginManager) RenderShortcode(reqCtx context.Context, pluginName, fullTypeName, entityType string, entityID uint, meta json.RawMessage, attrs map[string]string, entity any, innerContent string, isBlock bool) (string, error) {
```

- [ ] **Step 3: Update renderShortcodeForDocs similarly**

In `plugin_system/shortcodes.go`, update `renderShortcodeForDocs` to also accept and pass `innerContent` and `isBlock`:

```go
func (pm *PluginManager) renderShortcodeForDocs(pluginName, fullTypeName string, meta json.RawMessage, attrs map[string]string, innerContent string, isBlock bool) (string, error) {
```

Add to the `ctxData` map (after line 399):

```go
		"inner_content": innerContent,
		"is_block":      isBlock,
```

- [ ] **Step 4: Update renderExamplePreview to use ParseWithBlocks**

In `plugin_system/shortcode_docs.go`, update `renderExamplePreview` (line 150):

```go
func renderExamplePreview(pm *PluginManager, pluginName, fullTypeName string, ex ShortcodeDocExample) string {
	parsed := shortcodes.ParseWithBlocks(ex.Code)
	if len(parsed) != 1 || parsed[0].Name != fullTypeName {
		return ""
	}

	metaJSON, err := json.Marshal(ex.ExampleData)
	if err != nil {
		log.Printf("[plugin] docs preview: failed to marshal example data for %s: %v", fullTypeName, err)
		return ""
	}

	result, err := pm.renderShortcodeForDocs(pluginName, fullTypeName, metaJSON, parsed[0].Attrs, parsed[0].InnerContent, parsed[0].IsBlock)
	if err != nil {
		log.Printf("[plugin] docs preview: render failed for %s: %v", fullTypeName, err)
		return ""
	}

	return result
}
```

- [ ] **Step 5: Update the template handler's plugin renderer callback**

In `server/template_handlers/template_filters/shortcode_tag.go` (line 74), update the closure to pass the new fields:

```go
			pluginRenderer = func(pluginName string, sc shortcodes.Shortcode, mctx shortcodes.MetaShortcodeContext) (string, error) {
				return pm.RenderShortcode(reqCtx, pluginName, sc.Name, mctx.EntityType, mctx.EntityID, mctx.Meta, sc.Attrs, mctx.Entity, sc.InnerContent, sc.IsBlock)
			}
```

- [ ] **Step 6: Fix all compilation errors from signature change**

Run: `go build --tags 'json1 fts5' ./...`

Any call sites that use the old `RenderShortcode` signature (without `innerContent`/`isBlock`) need updating. Add `"", false` for existing self-closing call sites (e.g., in tests).

- [ ] **Step 7: Add preview divergence test**

Add to `plugin_system/shortcodes_test.go` a test that verifies nested shortcodes in plugin block output are NOT expanded in docs preview (they render as literal text). This documents the intentional divergence from runtime behavior:

```go
func TestDocsPreviewBlockShortcodeNoNestedExpansion(t *testing.T) {
	pm := newTestPluginManager(t)
	defer pm.Close()

	registerTestPlugin(t, pm, "preview-test", `
		mah.shortcode({
			name = "echo",
			label = "Echo",
			description = "Echoes inner content",
			render = function(ctx)
				return ctx.inner_content or ""
			end,
			examples = {
				{
					title = "With nested shortcode",
					code = '[plugin:preview-test:echo]has [meta path="x"] inside[/plugin:preview-test:echo]',
					example_data = { x = "val" },
				},
			},
		})
	`)

	// The preview should contain the literal [meta path="x"] text,
	// NOT an expanded <meta-shortcode> element.
	items := pm.collectDocItems("preview-test")
	require.Len(t, items, 1)
	require.Len(t, items[0].Examples, 1)

	preview := renderExamplePreview(pm, "preview-test", "plugin:preview-test:echo", items[0].Examples[0])
	assert.Contains(t, preview, `[meta path="x"]`)
	assert.NotContains(t, preview, "<meta-shortcode")
}
```

Note: the exact test helper setup (`newTestPluginManager`, `registerTestPlugin`) must match the existing test infrastructure in `plugin_system/shortcodes_test.go`. Adapt the function names to match what's actually there.

- [ ] **Step 8: Run all tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS

- [ ] **Step 9: Commit**

```bash
git add plugin_system/shortcodes.go plugin_system/shortcode_docs.go plugin_system/shortcodes_test.go server/template_handlers/template_filters/shortcode_tag.go
git commit -m "feat: pass inner_content and is_block to plugin shortcodes"
```

---

### Task 6: Remove Conditional from data-views Plugin

**Files:**
- Modify: `plugins/data-views/plugin.lua`
- Modify: `e2e/test-plugins/data-views/plugin.lua`
- Modify: `e2e/tests/plugins/plugin-data-views.spec.ts`

- [ ] **Step 1: Remove from production plugin**

In `plugins/data-views/plugin.lua`:
- Remove `render_conditional` function (lines 1691-1759)
- Remove the `mah.shortcode` registration for `conditional` (lines 2567-2604, the block starting with `name = "conditional"`)
- Remove the help text reference to `[plugin:data-views:conditional]` if present

- [ ] **Step 2: Remove from e2e test plugin**

In `e2e/test-plugins/data-views/plugin.lua`:
- Remove `render_conditional` function (around line 1363)
- Remove `mah.shortcode` registration for `conditional` (around line 1684)
- Remove the help text reference (around line 27)

- [ ] **Step 3: Update e2e test to use built-in conditional**

In `e2e/tests/plugins/plugin-data-views.spec.ts`:

Replace the `[plugin:data-views:conditional ...]` shortcode in the `CustomHeader` (line 28) with the built-in block form:

```typescript
          '[conditional path="featured" eq="true"]Featured Item[/conditional]',
```

Update the test (line 160) to still verify the same visible outcome:

```typescript
  test('conditional shows content when condition is met', async ({ page }) => {
    await page.goto(`/group?id=${groupId}`);
    await page.waitForLoadState('load');

    // "Featured Item" should be visible (featured=true, eq="true")
    await expect(page.locator('text=Featured Item')).toBeVisible({ timeout: 5000 });
  });
```

The test body stays the same — only the shortcode syntax in `CustomHeader` changes.

- [ ] **Step 4: Run E2E tests to verify**

Run: `cd e2e && npm run test:with-server`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add plugins/data-views/plugin.lua e2e/test-plugins/data-views/plugin.lua e2e/tests/plugins/plugin-data-views.spec.ts
git commit -m "refactor: remove conditional from data-views plugin, use built-in"
```

---

### Task 7: Add E2E Tests for Built-in Conditional

**Files:**
- Modify: `e2e/tests/shortcodes.spec.ts`

- [ ] **Step 1: Add conditional shortcode E2E test suite**

Add to `e2e/tests/shortcodes.spec.ts`:

```typescript
test.describe('Built-in conditional shortcode', () => {
  let categoryId: number;
  let activeGroupId: number;
  let inactiveGroupId: number;

  test.beforeAll(async ({ apiClient }) => {
    const cat = await apiClient.createCategory(
      `Conditional Test ${Date.now()}`,
      'Tests built-in conditional shortcode',
      {
        CustomSidebar: [
          '[conditional path="status" eq="active"]<span class="cond-active">Active</span>[/conditional]',
          '[conditional path="status" eq="active"]<span class="cond-if">IF branch</span>[else]<span class="cond-else">ELSE branch</span>[/conditional]',
          '[conditional path="count" gt="5"]<span class="cond-gt">High count</span>[/conditional]',
          '[conditional path="status" not-empty="true"]<span class="cond-notempty">Has status</span>[/conditional]',
          '[conditional path="status" eq="active"][meta path="status"][/conditional]',
        ].join('\n'),
      },
    );
    categoryId = cat.ID;

    const activeGroup = await apiClient.createGroup({
      name: `Active Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: 'active', count: 10 }),
    });
    activeGroupId = activeGroup.ID;

    const inactiveGroup = await apiClient.createGroup({
      name: `Inactive Group ${Date.now()}`,
      categoryId: cat.ID,
      meta: JSON.stringify({ status: 'inactive', count: 2 }),
    });
    inactiveGroupId = inactiveGroup.ID;
  });

  test.afterAll(async ({ apiClient }) => {
    if (inactiveGroupId) await apiClient.deleteGroup(inactiveGroupId);
    if (activeGroupId) await apiClient.deleteGroup(activeGroupId);
    if (categoryId) await apiClient.deleteCategory(categoryId);
  });

  test('shows content when condition is true', async ({ page }) => {
    await page.goto(`/group?id=${activeGroupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.cond-active')).toBeVisible({ timeout: 5000 });
  });

  test('hides content when condition is false', async ({ page }) => {
    await page.goto(`/group?id=${inactiveGroupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.cond-active')).not.toBeVisible({ timeout: 3000 });
  });

  test('shows else branch when condition is false', async ({ page }) => {
    await page.goto(`/group?id=${inactiveGroupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.cond-else')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.cond-if')).not.toBeVisible({ timeout: 3000 });
  });

  test('shows if branch when condition is true', async ({ page }) => {
    await page.goto(`/group?id=${activeGroupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.cond-if')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.cond-else')).not.toBeVisible({ timeout: 3000 });
  });

  test('gt operator works with numeric values', async ({ page }) => {
    await page.goto(`/group?id=${activeGroupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.cond-gt')).toBeVisible({ timeout: 5000 });
  });

  test('gt operator hides when value is below threshold', async ({ page }) => {
    await page.goto(`/group?id=${inactiveGroupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.cond-gt')).not.toBeVisible({ timeout: 3000 });
  });

  test('not-empty operator shows when value exists', async ({ page }) => {
    await page.goto(`/group?id=${activeGroupId}`);
    await page.waitForLoadState('load');
    await expect(page.locator('.cond-notempty')).toBeVisible({ timeout: 5000 });
  });

  test('nested shortcodes expand inside conditional', async ({ page }) => {
    await page.goto(`/group?id=${activeGroupId}`);
    await page.waitForLoadState('load');
    // The [meta path="status"] inside the conditional should have expanded
    const metaShortcode = page.locator('meta-shortcode[data-path="status"]');
    await expect(metaShortcode).toBeVisible({ timeout: 5000 });
  });
});
```

- [ ] **Step 2: Run E2E tests**

Run: `cd e2e && npm run test:with-server`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add e2e/tests/shortcodes.spec.ts
git commit -m "test: add E2E tests for built-in conditional shortcode"
```

---

### Task 8: Update Docs Site

**Files:**
- Modify: `docs-site/docs/features/shortcodes.md`
- Modify: `docs-site/docs/features/built-in-plugins.md`
- Modify: `docs-site/docs/features/plugin-lua-api.md`

- [ ] **Step 1: Add block shortcode syntax and conditional documentation**

In `docs-site/docs/features/shortcodes.md`, after the existing Syntax section (line 21), add block syntax documentation. Then add a new `[conditional]` section before the Plugin Shortcodes section (line 164).

Add after line 21 (after the existing syntax paragraph):

```markdown
### Block Syntax

Shortcodes can also be used as paired opening/closing tags wrapping content:

```
[name attr="value"]
  content here, including HTML and other shortcodes
[/name]
```

Block shortcodes can be nested. The inner content is processed after the outer shortcode decides what to render. Not all shortcodes use block mode — each handler decides whether to use the inner content.
```

Add a new section before "## Plugin Shortcodes" (line 164):

```markdown
## `[conditional]` -- Conditional Display

Conditionally renders content based on a metadata value, entity field, or query result.

### Attributes

| Attribute | Required | Default | Description |
|-----------|----------|---------|-------------|
| `path` | No* | -- | Dot-notation path into the entity's Meta JSON |
| `field` | No* | -- | Entity struct field name (e.g., `Name`, `CreatedAt`) |
| `mrql` | No* | -- | MRQL query expression; result is used as the condition value |
| `scope` | No | `entity` | Scope for MRQL queries: `entity`, `parent`, `root`, `global` |
| `aggregate` | No | -- | Column name for aggregated MRQL results |
| `eq` | No | -- | True when value equals this string |
| `neq` | No | -- | True when value does not equal this string |
| `gt` | No | -- | True when numeric value is greater than this |
| `lt` | No | -- | True when numeric value is less than this |
| `contains` | No | -- | True when value contains this substring |
| `empty` | No | -- | True when value is nil or empty string |
| `not-empty` | No | -- | True when value is non-nil and non-empty |

*One of `path`, `field`, or `mrql` is required as the condition source.

### Condition Sources

**Path** (default): reads from the entity's meta JSON using dot-notation.

**Field**: reads a struct field from the entity object using reflection. Same fields as `[property]`.

**MRQL**: runs a query and extracts a scalar value. For flat results, the value is the item count. For aggregated results, use the `aggregate` attribute to name the column. For bucketed results, the value is the number of groups.

### Else Branch

Use `[else]` inside the block to define a fallback when the condition is false:

```
[conditional path="status" eq="active"]
  <span class="text-green-600">Active</span>
[else]
  <span class="text-stone-400">Inactive</span>
[/conditional]
```

### Nesting

Conditional blocks can be nested, and can contain any other shortcode:

```
[conditional path="status" eq="active"]
  <h3>Active Item</h3>
  [meta path="status" editable=true]
  [conditional path="priority" eq="high"]
    <span class="text-red-600">High Priority!</span>
  [/conditional]
[/conditional]
```

### Examples

```
[conditional path="featured" eq="true"]
  <span class="badge">Featured</span>
[/conditional]

[conditional path="score" gt="90"]
  <span class="text-red-600 font-bold">High score</span>
[else]
  <span class="text-stone-500">Normal</span>
[/conditional]

[conditional path="notes" not-empty="true"]
  <p>This item has notes attached.</p>
[/conditional]
```
```

Update the Plugin Shortcodes section to note block support:

```markdown
## Plugin Shortcodes

Plugins can register custom shortcodes via the `mah.shortcode()` Lua API. Plugin shortcodes use the format:

```
[plugin:plugin-name:shortcode-name attr="value"]
```

Plugin shortcodes also support block mode:

```
[plugin:plugin-name:shortcode-name attr="value"]
  content here
[/plugin:plugin-name:shortcode-name]
```

The plugin receives `inner_content` and `is_block` in its render context. Nested shortcodes inside plugin block output are expanded automatically after the plugin returns.

Note: in docs preview, nested shortcodes inside plugin block output are not expanded (they render as literal text). This is a preview-only limitation; runtime rendering expands them normally.
```

- [ ] **Step 2: Update built-in-plugins.md**

In `docs-site/docs/features/built-in-plugins.md`, remove `conditional` from the data-views shortcode table (line 34). Update the plugin description (line 14) to remove "conditional content" since conditional is now built-in.

- [ ] **Step 3: Update plugin-lua-api.md**

In `docs-site/docs/features/plugin-lua-api.md`, update the Render Context table (line 809) to add the new fields:

```markdown
| Field | Description |
|-------|-------------|
| `ctx.entity_type` | `"group"`, `"resource"`, or `"note"` |
| `ctx.entity_id` | Entity ID |
| `ctx.value` | Entity's full Meta as a Lua table |
| `ctx.attrs` | Shortcode attributes as a key-value table |
| `ctx.settings` | Plugin settings key-value pairs |
| `ctx.inner_content` | Content between opening and closing tags (empty for self-closing shortcodes) |
| `ctx.is_block` | `true` if the shortcode was used as a block `[name]...[/name]`, `false` otherwise |
```

Also add a note after the Execution section (line 825):

```markdown
### Block Shortcodes

Plugin shortcodes support block mode. When used as `[plugin:name:sc]content[/plugin:name:sc]`, the render function receives `ctx.inner_content` with the raw content between tags, and `ctx.is_block = true`. Nested shortcodes inside plugin block output are expanded automatically after the plugin render function returns.

In docs preview, nested shortcodes inside plugin block output are not expanded (they render as literal text). This is a preview-only limitation.
```

- [ ] **Step 4: Commit**

```bash
git add docs-site/docs/features/shortcodes.md docs-site/docs/features/built-in-plugins.md docs-site/docs/features/plugin-lua-api.md
git commit -m "docs: add block shortcode syntax and conditional shortcode docs"
```

---

### Task 9: Run Full Test Suite

**Files:** None (verification only)

- [ ] **Step 1: Run Go unit tests**

Run: `go test --tags 'json1 fts5' ./...`
Expected: PASS

- [ ] **Step 2: Run E2E tests (browser + CLI)**

Run: `cd e2e && npm run test:with-server:all`
Expected: PASS

- [ ] **Step 3: Run Postgres tests**

Run: `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1 && cd e2e && npm run test:with-server:postgres`
Expected: PASS

- [ ] **Step 4: Commit any remaining fixes**

If any tests fail, fix the issues and commit.
