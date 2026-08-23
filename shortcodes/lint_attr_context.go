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
// The tokenizer is not a parser, and in three places its reading is not a
// browser's: it raw-texts ten element names wherever they appear, while a
// browser only does so in the HTML namespace; it raw-texts <noscript> whatever
// the scripting mode; and it reads "<![CDATA[" as a bogus comment everywhere
// unless told otherwise, while a browser reads a real CDATA section in foreign
// content. All three are corrected through the tokenizer's own escape hatches,
// used the way x/net/html's parser uses them: NextIsNotRawText keeps a foreign
// or <noscript> body streaming through the one tokenizer as ordinary tokens,
// and AllowCDATA is set before every token from the namespace of the element
// the scan is inside.

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
	// inertText marks an occurrence in a body the scan read and deliberately
	// had nothing to say about — an HTML raw-text or RCDATA body, where a "<"
	// starts nothing and a tag is literal text. It is the zero value as far as
	// the rules are concerned, and it exists only to keep the two fallbacks
	// below from answering a question the scan already settled.
	inertText bool
	// noscriptRaw marks an occurrence the scripting-enabled reading put inside
	// a <noscript> raw-text body. No rule reads it: it exists only so
	// attributeContextsFor knows which occurrences to ask the scripting-off
	// reading about, and it never survives that merge.
	noscriptRaw bool
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
	// First reading: scripting enabled, which is the default mode and the
	// tokenizer's own — a <noscript> body is raw text ending at the first
	// "</noscript>", and everything after that is live markup whatever the
	// body contained. An occurrence that lands INSIDE such a body is dead in
	// this reading and comes back marked noscriptRaw instead of answered.
	sawNoscriptBody := false
	scanMarkup(substituted, true, func(index int, ctx attrContext) {
		if ctx.noscriptRaw {
			sawNoscriptBody = true
		}
		if index >= 0 && index < len(spans) {
			out[spans[index].start] = ctx
		}
	})
	// Second reading, only when the first left questions: scripting disabled,
	// where the body is markup — in which an unclosed <textarea> can swallow
	// the very "</noscript>" the first reading stopped at, so the two
	// readings are different segmentations of one document and neither can
	// stand in for the other. Each marked occurrence takes its answer from
	// this reading, with noscript forced on it: whatever the scripting-off
	// scan made of it, its liveness is conditional on that mode, because the
	// scripting-on reading already showed it as raw text — including when an
	// end tag inside the body popped the <noscript> element itself before the
	// occurrence was reached.
	if sawNoscriptBody {
		off := make(map[int]attrContext, len(spans))
		scanMarkup(substituted, false, func(index int, ctx attrContext) {
			if index >= 0 && index < len(spans) {
				off[index] = ctx
			}
		})
		for i, sp := range spans {
			if ctx, ok := out[sp.start]; ok && ctx.noscriptRaw {
				oc := off[i]
				oc.noscript = true
				out[sp.start] = oc
			}
		}
	}

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
		if ctx, ok := out[sp.start]; ok && (ctx.inValue || ctx.inName || ctx.inertText) {
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

// markupFrame is one open element, carrying the little this scan wants from
// it: its own namespace, whether it is one of the two kinds of integration
// point (decided when it is opened, because <annotation-xml> needs its
// encoding attribute to answer), and the two per-element readings that follow
// from the namespace: whether it is a <noscript> read as markup, and whether
// its child text is program source.
type markupFrame struct {
	name            string
	ns              string // "", "svg" or "math": this element's own namespace
	htmlIntegration bool
	mathText        bool
	// noscript marks an HTML <noscript>, whose body this scan reads as markup
	// — the scripting-off reading, which is the mode the element exists for —
	// while every rule that describes execution is withheld for anything
	// inside one.
	noscript bool
	// program names this element when it is an SVG <script> or an SVG <style>
	// a browser would apply: the one foreign namespace where those two do
	// their HTML job while their bodies are still markup. A DIRECT child text
	// node of one is program source; anything deeper sits inside some other
	// frame and is markup like everything else.
	program string
	// id makes one opened element distinguishable from another spelled the
	// same, which the active formatting list needs: whether an entry is still
	// OPEN is a question about this element, not about its name.
	id int
}

// afeEntry is one entry in the list of active formatting elements: a
// formatting element that was opened, or the marker an applet, marquee,
// object, template, td, th or caption inserts so reconstruction cannot reach
// past it.
type afeEntry struct {
	frame  markupFrame
	marker bool
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

// scanMarkup tokenizes src and records the attribute context of every sentinel
// it can place, reading src the way a browser with the given scripting mode
// reads it.
//
// The tokenizer is namespace-unaware, and left alone its reading is not a
// browser's in two places: it raw-texts the ten names in rawTextElements
// wherever they appear, while a browser does so only in the HTML namespace —
// inside <svg> or <math> an <iframe> is an inert foreign element whose
// children are real markup — and it raw-texts <noscript>, which is a
// browser's reading only when scripting is enabled. Both are corrected the
// way x/net/html's own parser corrects them (parse.go: "Don't let the
// tokenizer go into raw text mode in foreign content", and its scripting-off
// <noscript> cases): Tokenizer.NextIsNotRawText clears the armed raw-text
// state, so the element's body streams through this same loop as ordinary
// tokens, read against the same stack. No region is copied and re-read and no
// second tokenizer exists, so a CDATA section swallows a fake end tag the way
// a browser's does, and an end tag inside such a body acts exactly where it
// stands — on the one stack, in document order.
//
// scripting decides only what a <noscript> body is: with it enabled the body
// is raw text and every sentinel inside comes back marked noscriptRaw, and
// with it disabled the body is markup. attributeContextsFor runs the enabled
// reading first and asks the disabled one about exactly the marked positions,
// because the two readings can disagree about where markup resumes.
func scanMarkup(src string, scripting bool, record func(index int, ctx attrContext)) {
	z := html.NewTokenizer(strings.NewReader(src))
	var stack []markupFrame
	// enclosing is the frame the next token is opened inside. The bottom of
	// the stack is the document itself, which is HTML.
	enclosing := func() markupFrame {
		if n := len(stack); n > 0 {
			return stack[n-1]
		}
		return markupFrame{}
	}
	inNoscript := func() bool {
		for _, f := range stack {
			if f.noscript {
				return true
			}
		}
		return false
	}
	leaveForeignContent := func() {
		for len(stack) > 0 && !stack[len(stack)-1].keepsForeignContent() {
			stack = stack[:len(stack)-1]
		}
	}
	htmlFrameOpen := func(name string) bool {
		for _, f := range stack {
			if f.ns == "" && f.name == name {
				return true
			}
		}
		return false
	}
	// closeP is the tree constructor's "close a p element in button scope",
	// which nearly every block-level start tag runs before inserting itself.
	closeP := func() {
		for i := len(stack) - 1; i >= 0; i-- {
			f := stack[i]
			if f.ns == "" && f.name == "p" {
				stack = stack[:i]
				return
			}
			if scopeBoundary(f) || (f.ns == "" && f.name == "button") {
				return
			}
		}
	}
	// The list of active formatting elements. A formatting element a scope
	// pop or the agency's surgery closed does not leave this list, and that
	// is the point of it: the next insertion in body content RECONSTRUCTS
	// every listed element that is no longer open, the way a browser re-opens
	// a <b> across block boundaries. This is not namespace-cosmetic — a
	// reconstructed <a> stands between an <svg> and the "</a>" that will run
	// the agency over both, so without it the scan and the parser disagree
	// about whether that svg is still open.
	var afe []afeEntry
	nextID := 0
	afeIndex := func(name string) int {
		for i := len(afe) - 1; i >= 0 && !afe[i].marker; i-- {
			if afe[i].frame.name == name {
				return i
			}
		}
		return -1
	}
	onStack := func(id int) bool {
		for _, f := range stack {
			if f.id == id {
				return true
			}
		}
		return false
	}
	clearAfeToMarker := func() {
		hasMarker := false
		for _, e := range afe {
			if e.marker {
				hasMarker = true
			}
		}
		if !hasMarker {
			return
		}
		for len(afe) > 0 {
			last := afe[len(afe)-1]
			afe = afe[:len(afe)-1]
			if last.marker {
				return
			}
		}
	}
	// reconstruct re-opens, in order, every entry after the last marker that
	// is not on the stack. Called where x/net's parser calls its own — before
	// body text and before the start tags whose in-body rules reconstruct —
	// and nowhere else: a <div> deliberately does not, which is why a <b> can
	// stay closed across one until text re-opens it.
	reconstruct := func() {
		n := len(afe)
		if n == 0 || afe[n-1].marker || onStack(afe[n-1].frame.id) {
			return
		}
		i := n - 1
		for i > 0 && !afe[i-1].marker && !onStack(afe[i-1].frame.id) {
			i--
		}
		for ; i < n; i++ {
			clone := afe[i].frame
			nextID++
			clone.id = nextID
			stack = append(stack, clone)
			afe[i].frame = clone
		}
	}
	inScope := func(name string) bool {
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].ns == "" && stack[i].name == name {
				return true
			}
			if scopeBoundary(stack[i]) {
				return false
			}
		}
		return false
	}
	// adoptionAgency is the agency's stack-visible effect, shared by the
	// formatting end tags and by the <a>/<nobr> start tags that run the
	// agency against an already-open element of their own name. The element
	// it acts on comes from the active formatting LIST, not from the stack: a
	// listed element that is no longer open is simply delisted, and one out
	// of scope is left alone. For an open one, the agency's outer loop reruns
	// on the clone it inserts until nothing special is open above one, and
	// each pass reparents one furthest block — so the net the stack keeps is
	// exactly the SPECIAL frames that were open above the formatting element,
	// each of which was a furthest block once, and everything else above it
	// is closed: "<b><div><svg></b>" ends with the div open and the svg gone,
	// which is where html.Parse puts the next token. Without a special
	// element above it the agency pops normally. It reports false when the
	// list holds no such element at all, which for an end tag means the
	// any-other rule speaks instead.
	adoptionAgency := func(name string) bool {
		ai := afeIndex(name)
		if ai < 0 {
			return false
		}
		fe := afe[ai].frame
		si := -1
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].id == fe.id {
				si = i
				break
			}
		}
		if si < 0 {
			afe = append(afe[:ai], afe[ai+1:]...)
			return true
		}
		for i := len(stack) - 1; i > si; i-- {
			if scopeBoundary(stack[i]) {
				return true // out of scope: parse error, nothing moves
			}
		}
		if stackHasSpecial(stack[si+1:]) {
			kept := stack[:si]
			for _, f := range stack[si+1:] {
				if isSpecialElement(f) {
					kept = append(kept, f)
				}
			}
			stack = kept
		} else {
			stack = stack[:si]
		}
		afe = append(afe[:ai], afe[ai+1:]...)
		return true
	}
	// liClosure is the loop the <li>, <dd> and <dt> start tags run: close the
	// nearest open element of their kind, walking past address, div and p and
	// stopping at anything else special — then close a p like every other
	// block start.
	liClosure := func(a, b string) {
		for i := len(stack) - 1; i >= 0; i-- {
			f := stack[i]
			if f.ns == "" && (f.name == a || f.name == b) {
				stack = stack[:i]
				break
			}
			if f.name == "address" || f.name == "div" || f.name == "p" {
				continue
			}
			if isSpecialElement(f) {
				break
			}
		}
		closeP()
	}

	// inertRest reports that the parser this scan mirrors has stopped
	// building anything: an honored <frameset> replaced the body and every
	// insertion mode from there ignores body content, or x/net's parser met a
	// <template> start tag with foreign content open and — its own documented
	// workaround — ignores every remaining token. Everything after either is
	// text to nobody.
	inertRest := false
	// framesetOK mirrors the parser's frameset-ok flag: a <frameset> replaces
	// the body only before any body content has been seen, and is an ignored
	// token afterwards.
	framesetOK := true

	// pending describes the raw-text body the tokenizer is about to hand back
	// as a single text token, for the elements where that reading is also a
	// browser's: the HTML raw-text and RCDATA names. The foreign and
	// <noscript> cases never set it — their bodies are never handed back as
	// one token at all.
	var pending struct {
		// language is the HTML <script> or <style> whose body is a program AND
		// raw text. Only HTML has one of those: an SVG program body is markup
		// as well, and reaches the language through markupFrame.program.
		language string
		// inert marks a body that is raw text to a browser as well, so the
		// scan reads nothing out of it and says so.
		inert bool
		// noscriptRaw marks the inert body of a <noscript> under scripting:
		// dead in THIS reading, but the caller has another one to ask.
		noscriptRaw bool
	}
	forget := func() { pending.language, pending.inert, pending.noscriptRaw = "", false, false }

	// closeTag applies an end tag to the stack. HTML gives an end tag one of
	// four readings, and which one is decided by name and namespace, not by a
	// single search: the foreign-content walk, the adoption agency, a
	// pop-until-in-scope rule the name carries, or "any other end tag".
	// Conflating them was measurably wrong in both directions — a generic
	// search that matched a foreign <title> from an HTML current node closed
	// an element a browser keeps open, and a special-element stop applied to
	// "</li>" refused a pop whose real boundary list ("list item scope")
	// walks straight past a <div> and a foreign root.
	closeTag := func(name string) {
		forget()
		// Foreign content first, which is the spec's own order: with a
		// foreign current node the end tag walks the foreign prefix of the
		// stack and pops to a name match. Only when the walk reaches an HTML
		// element without matching does the tag become HTML's business.
		if enclosing().ns != "" {
			// "</br>" acts as a <br> start tag, and that is a breakout.
			if name == "br" {
				leaveForeignContent()
				return
			}
			for i := len(stack) - 1; i >= 0 && stack[i].ns != ""; i-- {
				if stack[i].name == name {
					stack = stack[:i]
					return
				}
			}
		}
		// "</form>" REMOVES the form element from the stack rather than popping
		// down to it, so an <svg> the author forgot to close inside a form
		// survives it and everything after is still SVG — where an <iframe> has
		// no browsing context and its srcdoc is inert. Popping to the form took
		// the <svg> with it and made that srcdoc read as a real one.
		if name == "form" {
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i].ns == "" && stack[i].name == "form" {
					stack = append(stack[:i], stack[i+1:]...)
					break
				}
			}
			return
		}
		// A formatting element's end tag is the adoption agency's business,
		// and the agency's one property this stack has to keep is that a
		// SPECIAL element open above the formatting element — the "furthest
		// block" — stays open: "<b><div></b>" removes the b and keeps the div,
		// so a later "</div>" still closes it, and the <svg> that was opened
		// inside it. Popping through the div instead closed it early, and the
		// "</div>" that then matched nothing left that <svg> open where a
		// browser had left foreign content. Without a special element above it
		// the agency pops normally, and that is the whole of what is modelled:
		// the clone the agency appends and the in-between frames it removes
		// are DOM surgery that never changes which elements stay open under
		// which namespace. (A foreign element spelled like a formatting
		// element — an SVG <font> — was already taken by the foreign walk
		// above; the agency's own search is HTML-only.)
		if formattingElements[name] {
			if adoptionAgency(name) {
				return
			}
			// The list holds no such element, and the agency's answer for
			// that is the any-other rule below.
		}
		// The names whose end tags pop until the element is popped, provided
		// it is "in scope" — and each name's scope has its own boundary list,
		// which is why they cannot ride the any-other walk below: "</li>"
		// walks past a <div> and a foreign root that would stop an unknown
		// name, and "</table>" stops at nearly nothing. A boundary met first
		// means the element is not in scope and the tag is ignored.
		if scopePopTags[name] || headingTags[name] {
			boundary := func(f markupFrame) bool {
				switch {
				case name == "table":
					// Table scope: html, table, template — and the nearest
					// table is the match itself, so only template stops.
					return f.ns == "" && f.name == "template"
				case name == "template":
					// "</template>" pops to the template unconditionally —
					// the parser checks only that one is open, and nothing
					// stops the walk.
					return false
				case name == "li":
					// List item scope adds the list containers.
					return scopeBoundary(f) || (f.ns == "" && (f.name == "ol" || f.name == "ul"))
				case name == "p":
					// Button scope adds <button>.
					return scopeBoundary(f) || (f.ns == "" && f.name == "button")
				default:
					return scopeBoundary(f)
				}
			}
			for i := len(stack) - 1; i >= 0; i-- {
				f := stack[i]
				if f.ns == "" && (f.name == name || (headingTags[name] && headingTags[f.name])) {
					stack = stack[:i]
					if afeMarkerTags[name] {
						clearAfeToMarker()
					}
					return
				}
				if boundary(f) {
					break // not in scope: the tag closes nothing
				}
			}
			// "</p>" with no p in scope — whether the walk met a boundary or
			// ran out — inserts a p start tag and closes it, and a <p> start
			// tag is a breakout when the current node is foreign. The
			// insertion itself leaves the stack as it was.
			if name == "p" {
				leaveForeignContent()
			}
			return
		}
		// Any other end tag: the walk matches an HTML element with the same
		// name and stops at the first SPECIAL element. A foreign frame can
		// never be matched here — it is not an HTML element however it is
		// spelled — but a special one (an integration point, say) still stops
		// the walk. "<svg><title><g></title>" is the shape both halves pin:
		// the <g> inside the integration point is HTML, the SVG <title> is
		// special, and a browser ignores the tag and keeps all three open.
		for i := len(stack) - 1; i >= 0; i-- {
			if stack[i].ns == "" && stack[i].name == name {
				stack = stack[:i]
				if afeMarkerTags[name] {
					clearAfeToMarker()
				}
				return
			}
			if isSpecialElement(stack[i]) {
				return
			}
		}
	}
	for {
		// A "<![CDATA[" is a real CDATA section only when the current node is
		// foreign — in HTML it is a bogus comment — and the tokenizer cannot
		// know which, so it is told before every token. enclosing() is the
		// spec's adjusted current node, and the element's OWN namespace is
		// what answers: a CDATA section inside an SVG <title> — an HTML
		// integration point, but an SVG element — is character data, exactly
		// as html.Parse reads it.
		z.AllowCDATA(enclosing().ns != "")
		tt := z.Next()
		if inertRest && tt != html.ErrorToken {
			// The parser has stopped building; every remaining sentinel is in
			// text nobody renders, and saying so keeps the fallbacks quiet.
			for _, idx := range sentinelIndexes(string(z.Raw())) {
				record(idx.index, attrContext{inertText: true, noscript: inNoscript()})
			}
			continue
		}
		switch tt {
		case html.ErrorToken:
			return
		case html.StartTagToken, html.SelfClosingTagToken:
			raw := string(z.Raw())
			nameBytes, hasAttr := z.TagName()
			name := string(nameBytes)
			// Read from the tag's own source rather than through z.TagAttr,
			// which replaces NUL with U+FFFD and so erases the sentinel.
			hits := sentinelsInTag(raw)
			interpolatedEncoding := false
			for _, hit := range hits {
				if hit.ctx.inValue && hit.ctx.attr == "encoding" && encodingCouldBeHTML(hit.value) {
					interpolatedEncoding = true
				}
			}

			ns := enclosing().namespaceForChild(name)
			if ns != "" && breaksOutOfForeignContent(name, z, hasAttr) {
				leaveForeignContent()
				ns = enclosing().namespaceForChild(name)
			}
			enteredForeignFromHTML := false
			if ns == "" && (name == "svg" || name == "math") {
				ns = name
				enteredForeignFromHTML = true
			}
			// A start tag that is not head content closes an open <head> —
			// the in-head mode hands anything else to the body — and that
			// includes an <svg> or <math>, which is why this sits outside the
			// HTML-only corrections below.
			if t := enclosing(); t.ns == "" && t.name == "head" && !headContentTags[name] {
				stack = stack[:len(stack)-1]
			}
			if ns == "" {
				// The tree constructor's in-body corrections, which the
				// tokenizer knows nothing about: tokens the parser renames,
				// ignores outright, or answers by first closing an element
				// that is still open. A scan without these carries frames the
				// parser never kept, and a stray end tag later pops through
				// them into a different namespace than the browser's.
				if name == "image" {
					// The parser rewrites the token itself; <image> IS <img>.
					name = "img"
				}
				ignored := false
				switch {
				case name == "frameset":
					// Honored only before body content, where it replaces the
					// body outright; afterwards it is an ignored token.
					if framesetOK {
						inertRest = true
					}
					ignored = true
				case name == "head":
					// A real element only at the very start of the document.
					ignored = len(stack) > 0 || !framesetOK
				case name == "frame":
					// Real only inside a frameset document, whose whole body
					// inertRest already covers.
					ignored = true
				case tableSectionTags[name]:
					// In body these are ignored; only the table insertion
					// modes build them, approximated by an open <table>.
					ignored = !htmlFrameOpen("table")
				case name == "form":
					// A nested form is an ignored token while one is open.
					ignored = htmlFrameOpen("form")
				case name == "select":
					// A <select> with one already in scope pops to it and
					// inserts nothing — the parser's reading of nesting one.
					for i := len(stack) - 1; i >= 0; i-- {
						if stack[i].ns == "" && stack[i].name == "select" {
							stack = stack[:i]
							ignored = true
							break
						}
						if scopeBoundary(stack[i]) {
							break
						}
					}
				case name == "template":
					// x/net's parser refuses to mix templates with open
					// foreign content and ignores every remaining token — its
					// own documented workaround. The oracle is that parser,
					// so the scan stops where it stops.
					for _, f := range stack {
						if f.ns != "" {
							inertRest = true
							ignored = true
							break
						}
					}
				}
				if ignored {
					// The element never exists, so its attributes never do
					// either; recorded as settled so the fallbacks stay out.
					for _, hit := range hits {
						record(hit.index, attrContext{inertText: true, noscript: inNoscript()})
					}
					continue
				}
				if framesetFlipTags[name] {
					framesetOK = false
				} else if name == "input" {
					// An <input> counts as body content unless it is hidden.
					hidden := false
					for hasAttr {
						var key, val []byte
						key, val, hasAttr = z.TagAttr()
						if string(key) == "type" && asciiEqualFold(string(val), "hidden") {
							hidden = true
						}
					}
					if !hidden {
						framesetOK = false
					}
				}
				// Start tags that close an element before inserting
				// themselves.
				switch {
				case pCloserTags[name]:
					closeP()
					if headingTags[name] {
						// A heading start tag also pops a heading it is
						// directly inside.
						if t := enclosing(); t.ns == "" && headingTags[t.name] {
							stack = stack[:len(stack)-1]
						}
					}
					if name == "xmp" {
						// The one raw-text element whose in-body rule
						// reconstructs before it goes raw.
						reconstruct()
					}
				case name == "li":
					liClosure("li", "li")
				case name == "dd", name == "dt":
					liClosure("dd", "dt")
				case name == "button":
					// A button in scope is closed first.
					for i := len(stack) - 1; i >= 0; i-- {
						if stack[i].ns == "" && stack[i].name == "button" {
							stack = stack[:i]
							break
						}
						if scopeBoundary(stack[i]) {
							break
						}
					}
					reconstruct()
				case name == "a":
					// An <a> still in the list means the agency runs before
					// the new one is inserted — the list, not the stack, is
					// what the rule consults.
					if afeIndex("a") >= 0 {
						adoptionAgency("a")
					}
					reconstruct()
				case name == "nobr":
					reconstruct()
					if inScope("nobr") {
						adoptionAgency("nobr")
						reconstruct()
					}
				case formattingElements[name]:
					reconstruct()
				case name == "option", name == "optgroup":
					if t := enclosing(); t.ns == "" && t.name == "option" {
						stack = stack[:len(stack)-1]
					}
					reconstruct()
				case name == "applet", name == "marquee", name == "object", name == "select",
					name == "area", name == "br", name == "embed", name == "img",
					name == "keygen", name == "wbr", name == "input":
					reconstruct()
				default:
					// Any other start tag reconstructs before inserting —
					// except the raw-text names, whose in-body rules go
					// straight to the tokenizer.
					if !rawTextElements[name] {
						reconstruct()
					}
				}
			}
			if enteredForeignFromHTML {
				// An <svg> or <math> root is inserted by the in-body mode,
				// which reconstructs first like any other phrasing content.
				reconstruct()
			}
			// The namespace is settled before this tag's own attributes are
			// recorded, because it decides what some of them mean: a srcdoc= on
			// a foreign element is an attribute of an element with no browsing
			// context, not a document waiting to be parsed.
			noscript := inNoscript()
			for _, hit := range hits {
				ctx := hit.ctx
				ctx.noscript = noscript
				ctx.foreignRoot = ns
				ctx.element = name
				record(hit.index, ctx)
			}

			frame := markupFrame{
				name:            name,
				ns:              ns,
				htmlIntegration: isHTMLIntegrationPoint(ns, name, z, hasAttr, interpolatedEncoding),
				mathText:        ns == "math" && mathTextIntegrationPoints[name],
			}
			forget()
			if rawTextElements[name] {
				switch {
				case ns != "":
					// A foreign element is not the HTML element its name
					// spells, and a browser does not raw-text it. This is the
					// parser's own correction, and it is needed for a
					// self-closing tag too: the tokenizer arms its raw-text
					// state before it looks for the "/".
					z.NextIsNotRawText()
					// SVG is the one foreign namespace where <script> executes
					// and <style> applies — the parser spec special-cases the
					// SVG <script> end tag and has nothing of the sort for
					// MathML. The body is still markup, so the tags in it are
					// read like any others; only what lands in this element's
					// DIRECT child text is program source, which the text case
					// reads off this frame. An unsupported type= means the
					// browser applies nothing, and the body is markup and only
					// markup.
					if ns == "svg" && name == "script" {
						frame.program = name
					} else if ns == "svg" && name == "style" && styleTypeApplies(z, hasAttr, hits) {
						frame.program = name
					}
				case scriptLikeElements[name] && (name != "style" || styleTypeApplies(z, hasAttr, hits)):
					// HTML: the tokenizer's reading is a browser's, and the
					// body arrives as one text token of program source.
					pending.language = name
				case name == "noscript":
					// Markup with scripting off, raw text with scripting on,
					// and this scan reads it in its own mode. Off: the frame
					// marks everything inside as reachable only in that mode.
					// On: the body is raw text ending at the first
					// "</noscript>", and each sentinel in it is marked for
					// the caller to ask the other reading about.
					if scripting {
						pending.inert = true
						pending.noscriptRaw = true
					} else {
						z.NextIsNotRawText()
						frame.noscript = true
					}
				default:
					// The other seven, plus a <style> a browser will not
					// apply: raw text or RCDATA to the tokenizer and to a
					// browser alike, so a tag written in one is text.
					pending.inert = true
				}
			}
			// A void element is popped the instant it is inserted, and a "/"
			// only really closes a tag in foreign content — HTML ignores it, so
			// "<div/>" opens a div. Getting either wrong leaves a frame on the
			// stack that a browser does not have, and the namespace of
			// everything after it is then read off the wrong element.
			if !(voidElements[name] && ns == "") && !(tt == html.SelfClosingTagToken && ns != "") {
				nextID++
				frame.id = nextID
				stack = append(stack, frame)
				if ns == "" {
					if formattingElements[name] {
						afe = append(afe, afeEntry{frame: frame})
					}
					if afeMarkerTags[name] {
						afe = append(afe, afeEntry{marker: true})
					}
				}
			}
		case html.EndTagToken:
			nameBytes, _ := z.TagName()
			closeTag(string(nameBytes))
		case html.TextToken:
			// Body text is body content: any non-whitespace character outside
			// a raw-text body means a later <frameset> is an ignored token.
			if framesetOK && pending.language == "" && !pending.inert &&
				strings.TrimSpace(string(z.Raw())) != "" {
				framesetOK = false
			}
			// Body text also reconstructs the active formatting elements
			// before it is inserted, whitespace included.
			if enclosing().ns == "" && pending.language == "" && !pending.inert {
				reconstruct()
			}
			// A text token whose raw begins with the CDATA opener is a section
			// the tokenizer was allowed to read — a raw-text body that merely
			// starts with those bytes is caught by its pending case first.
			isCDATA := strings.HasPrefix(string(z.Raw()), "<![CDATA[")
			switch {
			case pending.inert:
				// An HTML raw-text or RCDATA body the scan passed over. Recorded
				// rather than left blank so the "<" proof below does not read
				// "<textarea><[meta …]" as an interpolated element name: there
				// is no tag there, and a browser shows the "<" as text.
				for _, idx := range sentinelIndexes(string(z.Raw())) {
					record(idx.index, attrContext{
						inertText:   true,
						noscript:    inNoscript(),
						noscriptRaw: pending.noscriptRaw,
					})
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
						noscript:       inNoscript(),
					})
				}
			case enclosing().program != "":
				// A DIRECT child text node of an SVG <script> or <style> is
				// the program. Not every descendant one: the spec builds the
				// source from "child text content", which is the Text children
				// and not textContent, so a value written inside a <g> in
				// there is markup that no browser executes — and this case
				// never fires there, because the <g>'s own frame is what
				// enclosing() reads.
				top := enclosing()
				for _, idx := range sentinelIndexes(string(z.Raw())) {
					fr := top.ns
					if isCDATA {
						// Entities are NOT decoded inside a CDATA section —
						// that is what Illustrator wraps a style body in one
						// for — so the escaping arrives intact and the HTML
						// message is the true one, not the foreign message
						// that says the parser undoes it.
						fr = ""
					}
					record(idx.index, attrContext{
						rawTextElement: top.program,
						foreignRoot:    fr,
						noscript:       inNoscript(),
					})
				}
			case isCDATA:
				// Character data in foreign content, outside any program.
				// Recorded as settled text so the fallbacks do not answer for
				// it: an unpaired quote in it is not an open attribute, and a
				// "<" in it starts nothing.
				for _, idx := range sentinelIndexes(string(z.Raw())) {
					record(idx.index, attrContext{inertText: true, noscript: inNoscript()})
				}
			}
		}
	}
}

