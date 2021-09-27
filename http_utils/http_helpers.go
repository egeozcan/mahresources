package http_utils

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func GetQueryParameter(request *http.Request, paramName string, defVal string) string {
	paramFromRes := request.URL.Query().Get(paramName)

	if paramFromRes != "" {
		return paramFromRes
	}

	return defVal
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

func GetFormParameter(request *http.Request, paramName string, defVal string) string {
	_ = request.ParseForm()
	paramFromRes := request.PostForm.Get(paramName)

	if paramFromRes != "" {
		return paramFromRes
	}

	if request.MultipartForm == nil {
		return defVal
	}

	values := request.MultipartForm.Value[paramName]

	if values == nil || len(values) == 0 {
		return defVal
	}

	paramFromRes = values[0]

	if paramFromRes != "" {
		return paramFromRes
	}

	return defVal
}

func GetIntFormParameter(request *http.Request, paramName string, defVal int64) int64 {
	paramFromRes := GetFormParameter(request, paramName, "")

	if paramFromRes == "" {
		return defVal
	}

	param, err := strconv.ParseInt(paramFromRes, 10, 0)

	if err != nil {
		return defVal
	}

	return param
}

func RedirectIfHTMLAccepted(writer http.ResponseWriter, request *http.Request, url string) bool {
	requestedBackUrl := GetQueryParameter(request, "redirect", "")

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

	accepts := request.Header["Accept"]
	fmt.Println("accepts", len(accepts))

	if len(accepts) == 0 {
		return false
	}

	for _, val := range accepts {

		fmt.Println("accepts", val)
		if strings.Contains(val, "text/html") {
			http.Redirect(writer, request, backUrl, http.StatusSeeOther)

			return true
		}
	}

	fmt.Println("oops")
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
