package api_tests

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"mahresources/server/api_handlers"
	"mahresources/shortcodes"
)

// toolFixture is a small library plus an executor over it.
type toolFixture struct {
	tc                   *TestContext
	cat                  *models.Category
	root, child, outside *models.Group
	note                 *models.Note
	exec                 application_context.TemplateToolExecutor
	cfg                  api_handlers.TemplateRunToolsConfig
	longMeta             string
}

func newToolFixture(t *testing.T, mutateCfg func(*application_context.MahresourcesConfig), tweak func(*api_handlers.TemplateRunToolsConfig, *toolFixture)) *toolFixture {
	t.Helper()
	return newToolFixtureOn(t, setupTestEnvWithConfig(t, mutateCfg), tweak)
}

func newToolFixtureOn(t *testing.T, tc *TestContext, tweak func(*api_handlers.TemplateRunToolsConfig, *toolFixture)) *toolFixture {
	t.Helper()
	f := &toolFixture{tc: tc, longMeta: strings.Repeat("long-meta-value-", 40)}
	create := func(v any) {
		t.Helper()
		if err := tc.DB.Create(v).Error; err != nil {
			t.Fatal(err)
		}
	}
	f.cat = &models.Category{Name: "Projects", Description: "project groups", CustomHeader: "<h1>[meta path=\"title\"]</h1>"}
	create(f.cat)
	f.root = &models.Group{Name: "Root"}
	f.outside = &models.Group{Name: "Elsewhere"}
	create(f.root)
	create(f.outside)
	f.child = &models.Group{Name: "Child", OwnerId: &f.root.ID, CategoryId: &f.cat.ID,
		Meta: types.JSON(fmt.Sprintf(`{"title":"Hello","long":%q,"n":12345678901234567890}`, f.longMeta))}
	create(f.child)
	f.note = &models.Note{Name: "A note", OwnerId: &f.child.ID}
	create(f.note)
	create(&models.TemplatePartial{Name: "badge", Description: "a badge", Content: "<b>badge</b>"})

	f.cfg = api_handlers.TemplateRunToolsConfig{
		Start:           tc.AppCtx,
		Principal:       auth.SystemPrincipal(),
		EntityType:      "group",
		CarrierID:       f.cat.ID,
		PreviewEntityID: f.child.ID,
		FieldValue: func(field string) (string, bool) {
			if field == "CustomHeader" {
				return f.cat.CustomHeader, true
			}
			return "", false
		},
	}
	if tweak != nil {
		tweak(&f.cfg, f)
	}
	f.exec = api_handlers.NewTemplateToolExecutor(f.cfg)
	return f
}

func (f *toolFixture) call(t *testing.T, tool string, args any) application_context.TemplateToolOutput {
	t.Helper()
	raw, _ := json.Marshal(args)
	out, err := f.exec.Execute(context.Background(), application_context.TemplateToolInvocation{CallID: "c1", Name: tool, Arguments: string(raw), Turn: 1})
	if err != nil {
		t.Fatalf("%s: Execute error %v", tool, err)
	}
	return out
}

// payload strips the fence a result is wrapped in.
func payload(t *testing.T, out application_context.TemplateToolOutput) string {
	t.Helper()
	if out.IsError {
		return out.Text
	}
	lines := strings.SplitN(out.Text, "\n", 2)
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "<<<BEGIN tool ") {
		t.Fatalf("result is not fenced: %q", out.Text)
	}
	body := lines[1]
	if i := strings.LastIndex(body, "\n<<<END tool "); i >= 0 {
		body = body[:i]
	}
	return body
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, s)
	}
	return m
}

func TestToolGetEntityReturnsFullMetaAndCarrier(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	out := f.call(t, "get_entity", map[string]any{"type": "group", "id": f.child.ID})
	if out.IsError {
		t.Fatal(out.Text)
	}
	body := payload(t, out)
	if !strings.Contains(body, f.longMeta) {
		t.Error("Meta was clipped; the Tool returns it whole")
	}
	if !strings.Contains(body, "12345678901234567890") {
		t.Error("a large Meta number lost precision")
	}
	m := decode(t, body)
	carrier := m["carrier"].(map[string]any)
	if carrier["name"] != "Projects" || uint(carrier["id"].(float64)) != f.cat.ID {
		t.Errorf("carrier = %v", carrier)
	}
	if entity := m["entity"].(map[string]any); entity["Name"] != "Child" || entity["Owner"] != "Root" {
		t.Errorf("entity = %v", entity)
	}
}

