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
	z := html.NewTokenizer(strings.NewReader(substituted))
	var openRawText string
tokenize:
	for {
		switch z.Next() {
		case html.ErrorToken:
			break tokenize
		case html.StartTagToken, html.SelfClosingTagToken:
			for _, hit := range sentinelsInTag(string(z.Raw())) {
				if hit.index >= 0 && hit.index < len(spans) {
					out[spans[hit.index].start] = hit.ctx
				}
			}
			name, _ := z.TagName()
			openRawText = ""
			if scriptLikeElements[string(name)] {
				openRawText = string(name)
			}
		case html.EndTagToken:
			openRawText = ""
		case html.TextToken:
			// A script or style body is where escaping helps least, not most:
			// the parser decodes no entities there, so the value lands in
			// JavaScript or CSS with its escaping still in it — contained
			// against a quote or a "<", and untouched in every character that
			// matters to a language.
			if openRawText != "" {
				for _, idx := range sentinelIndexes(string(z.Raw())) {
					if idx.index >= 0 && idx.index < len(spans) {
						out[spans[idx.index].start] = attrContext{rawTextElement: openRawText}
					}
				}
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
	// unless an unterminated tag is the better answer.
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
func unsafeAttributeContexts(ctx attrContext, raw, cssMode bool, label string) []string {
	if cssMode && !ctx.inValue && !ctx.inName {
		// A CustomCSS slot is a stylesheet with no <style> wrapper of its own,
		// so nothing in the markup says so — the editor has to.
		if raw {
			return []string{label + ` with raw= in a CSS slot is not escaped and lands in the stylesheet verbatim. Drop raw= here.`}
		}
		return []string{label + ` is in a CSS slot, where the value lands in a stylesheet verbatim: a ";" or "}" in it starts new declarations, and escaping touches neither.`}
	}
	if ctx.unterminated {
		return []string{label + ` is inside a tag that is never closed, so where it lands cannot be determined. Close the tag; until then treat the value as unsafe.`}
	}
	if ctx.rawTextElement != "" {
		// Only script and style ever set this, so the language is never empty.
		return []string{label + ` sits inside a <` + ctx.rawTextElement + `> body, which the browser does not decode entities in, so the value reaches ` + scriptLikeLanguage[ctx.rawTextElement] + ` with its escaping still in it rather than as the text you wrote. Escaping does not make the placement safe either — a "${...}" in a template literal or a ";" in a declaration contains nothing it touches. Pass the value in through a data- attribute instead.`}
	}
	if ctx.inName {
		return []string{label + ` is interpolated into a tag or attribute NAME, which nothing delimits: a value containing a space or "=" simply adds attributes of its own, and escaping does not touch either character. Build the name in the template instead.`}
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
		out = append(out, label+` sits in an unquoted attribute value, where escaping does not stop a value containing a space from adding attributes of its own. Quote the attribute.`)
	}
	if cssMode {
		out = append(out, label+` is in a CSS slot, where the value lands in a stylesheet verbatim: a ";" or "}" in it starts new declarations, and escaping touches neither.`)
	}
	if kind := expressionAttributeKind(attr); kind != "" {
		out = append(out, label+` sits in the "`+attr+`" `+kind+`, whose value is evaluated as script after the HTML parser has undone the escaping, so a value containing a quote can execute. Do not interpolate Meta into it.`)
	}
	if attr == "style" {
		out = append(out, label+` sits in a "style" attribute, where escaping does not prevent CSS injection.`)
	}
	if alwaysUnsafeAttrs[attr] {
		out = append(out, label+` sits in a "`+attr+`" attribute, whose value the browser decodes and parses as HTML, so escaping does not prevent script injection anywhere in it.`)
	}
	if urlBearingAttrs[attr] {
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

// scriptLikeElements have bodies in which the parser decodes no entities, so a
// value placed there arrives with its escaping still in it and escaping is not
// what decides whether the placement is safe. It is not nothing — an escaped
// value carries no "<" and no bare quote, so it can neither close the element
// nor end a JavaScript string — but a backtick, a "${...}" or a ";" is not
// escaped at all, and those are the characters that matter in a language.
//
// The tokenizer raw-texts ten elements, and no tag inside any of them is ever
// emitted as a tag, so a value in one is analysed as ordinary prose. That reads
// like a hole and mostly is not; the argument is written out because it has
// been got wrong twice, once by warning on all eight of the other bodies and
// once by re-reading a <noscript> body as markup. All of it is about the HTML
// namespace — see the last paragraph.
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
// noscript is the only one where the tokenizer's reading and a browser's can
// differ — its body is raw text when scripting is enabled and ordinary markup
// when it is not — and it is still not here, because the difference does not
// reach this rule. With scripting on the body is inert raw text; with scripting
// off the tags are real but no script in them can execute, so every rule this
// file exists for (an on* handler, an Alpine directive, a javascript: URL, a
// <script> body) is inapplicable to it. What still applies in that mode is
// everything that is not script — CSS, links, forms — which is the residue
// listed further down, not an exception to this. Warning on the body as a whole is a
// false positive on its ordinary use, and reading the body as markup — which
// was tried — reports those same executable placements as dangerous when they
// cannot execute in either reading.
//
// An UNCLOSED one of the other seven swallows the rest of the document rather
// than a body, and that is not a fail-open either: a browser's tokenizer runs
// to EOF in raw text (or RCDATA, or PLAINTEXT) too, so the href further down
// that this file then says nothing about is not a link there either.
//
// An unclosed <noscript> is not that, and it is the widest form of the residue
// rather than a case of the rule above: with scripting disabled it is an
// ordinary open element, so everything after it is live markup that this file
// reads as one raw-text run and says nothing about at all.
//
// The residue, known and accepted: with scripting disabled, a value inside a
// <noscript> — or after an unclosed one — can land in an unquoted attribute, a
// style= attribute, an interpolated attribute name, a <style> element, any
// URL-bearing attribute (a stylesheet href, a <base href> that re-points every
// relative URL on the page, a form action), or a srcdoc=, whose value is parsed
// as a whole document so even an escaped value becomes real markup inside the
// frame. None of them execute script in that mode and all of them are
// unwarned. Buying them means carrying a scripting-mode axis through every rule
// below, for an element that has to be nested inside a category template first.
// A raw= value in a <noscript> body already warns, through the raw= rule, which
// is the one that matters most.
//
// FOREIGN CONTENT is a wider and older gap that none of this closes and none of
// it depends on. The tokenizer is namespace-unaware, so it raw-texts these
// names inside <svg> and <math> as well, where a browser does not: inside <svg>
// a <script> really is a script element whose text IS entity-decoded, and
// <svg><iframe><a href="javascript:…"> is a live SVG link. Both are silent
// here, as they were before any of this. Closing it means tracking foreign
// content and its integration points, which is the parser's job rather than the
// tokenizer's.
var scriptLikeElements = map[string]bool{"script": true, "style": true}

var scriptLikeLanguage = map[string]string{"script": "JavaScript", "style": "CSS"}

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
