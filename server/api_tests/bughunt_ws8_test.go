package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"mahresources/models"

	"github.com/spf13/afero"
)

// Regression coverage for the WS8 cluster of the 2026-07-29 UI bug hunt
// (docs/todo.md). Each test names the finding it pins.

// Finding 47: GET /v1/note/block/types returned a different order on every
// request, because models/block_types/registry.go ranged over a Go map. A
// keyboard user could only ever insert whichever type randomly landed first.
func TestBlockTypesOrderIsDeterministic(t *testing.T) {
	tc := SetupTestEnv(t)

	orders := map[string]int{}
	for range 20 {
		resp := tc.MakeRequest(http.MethodGet, "/v1/note/block/types", nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("GET /v1/note/block/types = %d, want 200", resp.Code)
		}
		var types []struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(resp.Body.Bytes(), &types); err != nil {
			t.Fatalf("decode block types: %v", err)
		}
		if len(types) == 0 {
			t.Fatal("block types endpoint returned nothing; the rest of this test is meaningless")
		}
		names := make([]string, len(types))
		for i, ty := range types {
			names[i] = ty.Type
		}
		orders[strings.Join(names, ",")]++
	}

	if len(orders) != 1 {
		t.Errorf("block type order is not stable: %d distinct orders across 20 requests\n%v\n"+
			"Ranging a Go map randomises iteration order. Sort before encoding.", len(orders), orders)
	}
}

// Finding 85: the version download served Content-Disposition
// filename="v3_a3bf2ae2" — version number plus a hash prefix, with no
// extension, so the file will not open by double-click.
func TestVersionDownloadFilenameHasExtension(t *testing.T) {
	tc := SetupTestEnv(t)

	resource := &models.Resource{
		Name:        "Holiday Snapshot",
		ContentType: "image/png",
		Location:    "/resources/ab/cd/ef/abcdef0123456789.png",
	}
	if err := tc.DB.Create(resource).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}

	version := &models.ResourceVersion{
		ResourceID:    resource.ID,
		VersionNumber: 3,
		Hash:          "a3bf2ae2c0ffee0123456789abcdef0123456789",
		HashType:      "SHA1",
		ContentType:   "image/png",
		Location:      resource.Location,
		FileSize:      1234,
		CreatedAt:     time.Now(),
	}
	if err := tc.DB.Create(version).Error; err != nil {
		t.Fatalf("create version: %v", err)
	}

	fs, err := tc.AppCtx.GetFsForStorageLocation(nil)
	if err != nil {
		t.Fatalf("get fs: %v", err)
	}
	if err := afero.WriteFile(fs, version.Location, []byte("not really a png, but bytes"), 0o644); err != nil {
		t.Fatalf("seed version bytes: %v", err)
	}

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/resource/version/file?versionId=%d", version.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET version file = %d, want 200 (body: %s)", resp.Code, resp.Body.String())
	}

	disposition := resp.Header().Get("Content-Disposition")
	filename := filenameFromDisposition(disposition)
	if filename == "" {
		t.Fatalf("no filename in Content-Disposition %q", disposition)
	}
	if !strings.HasSuffix(filename, ".png") {
		t.Errorf("version download filename = %q, want a .png extension.\n"+
			"A file with no extension does not open by double-click on macOS or Windows.", filename)
	}
}

// Finding 118: the dashboard emitted <time datetime="..."> with the server's
// local wall-clock time and a literal "Z" suffix, so every timestamp parsed as
// the UTC offset into the future. The pongo2 `date` layout used a bare Z, which
// Go treats as a literal character rather than a zone.
func TestDashboardTimeAttributeIsARealInstant(t *testing.T) {
	tc := SetupTestEnv(t)

	// Recent Activity is built from real rows, so create one.
	tc.CreateDummyGroup("Bug hunt dashboard group")

	resp := tc.MakeRequest(http.MethodGet, "/dashboard", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /dashboard = %d, want 200", resp.Code)
	}

	matches := regexp.MustCompile(`datetime="([^"]+)"`).FindAllStringSubmatch(resp.Body.String(), -1)
	if len(matches) == 0 {
		t.Skip("dashboard rendered no <time datetime> elements; nothing to assert")
	}

	for _, m := range matches {
		parsed, err := time.Parse(time.RFC3339, m[1])
		if err != nil {
			t.Errorf("datetime=%q is not RFC3339: %v", m[1], err)
			continue
		}
		if drift := time.Until(parsed); drift > time.Minute {
			t.Errorf("datetime=%q resolves to %v in the future.\n"+
				"Local wall-clock time is being stamped with a literal Z. Use Z07:00, or emit UTC.",
				m[1], drift.Round(time.Second))
		}
	}
}