func TestToolResultsAreCappedAndSayWhenCut(t *testing.T) {
	f := newToolFixture(t, nil, func(_ *api_handlers.TemplateRunToolsConfig, f *toolFixture) {
		huge := strings.Repeat("é", 20<<10)
		f.tc.DB.Model(&models.Group{}).Where("id = ?", f.child.ID).Update("meta", types.JSON(fmt.Sprintf(`{"big":%q}`, huge)))
	})
	out := f.call(t, "get_entity", map[string]any{"type": "group", "id": f.child.ID})
	if !out.Truncated || out.FullBytes <= application_context.ToolCapGetEntityBytes {
		t.Fatalf("truncated=%v full=%d", out.Truncated, out.FullBytes)
	}
	if !strings.Contains(out.Text, fmt.Sprintf("bytes shown]")) || !strings.Contains(out.Text, "[cut: ") {
		t.Errorf("no cut notice")
	}
	if out.Bytes > application_context.ToolCapGetEntityBytes+200 {
		t.Errorf("sent %d bytes for an %d cap", out.Bytes, application_context.ToolCapGetEntityBytes)
	}
}

func TestToolMissingAndOutOfScopeEntitiesShareOneText(t *testing.T) {
	checkToolScopeText(t, newToolFixture(t, nil, nil))
}

func checkToolScopeText(t *testing.T, f *toolFixture) {
	t.Helper()
	principal := &auth.Principal{UserID: 1, Username: "confined", Role: models.RoleEditor, ScopeGroupID: &f.child.ID}
	scoped := newToolFixtureExec(f, func(c *api_handlers.TemplateRunToolsConfig) {
		c.Start = f.tc.AppCtx.WithPrincipal(principal)
		c.Principal = principal
	})
	inScope := scoped(t, "get_entity", map[string]any{"type": "group", "id": f.child.ID})
	if inScope.IsError {
		t.Fatalf("in-scope entity refused: %s", inScope.Text)
	}
	missing := scoped(t, "get_entity", map[string]any{"type": "group", "id": 99999})
	outside := scoped(t, "get_entity", map[string]any{"type": "group", "id": f.outside.ID})
	if !missing.IsError || missing.Text != outside.Text {
		t.Fatalf("missing = %q, out of scope = %q; they must be identical errors", missing.Text, outside.Text)
	}
	// Mutation check: unscoped, the outside group is readable, so the refusal
	// above came from the scope.
	if got := f.call(t, "get_entity", map[string]any{"type": "group", "id": f.outside.ID}); got.IsError {
		t.Fatalf("unscoped read of the outside group failed: %s", got.Text)
	}
	// The same holds for the list Tools.
	m := scoped(t, "list_related", map[string]any{"type": "group", "id": f.outside.ID, "direction": "up"})
	if m.Text != missing.Text {
		t.Errorf("list_related outside = %q", m.Text)
	}
}

func newToolFixtureExec(f *toolFixture, mutate func(*api_handlers.TemplateRunToolsConfig)) func(*testing.T, string, any) application_context.TemplateToolOutput {
	cfg := f.cfg
	mutate(&cfg)
	exec := api_handlers.NewTemplateToolExecutor(cfg)
	return func(t *testing.T, tool string, args any) application_context.TemplateToolOutput {
		t.Helper()
		raw, _ := json.Marshal(args)
		out, err := exec.Execute(context.Background(), application_context.TemplateToolInvocation{Name: tool, Arguments: string(raw)})
		if err != nil {
			t.Fatal(err)
		}
		return out
	}
}

func TestToolListRelatedMembersCarriersAndTemplate(t *testing.T) {
	checkToolReaders(t, newToolFixture(t, nil, nil))
}

