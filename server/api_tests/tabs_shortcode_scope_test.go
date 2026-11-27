package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"mahresources/models"
)

var lazyTokenPattern = regexp.MustCompile(`<lazy-shortcode data-token="([^"]+)"`)

// A lazy [tab] is a [lazy] block: the display page seals its body into a token
// and POST /v1/shortcodes/deferred renders it later, re-loading the entity
// under the caller's scope. A group-confined principal therefore gets the body
// for an entity inside its subtree and 404 for one outside it, even with a
// token the page really minted.
func TestTabsShortcode_LazyPanelIsScoped(t *testing.T) {
	tc := setupAuthEnv(t)
	mk := func(v any) {
		t.Helper()
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatalf("create %T: %v", v, err)
		}
	}
	cat := &models.Category{Name: "Tabs Cat", CustomHeader: `[tabs label="Views"][tab label="Overview"]eager-[property path="Name"][/tab][tab label="Later" lazy="true"]lazy-[property path="Name"][/tab][/tabs]`}
	mk(cat)
	scope := &models.Group{Name: "tabs-scope", CategoryId: &cat.ID}
	mk(scope)
	outside := &models.Group{Name: "tabs-outside-secret", CategoryId: &cat.ID}
	mk(outside)

	admin := roleBearer(t, tc, models.RoleAdmin)
	guest := scopedRoleBearer(t, tc, models.RoleGuest, scope.ID, "tabs-guest")

	// The token the page puts in the lazy tab; the page itself renders the
	// eager tab and only a placeholder for the lazy one.
	pageToken := func(bearer string, g *models.Group) string {
		t.Helper()
		page := getHTML(t, tc, bearer, fmt.Sprintf("/group?id=%d", g.ID))
		if !strings.Contains(page, `<tabs-shortcode class="tabs-shortcode"`) || !strings.Contains(page, "eager-"+g.Name) {
			t.Fatalf("page for %s does not render the tab set", g.Name)
		}
		if strings.Contains(page, "lazy-"+g.Name) {
			t.Fatalf("page for %s rendered the lazy tab inline", g.Name)
		}
		m := lazyTokenPattern.FindStringSubmatch(page)
		if m == nil {
			t.Fatalf("page for %s has no lazy tab placeholder", g.Name)
		}
		return html.UnescapeString(m[1])
	}
	deferred := func(bearer, token string) (int, string) {
		body, _ := json.Marshal(map[string]any{"token": token})
		headers := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}
		rr := doReq(tc, http.MethodPost, "/v1/shortcodes/deferred", headers, nil, bytes.NewReader(body))
		var resp deferredRenderResp
		_ = json.Unmarshal(rr.Body.Bytes(), &resp)
		return rr.Code, resp.HTML + rr.Body.String()
	}

	code, body := deferred(guest, pageToken(guest, scope))
	if code != http.StatusOK || !strings.Contains(body, "lazy-tabs-scope") {
		t.Errorf("guest, in-scope lazy tab: got %d %q, want 200 with the body", code, body)
	}

	outsideToken := pageToken(admin, outside)
	code, body = deferred(guest, outsideToken)
	if code != http.StatusNotFound || strings.Contains(body, "tabs-outside-secret") {
		t.Errorf("guest, out-of-subtree lazy tab: got %d %q, want 404 without the name", code, body)
	}
	if code, body := deferred(admin, outsideToken); code != http.StatusOK || !strings.Contains(body, "lazy-tabs-outside-secret") {
		t.Errorf("control: admin should render the outside lazy tab, got %d %q", code, body)
	}
}

func TestTabsShortcode_HoverCardStacksLabelledGroups(t *testing.T) {
	tc := setupAuthEnv(t)
	src := `[tabs label="Views"][tab label="Overview"]Overview body[/tab][tab label="Later" lazy="true"]Lazy body[/tab][/tabs]`
	cat := models.Category{Name: "Tooltip tabs", CustomHeader: src, CustomHoverCard: src}
	if err := tc.DB.Create(&cat).Error; err != nil {
		t.Fatal(err)
	}
	group := models.Group{Name: "Tooltip tabs group", CategoryId: &cat.ID}
	if err := tc.DB.Create(&group).Error; err != nil {
		t.Fatal(err)
	}
	admin := roleBearer(t, tc, models.RoleAdmin)
	card := getHTML(t, tc, admin, fmt.Sprintf("/hovercard?type=group&id=%d", group.ID))
	for _, want := range []string{`role="group" aria-label="Overview"`, `role="group" aria-label="Later"`, `>Overview</p>Overview body`, `<lazy-shortcode data-token=`} {
		if !strings.Contains(card, want) {
			t.Errorf("hover card missing %q: %s", want, card)
		}
	}
	for _, bad := range []string{`role="tablist"`, `role="tab"`, `<tabs-shortcode`} {
		if strings.Contains(card, bad) {
			t.Errorf("hover card exposes tab controls (%q): %s", bad, card)
		}
	}
	page := getHTML(t, tc, admin, fmt.Sprintf("/group?id=%d", group.ID))
	if !strings.Contains(page, `role="tablist" aria-label="Views"`) {
		t.Errorf("detail page did not keep the interactive tab set: %s", page)
	}
}
