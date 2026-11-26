package template_filters

import (
	"context"
	"errors"
	"strings"
	"sync"
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
	mu       sync.Mutex
	gate     func() // when set, runs first, outside mu
	timeout  time.Duration
	err      error
	calls    []shortcodes.AdjacentQuery
	deadline []time.Duration
}

func (f *fakeAdjacentContext) AdjacentEntities(ctx context.Context, q shortcodes.AdjacentQuery) (shortcodes.AdjacentResult, error) {
	if f.gate != nil {
		f.gate()
	}
	f.mu.Lock()
	defer f.mu.Unlock()
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

// The slots of one page can render concurrently against one context; the
// page cache must hold up (run with -race).
func TestAdjacentResolverIsSafeForConcurrentRenders(t *testing.T) {
	appCtx, _ := setupExecutorTestContext(t)
	f := &fakeAdjacentContext{MahresourcesContext: appCtx, timeout: time.Minute}
	resolve := BuildAdjacentResolver(f)

	// Identical lookups racing each other run once and spend one unit.
	budgeted := shortcodes.WithQueryBudget(context.Background(), 1)
	var same sync.WaitGroup
	for i := 0; i < 8; i++ {
		same.Add(1)
		go func() {
			defer same.Done()
			if res, err := resolve(budgeted, shortcodes.AdjacentQuery{EntityType: "group", EntityID: 50}); err != nil || res.Next == nil {
				t.Errorf("identical concurrent lookup: %+v, %v", res, err)
			}
		}()
	}
	same.Wait()
	if len(f.calls) != 1 {
		t.Fatalf("identical concurrent lookups ran %d times, want 1", len(f.calls))
	}

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(id uint) {
			defer wg.Done()
			for j := 0; j < 2; j++ {
				res, err := resolve(context.Background(), shortcodes.AdjacentQuery{EntityType: "note", EntityID: id})
				if err != nil || res.Next == nil || res.Next.ID != id+1 {
					t.Errorf("note %d: %+v, %v", id, res, err)
				}
			}
		}(uint(i%8 + 1))
	}
	wg.Wait()
}

// Distinct lookups do not queue behind each other, and a waiter for an
// identical lookup gives up with its own context.
func TestAdjacentResolverRunsDistinctLookupsInParallel(t *testing.T) {
	appCtx, _ := setupExecutorTestContext(t)
	entered, release := make(chan struct{}, 2), make(chan struct{})
	f := &fakeAdjacentContext{MahresourcesContext: appCtx, timeout: time.Minute, gate: func() {
		entered <- struct{}{}
		<-release
	}}
	resolve := BuildAdjacentResolver(f)

	var wg sync.WaitGroup
	for _, id := range []uint{1, 2} {
		wg.Add(1)
		go func(id uint) {
			defer wg.Done()
			_, _ = resolve(context.Background(), shortcodes.AdjacentQuery{EntityType: "note", EntityID: id})
		}(id)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-entered:
		case <-time.After(5 * time.Second):
			close(release)
			t.Fatal("the second distinct lookup waited for the first")
		}
	}

	// An identical lookup now waits on the one in flight; its own deadline
	// ends the wait.
	short, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := resolve(short, shortcodes.AdjacentQuery{EntityType: "note", EntityID: 1}); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("a waiter past its deadline: err = %v", err)
	}
	close(release)
	wg.Wait()
	if len(f.calls) != 2 {
		t.Fatalf("got %d lookups, want 2", len(f.calls))
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