func checkToolReaders(t *testing.T, f *toolFixture) {
	t.Helper()
	down := decode(t, payload(t, f.call(t, "list_related", map[string]any{"type": "group", "id": f.root.ID, "direction": "down"})))
	entries := down["entries"].([]any)
	if len(entries) != 1 || entries[0].(map[string]any)["name"] != "Child" || down["more"] != false {
		t.Fatalf("down = %v", down)
	}
	members := decode(t, payload(t, f.call(t, "list_members", map[string]any{"carrierType": "category", "id": f.cat.ID})))
	if got := members["entries"].([]any); len(got) != 1 || got[0].(map[string]any)["name"] != "Child" {
		t.Fatalf("members = %v", members)
	}
	carriers := decode(t, payload(t, f.call(t, "list_carriers", map[string]any{"carrierType": "category", "nameQuery": "Proj"})))
	if got := carriers["entries"].([]any); len(got) != 1 || got[0].(map[string]any)["name"] != "Projects" {
		t.Fatalf("carriers = %v", carriers)
	}
	tpl := decode(t, payload(t, f.call(t, "get_template", map[string]any{"carrierType": "category", "id": f.cat.ID, "fields": []string{"CustomHeader"}})))
	if tpl["fields"].(map[string]any)["CustomHeader"] != f.cat.CustomHeader {
		t.Fatalf("template = %v", tpl)
	}
	if out := f.call(t, "get_template", map[string]any{"carrierType": "category", "id": f.cat.ID, "fields": []string{"Nope"}}); !out.IsError || !strings.Contains(out.Text, "CustomHeader") {
		t.Errorf("unknown field: %q", out.Text)
	}
}

func TestToolListRevisionsIsBoundedOnTheSQLAndPages(t *testing.T) {
	checkToolRevisionPaging(t, newToolFixture(t, nil, nil))
}

func checkToolRevisionPaging(t *testing.T, f *toolFixture) {
	t.Helper()
	for i := 0; i < 59; i++ {
		_, err := f.tc.AppCtx.UpdateCategory(&query_models.CategoryEditor{ID: f.cat.ID, CategoryCreator: query_models.CategoryCreator{
			Name: f.cat.Name, CustomHeader: fmt.Sprintf("h%d", i),
		}})
		if err != nil {
			t.Fatal(err)
		}
	}
	var sqls []string
	name := "tooltest:revsql"
	_ = f.tc.DB.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "template_revisions" {
			sqls = append(sqls, tx.Statement.SQL.String())
		}
	})
	t.Cleanup(func() { _ = f.tc.DB.Callback().Query().Remove(name) })

	// The carrier being edited is the default.
	first := decode(t, payload(t, f.call(t, "list_revisions", map[string]any{})))
	if got := len(first["revisions"].([]any)); got != 50 || first["more"] != true {
		t.Fatalf("page 1 = %d more=%v", got, first["more"])
	}
	bounded := false
	for _, s := range sqls {
		if strings.Contains(s, "LIMIT 51") || strings.Contains(s, "LIMIT $") {
			bounded = true
		}
		if strings.Contains(s, "template_revisions") && strings.Contains(s, "carrier_type") && !strings.Contains(s, "LIMIT") {
			t.Errorf("an unbounded revision query ran: %s", s)
		}
	}
	if !bounded {
		t.Fatalf("no LIMIT 51 revision query in %v", sqls)
	}
	second := decode(t, payload(t, f.call(t, "list_revisions", map[string]any{"offset": 50})))
	if got := len(second["revisions"].([]any)); got != 10 || second["more"] != false {
		t.Fatalf("page 2 = %d more=%v", got, second["more"])
	}
	rev := decode(t, payload(t, f.call(t, "get_revision", map[string]any{"number": 2, "field": "CustomHeader"})))
	if rev["value"] != "h0" {
		t.Errorf("revision 2 CustomHeader = %v", rev["value"])
	}
	diff := f.call(t, "diff_revisions", map[string]any{"from": 1, "to": 3})
	if diff.IsError || !strings.Contains(payload(t, diff), "CustomHeader") {
		t.Errorf("diff = %q", diff.Text)
	}
}

func TestToolRevisionsFollowTheCarrierEditCapability(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	editor := newToolFixtureExec(f, func(c *api_handlers.TemplateRunToolsConfig) {
		c.Start = f.tc.AppCtx.WithPrincipal(&auth.Principal{UserID: 2, Role: models.RoleEditor})
	})
	// An editor may read carriers, but not a Category's revisions.
	if out := editor(t, "get_template", map[string]any{"carrierType": "category", "id": f.cat.ID}); out.IsError {
		t.Fatalf("an editor must read any carrier: %s", out.Text)
	}
	if out := editor(t, "list_carriers", map[string]any{"carrierType": "category"}); out.IsError {
		t.Fatalf("list_carriers refused: %s", out.Text)
	}
	out := editor(t, "list_revisions", map[string]any{})
	if !out.IsError || !strings.Contains(out.Text, "not allowed") {
		t.Fatalf("editor read a Category's revisions: %q", out.Text)
	}
	// The unscoped admin reads them.
	if ok := f.call(t, "list_revisions", map[string]any{}); ok.IsError {
		t.Fatalf("admin refused: %s", ok.Text)
	}
}

