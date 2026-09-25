package download_queue

import (
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"mahresources/hls"
)

// failureReason renders a transfer's error as a reason that names no URL beyond
// its origin.
//
// DownloadJob.Error keeps the error's own text, which is what the legacy surfaces
// have always shown the submitter. That text can name the URL a request failed on,
// path and query included, and the query is where a signed URL keeps its
// signature. The durable Job keeps its replay input sealed for exactly that
// reason, so its failure message, which is stored in the clear and searchable,
// takes this rendering instead.
//
// No one rule is enough, because the text comes from many places: net/http, a
// URL parser, the egress check, the HLS assembler, the resource writer. So three
// layers apply, from the most exact to the bluntest:
//
//  1. By error type. A *url.Error is rendered as its cause, without the URL it
//     carries; that covers every failed request and every failed parse, including
//     a playlist's relative reference, which has no scheme for a later layer to
//     find. An HTTP status is rendered from the code, never from the server's own
//     reason phrase, which is free text the server chose. A wrapper whose text
//     ends in its cause's keeps its own prefix and renders the cause the same way.
//  2. By the submission. A literal occurrence of the submitted URL is cut to its
//     origin, and one of its path, query, fragment or user info, or any one query
//     value or path segment of six characters or more, is removed wherever it
//     appears; so is a header value the download sent, whole or in part. That
//     covers a refusal that echoes the raw input it could not parse, which may
//     not look like a URL at all, and a server repeating a token.
//  3. As a backstop, every quoted string that looks like a URL or a reference is
//     cut to its origin, or to an ellipsis when it has none, and so is every
//     unquoted scheme://… token.
//
// What this cannot promise is that text a server authored holds nothing it
// chose to put there. A server that received a token can repeat it in any form,
// and layer 2 removes it only in the forms it was sent: whole, or split where a
// URL or a header value is split, down to six characters.
func failureReason(submitted string, headers map[string]string, err error) string {
	if err == nil {
		return ""
	}
	return scrubReasonText(submitted, headers, renderReason(err))
}

// renderReason is layer 1.
func renderReason(err error) string {
	switch e := err.(type) {
	case *httpStatusError:
		return statusReason(e.Code)
	case *hls.StatusError:
		return statusReason(e.Code)
	case *url.Error:
		if e.Err == nil {
			return e.Op + " failed"
		}
		return renderReason(e.Err)
	}
	inner := errors.Unwrap(err)
	if inner == nil {
		return err.Error()
	}
	outer, innerText := err.Error(), inner.Error()
	if strings.HasSuffix(outer, innerText) {
		return outer[:len(outer)-len(innerText)] + renderReason(inner)
	}
	// The cause sits somewhere other than the end, so the two texts cannot be
	// separated; the later layers see the whole of it.
	return outer
}

// statusReason says what an HTTP status means, in the standard's words.
func statusReason(code int) string {
	if text := http.StatusText(code); text != "" {
		return "HTTP " + strconv.Itoa(code) + " " + text
	}
	return "HTTP " + strconv.Itoa(code)
}

// reasonURLPattern finds a scheme:/… token, one slash or two, since a malformed
// URL is exactly what a refusal quotes. It stops only at whitespace or a quote: an
// apostrophe, a parenthesis and a comma are all legal inside a URL.
var reasonURLPattern = regexp.MustCompile(`[A-Za-z][A-Za-z0-9+.-]*:/[^\s"]*`)

// scrubReasonText is layers 2 and 3.
func scrubReasonText(submitted string, headers map[string]string, text string) string {
	if len(submitted) > 1 {
		text = strings.ReplaceAll(text, submitted, originOrEllipsis(submitted))
	}
	for _, secret := range submittedSecrets(submitted, headers) {
		text = strings.ReplaceAll(text, secret, "…")
	}
	text = scrubQuoted(text)
	return reasonURLPattern.ReplaceAllStringFunc(text, func(match string) string {
		// Punctuation that ends the match is the sentence's, not the URL's.
		trimmed := strings.TrimRight(match, ":;,.)")
		return originOrEllipsis(trimmed) + match[len(trimmed):]
	})
}

