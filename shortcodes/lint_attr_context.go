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
	// endTagName narrows inName to an END-tag name ("</s[x]>" or a raw-text
	// terminator "</text[x]>"). The danger there is NOT that the value adds
	// attributes — an end tag's attributes are ignored by the parser — but that
	// it chooses which element the tag closes, and so what the close reveals.
	// It needs its own message; the shared inName one states the wrong reason.
	endTagName bool
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
	// rawReachesLive marks a value the parser DROPPED (kAbsent) whose position
	// a raw= breakout could nonetheless reach live markup from — a re-opened
	// <body>/<html> whose duplicate attribute is dropped but whose tag merges
	// live, as opposed to a position past a <frameset> or x/net's foreign
	// <template> surrender, which the parser ignores entirely. Set from a
	// per-occurrence breakout probe, and read only for the raw= rules: an
	// unescaped value there can close the attribute and write markup, but only
	// where the parser builds anything at all.
	rawReachesLive bool
	// rawReachesOnlyOff narrows rawReachesLive to a breakout the parser builds
	// only with scripting DISABLED (a <noscript> reading that differs). The
	// raw= warning is then qualified and describes markup injection, not script
	// — nothing runs in the mode that builds it.
	rawReachesOnlyOff bool
	// unquotedSibling marks an UNQUOTED attribute value the parser dropped from
	// the tree — a repeated attribute (discarded), or a duplicate-named one on
	// a re-opened <body>/<html> that merges onto the singleton — yet whose
	// escaping still lets a space open a live sibling attribute. The dropped
	// name is gone, but the tag survives and keeps the injected attribute, so
	// the value is not harmless the way a QUOTED dropped one is. Set only when
	// the parse oracle confirms the injected attribute reaches a live element,
	// so a wholly ignored tag stays silent.
	unquotedSibling bool
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

// lintProbePrefix names the synthetic bareword attribute the unquoted-value
// oracle appends after a sentinel. It is per-process random like the sentinel
// so it cannot collide with real template bytes, and all lowercase so it
// survives the parser lowercasing an attribute name.
var lintProbePrefix = "mahprobe" + lintSentinelNonce + "x"

func lintProbe(i int) string { return lintProbePrefix + strconv.Itoa(i) }

// rawBreakoutReachesLive reports, per scripting mode, whether a raw= value at
// the occurrence whose sentinel occupies substituted[at:at+n] could reach live
// markup. A raw= value is unescaped, so its worst case closes whatever contains
// it and opens an element. The probe substitutes that worst case right after
// the sentinel and asks html.Parse whether a probe node survives:
//
//   - `'"` closes a single- OR double-quoted attribute value (the inactive one
//     is a harmless literal), then `>` closes the tag;
//   - the raw-text terminators close a <title>/<textarea>/<style>/<script>/
//     <xmp>/<noscript>/<noframes> body the value might sit in;
//   - `<...elem>` is a plain element for ordinary contexts, and `<frame ...elem>`
//     is the one element a <frameset> accepts, so a value dropped as frameset
//     TEXT is still caught.
//
// A probe node is the marker element OR any element carrying the marker
// attribute (the frame). A position the parser truly drops — past x/net's
// foreign-<template> surrender — builds none of them. Returned per mode so a
// breakout live only with scripting disabled (a <noscript> reading that differs)
// is warned as markup injection, qualified, not as script. One occurrence is
// probed at a time so a breakout cannot perturb another occurrence's answer.
func rawBreakoutReachesLive(substituted string, at, n int) (reachesOn, reachesOff bool) {
	if at < 0 || at+n > len(substituted) {
		return false, false
	}
	elem := lintProbePrefix + "elem"
	// The finite set of ways unescaped bytes at this position become a live
	// node. Two variants are needed because they are mutually exclusive in one
	// string: the element variant closes the tag; the attribute variant does
	// NOT, so it can add an attribute to the still-open start tag (a <plaintext>
	// or any start tag, where closing the tag would put the marker in a body
	// nothing escapes). Each variant leads with '" to close a single- OR
	// double-quoted value, and the element variant closes EVERY raw-text body a
	// value might sit in before opening its markers. The markers cover a plain
	// element, a <frameset>'s only child <frame>, and the <html>/<body> singleton
	// merges — all detected as the marker element OR the marker attribute.
	variants := []string{
		// element: close quotes, close the tag, close every raw-text body, then
		// open elements and merge onto the singletons.
		`'"></title></textarea></style></script></xmp></iframe></noembed></noframes></noscript><` +
			elem + `></` + elem + `><frame ` + elem + ` src=x><html ` + elem + `><body ` + elem + `>`,
		// attribute: close the quote and add a marker attribute WITHOUT closing
		// the tag, for a start tag whose body admits no markup (<plaintext>).
		`'" ` + elem + `=x `,
	}
	reaches := func(scripting bool) bool {
		opts := []html.ParseOption{}
		if !scripting {
			opts = append(opts, html.ParseOptionEnableScripting(false))
		}
		for _, breakout := range variants {
			src := substituted[:at+n] + breakout + substituted[at+n:]
			doc, err := html.ParseWithOptions(strings.NewReader(src), opts...)
			if err != nil {
				continue
			}
			found := false
			var walk func(*html.Node)
			walk = func(nd *html.Node) {
				if nd.Type == html.ElementNode {
					if nd.Data == elem {
						found = true
					}
					for _, a := range nd.Attr {
						if a.Key == elem {
							found = true
						}
					}
				}
				for c := nd.FirstChild; c != nil && !found; c = c.NextSibling {
					walk(c)
				}
			}
			walk(doc)
			if found {
				return true
			}
		}
		return false
	}
	return reaches(true), reaches(false)
}