func TestToolLintCheckMRQLPartialsAndDocs(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	lint := decode(t, payload(t, f.call(t, "lint", map[string]any{"field": "CustomHeader", "content": `[partial name="badge"][partial name="ghost"]`})))
	text, _ := json.Marshal(lint)
	if !strings.Contains(string(text), "ghost") || strings.Contains(string(text), `"badge"`) {
		t.Errorf("lint = %s", text)
	}
	// Omitted content lints the Turn-start snapshot's value.
	snap := decode(t, payload(t, f.call(t, "lint", map[string]any{"field": "CustomHeader"})))
	if snap["errors"].(float64) != 0 {
		t.Errorf("snapshot lint = %v", snap)
	}
	if out := f.call(t, "lint", map[string]any{"field": "CustomSidebar"}); !out.IsError || !strings.Contains(out.Text, "no content") {
		t.Errorf("no snapshot value: %q", out.Text)
	}
	if out := f.call(t, "lint", map[string]any{"field": "Bogus", "content": "x"}); !out.IsError {
		t.Error("an unknown field was linted")
	}
	over := f.call(t, "lint", map[string]any{"field": "CustomHeader", "content": strings.Repeat("a", application_context.MaxTemplateGeneratedContentLength+1)})
	if !over.IsError && !strings.Contains(over.Text, "bytes") {
		t.Errorf("oversize content: %q", over.Text)
	}

	ok := decode(t, payload(t, f.call(t, "check_mrql", map[string]any{"query": `type = "note"`})))
	if ok["ok"] != true {
		t.Errorf("check_mrql ok = %v", ok)
	}
	bad := decode(t, payload(t, f.call(t, "check_mrql", map[string]any{"query": `type = `})))
	if bad["ok"] != false || bad["error"] == "" {
		t.Errorf("check_mrql bad = %v", bad)
	}

	if body := payload(t, f.call(t, "get_partial", map[string]any{"name": "badge"})); !strings.Contains(body, "<b>badge</b>") {
		t.Errorf("get_partial = %s", body)
	}
	if out := f.call(t, "get_partial", map[string]any{"name": "ghost"}); !out.IsError {
		t.Error("a missing partial was returned")
	}
	if body := payload(t, f.call(t, "get_shortcode_docs", map[string]any{"name": "meta"})); !strings.Contains(body, "meta") {
		t.Errorf("docs = %s", body)
	}
	if out := f.call(t, "get_shortcode_docs", map[string]any{"name": "nothing-like-this"}); !out.IsError {
		t.Error("docs for an unknown shortcode")
	}
}

func TestToolPreviewRendersAndCarriesTheMRQLOptIn(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	out := f.call(t, "preview", map[string]any{"field": "CustomHeader", "content": `<h1>[meta path="title"]</h1>`})
	if out.IsError {
		t.Fatal(out.Text)
	}
	if html := decode(t, payload(t, out))["html"].(string); !strings.Contains(html, "Hello") {
		t.Errorf("html = %q", html)
	}
	// css defaults from the companion field's snapshot value, and a bare
	// preview names its entity from the config.
	if out := f.call(t, "preview", map[string]any{"field": "CustomHeader", "content": "x", "entityType": "group", "entityId": 99999}); !out.IsError {
		t.Error("a preview against a missing entity rendered")
	}

	// MRQL: off renders the inert comment and runs no query; on runs it.
	query := `[mrql query='type = "note"'][/mrql]`
	var runs atomic.Int32
	name := "tooltest:notequery"
	_ = f.tc.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "notes" {
			runs.Add(1)
		}
	})
	t.Cleanup(func() { _ = f.tc.DB.Callback().Query().Remove(name) })
	// Loading the entity reads notes too, so compare against a preview with
	// no [mrql] in it.
	f.call(t, "preview", map[string]any{"field": "CustomHeader", "content": "plain"})
	baseline := runs.Load()
	runs.Store(0)
	off := decode(t, payload(t, f.call(t, "preview", map[string]any{"field": "CustomHeader", "content": query})))["html"].(string)
	if strings.Contains(off, "A note") || runs.Load() != baseline {
		t.Fatalf("opt-in off ran a query (%d, baseline %d): %s", runs.Load(), baseline, off)
	}
	runs.Store(0)
	on := newToolFixtureExec(f, func(c *api_handlers.TemplateRunToolsConfig) { c.OptIns.PreviewMRQL = true })
	onOut := decode(t, payload(t, on(t, "preview", map[string]any{"field": "CustomHeader", "content": query})))["html"].(string)
	if !strings.Contains(onOut, "A note") || runs.Load() <= baseline {
		t.Fatalf("opt-in on did not run the query (%d): %s", runs.Load(), onOut)
	}
}

