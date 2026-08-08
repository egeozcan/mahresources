package application_context

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/models"

	"gorm.io/gorm"
)

func TestCreateAndValidateSession(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "sess", Password: "password1", Role: models.RoleUser})

	raw, session, err := ctx.CreateSession(u.ID, time.Hour, "agent", "1.2.3.4")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if raw == "" || session.TokenHash == raw {
		t.Error("raw token must be returned and must differ from the stored hash")
	}

	got, _, err := ctx.ValidateSession(raw)
	if err != nil {
		t.Fatalf("ValidateSession: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("validated wrong user: %d", got.ID)
	}

	if _, _, err := ctx.ValidateSession("not-a-real-token"); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("bogus token: got %v", err)
	}
	if _, _, err := ctx.ValidateSession(""); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("empty token: got %v", err)
	}
}

func TestAuthenticateAndCreateSessionSerializesPasswordReset(t *testing.T) {
	contexts := newConcurrentApiTokenTestContexts(t, 2)
	for _, testCtx := range contexts {
		testCtx.Config.MaxUserTokens = 0
	}
	user, err := contexts[0].CreateUser(&UserInput{Username: "login-race", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	credentialsRead := make(chan struct{})
	releaseLogin := make(chan struct{})
	var credentialsOnce sync.Once
	if err := contexts[0].db.Callback().Query().After("gorm:query").Register("test:pause_login_after_credentials", func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && strings.Contains(tx.Statement.SQL.String(), "username") {
			credentialsOnce.Do(func() {
				close(credentialsRead)
				<-releaseLogin
			})
		}
	}); err != nil {
		t.Fatalf("register login barrier: %v", err)
	}

	resetAttempted := make(chan struct{})
	var resetOnce sync.Once
	if err := contexts[1].db.Callback().Raw().Before("gorm:raw").Register("test:observe_reset_mutation_lock", func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), "UPDATE users SET id = id") {
			resetOnce.Do(func() { close(resetAttempted) })
		}
	}); err != nil {
		t.Fatalf("register reset barrier: %v", err)
	}

	type loginResult struct {
		raw string
		err error
	}
	loginDone := make(chan loginResult, 1)
	go func() {
		_, raw, _, err := contexts[0].AuthenticateAndCreateSession("login-race", "password1", time.Hour, "agent", "127.0.0.1")
		loginDone <- loginResult{raw: raw, err: err}
	}()
	<-credentialsRead

	resetDone := make(chan error, 1)
	go func() { resetDone <- contexts[1].SetUserPassword(user.ID, "password2") }()
	<-resetAttempted
	close(releaseLogin)

	login := <-loginDone
	if login.err != nil {
		t.Fatalf("login: %v", login.err)
	}
	if err := <-resetDone; err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, _, err := contexts[0].ValidateSession(login.raw); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("session created by a racing old-password login survived reset: %v", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "exp", Password: "password1", Role: models.RoleUser})

	raw, _, _ := ctx.CreateSession(u.ID, -time.Hour, "", "")
	if _, _, err := ctx.ValidateSession(raw); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("expired session should be invalid, got %v", err)
	}
}

func TestRevokeSession(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "rev", Password: "password1", Role: models.RoleUser})
	raw, _, _ := ctx.CreateSession(u.ID, time.Hour, "", "")

	if err := ctx.RevokeSession(raw); err != nil {
		t.Fatalf("RevokeSession: %v", err)
	}
	if _, _, err := ctx.ValidateSession(raw); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("revoked session should be invalid, got %v", err)
	}
}

func TestValidateSession_DisabledUser(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "dis", Password: "password1", Role: models.RoleUser})
	raw, _, _ := ctx.CreateSession(u.ID, time.Hour, "", "")

	u.Disabled = true
	ctx.db.Save(u)
	if _, _, err := ctx.ValidateSession(raw); !errors.Is(err, ErrUserDisabled) {
		t.Errorf("disabled user session should be rejected, got %v", err)
	}
}

func TestRevokeUserSessionsAndCleanup(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "multi", Password: "password1", Role: models.RoleUser})
	r1, _, _ := ctx.CreateSession(u.ID, time.Hour, "", "")
	ctx.CreateSession(u.ID, time.Hour, "", "")

	if err := ctx.RevokeUserSessions(u.ID); err != nil {
		t.Fatalf("RevokeUserSessions: %v", err)
	}
	if _, _, err := ctx.ValidateSession(r1); !errors.Is(err, ErrSessionInvalid) {
		t.Errorf("all sessions should be revoked, got %v", err)
	}

	// Expired-session sweep.
	ctx.CreateSession(u.ID, -time.Hour, "", "")
	n, err := ctx.DeleteExpiredSessions()
	if err != nil {
		t.Fatalf("DeleteExpiredSessions: %v", err)
	}
	if n < 1 {
		t.Errorf("expected at least one expired session removed, got %d", n)
	}
}
