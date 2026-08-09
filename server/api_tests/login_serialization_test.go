package api_tests

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"

	"gorm.io/gorm"
)

// The login handler's serialization guarantee lives in one line of wiring:
// startSession must call AuthenticateAndCreateSession rather than composing
// AuthenticateUser with CreateSession by hand. That composition compiles, reads
// as equivalent, and reopens the credential race for every real browser login,
// while every context-layer test keeps passing because it calls the combined
// method directly. This drives the actual HTTP route so the wiring itself is
// covered.
func TestLoginRejectsCredentialSupersededMidRequest(t *testing.T) {
	tc := setupAuthEnv(t)
	sqlDB, err := tc.DB.DB()
	if err != nil {
		t.Fatalf("database handle: %v", err)
	}
	// The paused login holds one connection while the reset needs another.
	sqlDB.SetMaxOpenConns(2)
	user, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "login-superseded", Password: "password1", Role: models.RoleEditor,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	credentialRead := make(chan struct{})
	releaseLogin := make(chan struct{})
	var paused atomic.Bool
	var released atomic.Bool
	release := func() {
		if released.CompareAndSwap(false, true) {
			close(releaseLogin)
		}
	}
	t.Cleanup(release)

	callback := "test:pause_login_after_credential_read"
	if err := tc.DB.Callback().Query().After("gorm:query").Register(callback, func(db *gorm.DB) {
		if db.Statement.Table != "users" ||
			!strings.Contains(db.Statement.SQL.String(), "username") ||
			!paused.CompareAndSwap(false, true) {
			return
		}
		close(credentialRead)
		<-releaseLogin
	}); err != nil {
		t.Fatalf("register credential-read barrier: %v", err)
	}
	t.Cleanup(func() { _ = tc.DB.Callback().Query().Remove(callback) })

	result := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		result <- doReq(tc, http.MethodPost, "/v1/auth/login",
			map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, nil,
			strings.NewReader(`{"username":"login-superseded","password":"password1"}`))
	}()
	select {
	case <-credentialRead:
	case <-time.After(5 * time.Second):
		t.Fatal("login request never read the stored credential")
	}

	if err := tc.AppCtx.SetUserPassword(user.ID, "password2"); err != nil {
		t.Fatalf("administrative reset while the login was in flight: %v", err)
	}
	release()

	response := <-result
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("login against a password superseded mid-request status=%d body=%s, want 401",
			response.Code, response.Body.String())
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == tc.AppCtx.SessionCookieName() && cookie.Value != "" {
			t.Fatalf("refused login still issued a session cookie %q", cookie.Value)
		}
	}
	var sessions int64
	if err := tc.DB.Model(&models.Session{}).Where("user_id = ?", user.ID).Count(&sessions).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("refused login left %d session rows behind", sessions)
	}
}

// A login that could not claim the user-management lock never judged the
// credential. Reporting that as "invalid username or password" blames the user
// for a database stall and, with throttling enabled, spends one of their
// attempts; the group bulk operations that hold this lock make it reachable.
func TestLoginReportsLockContentionAsUnavailable(t *testing.T) {
	tc := setupAuthEnv(t)
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "login-contended", Password: "password1", Role: models.RoleEditor,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}

	callback := "test:fail_session_insert_with_lock_contention"
	if err := tc.DB.Callback().Create().Before("gorm:create").Register(callback, func(db *gorm.DB) {
		if db.Statement.Table == "sessions" {
			_ = db.AddError(errors.New("database is locked"))
		}
	}); err != nil {
		t.Fatalf("register contention injector: %v", err)
	}
	t.Cleanup(func() { _ = tc.DB.Callback().Create().Remove(callback) })

	response := doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, nil,
		strings.NewReader(`{"username":"login-contended","password":"password1"}`))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("contended login status=%d body=%s, want 503", response.Code, response.Body.String())
	}
	if response.Header().Get("Retry-After") == "" {
		t.Error("a 503 login should tell the client when to retry")
	}
	if strings.Contains(response.Body.String(), "invalid username or password") {
		t.Errorf("a lock stall must not be reported as a credential failure: %s", response.Body.String())
	}
}
