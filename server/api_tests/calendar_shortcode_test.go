package api_tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"mahresources/models"
	"mahresources/models/types"
)

// [calendar] reads every count and title through the request's
// principal-scoped MRQL executor, and its day links reproduce the cell.

type calendarFixture struct {
	root, outside *models.Group
	notes         map[string]*models.Note
}

// buildCalendarFixture seeds notes owned by root with meta.due values of every
// shape the calendar must tell apart, and one note outside root due on a day
// root's notes share.
func buildCalendarFixture(t *testing.T, tc *TestContext) calendarFixture {
	t.Helper()
	f := calendarFixture{root: &models.Group{Name: "cal-root"}, outside: &models.Group{Name: "cal-outside"}, notes: map[string]*models.Note{}}
	mkSim(t, tc, f.root)
	mkSim(t, tc, f.outside)
	mk := func(name string, owner *models.Group, due string) {
		n := &models.Note{Name: name, OwnerId: &owner.ID, Meta: types.JSON(`{"due":` + due + `}`)}
		mkSim(t, tc, n)
		f.notes[name] = n
	}
	mk("Dentist", f.root, `"2026-03-04"`)
	mk("Call Bob", f.root, `"2026-03-04T23:30:00-05:00"`)
	mk("Numeric due", f.root, `20260304`)
	mk("Boolean due", f.root, `true`)
	mk("Text due", f.root, `"soon"`)
	mk("Pay rent", f.root, `"2026-03-09"`)
	mk("April thing", f.root, `"2026-04-02"`)
	mk("Secret outside", f.outside, `"2026-03-04"`)
	return f
}

var (
	calHiddenSpan = regexp.MustCompile(`<span[^>]*aria-hidden="true"[^>]*>[^<]*</span>`)
	calTag        = regexp.MustCompile(`<[^>]*>`)
	calCell       = regexp.MustCompile(`<td class="calendar-shortcode-cell[^"]*">.*?</td>`)
	calDayHref    = regexp.MustCompile(`class="calendar-shortcode-daylink" href="([^"]*)"`)
	calItemHref   = regexp.MustCompile(`<li><a href="/note\?id=(\d+)">`)
)

func calText(fragment string) string {
	return html.UnescapeString(strings.TrimSpace(calTag.ReplaceAllString(calHiddenSpan.ReplaceAllString(fragment, ""), "")))
}

// calendarCells maps "March 4" to its cell markup.
func calendarCells(t *testing.T, out string) map[string]string {
	t.Helper()
	cells := map[string]string{}
	for _, c := range calCell.FindAllString(out, -1) {
		text := calText(c)
		label, _, _ := strings.Cut(text, ",")
		cells[label] = c
	}
	if len(cells) == 0 {
		t.Fatalf("no calendar cells in %s", out)
	}
	return cells
}

