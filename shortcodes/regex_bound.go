package shortcodes

import (
	"context"
	"errors"
	"regexp"
	"regexp/syntax"
	"strings"
)

// A regular expression written by an author or a model can cost far more to
// compile than its source suggests: counted repetition is expanded into copies,
// and every \p{..} or [[:class:]] carries a rune table of over a thousand
// entries that each copy repeats. One RegexAllowance is the per-operation
// budget for all of it. The shortcode conditionals (runtime and lint) and the
// MetaSchema compiler's regexp engine all compile through one, so there is one
// rule: a conservative estimate from the source is charged before the pattern
// is parsed, the parse tree's instruction and rune cost (repetition counted as
// the compiler will expand it) before it is compiled, and the compiled program
// is measured and checked after. Admitted sources are cached for the operation
// and never charged twice; once the allowance is spent every further compile is
// refused.
const (
	// MaxRegexpProgramSize is the instruction allowance for one pattern.
	MaxRegexpProgramSize = 20_000
	// MaxRegexpRunes is the rune-table allowance for one pattern (4 bytes each).
	MaxRegexpRunes = 100_000
	// MaxRegexpSource bounds the pattern text parsed at all.
	MaxRegexpSource = 32 << 10

	regexAllowanceInsts = 200_000
	regexAllowanceRunes = 2_000_000

	// unicodeClassRunes is what one \p{..}, \P{..} or [[:name:]] expands to in
	// the worst case (\pL is 1,292 runes); the source scan charges it before
	// the parser can build it.
	unicodeClassRunes = 1_300
)

// RegexTooComplexError is a pattern (or an operation's patterns together) past
// the allowance.
type RegexTooComplexError struct{ Reason string }

func (e *RegexTooComplexError) Error() string {
	return "the regular expression is too complex to compile: " + e.Reason
}

// RegexAllowance is one operation's compile budget. It is not safe for
// concurrent use.
type RegexAllowance struct {
	ctx        context.Context
	insts      int64
	runes      int64
	compiled   map[string]*regexp.Regexp
	refusedErr error // sticky once the aggregate is spent
}

// NewRegexAllowance returns a fresh allowance. ctx may be nil.
func NewRegexAllowance(ctx context.Context) *RegexAllowance {
	return &RegexAllowance{ctx: ctx, compiled: map[string]*regexp.Regexp{}}
}

func (a *RegexAllowance) refuse(reason string) error {
	return &RegexTooComplexError{Reason: reason}
}

// Compile compiles pattern with regexp.Compile semantics within the allowance.
// A pattern that does not parse returns its syntax error. A pattern over the
// allowance (or any pattern once the aggregate is spent) returns
// *RegexTooComplexError; a done context returns its error.
func (a *RegexAllowance) Compile(pattern string) (*regexp.Regexp, error) {
	if a.ctx != nil {
		if err := a.ctx.Err(); err != nil {
			return nil, err
		}
	}
	if re, ok := a.compiled[pattern]; ok {
		return re, nil
	}
	if a.refusedErr != nil {
		return nil, a.refusedErr
	}
	fail := func(reason string) (*regexp.Regexp, error) {
		return nil, a.refuse(reason)
	}
	if len(pattern) > MaxRegexpSource {
		return fail("the pattern is longer than 32 KiB")
	}
	// Before parsing: what the parser itself would build for the class escapes.
	if est := int64(strings.Count(pattern, `\p`)+strings.Count(pattern, `\P`)+strings.Count(pattern, "[:")) * unicodeClassRunes; est > MaxRegexpRunes {
		return fail("too many Unicode classes")
	}
	parsed, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, err
	}
	insts, runes := regexCost(parsed)
	insts += regexProgOverhead
	if insts > MaxRegexpProgramSize || runes > MaxRegexpRunes {
		return fail("it would compile to more than the per-pattern allowance")
	}
	if a.insts+insts > regexAllowanceInsts || a.runes+runes > regexAllowanceRunes {
		a.refusedErr = a.refuse("the patterns of this operation add up past the allowance")
		return nil, a.refusedErr
	}
	// Measure the real program before compiling it into a Regexp.
	prog, err := syntax.Compile(parsed.Simplify())
	if err != nil {
		return nil, err
	}
	var progRunes int64
	for i := range prog.Inst {
		progRunes += int64(len(prog.Inst[i].Rune))
	}
	if int64(len(prog.Inst)) > MaxRegexpProgramSize || progRunes > MaxRegexpRunes {
		return fail("the compiled program is over the per-pattern allowance")
	}
	a.insts += max(insts, int64(len(prog.Inst)))
	a.runes += max(runes, progRunes)
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	a.compiled[pattern] = re
	return re, nil
}

