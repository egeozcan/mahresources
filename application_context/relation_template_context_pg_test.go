//go:build postgres && json1 && fts5

package application_context

import (
	"testing"

	"mahresources/models"
)

// Postgres parity for relation_template_context_test.go: the joined edge
// statement, the type-name order and the scope, and an allow-list past
// Postgres's 65535 bind parameters.

func newRelationsPGContext(t *testing.T) *MahresourcesContext {
	t.Helper()
	ctx := newMassEditPGContext(t)
	if err := models.EnsureSupplementalIndexes(ctx.db); err != nil {
		t.Fatalf("supplemental indexes: %v", err)
	}
	return ctx
}

func TestRelationsForTemplateListing_Postgres(t *testing.T) {
	runRelationsForTemplateListing(t, newRelationsPGContext(t))
}

func TestRelationsForTemplateScope_Postgres(t *testing.T) {
	runRelationsForTemplateScope(t, newRelationsPGContext(t))
}

func TestRelationsForTemplateLargeAllowList_Postgres(t *testing.T) {
	runRelationsForTemplateLargeAllowList(t, newRelationsPGContext(t), 70000)
}
