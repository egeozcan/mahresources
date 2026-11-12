package arch

import (
	"regexp"
	"strings"
	"testing"
)

var contentTypeInJSString = regexp.MustCompile(`'\{\{[^}]*\.ContentType\s*\}\}'`)

// TestNoContentTypeInAJSStringWithoutEscapejs: pongo2 autoescape is HTML
// encoding, which the HTML parser undoes before Alpine evaluates the attribute.
// A content type placed in a '...' literal inside an Alpine directive must go
// through |escapejs (issue #135).
func TestNoContentTypeInAJSStringWithoutEscapejs(t *testing.T) {
	directive := regexp.MustCompile(`(^|\s)(@[\w.:-]+|:[\w.-]+|x-[\w.:-]+)=`)
	for path, body := range templateFiles(t) {
		for i, line := range strings.Split(body, "\n") {
			if directive.MatchString(line) && contentTypeInJSString.MatchString(line) {
				t.Errorf("%s:%d: .ContentType in a JS string without |escapejs: %s", path, i+1, strings.TrimSpace(line))
			}
		}
	}
}
