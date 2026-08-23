package shortcodes

import (
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
// So the tag-finding is delegated to golang.org/x/net/html, which is the
// browsers' own reading of all of that. What is left — which attribute of one
// already-delimited tag a byte offset falls in, and whether that attribute was
// quoted — is a bounded scan over a single tag's source with no nesting and no
// escapes to resolve, and that part was never the problem.
//
// The tokenizer is not a parser, and in two places its reading is not a
// browser's: it raw-texts ten element names wherever they appear, while a
// browser only does so in the HTML namespace, and it raw-texts <noscript>
// whatever the scripting mode. Both are handled by reading the region the
// tokenizer handed back as one text token again, as markup, in scanMarkup —
// rather than by moving off the tokenizer, which would cost the byte offsets
// every diagnostic here is anchored to.

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
	// unterminated marks an occurrence inside a tag that is never closed, which
	// the tokenizer cannot place. Treated as unsafe rather than as "not in an
	// attribute", so a broken template fails closed.
	unterminated bool
}

const lintSentinelPrefix = "\x00mahlint"

func lintSentinel(i int) string { return lintSentinelPrefix + strconv.Itoa(i) + "\x00" }

// attributeContextsFor answers, for every occurrence, which attribute it sits
// in. Each occurrence is replaced by an inert unique sentinel, the result is
// tokenized, and the sentinels are located in the tags that come back.
func attributeContextsFor(input string, spans []bareValueSpan) map[int]attrContext {
	out := make(map[int]attrContext, len(spans))
	if len(spans) == 0 {
		return out
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })

	// A template that already contains the sentinel bytes could otherwise remap
	// one occurrence onto another's context. NUL carries no meaning in a
	// template, and replacing it byte-for-byte keeps every span offset valid.
	if strings.IndexByte(input, 0) >= 0 {
		input = strings.ReplaceAll(input, "\x00", " ")
	}

	var b strings.Builder
	// sentinelAt[i] is where span i's sentinel begins in the substituted text,
	// or -1 for a span that was not substituted. Recording it here rather than
	// searching for it afterwards is half of what keeps the fallback below
	// linear: a strings.Index over the whole document, once per span, made a
	// template full of bare values quadratic in how many it held.
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
	scanMarkup(substituted, scanMode{}, 0, func(index int, ctx attrContext) {
		if index >= 0 && index < len(spans) {
			out[spans[index].start] = ctx
		}
	})

	// An unterminated tag never reaches the tokenizer as a tag, so its
	// occurrences come back unresolved. Reporting those as "not in an
	// attribute" would fail open on exactly the template that is most broken:
	// <div title="[meta ... raw="true"] with no ">" is still an attribute an
	// author is interpolating into. Mark them unknown so the caller warns.
	//
	// Resolving to "not unterminated" writes nothing, deliberately: a raw-text
	// body's occurrence passes through here too, and it keeps that context
	// unless one of the two answers below is the better one. Both can overwrite
	// it — the element-name check by design, and a "<" written inside a script
	// string does reach it, which is fail-closed but a worse message.
	var ask []int     // sentinel offsets, ascending, still needing the test
	var askSpan []int // parallel: which span each of those belongs to
	for i, sp := range spans {
		if ctx, ok := out[sp.start]; ok && (ctx.inValue || ctx.inName) {
			continue
		}
		at := sentinelAt[i]
		if at < 0 {
			continue
		}
		// An interpolated ELEMENT name never reaches the tokenizer as a tag:
		// "<" followed by anything that is not a name character is text, and the
		// sentinel is deliberately not a name character. The "<" immediately
		// before it is the whole proof, so no tokenizer is needed.
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
	return out
}

// scanMode is how a browser would read the source being scanned. Neither half
// of it is anything the tokenizer knows: it reads one namespace and one
// scripting mode, and both of those are decided by the markup around the region
// rather than by the region itself.
type scanMode struct {
	// foreign is "" for HTML, or the name of the <svg>/<math> whose content
	// this is. Inside one, entities are decoded everywhere and the ten names in
	// rawTextElements are ordinary element names — with the two exceptions SVG
	// keeps, an <svg><script> that executes and an <svg><style> that applies.
	foreign string
	// noscript marks a <noscript> body. It is real markup with scripting
	// disabled and inert raw text with scripting enabled, so the placements
	// reachable there are the ones that need no script to hurt.
	noscript bool
	// enclosing names the element whose body this scan was handed, which is the
	// one element that can be closed from inside the region without ever having
	// been opened in it. Empty at the top of a document, and for the siblings of
	// a self-closed foreign element, which close nothing.
	enclosing string
}