// Finding 26: the log detail page rendered `{{ log.Details }}` for a
// types.JSON column, which pongo2 prints as the reflect wrapper
// "<types.JSON Value>". The browser parsed that as an unknown HTML element, so
// the Details row was an empty grey bar and Alpine threw on JSON.parse.
func TestLogDetailsRenderAsJSON(t *testing.T) {
	tc := SetupTestEnv(t)

	entry := &models.LogEntry{
		Level:      "info",
		Action:     "update",
		EntityType: "runtime_setting",
		Message:    "Changed a runtime setting",
		Details:    []byte(`{"key":"docs_site_base_url","old":"a","new":"b"}`),
	}
	if err := tc.DB.Create(entry).Error; err != nil {
		t.Fatalf("create log entry: %v", err)
	}

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/log?id=%d", entry.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /log = %d, want 200", resp.Code)
	}

	body := resp.Body.String()
	if strings.Contains(body, "types.JSON Value") {
		t.Errorf("log detail page rendered the Go reflect wrapper instead of the payload.\n" +
			"types.JSON has no String(), so pongo2 prints <types.JSON Value>.")
	}
	if !strings.Contains(body, "docs_site_base_url") {
		t.Errorf("log detail page does not contain the Details payload at all")
	}
}

// Finding 45: the /group/tree breadcrumb used a relative href ("groups"),
// which resolves to /groups from /group?id=N but to /group/groups — a 404 —
// from /group/tree. The same latent bug sits on two other pages.
func TestBreadcrumbHomeHrefIsAbsolute(t *testing.T) {
	tc := SetupTestEnv(t)
	group := tc.CreateDummyGroup("Bug hunt breadcrumb root")

	for _, path := range []string{
		fmt.Sprintf("/group/tree?root=%d", group.ID),
		fmt.Sprintf("/group?id=%d", group.ID),
	} {
		resp := tc.MakeRequest(http.MethodGet, path, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("GET %s = %d, want 200", path, resp.Code)
		}
		if strings.Contains(resp.Body.String(), `href="groups"`) {
			t.Errorf("%s renders a relative breadcrumb href=\"groups\".\n"+
				"Under /group/tree that resolves to /group/groups, which 404s.", path)
		}
	}
}

// Finding 43: "Show in Tree" always rendered a fixed 3-level slice, so a
// ?containing= target deeper than that never appeared — the page still claimed
// the teal nodes showed the path to the selected group.
func TestShowInTreeRevealsDeepTarget(t *testing.T) {
	tc := SetupTestEnv(t)

	var parentID *uint
	var deepest *models.Group
	for level := range 6 {
		g := &models.Group{Name: fmt.Sprintf("Depth level %d", level), OwnerId: parentID}
		if err := tc.DB.Create(g).Error; err != nil {
			t.Fatalf("create group at level %d: %v", level, err)
		}
		parentID = &g.ID
		deepest = g
	}

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/group/tree?containing=%d", deepest.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /group/tree?containing = %d, want 200", resp.Code)
	}
	if !strings.Contains(resp.Body.String(), deepest.Name) {
		t.Errorf("tree for ?containing=%d does not contain the target %q.\n"+
			"The fetch depth is hardcoded, so a target below it can never render.",
			deepest.ID, deepest.Name)
	}
}

// Finding 38: ?seriesId=N was accepted and silently ignored on the resource
// list, because ResourceSearchQuery has no SeriesId field at all — the field
// exists only on the create/edit shape, so the schema decoder drops it.
func TestResourceListFiltersBySeries(t *testing.T) {
	tc := SetupTestEnv(t)

	series := &models.Series{Name: "Bug hunt series"}
	if err := tc.DB.Create(series).Error; err != nil {
		t.Fatalf("create series: %v", err)
	}

	inSeries := &models.Resource{Name: "In the series", ContentType: "image/png", SeriesID: &series.ID}
	outOfSeries := &models.Resource{Name: "Not in the series", ContentType: "image/png"}
	for _, r := range []*models.Resource{inSeries, outOfSeries} {
		if err := tc.DB.Create(r).Error; err != nil {
			t.Fatalf("create resource %q: %v", r.Name, err)
		}
	}

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/v1/resources?seriesId=%d", series.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /v1/resources?seriesId = %d, want 200 (body: %s)", resp.Code, resp.Body.String())
	}

	var got []models.Resource
	if err := json.Unmarshal(resp.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode resources: %v", err)
	}

	// Positive control: without the control, an empty result set would satisfy
	// the "no out-of-series rows" assertion for free.
	if len(got) != 1 {
		t.Fatalf("?seriesId=%d returned %d resources, want exactly 1 (the filter is ignored)", series.ID, len(got))
	}
	if got[0].ID != inSeries.ID {
		t.Errorf("?seriesId=%d returned resource %d (%q), want %d (%q)",
			series.ID, got[0].ID, got[0].Name, inSeries.ID, inSeries.Name)
	}
}

