package api_handlers

import (
	"encoding/json"
	"github.com/gorilla/schema"
	"mahresources/constants"
	"mahresources/models/query_models"
	"net/http"
	"reflect"
	"strings"
)

var decoder = schema.NewDecoder()

func init() {
	decoder.IgnoreUnknownKeys(true)
	decoder.RegisterConverter(query_models.ColumnMeta{}, func(s string) reflect.Value {
		return reflect.ValueOf(query_models.ParseMeta(s))
	})
}

func tryFillStructValuesFromRequest(dst any, request *http.Request) error {
	contentTypeHeader := request.Header.Get("Content-type")

	if contentTypeHeader == constants.JSON {
		return json.NewDecoder(request.Body).Decode(dst)
	}

	if strings.HasPrefix(contentTypeHeader, constants.UrlEncodedForm) {
		if err := request.ParseForm(); err != nil {
			return err
		}
		return decoder.Decode(dst, request.PostForm)
	}

	if strings.HasPrefix(contentTypeHeader, constants.MultiPartForm) {
		if err := request.ParseMultipartForm(int64(4096) << 20); err != nil {
			return err
		}
		return decoder.Decode(dst, request.PostForm)
	}

	return decoder.Decode(dst, request.URL.Query())
}