// maxMarkupScanDepth bounds the recursion in scanMarkup. Each level builds a
// tokenizer over a copy of the region it reads, so a document that nests these
// regions without limit — "<svg><iframe><svg><iframe>…" — would cost
// O(len × depth) in both time and memory. Real markup does not nest them at
// all; past the bound a region is left unanalysed, which is what happened to
// every one of them before this existed.
const maxMarkupScanDepth = 8

// markupFrame is one open element, carrying the little this scan wants from
// it: its own namespace, and whether it is one of the two kinds of integration
// point, which is decided when it is opened because <annotation-xml> needs its
// encoding attribute to answer.
type markupFrame struct {
	name            string
	ns              string // "", "svg" or "math": this element's own namespace
	htmlIntegration bool
	mathText        bool
}

// namespaceForChild answers which namespace a start tag with this name lands in
// when it is opened inside this element. It is x/net/html's inForeignContent
// written as a function of one frame, and the order of the clauses is that
// function's: the MathML text points hand over to HTML for every start tag
// except <mglyph> and <malignmark>, an <svg> inside <annotation-xml> is taken
// by HTML rules whatever the encoding says (and the HTML rule for <svg> then
// enters SVG), and an HTML integration point hands over unconditionally.
func (f markupFrame) namespaceForChild(name string) string {
	if f.ns == "" {
		return ""
	}
	if f.mathText && name != "mglyph" && name != "malignmark" {
		return ""
	}
	if f.ns == "math" && f.name == "annotation-xml" && name == "svg" {
		return ""
	}
	if f.htmlIntegration {
		return ""
	}
	return f.ns
}

// keepsForeignContent reports whether a breakout stops at this frame. x/net's
// parseForeignContent discards every open element above the nearest one that is
// HTML or an integration point, and these are those three.
func (f markupFrame) keepsForeignContent() bool {
	return f.ns == "" || f.htmlIntegration || f.mathText
}

