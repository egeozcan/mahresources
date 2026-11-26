package shortcodes

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"

	"mahresources/constants"
	"mahresources/mrql"
)

const (
	// defaultSimilarLimit is how many lookalikes [similar] shows unasked.
	defaultSimilarLimit = 8
	// maxSimilarLimit caps limit=. It also bounds the IN list of the distance
	// query the executor runs for this shortcode.
	maxSimilarLimit = 24
	// defaultSimilarLabel is the caption, and so the strip's accessible name,
	// when label= is unset or blank.
	defaultSimilarLabel = "Similar resources"
)

// similarSize is one thumbnail size: the edge in pixels, which the preview URL
// and the width/height attributes carry, and the Tailwind classes for it. The
// classes are whole literals so the stylesheet's source scan finds them.
type similarSize struct {
	px    int
	class string
}

var similarSizes = map[string]similarSize{
	"sm": {48, "h-12 w-12"},
	"md": {96, "h-24 w-24"},
	"lg": {200, "h-[200px] w-[200px]"},
}

// similarOptions is a [similar] opener's attributes, validated.
type similarOptions struct {
	maxDistance int // -1 when unset: the runtime similarity threshold applies
	limit       int
	size        similarSize
	label       string
}

// parseSimilarAttrs validates a [similar] opener's attributes. problem is the
// author-facing message for the first invalid one; the renderer shows it in an
// error marker and the linter reports it verbatim, so the two cannot disagree.
// A blank attribute counts as unset.
func parseSimilarAttrs(attrs map[string]string) (similarOptions, string) {
	opts := similarOptions{maxDistance: -1, limit: defaultSimilarLimit, size: similarSizes["md"], label: defaultSimilarLabel}

	if raw := strings.TrimSpace(attrs["max-distance"]); raw != "" {
		d, ok := parseWholeNumber(raw)
		if !ok || d > mrql.MaxSimilarityDistance {
			return opts, fmt.Sprintf("[similar] max-distance must be a whole number from 0 to %d", mrql.MaxSimilarityDistance)
		}
		opts.maxDistance = d
	}
	if raw := strings.TrimSpace(attrs["limit"]); raw != "" {
		n, ok := parseWholeNumber(raw)
		if !ok || n < 1 || n > maxSimilarLimit {
			return opts, fmt.Sprintf("[similar] limit must be a whole number from 1 to %d", maxSimilarLimit)
		}
		opts.limit = n
	}
	if raw := strings.TrimSpace(attrs["size"]); raw != "" {
		size, ok := similarSizes[raw]
		if !ok {
			return opts, `[similar] size must be "sm", "md" or "lg"`
		}
		opts.size = size
	}
	if label := strings.TrimSpace(attrs["label"]); label != "" {
		opts.label = label
	}
	return opts, ""
}

// parseWholeNumber accepts plain ASCII digits only: no sign, no fraction, no
// exponent, at most four of them.
func parseWholeNumber(s string) (int, bool) {
	if s == "" || len(s) > 4 {
		return 0, false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return 0, false
		}
	}
	n, err := strconv.Atoi(s)
	return n, err == nil
}

// similarTierLabel is the tier of a perceptual distance: the visible badge word
// and the phrase the alt text speaks. The cut-off is the one the resource page's
// Similar Resources panel uses (models.Resource.SimilarityTier).
func similarTierLabel(d int) (badge, spoken string) {
	if d <= constants.SimilarityDuplicateMaxDistance {
		return "Duplicate", "near-certain duplicate"
	}
	return "Similar", "similar"
}

// similarQuery is the MRQL [similar] runs. Only the context's own ID and a
// validated distance go into the text.
func similarQuery(targetID uint, maxDistance int) string {
	within := ""
	if maxDistance >= 0 {
		within = fmt.Sprintf(" WITHIN %d", maxDistance)
	}
	return fmt.Sprintf("type = resource AND SIMILAR TO resource(%d)%s ORDER BY distance ASC", targetID, within)
}

