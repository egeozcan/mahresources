package shortcodes

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Where a shortcode that emits a bare value lands in the markup around it,
// which decides whether escaping is enough. That is [meta inline="true"],
// [property], [item] and [mrql value=] — see emitsBareValue; the file talks
// about "[meta inline]" in places because it was the first of them.
//
// Finding that out means answering "which attribute of
// which tag is this inside", and four rounds of review established that a
// hand-written answer is wrong on the next case every time: a ">" inside a
// handler is not a tag end, a "<" inside one is not a tag start, a <script>
// body is not markup, "</scripture>" does not close a script, a comment ends at
// "-->" and not at ">", and "java&#x73;cript" is a scheme.
//
// The tag-finding is the parser's, not a hand-written model of it. Earlier
// versions of this file tracked the open-elements stack themselves and spent
// many review rounds discovering, one HTML5 insertion mode at a time, every
// place that model diverged from a browser — foreign content, <noscript>
// scripting modes, the adoption agency, document phases, template surrender.
// All of that is now answered by parsing the substituted source with
// golang.org/x/net/html directly, in both scripting modes, and reading each
// occurrence's placement out of the DOM: it is a browser's reading by
// construction. What is left to a flat lexical pass is only the handful of
// syntactic facts the DOM does not carry — whether an attribute value is
// quoted, whether it is a repeated (discarded) one, whether an interpolation
// forms a tag or attribute NAME, whether it sits in a tag that is never
// closed — and those, as the note above says, were never the problem.

// bareValueSpan is one bare-value shortcode occurrence in the template source.
type bareValueSpan struct{ start, end int }

// attrContext is where one occurrence falls in the markup around it.
type attrContext struct {
	attr string // lowercased attribute name; "" when not in an attribute value
	// quoted reports whether the value was delimited. An unquoted value ends at
	// the first space, so a Meta value containing one adds attributes.
	quoted     bool
	valueSoFar string // the attribute text before the occurrence, entity-decoded
	inValue    bool
	// inName marks an occurrence in a tag or attribute NAME, where nothing
	// delimits the value at all.
	inName bool
	// rawTextElement names the script- or style-like element whose body the
	// occurrence sits in, where escaping contains the value without making the
	// placement safe.
	rawTextElement string
	// element is the lowercased name of the tag the occurrence sits in, which a
	// couple of rules need because an attribute means what its element makes it
	// mean: srcdoc is defined on <iframe> and nowhere else.
	element string
	// foreignRoot is the namespace the occurrence's own element is in — "svg",
	// "math", or "" for HTML. It narrows two things. It inverts the rationale
	// for a program body, because foreign content DOES decode entities, so the
	// escaping is undone before the language sees the value rather than
	// arriving intact. And it withholds the rules that describe what one
	// particular HTML element does, because a foreign element is not that
	// element however it is spelled.
	foreignRoot string
	// noscript marks an occurrence inside a <noscript> body, which is markup
	// only when scripting is disabled. Nothing there can execute under either
	// reading, so the rules that describe execution are withheld.
	noscript bool
	// inertText marks an occurrence the parser placed in a text node the rules
	// have nothing to say about. It is the zero value as far as the rules are
	// concerned, and exists only to keep the "<"-name and unterminated-tag
	// fallbacks from re-answering a placement the parse already settled.
	inertText bool
	// discarded marks an occurrence in the value of a REPEATED attribute,
	// which the parser drops: nothing there reaches the DOM, and only raw= —
	// whose unescaped bytes still pass through the tokenizer and can close
	// the attribute — has anything left to warn about.
	discarded bool
	// unterminated marks an occurrence inside a tag that is never closed. The
	// parser recovers such a tag (or drops it), so this is a lexical fact;
	// treated as unsafe rather than as "not in an attribute", so a broken
	// template fails closed.
	unterminated bool
}

// lintSentinelNonce randomises the sentinel prefix per process so it cannot
// collide with bytes a template already contains.
var lintSentinelNonce = randomSentinelNonce()

func randomSentinelNonce() string {
	var b [6]byte
	if _, err := cryptorand.Read(b[:]); err != nil {
		return "0f0f0f0f0f0f"
	}
	return hex.EncodeToString(b[:])
}

// lintSentinelPrefix marks a substituted occurrence. It is NUL-free and all
// lowercase ASCII, so it survives html.Parse intact — in an attribute value,
// in text, and lowercased into an element or attribute name — where the old
// NUL-delimited sentinel was mangled (the parser rewrites NUL to U+FFFD). A
// per-process random middle makes a collision with real template bytes
// vanishingly unlikely, and the one that could still happen is neutralised in
// attributeContextsFor.
var lintSentinelPrefix = "mahlintx" + lintSentinelNonce + "x"

const lintSentinelSuffix = "z"

func lintSentinel(i int) string { return lintSentinelPrefix + strconv.Itoa(i) + lintSentinelSuffix }

