package shortcodes

import (
	"regexp"
	"regexp/syntax"
)

// A conditional's matches= pattern is model- or author-controlled. regexp.Compile
// simplifies counted repetition and compiles the expansion, so a short pattern
// such as ((a{1000}){1000}){1000} would build a program far larger than its
// source before control returns. The pattern is parsed first (parsing is bounded
// by the pattern), the program size it would compile to is computed on the parse
// tree with saturating arithmetic, and only a pattern under the allowance is
// compiled.
const (
	maxRegexSourceBytes = 1024
	maxRegexProgramSize = 20_000
)

type regexTooComplexError struct{ pattern string }

func (e *regexTooComplexError) Error() string {
	return "the pattern is too complex to compile within the allowance"
}

// compileBoundedRegexp compiles pattern with regexp.Compile semantics when its
// compiled size is within the allowance. A pattern that does not parse returns
// the parse error (callers treat it as never matching, as before); one over the
// allowance returns *regexTooComplexError.
func compileBoundedRegexp(pattern string) (*regexp.Regexp, error) {
	if len(pattern) > maxRegexSourceBytes {
		return nil, &regexTooComplexError{pattern}
	}
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, err
	}
	if regexProgramSize(re) > maxRegexProgramSize {
		return nil, &regexTooComplexError{pattern}
	}
	return regexp.Compile(pattern)
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

// regexProgramSize estimates the instruction count regexp compiles re to, after
// it expands x{n,m} into copies.
func regexProgramSize(re *syntax.Regexp) int64 {
	var sub int64
	for _, s := range re.Sub {
		sub = satAdd(sub, regexProgramSize(s))
	}
	switch re.Op {
	case syntax.OpLiteral:
		return int64(len(re.Rune))
	case syntax.OpCapture:
		return satAdd(sub, 2)
	case syntax.OpStar, syntax.OpPlus, syntax.OpQuest:
		return satAdd(sub, 1)
	case syntax.OpRepeat:
		copies := int64(re.Min)
		switch {
		case re.Max < 0:
			copies = satAdd(copies, 1)
		default:
			copies = satAdd(copies, int64(re.Max-re.Min))
		}
		return satAdd(satMul(sub, max(copies, 1)), 1)
	case syntax.OpAlternate:
		return satAdd(sub, int64(len(re.Sub)))
	case syntax.OpConcat:
		return sub
	}
	return 1
}

// regexp returns the compiled pattern, cached for the rest of the render when a
// meter is present. A pattern over the allowance ends the render with the
// budget error and reports ok=false with a nil regexp; a pattern that does not
// parse reports ok=true with a nil regexp (it never matches).
func (m *renderMeter) regexp(pattern string) (re *regexp.Regexp, ok bool) {
	if m != nil {
		m.mu.Lock()
		if c, hit := m.regexes[pattern]; hit {
			m.mu.Unlock()
			return c, true
		}
		m.mu.Unlock()
	}
	re, err := compileBoundedRegexp(pattern)
	var tooComplex *regexTooComplexError
	if asTooComplex(err, &tooComplex) {
		m.fail("a matches= pattern exceeds the regular expression allowance")
		return nil, false
	}
	if err != nil {
		re = nil
	}
	if m != nil {
		m.mu.Lock()
		if m.regexes == nil {
			m.regexes = map[string]*regexp.Regexp{}
		}
		if len(m.regexes) < 256 {
			m.regexes[pattern] = re
		}
		m.mu.Unlock()
	}
	return re, true
}

func asTooComplex(err error, target **regexTooComplexError) bool {
	if e, ok := err.(*regexTooComplexError); ok {
		*target = e
		return true
	}
	return false
}

// AdmitRegexp reports whether pattern is within the allowance for compiling a
// regular expression, for callers (a MetaSchema's pattern and patternProperties)
// whose own compiler would otherwise expand it unchecked. A pattern that does
// not parse is admitted: the caller's compiler reports that in its own words.
func AdmitRegexp(pattern string) error {
	if _, err := compileBoundedRegexp(pattern); err != nil {
		var tc *regexTooComplexError
		if asTooComplex(err, &tc) {
			return err
		}
	}
	return nil
}
