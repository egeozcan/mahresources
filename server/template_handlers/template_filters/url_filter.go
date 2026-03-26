package template_filters

import (
	"github.com/flosch/pongo2/v4"
	"mahresources/models/types"
	"net/url"
	"strings"
)

// safeURLSchemes lists the URL schemes that are safe to render as clickable links.
var safeURLSchemes = map[string]bool{
	"http":  true,
	"https": true,
	"ftp":   true,
	"ftps":  true,
	"":      true, // relative URLs
}

// isSafeURL returns true if the URL uses a safe scheme.
func isSafeURL(u string) bool {
	lower := strings.ToLower(strings.TrimSpace(u))
	// Check for known dangerous patterns before parsing,
	// since url.Parse may not catch all edge cases (e.g., "javascript:...")
	if strings.HasPrefix(lower, "javascript:") || strings.HasPrefix(lower, "data:") || strings.HasPrefix(lower, "vbscript:") {
		return false
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	return safeURLSchemes[strings.ToLower(parsed.Scheme)]
}

//goland:noinspection GoUnusedParameter
func urlFilter(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	interfaceVal := in.Interface()
	input, ok := interfaceVal.(types.URL)

	if !ok {
		strInput, okStr := interfaceVal.(string)

		if okStr {
			if !isSafeURL(strInput) {
				return pongo2.AsValue(""), nil
			}
			return pongo2.AsValue(strInput), nil
		}

		input2 := interfaceVal.(*types.URL)

		if input2 == nil {
			return pongo2.AsValue(""), nil
		}

		input = *input2
	}

	converted := url.URL(input)
	result := converted.String()

	if !isSafeURL(result) {
		return pongo2.AsValue(""), nil
	}

	return pongo2.AsValue(result), nil
}