// attributeContextsFor answers, for every occurrence, the attribute context of
// its default (scripting-enabled) reading, and — as a second map keyed the
// same way — the scripting-DISABLED reading for the occurrences whose two
// readings differ, so a caller that evaluates both catches a placement live in
// only one mode. The second map is empty unless the source contains a
// <noscript>.
//
// The tree facts — which element an occurrence sits in, its namespace, whether
// it is an attribute value or a program body, whether it is inside a
// <noscript> — are read from golang.org/x/net/html's own parser, so they are a
// browser's exactly and by construction rather than by a hand-written model of
// the tree constructor. The syntactic facts the DOM does not carry — whether
// an attribute value is quoted, whether it is a repeated (discarded) one,
// whether an occurrence interpolates a tag or attribute NAME, whether it sits
// in a tag that is never closed — are read from a flat lexical pass over the
// source, which is a bounded scan of one tag at a time with no nesting.
func attributeContextsFor(input string, spans []bareValueSpan) (map[int]attrContext, map[int]attrContext) {
	out := make(map[int]attrContext, len(spans))
	alt := make(map[int]attrContext)
	if len(spans) == 0 {
		return out, alt
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	// A template that already contains the sentinel prefix could otherwise
	// remap one occurrence onto another's context. The prefix carries a random
	// nonce, so this is close to impossible, but replacing it keeps every span
	// offset valid if it does happen.
	if strings.Contains(input, lintSentinelPrefix) {
		input = strings.ReplaceAll(input, lintSentinelPrefix, strings.Repeat(" ", len(lintSentinelPrefix)))
	}

	var b strings.Builder
	// sentinelAt[i] is where span i's sentinel begins in the substituted text,
	// or -1 for a span that was not substituted.
	sentinelAt := make([]int, len(spans))
	last := 0
	for i, sp := range spans {
		sentinelAt[i] = -1
		if sp.start < last || sp.end > len(input) || sp.start > sp.end {
			out[sp.start] = attrContext{}
			continue
		}
		b.WriteString(input[last:sp.start])
		sentinelAt[i] = b.Len()
		b.WriteString(lintSentinel(i))
		last = sp.end
		out[sp.start] = attrContext{}
	}
	b.WriteString(input[last:])
	substituted := b.String()

	// The syntactic layer: quoted, discarded, valueSoFar, and the interpolated
	// tag/attribute NAME, from a flat pass over the source's tags.
	lex := lexicalContexts(substituted, len(spans))
	// Whether each occurrence sits in a CDATA section is a source fact the
	// parser erases (it strips the markers from a foreign text node), and it
	// only changes the message for an SVG <script>/<style> body — where a
	// CDATA section, unlike ordinary foreign content, does not decode
	// entities.
	for i := range spans {
		if sentinelAt[i] >= 0 {
			lex[i].cdata = inForeignCDATA(substituted, sentinelAt[i])
		}
	}

	// The tree layer: html.Parse in both scripting modes. An interpolated
	// encoding= or type= is parsed at its worst case first — as the value it
	// could complete — so a namespace or stylesheet it MIGHT decide is read
	// fail-closed. The occurrences that worst case removes (the encoding/type
	// values themselves, which are inert placements) keep their own contexts
	// from the untouched parse.
	worst, removed := substituteWorstCase(substituted, len(spans))
	on := classifyByParse(worst, true, len(spans))
	off := classifyByParse(worst, false, len(spans))
	if len(removed) > 0 {
		onPlain := classifyByParse(substituted, true, len(spans))
		offPlain := classifyByParse(substituted, false, len(spans))
		for idx := range removed {
			on[idx] = onPlain[idx]
			off[idx] = offPlain[idx]
		}
	}

	for i, sp := range spans {
		if sentinelAt[i] < 0 {
			continue
		}
		primary := mergeContext(lex[i], on[i])
		// A sentinel in a raw-text body that could complete the body's own
		// end tag is treated as an interpolated NAME, fail-closed.
		if primary.inertText || primary.rawTextElement != "" {
			if couldCompleteEndTag(substituted, sentinelAt[i], on[i].element) ||
				couldCompleteEndTag(substituted, sentinelAt[i], off[i].element) {
				primary = attrContext{inName: true}
			}
		}
		out[sp.start] = primary
		// The two readings are two real browser configurations, and a
		// placement dangerous in either must be reported. Compare the TREE
		// facts (the only thing scripting changes); where they differ the
		// scripting-off reading is kept alongside, qualified.
		if on[i] != off[i] {
			// alt exists exactly when the two scripting modes place the
			// occurrence differently, which means it is reachable in only one
			// of them — so the scripting-mode caveat holds even when the
			// scripting-off parse moved the occurrence's element out of the
			// <noscript> it came from (the reworded qualifier does not claim
			// the element still contains it).
			sc := mergeContext(lex[i], off[i])
			sc.noscript = true
			if sc != primary {
				alt[sp.start] = sc
			}
		}
	}

	// The two fallbacks the parse cannot answer, applied only to occurrences it
	// left unresolved: an interpolated element name (a "<" the parser reads as
	// text because the sentinel is not a name-start), and a tag that is never
	// closed.
	var ask []int
	var askSpan []int
	for i, sp := range spans {
		ctx, ok := out[sp.start]
		if !ok {
			continue
		}
		// A settled placement is not re-answered. An inertText from a
		// <noscript> body is settled only if the two scripting modes AGREE it
		// is text: where they differ, the body is markup with scripting off,
		// and an unterminated tag or interpolated name in it must fail closed.
		settledInert := ctx.inertText && on[i] == off[i]
		if ctx.inValue || ctx.inName || settledInert || ctx.discarded ||
			ctx.rawTextElement != "" || ctx.unterminated {
			continue
		}
		at := sentinelAt[i]
		if at < 0 {
			continue
		}
		if at > 0 {
			j := at - 1
			if j > 0 && substituted[j] == '/' {
				j--
			}
			if substituted[j] == '<' {
				out[sp.start] = attrContext{inName: true}
				continue
			}
		}
		ask = append(ask, at)
		askSpan = append(askSpan, i)
	}
	for k, unterminated := range insideUnterminatedTags(substituted, ask) {
		if unterminated {
			out[spans[askSpan[k]].start] = attrContext{unterminated: true}
		}
	}
	return out, alt
}

// lexFact is the syntactic half of an occurrence's context, from the source's
// tag structure alone.
type lexFact struct {
	inName     bool
	inValue    bool
	discarded  bool
	quoted     bool
	attr       string
	valueSoFar string
	cdata      bool // the occurrence sits in a CDATA section (entities not decoded)
}

// treeFact is the tree half, from html.Parse.
type treeFact struct {
	kind     treeKind
	element  string
	ns       string
	attr     string
	prefix   string // decoded attribute value before the occurrence
	noscript bool
}

type treeKind int

const (
	kAbsent treeKind = iota // the occurrence is in no DOM node (dropped, or in text)
	kAttrValue
	kName // element or attribute name
	kScript
	kStyle
	kText
)

// mergeContext combines the syntactic and tree halves into the attrContext the
// rules read. The syntactic NAME and unterminated facts win, being fail-closed;
// otherwise the parser's placement decides.
func mergeContext(l lexFact, t treeFact) attrContext {
	if t.kind == kName {
		// The parser read the occurrence as an element or attribute NAME. The
		// lexical layer is not consulted for this: a broken quote in a script
		// body can make its flat scan read a real attribute value as a name,
		// and the parser is the authority on which it is.
		return attrContext{inName: true, noscript: t.noscript}
	}
	switch t.kind {
	case kAttrValue:
		return attrContext{
			inValue:     true,
			attr:        t.attr,
			quoted:      l.quoted,
			valueSoFar:  t.prefix,
			discarded:   l.discarded,
			element:     t.element,
			foreignRoot: t.ns,
			noscript:    t.noscript,
		}
	case kScript:
		fr := t.ns
		if l.cdata {
			fr = ""
		}
		return attrContext{rawTextElement: "script", foreignRoot: fr, noscript: t.noscript}
	case kStyle:
		fr := t.ns
		if l.cdata {
			fr = ""
		}
		return attrContext{rawTextElement: "style", foreignRoot: fr, noscript: t.noscript}
	case kText:
		// The parser placed it in a text node the rules have nothing to say
		// about; recorded as settled so the fallbacks do not re-answer it.
		return attrContext{inertText: true, noscript: t.noscript}
	default:
		// kAbsent: the occurrence is in no DOM node — dropped by an ignored or
		// duplicate element, or in a name the parser did not keep. Here, and
		// only here, the lexical NAME reading is trusted: an interpolated
		// END-tag name ("</s[x]>") leaves no node, and the lexical scan is the
		// only thing that sees it. A discarded duplicate attribute is the one
		// such position with something left to say (raw= bytes still pass the
		// tokenizer); everything else is left UNRESOLVED for the "<"-name and
		// unterminated-tag fallbacks.
		if l.inName {
			return attrContext{inName: true}
		}
		if l.discarded {
			return attrContext{discarded: true, attr: l.attr, quoted: l.quoted}
		}
		return attrContext{}
	}
}

// inForeignCDATA reports whether the byte at offset `at` sits inside a CDATA
// section — the most recent of "<![CDATA[", a closing "]]>", and a tag
// boundary ">" before it is the opener. A CDATA section cannot cross a tag, so
// a ">" more recent than the opener means the section belongs to an earlier
// element; this keeps a CDATA in one element's body from being read as
// covering the next element's.
func inForeignCDATA(src string, at int) bool {
	if at > len(src) {
		at = len(src)
	}
	open := strings.LastIndex(src[:at], "<![CDATA[")
	if open < 0 {
		return false
	}
	closeAt := strings.LastIndex(src[:at], "]]>")
	tagEnd := strings.LastIndexByte(src[:at], '>')
	return open > closeAt && open > tagEnd
}

// couldCompleteEndTag reports whether the text right before offset `at` spells
// "</" plus a case-insensitive prefix of `name`, so the interpolated value
// there could complete the end tag that ends this raw-text body — "</text[x]>"
// is "</textarea>" for a value of "area", and the body and everything after it
// is then markup. The interpolated-NAME rule is the fail-closed answer, and it
// is a lexical fact the parse cannot supply because the parser reads the
// sentinel as part of a name that does not match.
func couldCompleteEndTag(text string, at int, name string) bool {
	if name == "" || name == "plaintext" {
		// <plaintext> has no end tag at all — its tokenizer state runs to EOF
		// — so nothing in its body can complete one.
		return false
	}
	i := at
	for i > 0 && asciiLowerByte(text[i-1]) >= 'a' && asciiLowerByte(text[i-1]) <= 'z' {
		i--
	}
	if i < 2 || text[i-1] != '/' || text[i-2] != '<' {
		return false
	}
	n := at - i
	return n <= len(name) && asciiEqualFold(text[i:at], name[:n])
}

// lexicalContexts walks the source's tags — quote-, comment- and
// CDATA-aware, but with no tree and no raw-text elements — and returns the
// syntactic facts for every sentinel that lands in one. A sentinel in text is
// left as the zero lexFact, to be answered by the parser.
// scanComment returns the index just past a comment beginning at src[i] ("<!--"),
// which the HTML tokenizer closes at "-->" OR "--!>". Returns len(src) for an
// unterminated comment.
func scanComment(src string, i int) int {
	j := i + 4
	for j < len(src) {
		if strings.HasPrefix(src[j:], "-->") {
			return j + 3
		}
		if strings.HasPrefix(src[j:], "--!>") {
			return j + 4
		}
		j++
	}
	return len(src)
}

// scanTagEnd returns the index of a start/end tag's closing ">" (or len(src)-1
// if it runs off the end), reading it the way the HTML tokenizer does: a quote
// delimits a value only when it opens one, i.e. in the "before attribute
// value" state right after "="; a quote anywhere in a name or an unquoted
// value is a literal character, not a delimiter.
func scanTagEnd(src string, i int) int {
	j := i + 1
	afterEquals := false
	for j < len(src) {
		c := src[j]
		if c == '>' {
			return j
		}
		if c == '=' {
			afterEquals = true
			j++
			continue
		}
		if afterEquals && !isASCIISpace(c) {
			if c == '"' || c == '\'' {
				// A quoted value: skip to the matching quote.
				k := j + 1
				for k < len(src) && src[k] != c {
					k++
				}
				j = k + 1
			} else {
				// An unquoted value: skip to whitespace or ">".
				for j < len(src) && !isASCIISpace(src[j]) && src[j] != '>' {
					j++
				}
			}
			afterEquals = false
			continue
		}
		j++
	}
	return len(src) - 1
}

// lexicalContexts walks the source's tags — comment-aware and with correct
// tag delimiting, but with no tree and no raw-text elements — and returns the
// syntactic facts for every sentinel that lands in one: quoted and discarded
// for a START-tag attribute value, and inName for a sentinel in an END-tag
// NAME. Interpolated START-tag element and attribute names are NOT reported
// here: the parser resolves those to kName, and an END-tag ATTRIBUTE (which
// the parser ignores) must not be mistaken for a name. A sentinel in text is
// left as the zero lexFact, to be answered by the parser.
func lexicalContexts(src string, nspans int) []lexFact {
	facts := make([]lexFact, nspans)
	i := 0
	for i < len(src) {
		if strings.HasPrefix(src[i:], "<!--") {
			i = scanComment(src, i)
			continue
		}
		if strings.HasPrefix(src[i:], "<![CDATA[") {
			if e := strings.Index(src[i+9:], "]]>"); e >= 0 {
				i = i + 9 + e + 3
			} else {
				i = len(src)
			}
			continue
		}
		endTag := i+1 < len(src) && src[i+1] == '/'
		if src[i] == '<' && i+1 < len(src) && (isTagNameStart(src[i+1]) || endTag) {
			end := scanTagEnd(src, i)
			tag := src[i : end+1]
			if endTag {
				// Only the NAME of an end tag matters — a sentinel completing
				// it could name a real element to close. Its attributes are
				// ignored by the parser, so a sentinel there is inert.
				k := i + 2
				for k <= end && !isASCIISpace(src[k]) && src[k] != '>' {
					k++
				}
				for _, p := range sentinelIndexes(src[i+2 : k]) {
					if p.index >= 0 && p.index < nspans {
						facts[p.index] = lexFact{inName: true}
					}
				}
			} else {
				for _, h := range sentinelsInTag(tag) {
					if h.index < 0 || h.index >= nspans {
						continue
					}
					// Only the attribute-value facts are taken; a start-tag
					// element or attribute NAME is the parser's kName.
					if h.ctx.inValue {
						facts[h.index] = lexFact{
							inValue:    true,
							attr:       h.ctx.attr,
							quoted:     h.ctx.quoted,
							valueSoFar: h.ctx.valueSoFar,
							discarded:  h.ctx.discarded,
						}
					}
				}
			}
			if end >= len(src)-1 {
				i = len(src)
			} else {
				i = end + 1
			}
			continue
		}
		i++
	}
	return facts
}

// substituteWorstCase replaces the value of an interpolated encoding= or type=
// attribute that COULD complete a namespace- or stylesheet-deciding value with
// that value, so a downstream placement is read fail-closed. It returns the
// rewritten source and the set of sentinel indices it removed (which the
// caller classifies from the untouched parse).
func substituteWorstCase(src string, nspans int) (string, map[int]bool) {
	removed := map[int]bool{}
	var b strings.Builder
	i := 0
	for i < len(src) {
		if strings.HasPrefix(src[i:], "<!--") {
			e := scanComment(src, i)
			b.WriteString(src[i:e])
			i = e
			continue
		}
		if strings.HasPrefix(src[i:], "<![CDATA[") {
			e := strings.Index(src[i+9:], "]]>")
			end := len(src)
			if e >= 0 {
				end = i + 9 + e + 3
			}
			b.WriteString(src[i:end])
			i = end
			continue
		}
		if src[i] == '<' && i+1 < len(src) && (isTagNameStart(src[i+1]) || src[i+1] == '/') {
			end := scanTagEnd(src, i)
			b.WriteString(rewriteWorstCaseTag(src[i:end+1], removed, nspans))
			if end >= len(src)-1 {
				i = len(src)
			} else {
				i = end + 1
			}
			continue
		}
		b.WriteByte(src[i])
		i++
	}
	return b.String(), removed
}

// rewriteWorstCaseTag rewrites one tag's encoding=/type= values to their
// dangerous completion when an interpolation could reach it.
func rewriteWorstCaseTag(tag string, removed map[int]bool, nspans int) string {
	replace := func(name, value string) (string, bool) {
		var target string
		switch name {
		case "encoding":
			if interpolatedValueCouldBe(value, htmlAnnotationEncodings) {
				target = htmlAnnotationEncodings[0]
			}
		case "type":
			if interpolatedValueCouldBe(value, styleSheetTypes) && strings.Contains(value, lintSentinelPrefix) {
				target = "text/css"
			}
		}
		if target == "" {
			return "", false
		}
		for _, p := range sentinelIndexes(value) {
			if p.index >= 0 && p.index < nspans {
				removed[p.index] = true
			}
		}
		return target, true
	}
	// Walk the tag's attributes, rewriting values in place.
	var out strings.Builder
	i := 0
	if i < len(tag) && tag[i] == '<' {
		i++
	}
	for i < len(tag) && !isASCIISpace(tag[i]) && tag[i] != '>' && tag[i] != '/' {
		i++
	}
	out.WriteString(tag[:i])
	for i < len(tag) {
		nameStart := i
		for i < len(tag) && (isASCIISpace(tag[i]) || tag[i] == '/') {
			i++
		}
		if i >= len(tag) || tag[i] == '>' {
			out.WriteString(tag[nameStart:])
			return out.String()
		}
		attrNameStart := i
		for i < len(tag) && tag[i] != '=' && tag[i] != '/' && !isASCIISpace(tag[i]) && tag[i] != '>' {
			i++
		}
		name := asciiLower(tag[attrNameStart:i])
		for i < len(tag) && isASCIISpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] != '=' {
			out.WriteString(tag[nameStart:i])
			continue
		}
		i++
		for i < len(tag) && isASCIISpace(tag[i]) {
			i++
		}
		var q byte
		if i < len(tag) && (tag[i] == '"' || tag[i] == '\'') {
			q = tag[i]
			i++
		}
		valStart := i
		for i < len(tag) {
			if q != 0 && tag[i] == q {
				break
			}
			if q == 0 && (isASCIISpace(tag[i]) || tag[i] == '>') {
				break
			}
			i++
		}
		value := tag[valStart:i]
		out.WriteString(tag[nameStart:valStart])
		if target, ok := replace(name, value); ok {
			out.WriteString(target)
		} else {
			out.WriteString(value)
		}
		if i < len(tag) {
			out.WriteString(tag[i : i+1])
			i++
		}
	}
	return out.String()
}