// Finding 70: switching to the Simple view while on page 2 landed on a
// completely blank page. The view switcher carries the whole query string
// across, but Simple renders four times as many rows per page, so "page 2"
// means something different in each view and page 2 of Simple is usually empty.
// Switching views must land on page 1.
func TestViewSwitcherDropsPageNumber(t *testing.T) {
	tc := SetupTestEnv(t)
	for i := range 3 {
		tc.CreateResourceWithType(t, fmt.Sprintf("Resource %d", i), "image/png")
	}

	resp := tc.MakeRequest(http.MethodGet, "/resources/details?page=2", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /resources/details?page=2 = %d, want 200", resp.Code)
	}

	body := resp.Body.String()
	links := regexp.MustCompile(`href="(/resources(?:/[a-z]+)?\?[^"]*)"`).FindAllStringSubmatch(body, -1)
	if len(links) == 0 {
		t.Fatal("no view-switcher links found; the rest of this test is meaningless")
	}

	for _, l := range links {
		if strings.Contains(l[1], "page=2") {
			t.Errorf("view-switcher link %q carries page=2 across a view change.\n"+
				"Simple renders 4x the rows per page, so the preserved page number is a blank dead end.", l[1])
		}
	}
}

// Findings 27 and 37: the /logs filter dropdowns omitted entity types and
// actions the log actually records (so the settings audit trail could not be
// filtered from the UI at all), and an unrecognised value in the URL collapsed
// the select to "" — clicking "Apply Filters" then silently discarded the
// filter and returned the full list.
func TestLogsFilterOffersRecordedValues(t *testing.T) {
	tc := SetupTestEnv(t)

	entry := &models.LogEntry{
		Level:      models.LogLevelInfo,
		Action:     models.LogActionReset,
		EntityType: "runtime_setting",
		Message:    "Reset a runtime setting",
	}
	if err := tc.DB.Create(entry).Error; err != nil {
		t.Fatalf("create log entry: %v", err)
	}

	resp := tc.MakeRequest(http.MethodGet, "/logs", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /logs = %d, want 200", resp.Code)
	}
	body := resp.Body.String()

	for _, want := range []string{`value="runtime_setting"`, `value="reset"`} {
		if !strings.Contains(body, want) {
			t.Errorf("/logs filter dropdowns do not offer %s, but the log records it.\n"+
				"That value cannot be filtered from the UI at all.", want)
		}
	}
}

func TestLogsFilterRoundTripsUnlistedValue(t *testing.T) {
	tc := SetupTestEnv(t)

	entry := &models.LogEntry{
		Level:      models.LogLevelInfo,
		Action:     models.LogActionUpdate,
		EntityType: "some_future_entity_type",
		Message:    "An entity type the curated dropdown does not know",
	}
	if err := tc.DB.Create(entry).Error; err != nil {
		t.Fatalf("create log entry: %v", err)
	}

	resp := tc.MakeRequest(http.MethodGet, "/logs?EntityType=some_future_entity_type", nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /logs?EntityType = %d, want 200", resp.Code)
	}
	body := resp.Body.String()

	// The applied filter must be reflected in the control, or re-submitting the
	// form silently drops it.
	sel := regexp.MustCompile(`(?s)<select[^>]*name="EntityType".*?</select>`).FindString(body)
	if sel == "" {
		t.Fatal("no EntityType select rendered; the rest of this test is meaningless")
	}
	if !strings.Contains(sel, "some_future_entity_type") {
		t.Errorf("EntityType select does not reflect the applied filter.\n"+
			"An unlisted value collapses the control to \"\", so Apply Filters wipes the filter.\nselect was: %s", sel)
	}
}

// Finding 84: "Copy Name" put a corrupted string on the clipboard for any name
// containing a character outside the BMP. pongo2's escapejs emits
// fmt.Sprintf("\\u%04X", r) — a *minimum* width of 4 — so U+1F3A8 became the
// 5-digit \u1F3A8, which JavaScript reads as \u1F3A followed by the literal "8".
// The page rendered the emoji correctly, so the corruption was invisible until
// you pasted.
func TestCopyButtonEscapesAstralCharacters(t *testing.T) {
	tc := SetupTestEnv(t)

	const name = "Ünïcødé Ñame 测试 🎨"
	resource := &models.Resource{Name: name, ContentType: "image/png"}
	if err := tc.DB.Create(resource).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}

	resp := tc.MakeRequest(http.MethodGet, fmt.Sprintf("/resource?id=%d", resource.ID), nil)
	if resp.Code != http.StatusOK {
		t.Fatalf("GET /resource = %d, want 200", resp.Code)
	}
	body := resp.Body.String()

	// U+1F3A8 must be emitted as a UTF-16 surrogate pair, never as a bare
	// 5-hex-digit escape.
	if strings.Contains(body, `\u1F3A8`) {
		t.Errorf("copy button emits the 5-hex-digit escape \\u1F3A8.\n" +
			"JavaScript parses that as \\u1F3A followed by \"8\", so the clipboard gets \"Ἲ8\".")
	}
	if !strings.Contains(body, `\uD83C\uDFA8`) {
		t.Errorf("copy button does not emit the surrogate pair \\uD83C\\uDFA8 for U+1F3A8")
	}
}

func filenameFromDisposition(disposition string) string {
	const key = `filename="`
	i := strings.Index(disposition, key)
	if i < 0 {
		return ""
	}
	rest := disposition[i+len(key):]
	j := strings.Index(rest, `"`)
	if j < 0 {
		return ""
	}
	return rest[:j]
}