// formattingElements are the names whose end tags the adoption agency
// algorithm owns — the spec's list of elements eligible for the active
// formatting list.
var formattingElements = map[string]bool{
	"a": true, "b": true, "big": true, "code": true, "em": true, "font": true,
	"i": true, "nobr": true, "s": true, "small": true, "strike": true,
	"strong": true, "tt": true, "u": true,
}

// afeMarkerTags insert a marker into the active formatting list when they
// open, and clear the list back to one when they close: reconstruction never
// reaches past the boundary they draw.
var afeMarkerTags = map[string]bool{
	"applet": true, "caption": true, "marquee": true, "object": true,
	"td": true, "template": true, "th": true,
}

// scopePopTags are the end-tag names the in-body insertion mode gives their
// own pop-until-popped rule, each guarded by a scope test. The table-section
// names (td, tr, tbody and friends) are deliberately absent: their rules live
// in the table insertion modes this scan does not model.
var scopePopTags = map[string]bool{
	"address": true, "applet": true, "article": true, "aside": true,
	"blockquote": true, "button": true, "center": true, "dd": true,
	"details": true, "dialog": true, "dir": true, "div": true, "dl": true,
	"dt": true, "fieldset": true, "figcaption": true, "figure": true,
	"footer": true, "header": true, "hgroup": true, "li": true,
	"listing": true, "main": true, "marquee": true, "menu": true, "nav": true,
	"object": true, "ol": true, "p": true, "pre": true, "section": true,
	"summary": true, "table": true, "template": true, "ul": true,
}

