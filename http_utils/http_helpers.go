package http_utils

import (
	"net/http"
	"strconv"
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

func RedirectBackIfHTMLAccepted(writer http.ResponseWriter, request *http.Request) bool {
	accepts := request.Header["Accept"]

	if len(accepts) == 0 {
		return false
	}

	for _, val := range accepts {
		if val == "text/html" {
			http.Redirect(writer, request, request.Referer(), http.StatusSeeOther)

			return true
		}
	}

	return false
}
