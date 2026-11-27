package template_filters

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/types"
	"mahresources/shortcodes"
)

// fakeRelationContext answers both statements itself and keeps the real
// logger.
type fakeRelationContext struct {
	*application_context.MahresourcesContext
	mu        sync.Mutex
	timeout   time.Duration
	err       error
	lists     []shortcodes.RelationsRequest
	details   [][]uint
	deadlines []time.Duration
}

func (f *fakeRelationContext) ListGroupRelationsForTemplate(ctx context.Context, req shortcodes.RelationsRequest) ([]shortcodes.GroupRelationEntry, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lists = append(f.lists, req)
	if d, ok := ctx.Deadline(); ok {
		f.deadlines = append(f.deadlines, time.Until(d))
	} else {
		f.deadlines = append(f.deadlines, -1)
	}
	if f.err != nil {
		return nil, f.err
	}
	return []shortcodes.GroupRelationEntry{
		{Direction: req.Direction, Type: "Depends on", GroupID: 12, GroupName: "Auth"},
		{Direction: req.Direction, Type: "Depends on", GroupID: 13, GroupName: "Queue"},
		{Direction: req.Direction, Type: "Owned by", GroupID: 12, GroupName: "Auth"},
	}, nil
}

func (f *fakeRelationContext) LoadRelatedGroupDetails(ctx context.Context, ids []uint) ([]models.Group, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.details = append(f.details, ids)
	// 13 is gone by now: its entry keeps empty details.
	return []models.Group{{ID: 12, Description: "Signs tokens", Meta: types.JSON(`{"status":"green"}`)}}, nil
}

func (f *fakeRelationContext) MRQLQueryTimeout() time.Duration { return f.timeout }

func TestRelationListerChargesOneUnitPerStatement(t *testing.T) {
	appCtx, db := setupExecutorTestContext(t)
	f := &fakeRelationContext{MahresourcesContext: appCtx, timeout: time.Minute}
	list := BuildRelationLister(f)
	reqCtx := shortcodes.WithQueryBudget(context.Background(), 3)
	budget := shortcodes.QueryBudgetFrom(reqCtx)

	plain := shortcodes.RelationsRequest{GroupID: 7, Direction: "out", Limit: 20}
	for i := 0; i < 2; i++ {
		if entries, err := list(reqCtx, plain); err != nil || len(entries) != 3 {
			t.Fatalf("call %d: %+v, %v", i, entries, err)
		}
	}
	if len(f.lists) != 1 || budget.Stats().Executions != 1 {
		t.Fatalf("an identical lookup is served from the page cache: %d lists, %d units", len(f.lists), budget.Stats().Executions)
	}
	if d := f.deadlines[0]; d <= 0 || d > time.Minute {
		t.Fatalf("the lookup runs under the MRQL timeout, deadline in %v", d)
	}

	// Details are a second statement and a second unit; the far IDs are
	// asked for once each, and a group that is gone keeps empty details.
	withDetails := plain
	withDetails.Details = true
	entries, err := list(reqCtx, withDetails)
	if err != nil {
		t.Fatal(err)
	}
	if budget.Stats().Executions != 3 {
		t.Fatalf("the details lookup spends two units, the budget shows %d in all", budget.Stats().Executions)
	}
	if len(f.details) != 1 || len(f.details[0]) != 2 {
		t.Fatalf("details asked for %v, want the two distinct far groups", f.details)
	}
	if entries[0].GroupDescription != "Signs tokens" || string(entries[0].GroupMeta) != `{"status":"green"}` ||
		entries[2].GroupDescription != "Signs tokens" || entries[1].GroupDescription != "" || entries[1].GroupMeta != nil {
		t.Fatalf("details merged wrongly: %+v", entries)
	}

	// The budget of 3 is spent: a new lookup is refused before it runs.
	_, err = list(reqCtx, shortcodes.RelationsRequest{GroupID: 8, Direction: "in", Limit: 20})
	if !errors.Is(err, shortcodes.ErrRelationsBudget) || !strings.Contains(err.Error(), "inline query budget exceeded (3 per page)") {
		t.Fatalf("err = %v, want the budget error", err)
	}
	if len(f.lists) != 2 {
		t.Fatalf("a refused lookup must not run, got %d lists", len(f.lists))
	}
	if rows := logRows(t, db, "mrql"); len(rows) != 1 {
		t.Fatalf("the exhausted budget is logged once per page, got %d rows", len(rows))
	}
}