// headingTags close as a class: "</h2>" pops to the nearest open heading of
// any rank.
var headingTags = map[string]bool{
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
}

// headContentTags are the start tags the in-head mode keeps for itself; any
// other start tag closes an open <head> and belongs to the body.
var headContentTags = map[string]bool{
	"base": true, "basefont": true, "bgsound": true, "head": true,
	"link": true, "meta": true, "noframes": true, "noscript": true,
	"script": true, "style": true, "template": true, "title": true,
}

// framesetFlipTags are the start tags that set the frameset-ok flag to "not
// ok" — the parser's enumerated list, not a complement: a formatting element,
// an unknown element and an <svg> all leave the flag alone, and a <frameset>
// after any of them still replaces the body.
var framesetFlipTags = map[string]bool{
	"applet": true, "area": true, "body": true, "br": true, "button": true,
	"dd": true, "dt": true, "embed": true, "hr": true, "iframe": true,
	"img": true, "keygen": true, "li": true, "listing": true,
	"marquee": true, "object": true, "pre": true, "select": true,
	"table": true, "template": true, "textarea": true, "wbr": true,
	"xmp": true,
}

// tableSectionTags are ignored tokens in the body; only the table insertion
// modes build them.
var tableSectionTags = map[string]bool{
	"caption": true, "col": true, "colgroup": true, "tbody": true, "td": true,
	"tfoot": true, "th": true, "thead": true, "tr": true,
}

