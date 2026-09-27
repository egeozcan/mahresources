//go:build postgres && json1 && fts5

package application_context

import "testing"

// The scrub reads the summary through a text cast and rewrites a json column,
// both of which PostgreSQL types differently from SQLite.
func TestTheRuntimeIdentityIsScrubbedFromStoredPluginActionSummariesOnPostgres(t *testing.T) {
	ctx, _, _ := newPostgresOwnershipFixture(t, 1)
	exerciseTheRuntimeIdentityScrub(t, ctx)
}
