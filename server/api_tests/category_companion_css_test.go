package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"mahresources/models"
)

func TestCategoryCompanionCSSPersistence(t *testing.T) {
	for _, form := range []bool{false, true} {
		t.Run(fmt.Sprintf("form=%t", form), func(t *testing.T) {
			tc := SetupTestEnv(t)
			fields := []string{}
			typ := reflect.TypeOf(models.Category{})
			for i := 0; i < typ.NumField(); i++ {
				name := typ.Field(i).Name
				if name != "CustomCSS" && strings.HasPrefix(name, "Custom") && strings.HasSuffix(name, "CSS") {
					fields = append(fields, name)
				}
			}
			require.NotEmpty(t, fields)
			send := func(values map[string]any) models.Category {
				var body []byte
				if form {
					valuesForm := url.Values{}
					for key, value := range values {
						valuesForm.Set(key, fmt.Sprint(value))
					}
					resp := tc.MakeFormRequest(http.MethodPost, "/v1/category", valuesForm)
					require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
					body = resp.Body.Bytes()
				} else {
					resp := tc.MakeRequest(http.MethodPost, "/v1/category", values)
					require.Equal(t, http.StatusOK, resp.Code, resp.Body.String())
					body = resp.Body.Bytes()
				}
				var category models.Category
				require.NoError(t, json.Unmarshal(body, &category))
				require.NoError(t, tc.DB.First(&category, category.ID).Error)
				return category
			}
			values := map[string]any{"Name": "Companion CSS"}
			for _, field := range fields {
				values[field] = ".created{color:red}"
			}
			created := send(values)
			check := func(category models.Category, want string) {
				for _, field := range fields {
					require.Equal(t, want, reflect.ValueOf(category).FieldByName(field).String(), field)
				}
			}
			check(created, ".created{color:red}")
			values = map[string]any{"ID": created.ID}
			for _, field := range fields {
				values[field] = ".updated{color:blue}"
			}
			check(send(values), ".updated{color:blue}")
			check(send(map[string]any{"ID": created.ID, "Name": "Rename without CSS"}), ".updated{color:blue}")
			for _, field := range fields {
				values[field] = ""
			}
			check(send(values), "")
		})
	}
}
