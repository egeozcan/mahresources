//go:build postgres

package api_tests

import (
	"testing"
	"time"

	"mahresources/application_context"
)

func setupPostgresAuthEnv(t *testing.T) *TestContext {
	t.Helper()
	tc := setupPostgresTestEnvWith(t, func(c *application_context.MahresourcesConfig) {
		c.AuthEnabled = true
		c.SessionTTL = time.Hour
	}, nil)
	if _, err := tc.AppCtx.EnsureAdminUser("admin", "adminpw1"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	return tc
}

func TestScopedNoteUpdateSparesUnseenAssociations_PG(t *testing.T) {
	checkScopedNoteUpdateSparesUnseen(t, setupPostgresAuthEnv(t))
}

func TestScopedResourceEditSparesUnseenAssociations_PG(t *testing.T) {
	checkScopedResourceEditSparesUnseen(t, setupPostgresAuthEnv(t))
}

func TestScopedEditRemovesVisibleLinksOnExplicitClear_PG(t *testing.T) {
	checkScopedEditRemovesVisibleLinks(t, setupPostgresAuthEnv(t))
}
