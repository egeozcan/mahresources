package shortcodes

import (
	"fmt"
	"html"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// [calc] evaluates a small, closed arithmetic language over values the render
// already holds: the entity's Meta, its struct fields and the current [each]
// element. It never queries and never calls a plugin, so it reads nothing the
// page had not loaded for this reader; see
// docs/plans/2026-11-24-shortcode-calc.md for the grammar and its reasons.
//
// Every bound below is a constant, so the work one [calc] can do is fixed: one
// pass over at most calcMaxExprBytes of source, at most calcMaxTokens AST nodes,
// one walk of them.
const (
	calcMaxExprBytes = 512
	calcMaxTokens    = 128
	calcMaxDepth     = 32
	calcMaxRefs      = 32
	calcMaxLiteral   = 32
	calcMaxArgs      = 8
	calcMaxDecimals  = 10
	// calcMaxNumericString bounds a Meta string read as a number.
	calcMaxNumericString = 64
	calcAutoDecimals     = 10
)

// calcNumericString is what a Meta string must look like to count as a number.
// Anchored and fixed, and applied to operand values, never to the expression.
var calcNumericString = regexp.MustCompile(`^[+-]?(\d+(\.\d*)?|\.\d+)([eE][+-]?\d+)?$`)

type calcTokKind int

const (
	calcTokNum calcTokKind = iota
	calcTokName
	calcTokOp
)

type calcToken struct {
	kind calcTokKind
	text string
	pos  int
}

// calcRef is one operand reference: root is "meta", "property" or "item", and
// path the dot path after it ("" for a bare item).
type calcRef struct {
	root string
	path string
}

type calcNodeKind int

const (
	calcNodeNum calcNodeKind = iota
	calcNodeRef
	calcNodeNeg
	calcNodeBin
	calcNodeCall
)

type calcNode struct {
	kind calcNodeKind
	num  float64
	ref  calcRef
	op   byte
	fn   string
	args []*calcNode
}

// calcProgram is a parsed expression. refs lists every reference in source
// order, which is what lint checks against the schema.
type calcProgram struct {
	root *calcNode
	refs []calcRef
}

var calcFuncArity = map[string][2]int{
	"min":      {1, calcMaxArgs},
	"max":      {1, calcMaxArgs},
	"abs":      {1, 1},
	"round":    {1, 2},
	"coalesce": {1, calcMaxArgs},
}

func calcTokenize(expr string) ([]calcToken, error) {
	if len(expr) > calcMaxExprBytes {
		return nil, fmt.Errorf("expression is %d bytes, more than the %d allowed", len(expr), calcMaxExprBytes)
	}
	var toks []calcToken
	add := func(tk calcToken) error {
		if len(toks) == calcMaxTokens {
			return fmt.Errorf("expression has more than %d tokens", calcMaxTokens)
		}
		toks = append(toks, tk)
		return nil
	}
	for i := 0; i < len(expr); {
		c := expr[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9':
			start := i
			for i < len(expr) && expr[i] >= '0' && expr[i] <= '9' {
				i++
			}
			if i < len(expr) && expr[i] == '.' {
				i++
				if i >= len(expr) || expr[i] < '0' || expr[i] > '9' {
					return nil, fmt.Errorf("number at offset %d needs a digit after the decimal point", start)
				}
				for i < len(expr) && expr[i] >= '0' && expr[i] <= '9' {
					i++
				}
			}
			if i-start > calcMaxLiteral {
				return nil, fmt.Errorf("number at offset %d is longer than %d characters", start, calcMaxLiteral)
			}
			if err := add(calcToken{kind: calcTokNum, text: expr[start:i], pos: start}); err != nil {
				return nil, err
			}
		case c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z':
			start := i
			for i < len(expr) && (calcIdentByte(expr[i]) || expr[i] == '.') {
				i++
			}
			if err := add(calcToken{kind: calcTokName, text: expr[start:i], pos: start}); err != nil {
				return nil, err
			}
		case strings.IndexByte("+-*/%(),", c) >= 0:
			if err := add(calcToken{kind: calcTokOp, text: expr[i : i+1], pos: i}); err != nil {
				return nil, err
			}
			i++
		default:
			r := []rune(expr[i:min(i+4, len(expr))])[0]
			return nil, fmt.Errorf("unexpected character %q at offset %d", r, i)
		}
	}
	return toks, nil
}

func calcIdentByte(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}

type calcParser struct {
	toks []calcToken
	i    int
	refs []calcRef
}

// parseCalcExpr parses expr into a program, or returns the author error.
func parseCalcExpr(expr string) (*calcProgram, error) {
	toks, err := calcTokenize(expr)
	if err != nil {
		return nil, err
	}
	if len(toks) == 0 {
		return nil, fmt.Errorf("expression is empty")
	}
	p := &calcParser{toks: toks}
	root, err := p.expr(0)
	if err != nil {
		return nil, err
	}
	if p.i < len(p.toks) {
		tk := p.toks[p.i]
		return nil, fmt.Errorf("unexpected %q at offset %d", tk.text, tk.pos)
	}
	return &calcProgram{root: root, refs: p.refs}, nil
}

func (p *calcParser) peekOp(ops string) (byte, bool) {
	if p.i >= len(p.toks) || p.toks[p.i].kind != calcTokOp {
		return 0, false
	}
	c := p.toks[p.i].text[0]
	return c, strings.IndexByte(ops, c) >= 0
}

func (p *calcParser) expr(depth int) (*calcNode, error) {
	left, err := p.term(depth)
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.peekOp("+-")
		if !ok {
			return left, nil
		}
		p.i++
		right, err := p.term(depth)
		if err != nil {
			return nil, err
		}
		left = &calcNode{kind: calcNodeBin, op: op, args: []*calcNode{left, right}}
	}
}

