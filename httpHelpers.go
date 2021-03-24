package main

import (
	"net/http"
	"strconv"
)

func getQueryParameter(request *http.Request, paramName string, defVal string) string {
	paramFromRes := request.URL.Query().Get(paramName)

	if paramFromRes != "" {
		return paramFromRes
	}

	return defVal
}

func getIntQueryParameter(request *http.Request, paramName string, defVal int64) int64 {
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

func getFormParameter(request *http.Request, paramName string, defVal string) string {
	paramFromRes := request.PostForm.Get(paramName)

	if paramFromRes != "" {
		return paramFromRes
	}

	return defVal
}

func getIntFormParameter(request *http.Request, paramName string, defVal int64) int64 {
	paramFromRes := getFormParameter(request, paramName, "")

	if paramFromRes == "" {
		return defVal
	}

	param, err := strconv.ParseInt(paramFromRes, 10, 0)

	if err != nil {
		return defVal
	}

	return param
}