// classifyByParse parses src with the given scripting mode and returns each
// sentinel's tree fact.
func classifyByParse(src string, scripting bool, nspans int) []treeFact {
	facts := make([]treeFact, nspans)
	opts := []html.ParseOption{}
	if !scripting {
		opts = append(opts, html.ParseOptionEnableScripting(false))
	}
	doc, err := html.ParseWithOptions(strings.NewReader(src), opts...)
	if err != nil {
		return facts
	}
	set := func(idx int, f treeFact) {
		if idx >= 0 && idx < nspans {
			facts[idx] = f
		}
	}
	var walk func(n *html.Node, noscript bool)
	walk = func(n *html.Node, noscript bool) {
		here := noscript
		if n.Type == html.ElementNode && n.Namespace == "" && n.Data == "noscript" {
			here = true
		}
		switch n.Type {
		case html.ElementNode:
			for _, p := range sentinelIndexes(n.Data) {
				set(p.index, treeFact{kind: kName, noscript: here})
			}
			for _, a := range n.Attr {
				for _, p := range sentinelIndexes(a.Key) {
					set(p.index, treeFact{kind: kName, noscript: here})
				}
				for _, p := range sentinelIndexes(a.Val) {
					set(p.index, treeFact{
						kind: kAttrValue, element: n.Data, ns: n.Namespace,
						attr: a.Key, prefix: a.Val[:p.at], noscript: here,
					})
				}
			}
		case html.CommentNode, html.DoctypeNode:
			// A sentinel the parser put in a comment or doctype is inert
			// text; recorded so the "<"-name fallback does not read comment
			// content as an interpolated tag.
			for _, p := range sentinelIndexes(n.Data) {
				set(p.index, treeFact{kind: kText, noscript: here})
			}
		case html.TextNode:
			// A text node's own Namespace is always "", so the parent — the
			// element whose DIRECT child text this is — carries what decides
			// the kind: an HTML or SVG <script> whose body is program source,
			// an applied <style> whose body is CSS, or anything else.
			kind := kText
			ns := ""
			if p := n.Parent; p != nil && p.Type == html.ElementNode {
				ns = p.Namespace
				// Only HTML and SVG have a <script> that runs and a <style>
				// that applies; a MathML <script> or <style> is inert markup.
				if ns == "" || ns == "svg" {
					if p.Data == "script" && programScript(p) {
						kind = kScript
					} else if p.Data == "style" && styleIsStylesheet(p) {
						kind = kStyle
					}
				}
			}
			for _, p := range sentinelIndexes(n.Data) {
				set(p.index, treeFact{kind: kind, element: parentData(n), ns: ns, noscript: here})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, here)
		}
	}
	walk(doc, false)
	return facts
}

