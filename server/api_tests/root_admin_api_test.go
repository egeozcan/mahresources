package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

// bearerForUser mints an API token for an existing user id.
func bearerForUser(t *testing.T, tc *TestContext, userID uint) string {
	t.Helper()
	raw, _, err := tc.AppCtx.CreateApiToken(userID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return "Bearer " + raw
}

// TestLastAdmin_APIReturns409 verifies the HTTP layer maps ErrLastAdmin to 409
// for both delete and demote of the sole admin, and that a normal delete of a
// non-last user still succeeds (200).
func TestLastAdmin_APIReturns409(t *testing.T) {
	tc := setupAuthEnv(t)
	admin, err := tc.AppCtx.GetUserByUsername("admin")
	if err != nil {
		t.Fatalf("load bootstrap admin: %v", err)
	}
	adminBearer := bearerForUser(t, tc, admin.ID)
	hdr := map[string]string{"Accept": "application/json", "Authorization": adminBearer}

	// Delete the sole admin → 409.
	rr := doReq(tc, http.MethodPost, fmt.Sprintf("/v1/user/delete?id=%d", admin.ID), hdr, nil, nil)
	if rr.Code != http.StatusConflict {
		t.Fatalf("delete sole admin: want 409, got %d body=%s", rr.Code, rr.Body.String())
	}

	// Demote the sole admin → 409.
	jsonHdr := map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": adminBearer}
	demote := fmt.Sprintf(`{"id":%d,"username":"admin","role":"editor"}`, admin.ID)
	rr = doReq(tc, http.MethodPost, "/v1/user", jsonHdr, nil, strings.NewReader(demote))
	if rr.Code != http.StatusConflict {
		t.Fatalf("demote sole admin: want 409, got %d body=%s", rr.Code, rr.Body.String())
	}

	// A normal, non-last-admin delete still succeeds.
	victim, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: "victim", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create victim: %v", err)
	}
	rr = doReq(tc, http.MethodPost, fmt.Sprintf("/v1/user/delete?id=%d", victim.ID), hdr, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete non-last user: want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

// TestNoAuthMeReportsRootIdentity verifies that under no-auth, /v1/auth/me reports
// the root user's id/username/role (built from RootAdminPrincipal), while keeping
// superUser=true.
func TestNoAuthMeReportsRootIdentity(t *testing.T) {
	tc := SetupTestEnv(t) // auth disabled
	root, err := tc.AppCtx.CreateUser(&application_context.UserInput{Username: "boss", Password: "password1", Role: models.RoleAdmin})
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	rr := doReq(tc, http.MethodGet, "/v1/auth/me", map[string]string{"Accept": "application/json"}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("/v1/auth/me: status %d body=%s", rr.Code, rr.Body.String())
	}
	var me map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &me); err != nil {
		t.Fatalf("parse me: %v", err)
	}
	if uid, _ := me["userId"].(float64); uint(uid) != root.ID {
		t.Errorf("userId=%v, want root %d", me["userId"], root.ID)
	}
	if me["username"] != "boss" {
		t.Errorf("username=%v, want 'boss'", me["username"])
	}
	if me["role"] != string(models.RoleAdmin) {
		t.Errorf("role=%v, want admin", me["role"])
	}
	if su, _ := me["superUser"].(bool); !su {
		t.Error("no-auth principal must keep superUser=true")
	}
}