// scanMarkup tokenizes src, records the attribute context of every sentinel it
// can place, and calls itself on the regions the tokenizer cannot describe.
//
// Those regions exist because the tokenizer is namespace-unaware: it raw-texts
// the ten names in rawTextElements wherever they appear, and a browser only
// does so in the HTML namespace. Inside <svg> or <math> an <iframe> is an inert
// foreign element whose children are real, so the markup the tokenizer handed
// back as one text token has to be read again — as markup this time. The same
// shape answers <noscript>, whose body the tokenizer raw-texts unconditionally
// while a browser does so only when scripting is enabled.
//
// depth is the recursion depth, bounded by maxMarkupScanDepth.
func scanMarkup(src string, mode scanMode, depth int, record func(index int, ctx attrContext)) {
	z := html.NewTokenizer(strings.NewReader(src))
	var stack []markupFrame
	// base is the namespace of content directly at the top of this scan, which
	// for a recursion is the namespace of the region it was handed. A breakout
	// tag can clear it: past one a browser has left foreign content for good,
	// and so has everything this scan has left to read.
	base := mode.foreign
	// enclosing is the frame a token at the top of this scan is opened inside.
	// The region's own element is not on the stack, and it does not need to be:
	// base already carries the namespace its children are in, and no element
	// this scan can be handed the inside of is an integration point of a kind
	// that would still matter (the ten raw-text names include none of the
	// MathML text points).
	enclosing := func() markupFrame {
		if n := len(stack); n > 0 {
			return stack[n-1]
		}
		return markupFrame{ns: base}
	}
	leaveForeignContent := func() {
		for len(stack) > 0 && !stack[len(stack)-1].keepsForeignContent() {
			stack = stack[:len(stack)-1]
		}
		if len(stack) == 0 {
			base = ""
		}
	}

	// pending describes the raw-text body the tokenizer is about to hand back
	// as a single text token, and what this scan makes of it.
	var pending struct {
		language string   // script or style, whose body is a program
		foreign  string   // the foreign root that program sits in, "" for HTML
		reread   bool     // read the body as markup instead
		mode     scanMode // ... in this mode
	}
	forget := func() { pending.language, pending.foreign, pending.reread = "", "", false }

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return
		case html.StartTagToken, html.SelfClosingTagToken:
			raw := string(z.Raw())
			nameBytes, hasAttr := z.TagName()
			name := string(nameBytes)

			ns := enclosing().namespaceForChild(name)
			if ns != "" && breaksOutOfForeignContent(name, z, hasAttr) {
				leaveForeignContent()
				ns = enclosing().namespaceForChild(name)
			}
			if ns == "" && (name == "svg" || name == "math") {
				ns = name
			}
			// The namespace is settled before this tag's own attributes are
			// recorded, because it decides what some of them mean: a srcdoc= on
			// a foreign element is an attribute of an element with no browsing
			// context, not a document waiting to be parsed.
			for _, hit := range sentinelsInTag(raw) {
				ctx := hit.ctx
				ctx.noscript = mode.noscript
				ctx.foreignRoot = ns
				record(hit.index, ctx)
			}

			frame := markupFrame{
				name:            name,
				ns:              ns,
				htmlIntegration: isHTMLIntegrationPoint(ns, name, z, hasAttr),
				mathText:        ns == "math" && mathTextIntegrationPoints[name],
			}
			forget()
			if rawTextElements[name] {
				switch {
				case ns == "":
					// HTML: the tokenizer's reading is a browser's, except for
					// <noscript>, whose body is markup with scripting off.
					if scriptLikeElements[name] {
						pending.language = name
					} else if name == "noscript" {
						pending.reread = true
						pending.mode = scanMode{noscript: true, enclosing: name}
					}
				case tt == html.SelfClosingTagToken:
					// "/>" really closes in foreign content, so what the
					// tokenizer offers as this element's body is its siblings,
					// which share this element's own namespace.
					pending.reread = true
					pending.mode = scanMode{foreign: ns, noscript: mode.noscript}
				case ns == "svg" && scriptLikeElements[name]:
					// SVG is the one foreign namespace that has these two: an
					// SVG <script> executes — the parser spec special-cases its
					// end tag and has nothing of the sort for MathML — and an
					// SVG <style> is a real stylesheet. So still a program, and
					// still worth naming its language; but foreign content
					// decodes entities, which inverts what escaping does to it.
					pending.language, pending.foreign = name, ns
				default:
					// Everything else here is an inert foreign element whose
					// children are real markup, including a MathML <script> or
					// <style>, which no browser runs or applies.
					pending.reread = true
					pending.mode = scanMode{foreign: frame.namespaceForChild("span"), noscript: mode.noscript, enclosing: name}
				}
			}
			if tt != html.SelfClosingTagToken {
				stack = append(stack, frame)
			}
		case html.EndTagToken:
			forget()
			nameBytes, _ := z.TagName()
			name := string(nameBytes)
			// "</br>" and "</p>" are the end-tag half of the breakout rule.
			if enclosing().ns != "" && (name == "br" || name == "p") {
				leaveForeignContent()
				continue
			}
			matched := false
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].name == name {
					stack, matched = stack[:i], true
					break
				}
			}
			// An end tag matching nothing this scan opened closes an element
			// opened OUTSIDE it, and exactly two of those end the region: a
			// foreign root, and the element whose body this is.
			// "<svg><iframe></svg><textarea>…" is the shape — a browser pops
			// both and reads the textarea as RCDATA, and without this the scan
			// would go on calling it foreign and warn about a link a browser
			// only ever shows as text.
			//
			// Any OTHER unmatched end tag is left alone on purpose. A browser
			// ignores a stray "</textarea>" in foreign content and stays where
			// it is, and treating one as an exit would put the scan back into
			// HTML, where a srcdoc= that is inert on a foreign element starts
			// warning again.
			if !matched && base != "" && (name == "svg" || name == "math" || name == mode.enclosing) {
				stack, base = stack[:0], ""
			}
		case html.TextToken:
			switch {
			case pending.reread:
				if depth < maxMarkupScanDepth {
					scanMarkup(string(z.Raw()), pending.mode, depth+1, record)
				}
			case pending.language != "":
				// A script or style body in the HTML namespace is where
				// escaping helps least, not most: the parser decodes no
				// entities there, so the value lands in JavaScript or CSS with
				// its escaping still in it. Contained against a quote or a "<",
				// and no help at all against a backtick, a "${...}", a ";" or a
				// brace, none of which it touches. In an SVG one the decoding
				// does happen, and unsafeAttributeContexts says so.
				for _, idx := range sentinelIndexes(string(z.Raw())) {
					record(idx.index, attrContext{
						rawTextElement: pending.language,
						foreignRoot:    pending.foreign,
						noscript:       mode.noscript,
					})
				}
			}
		}
	}
}

