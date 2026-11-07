package shortcodes

import (
	"context"
	"errors"
	"unicode/utf8"
)

// The schema expansion helpers run on schemas the author or a model wrote. Each
// pass shares one schemaWork: it counts node visits and merged entries, charges
// the bytes an expansion would serialize, follows the $ref chain of the current
// path, and checks the context. Nothing here limits an ordinary schema; it ends
// a pathological one (a reference per level fanning out, large descriptions
// repeated under many references) instead of letting it multiply.
const (
	schemaWorkUnits = 100_000
	schemaMaxDepth  = 32
	schemaWorkBytes = 1 << 20
)

// errSchemaTooComplex is what an exhausted schemaWork reports; lint turns it
// into one error issue, a budgeted render into ErrRenderBudgetExceeded.
var errSchemaTooComplex = errors.New("the schema is too complex to check")

type schemaWork struct {
	ctx      context.Context
	units    int
	bytes    int
	maxUnits int
	maxBytes int
	steps    uint32
	err      error
	refs     map[string]int // $ref pointers on the path being expanded
}

func newSchemaWork(ctx context.Context) *schemaWork {
	return &schemaWork{ctx: ctx, refs: map[string]int{}, maxUnits: schemaWorkUnits, maxBytes: schemaWorkBytes}
}

// Lint allowances. A template lint is bounded by the source it is given; these
// only stop a source built to make a quadratic pass worse (many numbered
// attributes on one opener, thousands of candidate positions).
const (
	lintWorkUnits = 5_000_000
	lintWorkBytes = 64 << 20
)

func newLintWork(ctx context.Context) *schemaWork {
	w := newSchemaWork(ctx)
	w.maxUnits, w.maxBytes = lintWorkUnits, lintWorkBytes
	return w
}

func (w *schemaWork) fail() {
	if w.err == nil {
		w.err = errSchemaTooComplex
	}
}

// Err is the sticky exhaustion (or cancellation) error.
func (w *schemaWork) Err() error {
	if w == nil {
		return nil
	}
	return w.err
}

// ok reports whether work may continue.
func (w *schemaWork) ok() bool { return w == nil || w.err == nil }

// charge reserves n work units before the work they stand for.
func (w *schemaWork) charge(n int) bool {
	if w == nil {
		return true
	}
	if w.err != nil {
		return false
	}
	if n < 0 || w.units > w.maxUnits-n {
		w.fail()
		return false
	}
	w.units += n
	return w.step()
}

// poll consults the context now, for operation entry and exit: the amortized
// check in step would let a small input finish before it ever looked.
func (w *schemaWork) poll() bool {
	if w == nil {
		return true
	}
	if w.err == nil && w.ctx != nil {
		if cerr := w.ctx.Err(); cerr != nil {
			w.err = errors.Join(errSchemaTooComplex, cerr)
		}
	}
	return w.err == nil
}

// step counts one iteration and consults the context every 64th.
func (w *schemaWork) step() bool {
	if w == nil {
		return true
	}
	w.steps++
	if w.ctx != nil && w.steps&0x3f == 0 {
		if cerr := w.ctx.Err(); cerr != nil && w.err == nil {
			w.err = errors.Join(errSchemaTooComplex, cerr)
		}
	}
	return w.err == nil
}

// chargeBytes reserves n bytes of expanded schema before they are copied or
// serialized.
func (w *schemaWork) chargeBytes(n int) bool {
	if w == nil {
		return true
	}
	if w.err != nil {
		return false
	}
	if n < 0 || w.bytes > w.maxBytes-n {
		w.fail()
		return false
	}
	w.bytes += n
	return true
}

// deep reports whether depth is past the cap, failing the work when it is.
func (w *schemaWork) deep(depth int) bool {
	if w == nil {
		return false
	}
	if depth > schemaMaxDepth {
		w.fail()
		return true
	}
	return false
}

// enterRef marks ref as being expanded. It reports false, without failing,
// when ref is already on the current path: a cycle stops there.
func (w *schemaWork) enterRef(ref string) bool {
	if w == nil {
		return true
	}
	if w.refs[ref] > 0 {
		return false
	}
	w.refs[ref]++
	return true
}

func (w *schemaWork) leaveRef(ref string) {
	if w != nil {
		w.refs[ref]--
	}
}

// valueBytes charges the size a value serializes to, counting every occurrence
// (a shared subtree reached twice is serialized twice) and weighting characters
// that HTML-escaping lengthens, since the slice is embedded in an attribute.
func (w *schemaWork) valueBytes(v any) bool {
	if w == nil {
		return true
	}
	switch x := v.(type) {
	case string:
		return w.chargeBytes(escapedSize(x)) && w.step()
	case map[string]any:
		if !w.chargeBytes(2 * htmlQuote) {
			return false
		}
		for k, e := range x {
			// key, its quotes, the colon and the comma
			if !w.chargeBytes(escapedSize(k)+2) || !w.step() || !w.valueBytes(e) {
				return false
			}
		}
	case []any:
		if !w.chargeBytes(2) {
			return false
		}
		for _, e := range x {
			if !w.chargeBytes(1) || !w.step() || !w.valueBytes(e) {
				return false
			}
		}
	default:
		// A float64 encodes in up to 24 bytes (-1.2345678901234567e-308);
		// true, false and null in at most 5.
		return w.chargeBytes(24) && w.step()
	}
	return true
}

// htmlQuote is the length of a quote character once the JSON is embedded in an
// HTML attribute (&#34;).
const htmlQuote = 5

// escapedSize is an upper bound on a string's size after json.Marshal and the
// HTML attribute escaping that follows: the two quotes at htmlQuote each, then
// per byte. Controls and "<", ">", "&" become six-byte \u escapes; a quote or
// backslash two bytes and then up to five more; U+2028 and U+2029 (always
// escaped by encoding/json) and every byte of invalid UTF-8 (replaced by the
// six-byte \ufffd) six bytes; any other multibyte character its own length.
func escapedSize(s string) int {
	n := 2 * htmlQuote
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			switch {
			case c == '"' || c == '\\' || c == '\'':
				n += 7
			case c == '<' || c == '>' || c == '&' || c < 0x20:
				n += 6
			default:
				n++
			}
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			n += 6
		case r == '\u2028' || r == '\u2029':
			n += 6
		default:
			n += size
		}
		i += size
	}
	return n
}
