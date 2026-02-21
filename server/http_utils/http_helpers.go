package http_utils

import (
	"encoding/json"
	"fmt"
	"html"
	"mahresources/constants"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func GetIntQueryParameter(request *http.Request, paramName string, defVal int64) int64 {
	paramFromRes := GetQueryParameter(request, paramName, "")

	if paramFromRes == "" {
		return defVal
	}

	param, err := strconv.ParseInt(paramFromRes, 10, 0)

	if err != nil {
		return defVal
	}

	return param
}

func GetUIntQueryParameter(request *http.Request, paramName string, defVal uint) uint {
	paramFromRes := GetQueryParameter(request, paramName, "")

	if paramFromRes == "" {
		return defVal
	}

	param, err := strconv.ParseUint(paramFromRes, 10, 0)

	if err != nil {
		return defVal
	}

	return uint(param)
}

func RedirectIfHTMLAccepted(writer http.ResponseWriter, request *http.Request, defaultURL string) bool {
	requestedBackUrl := GetQueryParameter(request, "redirect", "")

	if requestedBackUrl != "" && isSafeRedirect(requestedBackUrl) {
		http.Redirect(writer, request, requestedBackUrl, http.StatusSeeOther)

		return true
	}

	backUrl := defaultURL

	if defaultURL == "" {
		backUrl = request.Referer()
	}

	if backUrl == "" {
		return false
	}

	if RequestAcceptsHTML(request) {
		http.Redirect(writer, request, backUrl, http.StatusSeeOther)

		return true
	}

	return false
}

func RemoveValue(items []string, item string) []string {
	var newItems []string

	for _, i := range items {
		if i != item {
			newItems = append(newItems, i)
		}
	}

	return newItems
}

func HandleError(err error, writer http.ResponseWriter, request *http.Request, responseCode int) {
	fmt.Printf("\n[ERROR]: %v\n", err)

	if RequestAcceptsHTML(request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.WriteHeader(responseCode)
		_, _ = fmt.Fprintf(writer, `
			<html>
				<head><title>Error</title></head>
				<body><h1>An error has occured:</h1><pre><code>%v</code></pre></body>
			</html>
		`, html.EscapeString(err.Error()))
		return
	}

	writer.Header().Set("Content-Type", constants.JSON)
	writer.WriteHeader(responseCode)
	_ = json.NewEncoder(writer).Encode(map[string]string{"error": err.Error()})
}

// isSafeRedirect checks that a redirect URL is a relative path and not an open redirect.
func isSafeRedirect(rawURL string) bool {
	// Must start with a single slash (not //)
	if !strings.HasPrefix(rawURL, "/") || strings.HasPrefix(rawURL, "//") {
		return false
	}

	// Parse the URL to check for scheme-based tricks
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Reject if it has a scheme or host (e.g., "javascript:", "data:", or absolute URLs)
	if parsed.Scheme != "" || parsed.Host != "" {
		return false
	}

	return true
}

// RequestAcceptsHTML reports whether the request's Accept header includes text/html.
func RequestAcceptsHTML(request *http.Request) bool {
	accepts := request.Header["Accept"]

	if len(accepts) == 0 {
		return false
	}

	for _, val := range accepts {

		if strings.Contains(val, "text/html") {
			return true
		}
	}

	return false
}

// SetPaginationHeaders sets standard pagination response headers.
// totalCount of -1 means the total is unknown (header will not be set).
func SetPaginationHeaders(writer http.ResponseWriter, page, perPage int, totalCount int64) {
	writer.Header().Set("X-Page", strconv.Itoa(page))
	writer.Header().Set("X-Per-Page", strconv.Itoa(perPage))
	if totalCount >= 0 {
		writer.Header().Set("X-Total-Count", strconv.FormatInt(totalCount, 10))
	}
}

func GetQueryParameter(request *http.Request, paramName string, defVal string) string {
	paramFromRes := request.URL.Query().Get(paramName)

	if paramFromRes != "" {
		return paramFromRes
	}

	return defVal
}