func (p *calcParser) term(depth int) (*calcNode, error) {
	left, err := p.unary(depth)
	if err != nil {
		return nil, err
	}
	for {
		op, ok := p.peekOp("*/%")
		if !ok {
			return left, nil
		}
		p.i++
		right, err := p.unary(depth)
		if err != nil {
			return nil, err
		}
		left = &calcNode{kind: calcNodeBin, op: op, args: []*calcNode{left, right}}
	}
}

func (p *calcParser) unary(depth int) (*calcNode, error) {
	op, ok := p.peekOp("+-")
	if !ok {
		return p.primary(depth)
	}
	if depth+1 > calcMaxDepth {
		return nil, fmt.Errorf("expression nests more than %d levels deep", calcMaxDepth)
	}
	p.i++
	operand, err := p.unary(depth + 1)
	if err != nil {
		return nil, err
	}
	if op == '+' {
		return operand, nil
	}
	return &calcNode{kind: calcNodeNeg, args: []*calcNode{operand}}, nil
}

func (p *calcParser) primary(depth int) (*calcNode, error) {
	if p.i >= len(p.toks) {
		return nil, fmt.Errorf("expression ends where a value was expected")
	}
	tk := p.toks[p.i]
	switch tk.kind {
	case calcTokNum:
		p.i++
		v, err := strconv.ParseFloat(tk.text, 64)
		if err != nil || math.IsInf(v, 0) {
			return nil, fmt.Errorf("number %q at offset %d is out of range", tk.text, tk.pos)
		}
		return &calcNode{kind: calcNodeNum, num: v}, nil
	case calcTokName:
		p.i++
		if _, isFunc := calcFuncArity[tk.text]; isFunc {
			if op, ok := p.peekOp("("); ok && op == '(' {
				return p.call(tk, depth)
			}
			return nil, fmt.Errorf("%s at offset %d is a function: write %s(…)", tk.text, tk.pos, tk.text)
		}
		ref, err := calcParseRef(tk)
		if err != nil {
			return nil, err
		}
		if len(p.refs) == calcMaxRefs {
			return nil, fmt.Errorf("expression has more than %d references", calcMaxRefs)
		}
		p.refs = append(p.refs, ref)
		return &calcNode{kind: calcNodeRef, ref: ref}, nil
	default:
		if tk.text == "(" {
			if depth+1 > calcMaxDepth {
				return nil, fmt.Errorf("expression nests more than %d levels deep", calcMaxDepth)
			}
			p.i++
			inner, err := p.expr(depth + 1)
			if err != nil {
				return nil, err
			}
			if op, ok := p.peekOp(")"); !ok || op != ')' {
				return nil, fmt.Errorf("missing ) for the ( at offset %d", tk.pos)
			}
			p.i++
			return inner, nil
		}
		return nil, fmt.Errorf("unexpected %q at offset %d where a value was expected", tk.text, tk.pos)
	}
}

