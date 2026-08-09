//go:build postgres

package api_tests

import (
	"errors"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/models"
)

// A username no account can hold must be answered as a verdict on the credential,
// not as a server fault. PostgreSQL rejects a NUL byte and invalid UTF-8 in a text
// parameter, so passing one through to the lookup turns an impossible login into
// the "never judged" bucket: HTTP 500, deliberately *not* charged to the login
// rate limiter, and one application-log row per request. An unauthenticated
// client can then repeat it indefinitely — unthrottled by construction, and
// writing a row every time.
//
// No stored username can contain those bytes (the insert would fail for the same
// reason), so the answer is the one a nonexistent username already gets.
func TestPostgresLoginWithAnImpossibleUsernameIsACredentialFailure(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	if _, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: "pg-impossible-neighbour", Password: "password1", Role: models.RoleEditor,
	}); err != nil {
		t.Fatalf("create neighbour user: %v", err)
	}

	for name, username := range map[string]string{
		"NUL byte":      "pg-impossible-neighbour\x00",
		"invalid UTF-8": "pg-impossible-neighbour\xff",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := tc.AppCtx.AuthenticateUser(username, "password1"); !errors.Is(err, application_context.ErrInvalidCredentials) {
				t.Errorf("AuthenticateUser(%q) err=%v, want ErrInvalidCredentials", username, err)
			}
			if _, _, _, err := tc.AppCtx.AuthenticateAndCreateSession(
				username, "password1", time.Hour, "agent", "127.0.0.1",
			); !errors.Is(err, application_context.ErrInvalidCredentials) {
				t.Errorf("AuthenticateAndCreateSession(%q) err=%v, want ErrInvalidCredentials", username, err)
			}
		})
	}
}
