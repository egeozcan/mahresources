package shortcodes

import (
	"sort"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Where an inline [meta] lands in the markup around it, which decides whether
// escaping is enough. Finding that out means answering "which attribute of
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

// inlineMetaSpan is one [meta ... inline] occurrence in the template source.
type inlineMetaSpan struct{ start, end int }

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
func attributeContextsFor(input string, spans []inlineMetaSpan) map[int]attrContext {
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
	last := 0
	for i, sp := range spans {
		if sp.start < last || sp.end > len(input) || sp.start > sp.end {
			out[sp.start] = attrContext{}
			continue
		}
		b.WriteString(input[last:sp.start])
		b.WriteString(lintSentinel(i))
		last = sp.end
		out[sp.start] = attrContext{}
	}
	b.WriteString(input[last:])

	substituted := b.String()
	z := html.NewTokenizer(strings.NewReader(substituted))
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
		}
	}

	// An unterminated tag never reaches the tokenizer as a tag, so its
	// occurrences come back unresolved. Reporting those as "not in an
	// attribute" would fail open on exactly the template that is most broken:
	// <div title="[meta ... raw="true"] with no ">" is still an attribute an
	// author is interpolating into. Mark them unknown so the caller warns.
	for i, sp := range spans {
		if ctx, ok := out[sp.start]; ok && (ctx.inValue || ctx.inName) {
			continue
		}
		sentinel := lintSentinel(i)
		// An interpolated ELEMENT name never reaches the tokenizer as a tag:
		// "<" followed by anything that is not a name character is text, and the
		// sentinel is deliberately not a name character. The "<" immediately
		// before it is the whole proof, so no tokenizer is needed.
		if at := strings.Index(substituted, sentinel); at > 0 {
			j := at - 1
			if j > 0 && substituted[j] == '/' {
				j--
			}
			if substituted[j] == '<' {
				out[sp.start] = attrContext{inName: true}
				continue
			}
		}
		if insideUnterminatedTag(substituted, sentinel) {
			out[sp.start] = attrContext{unterminated: true}
		}
	}
	return out
}

// insideUnterminatedTag reports whether the sentinel sits after a "<" that is
// never closed, which is the one case the tokenizer cannot describe.
func insideUnterminatedTag(substituted, sentinel string) bool {
	at := strings.Index(substituted, sentinel)
	if at < 0 {
		return false
	}
	open := strings.LastIndexByte(substituted[:at], '<')
	if open < 0 {
		return false
	}
	// The terminator has to be found quote-aware: <div title="before > [meta …]
	// contains a ">" that closes nothing.
	var q byte
	for i := open; i < len(substituted); i++ {
		c := substituted[i]
		switch {
		case q != 0:
			if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '>':
			return false
		}
	}
	return true
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

// unsafeAttributeContexts reports the ways an inline [meta] value is still
// dangerous where it is being placed.
//
// html.EscapeString covers & < > ' " and nothing else — exactly enough to keep a
// value inside a quoted attribute, and nothing at all once the browser re-parses
// that value. The distinction has a privilege boundary behind it: an admin or
// editor writes the template, but the Meta value it interpolates is written by
// whoever can edit the entity, which includes the plain user role.
//
// raw disables even that much, so with it every attribute position is unsafe.
func unsafeAttributeContexts(ctx attrContext, raw bool) []string {
	if ctx.unterminated {
		return []string{`[meta inline] is inside a tag that is never closed, so where it lands cannot be determined. Close the tag; until then treat the value as unsafe.`}
	}
	if ctx.inName {
		return []string{`[meta inline] is interpolated into a tag or attribute NAME, which nothing delimits: a value containing a space or "=" simply adds attributes of its own, and escaping does not touch either character. Build the name in the template instead.`}
	}
	if !ctx.inValue || ctx.attr == "" {
		return nil
	}
	attr := ctx.attr

	var out []string
	if raw {
		out = append(out, `[meta inline raw] is not escaped at all, so placing it in the "`+attr+`" attribute lets the value close the attribute and add its own. Drop raw= here.`)
	}
	if !ctx.quoted {
		out = append(out, `[meta inline] sits in an unquoted attribute value, where escaping does not stop a value containing a space from adding attributes of its own. Quote the attribute.`)
	}
	if kind := expressionAttributeKind(attr); kind != "" {
		out = append(out, `[meta inline] sits in the "`+attr+`" `+kind+`, whose value is evaluated as script after the HTML parser has undone the escaping, so a value containing a quote can execute. Do not interpolate Meta into it.`)
	}
	if attr == "style" {
		out = append(out, `[meta inline] sits in a "style" attribute, where escaping does not prevent CSS injection.`)
	}
	if alwaysUnsafeAttrs[attr] {
		out = append(out, `[meta inline] sits in a "`+attr+`" attribute, whose value the browser decodes and parses as HTML, so escaping does not prevent script injection anywhere in it.`)
	}
	if urlBearingAttrs[attr] {
		scheme, fixed := urlSchemeBefore(ctx.valueSoFar)
		switch {
		case fixed && executableURLSchemes[scheme]:
			out = append(out, `[meta inline] continues a "`+scheme+`:" URL in "`+attr+`", which the browser executes rather than fetches.`)
		case !fixed && couldStillBecomeExecutable(scheme):
			out = append(out, `[meta inline] can still choose the scheme of the "`+attr+`" URL, and escaping does not stop a "javascript:" value. Put a path or a complete scheme in front of it, e.g. href="/x/[meta ...]".`)
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
	// x-on:click / x-bind:href / x-init / x-text / x-show / ...
	if strings.HasPrefix(attr, "x-") {
		return "Alpine directive"
	}
	// @click and :href are the shorthands for x-on: and x-bind:. Only a leading
	// colon is Alpine — xlink:href has one in the middle and is a URL.
	if strings.HasPrefix(attr, "@") || strings.HasPrefix(attr, ":") {
		return "Alpine directive"
	}
	return ""
}
