// Package templatecomment finds and removes template comments,
// [comment]...[/comment], from template and Description text. It has no
// dependencies so that both the template engine (shortcodes/) and the models
// that assemble template text (models/) can use it.
//
// A comment is removed before any shortcode is parsed, so nothing inside it
// runs and none of it is rendered. That is what sets it apart from an HTML
// comment, which is sent to the browser with every shortcode inside it
// already expanded.
//
// Comments nest, so a region that already holds a comment can be commented
// out. An unclosed opener hides everything to the end of the text, the way an
// unclosed <!-- does, and a closer with no opener is dropped; the linter
// reports both. The opener takes attributes (e.g. reason="…") and ignores
// them; it ends at its first "]", so a value cannot hold one, and it fits on
// one line. No tag spans a line break, so text joined with "\n", like a
// carrier's CSS fields, cannot form a tag across the join. There is no escape
// for writing a literal [comment].
//
// Removal repeats until no comment is left, because removing one can join the
// text around it into a new tag: [comm[comment][/comment]ent] becomes
// [comment]. Stripping once more is then a no-op, so every surface agrees no
// matter how many times its text passes through. Only text built to join tags
// needs a second pass, and maxPasses bounds the work on text built to need
// thousands.
//
// The editor's copy of these rules is src/components/templateCommentRanges.js.
package templatecomment

import (
	"sort"
	"strings"
)

const (
	openPrefix = "[comment"
	closeTag   = "[/comment]"
)

// Span is a byte range [Start, End) of the scanned text.
type Span struct{ Start, End int }

// Scan is where the comments in a text are.
type Scan struct {
	Regions  []Span // byte ranges to remove, in order, non-overlapping
	Unclosed []Span // the outermost opener of a region that runs to the end
	Strays   []Span // closers with no opener
}

// isSpace is the space that may follow the name: Go's \s without the line
// breaks, spelled out so the JavaScript copy matches it exactly (JavaScript's
// \s also takes NBSP and other Unicode spaces).
func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\f'
}

// openerEnd is the index of the "]" closing an opener whose attributes start
// at from, or, when a line break or the end comes first, -1 and where the scan
// stopped.
func openerEnd(text string, from int) (int, int) {
	for i := from; i < len(text); i++ {
		switch text[i] {
		case ']':
			return i, i
		case '\n', '\r':
			return -1, i
		}
	}
	return -1, len(text)
}

// openers finds every [comment] and [comment attr…] in a forward-only pass. An
// opener that meets a line break before its "]" fails, and so does every later
// one on that line, so the scan resumes at the break, which is what keeps a
// text of unfinished openers linear.
func openers(text string) []Span {
	var spans []Span
	for pos := 0; ; {
		i := strings.Index(text[pos:], openPrefix)
		if i < 0 {
			return spans
		}
		start := pos + i
		after := start + len(openPrefix)
		switch {
		case after < len(text) && text[after] == ']':
			spans = append(spans, Span{start, after + 1})
			pos = after + 1
		case after < len(text) && isSpace(text[after]):
			close, stop := openerEnd(text, after)
			if close < 0 {
				pos = stop
				continue
			}
			spans = append(spans, Span{start, close + 1})
			pos = close + 1
		default:
			pos = after
		}
	}
}

func closers(text string) []Span {
	var spans []Span
	for pos := 0; ; {
		i := strings.Index(text[pos:], closeTag)
		if i < 0 {
			return spans
		}
		start := pos + i
		spans = append(spans, Span{start, start + len(closeTag)})
		pos = start + len(closeTag)
	}
}