func TestToolMRQLExhaustionWritesNoLogRowInAModelPreview(t *testing.T) {
	f := newToolFixture(t, func(c *application_context.MahresourcesConfig) { c.MRQLPageQueryBudget = 1 }, nil)
	on := newToolFixtureExec(f, func(c *api_handlers.TemplateRunToolsConfig) { c.OptIns.PreviewMRQL = true })
	two := `[mrql query='type = "note"'][/mrql][mrql query='type = "resource"'][/mrql]`
	logRows := func() int64 {
		var n int64
		f.tc.DB.Model(&models.LogEntry{}).Where("entity_type = ?", "mrql").Count(&n)
		return n
	}
	before := logRows()
	out := on(t, "preview", map[string]any{"field": "CustomHeader", "content": two})
	if !strings.Contains(out.Text, "budget exceeded") {
		t.Fatalf("the second query was not refused by the budget (the test cannot tell): %s", out.Text)
	}
	if logRows() != before {
		t.Errorf("a model preview wrote a budget-exhaustion log row")
	}
}

func TestToolPanicIsContained(t *testing.T) {
	f := newToolFixture(t, nil, func(c *api_handlers.TemplateRunToolsConfig, _ *toolFixture) {
		c.FieldValue = func(string) (string, bool) { panic("boom") }
	})
	out := f.call(t, "lint", map[string]any{"field": "CustomHeader"})
	if !out.IsError || out.Text != "error: the Tool failed" {
		t.Fatalf("out = %+v", out)
	}
	if again := f.call(t, "get_entity", map[string]any{"type": "group", "id": f.child.ID}); again.IsError {
		t.Error("the executor is unusable after a panic")
	}
}

func TestToolBadArgumentsAreResultsNotFailures(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	out, err := f.exec.Execute(context.Background(), application_context.TemplateToolInvocation{Name: "get_entity", Arguments: `{"type":"group"`})
	if err != nil || !out.IsError || !strings.Contains(out.Text, "not valid JSON") {
		t.Fatalf("out=%+v err=%v", out, err)
	}
}

// A blocked read is released by cancelling the Tool's context, because every
// Tool runs on a copy of the context rebased onto it.
func TestToolCancellationReachesABlockedDBRead(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	started := make(chan struct{})
	name := "tooltest:block"
	_ = f.tc.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "groups" {
			return
		}
		select {
		case started <- struct{}{}:
		default:
		}
		<-tx.Statement.Context.Done()
		_ = tx.AddError(tx.Statement.Context.Err())
	})
	t.Cleanup(func() { _ = f.tc.DB.Callback().Query().Remove(name) })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := f.exec.Execute(ctx, application_context.TemplateToolInvocation{Name: "get_entity", Arguments: fmt.Sprintf(`{"type":"group","id":%d}`, f.child.ID)})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the read never started")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelling did not release the blocked read")
	}
}