// breaksOutOfForeignContent reports whether this start tag takes a browser back
// out of <svg>/<math> and into HTML rules.
//
// It is not a refinement: without it an <svg> the author forgot to close would
// make every later <textarea> and <iframe> foreign content, and this file would
// warn about a link written inside one when a browser reads it as text. A
// template typo is the last input that should draw a warning.
//
// The names are the HTML parser's own list (section 12.2.6.5, and x/net/html's
// "breakout" map in foreign.go), copied rather than reasoned out.
func breaksOutOfForeignContent(name string, z *html.Tokenizer, hasAttr bool) bool {
	if foreignBreakoutTags[name] {
		return true
	}
	// <font> is one of them only when it carries a presentational attribute.
	if name != "font" {
		return false
	}
	for hasAttr {
		var key []byte
		key, _, hasAttr = z.TagAttr()
		switch string(key) {
		case "color", "face", "size":
			return true
		}
	}
	return false
}

// isHTMLIntegrationPoint reports whether this element's children are read with
// HTML rules again despite it sitting inside <svg> or <math>. <svg><title> is
// the one that earns its keep: it is the accessible name of an inline icon, so
// it is ordinary in a category template, and its content is HTML rather than
// SVG.
func isHTMLIntegrationPoint(ns, name string, z *html.Tokenizer, hasAttr bool) bool {
	switch ns {
	case "svg":
		return name == "desc" || name == "foreignobject" || name == "title"
	case "math":
		if name != "annotation-xml" {
			return false
		}
		// Only with an HTML encoding; with any other one, or none, the children
		// are still MathML. (A start-tag <svg> inside one is taken by HTML rules
		// regardless, which namespaceForChild handles separately.)
		for hasAttr {
			var key, val []byte
			key, val, hasAttr = z.TagAttr()
			if string(key) != "encoding" {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(string(val))) {
			case "text/html", "application/xhtml+xml":
				return true
			}
			return false
		}
	}
	return false
}

// mathTextIntegrationPoints hand their children to HTML rules, with the two
// exceptions namespaceForChild names.
var mathTextIntegrationPoints = map[string]bool{
	"mi": true, "mo": true, "mn": true, "ms": true, "mtext": true,
}

// rawTextElements are the ten names golang.org/x/net/html's tokenizer reads as
// raw text or RCDATA. It does so by name and wherever the name appears, which
// is the whole of the foreign-content problem: a browser only reads them that
// way in the HTML namespace.
var rawTextElements = map[string]bool{
	"iframe": true, "noembed": true, "noframes": true, "noscript": true,
	"plaintext": true, "script": true, "style": true, "textarea": true,
	"title": true, "xmp": true,
}

// foreignBreakoutTags are the HTML start tag names that end foreign content,
// copied from section 12.2.6.5 by way of x/net/html's own "breakout" map.
// <font> is conditional and is handled in breaksOutOfForeignContent.
var foreignBreakoutTags = map[string]bool{
	"b": true, "big": true, "blockquote": true, "body": true, "br": true,
	"center": true, "code": true, "dd": true, "div": true, "dl": true,
	"dt": true, "em": true, "embed": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "head": true, "hr": true, "i": true,
	"img": true, "li": true, "listing": true, "menu": true, "meta": true,
	"nobr": true, "ol": true, "p": true, "pre": true, "ruby": true, "s": true,
	"small": true, "span": true, "strong": true, "strike": true, "sub": true,
	"sup": true, "table": true, "tt": true, "u": true, "ul": true, "var": true,
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
}

