package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"mahresources/models"
)

const (
	contextAction = `[action plugin="tpl-act" name="greet"]`
	contextCSS    = `form[action]{margin-block:1rem}.w[tabs]{color:red}`
)

func TestStylesheetShortcodesPreserveAttributeSelectorsOnPageAndLightbox(t *testing.T) {
	tc := SetupTestEnv(t)
	category := &models.ResourceCategory{Name: "Selector CSS", CustomCSS: contextCSS + `/* ` + contextAction + ` */`}
	if err := tc.DB.Create(category).Error; err != nil {
		t.Fatal(err)
	}
	resource := &models.Resource{Name: "Selector CSS resource", ContentType: "image/png", ResourceCategoryId: category.ID}
	if err := tc.DB.Create(resource).Error; err != nil {
		t.Fatal(err)
	}
	want := contextCSS + "/* " + actionUnavailable + " */"
	page := getHTML(t, tc, "", fmt.Sprintf("/resource?id=%d", resource.ID))
	style := fmt.Sprintf(`<style data-mr-custom-css="resource:%d">%s</style>`, category.ID, want)
	if !strings.Contains(page, style) {
		actual := regexp.MustCompile(`<style data-mr-custom-css="resource:\d+">.*?</style>`).FindString(page)
		t.Errorf("detail page stylesheet: got %q, want %q", actual, style)
	}

	rr := doReq(tc, http.MethodGet, fmt.Sprintf("/resource.json?id=%d", resource.ID),
		map[string]string{"Accept": "application/json"}, nil, nil)
	var response struct {
		Resource struct {
			ResourceCategory struct{ CustomCSS string } `json:"resourceCategory"`
		} `json:"resource"`
	}
	if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &response) != nil {
		t.Fatalf("lightbox JSON: %d %s", rr.Code, rr.Body.String())
	}
	if css := response.Resource.ResourceCategory.CustomCSS; css != want {
		t.Errorf("lightbox stylesheet: got %q, want %q", css, want)
	}
}

func TestInlineMRQLStylesheetMasksActionResolverOnlyForCSS(t *testing.T) {
	tc := setupPluginEnv(t, false, writeTemplateActionPlugin, "tpl-act")
	f := buildActionFixture(t, tc)
	if err := tc.DB.Model(f.category).Updates(map[string]any{
		"custom_css":         contextCSS + `/* ` + contextAction + ` */`,
		"custom_mrql_result": `<div class="inline-result-action">` + contextAction + `</div>`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	cat := &models.Category{Name: "Inline Action CSS", CustomSidebar: fmt.Sprintf(`[mrql query="type = resource AND id = %d"]`, f.png.ID)}
	if err := tc.DB.Create(cat).Error; err != nil {
		t.Fatal(err)
	}
	if err := tc.DB.Model(f.scope).Update("category_id", cat.ID).Error; err != nil {
		t.Fatal(err)
	}
	page := getHTML(t, tc, "", fmt.Sprintf("/group?id=%d", f.scope.ID))
	if !strings.Contains(page, `<div class="inline-result-action">`+greetButton(f.png.ID)) {
		t.Fatal("inline MRQL custom result card did not render its action button")
	}
	style := regexp.MustCompile(`<style data-mr-custom-css="resource:\d+">(.*?)</style>`).FindStringSubmatch(page)
	if len(style) != 2 {
		t.Fatal("inline MRQL did not emit the category's stylesheet")
	}
	if got, want := style[1], contextCSS+"/* "+actionUnavailable+" */"; got != want {
		t.Errorf("inline MRQL stylesheet: got %q, want %q", got, want)
	}
}

func TestHoverCardDeferredAndRefreshTokensRetainTooltip(t *testing.T) {
	tc := setupPluginEnv(t, true, writeTemplateActionPlugin, "tpl-act")
	f := buildActionFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)
	// The second [lazy] exercises re-sealing by the deferred endpoint itself.
	body := `[lazy]` + contextAction + `[/lazy][lazy][lazy]` + contextAction + `[/lazy][/lazy]` +
		`[autorefresh]` + contextAction + `[/autorefresh]`
	if err := tc.DB.Model(f.category).Updates(map[string]any{"custom_hover_card": body, "custom_sidebar": body}).Error; err != nil {
		t.Fatal(err)
	}
	for _, surface := range []struct {
		name, path string
		tooltip    bool
	}{
		{"hovercard", fmt.Sprintf("/hovercard?type=resource&id=%d", f.png.ID), true},
		{"detail", fmt.Sprintf("/resource?id=%d", f.png.ID), false},
	} {
		t.Run(surface.name, func(t *testing.T) {
			page := getHTML(t, tc, admin, surface.path)
			for _, block := range []struct {
				name, tag string
				index     int
			}{
				{"lazy", "lazy", 0}, {"nested lazy", "lazy", 1}, {"autorefresh", "autorefresh", 0},
			} {
				t.Run(block.name, func(t *testing.T) {
					pattern := regexp.MustCompile(`<` + block.tag + `-shortcode data-token="([^"]+)"`)
					matches := pattern.FindAllStringSubmatch(page, -1)
					if len(matches) <= block.index {
						t.Fatal("initial render did not emit the required token")
					}
					body := postDeferredToken(t, tc, admin, matches[block.index][1], http.StatusOK)
					if block.index == 1 {
						match := pattern.FindStringSubmatch(body)
						if len(match) != 2 {
							t.Fatalf("deferred body did not emit a nested token: %q", body)
						}
						body = postDeferredToken(t, tc, admin, match[1], http.StatusOK)
					}
					if surface.tooltip && body != actionUnavailable {
						t.Errorf("tooltip round-trip: got %q, want %q", body, actionUnavailable)
					}
					if !surface.tooltip && !strings.HasPrefix(body, greetButton(f.png.ID)) {
						t.Errorf("detail control did not render a button: %q", body)
					}
				})
			}
		})
	}
}

func TestTemplatePreviewPreservesStylesheetAttributeSelectors(t *testing.T) {
	tc := SetupTestEnv(t)
	resource := &models.Resource{Name: "Preview CSS resource", ContentType: "image/png"}
	if err := tc.DB.Create(resource).Error; err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{
		"entityId": resource.ID, "slot": "CustomCSS", "content": contextCSS, "css": contextCSS,
	})
	rr := doReq(tc, http.MethodPost, "/v1/resourceCategory/previewTemplate",
		map[string]string{"Accept": "application/json", "Content-Type": "application/json"}, nil, bytes.NewReader(payload))
	var response struct{ HTML, CSS string }
	if rr.Code != http.StatusOK || json.Unmarshal(rr.Body.Bytes(), &response) != nil {
		t.Fatalf("template preview: %d %s", rr.Code, rr.Body.String())
	}
	if response.HTML != contextCSS || response.CSS != contextCSS {
		t.Errorf("preview stylesheets: html=%q css=%q, want both %q", response.HTML, response.CSS, contextCSS)
	}
}