func (p *calcParser) call(name calcToken, depth int) (*calcNode, error) {
	if depth+1 > calcMaxDepth {
		return nil, fmt.Errorf("expression nests more than %d levels deep", calcMaxDepth)
	}
	p.i++ // "("
	var args []*calcNode
	if op, ok := p.peekOp(")"); !ok || op != ')' {
		for {
			if len(args) == calcMaxArgs {
				return nil, fmt.Errorf("%s at offset %d takes at most %d arguments", name.text, name.pos, calcMaxArgs)
			}
			arg, err := p.expr(depth + 1)
			if err != nil {
				return nil, err
			}
			args = append(args, arg)
			if op, ok := p.peekOp(","); ok && op == ',' {
				p.i++
				continue
			}
			break
		}
	}
	if op, ok := p.peekOp(")"); !ok || op != ')' {
		return nil, fmt.Errorf("missing ) for %s( at offset %d", name.text, name.pos)
	}
	p.i++
	arity := calcFuncArity[name.text]
	if len(args) < arity[0] || len(args) > arity[1] {
		want := strconv.Itoa(arity[0])
		if arity[1] != arity[0] {
			want += " to " + strconv.Itoa(arity[1])
		}
		return nil, fmt.Errorf("%s at offset %d takes %s arguments, not %d", name.text, name.pos, want, len(args))
	}
	return &calcNode{kind: calcNodeCall, fn: name.text, args: args}, nil
}

// calcParseRef validates a dotted name token as a reference.
func calcParseRef(tk calcToken) (calcRef, error) {
	segs := strings.Split(tk.text, ".")
	root := segs[0]
	switch root {
	case "meta", "property":
		if len(segs) < 2 {
			return calcRef{}, fmt.Errorf("%s at offset %d needs a field: write %s.<name>", root, tk.pos, root)
		}
	case "item":
	default:
		return calcRef{}, fmt.Errorf("unknown name %q at offset %d: a reference starts with meta., property. or item, and the functions are min, max, abs, round and coalesce", tk.text, tk.pos)
	}
	for _, seg := range segs[1:] {
		if seg == "" {
			return calcRef{}, fmt.Errorf("%q at offset %d has an empty path segment", tk.text, tk.pos)
		}
		if root == "property" && isNumericSegment(seg) {
			continue
		}
		if seg[0] >= '0' && seg[0] <= '9' {
			return calcRef{}, fmt.Errorf("%q at offset %d: a %s path segment cannot start with a digit", tk.text, tk.pos, root)
		}
	}
	return calcRef{root: root, path: strings.Join(segs[1:], ".")}, nil
}

// calcEnv is what a reference can read: the render's context and, inside
// [each], the current element.
type calcEnv struct {
	ctx     MetaShortcodeContext
	elem    any
	hasElem bool
}

// eval returns the node's value, or ok=false for "no value": a missing or
// non-numeric operand, or a NaN/Inf anywhere on the way.
func (e calcEnv) eval(n *calcNode) (float64, bool) {
	var v float64
	switch n.kind {
	case calcNodeNum:
		return n.num, true
	case calcNodeRef:
		return e.resolve(n.ref)
	case calcNodeNeg:
		x, ok := e.eval(n.args[0])
		if !ok {
			return 0, false
		}
		v = -x
	case calcNodeBin:
		a, ok := e.eval(n.args[0])
		if !ok {
			return 0, false
		}
		b, ok := e.eval(n.args[1])
		if !ok {
			return 0, false
		}
		switch n.op {
		case '+':
			v = a + b
		case '-':
			v = a - b
		case '*':
			v = a * b
		case '/':
			v = a / b
		case '%':
			v = math.Mod(a, b)
		}
	case calcNodeCall:
		return e.call(n)
	}
	return v, calcFinite(v)
}

