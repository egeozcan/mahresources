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

type loginResult struct {
	raw string
	err error
}

// pauseAfterCredentialRead stops a login between reading the stored credential
// and minting the session — the window in which a concurrent reset must be
// noticed.
func pauseAfterCredentialRead(t *testing.T, ctx *MahresourcesContext, name string) (reached chan struct{}, release func()) {
	t.Helper()
	reached = make(chan struct{})
	releaseChannel := make(chan struct{})
	release = releaseBarrier(t, releaseChannel)
	var once sync.Once
	if err := ctx.db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "users" && strings.Contains(tx.Statement.SQL.String(), "username") {
			once.Do(func() {
				close(reached)
				<-releaseChannel
			})
		}
	}); err != nil {
		t.Fatalf("register credential-read barrier: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name) })
	return reached, release
}

// observeMutationLockAttempt signals when an operation reaches the SQLite
// user-management serialization write.
func observeMutationLockAttempt(t *testing.T, ctx *MahresourcesContext, name string) chan struct{} {
	t.Helper()
	attempted := make(chan struct{})
	var once sync.Once
	if err := ctx.db.Callback().Raw().Before("gorm:raw").Register(name, func(tx *gorm.DB) {
		if strings.Contains(tx.Statement.SQL.String(), sqliteUserManagementLockStatement) {
			once.Do(func() { close(attempted) })
		}
	}); err != nil {
		t.Fatalf("register mutation-lock barrier: %v", err)
	}
	t.Cleanup(func() { _ = ctx.db.Callback().Raw().Remove(name) })
	return attempted
}

// A login verifies the password before it takes the user-management lock, so
// hashing — tens of milliseconds of bcrypt on an unauthenticated request — must
// not block user management. Holding that lock across the compare is what let a
// handful of anonymous login attempts stall every writer in the process.
func TestAuthenticateAndCreateSessionVerifiesCredentialOutsideTheMutationLock(t *testing.T) {
	contexts := newConcurrentApiTokenTestContexts(t, 2)
	user, err := contexts[0].CreateUser(&UserInput{Username: "login-verify", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	credentialsRead, releaseLogin := pauseAfterCredentialRead(t, contexts[0], "test:pause_login_after_credentials")

	loginDone := make(chan loginResult, 1)
	go func() {
		_, raw, _, loginErr := contexts[0].AuthenticateAndCreateSession("login-verify", "password1", time.Hour, "agent", "127.0.0.1")
		loginDone <- loginResult{raw: raw, err: loginErr}
	}()
	waitBarrier(t, credentialsRead, "login never read the stored credential")

	resetDone := make(chan error, 1)
	go func() { resetDone <- contexts[1].SetUserPassword(user.ID, "password2") }()
	assertCompletes(t, resetDone, "password reset was blocked while a login verified its password")
	releaseLogin()

	// The reset won, so the hash the login verified is stale and the session must
	// be refused rather than minted against a superseded password.
	login := <-loginDone
	if !errors.Is(login.err, ErrInvalidCredentials) {
		t.Fatalf("login against a superseded password err=%v, want ErrInvalidCredentials", login.err)
	}
	var sessions int64
	if err := contexts[0].db.Model(&models.Session{}).Where("user_id = ?", user.ID).Count(&sessions).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if sessions != 0 {
		t.Fatalf("refused login left %d session rows behind", sessions)
	}
}

// The other half of the invariant: a reset that arrives while the login holds
// the lock cannot commit until the session row exists, and then revokes it.
func TestAuthenticateAndCreateSessionSerializesPasswordReset(t *testing.T) {
	contexts := newConcurrentApiTokenTestContexts(t, 2)
	user, err := contexts[0].CreateUser(&UserInput{Username: "login-race", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	sessionInsert := make(chan struct{})
	releaseInsertChannel := make(chan struct{})
	releaseInsert := releaseBarrier(t, releaseInsertChannel)
	var insertOnce sync.Once
	if err := contexts[0].db.Callback().Create().Before("gorm:create").Register("test:pause_login_session_insert", func(tx *gorm.DB) {
		if tx.Statement.Table == "sessions" {
			insertOnce.Do(func() {
				close(sessionInsert)
				<-releaseInsertChannel
			})
		}
	}); err != nil {
		t.Fatalf("register session insert barrier: %v", err)
	}
	t.Cleanup(func() { _ = contexts[0].db.Callback().Create().Remove("test:pause_login_session_insert") })

	resetAttempted := observeMutationLockAttempt(t, contexts[1], "test:observe_reset_mutation_lock")

	loginDone := make(chan loginResult, 1)
	go func() {
		_, raw, _, loginErr := contexts[0].AuthenticateAndCreateSession("login-race", "password1", time.Hour, "agent", "127.0.0.1")
		loginDone <- loginResult{raw: raw, err: loginErr}
	}()
	waitBarrier(t, sessionInsert, "login never reached the session insert inside the mutation lock")

	resetDone := make(chan error, 1)
	go func() { resetDone <- contexts[1].SetUserPassword(user.ID, "password2") }()
	waitBarrier(t, resetAttempted, "password reset never attempted the mutation lock")
	assertStillBlocked(t, resetDone, "password reset was not serialized behind the login holding the mutation lock")
	releaseInsert()

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
