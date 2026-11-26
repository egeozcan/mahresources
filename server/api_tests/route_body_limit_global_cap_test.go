package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// With a global JSON cap configured, the route limit still bounds the bytes after the
// value: the trailing-data check must not depend on which wrapper is outermost.
func TestRouteBodyLimitRejectsTrailingBytesWithAGlobalJSONCap(t *testing.T) {
	for _, globalCap := range []int64{64 << 20, 600 << 10} {
		for _, c := range []struct {
			path string
			body func(id uint) string
		}{
			{"/v1/resources/delete", func(id uint) string { return `{"ID":[` + trailingID(id) + `]}` + strings.Repeat(" ", 1<<20+1) }},
			{"/v1/resources/merge", func(id uint) string {
				return `{"Winner":` + trailingID(id+1) + `,"Losers":[` + trailingID(id) + `]}` + strings.Repeat("\n", 256<<10+1)
			}},
		} {
			jsonCap := globalCap
			t.Run(c.path, func(t *testing.T) {
				tc := setupTestEnvWithConfig(t, func(cfg *application_context.MahresourcesConfig) { cfg.MaxJSONBodySize = jsonCap })
				a := &models.Resource{Name: "a", Hash: "ha", Location: "/a"}
				b := &models.Resource{Name: "b", Hash: "hb", Location: "/b"}
				if err := tc.DB.Create(a).Error; err != nil {
					t.Fatal(err)
				}
				if err := tc.DB.Create(b).Error; err != nil {
					t.Fatal(err)
				}
				rr := doReq(tc, http.MethodPost, c.path,
					map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, nil, strings.NewReader(c.body(a.ID)))
				if rr.Code < 400 {
					t.Fatalf("global cap %d: want a refusal, got %d (%s)", jsonCap, rr.Code, rr.Body.String())
				}
				var n int64
				if err := tc.DB.Model(&models.Resource{}).Where("id IN ?", []uint{a.ID, b.ID}).Count(&n).Error; err != nil {
					t.Fatal(err)
				}
				if n != 2 {
					t.Fatalf("global cap %d: the request acted although its body was over the route limit: %d of 2 resources left", jsonCap, n)
				}
			})
		}
	}
}
