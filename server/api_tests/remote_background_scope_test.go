package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// TestScopedUser_RemoteBackgroundConfined mirrors TestScopedUser_DownloadSubmitConfined
// for POST /v1/resource/remote?background=true: because the download worker
// creates on the unscoped system context (attribution binds only the actor id),
// a group-limited principal's background download must be confined to its subtree
// at enqueue time. Out-of-subtree owner/groups, a missing owner, and group
// creation via GroupName are all refused; an in-subtree target is accepted; and
// an admin is unrestricted.
func TestScopedUser_RemoteBackgroundConfined(t *testing.T) {
	tc := setupAuthEnv(t)
	root := &models.Group{Name: "rbg-root"}
	tc.DB.Create(root)
	outside := &models.Group{Name: "rbg-outside"}
	tc.DB.Create(outside)
	bearer := scopedUserBearer(t, tc, root.ID)
	h := map[string]string{"Content-Type": "application/json", "Authorization": bearer}

	const path = "/v1/resource/remote?background=true"
	post := func(body string) int {
		return doReq(tc, http.MethodPost, path, h, nil, strings.NewReader(body)).Code
	}

	if c := post(fmt.Sprintf(`{"URL":"http://example.com/a","OwnerId":%d}`, outside.ID)); c != http.StatusForbidden {
		t.Fatalf("out-of-subtree owner should be 403, got %d", c)
	}
	if c := post(`{"URL":"http://example.com/a"}`); c != http.StatusForbidden {
		t.Fatalf("missing owner should be 403 for a scoped user, got %d", c)
	}
	if c := post(`{"URL":"http://example.com/a","GroupName":"brand-new"}`); c != http.StatusForbidden {
		t.Fatalf("group creation via GroupName should be 403 for a scoped user, got %d", c)
	}
	if c := post(fmt.Sprintf(`{"URL":"http://example.com/a","Groups":[%d]}`, outside.ID)); c != http.StatusForbidden {
		t.Fatalf("out-of-subtree attached group should be 403, got %d", c)
	}
	// In-subtree owner is accepted (enqueued, not scope-refused).
	if c := post(fmt.Sprintf(`{"URL":"http://example.com/a","OwnerId":%d}`, root.ID)); c == http.StatusForbidden {
		t.Fatalf("in-subtree owner should not be 403, got %d", c)
	}

	// An admin is unrestricted: an out-of-subtree owner is fine.
	adminBearer := roleBearer(t, tc, models.RoleAdmin)
	ah := map[string]string{"Content-Type": "application/json", "Authorization": adminBearer}
	if c := doReq(tc, http.MethodPost, path, ah, nil,
		strings.NewReader(fmt.Sprintf(`{"URL":"http://example.com/a","OwnerId":%d}`, outside.ID))).Code; c == http.StatusForbidden {
		t.Fatalf("admin background download should not be scope-restricted, got %d", c)
	}
}
