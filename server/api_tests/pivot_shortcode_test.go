package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gorm.io/gorm"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/types"
)

// [pivot] reads its one aggregated query through the request's
// principal-scoped MRQL executor, and in count mode every cell and total links
// to the MRQL page filtered to what it counted.

type pivotFixture struct {
	root, inboxA, inboxB, work, outside, tagged *models.Group
}

// buildPivotFixture seeds notes under root whose meta.status differs in case,
// number versus string and boolean versus string, owned by two groups that
// share a name up to case, plus one note outside root.
func buildPivotFixture(t *testing.T, tc *TestContext) pivotFixture {
	t.Helper()
	f := pivotFixture{root: &models.Group{Name: "pv-root"}, outside: &models.Group{Name: "pv-outside"}}
	mkSim(t, tc, f.root)
	mkSim(t, tc, f.outside)
	f.inboxA = &models.Group{Name: "Inbox", OwnerId: &f.root.ID}
	f.inboxB = &models.Group{Name: "inbox", OwnerId: &f.root.ID}
	f.work = &models.Group{Name: "Work", OwnerId: &f.root.ID}
	f.tagged = &models.Group{Name: "Related", OwnerId: &f.root.ID}
	for _, g := range []*models.Group{f.inboxA, f.inboxB, f.work, f.tagged} {
		mkSim(t, tc, g)
	}
	mk := func(owner *models.Group, meta string) *models.Note {
		n := &models.Note{Name: "pv-note", OwnerId: &owner.ID, Meta: types.JSON(meta)}
		mkSim(t, tc, n)
		return n
	}
	related := mk(f.inboxA, `{"status":"open","hours":1.5}`)
	mk(f.inboxA, `{"status":"Open","hours":"2"}`)
	mk(f.inboxA, `{"status":3,"hours":3}`)
	mk(f.inboxA, `{"status":"true"}`)
	mk(f.inboxB, `{"status":"open","hours":true}`)
	mk(f.inboxB, `{"status":"3","hours":"x"}`)
	mk(f.inboxB, `{"status":true}`)
	mk(f.work, `{"hours":4}`)
	mk(f.work, `{"status":"open"}`)
	mk(f.work, `{"status":true}`)
	mk(f.outside, `{"status":"open","hours":100}`)
	if err := tc.DB.Model(related).Association("Groups").Append(f.tagged, f.work); err != nil {
		t.Fatal(err)
	}
	return f
}

// pivotGridCell is one rendered cell: the visible number, the link's MRQL
// filter (empty when unlinked) and the link's accessible name.
type pivotGridCell struct {
	text, query, label string
}

var (
	pvRow  = regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	pvCell = regexp.MustCompile(`(?s)<(th|td)[^>]*>(.*?)</(?:th|td)>`)
	pvLink = regexp.MustCompile(`<a href="([^"]*)" [^>]*aria-label="([^"]*)">`)
	pvTag  = regexp.MustCompile(`<[^>]*>`)
	pvSR   = regexp.MustCompile(`<span class="sr-only">[^<]*</span>`)
)

// pivotRows maps each row's header text (case-folded) to its cells, the
// header row under "#head" and the footer under "#total".
func pivotRows(t *testing.T, out string) map[string][]pivotGridCell {
	t.Helper()
	start := strings.Index(out, `<div class="mrql-pivot`)
	if start < 0 {
		t.Fatalf("no pivot in %s", out)
	}
	rows := map[string][]pivotGridCell{}
	for i, tr := range pvRow.FindAllStringSubmatch(out[start:], -1) {
		var cells []pivotGridCell
		for _, m := range pvCell.FindAllStringSubmatch(tr[1], -1) {
			c := pivotGridCell{text: html.UnescapeString(strings.TrimSpace(pvTag.ReplaceAllString(pvSR.ReplaceAllString(m[2], ""), "")))}
			if l := pvLink.FindStringSubmatch(m[2]); l != nil {
				u, err := url.Parse(html.UnescapeString(l[1]))
				if err != nil {
					t.Fatal(err)
				}
				c.query, c.label = u.Query().Get("q"), html.UnescapeString(l[2])
			}
			cells = append(cells, c)
		}
		key := strings.ToLower(cells[0].text)
		if i == 0 {
			key = "#head"
		} else if cells[0].text == "Total" {
			key = "#total"
		}
		rows[key] = cells
	}
	return rows
}

