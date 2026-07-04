package application_context

import (
	"context"
	"strings"
	"testing"
)

// TestMRQLRegexGateSQLite pins the up-front regex/dialect gate: a regex query
// without a `type =` filter runs cross-entity, which swallows per-entity
// TranslateErrors. On SQLite it must return a clear "requires PostgreSQL" error
// rather than silent empty results.
func TestMRQLRegexGateSQLite(t *testing.T) {
	ctx := setupTestContext(t)

	_, err := ctx.ExecuteMRQL(context.Background(), `name ~* "^x"`, 0, 0, nil)
	if err == nil {
		t.Fatalf("expected an error for SQLite regex without a type filter, got nil")
	}
	if !strings.Contains(err.Error(), "requires PostgreSQL") {
		t.Fatalf("expected 'requires PostgreSQL' error, got %v", err)
	}
}

// TestMRQLRegexGateSQLiteDeterminedEntity confirms the determined-entity path
// also rejects regex on SQLite (via the per-comparison TranslateError).
func TestMRQLRegexGateSQLiteDeterminedEntity(t *testing.T) {
	ctx := setupTestContext(t)

	_, err := ctx.ExecuteMRQL(context.Background(), `type = "resource" AND name ~* "^x"`, 0, 0, nil)
	if err == nil || !strings.Contains(err.Error(), "requires PostgreSQL") {
		t.Fatalf("expected 'requires PostgreSQL' error, got %v", err)
	}
}