// unquotedSiblingInjectors reports, per span, whether an UNQUOTED attribute
// value at that occurrence would add a live sibling attribute. Escaping does
// not touch the space that opens one, so a value the parser keeps in an
// unquoted position can write attributes of its own — even when the parser
// then DROPS the value's own attribute as a duplicate, because the tag itself
// survives and keeps the sibling. The oracle appends a unique bareword probe
// after each unquoted-value sentinel and asks html.Parse whether the probe
// survives on some element: a wholly ignored tag drops the probe with
// everything else, so silence there is the parser's own answer rather than a
// guess about which tags live. Quoted values (the space is contained) and NAME
// occurrences (already fail-closed) are not probed.
func unquotedSiblingInjectors(substituted string, sentinelAt []int, lex []lexFact, scripting bool) map[int]bool {
	// Splice the probes in from left to right; sentinelAt is increasing in i.
	var b strings.Builder
	prev := 0
	any := false
	for i := range lex {
		if sentinelAt[i] < 0 || !lex[i].inValue || lex[i].quoted {
			continue
		}
		end := sentinelAt[i] + len(lintSentinel(i))
		if end > len(substituted) || end < prev {
			continue
		}
		b.WriteString(substituted[prev:end])
		// A space on BOTH sides: the leading one opens the probe as a sibling
		// attribute; the trailing one ends its NAME, so literal template text
		// glued to the occurrence ("title=[property]SAFE") becomes its own
		// attribute rather than corrupting the probe's name into "…0safe",
		// which then fails the index parse and silently drops the probe.
		b.WriteByte(' ')
		b.WriteString(lintProbe(i))
		b.WriteByte(' ')
		prev = end
		any = true
	}
	if !any {
		return nil
	}
	b.WriteString(substituted[prev:])

	injects := map[int]bool{}
	opts := []html.ParseOption{}
	if !scripting {
		opts = append(opts, html.ParseOptionEnableScripting(false))
	}
	doc, err := html.ParseWithOptions(strings.NewReader(b.String()), opts...)
	if err != nil {
		return injects
	}
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			for _, a := range n.Attr {
				if a.Namespace != "" || !strings.HasPrefix(a.Key, lintProbePrefix) {
					continue
				}
				if idx, err := strconv.Atoi(a.Key[len(lintProbePrefix):]); err == nil {
					injects[idx] = true
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return injects
}

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
	// The oracle for the one danger the parser's placement hides: an unquoted
	// value the parser dropped (a repeated attribute, or a merge duplicate on
	// <body>/<html>) still adds a live sibling attribute, because escaping does
	// not touch the space that opens one. Run in both scripting modes: a drop
	// that happens only with scripting off (the value is inert <noscript> text
	// with it on) still injects there, and its warning is qualified to match.
	injectsOn := unquotedSiblingInjectors(substituted, sentinelAt, lex, true)
	injectsOff := unquotedSiblingInjectors(substituted, sentinelAt, lex, false)
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
		// A sentinel in an HTML raw-text body ("<textarea>...</text[X]>") that
		// could complete that body's own end tag is treated as an interpolated
		// NAME, fail-closed: X could spell the rest of the terminator. Only the
		// HTML raw-text elements have a body the tokenizer reads to "</name>";
		// a foreign body is markup (its interpolated end-tag names are the
		// lexical pass's job) and a CDATA section is character data.
		if primary.inertText || primary.rawTextElement != "" {
			if endTagCompletable(on[i]) && couldCompleteEndTag(substituted, sentinelAt[i], on[i].element) ||
				endTagCompletable(off[i]) && couldCompleteEndTag(substituted, sentinelAt[i], off[i].element) {
				// Completing a raw-text element's own end tag ("</text[x]>"):
				// the value chooses to close it, the same end-tag-name mechanism.
				primary = attrContext{inName: true, endTagName: true}
			}
		}
		// An unquoted value the parser dropped is not harmless: it still adds a
		// live sibling attribute (the oracle confirmed one reaches an element).
		// kAbsent is exactly "placed in no node" — a discarded duplicate, or a
		// merge duplicate on <body>/<html> — so a value the parser DID place
		// keeps its ordinary unquoted rule and is never doubly warned.
		if injectsOn[i] && on[i].kind == kAbsent {
			primary.unquotedSibling = true
		}
		// A value the parser dropped may still be a live raw= breakout site (a
		// merge-dropped <body>/<html> attribute, a <frame> a <frameset> accepts,
		// an end-tag attribute the value closes) rather than a truly dead one
		// (past a foreign-<template> surrender). The breakout probe decides
		// which, per scripting mode; read only by the raw= rules. Run for ANY
		// dropped occurrence, not only attribute values, so frameset TEXT and
		// end-tag-attribute positions are covered too.
		if on[i].kind == kAbsent || off[i].kind == kAbsent {
			reachOn, reachOff := rawBreakoutReachesLive(substituted, sentinelAt[i], len(lintSentinel(i)))
			if reachOn || reachOff {
				primary.rawReachesLive = true
				primary.rawReachesOnlyOff = !reachOn && reachOff
			}
		}
		out[sp.start] = primary
		// The two readings are two real browser configurations, and a
		// placement dangerous in either must be reported. The scripting-off
		// reading is kept alongside, qualified, when it says something the
		// primary does not: a different placement (the tree facts differ), OR a
		// sibling the scripting-off drop injects that the on-mode reading did
		// not warn — which the tree facts alone do NOT reveal, since both modes
		// report kAbsent for different reasons (a <frameset> discards the whole
		// element with scripting on while only its duplicate name is dropped
		// with it off, leaving the element and the injected sibling live).
		offInjects := injectsOff[i] && off[i].kind == kAbsent && !primary.unquotedSibling
		if on[i] != off[i] || offInjects {
			sc := mergeContext(lex[i], off[i])
			sc.noscript = true
			if offInjects {
				sc.unquotedSibling = true
			}
			// The raw= breakout verdict is the occurrence's, not a mode's, so
			// the scripting-off reading carries it too — a value live only with
			// scripting off (a <textarea> body a surrender hid with it on) warns
			// as markup, qualified, through this context rather than as script.
			sc.rawReachesLive = primary.rawReachesLive
			sc.rawReachesOnlyOff = primary.rawReachesOnlyOff
			if sc != primary {
				alt[sp.start] = sc
			}
		}
	}

	// The two fallbacks the parse cannot answer, applied only to occurrences it
	// left unresolved: an interpolated element name (a "<" the parser reads as
	// text because the sentinel is not a name-start), and a tag that is never
	// closed.
	for i, sp := range spans {
		ctx, ok := out[sp.start]
		if !ok {
			continue
		}
		// A settled placement is not re-answered. An inertText from a
		// <noscript> body is settled only if the two scripting modes AGREE it
		// is text: where they differ, the body is markup with scripting off,
		// and an interpolated name in it must fail closed.
		settledInert := ctx.inertText && on[i] == off[i]
		if ctx.inValue || ctx.inName || settledInert || ctx.discarded ||
			ctx.rawTextElement != "" || ctx.unterminated || ctx.unquotedSibling ||
			ctx.rawReachesLive {
			continue
		}
		at := sentinelAt[i]
		if at <= 0 {
			continue
		}
		// The interpolated element name the parser reads as text — a "<"
		// (optionally "</") immediately before the sentinel. With the
		// sentinel being a name-start, the parser usually reads it AS a name
		// (kName), so this is a belt-and-suspenders for the rare non-name-start
		// completion.
		j := at - 1
		if j > 0 && substituted[j] == '/' {
			j--
		}
		if substituted[j] == '<' {
			out[sp.start] = attrContext{inName: true}
		}
	}
	return out, alt
}