func (e calcEnv) call(n *calcNode) (float64, bool) {
	if n.fn == "coalesce" {
		for _, arg := range n.args {
			if v, ok := e.eval(arg); ok {
				return v, true
			}
		}
		return 0, false
	}
	vals := make([]float64, len(n.args))
	for i, arg := range n.args {
		v, ok := e.eval(arg)
		if !ok {
			return 0, false
		}
		vals[i] = v
	}
	var v float64
	switch n.fn {
	case "min", "max":
		v = vals[0]
		for _, x := range vals[1:] {
			if n.fn == "min" {
				v = math.Min(v, x)
			} else {
				v = math.Max(v, x)
			}
		}
	case "abs":
		v = math.Abs(vals[0])
	case "round":
		d := 0
		if len(vals) == 2 {
			if vals[1] != math.Trunc(vals[1]) || vals[1] < 0 || vals[1] > calcMaxDecimals {
				return 0, false
			}
			d = int(vals[1])
		}
		r, err := strconv.ParseFloat(calcRoundString(vals[0], d), 64)
		if err != nil {
			return 0, false
		}
		v = r
	}
	return v, calcFinite(v)
}

func (e calcEnv) resolve(r calcRef) (float64, bool) {
	switch r.root {
	case "meta":
		v, _ := navigateJSONValue(e.ctx.decodeMeta(), r.path)
		return calcJSONNumber(v)
	case "item":
		if !e.hasElem {
			return 0, false
		}
		v, _ := navigateJSONValue(e.elem, r.path)
		return calcJSONNumber(v)
	case "property":
		if e.ctx.Entity == nil {
			return 0, false
		}
		field, ok := traversePropertyPath(e.ctx.Entity, r.path, e.ctx.credentialsVisible)
		if !ok {
			return 0, false
		}
		return calcFieldNumber(field)
	}
	return 0, false
}

// calcJSONNumber reads a decoded JSON value as a number: a number, or a string
// that is wholly a decimal number. Nothing else is one.
func calcJSONNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, calcFinite(x)
	case string:
		// No finite float64 needs more bytes than this, and the cap keeps the
		// work per operand fixed however long a stored string is.
		if len(x) > calcMaxNumericString {
			return 0, false
		}
		s := strings.TrimSpace(x)
		if !calcNumericString.MatchString(s) {
			return 0, false
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil || !calcFinite(f) {
			return 0, false
		}
		return f, true
	}
	return 0, false
}

// calcFieldNumber reads a struct field as a number. Only numeric kinds count; a
// string field is text, exactly as it is to [property].
func calcFieldNumber(v reflect.Value) (float64, bool) {
	for v.Kind() == reflect.Ptr || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return 0, false
		}
		v = v.Elem()
	}
	switch v.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(v.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return float64(v.Uint()), true
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		return f, calcFinite(f)
	}
	return 0, false
}

func calcFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// calcRoundString rounds v half away from zero to d fraction digits and writes
// it in plain decimal notation, with exactly d fraction digits. The rounding is
// done on the shortest decimal string that reads back as v, so 2.675 (stored as
// 2.67499999…) rounds to 2.68, as the author who wrote 2.675 expects. A result
// that is zero carries no sign.
func calcRoundString(v float64, d int) string {
	s := strconv.FormatFloat(math.Abs(v), 'f', -1, 64)
	intPart, frac, _ := strings.Cut(s, ".")
	digits := []byte(intPart + frac)
	n := len(intPart) + d // digits kept
	if len(frac) > d {
		roundUp := frac[d] >= '5'
		digits = digits[:n]
		if roundUp {
			i := n - 1
			for ; i >= 0; i-- {
				if digits[i] == '9' {
					digits[i] = '0'
					continue
				}
				digits[i]++
				break
			}
			if i < 0 {
				digits = append([]byte{'1'}, digits...)
				n++
			}
		}
	} else {
		for len(digits) < n {
			digits = append(digits, '0')
		}
	}
	intDigits := string(digits[:n-d])
	fracDigits := string(digits[n-d:])

	out := intDigits
	if d > 0 {
		out += "." + fracDigits
	}
	if v < 0 && strings.Trim(intDigits+fracDigits, "0") != "" {
		out = "-" + out
	}
	return out
}