// mrqlNoteCount runs a filter through POST /v1/mrql as bearer.
func mrqlNoteCount(t *testing.T, tc *TestContext, bearer, query string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"query": query})
	rr := doReq(tc, http.MethodPost, "/v1/mrql", map[string]string{"Accept": "application/json", "Content-Type": "application/json", "Authorization": bearer}, nil, bytes.NewReader(body))
	if rr.Code != http.StatusOK {
		t.Fatalf("mrql %q: %d %s", query, rr.Code, rr.Body.String())
	}
	var res struct {
		Notes []json.RawMessage `json:"notes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	return len(res.Notes)
}

// checkEveryLinkCountsItsCell opens every link of a count pivot as bearer and
// asserts the MRQL page lists exactly as many notes as the number linked.
func checkEveryLinkCountsItsCell(t *testing.T, tc *TestContext, bearer string, rows map[string][]pivotGridCell) int {
	t.Helper()
	links := 0
	for key, cells := range rows {
		for _, c := range cells {
			if c.query == "" {
				continue
			}
			links++
			want, err := strconv.Atoi(c.text)
			if err != nil {
				t.Fatalf("%s: linked text %q is not a count", key, c.text)
			}
			if got := mrqlNoteCount(t, tc, bearer, c.query); got != want {
				t.Errorf("%s: %q links %q, which lists %d notes", key, c.label, c.query, got)
			}
		}
	}
	return links
}

func pivotTexts(cells []pivotGridCell) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = c.text
	}
	return out
}

// checkPivotLinkCountsMatchCells is shared with the Postgres run. postgres
// selects the one documented difference: SQLite groups a JSON true with 1,
// Postgres with the string "true".
func checkPivotLinkCountsMatchCells(t *testing.T, tc *TestContext, postgres bool) {
	f := buildPivotFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)
	tpl := fmt.Sprintf(`[pivot query="type = note" rows="meta.status" cols="owner" scope="%d"]`, f.root.ID)
	rows := pivotRows(t, deferredHTML(t, tc, admin, "group", f.root.ID, tpl))

	// Inbox and inbox are two groups whose names MRQL compares as equal; which
	// spelling heads the merged column depends on the database's row order.
	if got := strings.ToLower(fmt.Sprint(pivotTexts(rows["#head"]))); got != "[status inbox work total]" {
		t.Errorf("columns: %v", got)
	}
	want := map[string]string{
		"open":   "[open 3 1 4]",
		"3":      "[3 2 0 2]",
		"(none)": "[(none) 0 1 1]",
		"#total": "[Total 7 3 10]",
	}
	if postgres {
		want["true"] = "[true 2 1 3]"
	} else {
		want["true"] = "[true 1 0 1]"
		want["1"] = "[1 1 1 2]"
	}
	if len(rows) != len(want)+1 {
		t.Errorf("rows: %v", rows)
	}
	for key, w := range want {
		if got := strings.ToLower(fmt.Sprint(pivotTexts(rows[key]))); got != strings.ToLower(w) {
			t.Errorf("row %q: %s, want %s", key, got, w)
		}
	}
	if strings.Contains(fmt.Sprint(rows), "pv-outside") {
		t.Errorf("the outside note's owner is out of scope: %v", rows)
	}
	if n := checkEveryLinkCountsItsCell(t, tc, admin, rows); n < 11 {
		t.Errorf("only %d links checked", n)
	}
	if rows["(none)"][2].query != "" {
		t.Errorf("the none-bucket is not linked: %+v", rows["(none)"])
	}

	// owner as the rows, a multi-valued relation through its alias as the
	// columns: ORDER BY group resolves to the surviving SELECT alias.
	tpl = fmt.Sprintf(`[pivot query="type = note" rows="owner" cols="group" scope="%d"]`, f.root.ID)
	rows = pivotRows(t, deferredHTML(t, tc, admin, "group", f.root.ID, tpl))
	// group is multi-valued, so there is no Total column; the related note
	// is in Related and Work, the rest in (none), which goes last.
	if got := fmt.Sprint(pivotTexts(rows["#head"])); got != "[owner Related Work (none)]" {
		t.Errorf("owner by group columns: %s", got)
	}
	if got := strings.ToLower(fmt.Sprint(pivotTexts(rows["inbox"]))); got != "[inbox 1 1 6]" {
		t.Errorf("owner Inbox by group: %s (%v)", got, rows)
	}
	if got := fmt.Sprint(pivotTexts(rows["#total"])); got != "[Total 1 1 9]" {
		t.Errorf("a single-valued rows dimension keeps the column totals: %s", got)
	}
	checkEveryLinkCountsItsCell(t, tc, admin, rows)
}

// checkPivotSumSemantics pins what value="sum" adds on each database: numbers
// always; a string holding a decimal too; a JSON true counts 1 on SQLite and
// nothing on Postgres; other strings nothing.
func checkPivotSumSemantics(t *testing.T, tc *TestContext, postgres bool) {
	f := buildPivotFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)
	tpl := fmt.Sprintf(`[pivot query="type = note" rows="owner" cols="name" value="sum" field="meta.hours" scope="%d"]`, f.root.ID)
	out := deferredHTML(t, tc, admin, "group", f.root.ID, tpl)
	if strings.Contains(out, "<a ") {
		t.Errorf("a sum pivot has no links: %s", out)
	}
	rows := pivotRows(t, out)
	// Inbox: 1.5 + "2" + 3 + "x", and true on SQLite only.
	want := "[Inbox 6.5 6.5]"
	if !postgres {
		want = "[Inbox 7.5 7.5]"
	}
	if got := strings.ToLower(fmt.Sprint(pivotTexts(rows["inbox"]))); got != strings.ToLower(want) {
		t.Errorf("Inbox hours: %s, want %s", got, want)
	}
	if got := fmt.Sprint(pivotTexts(rows["work"])); got != "[Work 4 4]" {
		t.Errorf("Work hours: %s", got)
	}
}

// checkPivotRunsOneStatement counts the aggregated statements one render runs.
func checkPivotRunsOneStatement(t *testing.T, tc *TestContext) {
	f := buildPivotFixture(t, tc)
	admin := roleBearer(t, tc, models.RoleAdmin)
	var grouped []string
	name := "pivottest:groupby"
	if err := tc.DB.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if sql := tx.Statement.SQL.String(); strings.Contains(sql, "GROUP BY") {
			// Postgres binds the limit; SQLite inlines it.
			grouped = append(grouped, sql+" -- "+fmt.Sprint(tx.Statement.Vars...))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tc.DB.Callback().Query().Remove(name) })
	tpl := fmt.Sprintf(`[pivot query="type = note" rows="meta.status" cols="owner" scope="%d"]`, f.root.ID)
	deferredHTML(t, tc, admin, "group", f.root.ID, tpl+tpl)
	if len(grouped) != 1 {
		t.Fatalf("two identical pivots ran %d GROUP BY statements, want 1: %q", len(grouped), grouped)
	}
	if !strings.Contains(grouped[0], "LIMIT 5001") && !(strings.Contains(grouped[0], "LIMIT $") && strings.HasSuffix(grouped[0], "5001")) {
		t.Errorf("the statement is bounded: %s", grouped[0])
	}
}

// checkPivotScopedPrincipal: a user confined to root never counts or names
// anything outside it, whatever scope says.
func checkPivotScopedPrincipal(t *testing.T, tc *TestContext) {
	f := buildPivotFixture(t, tc)
	scoped := scopedUserBearer(t, tc, f.root.ID)
	admin := roleBearer(t, tc, models.RoleAdmin)
	render := func(bearer, scope string) string {
		return deferredHTML(t, tc, bearer, "group", f.root.ID, `[pivot query="type = note" rows="owner" cols="meta.status" scope="`+scope+`"]`)
	}

	for _, scope := range []string{"global", strconv.Itoa(int(f.outside.ID)), "entity"} {
		out := render(scoped, scope)
		if strings.Contains(out, "pv-outside") || strings.Contains(out, ">100<") {
			t.Errorf("scope=%s: the scoped user saw the outside group: %s", scope, out)
		}
		if scope == strconv.Itoa(int(f.outside.ID)) {
			if !strings.Contains(out, "No results.") {
				t.Errorf("an outside scope intersects with the subtree to nothing: %s", out)
			}
			continue
		}
		rows := pivotRows(t, out)
		if got := rows["#total"]; len(got) == 0 || got[len(got)-1].text != "10" {
			t.Errorf("scope=%s: scoped grand total %v", scope, got)
		}
		checkEveryLinkCountsItsCell(t, tc, scoped, rows)
	}

	// Controls: the admin counts the outside note under global and its scope.
	if out := render(admin, "global"); !strings.Contains(out, "pv-outside") {
		t.Errorf("admin control, global: %s", out)
	}
	if out := render(admin, strconv.Itoa(int(f.outside.ID))); !strings.Contains(out, "pv-outside") {
		t.Errorf("admin control, outside scope: %s", out)
	}

	// A user confined to an empty group sees nothing at all.
	empty := &models.Group{Name: "pv-empty"}
	mkSim(t, tc, empty)
	denied := deferredHTML(t, tc, pivotScopedBearer(t, tc, empty.ID, "pv-empty-user"), "group", empty.ID, `[pivot query="type = note" rows="owner" cols="meta.status" scope="global"]`)
	if !strings.Contains(denied, "No results.") {
		t.Errorf("an empty subtree: %s", denied)
	}
}

// pivotScopedBearer is scopedUserBearer for a second confined user.
func pivotScopedBearer(t *testing.T, tc *TestContext, scopeGroupID uint, username string) string {
	t.Helper()
	u, err := tc.AppCtx.CreateUser(&application_context.UserInput{
		Username: username, Password: "password1", Role: models.RoleUser, ScopeGroupId: &scopeGroupID,
	})
	if err != nil {
		t.Fatalf("create scoped user: %v", err)
	}
	raw, _, err := tc.AppCtx.CreateApiToken(u.ID, "t", nil)
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	return "Bearer " + raw
}

func TestPivotShortcode_LinkCountsMatchCells(t *testing.T) {
	checkPivotLinkCountsMatchCells(t, setupAuthEnv(t), false)
}

func TestPivotShortcode_SumSemantics(t *testing.T) {
	checkPivotSumSemantics(t, setupAuthEnv(t), false)
}

func TestPivotShortcode_OneStatement(t *testing.T) {
	checkPivotRunsOneStatement(t, setupAuthEnv(t))
}

func TestPivotShortcode_ScopedPrincipalCannotSeeOutside(t *testing.T) {
	checkPivotScopedPrincipal(t, setupAuthEnv(t))
}

// The description preview renders through the same executor, as the viewer.
func TestPivotShortcode_PreviewIsScoped(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildPivotFixture(t, tc)
	scoped := scopedUserBearer(t, tc, f.root.ID)
	out := previewHTML(t, previewDescription(tc, scoped, "group", f.root.ID, `[pivot query="type = note" rows="owner" cols="meta.status" scope="global"]`))
	if !strings.Contains(out, "Work") || strings.Contains(out, "pv-outside") {
		t.Errorf("preview as the scoped user: %s", out)
	}
}