const satCap = int64(1) << 40

func satAdd(a, b int64) int64 {
	if a+b > satCap {
		return satCap
	}
	return a + b
}

func satMul(a, b int64) int64 {
	if a == 0 || b == 0 {
		return 0
	}
	if a > satCap/b {
		return satCap
	}
	return a * b
}

// regexCost estimates, from the parse tree and with saturating arithmetic, the
// instructions and rune-table elements regexp compiles re to once x{n,m} is
// expanded: n copies plus m-n nested optional copies (each with its own
// alternation), x{n,} as n copies and a star, and a constant for the program's
// prologue and match. It never undercounts the compiled program
// (TestRegexCostBoundsCompiledProgram).
func regexCost(re *syntax.Regexp) (insts, runes int64) {
	var subI, subR int64
	for _, s := range re.Sub {
		i, r := regexCost(s)
		subI, subR = satAdd(subI, i), satAdd(subR, r)
	}
	switch re.Op {
	case syntax.OpLiteral:
		// One instruction per rune; under (?i) each also carries a fold table.
		return int64(len(re.Rune)), int64(len(re.Rune)) * 2
	case syntax.OpCharClass:
		return 1, int64(len(re.Rune))
	case syntax.OpCapture:
		return satAdd(subI, 2), subR
	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest:
		// Extra overhead for an empty-width loop body (Go rewrites (a*)* forms).
		return satAdd(subI, 3), subR
	case syntax.OpRepeat:
		min, max := int64(re.Min), int64(re.Max)
		var copies, optional int64
		switch {
		case max < 0:
			copies, optional = min+1, 1
		default:
			copies, optional = max, max-min
		}
		if copies < 1 {
			copies = 1
		}
		i := satAdd(satMul(subI, copies), satMul(optional, 3))
		return satAdd(i, 3), satMul(subR, copies)
	case syntax.OpAlternate:
		return satAdd(subI, int64(len(re.Sub))+1), subR
	case syntax.OpConcat:
		return satAdd(subI, 1), subR
	case syntax.OpAnyChar, syntax.OpAnyCharNotNL:
		return 1, 4
	}
	return 1, 0
}

// regexProgOverhead covers the program's own prologue (fail, the two capture
// instructions of the whole match) and its match instruction.
const regexProgOverhead = 8

// regexp returns the compiled pattern for a conditional, cached for the rest of
// the render by the meter's allowance. A pattern over the allowance ends a
// budgeted render with the budget error; on an unbudgeted page it is local to
// that conditional, which never matches, like an invalid pattern. A done
// context ends either.
func (m *renderMeter) regexp(pattern string) (re *regexp.Regexp, ok bool) {
	if m == nil {
		re, _ = NewRegexAllowance(nil).Compile(pattern)
		return re, true
	}
	m.mu.Lock()
	if m.regexAllowance == nil {
		m.regexAllowance = NewRegexAllowance(m.ctx)
	}
	allowance := m.regexAllowance
	m.mu.Unlock()
	re, err := allowance.Compile(pattern)
	var tooBig *RegexTooComplexError
	switch {
	case err == nil:
		return re, true
	case errors.As(err, &tooBig):
		if m.limited() {
			m.fail("a matches= pattern exceeds the regular expression allowance: %s", tooBig.Reason)
			return nil, false
		}
		return nil, true
	case errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded):
		m.failErr(err)
		return nil, false
	}
	return nil, true // does not parse: never matches
}
