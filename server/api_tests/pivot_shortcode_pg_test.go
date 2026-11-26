//go:build postgres

package api_tests

import (
	"fmt"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/types"
)

// The SQLite checks on Postgres: ->> text keys and the boolean label, the
// numeric SUM cast, the group alias in ORDER BY, and the principal-scoped
// executor with visible joins.

func TestPivotShortcode_LinkCountsMatchCells_Postgres(t *testing.T) {
	checkPivotLinkCountsMatchCells(t, setupPostgresAuthEnv(t), true)
}

func TestPivotShortcode_SumSemantics_Postgres(t *testing.T) {
	checkPivotSumSemantics(t, setupPostgresAuthEnv(t), true)
}

func TestPivotShortcode_OneAggregateStatement_Postgres(t *testing.T) {
	checkPivotRunsOneAggregateStatement(t, setupPostgresAuthEnv(t))
}

func TestPivotShortcode_ScopedPrincipalCannotSeeOutside_Postgres(t *testing.T) {
	checkPivotScopedPrincipal(t, setupPostgresAuthEnv(t))
}

func TestPivotShortcode_ConfinedDescendantDoesNotLink_Postgres(t *testing.T) {
	checkPivotConfinedDescendantDoesNotLink(t, setupPostgresAuthEnv(t))
}

func TestPivotShortcode_ConfinedJoinedLabels_Postgres(t *testing.T) {
	checkPivotConfinedJoinedLabels(t, setupPostgresAuthEnv(t))
}

func TestPivotShortcode_AliasByteLimit_Postgres(t *testing.T) {
	tc := setupPostgresAuthEnv(t)
	admin := roleBearer(t, tc, models.RoleAdmin)
	g := &models.Group{Name: "pivot-alias"}
	mkSim(t, tc, g)
	axisKey, sumKey := strings.Repeat("r", 58), strings.Repeat("s", 54)
	meta := fmt.Sprintf(`{"%s":"axis-label","%sx":"long-axis-label","%s":5,"%sx":6}`, axisKey, axisKey, sumKey, sumKey)
	mkSim(t, tc, &models.Note{Name: "matching", OwnerId: &g.ID, Meta: types.JSON(meta)})
	const tpl = `[pivot query="type = note" rows="meta.%s" cols="name" value="sum" field="meta.%s"]`
	out := deferredHTML(t, tc, admin, "group", g.ID, fmt.Sprintf(tpl, axisKey, sumKey))
	rows := pivotRows(t, out)
	if got := fmt.Sprint(pivotTexts(rows["axis-label"])); got != "[axis-label 5 5]" {
		t.Errorf("63-byte dimension and sum aliases must both round-trip: %s", got)
	}
	for _, src := range []string{fmt.Sprintf(tpl, axisKey+"x", sumKey), fmt.Sprintf(tpl, axisKey, sumKey+"x")} {
		out := deferredHTML(t, tc, admin, "group", g.ID, src)
		if !strings.Contains(out, "63-byte SQL alias limit") || strings.Contains(out, "mrql-pivot") {
			t.Errorf("64-byte alias should be refused instead of truncated: %s", out)
		}
	}
}