func TestToolCheckAccessSwapsOnUnchangedAccessAndErrorsOnChanged(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	var calls atomic.Int32
	var changed atomic.Bool
	scopedFor := f.tc.AppCtx.WithPrincipal(&auth.Principal{UserID: 1, Role: models.RoleEditor, ScopeGroupID: &f.child.ID})
	exec := api_handlers.NewTemplateToolExecutor(api_handlers.TemplateRunToolsConfig{
		Start: f.tc.AppCtx, EntityType: "group", CarrierID: f.cat.ID,
		Access: func(ctx context.Context) (*application_context.MahresourcesContext, error) {
			calls.Add(1)
			if changed.Load() {
				return nil, application_context.ErrTemplateRunAccessChanged
			}
			return scopedFor, nil
		},
	})
	read := func(id uint) bool {
		out, err := exec.Execute(context.Background(), application_context.TemplateToolInvocation{Name: "get_entity", Arguments: fmt.Sprintf(`{"type":"group","id":%d}`, id)})
		if err != nil {
			t.Fatal(err)
		}
		return !out.IsError
	}
	if !read(f.outside.ID) {
		t.Fatal("the initial (unscoped) context could not read the outside group")
	}
	if err := exec.CheckAccess(context.Background()); err != nil {
		t.Fatal(err)
	}
	if read(f.outside.ID) {
		t.Fatal("CheckAccess did not rebind the scoped context")
	}
	changed.Store(true)
	if err := exec.CheckAccess(context.Background()); !errors.Is(err, application_context.ErrTemplateRunAccessChanged) {
		t.Fatalf("err = %v", err)
	}
	if read(f.outside.ID) {
		t.Fatal("a failed CheckAccess widened the bound context")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := exec.CheckAccess(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled CheckAccess = %v", err)
	}
	if calls.Load() != 2 {
		t.Errorf("factory calls = %d, want 2 (a cancelled context must not reach it)", calls.Load())
	}
}

func TestToolParallelPreviewsStayWithinTwo(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	var inFlight, peak atomic.Int32
	name := "tooltest:preview-concurrency"
	_ = f.tc.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "categories" {
			return
		}
		n := inFlight.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(30 * time.Millisecond)
		inFlight.Add(-1)
	})
	t.Cleanup(func() { _ = f.tc.DB.Callback().Query().Remove(name) })
	done := make(chan struct{})
	for i := 0; i < 6; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			f.call(t, "preview", map[string]any{"field": "CustomHeader", "content": "x"})
		}()
	}
	for i := 0; i < 6; i++ {
		<-done
	}
	if peak.Load() > 2 {
		t.Fatalf("%d previews rendered at once, the limit is 2", peak.Load())
	}
	if peak.Load() == 0 {
		t.Skip("the preview path read no category, so the probe saw nothing")
	}
}

// With the plugin opt-in on, a model-written preview runs the installed plugin
// read-only: its writes fail, its reads render, and the opt-in off reaches no
// plugin at all. A human preview of the same shortcode is unchanged.
func TestToolPreviewRunsPluginsReadOnly(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "wp"), 0o755); err != nil {
		t.Fatal(err)
	}
	lua := `
plugin = { name = "wp", version = "1.0", api_version = 1, capabilities = { "render", "db:read", "db:write", "media" } }
function init()
  mah.shortcode({name = "del", label = "d", render = function(ctx) mah.db.delete_note(tonumber(ctx.attrs.id)); return "DELETED" end})
  mah.shortcode({name = "trim", label = "t", render = function(ctx) mah.media.trim(1, 0, 1); return "TRIMMED" end})
  mah.shortcode({name = "get", label = "g", render = function(ctx) local g, err = mah.db.get_group(tonumber(ctx.attrs.id)); if not g then return "ERR:" .. tostring(err) .. " id=" .. tostring(ctx.attrs.id) end; return "GROUP:" .. g.name end})
  mah.shortcode({name = "late", label = "l", render = function(ctx) mah.shortcode({name = "x", label = "x", render = function() return "" end}); return "REGISTERED" end})
end
`
	if err := os.WriteFile(filepath.Join(dir, "wp", "plugin.lua"), []byte(lua), 0o644); err != nil {
		t.Fatal(err)
	}
	f := newToolFixture(t, func(c *application_context.MahresourcesConfig) { c.PluginPath = dir }, nil)
	pm := f.tc.AppCtx.PluginManager()
	if pm == nil {
		t.Skip("no plugin manager in this environment")
	}
	if err := pm.EnablePlugin("wp"); err != nil {
		t.Fatal(err)
	}
	on := newToolFixtureExec(f, func(c *api_handlers.TemplateRunToolsConfig) { c.OptIns.PreviewPlugins = true })
	html := func(content string) string {
		return decode(t, payload(t, on(t, "preview", map[string]any{"field": "CustomHeader", "content": content})))["html"].(string)
	}
	noteID := f.note.ID

	if got := html(fmt.Sprintf(`[plugin:wp:get id="%d"]`, f.child.ID)); !strings.Contains(got, "GROUP:Child") {
		t.Fatalf("a read did not render: %q", got)
	}
	for name, content := range map[string]string{
		"delete":   fmt.Sprintf(`[plugin:wp:del id="%d"]`, noteID),
		"trim":     `[plugin:wp:trim]`,
		"register": `[plugin:wp:late]`,
	} {
		got := html(content)
		for _, ran := range []string{"DELETED", "TRIMMED", "REGISTERED"} {
			if strings.Contains(got, ran) {
				t.Errorf("%s: the plugin ran past the guard: %q", name, got)
			}
		}
	}
	var n int64
	f.tc.DB.Model(&models.Note{}).Where("id = ?", noteID).Count(&n)
	if n != 1 {
		t.Fatal("a model preview deleted the note")
	}
	if pm.GetPluginShortcode("plugin:wp:x") != nil {
		t.Error("a model preview registered a shortcode")
	}

	// Opt-in off: the plugin is not reached at all.
	off := decode(t, payload(t, f.call(t, "preview", map[string]any{"field": "CustomHeader", "content": fmt.Sprintf(`[plugin:wp:get id="%d"]`, f.child.ID)})))["html"].(string)
	if strings.Contains(off, "GROUP:") {
		t.Errorf("opt-in off reached the plugin: %q", off)
	}

	// A human preview of the same shortcode behaves as today: the write runs.
	if err := f.tc.DB.Create(&models.Note{Name: "doomed"}).Error; err != nil {
		t.Fatal(err)
	}
	var doomed models.Note
	f.tc.DB.Where("name = ?", "doomed").First(&doomed)
	rr := f.tc.MakeRequest("POST", "/v1/category/previewTemplate", map[string]any{
		"entityId": f.child.ID, "content": fmt.Sprintf(`[plugin:wp:del id="%d"]`, doomed.ID),
	})
	if rr.Code != 200 || !strings.Contains(rr.Body.String(), "DELETED") {
		t.Fatalf("human preview: %d %s", rr.Code, rr.Body.String())
	}
}