// sentinelsInTag scans one tag's own source for sentinels sitting in attribute
// values. The tag is already delimited by the tokenizer, so this is a flat walk
// over "name=value" pairs — no comments, no raw-text bodies, no nesting.
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
		name := strings.ToLower(tag[nameStart:i])
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
			ctx := attrContext{attr: name, quoted: q != 0, inValue: !duplicate}
			// Only the URL rules read the prefix, and unescaping a growing
			// prefix once per occurrence is quadratic in a value holding many.
			if urlBearingAttrs[name] {
				// html.UnescapeString so a prefix written as "java&#x73;cript"
				// is judged as the "javascript" the browser will see.
				ctx.valueSoFar = html.UnescapeString(value[:idx.at])
			}
			hits = append(hits, sentinelHit{index: idx.index, ctx: ctx})
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
		end := strings.IndexByte(rest, 0)
		if end < 0 {
			return out
		}
		if n, err := strconv.Atoi(rest[:end]); err == nil {
			out = append(out, sentinelPos{index: n, at: i})
		}
		from = i + len(lintSentinelPrefix) + end + 1
	}
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
// the URL rules, deliberately — an SVG <a href="javascript:…"> runs, and those
// rules have always matched on attribute name rather than on element, the same
// looseness that warns about <div href="javascript:…"> in HTML.
//
// ctx.noscript: a <noscript> body is markup only when scripting is disabled, so
// the rules that describe execution are withheld there entirely and the ones
// that describe a placement carry noscriptQualifier.
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
	if ctx.rawTextElement != "" {
		// Only script and style ever set this, so the language is never empty.
		switch {
		case ctx.noscript:
			// Neither reading of a <noscript> body puts a language behind this.
			// With scripting enabled the body is inert raw text; with it
			// disabled the <script> is a real element that does not run, and a
			// <style> is the residue named in scriptLikeElements' comment. What
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
		return []string{label + ` sits inside a <` + ctx.rawTextElement + `> body, which the browser does not decode entities in, so the value reaches ` + scriptLikeLanguage[ctx.rawTextElement] + ` with its escaping still in it rather than as the text you wrote. Escaping does not make the placement safe either — a "${...}" in a template literal or a ";" in a declaration contains nothing it touches. Pass the value in through a data- attribute instead.`}
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
	// it says nothing about the same attribute written on a foreign one.
	if alwaysUnsafeAttrs[attr] && ctx.foreignRoot == "" {
		out = append(out, qualify(label+` sits in a "`+attr+`" attribute, whose value the browser decodes and parses as HTML, so escaping does not prevent script injection anywhere in it.`))
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
			return strings.ToLower(strings.TrimSpace(prefix[:i])), true
		case '/', '?', '#':
			return "", true
		}
	}
	return strings.ToLower(strings.TrimSpace(prefix)), false
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

// scriptLikeElements are the two whose body is a program rather than markup, so
// a value placed in one is judged as landing in a language. In the HTML
// namespace the parser decodes no entities there, so the value arrives with its
// escaping still in it and escaping is not what decides whether the placement is
// safe. It is not nothing — an escaped value carries no "<" and no bare quote,
// so it can neither close the element nor end a JavaScript string — but a
// backtick, a "${...}" or a ";" is not escaped at all, and those are the
// characters that matter in a language. In foreign content the sentence inverts;
// see below.
//
// The tokenizer raw-texts ten names (rawTextElements), and no tag inside any of
// them is emitted as a tag. Two of the ten are here. In the HTML namespace the
// other eight are read as ordinary prose, which is a decision rather than an
// oversight: it has been got wrong twice, once by warning on all eight bodies
// and once by re-reading a <noscript> body as markup and reporting placements
// in it that cannot execute under either reading.
//
// textarea and title are RCDATA: entities *are* decoded there, so escaping
// works exactly as it does in an attribute — and a tag written inside one is
// literal text to the browser too, which is precisely what the zero-value "not
// in an attribute" context already reports.
//
// iframe, noembed, noframes, xmp and plaintext are raw text, which makes the
// same answer right for a different reason: a tag inside one is text to the
// browser as well, so "not in an attribute" is the truth rather than a gap. An
// escaped value cannot even close the element — html.EscapeString leaves no "<"
// and no entity is decoded to give one back — and a raw value that closes it
// with "</xmp>" is already covered by the raw= rule in
// unsafeAttributeContexts, which is the message that case wants. plaintext runs
// to EOF and cannot be closed at all.
//
// An UNCLOSED one of those seven swallows the rest of the document rather than
// a body, and that is not a fail-open either: a browser's tokenizer runs to EOF
// in raw text (or RCDATA, or PLAINTEXT) too, so the href further down that this
// file then says nothing about is not a link there either.
//
// Both of the places where the tokenizer's reading is not a browser's — the
// <noscript> body and foreign content — are handled by re-reading the region in
// scanMarkup rather than by being listed here.
//
// NOSCRIPT is raw text to the tokenizer whatever the scripting flag says, and
// markup to a browser only when scripting is disabled — which is the mode the
// element exists for, so the placements live in that mode are worth reporting.
// The set that is reported is exactly the one that needs no script: srcdoc, an
// unquoted attribute, style=, an interpolated attribute name, and raw=. Every
// script-execution rule (an on* handler, an Alpine directive, a javascript: URL,
// a <script> body) is withheld, because it is inapplicable under BOTH readings —
// with scripting on the body is inert raw text, with it off the handler is real
// and does nothing — and reporting it is the false positive that had the earlier
// attempt withdrawn. Every message from a body carries noscriptQualifier, which
// names the mode it applies in. An unclosed <noscript> is the same case at its
// widest and is read the same way, since with scripting disabled everything
// after it is live markup.
//
// Two members of the non-execution set stay unreported and are residue. A
// <style> ELEMENT inside a <noscript> is a real stylesheet in that mode, and is
// left out because it is the language rule above rather than a placement rule,
// and the language rule reports a <script> in the same breath. And every
// URL-bearing attribute other than an executable scheme — a stylesheet href, a
// <base href> that re-points every relative URL on the page, a form action — is
// out because the two URL rules only ever describe an executable scheme, which
// is exactly what that mode cannot reach.
//
// One thing here is not a <noscript> matter at all and is unwarned everywhere: a
// refresh <meta content="0;url=…"> chooses a navigation target, and "content"
// is not in urlBearingAttrs — recognising it needs the sibling http-equiv,
// since the same attribute on <meta name="description"> is prose.
//
// FOREIGN CONTENT is where the tokenizer is simply wrong rather than
// conservative, because it is namespace-unaware and raw-texts all ten names
// inside <svg> and <math> as well. Three consequences, all handled by
// scanMarkup and the namespace it puts on every context:
//
//   - An SVG <script> or <style> still holds a program — the parser spec
//     special-cases the end tag of an SVG script and has nothing of the sort
//     for MathML, and an SVG <style> is a real stylesheet — but foreign content
//     DOES decode entities, so the escaping is undone before the language sees
//     the value and an escaped quote arrives as a real quote. That placement is
//     worse than the HTML one, not better, and the old message claimed the
//     opposite.
//   - MathML has neither, so a <math><script> runs nothing and a <math><style>
//     applies nothing. Both are read as the ordinary markup they are.
//   - The remaining names are inert foreign elements whose children are real,
//     so the region is read again as markup. <svg><iframe><a
//     href="javascript:…"> is a live SVG link and was silent; <svg><title> is
//     an HTML integration point, so the anchor in it is a real HTML anchor.
//
// An element being foreign also narrows the rules that describe what one
// particular HTML element DOES, since a foreign element is not that element
// however it is spelled: a foreign <iframe> has no browsing context, so its
// srcdoc is never parsed as a document. What is untouched is everything about
// markup syntax (raw=, an unquoted value, an interpolated name), the two
// attributes every namespace honours (style=, on*), and the URL rules, which
// have always matched on attribute name rather than on element.
//
// Three things keep this from becoming a third false-positive machine, and each
// was a false positive first. breaksOutOfForeignContent: ~40 HTML tag names end
// foreign content (the list copied from x/net's own breakout map, plus the
// "</br>"/"</p>" end tags), so an <svg> the author forgot to close does not turn
// every later <textarea> into a live region. markupFrame.namespaceForChild:
// x/net's inForeignContent written against one frame, so <svg><title>, the
// MathML text points and their <mglyph>/<malignmark> exceptions, and an <svg>
// inside <annotation-xml> all land where a browser puts them. And an end tag
// naming a foreign root or the region's own element ends the region, because a
// re-read region does not carry the elements open above it.
//
// TestLintPlacementsAgainstTheParser is what holds all of it: it asks
// x/net/html's parser, over a few thousand generated nests, whether the element
// that would make each placement dangerous exists at all.
var scriptLikeElements = map[string]bool{"script": true, "style": true}

var scriptLikeLanguage = map[string]string{"script": "JavaScript", "style": "CSS"}

// noscriptQualifier names the mode a placement reason applies in, for the
// reasons reported from inside a <noscript> body. It deliberately does not say
// the value is harmless with scripting enabled; qualify, in
// unsafeAttributeContexts, is where which reasons take it is decided and why.
const noscriptQualifier = ` This is inside a <noscript>, whose body is markup only when scripting is disabled — which is the mode that element exists for.`

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