// lexFact is the syntactic half of an occurrence's context, from the source's
// tag structure alone.
type lexFact struct {
	inName       bool
	inValue      bool
	discarded    bool
	quoted       bool
	attr         string
	valueSoFar   string
	unterminated bool
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
		// SVG <script>/<style> decode entities in their body — verified
		// against html.Parse, CDATA-wrapped or not — so an SVG body always
		// takes the foreign message (escaping undone); an HTML one (ns "")
		// takes the raw-text message (escaping intact).
		return attrContext{rawTextElement: "script", foreignRoot: t.ns, noscript: t.noscript}
	case kStyle:
		return attrContext{rawTextElement: "style", foreignRoot: t.ns, noscript: t.noscript}
	case kText:
		// The parser placed it in a text node the rules have nothing to say
		// about; recorded as settled so the fallbacks do not re-answer it. The
		// element is carried so the raw= text rule can tell a <plaintext> body
		// — which runs to EOF and admits no markup — from ordinary text.
		return attrContext{inertText: true, element: t.element, noscript: t.noscript}
	default:
		// kAbsent: the occurrence is in no DOM node — dropped by an ignored or
		// duplicate element, or in a name the parser did not keep. Here, and
		// only here, the lexical NAME reading is trusted: an interpolated
		// END-tag name ("</s[x]>") leaves no node, and the lexical scan is the
		// only thing that sees it. A discarded duplicate attribute is the one
		// such position with something left to say (raw= bytes still pass the
		// tokenizer); everything else is left UNRESOLVED for the "<"-name and
		// unterminated-tag fallbacks.
		// The lexical NAME and unterminated readings are trusted ONLY here,
		// where the parser placed the occurrence nowhere: an interpolated
		// END-tag name ("</s[x]>") leaves no node, and an unterminated tag is
		// recovered as text by the parser but must fail closed. When the
		// parser DID place it (as text, an attribute value, a program body),
		// a lexical fact is a phantom from the raw-text-cleared tokenizer and
		// the parser wins.
		if l.unterminated {
			return attrContext{unterminated: true}
		}
		if l.inName {
			// lexFact.inName is set only for an END-tag name (start-tag and
			// attribute names come through the tree's kName above), so this is
			// the "chooses which element closes" mechanism, not "adds
			// attributes".
			return attrContext{inName: true, endTagName: true}
		}
		if l.discarded {
			return attrContext{discarded: true, attr: l.attr, quoted: l.quoted}
		}
		if l.inValue {
			// The lexical pass read an attribute value here, but the parser
			// placed it nowhere — most often a duplicate-named attribute the
			// singleton merge of a re-opened <body>/<html> dropped. It is not a
			// within-one-tag duplicate (that is l.discarded above), so the tag
			// itself is live and a raw= value can still inject onto it. Carry
			// the attribute name so the raw= rule can name it; nothing else
			// fires, since inValue is left false (the value reached no node).
			return attrContext{attr: l.attr, quoted: l.quoted}
		}
		return attrContext{}
	}
}

