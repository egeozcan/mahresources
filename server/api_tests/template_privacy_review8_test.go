package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/types"
)

// Classic MetaSchema over the real adapter: a meta string "false" is masked and
// the sample's HasURL:false (a JSON literal) does not stop the request (round 6, #1).
func TestPrivacyModeClassicMetaSchemaWithAFalseStringIsSent(t *testing.T) {
	f := newPrivacyFixture(t)
	cat := &models.Category{Name: "Flags READABLE_FLAGS"}
	if err := f.tc.DB.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.tc.DB.Create(&models.Group{Name: "Flagged Group", CategoryId: &cat.ID, Meta: types.JSON(`{"flag":"false"}`)}).Error; err != nil {
		t.Fatal(err)
	}
	f.script = func(n int, req map[string]any) map[string]any {
		return classicReply(`{"content":"{\"type\":\"object\",\"properties\":{\"flag\":{\"type\":\"string\"}}}","explanation":"x"}`)
	}
	f.install(t, "classic")
	path := "/v1/category/generateTemplate"
	body := map[string]any{"target": "metaschema", "mode": "json", "prompt": "READABLE_USER_REQUEST: schema", "categoryId": cat.ID}
	hash := disclose(t, f.tc, path, body)
	rec := f.tc.MakeRequest(http.MethodPost, path, withHash(body, hash))
	if rec.Code != http.StatusOK || f.calls != 1 {
		t.Fatalf("the request must be sent: %d calls=%d %.300s", rec.Code, f.calls, rec.Body.String())
	}
	if strings.Contains(f.allBodies(), `flag\":\"false`) {
		t.Fatal("the string value \"false\" must stay masked")
	}
}