// mrqlNoteIDs runs a query through POST /v1/mrql as bearer and returns the
// note IDs, sorted.
func mrqlNoteIDs(t *testing.T, tc *TestContext, bearer, query string) []uint {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"query": query})
	headers := map[string]string{"Accept": "application/json", "Content-Type": "application/json"}
	if bearer != "" {
		headers["Authorization"] = bearer
	}
	rr := doReq(tc, http.MethodPost, "/v1/mrql", headers, nil, bytes.NewReader(body))
	if rr.Code != http.StatusOK {
		t.Fatalf("mrql %q: %d %s", query, rr.Code, rr.Body.String())
	}
	var res struct {
		Notes []struct {
			ID uint `json:"ID"`
		} `json:"notes"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatal(err)
	}
	var ids []uint
	for _, n := range res.Notes {
		ids = append(ids, n.ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func cellNoteIDs(cell string) []uint {
	var ids []uint
	for _, m := range calItemHref.FindAllStringSubmatch(cell, -1) {
		var id uint
		fmt.Sscan(m[1], &id)
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func dayLinkQuery(t *testing.T, cell string) string {
	t.Helper()
	m := calDayHref.FindStringSubmatch(cell)
	if m == nil {
		t.Fatalf("cell has no day link: %s", cell)
	}
	u, err := url.Parse(html.UnescapeString(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	return u.Query().Get("q")
}

func (f calendarFixture) ids(names ...string) []uint {
	var ids []uint
	for _, n := range names {
		ids = append(ids, f.notes[n].ID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

const calendarMarchTpl = `[calendar mrql='type = "note"' date="meta.due" month="2026-03" scope="global"]`

func TestCalendarShortcode_ItemsModeAgreesWithItsDayLinks(t *testing.T) {
	tc := SetupTestEnv(t)
	checkCalendarItemsMode(t, tc, buildCalendarFixture(t, tc))
}

// checkCalendarItemsMode is shared with the Postgres run.
func checkCalendarItemsMode(t *testing.T, tc *TestContext, f calendarFixture) {
	t.Helper()
	out := deferredHTML(t, tc, "", "group", f.root.ID, calendarMarchTpl)
	if !strings.Contains(out, "March 2026 <span class=\"calendar-shortcode-total\">(4 items)</span>") {
		t.Fatalf("caption: %s", out)
	}
	for _, absent := range []string{"Numeric due", "Boolean due", "Text due", "April thing"} {
		if strings.Contains(out, absent) {
			t.Errorf("%s must not appear in March: %s", absent, out)
		}
	}
	cells := calendarCells(t, out)
	want := map[string][]uint{
		"March 4": f.ids("Dentist", "Call Bob", "Secret outside"),
		"March 9": f.ids("Pay rent"),
	}
	for day, ids := range want {
		cell := cells[day]
		if got := cellNoteIDs(cell); fmt.Sprint(got) != fmt.Sprint(ids) {
			t.Errorf("%s titles: got %v, want %v", day, got, ids)
		}
		if got := mrqlNoteIDs(t, tc, "", dayLinkQuery(t, cell)); fmt.Sprint(got) != fmt.Sprint(ids) {
			t.Errorf("%s day link lists %v, the cell counted %v", day, got, ids)
		}
	}
	if !strings.HasPrefix(calText(regexp.MustCompile(`<a class="calendar-shortcode-daylink".*?</a>`).FindString(cells["March 4"])), "March 4, 3 items") {
		t.Errorf("March 4 day link name: %s", cells["March 4"])
	}
	if strings.Contains(cells["March 5"], "<a ") || calText(cells["March 5"]) != "March 5, no items" {
		t.Errorf("an empty day is text: %s", cells["March 5"])
	}

	// The deferred render carries navigation; Next renders April.
	next := regexp.MustCompile(`data-next-token="([^"]+)"`).FindStringSubmatch(out)
	if next == nil {
		t.Fatalf("no next token: %s", out[:300])
	}
	april := postDeferredToken(t, tc, "", html.UnescapeString(next[1]), http.StatusOK)
	if !strings.Contains(april, "April 2026 <span class=\"calendar-shortcode-total\">(1 item)</span>") || !strings.Contains(april, ">April thing<") {
		t.Errorf("next month: %s", april)
	}
	if !strings.Contains(april, `<span class="sr-only">Previous month, </span>March 2026</button>`) {
		t.Errorf("April's Prev names March: %s", april)
	}
}

