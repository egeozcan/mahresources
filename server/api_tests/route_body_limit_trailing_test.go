package api_tests

import (
	"net/http"
	"strconv"
	"strings"
	"testing"

	"mahresources/models"
)

// A JSON value inside the limit followed by whitespace beyond it must not commit: the
// decoder stops at the end of the value and never read the rest, so the limit capped
// nothing.
func TestRouteBodyLimitRejectsTrailingBytesBeyondTheLimit(t *testing.T) {
	cases := []struct {
		path string
		body func(id uint) string
	}{
		{"/v1/resources/delete", func(id uint) string { return `{"ID":[` + trailingID(id) + `]}` + strings.Repeat(" ", 1<<20+1) }},
		{"/v1/resources/merge", func(id uint) string {
			return `{"Winner":` + trailingID(id+1) + `,"Losers":[` + trailingID(id) + `]}` + strings.Repeat("\n", 256<<10+1)
		}},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			tc := SetupTestEnv(t)
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
				t.Fatalf("want a refusal, got %d (%s)", rr.Code, rr.Body.String())
			}
			var n int64
			if err := tc.DB.Model(&models.Resource{}).Where("id IN ?", []uint{a.ID, b.ID}).Count(&n).Error; err != nil {
				t.Fatal(err)
			}
			if n != 2 {
				t.Fatalf("the request acted although its body was over the limit: %d of 2 resources left", n)
			}
		})
	}
}

func trailingID(id uint) string { return strconv.Itoa(int(id)) }