func parentData(n *html.Node) string {
	if n.Parent != nil {
		return n.Parent.Data
	}
	return ""
}

// programScript reports whether a <script> element's body is program source.
// The element's name decides it, never its type= — the documented residue that
// a data <script type="application/json"> still draws the JavaScript message.
func programScript(n *html.Node) bool { return true }

// styleIsStylesheet reports whether a browser applies this <style> element, so
// its body is CSS rather than inert text: the type must be absent, empty, or
// text/css.
func styleIsStylesheet(n *html.Node) bool {
	for _, a := range n.Attr {
		if a.Key != "type" {
			continue
		}
		for _, want := range styleSheetTypes {
			if asciiEqualFold(a.Val, want) {
				return true
			}
		}
		return false
	}
	return true
}

// insideUnterminatedTags reports, for each offset in at (ascending), whether it
// sits after a "<" that is never closed — the one case the tokenizer cannot
// describe. One left-to-right pass answers for all of them, because asking per
// offset means walking back to the nearest "<" and then forward to EOF, which
// is the whole document once per occurrence.
//
// The pass can carry every pending answer at once because there are only ever
// three of them to carry. Each byte acts on the quote state as a permutation
// (a '"' swaps "outside" with "inside a double quote" and fixes the third; a
// "'" does the mirror; every other byte is the identity), so two scans in
// different states never converge into one, and three states is the ceiling on
// how many distinct scans can be in flight. pending[q] holds the offsets whose
// scan currently sits in state q, and a byte that permutes the states just
// swaps those buckets.
func insideUnterminatedTags(substituted string, at []int) []bool {
	out := make([]bool, len(at))
	if len(at) == 0 {
		return out
	}
	const (
		outside = iota // not inside an attribute value
		inDouble
		inSingle
	)
	var pending [3][]int
	// active is the quote state of the scan belonging to the nearest "<" that
	// could open a tag, or -1 when the nearest one could not open one, was
	// already closed by a ">", or does not exist yet.
	active := -1
	remaining, next := 0, 0
	// The bound is inclusive: an offset at the very end of the document is a
	// legitimate question ("<a", 2), and answering it needs one pass with no
	// byte left to read.
	for i := 0; i <= len(substituted); i++ {
		for next < len(at) && at[next] == i {
			// Everything the answer depends on is at or after this byte, so an
			// offset is registered before its own byte is processed — which is
			// what "the nearest < strictly before it" means.
			if active >= 0 {
				pending[active] = append(pending[active], next)
				remaining++
			}
			next++
		}
		if i == len(substituted) {
			break
		}
		switch substituted[i] {
		case '"':
			pending[outside], pending[inDouble] = pending[inDouble], pending[outside]
			switch active {
			case outside:
				active = inDouble
			case inDouble:
				active = outside
			}
		case '\'':
			pending[outside], pending[inSingle] = pending[inSingle], pending[outside]
			switch active {
			case outside:
				active = inSingle
			case inSingle:
				active = outside
			}
		case '>':
			// Only a scan that is outside quotes is closed by this ">":
			// <div title="before > [meta …] contains one that closes nothing.
			remaining -= len(pending[outside])
			pending[outside] = pending[outside][:0]
			if active == outside {
				active = -1
			}
		case '<':
			// "<" only opens a tag when a name or a markup declaration follows
			// it; "Score < [meta ...]" is text and closes nothing. Scans already
			// in flight are unaffected — each keeps the quote state it has had
			// since its own "<".
			if i+1 < len(substituted) && isTagNameStart(substituted[i+1]) {
				active = outside
			} else {
				active = -1
			}
		}
		if next == len(at) && remaining == 0 {
			break
		}
	}
	// Whatever is still pending met no ">" outside quotes before the end.
	for q := range pending {
		for _, k := range pending[q] {
			out[k] = true
		}
	}
	return out
}