// A partial lookup blocked in the database is released when the Tool's context
// ends, and the lint says it could not check rather than calling the partial
// missing.
func TestToolLintPartialLookupIsCancelledWithTheTool(t *testing.T) {
	f := newToolFixture(t, nil, nil)
	name := "tooltest:block-partials"
	_ = f.tc.DB.Callback().Query().Before("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table == "template_partials" {
			<-tx.Statement.Context.Done()
			_ = tx.AddError(tx.Statement.Context.Err())
		}
	})
	t.Cleanup(func() { _ = f.tc.DB.Callback().Query().Remove(name) })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	out, err := f.exec.Execute(ctx, application_context.TemplateToolInvocation{
		Name: "lint", Arguments: `{"field":"CustomHeader","content":"[partial name=\"badge\"]"}`,
	})
	if time.Since(start) > 3*time.Second {
		t.Fatal("the blocked partial lookup was not released by the Tool's deadline")
	}
	// The deadline passed during the lookup, so no result is released.
	if !errors.Is(err, context.DeadlineExceeded) || out.Text != "" {
		t.Errorf("out=%q err=%v, want the context error and no result", out.Text, err)
	}
}

func TestToolLintUsesTheInjectedValidatorAndTheTurnSnapshot(t *testing.T) {
	var gotField, gotContent string
	f := newToolFixture(t, nil, func(c *api_handlers.TemplateRunToolsConfig, _ *toolFixture) {
		c.Validate = func(_ context.Context, _ application_context.TemplateGenerationInput, field, content string) []shortcodes.LintIssue {
			gotField, gotContent = field, content
			return []shortcodes.LintIssue{{Severity: shortcodes.SeverityError, Message: "from the fake"}}
		}
	})
	raw, _ := json.Marshal(map[string]any{"field": "CustomHeader"})
	out, err := f.exec.Execute(context.Background(), application_context.TemplateToolInvocation{
		Name: "lint", Arguments: string(raw), Snapshot: map[string]string{"CustomHeader": "from snapshot"},
	})
	if err != nil || !strings.Contains(out.Text, "from the fake") {
		t.Fatalf("out=%q err=%v", out.Text, err)
	}
	if gotField != "CustomHeader" || gotContent != "from snapshot" {
		t.Errorf("validator saw %q %q; the snapshot value must win over the editor value", gotField, gotContent)
	}
	// submit is the loop's, never the executor's.
	if out := f.call(t, "submit", map[string]any{"fields": map[string]string{}}); !out.IsError {
		t.Error("the executor ran submit")
	}
}
