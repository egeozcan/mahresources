package auth

import (
	"context"
	"testing"

	"mahresources/models"
)

func TestPluginCodeAllowed(t *testing.T) {
	scope := uint(7)

	cases := []struct {
		name      string
		principal *Principal
		want      bool
	}{
		{
			// Not "auth off": withAuthentication attaches a principal to every
			// request in both modes (root admin when auth is disabled). A missing
			// one means this is not a request context, so it fails closed.
			name:      "no principal",
			principal: nil,
			want:      false,
		},
		{
			name:      "super user",
			principal: &Principal{SuperUser: true},
			want:      true,
		},
		{
			name:      "admin",
			principal: &Principal{Role: models.RoleAdmin, UserID: 1},
			want:      true,
		},
		{
			name:      "editor",
			principal: &Principal{Role: models.RoleEditor, UserID: 2},
			want:      true,
		},
		{
			name:      "unscoped user",
			principal: &Principal{Role: models.RoleUser, UserID: 3},
			want:      true,
		},
		{
			name:      "group-limited user",
			principal: &Principal{Role: models.RoleUser, UserID: 4, ScopeGroupID: &scope},
			want:      false,
		},
		{
			// A guest is always confined, even with no scope group resolved yet:
			// RequiresScope() is true, so it must fail closed.
			name:      "guest without scope group",
			principal: &Principal{Role: models.RoleGuest, UserID: 5},
			want:      false,
		},
		{
			name:      "guest with scope group",
			principal: &Principal{Role: models.RoleGuest, UserID: 6, ScopeGroupID: &scope},
			want:      false,
		},
		{
			// An admin carrying a scope group is still an admin: IsAdmin short
			// circuits, matching principalIsRestricted in the server package.
			name:      "admin with scope group",
			principal: &Principal{Role: models.RoleAdmin, UserID: 7, ScopeGroupID: &scope},
			want:      true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.principal != nil {
				ctx = WithPrincipal(ctx, tc.principal)
			}
			if got := PluginCodeAllowed(ctx); got != tc.want {
				t.Fatalf("PluginCodeAllowed = %v, want %v", got, tc.want)
			}
		})
	}
}

// A nil context must fail closed rather than panic: callers reach this helper
// from template tags where the request context is not guaranteed to be threaded
// through, and the safe answer there is to skip plugin rendering.
func TestPluginCodeAllowedNilContext(t *testing.T) {
	// Held in a variable rather than passed as a literal: a nil Context is
	// exactly what is under test here, and the literal form trips SA1012.
	var nilCtx context.Context
	if PluginCodeAllowed(nilCtx) {
		t.Fatal("PluginCodeAllowed(nil) = true, want false (fail closed)")
	}
}
