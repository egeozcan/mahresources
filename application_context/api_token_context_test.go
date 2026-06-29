package application_context

import (
	"errors"
	"testing"
	"time"

	"mahresources/models"
)

func TestCreateAndValidateApiToken(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "tok", Password: "password1", Role: models.RoleEditor})

	raw, token, err := ctx.CreateApiToken(u.ID, "ci", nil)
	if err != nil {
		t.Fatalf("CreateApiToken: %v", err)
	}
	if raw == "" || token.TokenHash == raw {
		t.Error("raw token must be returned and differ from stored hash")
	}
	if token.Prefix == "" {
		t.Error("token prefix should be set for display")
	}

	got, _, err := ctx.ValidateApiToken(raw)
	if err != nil {
		t.Fatalf("ValidateApiToken: %v", err)
	}
	if got.ID != u.ID {
		t.Errorf("validated wrong user: %d", got.ID)
	}

	if _, _, err := ctx.ValidateApiToken("nope"); !errors.Is(err, ErrApiTokenInvalid) {
		t.Errorf("bogus token: got %v", err)
	}
}

func TestApiTokenDisabledAndExpired(t *testing.T) {
	ctx := newAuthTestContext(t)
	u, _ := ctx.CreateUser(&UserInput{Username: "te", Password: "password1", Role: models.RoleEditor})

	// Disabled token.
	rawDisabled, tokDisabled, _ := ctx.CreateApiToken(u.ID, "d", nil)
	ctx.db.Model(&models.ApiToken{}).Where("id = ?", tokDisabled.ID).Update("disabled", true)
	if _, _, err := ctx.ValidateApiToken(rawDisabled); !errors.Is(err, ErrApiTokenInvalid) {
		t.Errorf("disabled token: got %v", err)
	}

	// Expired token.
	past := time.Now().Add(-time.Hour)
	rawExpired, _, _ := ctx.CreateApiToken(u.ID, "e", &past)
	if _, _, err := ctx.ValidateApiToken(rawExpired); !errors.Is(err, ErrApiTokenInvalid) {
		t.Errorf("expired token: got %v", err)
	}

	// Disabled user invalidates an otherwise-valid token.
	rawOK, _, _ := ctx.CreateApiToken(u.ID, "ok", nil)
	u.Disabled = true
	ctx.db.Save(u)
	if _, _, err := ctx.ValidateApiToken(rawOK); !errors.Is(err, ErrUserDisabled) {
		t.Errorf("disabled user token: got %v", err)
	}
}

func TestRevokeApiTokenScopedToOwner(t *testing.T) {
	ctx := newAuthTestContext(t)
	owner, _ := ctx.CreateUser(&UserInput{Username: "owner", Password: "password1", Role: models.RoleEditor})
	other, _ := ctx.CreateUser(&UserInput{Username: "other", Password: "password1", Role: models.RoleEditor})

	raw, token, _ := ctx.CreateApiToken(owner.ID, "k", nil)

	// Another user cannot revoke this token by ID.
	if err := ctx.RevokeApiToken(token.ID, other.ID); !errors.Is(err, ErrApiTokenNotFound) {
		t.Errorf("cross-user revoke should fail, got %v", err)
	}
	if _, _, err := ctx.ValidateApiToken(raw); err != nil {
		t.Errorf("token should still be valid after failed cross-user revoke: %v", err)
	}

	// Owner can revoke.
	if err := ctx.RevokeApiToken(token.ID, owner.ID); err != nil {
		t.Fatalf("owner revoke: %v", err)
	}
	if _, _, err := ctx.ValidateApiToken(raw); !errors.Is(err, ErrApiTokenInvalid) {
		t.Errorf("revoked token should be invalid, got %v", err)
	}

	if tokens, _ := ctx.ListApiTokens(owner.ID); len(tokens) != 0 {
		t.Errorf("owner should have no tokens left, got %d", len(tokens))
	}
}

func TestCreateApiTokenPerUserCap(t *testing.T) {
	ctx := newAuthTestContext(t)
	ctx.Config.MaxUserTokens = 2
	u, _ := ctx.CreateUser(&UserInput{Username: "capped", Password: "password1", Role: models.RoleEditor})
	other, _ := ctx.CreateUser(&UserInput{Username: "uncapped-peer", Password: "password1", Role: models.RoleEditor})

	if _, _, err := ctx.CreateApiToken(u.ID, "a", nil); err != nil {
		t.Fatalf("first token: %v", err)
	}
	if _, _, err := ctx.CreateApiToken(u.ID, "b", nil); err != nil {
		t.Fatalf("second token: %v", err)
	}
	// Third token exceeds the cap.
	if _, _, err := ctx.CreateApiToken(u.ID, "c", nil); !errors.Is(err, ErrApiTokenLimitReached) {
		t.Fatalf("third token should hit the cap, got %v", err)
	}
	// The cap is per-user: a different user is unaffected.
	if _, _, err := ctx.CreateApiToken(other.ID, "a", nil); err != nil {
		t.Fatalf("peer's token should not be blocked by another user's cap: %v", err)
	}
	// Revoking frees a slot.
	tokens, _ := ctx.ListApiTokens(u.ID)
	if err := ctx.RevokeApiToken(tokens[0].ID, u.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if _, _, err := ctx.CreateApiToken(u.ID, "c", nil); err != nil {
		t.Fatalf("token after freeing a slot should succeed: %v", err)
	}
}

func TestCreateApiTokenUnlimitedByDefault(t *testing.T) {
	ctx := newAuthTestContext(t) // MaxUserTokens defaults to 0 = unlimited
	u, _ := ctx.CreateUser(&UserInput{Username: "many", Password: "password1", Role: models.RoleEditor})
	for i := 0; i < 25; i++ {
		if _, _, err := ctx.CreateApiToken(u.ID, "k", nil); err != nil {
			t.Fatalf("token %d should succeed with no cap: %v", i, err)
		}
	}
}