// pCloserTags are the block-level start tags that close a p element in button
// scope before inserting themselves. The headings are here too, with their
// extra rule applied at the call site.
var pCloserTags = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true,
	"center": true, "details": true, "dialog": true, "dir": true, "div": true,
	"dl": true, "fieldset": true, "figcaption": true, "figure": true,
	"footer": true, "form": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "header": true, "hgroup": true,
	"hr": true, "listing": true, "main": true, "menu": true, "nav": true,
	"ol": true, "p": true, "plaintext": true, "pre": true, "section": true,
	"summary": true, "table": true, "ul": true, "xmp": true,
}

// scopeBoundary reports whether HTML's "has an element in scope" walk stops at
// this element — a different and shorter list than the special one. Copied
// from the spec's particular-element-scope list, with its MathML and SVG
// entries.
func scopeBoundary(f markupFrame) bool {
	switch f.ns {
	case "":
		switch f.name {
		case "applet", "caption", "html", "table", "td", "th", "marquee",
			"object", "template":
			return true
		}
	case "math":
		switch f.name {
		case "mi", "mo", "mn", "ms", "mtext", "annotation-xml":
			return true
		}
	case "svg":
		switch f.name {
		case "foreignobject", "desc", "title":
			return true
		}
	}
	return false
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
func isHTMLIntegrationPoint(ns, name string, z *html.Tokenizer, hasAttr, interpolatedEncoding bool) bool {
	switch ns {
	case "svg":
		return name == "desc" || name == "foreignobject" || name == "title"
	case "math":
		if name != "annotation-xml" {
			return false
		}
		// An encoding an interpolation could complete decides the namespace of
		// everything inside, from a value a lower-privileged user writes, so it
		// reads as the HTML integration point it may become — the same
		// fail-closed choice the unterminated-tag case makes. "Could complete"
		// and not "contains one": see encodingCouldBeHTML.
		if interpolatedEncoding {
			return true
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
			// Not trimmed: the match is ASCII case-insensitive and nothing more,
			// so encoding=" text/html " is NOT an integration point.
			for _, enc := range htmlAnnotationEncodings {
				if asciiEqualFold(string(val), enc) {
					return true
				}
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

// styleTypeApplies reports whether a browser will treat this <style> element as
// a stylesheet. The type must be absent, empty, or text/css; anything else and
// nothing is applied, so a value in the body is inert text.
//
// <script> deliberately gets no equivalent, and the asymmetry is the two specs'
// rather than this file's: a style has exactly one valid type, while deciding
// whether a script runs means classifying the JavaScript MIME types, "module",
// and the data types (importmap, speculationrules) that are neither. That one
// is named as residue instead.
func styleTypeApplies(z *html.Tokenizer, hasAttr bool, hits []sentinelHit) bool {
	// An interpolated type= is asked what it could complete, exactly as an
	// interpolated <annotation-xml> encoding is: type="text/plain[meta …]"
	// cannot become text/css, so nothing is applied and the body is inert.
	for _, hit := range hits {
		if hit.ctx.inValue && hit.ctx.attr == "type" {
			return interpolatedValueCouldBe(hit.value, styleSheetTypes)
		}
	}
	for hasAttr {
		var key, val []byte
		key, val, hasAttr = z.TagAttr()
		if string(key) != "type" {
			continue
		}
		// Not trimmed: the match is "the empty string, or an ASCII
		// case-insensitive match for text/css", so type=" text/css " applies
		// nothing at all.
		for _, want := range styleSheetTypes {
			if asciiEqualFold(string(val), want) {
				return true
			}
		}
		return false
	}
	return true
}

// stackHasSpecial reports whether HTML's walk would stop somewhere in this
// part of the stack rather than reaching past it. The adoption agency clause
// asks it to decide whether a formatting element has a furthest block above
// it, and the end-tag search asks it before leaving foreign content from an
// HTML current node.
//
// Asked only when the current node is HTML, because the stop is HTML's rule.
// The foreign rule has none: in "<svg><iframe><foreignObject><math></svg>" a
// browser walks past the special <foreignObject>, finds the svg and leaves —
// and refusing to leave there left a real HTML <iframe srcdoc> unwarned.
func stackHasSpecial(stack []markupFrame) bool {
	for _, f := range stack {
		if isSpecialElement(f) {
			return true
		}
	}
	return false
}

// isSpecialElement reports whether HTML's "any other end tag" walk stops at this
// element. Copied from x/net/html's own isSpecialElementMap and the namespace
// switch beside it.
func isSpecialElement(f markupFrame) bool {
	switch f.ns {
	case "":
		return htmlSpecialElements[f.name]
	case "math":
		switch f.name {
		case "mi", "mo", "mn", "ms", "mtext", "annotation-xml":
			return true
		}
	case "svg":
		switch f.name {
		case "foreignobject", "desc", "title":
			return true
		}
	}
	return false
}

var htmlSpecialElements = map[string]bool{
	"address": true, "applet": true, "area": true, "article": true,
	"aside": true, "base": true, "basefont": true, "bgsound": true,
	"blockquote": true, "body": true, "br": true, "button": true,
	"caption": true, "center": true, "col": true, "colgroup": true,
	"dd": true, "details": true, "dir": true, "div": true, "dl": true,
	"dt": true, "embed": true, "fieldset": true, "figcaption": true,
	"figure": true, "footer": true, "form": true, "frame": true,
	"frameset": true, "h1": true, "h2": true, "h3": true, "h4": true,
	"h5": true, "h6": true, "head": true, "header": true, "hgroup": true,
	"hr": true, "html": true, "iframe": true, "img": true, "input": true,
	"keygen": true, "li": true, "link": true, "listing": true, "main": true,
	"marquee": true, "menu": true, "meta": true, "nav": true, "noembed": true,
	"noframes": true, "noscript": true, "object": true, "ol": true, "p": true,
	"param": true, "plaintext": true, "pre": true, "script": true,
	"section": true, "select": true, "source": true, "style": true,
	"summary": true, "table": true, "tbody": true, "td": true,
	"template": true, "textarea": true, "tfoot": true, "th": true,
	"thead": true, "title": true, "tr": true, "track": true, "ul": true,
	"wbr": true, "xmp": true,
}

// voidElements have no end tag and no children, so a browser pops one as soon
// as it inserts it. Keeping a frame for one would put the namespace of its
// following siblings on the wrong element.
//
// The list is HTML's, and so is the rule: an <svg><source> is an ordinary
// foreign element that stays open, and dropping its frame made the text under
// it read as a direct child of whatever was above.
var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
	// Not HTML's official void list but the parser's behaviour: it inserts
	// and immediately pops all three, so no end tag ever finds one open.
	// (<image> never reaches this map at all — the parser renames the token
	// to img.)
	"basefont": true, "bgsound": true, "keygen": true,
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
	// value is the whole attribute value the sentinel sits in, as written. It
	// is a slice of the tag's own source, so keeping it copies nothing, and one
	// rule needs the text on BOTH sides of the interpolation rather than only
	// the prefix ctx.valueSoFar carries.
	value string
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
			ctx := attrContext{attr: name, quoted: q != 0, inValue: !duplicate}
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

// encodingCouldBeHTML reports whether an interpolated <annotation-xml>
// encoding= could come out as one of those two, which is the question the
// namespace of everything inside the element turns on.
//
// The shape is couldStillBecomeExecutable's: what is fixed is every run of text
// an interpolation cannot change, so encoding="text/[meta …]" could and
// encoding="image/[meta …]" could not. The runs BETWEEN two interpolations are
// fixed as well and are checked, so encoding="[meta …]zzz[meta …]" could not
// either. Asking only whether the value holds an interpolation at all failed
// closed on all three, which is a warning about a script no browser runs.
func encodingCouldBeHTML(value string) bool {
	return interpolatedValueCouldBe(value, htmlAnnotationEncodings)
}

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
		at = h.at + len(lintSentinelPrefix)
		if i := strings.IndexByte(value[at:], 0); i >= 0 {
			at += i + 1
		} else {
			at = len(value)
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
// them is emitted as a tag. Two of the ten are here, and <noscript> is re-read
// (below). The remaining SEVEN are read as ordinary prose while they are in the
// HTML namespace, which is a decision rather than an oversight: it has been got
// wrong twice, once by warning on all eight of the non-program bodies and once
// by re-reading a <noscript> body as markup and reporting placements in it that
// cannot execute under either reading.
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
// <noscript> body and foreign content — are read in place by scanMarkup, which
// clears the tokenizer's raw-text state for them, rather than being listed
// here.
//
// NOSCRIPT is raw text to the tokenizer whatever the scripting flag says, and
// markup to a browser only when scripting is disabled — which is the mode the
// element exists for, so the placements live in that mode are worth reporting.
// The set that is reported is exactly the one that needs no script: srcdoc, an
// unquoted attribute, style=, a <style> ELEMENT a browser would apply, an
// interpolated attribute name, and raw=. Every
// script-execution rule (an on* handler, an Alpine directive, a javascript: URL,
// a <script> body) is withheld, because it is inapplicable under BOTH readings —
// with scripting on the body is inert raw text, with it off the handler is real
// and does nothing — and reporting it is the false positive that had the earlier
// attempt withdrawn. Every message from a body carries noscriptQualifier, which
// names the mode it applies in. An unclosed <noscript> is the same case at its
// widest and is read the same way, since with scripting disabled everything
// after it is live markup.
//
// A <style> element and a <script> element are the pair that shows where the
// dividing line runs. Both are real elements with scripting disabled; only one
// of them does anything. The stylesheet applies, and a value in it can close
// the declaration and open another — the same hazard style= carries, and it
// needs no script; the script is inert. So the language rule reports the first
// and withholds the second, rather than treating "a program body" as one case.
// A <style> whose type= no browser supports is not a stylesheet either, and
// styleTypeApplies is where that is asked, everywhere rather than only here.
//
// One member of the non-execution set stays unreported and is residue: every
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
//     opposite. The body is not raw text either, so it is read as markup like
//     every other foreign body, and what is program source is its CHILD TEXT
//     CONTENT — the direct Text children a script's source is built from, not
//     textContent. An attribute on an element inside one is an attribute of a
//     real element, and text inside that element is markup nothing runs.
//   - MathML has neither, so a <math><script> runs nothing and a <math><style>
//     applies nothing. Both are read as the ordinary markup they are.
//   - The remaining names are inert foreign elements whose children are real,
//     so their bodies are read as the markup they are. <svg><iframe><a
//     href="javascript:…"> is a live SVG link and was silent; <svg><title> is
//     an HTML integration point, so the anchor in it is a real HTML anchor.
//
// Foreign content is also where "<![CDATA[" opens a real CDATA section — the
// wrapper Illustrator and Inkscape put around a style body — whose content is
// character data with no entity decoding. The tokenizer is told per token via
// AllowCDATA, from the namespace this scan tracks anyway. Both directions were
// defects first: a bogus-comment reading of the section ended at its first ">"
// and read the rest as markup, so character data holding "<div>" broke out of
// SVG and a foreign iframe after it warned as an HTML one; and a CDATA-wrapped
// SVG program body never reached the language rules at all. A program value
// inside one takes the HTML message rather than the foreign one, because the
// undoing of the escaping is exactly what a CDATA section does not do.
//
// An element being foreign also narrows the rules that describe what one
// particular HTML element DOES, since a foreign element is not that element
// however it is spelled: a foreign <iframe> has no browsing context, so its
// srcdoc is never parsed as a document. What is untouched is everything about
// markup syntax (raw=, an unquoted value, an interpolated name), the two
// attributes every namespace honours (style=, on*), and the URL rules, which
// stay element-blind on purpose (unsafeAttributeContexts says why).
//
// Three things keep this from becoming a third false-positive machine, and each
// was a false positive first. breaksOutOfForeignContent: ~40 HTML tag names end
// foreign content (the list copied from x/net's own breakout map, plus the
// "</br>"/"</p>" end tags), so an <svg> the author forgot to close does not turn
// every later <textarea> into a live one. markupFrame.namespaceForChild and
// the end-tag search: x/net's inForeignContent and parseForeignContent written
// against one frame, so <svg><title>, the
// MathML text points and their <mglyph>/<malignmark> exceptions, and an <svg>
// inside <annotation-xml> all land where a browser puts them. And an end tag
// matching nothing open is ignored, as both HTML's rule and the foreign one
// ignore it, rather than being read as an exit from anything.
//
// One thing predates all of this and is unchanged by it: whether a <script>
// body is a program at all is decided by the element's name, never by its
// type=. A <script type="application/json"> holds data rather than code, so
// naming JavaScript there overstates it — though an escaped value in one can
// neither close the element nor be decoded back into a quote, so what it
// overstates is the verdict as well: nothing there reaches JavaScript, and a
// <script type="application/ld+json"> holding an escaped value is a warning
// about a program that does not exist. What the escaping does still hold is
// real — no "<" and no decoded quote, so the element cannot be closed — which
// is why this is a wrong reason attached to a wrong verdict rather than a
// missed hazard. <style> DID get the
// equivalent check (styleTypeApplies), and the asymmetry is the two specs'
// rather than this file's: a style has exactly one valid type, while deciding
// whether a script runs means classifying the JavaScript MIME types, "module",
// and the data types (importmap, speculationrules) that are neither.
//
// What the stack still does not model is HTML's own tree construction at its
// deepest, and no claim is made here about which direction any of it fails in.
// The adoption agency is kept to its stack-visible net — the special frames
// above the formatting element survive and everything else above it closes —
// and the active formatting list exists (a scope pop does not delist a
// formatting element, and the next body insertion reconstructs it), but
// without the spec's Noah's Ark clause, so more than three identical entries
// can accumulate where a browser would cap them. The end tags with their own
// pop-until-in-scope rules carry their own boundary lists in closeTag, but
// only as the in-body insertion mode gives them: the table insertion modes do
// not exist here, so the table-section end tags (td, tr, tbody and friends)
// ride the any-other walk, foster parenting never happens, and the in-body
// corrections in scanMarkup honor the table-section START tags whenever a
// <table> is open rather than only in the modes that really do.
//
// TestLintPlacementsAgainstTheParser is what holds all of it: it asks
// x/net/html's parser, over a few thousand generated nests, whether the element
// that would make each placement dangerous exists at all — and its tag-soup
// sibling, TestLintDifferentialTagSoup, asks the same of markup nobody wrote
// on purpose.
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