// htmlRawTextElements are the HTML elements whose body the tokenizer reads as
// one run to "</name>", so a value in the body that completes that name closes
// the element. A foreign element of the same spelling is not one of these.
var htmlRawTextElements = map[string]bool{
	"script": true, "style": true, "textarea": true, "title": true,
	"xmp": true, "iframe": true, "noembed": true, "noframes": true,
}

// endTagCompletable reports whether an occurrence sits in an HTML raw-text
// element's body, the only place couldCompleteEndTag applies.
func endTagCompletable(t treeFact) bool {
	return t.ns == "" && htmlRawTextElements[t.element]
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

// lexicalContexts runs golang.org/x/net/html's own TOKENIZER over the source
// — which delimits TAGS as a browser does, so no hand-written scanner can
// diverge from it there — and reads the syntactic facts off each tag: quoted
// and discarded for a START-tag attribute value, and inName for a sentinel in
// an END-tag NAME. Interpolated START-tag element and attribute names are the
// parser's kName and are not reported here, and an END-tag ATTRIBUTE (ignored
// by the parser) is not a name. Raw-text mode is cleared after every tag
// (NextIsNotRawText), so the tags nested in a <script>, <iframe> or <noscript>
// body ARE emitted and their quoted-ness is read — a sentinel really in one of
// those bodies is answered by the parser (kScript, kText), so this over-reading
// never reaches a rule.
//
// It does NOT enable AllowCDATA or track namespaces, so it does not treat a
// "<![CDATA[" section as the character data a foreign-content parser does: the
// tokenizer reads it as a bogus comment, whose token this switch ignores. That
// is harmless, not authoritative — a sentinel inside CDATA falls in that
// ignored comment and gets the zero lexFact, and the PARSE, which does place it
// in a text node (kText), is what answers it. A sentinel in ordinary text is
// likewise the zero lexFact, answered by the parser.
func lexicalContexts(src string, nspans int) []lexFact {
	facts := make([]lexFact, nspans)
	z := html.NewTokenizer(strings.NewReader(src))
	for {
		tt := z.Next()
		// Never let the tokenizer go into raw-text mode: a browser reads the
		// bodies of <script>, <iframe>, <noscript> and friends as raw text,
		// but for the SYNTACTIC facts this pass wants — the quoted-ness of the
		// tags NESTED in them — every tag has to be emitted. A sentinel that
		// really is in one of those bodies is answered by the parser (kScript,
		// kText), so this over-reading never reaches a rule.
		z.NextIsNotRawText()
		switch tt {
		case html.ErrorToken:
			// A tag the tokenizer could not close runs to EOF and comes back
			// as the final ErrorToken's raw, starting with "<". Its sentinels
			// are inside a tag that is never closed — the quote state that
			// decides where it ends is the tokenizer's, so no hand-written
			// scan can disagree with it.
			raw := string(z.Raw())
			if len(raw) > 1 && raw[0] == '<' && (isTagNameStart(raw[1]) || raw[1] == '/') {
				for _, p := range sentinelIndexes(raw) {
					if p.index >= 0 && p.index < nspans {
						facts[p.index] = lexFact{unterminated: true}
					}
				}
			}
			return facts
		case html.StartTagToken, html.SelfClosingTagToken:
			for _, h := range sentinelsInTag(string(z.Raw())) {
				if h.index < 0 || h.index >= nspans || !h.ctx.inValue {
					continue
				}
				facts[h.index] = lexFact{
					inValue:    true,
					attr:       h.ctx.attr,
					quoted:     h.ctx.quoted,
					valueSoFar: h.ctx.valueSoFar,
					discarded:  h.ctx.discarded,
				}
			}
		case html.EndTagToken:
			// Only the NAME of an end tag matters — a sentinel completing it
			// could name a real element to close; the attributes an end tag
			// may carry are ignored by the parser, so a sentinel there is
			// inert. The name is the run right after "</".
			raw := string(z.Raw())
			k := 2
			for k < len(raw) && !isASCIISpace(raw[k]) && raw[k] != '/' && raw[k] != '>' {
				k++
			}
			if k > 2 {
				for _, p := range sentinelIndexes(raw[2:k]) {
					if p.index >= 0 && p.index < nspans {
						facts[p.index] = lexFact{inName: true}
					}
				}
			}
		}
	}
}

// substituteWorstCase replaces the value of an interpolated encoding= or type=
// attribute that COULD complete a namespace- or stylesheet-deciding value with
// that value, so a downstream placement is read fail-closed. It returns the
// rewritten source and the set of sentinel indices it removed (which the
// caller classifies from the untouched parse).
func substituteWorstCase(src string, nspans int) (string, map[int]bool) {
	removed := map[int]bool{}
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(src))
	for {
		tt := z.Next()
		// Clear raw-text mode after every token, like lexicalContexts, so an
		// encoding= or type= nested in a <noscript> or <script> body is still
		// seen and rewritten to its worst case.
		z.NextIsNotRawText()
		if tt == html.ErrorToken {
			// The final token's raw holds any trailing unterminated tag; it
			// must be copied through, or the reconstructed source is truncated
			// and every sentinel in that tail vanishes from the parse.
			b.WriteString(string(z.Raw()))
			return b.String(), removed
		}
		// Concatenating every token's Raw() reproduces the input exactly, so
		// rewriting only the start tags leaves everything else byte-identical
		// and every sentinel still findable by its prefix.
		raw := string(z.Raw())
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken {
			b.WriteString(rewriteWorstCaseTag(raw, removed, nspans))
		} else {
			b.WriteString(raw)
		}
	}
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
	seen := make([]bool, nspans)
	set := func(idx int, f treeFact) {
		// First writer wins: a character reference elsewhere in the document
		// could decode to a real occurrence's sentinel, and the genuine
		// substituted one is the earlier of the two in document order.
		if idx >= 0 && idx < nspans && !seen[idx] {
			seen[idx] = true
			facts[idx] = f
		}
	}
	var walk func(n *html.Node, noscript bool)
	walk = func(n *html.Node, noscript bool) {
		// A <noscript> makes only its CHILDREN noscript-body content; its own
		// attributes are the element's and are live under both readings.
		childNoscript := noscript
		if n.Type == html.ElementNode && n.Namespace == "" && n.Data == "noscript" {
			childNoscript = true
		}
		switch n.Type {
		case html.ElementNode:
			for _, p := range sentinelIndexes(n.Data) {
				set(p.index, treeFact{kind: kName, noscript: noscript})
			}
			for _, a := range n.Attr {
				// The namespaced name, so an SVG xlink:href is named as itself
				// rather than as the ordinary "href" beside it.
				attrName := a.Key
				if a.Namespace != "" {
					attrName = a.Namespace + ":" + a.Key
				}
				for _, p := range sentinelIndexes(a.Key) {
					set(p.index, treeFact{kind: kName, noscript: noscript})
				}
				for _, p := range sentinelIndexes(a.Val) {
					set(p.index, treeFact{
						kind: kAttrValue, element: n.Data, ns: n.Namespace,
						attr: attrName, prefix: a.Val[:p.at], noscript: noscript,
					})
				}
			}
		case html.CommentNode, html.DoctypeNode:
			// A sentinel the parser put in a comment or doctype is inert
			// text; recorded so the "<"-name fallback does not read comment
			// content as an interpolated tag.
			for _, p := range sentinelIndexes(n.Data) {
				set(p.index, treeFact{kind: kText})
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
				set(p.index, treeFact{kind: kind, element: parentData(n), ns: ns, noscript: childNoscript})
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, childNoscript)
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
		// Only the ordinary, un-namespaced type= decides; an SVG xlink:type is
		// a different attribute and leaves the stylesheet applying.
		if a.Namespace != "" || a.Key != "type" {
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
	// rawDrop qualifies a raw= dropped-position message: when the breakout is
	// one the parser builds only with scripting disabled, it is reachable in
	// that mode alone, so the message carries the scripting-disabled note.
	rawDrop := func(s string) string {
		if ctx.rawReachesOnlyOff {
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
		// one, so no rule about the attribute's meaning applies. Two things
		// still do. raw= — the unescaped bytes pass through the tokenizer
		// whatever the DOM keeps, and a value containing a quote closes the
		// attribute and reshapes the tag. And an UNQUOTED value — a space in
		// it adds a sibling attribute the surviving tag keeps even as the
		// duplicate name is dropped (unquotedSibling, oracle-confirmed).
		var msgs []string
		if raw && ctx.rawReachesLive {
			// The parser drops this duplicate, but the raw= bytes still pass
			// the tokenizer and — where the tag is live (a re-opened <body>, a
			// still-open element) — can close the attribute and add their own.
			// Gated on rawReachesLive so a duplicate past a foreign-<template>
			// surrender, whose tag the parser ignores entirely, is not warned
			// about a breakout that reaches nothing.
			msgs = append(msgs, rawDrop(label+` with raw= is not escaped at all, so placing it in the "`+ctx.attr+`" attribute lets the value close the attribute and add its own. Drop raw= here.`))
		}
		if ctx.unquotedSibling {
			msgs = append(msgs, qualify(label+` sits in an unquoted attribute value, where escaping does not stop a value containing a space from adding attributes of its own. Quote the attribute.`))
		}
		return msgs
	}
	if ctx.rawTextElement != "" {
		// Only script and style ever set this, so the language is never empty.
		if raw {
			// raw= performs no escaping, so the value keeps its "<": it can
			// write "</` + `script>" (or "</` + `style>") and everything after
			// is real markup. That is the hazard, not escaped text reaching
			// the language — and it holds under every reading, noscript
			// included.
			return []string{label + ` with raw= is not escaped, so a value containing markup can close the <` + ctx.rawTextElement + `> and become real elements on the page. Anyone who can edit the entity can then inject script. Drop raw= unless the value is authored by someone you would trust with the template itself.`}
		}
		switch {
		case ctx.noscript && ctx.rawTextElement == "script":
			// A <script> in a <noscript> body reaches nothing under either
			// reading: with scripting enabled the body is inert raw text, and
			// with it disabled the element is real and does not run.
			return nil
		case ctx.foreignRoot != "":
			// A placement reason like the non-foreign one below, so it takes
			// the same qualifier: an SVG <style>/<script> that exists only
			// with scripting disabled (lifted out of a <noscript> by the
			// scripting-off parse) must say so, not read as always live.
			return []string{qualify(label + ` sits inside a <` + ctx.rawTextElement + `> that is inside <` + ctx.foreignRoot +
				`>, which is foreign content, where the parser decodes entities — unlike an HTML <` + ctx.rawTextElement +
				`> body. The escaping is therefore undone before ` + scriptLikeLanguage[ctx.rawTextElement] +
				` sees the value: an escaped quote arrives as a real quote and can ` + foreignEscapeConsequence[ctx.rawTextElement] +
				`. Escaping makes this placement worse than the HTML one rather than safer. Pass the value in through a data- attribute instead.`)}
		}
		return []string{qualify(label + ` sits inside a <` + ctx.rawTextElement + `> body, which the browser does not decode entities in, so the value reaches ` + scriptLikeLanguage[ctx.rawTextElement] + ` with its escaping still in it rather than as the text you wrote. Escaping does not make the placement safe either — a "${...}" in a template literal or a ";" in a declaration contains nothing it touches. Pass the value in through a data- attribute instead.`)}
	}
	if ctx.inName {
		if ctx.endTagName {
			// An end tag's attributes are ignored, so the danger is not added
			// attributes but the choice of WHICH element the name closes: a
			// value of "vg" in "</s[value]>" closes a <svg>, and whatever the
			// close reveals — an <iframe srcdoc>, a raw-text body that was
			// holding markup inert — becomes live. Escaping does not change
			// which element the name matches.
			return []string{qualify(label + ` completes the NAME of an end tag, choosing which element it closes — a value can close an ancestor and make whatever that reveals (an <iframe srcdoc>, a raw-text body) live. Escaping does not change which element the name matches. Build the tag in the template instead.`)}
		}
		return []string{qualify(label + ` is interpolated into a tag or attribute NAME, which nothing delimits: a value containing a space or "=" simply adds attributes of its own, and escaping does not touch either character. Build the name in the template instead.`)}
	}
	if !ctx.inValue || ctx.attr == "" {
		if ctx.unquotedSibling {
			// An unquoted value the parser dropped as a merge duplicate on a
			// re-opened <body>/<html>: the singleton keeps the sibling the
			// value's space opens even though its own name was a duplicate.
			return []string{qualify(label + ` sits in an unquoted attribute value, where escaping does not stop a value containing a space from adding attributes of its own. Quote the attribute.`)}
		}
		if raw && ctx.rawReachesLive && ctx.attr != "" {
			// A raw= value the merge dropped (its name duplicated the
			// singleton's) but whose tag is live: the unescaped bytes still
			// close the attribute and add their own onto the merged element.
			return []string{rawDrop(label + ` with raw= is not escaped at all, so placing it in the "` + ctx.attr + `" attribute lets the value close the attribute and add its own. Drop raw= here.`)}
		}
		if raw && (ctx.inertText || ctx.rawReachesLive) && ctx.element != "plaintext" {
			// raw= disables escaping outright, so it is unsafe wherever it can
			// reach live markup: a Meta value of "<img src=x onerror=...>"
			// becomes a real element. inertText is the parser confirming a live
			// text node; rawReachesLive is the breakout probe confirming a
			// dropped position the value can still reach (frameset TEXT via a
			// <frame>, an end-tag attribute the value closes). A truly dead
			// position — past a foreign-<template> surrender — is neither, so
			// the warning is not false. The one live-text position raw cannot
			// inject is a <plaintext> body, whose tokenizer runs to EOF.
			inject := ` Anyone who can edit the entity can then inject script.`
			if ctx.rawReachesOnlyOff {
				// The breakout is built only with scripting disabled (the probe
				// found it in that mode alone), where no injected handler runs:
				// markup injection, not script. This is NOT the same as being in
				// a <noscript> body, where a raw= value can write "</noscript>"
				// and go on in markup under BOTH readings — that stays unqualified.
				inject = ` Anyone who can edit the entity can then inject markup — no script runs with scripting disabled, but it can still deface the page or add phishing content.` + noscriptQualifier
			}
			return []string{label + ` with raw= is not escaped, so a value containing markup becomes real elements on the page.` + inject + ` Drop raw= unless the value is authored by someone you would trust with the template itself.`}
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
		// A sentinel in the prefix is ANOTHER occurrence in the same value,
		// whose text is unknown. The LITERAL run after that other occurrence
		// decides what is still open: a "/", "?" or "#" makes the URL RELATIVE
		// before this occurrence, which is safe; but a bare ":" does NOT — it
		// only names a scheme whose text is the earlier unknown occurrence,
		// which could be "javascript", leaving this occurrence inside an
		// executable URL. So suppress only on a relative fix (a fixed scheme
		// whose name resolved to ""), and warn otherwise — whether this
		// occurrence could still CHOOSE the scheme (the earlier one is empty)
		// or sits in a scheme the earlier one chose.
		if strings.Contains(ctx.valueSoFar, lintSentinelPrefix) {
			last := strings.LastIndex(ctx.valueSoFar, lintSentinelPrefix)
			if scheme, fixed := urlSchemeBefore(ctx.valueSoFar[last:]); !(fixed && scheme == "") {
				out = append(out, label+` shares the "`+attr+`" URL with an earlier interpolation, so its scheme is not fixed to a safe one — the earlier value could make it "javascript:", and escaping does not stop that. Put a fixed path or a complete scheme at the start of the URL, e.g. href="/x/[meta ...]".`)
			}
		} else {
			scheme, fixed := urlSchemeBefore(ctx.valueSoFar)
			switch {
			case fixed && executableURLSchemes[scheme]:
				out = append(out, label+` continues a "`+scheme+`:" URL in "`+attr+`", which the browser executes rather than fetches.`)
			case !fixed && couldStillBecomeExecutable(scheme):
				out = append(out, label+` can still choose the scheme of the "`+attr+`" URL, and escaping does not stop a "javascript:" value. Put a path or a complete scheme in front of it, e.g. href="/x/[meta ...]".`)
			}
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
			return asciiLower(trimURLLeading(prefix[:i])), true
		case '/', '?', '#':
			return "", true
		}
	}
	return asciiLower(trimURLLeading(prefix)), false
}

// trimURLLeading removes only the LEADING bytes a browser strips from a URL
// before resolving it — C0 controls and ASCII space, everything <= 0x20. It
// does not trim the trailing edge, because this operates on the PREFIX before
// an interpolation, whose trailing side is interior to the completed URL: a
// space there ("java [value]") stays a space the value cannot cross, not an
// edge, so "java script:" is not the javascript: scheme. And it does not trim
// Unicode whitespace (U+00A0, U+2028, …), which a browser keeps, so a leading
// U+00A0 before "javascript:" is not the javascript: scheme either.
func trimURLLeading(s string) string {
	return strings.TrimLeftFunc(s, func(r rune) bool { return r <= 0x20 })
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
//     which foreignRoot carries into the message. html.Parse decodes entities
//     in an SVG body whether or not it is CDATA-wrapped (verified against the
//     parser), so a CDATA section takes the foreign message too.
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
//
// It is one face of the architecture's one real limit: the engine parses ONE
// concrete assignment — each occurrence substituted by its sentinel — so an
// interpolation whose VALUE (not just its own placement) would change the tree
// somewhere ELSE is judged as if it took the sentinel's value, not every value
// it could take. The same shape recurs:
//   - completing a raw-text end tag: "<textarea></text[Close]>", Close="area"
//     closes the textarea and makes a later <iframe srcdoc> live.
//   - adding a deciding attribute: "<annotation-xml title=[A]>", A could be
//     `x encoding=text/html`, making a later <script> HTML content.
//   - deciding a document phase: an empty text run or an <input type=[T]>
//     before a <frameset> decides whether the frameset is honored, and so
//     whether a <frame src=...> after it exists at all.
//   - completing a character reference: `href="java&#[C]cript:"`, C="115;"
//     decodes to "javascript:"; and the same across two interpolations —
//     `encoding="text&#[E];html"` with E="47" selects text/html, making a later
//     <script> a program. This is NOT explored, in a URL or an encoding value.
//     An attempt to read a trailing "&#" as a value-chosen scheme was reverted:
//     ctx.valueSoFar is entity-DECODED, so an existing partial reference has
//     already lost the digits the answer depends on ("&#0" is U+FFFD there, not
//     "0"), and reasoning from the decoded prefix produced both a miss and a
//     false positive. A completing occurrence in a reference is therefore not
//     warned in place either — the reference spans the occurrence, and what it
//     resolves to is exactly the unexplored value.
// The one deciding case the engine DOES explore is a SINGLE encoding=/type=
// interpolation (substituteWorstCase parses it at its dangerous completion),
// because a namespace or stylesheet it might select is a common, bounded case;
// the rest are left because the completion is unbounded and re-warning the tail
// on the chance the author chose the one dangerous value flags markup safe under
// every other choice — the false positive this file treats as worse than a
// miss. Where there IS a completing occurrence whose OWN placement is dangerous
// it is warned in place (the interpolated name, the unquoted value), which is
// the signal that the region is author-controlled; a reference-completing one,
// whose own placement is an ordinary escaped value, is not.
//
// A last, non-exploitable corner: a character reference in the template that
// decodes to a genuine occurrence's sentinel could shadow it. The sentinel
// carries a per-process CRYPTO-RANDOM nonce, so no template author can produce
// it on purpose, and a chance collision needs the input to contain those 48
// random bits (literally or entity-encoded) — about 2^-48. classifyByParse's
// first-writer-wins keeps the genuine, earlier placement in the ordinary case.

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
