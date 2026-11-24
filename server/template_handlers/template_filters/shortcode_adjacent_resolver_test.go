package template_filters

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/shortcodes"
)

// fakeAdjacentContext answers the lookup itself and keeps the real logger.
type fakeAdjacentContext struct {
	*application_context.MahresourcesContext
	timeout  time.Duration
	err      error
	calls    []shortcodes.AdjacentQuery
	deadline []time.Duration
}

func (f *fakeAdjacentContext) AdjacentEntities(ctx context.Context, q shortcodes.AdjacentQuery) (shortcodes.AdjacentResult, error) {
	f.calls = append(f.calls, q)
	if d, ok := ctx.Deadline(); ok {
		f.deadline = append(f.deadline, time.Until(d))
	} else {
		f.deadline = append(f.deadline, -1)
	}
	if f.err != nil {
		return shortcodes.AdjacentResult{}, f.err
	}
	return shortcodes.AdjacentResult{Next: &shortcodes.AdjacentLink{ID: q.EntityID + 1, Name: "next"}}, nil
}

func (f *fakeAdjacentContext) MRQLQueryTimeout() time.Duration { return f.timeout }

func logRows(t *testing.T, db *gorm.DB, entityType string) []models.LogEntry {
	t.Helper()
	var rows []models.LogEntry
	if err := db.Where("entity_type = ?", entityType).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	return rows
}

func TestAdjacentResolverCachesBudgetsAndBounds(t *testing.T) {
	appCtx, db := setupExecutorTestContext(t)
	f := &fakeAdjacentContext{MahresourcesContext: appCtx, timeout: time.Minute}
	resolve := BuildAdjacentResolver(f)
	reqCtx := shortcodes.WithQueryBudget(context.Background(), 1)

	q := shortcodes.AdjacentQuery{EntityType: "note", EntityID: 4}
	for i := 0; i < 2; i++ {
		res, err := resolve(reqCtx, q)
		if err != nil || res.Next == nil || res.Next.ID != 5 {
			t.Fatalf("call %d: %+v, %v", i, res, err)
		}
	}
	if len(f.calls) != 1 {
		t.Fatalf("an identical lookup must be served from the page cache, got %d calls", len(f.calls))
	}
	if d := f.deadline[0]; d <= 0 || d > time.Minute {
		t.Fatalf("the lookup must run under the MRQL timeout, deadline in %v", d)
	}

	// The second distinct lookup is past the budget of one.
	_, err := resolve(reqCtx, shortcodes.AdjacentQuery{EntityType: "note", EntityID: 9})
	if !errors.Is(err, shortcodes.ErrAdjacentBudget) || !strings.Contains(err.Error(), "inline query budget exceeded (1 per page)") {
		t.Fatalf("err = %v, want the budget error", err)
	}
	_, _ = resolve(reqCtx, shortcodes.AdjacentQuery{EntityType: "note", EntityID: 10})
	if len(f.calls) != 1 {
		t.Fatalf("a refused lookup must not run, got %d calls", len(f.calls))
	}
	if rows := logRows(t, db, "mrql"); len(rows) != 1 {
		t.Fatalf("the exhausted budget is logged once per page, got %d rows", len(rows))
	}
}

func TestAdjacentResolverLogsAndDoesNotCacheFailures(t *testing.T) {
	appCtx, db := setupExecutorTestContext(t)
	f := &fakeAdjacentContext{MahresourcesContext: appCtx, timeout: time.Minute, err: errors.New("boom")}
	resolve := BuildAdjacentResolver(f)
	q := shortcodes.AdjacentQuery{EntityType: "resource", EntityID: 4}
	for i := 0; i < 2; i++ {
		if _, err := resolve(context.Background(), q); err == nil {
			t.Fatal("the error must reach the handler")
		}
	}
	if len(f.calls) != 2 {
		t.Fatalf("a failure must not be cached, got %d calls", len(f.calls))
	}
	rows := logRows(t, db, "shortcode")
	if len(rows) != 2 || rows[0].Level != models.LogLevelWarning || !strings.Contains(rows[0].Message, "boom") {
		t.Fatalf("failures are logged at warning: %+v", rows)
	}
}

func TestWithAdjacentResolverForNeedsTheCapability(t *testing.T) {
	base := context.Background()
	if got := WithAdjacentResolverFor(base, struct{}{}); got != base {
		t.Fatal("a context without AdjacentEntities must get no resolver")
	}
	if got := WithAdjacentResolverFor(base, nil); got != base {
		t.Fatal("a nil context must get no resolver")
	}
	var nilCtx *application_context.MahresourcesContext
	if got := WithAdjacentResolverFor(base, nilCtx); got != base {
		t.Fatal("a nil *MahresourcesContext must get no resolver")
	}
	appCtx, _ := setupExecutorTestContext(t)
	html := shortcodes.Process(WithAdjacentResolverFor(base, appCtx), "[adjacent]", shortcodes.MetaShortcodeContext{EntityType: "note", EntityID: 1}, nil, nil)
	if strings.Contains(html, "unavailable") {
		t.Fatalf("a MahresourcesContext installs the resolver, got %q", html)
	}
}