// submittedSecrets lists what the submission carried that must not appear: the
// parts of its URL and the values of the headers it sent. Longest first, so a
// longer part is replaced before any part inside it. The URL as a whole is
// replaced by its origin before these run.
func submittedSecrets(submitted string, headers map[string]string) []string {
	var secrets []string
	add := func(s string) {
		// A one-character path such as "/" would erase every slash in the text.
		if len(s) > 1 {
			secrets = append(secrets, s)
		}
	}
	parsed, err := url.Parse(submitted)
	if err == nil {
		if parsed.Host != "" {
			if i := strings.Index(submitted, parsed.Host); i >= 0 {
				add(submitted[i+len(parsed.Host):])
			}
		}
		add(parsed.RawQuery)
		add(parsed.Fragment)
		add(parsed.EscapedPath())
		add(parsed.Path)
		if parsed.User != nil {
			add(parsed.User.String())
			add(parsed.User.Username())
			if password, ok := parsed.User.Password(); ok {
				add(password)
			}
		}
		// Each value on its own too, raw and decoded: a server can repeat a token
		// without the rest of the URL around it, in a header it sends back or in a
		// playlist attribute an HLS error then quotes. A capability URL keeps its
		// token in a path segment instead, so those count as well.
		for _, pair := range strings.Split(parsed.RawQuery, "&") {
			_, value, _ := strings.Cut(pair, "=")
			addPart(add, value)
		}
		for _, segment := range strings.Split(parsed.EscapedPath(), "/") {
			addPart(add, segment)
		}
	}
	// A header value is a credential as often as not (a Cookie, an Authorization),
	// and one is more often repeated in part than whole: a cookie's value, the
	// token after "Bearer".
	for _, value := range headers {
		if len(value) >= minSecretPartLength {
			add(value)
		}
		for _, part := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ';' || r == '=' || r == ',' || r == ':' || r == ' ' || r == '\t'
		}) {
			if len(part) >= minSecretPartLength {
				add(part)
			}
		}
	}
	// Longest first.
	for i := 1; i < len(secrets); i++ {
		for j := i; j > 0 && len(secrets[j]) > len(secrets[j-1]); j-- {
			secrets[j], secrets[j-1] = secrets[j-1], secrets[j]
		}
	}
	return secrets
}

// minSecretPartLength is the shortest single query value or path segment removed
// on its own. Shorter ones ("1", "en", "v2") are not tokens, and removing them
// would erase the same characters from unrelated words in the reason.
const minSecretPartLength = 6

// addPart adds one escaped URL part, and its decoded form when that differs.
func addPart(add func(string), escaped string) {
	if len(escaped) >= minSecretPartLength {
		add(escaped)
	}
	if decoded, err := url.QueryUnescape(escaped); err == nil && decoded != escaped && len(decoded) >= minSecretPartLength {
		add(decoded)
	}
}

// scrubQuoted cuts every Go-quoted string that looks like a URL or a reference.
// Go's errors quote the URL they failed on with %q, which escapes a quote inside
// it, so reading the quoted string whole is what finds where it ends.
func scrubQuoted(text string) string {
	var out strings.Builder
	for {
		start := strings.IndexByte(text, '"')
		if start < 0 {
			out.WriteString(text)
			return out.String()
		}
		out.WriteString(text[:start])
		quoted, err := strconv.QuotedPrefix(text[start:])
		if err != nil {
			// An unterminated quote: nothing after it can be read as one string.
			rest := text[start:]
			if looksLikeReference(rest) {
				out.WriteString(`"…`)
			} else {
				out.WriteString(rest)
			}
			return out.String()
		}
		value, err := strconv.Unquote(quoted)
		if err == nil && looksLikeReference(value) {
			out.WriteString(strconv.Quote(originOrEllipsis(value)))
		} else {
			out.WriteString(quoted)
		}
		text = text[start+len(quoted):]
	}
}

// looksLikeReference is true of anything that could be a URL, a path or a query.
func looksLikeReference(s string) bool {
	return strings.ContainsAny(s, "/?#@%&=")
}

// originOrEllipsis keeps an absolute URL's origin and drops everything else.
func originOrEllipsis(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "…"
	}
	// Host, not the whole authority: user info is a credential.
	return parsed.Scheme + "://" + parsed.Host
}