// postDeferredToken posts an already sealed token, as the browser does.
func postDeferredToken(t *testing.T, tc *TestContext, bearer, token string, want int) string {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"token": token})
	headers := map[string]string{"Accept": "application/json", "Content-Type": "application/json"}
	if bearer != "" {
		headers["Authorization"] = bearer
	}
	rr := doReq(tc, http.MethodPost, "/v1/shortcodes/deferred", headers, nil, bytes.NewReader(payload))
	if rr.Code != want {
		t.Fatalf("deferred: %d (want %d) %s", rr.Code, want, rr.Body.String())
	}
	var resp struct {
		HTML string `json:"html"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	return resp.HTML
}

func TestCalendarShortcode_CreatedDatesAndCountsMode(t *testing.T) {
	checkCalendarCountsMode(t, SetupTestEnv(t))
}

// checkCalendarCountsMode is shared with the Postgres run.
func checkCalendarCountsMode(t *testing.T, tc *TestContext) {
	t.Helper()
	g := &models.Group{Name: "cal-bulk"}
	mkSim(t, tc, g)
	var notes []models.Note
	for i := 0; i < 150; i++ {
		day := 1 + i%3 // 50 each on March 1, 2 and 3
		at := time.Date(2026, 3, day, 12, 0, 0, 0, time.Local)
		notes = append(notes, models.Note{Name: fmt.Sprintf("bulk-%03d", i), OwnerId: &g.ID, CreatedAt: at, UpdatedAt: at})
	}
	if err := tc.DB.CreateInBatches(notes, 50).Error; err != nil {
		t.Fatal(err)
	}
	few := []models.Note{
		{Name: "few-a", OwnerId: &g.ID, CreatedAt: time.Date(2026, 5, 7, 8, 0, 0, 0, time.Local)},
		{Name: "few-b", OwnerId: &g.ID, CreatedAt: time.Date(2026, 5, 7, 9, 0, 0, 0, time.Local)},
	}
	if err := tc.DB.Create(&few).Error; err != nil {
		t.Fatal(err)
	}

	out := deferredHTML(t, tc, "", "group", g.ID, `[calendar mrql='type = "note" AND name ~ "bulk-*"' month="2026-03"]`)
	if !strings.Contains(out, "Showing counts only: more than 100 items this month.") {
		t.Fatalf("150 notes should switch to counts: %s", out)
	}
	cells := calendarCells(t, out)
	for _, day := range []string{"March 1", "March 2", "March 3"} {
		if got := calText(cells[day]); got != day+", 50 items" {
			t.Errorf("%s: %q", day, got)
		}
		if n := len(mrqlNoteIDs(t, tc, "", dayLinkQuery(t, cells[day]))); n != 50 {
			t.Errorf("%s day link lists %d", day, n)
		}
	}
	if strings.Contains(out, "calendar-shortcode-items") {
		t.Errorf("counts mode lists no titles")
	}

	out = deferredHTML(t, tc, "", "group", g.ID, `[calendar mrql='type = "note" AND name ~ "few-*"' month="2026-05" date="created"]`)
	if c := calendarCells(t, out)["May 7"]; !strings.Contains(c, ">few-a<") || !strings.Contains(c, ">few-b<") {
		t.Errorf("created dates: %s", c)
	}
}

func TestCalendarShortcode_ScopedPrincipal(t *testing.T) {
	tc := setupAuthEnv(t)
	f := buildCalendarFixture(t, tc)
	scoped := scopedUserBearer(t, tc, f.root.ID)
	admin := roleBearer(t, tc, models.RoleAdmin)
	secret := f.notes["Secret outside"]

	out := deferredHTML(t, tc, scoped, "group", f.root.ID, calendarMarchTpl)
	cell := calendarCells(t, out)["March 4"]
	if got := cellNoteIDs(cell); fmt.Sprint(got) != fmt.Sprint(f.ids("Dentist", "Call Bob")) {
		t.Errorf("scoped titles on March 4: %v", got)
	}
	if strings.Contains(out, "Secret outside") || !strings.Contains(out, "(3 items)") {
		t.Errorf("scoped user counted the outside note: %s", out)
	}
	if got := mrqlNoteIDs(t, tc, scoped, dayLinkQuery(t, cell)); fmt.Sprint(got) != fmt.Sprint(f.ids("Dentist", "Call Bob")) {
		t.Errorf("scoped day link lists %v", got)
	}

	// An explicit outside scope intersects with the subtree: nothing.
	outsideTpl := fmt.Sprintf(`[calendar mrql='type = "note"' date="meta.due" month="2026-03" scope="%d"]`, f.outside.ID)
	out = deferredHTML(t, tc, scoped, "group", f.root.ID, outsideTpl)
	if !strings.Contains(out, "(no items)") || strings.Contains(out, "Secret outside") {
		t.Errorf("an outside scope must show nothing to the scoped user: %s", out)
	}
	// Control: the admin sees the outside note there.
	out = deferredHTML(t, tc, admin, "group", f.root.ID, outsideTpl)
	if !strings.Contains(out, fmt.Sprintf(`<li><a href="/note?id=%d">Secret outside</a></li>`, secret.ID)) || !strings.Contains(out, "(1 item)") {
		t.Errorf("admin control: %s", out)
	}
	out = deferredHTML(t, tc, admin, "group", f.root.ID, calendarMarchTpl)
	if !strings.Contains(out, "(4 items)") {
		t.Errorf("admin counts the outside note: %s", out)
	}

	// Navigation as the scoped user stays scoped.
	next := regexp.MustCompile(`data-next-token="([^"]+)"`).FindStringSubmatch(deferredHTML(t, tc, scoped, "group", f.root.ID, calendarMarchTpl))
	if next == nil {
		t.Fatal("no next token")
	}
	if april := postDeferredToken(t, tc, scoped, html.UnescapeString(next[1]), http.StatusOK); !strings.Contains(april, ">April thing<") {
		t.Errorf("scoped next month: %s", april)
	}

	// A token minted for an entity outside the subtree is refused.
	adminOut := deferredHTML(t, tc, admin, "group", f.outside.ID, calendarMarchTpl)
	outsideNext := regexp.MustCompile(`data-next-token="([^"]+)"`).FindStringSubmatch(adminOut)
	if outsideNext == nil {
		t.Fatal("no next token on the outside group")
	}
	postDeferredToken(t, tc, scoped, html.UnescapeString(outsideNext[1]), http.StatusNotFound)
}
