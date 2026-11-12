package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// Merging a group that is some account's scope moves that account's
// scope_group_id onto the winner. Assigning a scope is admin-only, so only an
// admin (or the system) may cause that move; every other caller gets the same
// 409 a delete of that group already answers.
func TestMergeGroupsHTTP_UserScopeLoserIsAdminOnly(t *testing.T) {
	cases := []struct {
		name       string
		bearer     func(t *testing.T, tc *TestContext, rootID uint) string
		wantStatus int
		wantScope  func(rootID, childID uint) uint
	}{
		{
			name:       "scoped user",
			bearer:     func(t *testing.T, tc *TestContext, rootID uint) string { return scopedUserBearer(t, tc, rootID) },
			wantStatus: http.StatusConflict,
			wantScope:  func(_, childID uint) uint { return childID },
		},
		{
			name:       "editor",
			bearer:     func(t *testing.T, tc *TestContext, _ uint) string { return roleBearer(t, tc, models.RoleEditor) },
			wantStatus: http.StatusConflict,
			wantScope:  func(_, childID uint) uint { return childID },
		},
		{
			name:       "admin",
			bearer:     func(t *testing.T, tc *TestContext, _ uint) string { return roleBearer(t, tc, models.RoleAdmin) },
			wantStatus: http.StatusOK,
			wantScope:  func(rootID, _ uint) uint { return rootID },
		},
		{
			// Unchanged: the path policy refuses a guest any write.
			name:       "guest",
			bearer:     func(t *testing.T, tc *TestContext, _ uint) string { return roleBearer(t, tc, models.RoleGuest) },
			wantStatus: http.StatusForbidden,
			wantScope:  func(_, childID uint) uint { return childID },
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc := setupAuthEnv(t)
			root := &models.Group{Name: "merge-scope-root"}
			if err := tc.DB.Create(root).Error; err != nil {
				t.Fatalf("create root: %v", err)
			}
			child := &models.Group{Name: "merge-scope-child", OwnerId: &root.ID}
			if err := tc.DB.Create(child).Error; err != nil {
				t.Fatalf("create child: %v", err)
			}
			victim, err := tc.AppCtx.CreateUser(&application_context.UserInput{
				Username: "merge-victim", Password: "password1", Role: models.RoleGuest, ScopeGroupId: &child.ID,
			})
			if err != nil {
				t.Fatalf("create victim: %v", err)
			}

			body := fmt.Sprintf(`{"Winner":%d,"Losers":[%d]}`, root.ID, child.ID)
			rr := doReq(tc, http.MethodPost, "/v1/groups/merge", map[string]string{
				"Accept":        "application/json",
				"Content-Type":  "application/json",
				"Authorization": c.bearer(t, tc, root.ID),
			}, nil, strings.NewReader(body))
			if rr.Code != c.wantStatus {
				t.Fatalf("POST /v1/groups/merge as %s: status %d, want %d (body %s)", c.name, rr.Code, c.wantStatus, rr.Body.String())
			}

			var stored models.User
			if err := tc.DB.First(&stored, victim.ID).Error; err != nil {
				t.Fatalf("reload victim: %v", err)
			}
			want := c.wantScope(root.ID, child.ID)
			if stored.ScopeGroupId == nil || *stored.ScopeGroupId != want {
				t.Fatalf("victim scope_group_id = %v, want %d", stored.ScopeGroupId, want)
			}

			var childCount int64
			tc.DB.Model(&models.Group{}).Where("id = ?", child.ID).Count(&childCount)
			if wantGone := c.wantStatus == http.StatusOK; wantGone != (childCount == 0) {
				t.Fatalf("child group count = %d after a %d merge", childCount, rr.Code)
			}
		})
	}
}
