package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// A [reload] slot region is replaced when it reloads, so the stylesheet an
// [mrql] result inside it emits must not suppress the one a card after it
// needs: the owned-group card for the same category carries its own.
func TestReloadRegionStylesheetDoesNotSuppressALaterCard(t *testing.T) {
	tc := SetupTestEnv(t)

	styled := &models.Category{Name: "CSS Region Styled", CustomMRQLResult: `<i class="rr">[property path="Name"]</i>`, CustomCSS: `.rr { color: red; }`}
	if err := tc.DB.Create(styled).Error; err != nil {
		t.Fatal(err)
	}
	host := &models.Category{Name: "CSS Region Host", CustomHeader: `[mrql query='type = group AND name = "CSS Region Child"'][reload label="Refresh"]`}
	if err := tc.DB.Create(host).Error; err != nil {
		t.Fatal(err)
	}
	g := &models.Group{Name: "CSS Region Owner", CategoryId: &host.ID}
	if err := tc.DB.Create(g).Error; err != nil {
		t.Fatal(err)
	}
	child := &models.Group{Name: "CSS Region Child", CategoryId: &styled.ID, OwnerId: &g.ID}
	if err := tc.DB.Create(child).Error; err != nil {
		t.Fatal(err)
	}

	rr := doReq(tc, http.MethodGet, "/group?id="+itoa(int(g.ID)), map[string]string{"Accept": "text/html"}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /group: %d", rr.Code)
	}
	body := rr.Body.String()
	marker := `data-mr-custom-css="group:` + itoa(int(styled.ID)) + `"`
	start := strings.Index(body, `data-shortcode-region="`)
	if start < 0 || !strings.Contains(body, `class="rr"`) {
		t.Fatalf("want a reloadable region holding the styled result; body has region=%v", start >= 0)
	}
	if n := strings.Count(body, marker); n < 2 {
		t.Fatalf("stylesheet blocks for the styled category = %d, want one in the region and one for the card", n)
	}
}
