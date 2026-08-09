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

// setupThrottledAuthEnv is setupAuthEnv with throttling armed at a single
// attempt, which makes "was this charged to the rate limiter?" observable: if an
// outcome counted as a failed attempt, the next login is rejected with 429
// before it ever reaches the credential.
func setupThrottledAuthEnv(t *testing.T) *TestContext {
	tc := setupTestEnvWithConfig(t, func(c *application_context.MahresourcesConfig) {
		c.AuthEnabled = true
		c.SessionTTL = time.Hour
		c.LoginRateLimit = 1
		c.LoginRateWindow = time.Hour
	})
	if sqlDB, err := tc.DB.DB(); err == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	return tc
}

// failCredentialRead makes the first username lookup fail, the way a database
// outage or a busy database surfaces to AuthenticateUser. That read happens
// before the login transaction opens, so the error arrives raw from the driver
// rather than as one of the login's own sentinels.
func failCredentialRead(t *testing.T, tc *TestContext, name string, injected error) {
	t.Helper()
	var fired atomic.Bool
	if err := tc.DB.Callback().Query().After("gorm:query").Register(name, func(db *gorm.DB) {
		if db.Statement.Table != "users" ||
			!strings.Contains(db.Statement.SQL.String(), "username") ||
			!fired.CompareAndSwap(false, true) {
			return
		}
		_ = db.AddError(injected)
	}); err != nil {
		t.Fatalf("register credential-read failure: %v", err)
	}
	t.Cleanup(func() { _ = tc.DB.Callback().Query().Remove(name) })
}

func loginJSON(tc *TestContext, username, password string) *httptest.ResponseRecorder {
	return doReq(tc, http.MethodPost, "/v1/auth/login",
		map[string]string{"Content-Type": "application/json", "Accept": "application/json"}, nil,
		strings.NewReader(`{"username":"`+username+`","password":"`+password+`"}`))
}

// assertAttemptWasNotCharged proves the preceding failure did not consume the
// account's single permitted attempt: a correct password still authenticates.
func assertAttemptWasNotCharged(t *testing.T, tc *TestContext, username, password string) {
	t.Helper()
	response := loginJSON(tc, username, password)
	if response.Code == http.StatusTooManyRequests {
		t.Fatalf("a login that never judged the credential was charged to the rate limiter: %s", response.Body.String())
	}
	if response.Code != http.StatusOK {
		t.Fatalf("valid credentials after the failure status=%d body=%s, want 200", response.Code, response.Body.String())
	}
}

// A busy database while reading the stored hash is the same outage the session
// insert already reports as 503, and it is reachable through the same
// contention: AuthenticateUser runs before the transaction opens, so its error
// bypasses the mapping applied to the transaction's.
func TestLoginContentionReadingTheCredentialIsReportedAsUnavailable(t *testing.T) {
	tc := setupThrottledAuthEnv(t)
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "read-contended", Password: "password1", Role: models.RoleEditor,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	failCredentialRead(t, tc, "test:contended_credential_read", errors.New("database is locked"))

	response := loginJSON(tc, "read-contended", "password1")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("login contended while reading the credential status=%d body=%s, want 503",
			response.Code, response.Body.String())
	}
	if response.Header().Get("Retry-After") == "" {
		t.Error("a 503 login should tell the client when to retry")
	}
	assertAttemptWasNotCharged(t, tc, "read-contended", "password1")
}

// Every other infrastructure failure — a dropped connection, a statement
// timeout, an exhausted pool — is equally not a verdict on the password. 401
// tells the user to doubt a password that was never checked, and spends one of
// their attempts doing it.
func TestLoginInfrastructureFailureIsNotACredentialFailure(t *testing.T) {
	tc := setupThrottledAuthEnv(t)
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "read-broken", Password: "password1", Role: models.RoleEditor,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	failCredentialRead(t, tc, "test:broken_credential_read", errors.New("connection reset by peer"))

	response := loginJSON(tc, "read-broken", "password1")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("login broken while reading the credential status=%d body=%s, want 500",
			response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "invalid username or password") {
		t.Errorf("an infrastructure failure must not be reported as a credential failure: %s", response.Body.String())
	}
	assertAttemptWasNotCharged(t, tc, "read-broken", "password1")
}

// The browser form has no status code to carry the distinction, so it carries
// it in the redirect: ?error=1 is "check your password", and sending a user
// there after an outage starts them resetting a password that still works.
func TestLoginFormReportsInfrastructureFailureSeparatelyFromBadCredentials(t *testing.T) {
	tc := setupThrottledAuthEnv(t)
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "form-broken", Password: "password1", Role: models.RoleEditor,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	failCredentialRead(t, tc, "test:form_broken_credential_read", errors.New("connection reset by peer"))

	response := doReq(tc, http.MethodPost, "/login",
		map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, nil,
		strings.NewReader("username=form-broken&password=password1"))
	location := response.Header().Get("Location")
	if !strings.Contains(location, "error=unavailable") {
		t.Fatalf("form login after an infrastructure failure redirected to %q, want error=unavailable", location)
	}
	assertAttemptWasNotCharged(t, tc, "form-broken", "password1")
}
