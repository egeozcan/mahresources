package api_tests

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

const metaSchemaSubmit = `{"type":"object","properties":{"title":{"type":"string"}}}`

// Tool-owned JSON keys and preserved enum leaves are vocabulary. A sample named
// "Warnings" next to lint's "warnings" key, and one named "Baseline" next to a
// revision whose source is "baseline", must not stop a MetaSchema run (round 7, #1).
func TestPrivacyModeToolKeysAndEnumLeavesAreReadable(t *testing.T) {
	f := newPrivacyFixture(t)
	cat := &models.Category{Name: "Words READABLE_WORDS"}
	if err := f.tc.DB.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Warnings", "Baseline"} {
		if err := f.tc.DB.Create(&models.Group{Name: name, CategoryId: &cat.ID}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := f.tc.DB.Create(&models.TemplateRevision{CarrierType: models.TemplateCarrierCategory, CarrierID: cat.ID, Number: 1, Source: "baseline",
		Snapshot: `{"CustomHeader":""}`, Applied: `[]`, ChangedSlots: `[]`}).Error; err != nil {
		t.Fatal(err)
	}
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return chatToolCalls(
				call("lt", "lint", map[string]any{"field": "MetaSchema", "content": metaSchemaSubmit}),
				call("lr", "list_revisions", map[string]any{"carrierType": "category", "id": cat.ID}),
			)
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"MetaSchema": metaSchemaSubmit}, "done": true}))
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/category/generateTemplate", map[string]any{"target": "metaschema", "slot": "", "mode": "json", "categoryId": cat.ID, "entityId": 0})
	if res.code != http.StatusOK || f.calls != 2 {
		t.Fatalf("a Tool's own key or enum equal to a sample name stopped the run: %d calls=%d %.300s", res.code, f.calls, res.body)
	}
	seed := string(f.bodies[0])
	if strings.Contains(seed, `\"Name\":\"Warnings\"`) || strings.Contains(seed, `\"Name\":\"Baseline\"`) {
		t.Fatal("the sample names must stay masked in the Seed")
	}
	if !strings.Contains(f.toolResults()["lt"], `warnings`) || !strings.Contains(f.toolResults()["lr"], `baseline`) {
		t.Fatalf("the Tools' own words are sent: %s | %s", f.toolResults()["lt"], f.toolResults()["lr"])
	}
}

// SQL expression columns (meta.region) come back as pointer cells. Bucket keys
// are masked, repeated renders through the real cache give stable Placeholders,
// and the cached raw input is unchanged (round 7, #2).
func TestPrivacyModeBucketKeysFromSQLExpressionsAreMasked(t *testing.T) {
	f := newPrivacyFixture(t)
	// The preview caps a query at five rows, so every group carries a region:
	// odd IDs EAST, even IDs WEST.
	if err := f.tc.DB.Exec(`UPDATE groups SET meta = '{"region":"PROT_REGION_EAST"}' WHERE id % 2 = 1`).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.tc.DB.Exec(`UPDATE groups SET meta = '{"region":"PROT_REGION_WEST"}' WHERE id % 2 = 0`).Error; err != nil {
		t.Fatal(err)
	}
	query := `type = group GROUP BY meta.region`
	both := `[mrql query='` + query + `'][mrql query='` + query + `']`
	f.script = func(n int, req map[string]any) map[string]any {
		if n == 1 {
			return chatToolCalls(call("b1", "preview", map[string]any{"field": "CustomHeader", "content": both, "css": both, "entityType": "group", "entityId": f.child.ID}))
		}
		return chatToolCalls(call("s1", "submit", map[string]any{"fields": map[string]string{"CustomCSS": ".a{}"}, "done": true}))
	}
	f.install(t, "harness")
	res := f.generate(t, "/v1/category/generateTemplate", nil)
	if res.code != http.StatusOK {
		t.Fatalf("%d %.300s", res.code, res.body)
	}
	out := f.toolResults()["b1"]
	if strings.Contains(f.allBodies(), "PROT_REGION") {
		t.Fatal("a bucket key from an SQL expression column reached the provider")
	}
	if strings.Contains(out, "0xc0") || strings.Contains(out, "mrql-error") {
		t.Fatalf("the bucket header must not print a pointer or an error: %s", out)
	}
	var parsed struct {
		Legend []struct{ Value, Placeholder string }
	}
	if err := json.Unmarshal(res.body, &parsed); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, e := range parsed.Legend {
		if e.Value == "PROT_REGION_EAST" {
			found = strings.Count(out, e.Placeholder) >= 4 // two renders, in HTML and in CSS
		}
	}
	if !found {
		t.Fatalf("the region's Placeholder must appear in every render and be legend-backed: %s", out)
	}
}

// The overflow answer is exactly the fixed text (round 7, #4).
func TestPrivacyModeSeedOverflowIsExactlyTheFixedText(t *testing.T) {
	f := newPrivacyFixture(t)
	f.script = harnessScript(f)
	f.install(t, "harness")
	if err := f.tc.DB.Model(&models.Group{}).Where("id = ?", f.child.ID).Update("description", strings.Repeat("x", 300<<10)).Error; err != nil {
		t.Fatal(err)
	}
	rec := f.tc.MakeRequest(http.MethodPost, "/v1/category/generateTemplate", f.requestBody(map[string]any{"dryRun": true}))
	var body struct{ Error string }
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if rec.Code != http.StatusBadRequest || body.Error != "too much entity data for Privacy mode" {
		t.Fatalf("%d %q", rec.Code, body.Error)
	}
}
