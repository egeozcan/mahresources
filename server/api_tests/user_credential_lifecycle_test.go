package api_tests

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"

	"gorm.io/gorm"
)

func TestSelfPasswordChangeAlreadyStartedFailsAfterCompletedAdminReset(t *testing.T) {
	tc := setupAuthEnv(t)
	sqlDB, err := tc.DB.DB()
	if err != nil {
		t.Fatalf("database handle: %v", err)
	}
	sqlDB.SetMaxOpenConns(2)
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "self-admin-reset-race", Password: "password1", Role: models.RoleEditor,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	login := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json"}, nil,
		strings.NewReader(`{"username":"self-admin-reset-race","password":"password1"}`))
	cookie := sessionCookie(t, login)
	csrf := csrfFor(t, tc, cookie)

	selfAtMutation := make(chan struct{})
	releaseSelf := make(chan struct{})
	var first atomic.Bool
	callback := "test:pause_self_password_before_mutation_lock"
	if err := tc.DB.Callback().Raw().Before("gorm:raw").Register(callback, func(db *gorm.DB) {
		if !strings.Contains(db.Statement.SQL.String(), "UPDATE users SET id = id") || !first.CompareAndSwap(false, true) {
			return
		}
		close(selfAtMutation)
		<-releaseSelf
	}); err != nil {
		t.Fatalf("register mutation barrier: %v", err)
	}
	defer tc.DB.Callback().Raw().Remove(callback)

	result := make(chan int, 1)
	go func() {
		response := doReq(tc, http.MethodPost, "/v1/account/password", map[string]string{
			"Accept": "application/json", "Content-Type": "application/json", "X-CSRF-Token": csrf,
		}, []*http.Cookie{cookie}, strings.NewReader(`{"currentPassword":"password1","newPassword":"self-password"}`))
		result <- response.Code
	}()
	select {
	case <-selfAtMutation:
	case <-time.After(3 * time.Second):
		t.Fatal("self-password request did not reach the in-transaction mutation barrier")
	}
	if err := tc.AppCtx.SetUserPassword(user.ID, "admin-password"); err != nil {
		t.Fatalf("concurrent admin reset: %v", err)
	}
	close(releaseSelf)
	if status := <-result; status != http.StatusUnauthorized {
		t.Fatalf("stale self-password status=%d, want 401", status)
	}
	if _, err := tc.AppCtx.AuthenticateUser(user.Username, "admin-password"); err != nil {
		t.Fatalf("completed admin reset did not win: %v", err)
	}
}

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

func TestSelfPasswordChangeUsesRecordedCookieAuthenticationMechanism(t *testing.T) {
	for _, authorization := range []string{"bearer ignored", "  Bearer ignored  "} {
		t.Run(strings.ReplaceAll(authorization, " ", "_"), func(t *testing.T) {
			tc := setupAuthEnv(t)
			user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
				Username: "mechanism-user", Password: "password1", Role: models.RoleEditor,
			})
			if err != nil {
				t.Fatalf("create user: %v", err)
			}
			login := doReq(tc, http.MethodPost, "/v1/auth/login",
				map[string]string{"Content-Type": "application/json"}, nil,
				strings.NewReader(`{"username":"mechanism-user","password":"password1"}`))
			cookie := sessionCookie(t, login)
			headers := map[string]string{
				"Accept": "application/json", "Content-Type": "application/json",
				"X-CSRF-Token": csrfFor(t, tc, cookie), "Authorization": authorization,
			}
			changed := doReq(tc, http.MethodPost, "/v1/account/password", headers, []*http.Cookie{cookie},
				strings.NewReader(`{"currentPassword":"password1","newPassword":"new-password"}`))
			if changed.Code != http.StatusOK {
				t.Fatalf("password change status=%d body=%s", changed.Code, changed.Body.String())
			}
			if response := doReq(tc, http.MethodGet, "/v1/auth/me", map[string]string{"Accept": "application/json"}, []*http.Cookie{cookie}, nil); response.Code != http.StatusOK {
				t.Fatalf("cookie selected by middleware was revoked: status=%d body=%s user=%d", response.Code, response.Body.String(), user.ID)
			}
		})
	}
}