type sentinelHit struct {
	index int
	ctx   attrContext
	// value is the whole attribute value the sentinel sits in, as written. It
	// is a slice of the tag's own source, so keeping it copies nothing, and one
	// rule needs the text on BOTH sides of the interpolation rather than only
	// the prefix ctx.valueSoFar carries.
	value string
}

// sentinelsInTag scans one tag's own source for sentinels sitting in an
// attribute value or a tag/attribute name. The tag is already delimited by
// lexicalContexts, so this is a flat walk over "name=value" pairs — no
// comments, no raw-text bodies, no nesting.
func sentinelsInTag(tag string) []sentinelHit {
	var hits []sentinelHit
	i := 0
	// Skip "<" and the element name — but not before checking it, since an
	// interpolated element name is undelimited in the same way a bare
	// attribute name is.
	if i < len(tag) && tag[i] == '<' {
		i++
	}
	elemStart := i
	for i < len(tag) && !isASCIISpace(tag[i]) && tag[i] != '>' && tag[i] != '/' {
		i++
	}
	for _, idx := range sentinelIndexes(tag[elemStart:i]) {
		hits = append(hits, sentinelHit{index: idx.index, ctx: attrContext{inName: true}})
	}
	seen := map[string]bool{}
	for i < len(tag) {
		for i < len(tag) && (isASCIISpace(tag[i]) || tag[i] == '/') {
			i++
		}
		if i >= len(tag) || tag[i] == '>' {
			break
		}
		nameStart := i
		// "/" separates names as well: <div x/onclick="..."> is two attributes
		// to the tokenizer, and reading it as one hid the handler.
		for i < len(tag) && tag[i] != '=' && tag[i] != '/' && !isASCIISpace(tag[i]) && tag[i] != '>' {
			i++
		}
		// asciiLower, not strings.ToLower, which folds U+212A onto "k": an
		// attribute spelled with one is not "onkeydown" to a browser, and
		// reading it as one reported an event handler that does not exist.
		name := asciiLower(tag[nameStart:i])
		// An interpolation in the NAME is worse than one in a value: nothing
		// delimits it, so a Meta value containing a space simply adds
		// attributes. <div data-[meta ...]="x"> is the realistic shape.
		for _, idx := range sentinelIndexes(name) {
			hits = append(hits, sentinelHit{index: idx.index, ctx: attrContext{inName: true}})
		}
		// A repeated attribute is dropped by the parser, so an occurrence in one
		// never reaches the page.
		duplicate := seen[name]
		seen[name] = true
		for i < len(tag) && isASCIISpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] != '=' {
			continue // valueless attribute
		}
		i++
		for i < len(tag) && isASCIISpace(tag[i]) {
			i++
		}
		var q byte
		if i < len(tag) && (tag[i] == '"' || tag[i] == '\'') {
			q = tag[i]
			i++
		}
		valStart := i
		for i < len(tag) {
			if q != 0 && tag[i] == q {
				break
			}
			if q == 0 && (isASCIISpace(tag[i]) || tag[i] == '>') {
				break
			}
			i++
		}
		value := tag[valStart:i]
		for _, idx := range sentinelIndexes(value) {
			ctx := attrContext{attr: name, quoted: q != 0, inValue: true, discarded: duplicate}
			// Only the URL rules read the prefix, and unescaping a growing
			// prefix once per occurrence is quadratic in a value holding many.
			if urlBearingAttrs[name] {
				// html.UnescapeString so a prefix written as "java&#x73;cript"
				// is judged as the "javascript" the browser will see.
				ctx.valueSoFar = html.UnescapeString(value[:idx.at])
			}
			hits = append(hits, sentinelHit{index: idx.index, ctx: ctx, value: value})
		}
		if i < len(tag) {
			i++
		}
	}
	return hits
}

type sentinelPos struct{ index, at int }

func sentinelIndexes(value string) []sentinelPos {
	var out []sentinelPos
	for from := 0; ; {
		i := strings.Index(value[from:], lintSentinelPrefix)
		if i < 0 {
			return out
		}
		i += from
		rest := value[i+len(lintSentinelPrefix):]
		// The index is the digit run between the prefix and the "z" suffix.
		j := 0
		for j < len(rest) && rest[j] >= '0' && rest[j] <= '9' {
			j++
		}
		if j > 0 && j < len(rest) && rest[j] == lintSentinelSuffix[0] {
			if n, err := strconv.Atoi(rest[:j]); err == nil {
				out = append(out, sentinelPos{index: n, at: i})
			}
		}
		from = i + len(lintSentinelPrefix) + j
	}
}

// hasNonHTMLSpace reports whether s holds any character that is not HTML
// whitespace (space, tab, LF, FF, CR). It is a byte scan on purpose: a NBSP
// (U+00A0) is body content to the parser, and its UTF-8 bytes are not any of
// these, so it reads as content — where strings.TrimSpace, which trims every
// Unicode space, would wrongly treat it as blank.
func hasNonHTMLSpace(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\f', '\r':
		default:
			return true
		}
	}
	return false
}

func isASCIISpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f'
}

// unsafeAttributeContexts reports the ways a bare shortcode value is still
// dangerous where it is being placed.
//
// html.EscapeString covers & < > ' " and nothing else — exactly enough to keep a
// value inside a quoted attribute, and nothing at all once the browser re-parses
// that value. The distinction has a privilege boundary behind it: an admin or
// editor writes the template, but the Meta value it interpolates is written by
// whoever can edit the entity, which includes the plain user role.
//
// raw disables even that much, so with it every attribute position is unsafe.
//
// Two properties of the markup around the value narrow this set rather than
// adding to it, and neither is anything the tokenizer alone can see.
//
// ctx.foreignRoot: inside <svg> or <math> an element is not an HTML element, so
// the rules that describe what one particular HTML element DOES are withheld.
// srcdoc= is the one that matters: a foreign <iframe> is an inert element with
// no browsing context, so its srcdoc is never parsed as a document. The rules
// about markup syntax (raw=, an unquoted value, an interpolated name) and the
// two attributes every namespace honours (style=, on*) are untouched. So are
// the URL rules, deliberately — an SVG <a href="javascript:…"> runs.
//
// Those stay element-blind, and it is a real trade rather than an oversight. An
// href on an SVG <g>, like one on an HTML <div>, navigates nowhere, so warning
// about it is a false positive. But the alternative is an allow-list of which
// element makes which attribute a URL, and a list one entry short is a missed
// warning on a real link — <a>, <area>, <base>, <link>, <form action>, SVG <a>,
// <image>, <use>, <animate>… The false positives the list would buy are markup
// nobody writes; the misses it would risk are markup people do. srcdoc is the
// one attribute checked against its element, and only because it has exactly
// one host and no ambiguity about it.
//
// ctx.noscript: a <noscript> body is markup only when scripting is disabled, so
// the rules that describe execution are withheld there entirely and the ones
// that describe a PLACEMENT carry noscriptQualifier. The raw= rules are neither
// and carry nothing — qualify, below, is where that is decided and why.
func unsafeAttributeContexts(ctx attrContext, raw, cssMode bool, label string) []string {
	// qualify names the mode a reason applies in. Only the reasons that
	// describe a placement — an attribute, a quote, a name — take it, because
	// only those depend on the body being markup at all. The raw= reasons do
	// not: an unescaped value can write "</noscript>" and continue in markup
	// whichever way the body was read, which is the argument the raw= rule
	// already makes about "</xmp>", so qualifying one would understate it.
	qualify := func(s string) string {
		if ctx.noscript {
			return s + noscriptQualifier
		}
		return s
	}
	// A <noscript> body reaches nothing that needs a script to run: with
	// scripting enabled the body is inert raw text, and with it disabled the
	// handler and the "javascript:" link are real and do nothing. Reporting
	// them there is the false positive that had an earlier attempt at reading
	// these bodies withdrawn.
	scriptCanRun := !ctx.noscript

	if cssMode && !ctx.inValue && !ctx.inName {
		// A CustomCSS slot is a stylesheet with no <style> wrapper of its own,
		// so nothing in the markup says so — the editor has to.
		if raw {
			return []string{label + ` with raw= in a CSS slot is not escaped and lands in the stylesheet verbatim. Drop raw= here.`}
		}
		return []string{label + ` is in a CSS slot, where the value lands in a stylesheet with nothing to re-parse it as HTML: a ";" in it can start another declaration and a "}" can escape the rule, and escaping touches neither.`}
	}
	if ctx.unterminated {
		return []string{label + ` is inside a tag that is never closed, so where it lands cannot be determined. Close the tag; until then treat the value as unsafe.`}
	}
	if ctx.discarded {
		// A repeated attribute: the parser keeps the first and drops this
		// one, so no rule about the attribute's meaning applies. raw= still
		// does — the unescaped bytes pass through the tokenizer whatever the
		// DOM keeps, and a value containing a quote closes the attribute and
		// reshapes the tag.
		if raw {
			return []string{label + ` with raw= is not escaped at all, so placing it in the "` + ctx.attr + `" attribute lets the value close the attribute and add its own. Drop raw= here.`}
		}
		return nil
	}
	if ctx.rawTextElement != "" {
		// Only script and style ever set this, so the language is never empty.
		switch {
		case ctx.noscript && ctx.rawTextElement == "script":
			// A <script> in a <noscript> body reaches nothing under either
			// reading: with scripting enabled the body is inert raw text, and
			// with it disabled the element is real and does not run. What
			// survives is raw=, which is unescaped either way and can write
			// "</noscript>" and go on in markup.
			if raw {
				return []string{label + ` with raw= is not escaped, so a value containing markup becomes real elements on the page. Anyone who can edit the entity can then inject script. Drop raw= unless the value is authored by someone you would trust with the template itself.`}
			}
			return nil
		case ctx.foreignRoot != "":
			return []string{label + ` sits inside a <` + ctx.rawTextElement + `> that is inside <` + ctx.foreignRoot +
				`>, which is foreign content, where the parser decodes entities — unlike an HTML <` + ctx.rawTextElement +
				`> body. The escaping is therefore undone before ` + scriptLikeLanguage[ctx.rawTextElement] +
				` sees the value: an escaped quote arrives as a real quote and can ` + foreignEscapeConsequence[ctx.rawTextElement] +
				`. Escaping makes this placement worse than the HTML one rather than safer. Pass the value in through a data- attribute instead.`}
		}
		return []string{qualify(label + ` sits inside a <` + ctx.rawTextElement + `> body, which the browser does not decode entities in, so the value reaches ` + scriptLikeLanguage[ctx.rawTextElement] + ` with its escaping still in it rather than as the text you wrote. Escaping does not make the placement safe either — a "${...}" in a template literal or a ";" in a declaration contains nothing it touches. Pass the value in through a data- attribute instead.`)}
	}
	if ctx.inName {
		return []string{qualify(label + ` is interpolated into a tag or attribute NAME, which nothing delimits: a value containing a space or "=" simply adds attributes of its own, and escaping does not touch either character. Build the name in the template instead.`)}
	}
	if !ctx.inValue || ctx.attr == "" {
		if raw {
			// raw= disables escaping outright, so it is unsafe wherever it
			// lands, including ordinary text: a Meta value of
			// "<img src=x onerror=...>" becomes a real element.
			return []string{label + ` with raw= is not escaped, so a value containing markup becomes real elements on the page. Anyone who can edit the entity can then inject script. Drop raw= unless the value is authored by someone you would trust with the template itself.`}
		}
		return nil
	}
	attr := ctx.attr

	var out []string
	if raw {
		out = append(out, label+` with raw= is not escaped at all, so placing it in the "`+attr+`" attribute lets the value close the attribute and add its own. Drop raw= here.`)
	}
	if !ctx.quoted {
		out = append(out, qualify(label+` sits in an unquoted attribute value, where escaping does not stop a value containing a space from adding attributes of its own. Quote the attribute.`))
	}
	if cssMode {
		out = append(out, qualify(label+` is in a CSS slot, where the value lands in a stylesheet with nothing to re-parse it as HTML: a ";" in it can start another declaration and a "}" can escape the rule, and escaping touches neither.`))
	}
	if kind := expressionAttributeKind(attr); kind != "" && scriptCanRun {
		out = append(out, label+` sits in the "`+attr+`" `+kind+`, whose value is evaluated as script after the HTML parser has undone the escaping, so a value containing a quote can execute. Do not interpolate Meta into it.`)
	}
	if attr == "style" {
		out = append(out, qualify(label+` sits in a "style" attribute, where escaping does not prevent CSS injection.`))
	}
	// srcdoc is the property of one HTML element rather than of the markup, so
	// it says nothing about the same attribute written on a foreign <iframe>,
	// which has no browsing context, or on an HTML element that is not an
	// iframe, where it is an inert unknown attribute.
	if alwaysUnsafeAttrs[attr] && ctx.foreignRoot == "" && ctx.element == "iframe" {
		injection := "script"
		if ctx.noscript {
			// With scripting disabled the srcdoc document is still parsed —
			// that is what makes the placement live at all — but no script in
			// it runs, so what the attribute admits there is markup.
			injection = "markup"
		}
		out = append(out, qualify(label+` sits in a "`+attr+`" attribute, whose value the browser decodes and parses as HTML, so escaping does not prevent `+injection+` injection anywhere in it.`))
	}
	if urlBearingAttrs[attr] && scriptCanRun {
		scheme, fixed := urlSchemeBefore(ctx.valueSoFar)
		switch {
		case fixed && executableURLSchemes[scheme]:
			out = append(out, label+` continues a "`+scheme+`:" URL in "`+attr+`", which the browser executes rather than fetches.`)
		case !fixed && couldStillBecomeExecutable(scheme):
			out = append(out, label+` can still choose the scheme of the "`+attr+`" URL, and escaping does not stop a "javascript:" value. Put a path or a complete scheme in front of it, e.g. href="/x/[meta ...]".`)
		}
	}
	return out
}