// calcFormat writes v per decimals (-1: automatic) and format.
func calcFormat(v float64, decimals int, format string) (string, bool) {
	suffix := ""
	if format == "percent" {
		v *= 100
		if !calcFinite(v) {
			return "", false
		}
		suffix = "%"
	}
	var s string
	if decimals < 0 {
		s = calcRoundString(v, calcAutoDecimals)
		if strings.Contains(s, ".") {
			s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
		}
	} else {
		s = calcRoundString(v, decimals)
	}
	if format == "grouped" {
		s = calcGroup(s)
	}
	return s + suffix, true
}

// calcGroup puts a comma between every three integer digits of a plain
// decimal string.
func calcGroup(s string) string {
	sign := ""
	if strings.HasPrefix(s, "-") {
		sign, s = "-", s[1:]
	}
	intPart, frac, hasFrac := strings.Cut(s, ".")
	var b strings.Builder
	for i := range len(intPart) {
		if i > 0 && (len(intPart)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteByte(intPart[i])
	}
	out := sign + b.String()
	if hasFrac {
		out += "." + frac
	}
	return out
}

// calcOptions validates decimals= and format=. decimals is -1 when unset.
func calcOptions(attrs map[string]string) (decimals int, format string, err error) {
	decimals = -1
	if raw, set := attrs["decimals"]; set {
		s := strings.TrimSpace(raw)
		n, convErr := strconv.Atoi(s)
		if !isNumericSegment(s) || convErr != nil || n > calcMaxDecimals {
			return 0, "", fmt.Errorf("decimals must be a whole number from 0 to %d, not %q", calcMaxDecimals, raw)
		}
		decimals = n
	}
	format = strings.TrimSpace(attrs["format"])
	switch format {
	case "", "number", "grouped", "percent":
	default:
		return 0, "", fmt.Errorf(`format must be "number", "grouped" or "percent", not %q`, attrs["format"])
	}
	return decimals, format, nil
}

// RenderCalcShortcode expands an inline [calc] outside an [each] element: its
// item operands have no value.
func RenderCalcShortcode(sc Shortcode, ctx MetaShortcodeContext) string {
	return renderCalc(ctx, sc, nil, false)
}

// renderCalc evaluates one [calc]. hasElem says elem is the current [each]
// element; the each handler calls it at the token's sentinel.
func renderCalc(ctx MetaShortcodeContext, sc Shortcode, elem any, hasElem bool) string {
	expr := sc.Attrs["expr"]
	if strings.TrimSpace(expr) == "" {
		return ""
	}
	decimals, format, err := calcOptions(sc.Attrs)
	if err != nil {
		return shortcodeErrorMarker("calc", err.Error())
	}
	prog, err := parseCalcExpr(expr)
	if err != nil {
		detail := "expr: " + err.Error()
		if len(expr) <= calcMaxExprBytes {
			detail += " in " + strconv.Quote(expr)
		}
		return shortcodeErrorMarker("calc", detail)
	}
	if v, ok := (calcEnv{ctx: ctx, elem: elem, hasElem: hasElem}).eval(prog.root); ok {
		if text, ok := calcFormat(v, decimals, format); ok {
			if ctx.scalarObserver != nil {
				ctx.scalarObserver(text)
			}
			return text
		}
	}
	return html.EscapeString(sc.Attrs["default"])
}
