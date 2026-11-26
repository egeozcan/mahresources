//go:build postgres

package api_tests

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/server/template_handlers/template_filters"
	"mahresources/shortcodes"
)

// The same seeds as the SQLite tests, on Postgres: the jsonb ->> range and the
// date-shape guard over numeric and boolean jsonb, to_char day buckets in counts
// mode, and the principal-scoped executor.
func TestCalendarShortcode_Postgres(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	f := buildCalendarFixture(t, tc)
	checkCalendarItemsMode(t, tc, f)
	checkCalendarCountsMode(t, tc)

	scoped := accountCtx(t, tc, models.RoleUser, &f.root.ID)
	ctx := shortcodes.MetaShortcodeContext{EntityType: "group", EntityID: f.root.ID, Entity: f.root, ScopeGroupID: f.root.ID}
	out := shortcodes.Process(context.Background(), calendarMarchTpl, ctx, nil, template_filters.BuildQueryExecutor(scoped))
	cell := calendarCells(t, out)["March 4"]
	if got := cellNoteIDs(cell); fmt.Sprint(got) != fmt.Sprint(f.ids("Dentist", "Call Bob")) {
		t.Errorf("confined principal's March 4 titles: %v", got)
	}
	if strings.Contains(out, "Secret outside") || !strings.Contains(out, "(3 items)") {
		t.Errorf("confined principal counted the outside note: %s", out)
	}
}