// urlSchemeBefore reports what the text before the interpolation has settled
// about the URL: the scheme it named, and whether the kind of URL is decided at
// all. A "/", "?" or "#" reaching the front makes it relative and therefore
// safe; a ":" names a scheme, which may itself be an executable one.
//
// Emptiness is the wrong test on its own: href="java[meta ...]" has a non-empty
// prefix and a Meta value of "script:alert(1)" still completes javascript:.
func urlSchemeBefore(prefix string) (scheme string, fixed bool) {
	// Browsers strip tab, newline and carriage return from a URL before
	// resolving it, so "java&#9;script:" is javascript: to them. Comparing the
	// bytes as written let that through.
	prefix = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' || r == 0 {
			return -1
		}
		return r
	}, prefix)
	for i := 0; i < len(prefix); i++ {
		switch prefix[i] {
		case ':':
			return asciiLower(strings.TrimSpace(prefix[:i])), true
		case '/', '?', '#':
			return "", true
		}
	}
	return asciiLower(strings.TrimSpace(prefix)), false
}

// couldStillBecomeExecutable reports whether a value appended to this prefix
// could complete an executable scheme. href="java[meta ...]" can, because
// "javascript" starts with "java"; href="https[meta ...]" cannot, because no
// executable scheme does.
func couldStillBecomeExecutable(prefix string) bool {
	for scheme := range executableURLSchemes {
		if strings.HasPrefix(scheme, prefix) {
			return true
		}
	}
	return false
}

// asciiEqualFold and asciiLower implement the "ASCII case-insensitive match"
// every one of these attribute comparisons is specified with. strings.EqualFold
// is Unicode simple folding, which makes "text/cſſ" equal to "text/css" — a
// browser does not, so a stylesheet it never builds was drawing a CSS warning.
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if asciiLowerByte(a[i]) != asciiLowerByte(b[i]) {
			return false
		}
	}
	return true
}

func asciiLower(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			b := []byte(s)
			for j := i; j < len(b); j++ {
				b[j] = asciiLowerByte(b[j])
			}
			return string(b)
		}
	}
	return s
}

