package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"regexp/syntax"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The estimate must never undercount what the compiler builds.
func TestRegexCostBoundsCompiledProgram(t *testing.T) {
	patterns := []string{
		`a`, `abc`, `(?i)hello`, `a*`, `(a*)*`, `(a*)+`, `(a?){0,50}`, `(a|b|c){2,5}`, `x{2,}`, `x{0,}`, `x{3,}`,
		`(?:a{3}){10}`, `a{0}`, `a{1}`, `a{0,1}`, `a{0,1000}`, strings.Repeat(`a{0,1000}`, 19), strings.Repeat(`a{1000}`, 5),
		`.*`, `(?s).{5,10}`, `[a-z]+`, `\pL`, `\pL{3}`, `\p{Greek}{2,4}`, `[[:alpha:]]`, `[[:alpha:][:digit:]]{2}`, `\w+\d*\s?`,
		`(?i)\pL`, `[^a]`, `(a|b)*c`, `^(a+)+$`, `(foo|bar|baz)+?`, `(?:(?:a|b){2}){3}`, `\b\w+\b`, `(x*)*y`, `(?:x?){20}`,
		`[\p{L}\p{N}]{2}`, `(?i)[a-z]{10}`,
	}
	for _, p := range patterns {
		parsed, err := syntax.Parse(p, syntax.Perl)
		require.NoError(t, err, p)
		insts, runes := regexCost(parsed)
		insts += regexProgOverhead
		prog, err := syntax.Compile(parsed.Simplify())
		require.NoError(t, err, p)
		var progRunes int64
		for i := range prog.Inst {
			progRunes += int64(len(prog.Inst[i].Rune))
		}
		assert.GreaterOrEqual(t, insts, int64(len(prog.Inst)), "instructions: %s", p)
		assert.GreaterOrEqual(t, runes, progRunes, "runes: %s", p)
	}
}

// The fixture is under MaxRegexpSource, so the source-length gate cannot be what
// refuses it: only the Unicode pre-parse estimate can (deleting that estimate
// makes this test fail on allocation).
func TestRegexAllowanceUnicodeClassesAreRefusedBeforeParsing(t *testing.T) {
	p := strings.Repeat(`\pL`, 10000)
	require.Less(t, len(p), MaxRegexpSource, "prerequisite: the source-length gate must not apply")
	var err error
	grown := allocDuring(func() { _, err = NewRegexAllowance(nil).Compile(p) })
	var tc *RegexTooComplexError
	require.ErrorAs(t, err, &tc)
	assert.Contains(t, tc.Reason, "Unicode")
	assert.Less(t, grown, int64(4<<20), "the parser must not build 10,000 rune tables")

	// A handful of classes is fine and matches.
	re, err := NewRegexAllowance(nil).Compile(`^\pL+\pN$`)
	require.NoError(t, err)
	assert.True(t, re.MatchString("abc1"))
}

// Literal text is not a class: quoted, escaped-backslash and out-of-class
// look-alikes keep matching.
func TestRegexPreflightPreservesLiterals(t *testing.T) {
	quoted := `\Q` + strings.Repeat(`\p`, 77) + `\E`
	re, err := NewRegexAllowance(nil).Compile(quoted)
	require.NoError(t, err)
	assert.True(t, re.MatchString(strings.Repeat(`\p`, 77)))

	escapedBackslash := strings.Repeat(`\\p`, 200) // a literal backslash then "p", 200 times
	re, err = NewRegexAllowance(nil).Compile(escapedBackslash)
	require.NoError(t, err)
	assert.True(t, re.MatchString(strings.Repeat(`\p`, 200)))

	outsideClass := strings.Repeat(`\Q[:\E`, 100) // "[:" only as quoted text
	_, err = NewRegexAllowance(nil).Compile(outsideClass)
	require.NoError(t, err)

	assert.Equal(t, int64(0), preParseRunes(quoted))
	assert.Equal(t, int64(0), preParseRunes(escapedBackslash))
	assert.Equal(t, int64(unicodeClassRunes), preParseRunes(`[[:alpha:]]`))
	assert.Equal(t, int64(unicodeClassRunes), preParseRunes(`[\p{L}]`))
	assert.Equal(t, int64(0), preParseRunes(`a[:b`)) // "[:" opens a class here, not a POSIX name
	assert.Equal(t, int64(unicodeClassRunes), preParseRunes(`\\\pL`))
}

