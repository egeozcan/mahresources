package application_context

import (
	"testing"

	"mahresources/models"
)

// PostgreSQL refuses a NUL byte or invalid UTF-8 in a text parameter, so a login
// carrying one is screened before the lookup rather than allowed to surface a
// driver error the login would classify as an outage. SQLite has no such
// restriction: it stores those bytes and matches them, so an account can hold
// such a name there and must keep authenticating.
//
// Applying the screen to every backend is the tempting simplification, and it
// silently locks those accounts out — they can be created (CreateUser does not
// restrict the character set) but never log in again.
func TestAuthenticateUserAcceptsBytesOnlyPostgresRejects(t *testing.T) {
	for label, username := range map[string]string{
		"NUL byte":      "sqlite-nul\x00name",
		"invalid UTF-8": "sqlite-badutf8\xffname",
	} {
		t.Run(label, func(t *testing.T) {
			contexts := newConcurrentApiTokenTestContexts(t, 1)
			user, err := contexts[0].CreateUser(&UserInput{
				Username: username, Password: "password1", Role: models.RoleUser,
			})
			if err != nil {
				t.Skipf("this backend refused to store %q at creation: %v", username, err)
			}

			authenticated, err := contexts[0].AuthenticateUser(username, "password1")
			if err != nil {
				t.Fatalf("AuthenticateUser(%q) err=%v, want the account it just created", username, err)
			}
			if authenticated.ID != user.ID {
				t.Fatalf("authenticated id=%d, want %d", authenticated.ID, user.ID)
			}
		})
	}
}
