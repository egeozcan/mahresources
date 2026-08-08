package api_tests

import (
	"net/http"
	"strings"
	"testing"

	"mahresources/application_context"
	"mahresources/models"
)

func TestSelfPasswordChangePreservesCurrentSessionOnly(t *testing.T) {
	tc := setupAuthEnv(t)
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "lifecycle", Password: "password1", Role: models.RoleEditor,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	login := func() *http.Cookie {
		response := doReq(tc, http.MethodPost, "/v1/auth/login",
			map[string]string{"Content-Type": "application/json"}, nil,
			strings.NewReader(`{"username":"lifecycle","password":"password1"}`))
		if response.Code != http.StatusOK {
			t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
		}
		return sessionCookie(t, response)
	}
	current := login()
	peer := login()
	csrf := csrfFor(t, tc, current)
	headers := map[string]string{
		"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": csrf,
	}

	mint := doReq(tc, http.MethodPost, "/v1/account/tokens", headers, []*http.Cookie{current}, strings.NewReader(`{"name":"cli"}`))
	if mint.Code != http.StatusOK {
		t.Fatalf("mint token status=%d body=%s", mint.Code, mint.Body.String())
	}
	token := extractJSONString(mint.Body.String(), "token")

	change := doReq(tc, http.MethodPost, "/v1/account/password", headers, []*http.Cookie{current},
		strings.NewReader(`{"currentPassword":"password1","newPassword":"new-password"}`))
	if change.Code != http.StatusOK {
		t.Fatalf("password change status=%d body=%s", change.Code, change.Body.String())
	}
	if response := doReq(tc, http.MethodGet, "/v1/auth/me", map[string]string{"Accept": "application/json"}, []*http.Cookie{current}, nil); response.Code != http.StatusOK {
		t.Fatalf("current session status=%d body=%s", response.Code, response.Body.String())
	}
	if response := doReq(tc, http.MethodGet, "/v1/auth/me", map[string]string{"Accept": "application/json"}, []*http.Cookie{peer}, nil); response.Code != http.StatusUnauthorized {
		t.Fatalf("peer session status=%d, want 401; body=%s", response.Code, response.Body.String())
	}
	if response := doReq(tc, http.MethodGet, "/v1/auth/me", map[string]string{
		"Accept": "application/json", "Authorization": "Bearer " + token,
	}, nil, nil); response.Code != http.StatusOK {
		t.Fatalf("API token status=%d body=%s", response.Code, response.Body.String())
	}
}
