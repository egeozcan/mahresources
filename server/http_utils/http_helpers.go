package http_utils

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"mahresources/constants"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

// ErrInvalidSortColumn is returned when a sort column does not exist in the database.
var ErrInvalidSortColumn = errors.New("invalid sort column")

// IsDateFilterError checks whether an error wraps the ErrInvalidDateFilter sentinel
// from the database_scopes package. When true, the handler should return HTTP 400.
func IsDateFilterError(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "invalid date filter value")
}

// IsColumnError checks whether an error is a database "no such column" error
// (from SQLite) or "column ... does not exist" (from Postgres).
// When true, the raw error should not be exposed to the client.
func IsColumnError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such column") ||
		strings.Contains(msg, "does not exist")
}

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

// GetUIntFormValue reads a uint parameter from the request using FormValue,
// which checks URL query parameters, application/x-www-form-urlencoded bodies,
// and multipart/form-data bodies. Returns defVal if the parameter is missing or
// cannot be parsed.
func GetUIntFormValue(request *http.Request, paramName string, defVal uint) uint {
	// Go's FormValue only parses bodies for POST/PUT/PATCH. For DELETE
	// (and other methods) we need to manually parse the body first.
	if request.Method == http.MethodDelete && request.PostForm == nil {
		ct := request.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "application/x-www-form-urlencoded") && request.Body != nil {
			bodyBytes, err := io.ReadAll(request.Body)
			if err == nil {
				request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
				if parsed, err := url.ParseQuery(string(bodyBytes)); err == nil {
					request.PostForm = parsed
				}
			}
		}
	}

	val := request.FormValue(paramName)
	if val == "" {
		return defVal
	}

	parsed, err := strconv.ParseUint(val, 10, 0)
	if err != nil {
		return defVal
	}

	return uint(parsed)
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

// SanitizeSchemaError rewrites raw gorilla/schema conversion errors into
// user-friendly messages. If the error is not a schema error, it is returned
// unchanged.
func SanitizeSchemaError(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if !strings.Contains(msg, "schema: error converting value") &&
		!strings.Contains(msg, "schema: invalid path") {
		return err
	}
	return fmt.Errorf("invalid value for %s: must be a valid number", extractSchemaFieldName(msg))
}

// extractSchemaFieldName extracts the quoted field name from a gorilla/schema
// error message like `schema: error converting value for "id"` or
// `schema: error converting value for index 0 of "Tags"`.
func extractSchemaFieldName(msg string) string {
	// Look for the last quoted string in the message
	lastQuote := strings.LastIndex(msg, "\"")
	if lastQuote <= 0 {
		return "parameter"
	}
	penultimateQuote := strings.LastIndex(msg[:lastQuote], "\"")
	if penultimateQuote < 0 {
		return "parameter"
	}
	return "\"" + msg[penultimateQuote+1:lastQuote] + "\""
}

func HandleError(err error, writer http.ResponseWriter, request *http.Request, responseCode int) {
	err = SanitizeSchemaError(err)
	fmt.Printf("\n[ERROR]: %v\n", err)

	if RequestAcceptsHTML(request) {
		writer.Header().Set("Content-Type", "text/html")
		writer.WriteHeader(responseCode)
		_, _ = fmt.Fprintf(writer, `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Error %d</title>
    <link rel="stylesheet" href="/public/tailwind.css">
    <link rel="stylesheet" href="/public/index.css">
    <style>
        .error-container { max-width: 40rem; margin: 4rem auto; padding: 2rem; }
        .error-heading { font-size: 1.5rem; font-weight: 700; color: #991b1b; margin-bottom: 1rem; }
        .error-detail { background: #fef2f2; border: 1px solid #fecaca; border-radius: 0.5rem; padding: 1rem; }
        .error-detail code { font-size: 0.875rem; color: #7f1d1d; white-space: pre-wrap; word-break: break-word; }
        .error-back { margin-top: 1.5rem; }
        .error-back a { color: #2563eb; text-decoration: underline; }
    </style>
</head>
<body>
    <div class="error-container">
        <h1 class="error-heading">An error has occurred</h1>
        <div class="error-detail"><pre><code>%v</code></pre></div>
        <p class="error-back"><a href="javascript:history.back()">Go back</a></p>
    </div>
</body>
</html>`, responseCode, html.EscapeString(err.Error()))
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

// maxPage is a safe upper bound that prevents integer overflow when computing
// offset = (page-1) * perPage.  For perPage up to 200 (the template-layer
// maximum), (maxPage-1)*200 is well within int64 range.
const maxPage int64 = 1_000_000_000

// GetPageParameter returns the "page" query parameter, clamped to [1, maxPage].
// The upper bound prevents integer overflow when computing pagination offsets.
func GetPageParameter(request *http.Request) int64 {
	page := GetIntQueryParameter(request, "page", 1)
	if page < 1 {
		page = 1
	}
	if page > maxPage {
		page = maxPage
	}
	return page
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

// GetUIntFormParameter reads a uint parameter from the request body
// (form-encoded) first, then falls back to URL query string. Unlike
// http.Request.FormValue, this explicitly parses the body for all HTTP
// methods including DELETE, which Go's standard library skips.
func GetUIntFormParameter(request *http.Request, paramName string, defVal uint) uint {
	// Ensure the form body is parsed for all methods. Go's ParseForm only
	// reads the body for POST, PUT, and PATCH, so for DELETE (and other
	// methods) we need to parse it manually.
	if request.PostForm == nil {
		ct := request.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "application/x-www-form-urlencoded") {
			if request.Body != nil {
				bodyBytes, err := io.ReadAll(request.Body)
				if err == nil {
					// Restore the body so it can be read again if needed
					request.Body = io.NopCloser(bytes.NewReader(bodyBytes))
					parsed, err := url.ParseQuery(string(bodyBytes))
					if err == nil {
						request.PostForm = parsed
					}
				}
			}
		}
	}

	// Check POST body first, then URL query string
	val := ""
	if request.PostForm != nil {
		val = request.PostForm.Get(paramName)
	}
	if val == "" {
		val = request.URL.Query().Get(paramName)
	}
	if val == "" {
		return defVal
	}
	param, err := strconv.ParseUint(val, 10, 0)
	if err != nil {
		return defVal
	}
	return uint(param)
}

func GetQueryParameter(request *http.Request, paramName string, defVal string) string {
	paramFromRes := request.URL.Query().Get(paramName)

	if paramFromRes != "" {
		return paramFromRes
	}

	return defVal
}
