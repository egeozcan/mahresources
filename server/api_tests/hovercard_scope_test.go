package api_tests

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func hcUintStr(u uint) string { return strconv.FormatUint(uint64(u), 10) }

// The /hovercard preview fragment must be fail-closed for a group-limited
// principal: hovering a link to an entity outside its subtree yields the
// "Preview unavailable" fragment, never a leaked preview. In-subtree entities
// preview normally. (Phase 6 item 3.)
func TestHoverCard_ScopedPrincipal_FailClosed(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildScopingFixture(t, tc)
	h := map[string]string{"Authorization": f.bearer}

	unavailable := func(path string) bool {
		body := doReq(tc, http.MethodGet, path, h, nil, nil).Body.String()
		return strings.Contains(body, "Preview unavailable")
	}
	available := func(path, wantText string) bool {
		body := doReq(tc, http.MethodGet, path, h, nil, nil).Body.String()
		return strings.Contains(body, "hovercard-card") && strings.Contains(body, wantText)
	}

	// In-subtree group/child previews render.
	if !available("/hovercard?type=group&id="+hcUintStr(f.childID), "sf-child") {
		t.Error("in-subtree group should preview")
	}
	// Out-of-subtree entities are fail-closed.
	if !unavailable("/hovercard?type=group&id=" + hcUintStr(f.outsideID)) {
		t.Error("out-of-subtree group must be unavailable")
	}
	if !unavailable("/hovercard?type=resource&id=" + hcUintStr(f.rOutID)) {
		t.Error("out-of-subtree resource must be unavailable")
	}
	if !unavailable("/hovercard?type=note&id=" + hcUintStr(f.nOutID)) {
		t.Error("out-of-subtree note must be unavailable")
	}
	// In-subtree resource/note preview.
	if !available("/hovercard?type=resource&id="+hcUintStr(f.rInID), "sf-rIn") {
		t.Error("in-subtree resource should preview")
	}
	if !available("/hovercard?type=note&id="+hcUintStr(f.nInID), "sf-nIn") {
		t.Error("in-subtree note should preview")
	}
}