func asciiLowerByte(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// styleSheetTypes are the type= values that leave a <style> element a
// stylesheet: the empty string, or text/css. The match is ASCII
// case-insensitive and nothing more — no trimming, no parameters.
var styleSheetTypes = []string{"", "text/css"}

// htmlAnnotationEncodings are the two <annotation-xml> encodings that make its
// children HTML. The match is ASCII case-insensitive and nothing more — no
// trimming, no parameters.
var htmlAnnotationEncodings = []string{"text/html", "application/xhtml+xml"}

// interpolatedValueCouldBe reports whether some assignment to the
// interpolations in value could make the whole of it one of wants. Two
// attributes turn on this question — <annotation-xml>'s encoding, which decides
// a namespace, and <style>'s type, which decides whether there is a stylesheet
// at all — and both are written by whoever can edit the entity.
func interpolatedValueCouldBe(value string, wants []string) bool {
	runs := fixedRunsAround(value)
	if len(runs) < 2 {
		return false // no interpolation in it at all
	}
	for _, want := range wants {
		if fixedRunsFit(runs, want) {
			return true
		}
	}
	return false
}

// fixedRunsAround splits an attribute value into the text an interpolation
// cannot change: the run before the first sentinel, the runs between them, and
// the run after the last. Returns nil when the value holds no sentinel.
//
// Each run is unescaped, because the static path compares the value the browser
// sees — z.TagAttr decodes entities, so encoding="text&#x2f;[meta …]" has to be
// judged as the "text/" it becomes. Same bypass the URL rules close with
// html.UnescapeString.
func fixedRunsAround(value string) []string {
	hits := sentinelIndexes(value)
	if len(hits) == 0 {
		return nil
	}
	runs := make([]string, 0, len(hits)+1)
	at := 0
	for _, h := range hits {
		runs = append(runs, asciiLower(html.UnescapeString(value[at:h.at])))
		// Skip past the whole sentinel: prefix, digit run, and "z" suffix.
		at = h.at + len(lintSentinelPrefix)
		for at < len(value) && value[at] >= '0' && value[at] <= '9' {
			at++
		}
		if at < len(value) && value[at] == lintSentinelSuffix[0] {
			at++
		}
	}
	return append(runs, asciiLower(html.UnescapeString(value[at:])))
}

// fixedRunsFit reports whether some assignment to the interpolations could make
// the whole value equal want. The first run has to start it, the last has to end
// it, and the ones between have to appear in order in what is left — which is
// exact rather than conservative, so encoding="[meta …]zzz[meta …]" is refused:
// neither interpolation can delete the "zzz", and no encoding contains one.
func fixedRunsFit(runs []string, want string) bool {
	if !strings.HasPrefix(want, runs[0]) {
		return false
	}
	pos := len(runs[0])
	for _, r := range runs[1 : len(runs)-1] {
		i := strings.Index(want[pos:], r)
		if i < 0 {
			return false
		}
		pos += i + len(r)
	}
	last := runs[len(runs)-1]
	return len(want)-len(last) >= pos && strings.HasSuffix(want, last)
}

// executableURLSchemes run rather than fetch, so continuing one is unsafe even
// though the scheme is already fixed.
var executableURLSchemes = map[string]bool{
	"javascript": true, "data": true, "vbscript": true,
}

// alwaysUnsafeAttrs have values the browser decodes and then parses as markup,
// so escaping protects nothing wherever the interpolation lands in them.
var alwaysUnsafeAttrs = map[string]bool{"srcdoc": true}

// urlBearingAttrs are the attributes whose value the browser resolves as a URL.
var urlBearingAttrs = map[string]bool{
	"href": true, "src": true, "action": true, "formaction": true,
	"data": true, "poster": true, "xlink:href": true, "ping": true,
	"background": true, "cite": true, "longdesc": true, "manifest": true,
}

// expressionAttributeKind names the kind of attribute whose value a browser or
// Alpine evaluates as script, or "" when the value is inert text.
//
// Alpine is the reason this is not just "on*": this app wraps every entity-bound
// slot in an x-data scope and its own documentation recommends directives like
// :href for reading Meta client-side, so @click="f('...')" is a template an
// author here would plausibly write. Alpine evaluates the attribute's value as a
// JavaScript expression after the HTML parser has already decoded the escaping,
// which is the same sequence that makes onclick unsafe.
func expressionAttributeKind(attr string) string {
	// "on" alone is not an event handler; "onclick" is.
	if len(attr) > 2 && strings.HasPrefix(attr, "on") {
		return "event handler"
	}
	// x-on:click / x-bind:href / x-init / x-text / x-show / ... but not the
	// handful whose value Alpine reads as a literal rather than evaluating.
	if strings.HasPrefix(attr, "x-") && !isInertAlpineDirective(attr) {
		return "Alpine directive"
	}
	// @click and :href are the shorthands for x-on: and x-bind:. Only a leading
	// colon is Alpine — xlink:href has one in the middle and is a URL.
	if strings.HasPrefix(attr, "@") || strings.HasPrefix(attr, ":") {
		return "Alpine directive"
	}
	return ""
}

// scriptLikeElements are the two whose body is a program rather than markup,
// so a value placed in one is judged as landing in a language. Whether a given
// occurrence sits in such a body — rather than in a nested element's markup —
// is read from the DOM: classifyByParse marks a sentinel kScript or kStyle
// only when its DIRECT parent is the script or style element, which is the
// spec's "child text content" and free from the parser. The rules the two
// bodies then draw differ by namespace, and unsafeAttributeContexts is where
// that is decided:
//
//   - In the HTML namespace the parser decodes no entities in these bodies, so
//     an escaped value arrives with its escaping still in it. That is not
//     nothing — no "<" and no bare quote, so it can neither close the element
//     nor end a string — but a backtick, a "${...}" or a ";" is untouched, and
//     those are the characters that matter in a language.
//   - In SVG the sentence inverts: foreign content DOES decode entities, so the
//     escaping is undone before the language sees the value and an escaped
//     quote arrives as a real one. That placement is worse than the HTML one,
//     which foreignRoot carries into the message. A CDATA section decodes
//     nothing even in foreign content, so a CDATA-wrapped SVG body takes the
//     HTML message instead — the one lexical fact (l.cdata) the body needs.
//   - MathML has neither a script that runs nor a style that applies, so a
//     MathML <script>/<style> body is inert markup and classifyByParse leaves
//     it kText.
//
// A <style> is a program body only when a browser would apply it — type absent,
// empty, or text/css (styleIsStylesheet) — and an interpolated encoding= or
// type= that COULD complete a namespace- or stylesheet-deciding value is parsed
// at that worst case (substituteWorstCase), so a placement it might decide is
// read fail-closed.
//
// The residue that remains, all of it small and none of it in the tree model
// the parser now owns: whether a <script> is a program is decided by its name
// and never its type=, so a <script type="application/ld+json"> holding an
// escaped value still draws the JavaScript message — a wrong reason attached to
// a wrong verdict (nothing there reaches JavaScript) rather than a missed
// hazard, since the escaping there can neither close the element nor be decoded
// back into a quote. The URL rules stay element-blind (a <div href> warns), the
// on* match is a prefix, and a refresh <meta content="0;url=…"> is unwarned
// because "content" is not a URL attribute without its sibling http-equiv.
//
// One more, deliberately not closed: an interpolated ELEMENT name — "<sty[Tag]>"
// — is warned in place as an interpolated name, the strongest "do not do this"
// the linter has, but the content AFTER it is classified under the element the
// sentinel actually forms (an unknown element, whose body is inert markup), not
// under every element the name could complete. So "<sty[Tag]>.x{color:[C]}" is
// silent on [C], which would be live CSS only if [Tag] came out "le". Guessing
// the completion to re-warn [C] is refused because the name is unbounded — it
// could equally be an ordinary custom element, and warning [C] then is a false
// positive on safe markup, which this file treats as worse than a miss. The
// in-place name warning is the signal that the region is author-controlled.
var scriptLikeElements = map[string]bool{"script": true, "style": true}

var scriptLikeLanguage = map[string]string{"script": "JavaScript", "style": "CSS"}

// noscriptQualifier names the mode a placement reason applies in, for the
// reasons reported from inside a <noscript> body. It deliberately does not say
// the value is harmless with scripting enabled; qualify, in
// unsafeAttributeContexts, is where which reasons take it is decided and why.
const noscriptQualifier = ` This placement is reachable only with scripting disabled — the mode a <noscript>, whose body is markup only when scripting is disabled, exists for.`

// foreignEscapeConsequence names what a decoded quote does to each language,
// which is the part of the foreign-content message that is not shared.
var foreignEscapeConsequence = map[string]string{
	"script": "close a string literal and run whatever follows it",
	"style":  "close a string and end the declaration",
}

// inertAlpineDirectives take a literal value rather than an expression, so
// interpolating into one is no more dangerous than any other text attribute.
var inertAlpineDirectives = map[string]bool{
	// x-id is deliberately absent: its value is an array expression, which
	// Alpine evaluates.
	"x-ref": true, "x-cloak": true, "x-ignore": true, "x-transition": true,
	"x-teleport": true,
}

// isTagNameStart reports whether c can follow "<" to open a tag.
func isTagNameStart(c byte) bool {
	return c == '/' || c == '!' || c == '?' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// isInertAlpineDirective covers the modifier forms too: x-transition:enter and
// x-transition.opacity take class lists and durations, not expressions.
func isInertAlpineDirective(attr string) bool {
	base := attr
	if i := strings.IndexAny(base, ":."); i > 0 {
		base = base[:i]
	}
	return inertAlpineDirectives[base]
}

// emitsBareValue reports whether a shortcode writes a value straight into the
// surrounding markup, which is what makes where it sits matter. [meta] only does
// so with inline=, and [mrql] only with value=; [property] and [item] always do.
func emitsBareValue(name string, attrs map[string]string) bool {
	switch name {
	case "meta":
		return attrs["inline"] == "true"
	case "property", "item":
		return true
	case "mrql":
		return attrs["value"] != ""
	}
	return false
}
