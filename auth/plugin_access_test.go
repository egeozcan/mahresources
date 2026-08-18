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
			// non-public path in both modes (root admin when auth is disabled), and
			// no public path renders a plugin surface. A missing one means this is
			// not a request context, so it fails closed.
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

// PluginAccessFor answers "may this caller reach this plugin's code", and that
// is all it answers. The surfaces it gates are reads a guest is entitled to
// perform once an operator has opened the plugin: pages, shortcodes and slots
// are all capRead.
//
// Running an action is not one of them. It is a write, refused for a guest by
// the capability check in withAuthorization, so the predicate that decides which
// actions to OFFER has to be stricter than this one. Stricter at the offer,
// though, not here: folding the write capability into this function would take a
// guest's toggled-on pages away with it, which no rule asks for and no other
// test would notice, because the scoped principals the render-gate tests use are
// users and users may write.
func TestPluginAccessFor_OpenPluginStaysReachableForAReadOnlyGuest(t *testing.T) {
	scope := uint(7)
	ctx := WithPrincipal(context.Background(), &Principal{UserID: 5, Role: models.RoleGuest, ScopeGroupID: &scope})

	access := PluginAccessFor(ctx, func(name string) bool { return name == "open-plugin" })
	if !access("open-plugin") {
		t.Error("a guest is refused a plugin an operator opened to group-limited accounts")
	}
	if access("shut-plugin") {
		t.Error("a guest reaches a plugin no operator opened")
	}
}