// With one unit left, the edge statement runs and the details statement is
// refused: the whole [relations] renders the budget marker.
func TestRelationListerRefusesDetailsPastTheBudget(t *testing.T) {
	appCtx, _ := setupExecutorTestContext(t)
	f := &fakeRelationContext{MahresourcesContext: appCtx, timeout: time.Minute}
	reqCtx := shortcodes.WithQueryBudget(context.Background(), 1)
	_, err := BuildRelationLister(f)(reqCtx, shortcodes.RelationsRequest{GroupID: 7, Direction: "out", Limit: 5, Details: true})
	if !errors.Is(err, shortcodes.ErrRelationsBudget) {
		t.Fatalf("err = %v, want the budget error", err)
	}
	if len(f.lists) != 1 || len(f.details) != 0 {
		t.Fatalf("lists %d, details %d; want the edge statement only", len(f.lists), len(f.details))
	}
}

func TestRelationListerReportsFailuresAndDoesNotCacheThem(t *testing.T) {
	appCtx, db := setupExecutorTestContext(t)
	f := &fakeRelationContext{MahresourcesContext: appCtx, timeout: time.Minute, err: errors.New("boom")}
	list := BuildRelationLister(f)
	req := shortcodes.RelationsRequest{GroupID: 7, Direction: "out", Limit: 5}
	if _, err := list(context.Background(), req); err == nil {
		t.Fatal("want the error")
	}
	if rows := logRows(t, db, "shortcode"); len(rows) != 1 || !strings.Contains(rows[0].Message, "boom") {
		t.Fatalf("a failed lookup is logged with its error: %+v", rows)
	}
	f.err = nil
	if entries, err := list(context.Background(), req); err != nil || len(entries) != 3 {
		t.Fatalf("a failure is not cached: %+v, %v", entries, err)
	}

	// Unavailable is an answer about the binding, not a failure: no log.
	f.err = shortcodes.ErrRelationsUnavailable
	if _, err := list(context.Background(), shortcodes.RelationsRequest{GroupID: 9, Direction: "out", Limit: 5}); !errors.Is(err, shortcodes.ErrRelationsUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if rows := logRows(t, db, "shortcode"); len(rows) != 1 {
		t.Fatalf("unavailable must not be logged, got %d rows", len(rows))
	}
}

// [relations] and [mrql] draw on one page budget.
func TestRelationListerSharesThePageBudgetWithMRQL(t *testing.T) {
	appCtx, _ := setupExecutorTestContext(t)
	f := &fakeRelationContext{MahresourcesContext: appCtx, timeout: time.Minute}
	executions := 0
	executor := shortcodes.BudgetedExecutor(func(context.Context, string, shortcodes.QueryOptions) (*shortcodes.QueryResult, error) {
		executions++
		return &shortcodes.QueryResult{}, nil
	}, nil)
	reqCtx := shortcodes.WithRelationLister(shortcodes.WithQueryBudget(context.Background(), 3), BuildRelationLister(f))
	out := shortcodes.Process(reqCtx,
		`[relations][mrql query="type = note"][mrql query="type = group"][mrql query="type = resource"]`,
		shortcodes.MetaShortcodeContext{EntityType: "group", EntityID: 7}, nil, executor)
	if len(f.lists) != 1 || executions != 2 {
		t.Fatalf("lists %d, executions %d; want 1 and 2", len(f.lists), executions)
	}
	if !strings.Contains(out, "relations-shortcode") || strings.Count(out, "inline query budget exceeded (3 per page)") != 1 {
		t.Fatalf("want the list and one budget marker, on the third [mrql]: %s", out)
	}
}

func TestBuildRelationListerToleratesAMissingContext(t *testing.T) {
	if BuildRelationLister(nil) != nil {
		t.Fatal("a nil context must give a nil lister")
	}
	var typedNil *application_context.MahresourcesContext
	if BuildRelationLister(typedNil) != nil {
		t.Fatal("a typed-nil context must give a nil lister")
	}
	group := shortcodes.MetaShortcodeContext{EntityType: "group", EntityID: 7}
	if out := shortcodes.Process(buildPageRenderContext(context.Background(), nil), `[relations]`, group, nil, nil); out != "<!-- mr:relations unavailable in this context -->" {
		t.Fatalf("no context renders unavailable, got %q", out)
	}
}

// relationPageContext is a PageRenderContext whose [relations] statements are
// answered by the fake.
type relationPageContext struct {
	countingPageContext
	f *fakeRelationContext
}

func (c *relationPageContext) IsGroupVisibleForTemplateRelations(groupID uint) (bool, error) {
	return c.f.IsGroupVisibleForTemplateRelations(groupID)
}

func (c *relationPageContext) ListGroupRelationsForTemplate(ctx context.Context, req shortcodes.RelationsRequest) ([]shortcodes.GroupRelationEntry, error) {
	return c.f.ListGroupRelationsForTemplate(ctx, req)
}

func (c *relationPageContext) LoadRelatedGroupDetails(ctx context.Context, ids []uint) ([]models.Group, error) {
	return c.f.LoadRelatedGroupDetails(ctx, ids)
}

func (c *relationPageContext) MRQLQueryTimeout() time.Duration     { return c.f.MRQLQueryTimeout() }
func (c *relationPageContext) Logger() *application_context.Logger { return c.f.Logger() }

func TestPageRenderContextInstallsTheRelationLister(t *testing.T) {
	appCtx, _ := setupExecutorTestContext(t)
	f := &fakeRelationContext{MahresourcesContext: appCtx, timeout: time.Minute}
	page := &relationPageContext{f: f}
	page.parent, page.roots = map[uint]int{}, map[uint]int{}
	reqCtx := buildPageRenderContext(context.Background(), page)
	out := shortcodes.Process(reqCtx, `[relations type="Depends on" direction="in" limit="4"]`,
		shortcodes.MetaShortcodeContext{EntityType: "group", EntityID: 7}, nil, nil)
	if !strings.Contains(out, `<a href="/group?id=12">Auth</a>`) {
		t.Fatalf("the page lister was not installed: %s", out)
	}
	want := shortcodes.RelationsRequest{GroupID: 7, Type: "Depends on", Direction: "in", Limit: 4}
	if len(f.lists) != 1 || f.lists[0] != want {
		t.Fatalf("forwarded %+v, want %+v", f.lists, want)
	}
}

func TestRelationListerNoStatementLeavesBudgetUnchanged(t *testing.T) {
	appCtx, db := setupExecutorTestContext(t)
	root := models.Group{Name: "scope root"}
	if err := db.Create(&root).Error; err != nil {
		t.Fatal(err)
	}
	principal := &auth.Principal{Role: models.RoleGuest, ScopeGroupID: &root.ID}
	for _, tt := range []struct {
		name string
		app  *application_context.MahresourcesContext
		want string
	}{
		{"MRQL-only binding", appCtx.WithMRQLPrincipal(context.Background(), principal), "<!-- mr:relations unavailable in this context -->"},
		{"outside carrier", appCtx.WithPrincipal(principal), "None"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			reqCtx := shortcodes.WithQueryBudget(context.Background(), 1)
			reqCtx = shortcodes.WithRelationLister(reqCtx, BuildRelationLister(tt.app))
			budget := shortcodes.QueryBudgetFrom(reqCtx)
			for _, used := range []int{0, 1} {
				if used == 1 && !budget.Allow() {
					t.Fatal("the lookup already spent the only unit")
				}
				got := shortcodes.Process(reqCtx, `[relations][item path="GroupName"][else]None[/relations]`,
					shortcodes.MetaShortcodeContext{EntityType: "group", EntityID: root.ID + 1}, nil, nil)
				if got != tt.want || budget.Stats().Executions != used || budget.Stats().Exceeded {
					t.Errorf("got %q with stats %+v; want %q with %d executions and no refusal", got, budget.Stats(), tt.want, used)
				}
			}
		})
	}
}