// ScanText finds the comments in text.
func ScanText(text string) Scan {
	var scan Scan
	if !strings.Contains(text, "comment") {
		return scan
	}

	type tag struct {
		Span
		closing bool
	}
	var tags []tag
	for _, s := range openers(text) {
		tags = append(tags, tag{Span: s})
	}
	for _, s := range closers(text) {
		tags = append(tags, tag{Span: s, closing: true})
	}
	if len(tags) == 0 {
		return scan
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].Start < tags[j].Start })

	depth := 0
	var open Span
	lastEnd := 0
	for _, tg := range tags {
		// A closer that overlaps an opener, as in [comment x="[/comment], is
		// part of the opener.
		if tg.Start < lastEnd {
			continue
		}
		lastEnd = tg.End
		switch {
		case !tg.closing:
			if depth == 0 {
				open = tg.Span
			}
			depth++
		case depth == 0:
			scan.Strays = append(scan.Strays, tg.Span)
			scan.Regions = append(scan.Regions, tg.Span)
		default:
			depth--
			if depth == 0 {
				scan.Regions = append(scan.Regions, Span{open.Start, tg.End})
			}
		}
	}
	if depth > 0 {
		scan.Unclosed = append(scan.Unclosed, open)
		scan.Regions = append(scan.Regions, Span{open.Start, len(text)})
	}
	return scan
}

// maxPasses caps the repeated removal. The JavaScript copy uses the same cap.
const maxPasses = 16

// Strip removes every template comment from text.
func Strip(text string) string {
	for range maxPasses {
		scan := ScanText(text)
		if len(scan.Regions) == 0 {
			break
		}
		text, _ = stripOnce(text, scan)
	}
	return text
}

// Stripped is text with its template comments removed, as Strip removes them,
// with what a linter needs to report on it in terms of the original text.
type Stripped struct {
	Text string
	// ToOriginal maps an offset in Text back to one in the original text. An
	// offset where a comment was removed maps to just after it, which is right
	// for a start offset; use EndToOriginal for an exclusive end.
	ToOriginal func(int) int
	// Unclosed and Strays are every pass's unclosed openers and stray
	// closers, including tags that only formed once an earlier pass joined the
	// text around a removed comment, in original offsets.
	Unclosed, Strays []Span
}

// StripMapped removes every template comment from text, as Strip does.
func StripMapped(text string) Stripped {
	out := Stripped{ToOriginal: func(o int) int { return o }}
	for range maxPasses {
		scan := ScanText(text)
		if len(scan.Regions) == 0 {
			break
		}
		toOriginal := out.ToOriginal
		mapSpan := func(s Span) Span {
			return Span{toOriginal(s.Start), EndToOriginal(toOriginal, s.End)}
		}
		for _, s := range scan.Unclosed {
			out.Unclosed = append(out.Unclosed, mapSpan(s))
		}
		for _, s := range scan.Strays {
			out.Strays = append(out.Strays, mapSpan(s))
		}
		var step func(int) int
		text, step = stripOnce(text, scan)
		out.ToOriginal = func(o int) int { return toOriginal(step(o)) }
	}
	out.Text = text
	return out
}

// EndToOriginal maps an exclusive end offset through toOriginal (from
// StripMapped) so that it stays at the end of the text before it rather than
// moving past a comment removed right after it.
func EndToOriginal(toOriginal func(int) int, end int) int {
	if end <= 0 {
		return toOriginal(0)
	}
	return toOriginal(end-1) + 1
}

func stripOnce(text string, scan Scan) (string, func(int) int) {
	// kept[i] is a run of text that survives: it starts at orig in text and at
	// at in the result.
	type run struct{ at, orig int }
	var kept []run
	var b strings.Builder
	b.Grow(len(text))
	last := 0
	for _, r := range scan.Regions {
		kept = append(kept, run{b.Len(), last})
		b.WriteString(text[last:r.Start])
		last = r.End
	}
	kept = append(kept, run{b.Len(), last})
	b.WriteString(text[last:])

	toOriginal := func(o int) int {
		i := max(sort.Search(len(kept), func(i int) bool { return kept[i].at > o })-1, 0)
		return kept[i].orig + (o - kept[i].at)
	}
	return b.String(), toOriginal
}
