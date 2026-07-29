package template_filters

import (
	"fmt"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/flosch/pongo2/v4"
)

func init() {
	err := pongo2.RegisterFilter("base64", base64Filter)

	if err != nil {
		fmt.Println("error when registering base64 filter", err)
	}

	err = pongo2.RegisterFilter("datetime", filterDateTime)

	if err != nil {
		fmt.Println("error when registering datetime filter", err)
	}

	humanReadableSizeErr := pongo2.RegisterFilter("humanReadableSize", humanReadableSize)

	if humanReadableSizeErr != nil {
		fmt.Println("error when registering humanReadableSize filter", humanReadableSizeErr)
	}

	nanoidErr := pongo2.RegisterFilter("nanoid", nanoidFilter)

	if nanoidErr != nil {
		fmt.Println("error when registering nanoid filter", nanoidErr)
	}

	jsonErr := pongo2.RegisterFilter("json", jsonFilter)

	if jsonErr != nil {
		fmt.Println("error when registering json filter", jsonErr)
	}

	urlErr := pongo2.RegisterFilter("printUrl", urlFilter)

	if urlErr != nil {
		fmt.Println("error when registering url print filter", urlErr)
	}

	markdownErr := pongo2.RegisterFilter("markdown2", markDownFilter)

	if markdownErr != nil {
		fmt.Println("error when registering url markdown2 filter", markdownErr)
	}

	lookupErr := pongo2.RegisterFilter("lookup", lookupFilter)

	if lookupErr != nil {
		fmt.Println("error when registering lookup filter", lookupErr)
	}

	timeagoErr := pongo2.RegisterFilter("timeago", timeagoFilter)

	if timeagoErr != nil {
		fmt.Println("error when registering timeago filter", timeagoErr)
	}

	entityPathErr := pongo2.RegisterFilter("entityPath", entityPathFilter)

	if entityPathErr != nil {
		fmt.Println("error when registering entityPath filter", entityPathErr)
	}

	mentionsErr := pongo2.RegisterFilter("render_mentions", renderMentionsFilter)

	if mentionsErr != nil {
		fmt.Println("error when registering render_mentions filter", mentionsErr)
	}

	// Replaces pongo2's built-in escapejs, which corrupts every character
	// outside the BMP. See escapeJsFilter.
	escapejsErr := pongo2.ReplaceFilter("escapejs", escapeJsFilter)

	if escapejsErr != nil {
		fmt.Println("error when replacing escapejs filter", escapejsErr)
	}
}

// escapeJsFilter escapes a string for embedding in a JavaScript string literal.
//
// It replaces pongo2's built-in escapejs, which formats every non-alphanumeric
// rune with fmt.Sprintf(`\u%04X`, r). %04X is a *minimum* width, so an astral
// character such as U+1F3A8 (🎨) is emitted as the five-digit `Ἲ8`, and
// JavaScript reads that as `Ἲ` followed by a literal "8" — the clipboard
// silently received "Ἲ8" while the page still rendered the emoji correctly.
//
// Runes above the BMP are emitted as a UTF-16 surrogate pair, which is the only
// form a `\uXXXX` escape sequence can express.
func escapeJsFilter(in *pongo2.Value, _ *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	var b strings.Builder

	for _, r := range in.String() {
		switch {
		case r == utf8.RuneError:
			continue
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == ' ':
			b.WriteRune(r)
		case r > 0xFFFF:
			r1, r2 := utf16.EncodeRune(r)
			fmt.Fprintf(&b, `\u%04X\u%04X`, r1, r2)
		default:
			fmt.Fprintf(&b, `\u%04X`, r)
		}
	}

	return pongo2.AsValue(b.String()), nil
}
