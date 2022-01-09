package http_utils

import (
	"fmt"
	"mahresources/constants"
	"net/http"
	"strconv"
	"strings"
)

func GetIntQueryParameter(request *http.Request, paramName string, defVal int64) int64 {
	paramFromRes := getQueryParameter(request, paramName, "")

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
	paramFromRes := getQueryParameter(request, paramName, "")

	if paramFromRes == "" {
		return defVal
	}

	param, err := strconv.ParseUint(paramFromRes, 10, 0)

	if err != nil {
		return defVal
	}

	return uint(param)
}

func RedirectIfHTMLAccepted(writer http.ResponseWriter, request *http.Request, url string) bool {
	requestedBackUrl := getQueryParameter(request, "redirect", "")

	if requestedBackUrl != "" {
		http.Redirect(writer, request, requestedBackUrl, http.StatusSeeOther)

		return true
	}

	backUrl := url

	if url == "" {
		backUrl = request.Referer()
	}

	if backUrl == "" {
		return false
	}

	if requestAcceptsHTML(request) {
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
	writer.WriteHeader(responseCode)
	fmt.Printf("\n[ERROR]: %v\n", err)

	if requestAcceptsHTML(request) {
		writer.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprintf(writer, `
			<html>
				<head><title>Error</title></head>
				<body><h1>An error has occured:</h1><pre><code>%v</code></pre></body>
			</html>
		`, err.Error())
		return
	}

	writer.Header().Set("Content-Type", constants.JSON)
	_, _ = fmt.Fprint(writer, err.Error())
}

func requestAcceptsHTML(request *http.Request) bool {
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

func getQueryParameter(request *http.Request, paramName string, defVal string) string {
	paramFromRes := request.URL.Query().Get(paramName)

	if paramFromRes != "" {
		return paramFromRes
	}

	return defVal
}
