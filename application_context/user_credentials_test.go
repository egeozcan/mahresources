package application_context

import (
	"errors"
	"testing"
	"time"

	"mahresources/models"
)

func assertNoUserCredentials(t *testing.T, ctx *MahresourcesContext, userID uint) {
	t.Helper()
	var sessions, tokens int64
	if err := ctx.db.Model(&models.Session{}).Where("user_id = ?", userID).Count(&sessions).Error; err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	if err := ctx.db.Model(&models.ApiToken{}).Where("user_id = ?", userID).Count(&tokens).Error; err != nil {
		t.Fatalf("count tokens: %v", err)
	}
	if sessions != 0 || tokens != 0 {
		t.Fatalf("credentials remain: sessions=%d tokens=%d", sessions, tokens)
	}
}

func addUserCredentials(t *testing.T, ctx *MahresourcesContext, userID uint) (string, string) {
	t.Helper()
	session, _, err := ctx.CreateSession(userID, time.Hour, "", "")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	token, _, err := ctx.CreateApiToken(userID, "test", nil)
	if err != nil {
		t.Fatalf("create api token: %v", err)
	}
	return session, token
}

func TestEnsureAdminUserRevokesPrePromotionCredentials(t *testing.T) {
	ctx := newAuthTestContext(t)
	user, err := ctx.CreateUser(&UserInput{Username: "promote", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	oldSession, oldToken := addUserCredentials(t, ctx, user.ID)

	promoted, err := ctx.EnsureAdminUser("promote", "new-password")
	if err != nil {
		t.Fatalf("EnsureAdminUser: %v", err)
	}
	if promoted.Role != models.RoleAdmin || promoted.Disabled {
		t.Fatalf("not promoted to enabled admin: role=%s disabled=%v", promoted.Role, promoted.Disabled)
	}
	assertNoUserCredentials(t, ctx, user.ID)
	if _, _, err := ctx.ValidateSession(oldSession); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("pre-promotion session remains valid: %v", err)
	}
	if _, _, err := ctx.ValidateApiToken(oldToken); !errors.Is(err, ErrApiTokenInvalid) {
		t.Fatalf("pre-promotion token remains valid: %v", err)
	}
}

func TestAdminPasswordResetRevokesAllCredentials(t *testing.T) {
	ctx := newAuthTestContext(t)
	user, err := ctx.CreateUser(&UserInput{Username: "reset", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	addUserCredentials(t, ctx, user.ID)
	if err := ctx.SetUserPassword(user.ID, "new-password"); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	assertNoUserCredentials(t, ctx, user.ID)
}

func TestChangeOwnPasswordPreservesCurrentSessionOnly(t *testing.T) {
	ctx := newAuthTestContext(t)
	user, err := ctx.CreateUser(&UserInput{Username: "self", Password: "password1", Role: models.RoleUser})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	currentRaw, current, err := ctx.CreateSession(user.ID, time.Hour, "current", "")
	if err != nil {
		t.Fatalf("create current session: %v", err)
	}
	peerRaw, _, err := ctx.CreateSession(user.ID, time.Hour, "peer", "")
	if err != nil {
		t.Fatalf("create peer session: %v", err)
	}
	tokenRaw, _, err := ctx.CreateApiToken(user.ID, "cli", nil)
	if err != nil {
		t.Fatalf("create token: %v", err)
	}

	keep := current.TokenHash
	if err := ctx.ChangeOwnPassword(user.ID, "new-password", &keep); err != nil {
		t.Fatalf("ChangeOwnPassword: %v", err)
	}
	if _, _, err := ctx.ValidateSession(currentRaw); err != nil {
		t.Fatalf("current session revoked: %v", err)
	}
	if _, _, err := ctx.ValidateSession(peerRaw); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("peer session remains valid: %v", err)
	}
	if _, _, err := ctx.ValidateApiToken(tokenRaw); err != nil {
		t.Fatalf("API token should remain valid: %v", err)
	}
}