// RenderSimilarShortcode renders [similar]: thumbnails of the resources whose
// stored perceptual-hash pair with the current resource is within the
// threshold, nearest first, each with its tier, distance and a compare link.
//
// It runs one SIMILAR TO query through the executor, so the principal's subtree
// confinement, the page query budget and its cache apply exactly as they do to
// [mrql]. Global scope is requested on purpose (duplicates cross groups); a
// confined principal is forced back to its subtree by the executor. The output
// is final HTML and is never expanded again, so a resource name that looks like
// a shortcode stays text.
func RenderSimilarShortcode(reqCtx context.Context, sc Shortcode, ctx MetaShortcodeContext, executor QueryExecutor) string {
	opts, problem := parseSimilarAttrs(sc.Attrs)
	if problem != "" {
		return shortcodeErrorMarker("similar", problem)
	}
	// A <button> takes phrasing content only; a figure of links is neither.
	if withinReloadFace(reqCtx) {
		return shortcodeErrorMarker("similar", "[similar] cannot be used inside a [reload] button face")
	}
	if ctx.EntityType != "resource" || ctx.EntityID == 0 {
		return shortcodeErrorMarker("similar", `[similar] works only on a resource; use [using type="resource" …] to target one`)
	}

	result, err := executor(reqCtx, similarQuery(ctx.EntityID, opts.maxDistance), QueryOptions{
		Limit:                  opts.limit,
		WantSimilarityDistance: true,
	})
	if err != nil {
		if errors.Is(err, ErrQueryUnavailable) {
			return unavailableComment(err)
		}
		return mrqlErrorHTML(err, false)
	}
	if !meterFrom(reqCtx).alive() {
		return ""
	}
	if result == nil || result.Mode != "flat" || len(result.Items) == 0 {
		return ""
	}

	targetName := readStringField(ctx.Entity, "Name")
	if targetName == "" {
		targetName = fmt.Sprintf("resource %d", ctx.EntityID)
	}

	var items strings.Builder
	shown := 0
	for _, item := range result.Items {
		if shown >= opts.limit {
			break
		}
		if item.EntityType != "resource" || item.EntityID == 0 {
			continue
		}
		shown++
		writeSimilarItem(&items, item, ctx.EntityID, targetName, opts.size)
	}
	if shown == 0 {
		return ""
	}

	return fmt.Sprintf(
		`<figure class="similar-strip my-2" data-similar-for="%d"><figcaption class="text-xs font-semibold mb-1">%s</figcaption><ul class="flex flex-wrap gap-2" role="list">%s</ul></figure>`,
		ctx.EntityID, html.EscapeString(opts.label), items.String())
}

// similarFocus is the focus ring both links carry: amber-500 stays above 3:1
// on the white page and on the lightbox's stone-900 panel.
const similarFocus = "focus-visible:outline focus-visible:outline-2 focus-visible:outline-amber-500"

func writeSimilarItem(b *strings.Builder, item QueryResultItem, targetID uint, targetName string, size similarSize) {
	name := readStringField(item.Entity, "Name")
	if name == "" {
		name = fmt.Sprintf("Resource %d", item.EntityID)
	}
	hash := readStringField(item.Entity, "Hash")

	alt := name
	badge := ""
	if d := item.SimilarityDistance; d != nil {
		word, spoken := similarTierLabel(*d)
		alt = fmt.Sprintf("%s, %s, distance %d", name, spoken, *d)
		colours := "text-stone-700 bg-stone-100"
		if word == "Duplicate" {
			colours = "text-rose-800 bg-rose-100"
		}
		// Hidden from assistive technology: the image's alt already says it.
		badge = fmt.Sprintf(`<span class="text-[11px] font-medium rounded px-1 %s" aria-hidden="true">%s · %d</span>`, colours, word, *d)
	}

	fmt.Fprintf(b, `<li class="flex flex-col items-center gap-0.5">`)
	fmt.Fprintf(b, `<a href="/resource?id=%d" class="block rounded %s">`, item.EntityID, similarFocus)
	fmt.Fprintf(b,
		`<img src="/v1/resource/preview?id=%d&amp;width=%d&amp;height=%d&amp;v=%s" width="%d" height="%d" loading="lazy" decoding="async" alt="%s" class="rounded object-cover %s bg-stone-100">`,
		item.EntityID, size.px, size.px, html.EscapeString(url.QueryEscape(hash)), size.px, size.px, html.EscapeString(alt), size.class)
	b.WriteString(`</a>`)
	b.WriteString(badge)
	fmt.Fprintf(b,
		`<a href="/resource/compare?r1=%d&amp;r2=%d" class="text-xs underline inline-flex items-center min-h-6 px-1 rounded %s">Compare<span class="sr-only"> %s with %s</span></a>`,
		targetID, item.EntityID, similarFocus, html.EscapeString(name), html.EscapeString(targetName))
	b.WriteString(`</li>`)
}