// A failed parse still spent its work and its outcome is cached: the same
// invalid pattern is parsed once, and distinct ones run the aggregate dry.
func TestRegexAllowanceChargesAndCachesFailedParses(t *testing.T) {
	bad := strings.Repeat(`\pL`, 75) + `)`
	require.Less(t, len(bad), MaxRegexpSource)
	a := NewRegexAllowance(nil)
	_, first := a.Compile(bad)
	require.Error(t, first)
	spent := a.runes
	assert.GreaterOrEqual(t, spent, int64(75*unicodeClassRunes), "the parse work is kept on failure")
	grown := allocDuring(func() {
		for i := 0; i < 1000; i++ {
			_, err := a.Compile(bad)
			assert.Equal(t, first, err)
		}
	})
	assert.Equal(t, spent, a.runes, "repeats are free")
	assert.Less(t, grown, int64(1<<20))

	// Distinct invalid patterns exhaust the aggregate instead of parsing forever.
	b := NewRegexAllowance(nil)
	refused := 0
	for i := 0; i < 60; i++ {
		_, err := b.Compile(fmt.Sprintf("%s%d)", strings.Repeat(`\pL`, 75), i))
		var tc *RegexTooComplexError
		if errors.As(err, &tc) {
			refused++
		}
	}
	assert.Greater(t, refused, 30)
}

func TestRegexAllowanceIsPerOperationAndCaches(t *testing.T) {
	a := NewRegexAllowance(nil)
	base := strings.Repeat("a{1000}", 19)
	var refused int
	for i := 0; i < 40; i++ {
		_, err := a.Compile(fmt.Sprintf("%s%d", base, i))
		var tc *RegexTooComplexError
		if err != nil {
			require.ErrorAs(t, err, &tc)
			refused++
		}
	}
	assert.Greater(t, refused, 25, "about ten 19,000-instruction patterns fit; the rest are refused")
	// A refused aggregate stays refused, even for a cheap pattern.
	_, err := a.Compile(`a`)
	assert.Error(t, err)
	// An admitted source is served from the cache after the aggregate is spent.
	b := NewRegexAllowance(nil)
	re1, err := b.Compile(`^x+$`)
	require.NoError(t, err)
	re2, err := b.Compile(`^x+$`)
	require.NoError(t, err)
	assert.Same(t, re1, re2)
}

func TestRegexAllowanceChecksContextBeforeEachCompile(t *testing.T) {
	cctx, cancel := context.WithCancel(context.Background())
	a := NewRegexAllowance(cctx)
	_, err := a.Compile(`a`)
	require.NoError(t, err)
	cancel()
	_, err = a.Compile(`b`)
	assert.ErrorIs(t, err, context.Canceled)
}

// Runtime: hundreds of individually admitted patterns end a budgeted render
// (previously only the first 256 were cached and the rest recompiled forever).
func TestManyConditionalPatternsAreBoundedAtRuntimeAndInLint(t *testing.T) {
	base := strings.Repeat("a{1000}", 19)
	var sb strings.Builder
	for i := 0; i < 300; i++ {
		fmt.Fprintf(&sb, `[conditional path="a" matches="%s%d"]Y[/conditional]`, base, i)
	}
	in := sb.String()
	ctx := MetaShortcodeContext{Meta: []byte(`{"a":"x"}`)}
	var err error
	grown := allocDuring(func() {
		_, err = ProcessChecked(WithRenderBudget(context.Background(), RenderBudget{OutputBytes: 1 << 20, ParsedSourceBytes: 1 << 20}), in, ctx, nil, nil)
	})
	require.ErrorIs(t, err, ErrRenderBudgetExceeded)
	assert.Less(t, grown, int64(256<<20))

	// Lint reports the refused ones once the aggregate is spent, without
	// compiling them.
	var issues []LintIssue
	grown = allocDuring(func() { issues = Lint(in, LintOptions{Known: KnownFromBuiltins()}) })
	tooComplex := 0
	for _, i := range issues {
		if strings.Contains(i.Message, "too complex") {
			tooComplex++
		}
	}
	assert.Greater(t, tooComplex, 200)
	assert.Less(t, grown, int64(256<<20))
}

// Round 5: a literal "[:" inside a class is not POSIX; only complete tokens are.
func TestRegexPreflightOnlyCountsCompletePOSIXTokens(t *testing.T) {
	lit := `[a` + strings.Repeat(`[:`, 77) + `b]`
	re, err := NewRegexAllowance(nil).Compile(lit)
	require.NoError(t, err, "valid and cheap")
	assert.True(t, re.MatchString("a"))
	assert.Equal(t, int64(0), preParseRunes(lit))

	// Complete tokens count, including the negated form, and their "]" does not
	// close the class.
	assert.Equal(t, int64(3*unicodeClassRunes), preParseRunes(`[[:alpha:][:^digit:][:word:]x]`))
	// A token-shaped unknown name is a literal run.
	assert.Equal(t, int64(0), preParseRunes(`[[:nope:]]`))
	// After the first token the class is still open: a following \p counts.
	assert.Equal(t, int64(2*unicodeClassRunes), preParseRunes(`[[:alpha:]\pL]`))

	real := `^[[:alpha:][:^digit:][:punct:]]+[[:xdigit:]]$`
	re, err = NewRegexAllowance(nil).Compile(real)
	require.NoError(t, err)
	assert.True(t, re.MatchString("ab-f"))
}
