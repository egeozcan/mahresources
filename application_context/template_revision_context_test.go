package application_context

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

func listRevisionRows(t *testing.T, ctx *MahresourcesContext, carrierType string, id uint) []models.TemplateRevision {
	t.Helper()
	var revs []models.TemplateRevision
	if err := ctx.db.Where("carrier_type = ? AND carrier_id = ?", carrierType, id).Order("number").Find(&revs).Error; err != nil {
		t.Fatalf("list revisions: %v", err)
	}
	return revs
}

func revisionSummary(revs []models.TemplateRevision) string {
	var parts []string
	for _, r := range revs {
		parts = append(parts, strings.Join([]string{string(rune('0' + r.Number)), r.Source}, ":"))
	}
	return strings.Join(parts, " ")
}

func mustCategory(t *testing.T, ctx *MahresourcesContext, c query_models.CategoryCreator) *models.Category {
	t.Helper()
	cat, err := ctx.CreateCategory(&c)
	if err != nil {
		t.Fatalf("CreateCategory: %v", err)
	}
	return cat
}

func updateCategory(ctx *MahresourcesContext, id uint, mutate func(*query_models.CategoryEditor)) error {
	cat, err := ctx.GetCategory(id)
	if err != nil {
		return err
	}
	ed := &query_models.CategoryEditor{ID: id, CategoryCreator: query_models.CategoryCreator{
		Name: cat.Name, Description: cat.Description,
		CustomHeader: cat.CustomHeader, CustomHeaderCSS: cat.CustomHeaderCSS,
		CustomSidebar: cat.CustomSidebar, MetaSchema: cat.MetaSchema,
	}}
	mutate(ed)
	_, err = ctx.UpdateCategory(ed)
	return err
}

func TestRevision_CreateRecordsOneCreateRevision_EmptyOrNot(t *testing.T) {
	ctx := createTestContext(t)

	empty := mustCategory(t, ctx, query_models.CategoryCreator{Name: "empty-cat"})
	if got := revisionSummary(listRevisionRows(t, ctx, models.TemplateCarrierCategory, empty.ID)); got != "1:create" {
		t.Errorf("empty create = %q, want exactly one create", got)
	}

	full := mustCategory(t, ctx, query_models.CategoryCreator{Name: "full-cat", CustomHeader: "<h1>x</h1>", MetaSchema: `{"type":"object"}`})
	revs := listRevisionRows(t, ctx, models.TemplateCarrierCategory, full.ID)
	if got := revisionSummary(revs); got != "1:create" {
		t.Fatalf("non-empty create = %q, want exactly one create", got)
	}
	var changed []string
	_ = json.Unmarshal(revs[0].ChangedSlots, &changed)
	if strings.Join(changed, ",") != "CustomHeader,MetaSchema" {
		t.Errorf("changed slots of #1 = %v", changed)
	}

	rc, err := ctx.CreateResourceCategory(&query_models.ResourceCategoryCreator{Name: "rc"})
	if err != nil {
		t.Fatal(err)
	}
	if got := revisionSummary(listRevisionRows(t, ctx, models.TemplateCarrierResourceCategory, rc.ID)); got != "1:create" {
		t.Errorf("resource category create = %q", got)
	}
}

func TestRevision_NoOpSaveWritesNothing(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c", CustomHeader: "h"})

	for i := 0; i < 2; i++ {
		if err := updateCategory(ctx, cat.ID, func(e *query_models.CategoryEditor) {}); err != nil {
			t.Fatal(err)
		}
	}
	// Name and Description are untracked: changing them records nothing either.
	if err := updateCategory(ctx, cat.ID, func(e *query_models.CategoryEditor) { e.Description = "new description" }); err != nil {
		t.Fatal(err)
	}
	// An empty SectionConfig keeps the current one; an explicit "{}" is the same canonical config.
	if err := updateCategory(ctx, cat.ID, func(e *query_models.CategoryEditor) { e.SectionConfig = "{}" }); err != nil {
		t.Fatal(err)
	}
	if got := revisionSummary(listRevisionRows(t, ctx, models.TemplateCarrierCategory, cat.ID)); got != "1:create" {
		t.Errorf("revisions after no-op saves = %q", got)
	}
}

func TestRevision_ChangingSaveRecordsNextNumberWithChangedSlotsAndApplied(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c"})

	err := updateCategory(ctx, cat.ID, func(e *query_models.CategoryEditor) {
		e.CustomSidebar = "side"
		e.RevisionApplied = []string{"preset:dashboard,junk", "loaded:1:CustomHeader", "<b>"}
	})
	if err != nil {
		t.Fatal(err)
	}
	revs := listRevisionRows(t, ctx, models.TemplateCarrierCategory, cat.ID)
	if got := revisionSummary(revs); got != "1:create 2:api" {
		t.Fatalf("revisions = %q", got)
	}
	var changed, applied []string
	_ = json.Unmarshal(revs[1].ChangedSlots, &changed)
	_ = json.Unmarshal(revs[1].Applied, &applied)
	if strings.Join(changed, ",") != "CustomSidebar" {
		t.Errorf("changed slots = %v", changed)
	}
	if strings.Join(applied, ",") != "preset:dashboard,loaded:1:CustomHeader" {
		t.Errorf("applied = %v, want only the recognised entries", applied)
	}
}

// Seeds, the download-created Category and the default Resource Category are
// created without a revision. Their first changing save records what they were.
func TestRevision_ExemptCarrierGetsBaselineOnFirstChangingSave(t *testing.T) {
	ctx := createTestContext(t)
	seeded := models.Category{Name: "seeded", CustomHeader: "seed header"}
	if err := ctx.db.Create(&seeded).Error; err != nil {
		t.Fatal(err)
	}

	// Unchanged save: still nothing, so an exempt carrier's history stays empty.
	if err := updateCategory(ctx, seeded.ID, func(e *query_models.CategoryEditor) {}); err != nil {
		t.Fatal(err)
	}
	if n := len(listRevisionRows(t, ctx, models.TemplateCarrierCategory, seeded.ID)); n != 0 {
		t.Fatalf("a no-op save on an exempt carrier recorded %d revisions", n)
	}

	if err := updateCategory(ctx, seeded.ID, func(e *query_models.CategoryEditor) { e.CustomHeader = "edited" }); err != nil {
		t.Fatal(err)
	}
	revs := listRevisionRows(t, ctx, models.TemplateCarrierCategory, seeded.ID)
	if got := revisionSummary(revs); got != "1:baseline 2:api" {
		t.Fatalf("revisions = %q", got)
	}
	base, _ := models.ParseTemplateSnapshot(revs[0].Snapshot)
	if base["CustomHeader"] != "seed header" {
		t.Errorf("baseline must hold the pre-save state, got %v", base["CustomHeader"])
	}

	// Default Resource Category: raw insert like main.go's, then a first edit.
	if err := ctx.db.Exec("INSERT INTO resource_categories (name, description) VALUES ('RawDefault', 'x')").Error; err != nil {
		t.Fatal(err)
	}
	var rc models.ResourceCategory
	if err := ctx.db.Where("name = 'RawDefault'").First(&rc).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.UpdateResourceCategory(&query_models.ResourceCategoryEditor{
		ID: rc.ID, ResourceCategoryCreator: query_models.ResourceCategoryCreator{Name: "RawDefault", CustomSummary: "s"},
	}); err != nil {
		t.Fatal(err)
	}
	if got := revisionSummary(listRevisionRows(t, ctx, models.TemplateCarrierResourceCategory, rc.ID)); got != "1:baseline 2:api" {
		t.Errorf("default resource category revisions = %q", got)
	}
}

func TestRevision_NoteTypeCreateVersusUpdate(t *testing.T) {
	ctx := createTestContext(t)
	nt, err := ctx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{Name: "nt"})
	if err != nil {
		t.Fatal(err)
	}
	if got := revisionSummary(listRevisionRows(t, ctx, models.TemplateCarrierNoteType, nt.ID)); got != "1:create" {
		t.Fatalf("create = %q", got)
	}
	// Same content again: an update that changes nothing.
	if _, err := ctx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{ID: nt.ID, Name: "nt"}); err != nil {
		t.Fatal(err)
	}
	if got := revisionSummary(listRevisionRows(t, ctx, models.TemplateCarrierNoteType, nt.ID)); got != "1:create" {
		t.Fatalf("no-op update = %q", got)
	}
	if _, err := ctx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{ID: nt.ID, Name: "nt", ApplyTemplatesToShares: true}); err != nil {
		t.Fatal(err)
	}
	if got := revisionSummary(listRevisionRows(t, ctx, models.TemplateCarrierNoteType, nt.ID)); got != "1:create 2:api" {
		t.Errorf("changing update = %q", got)
	}
}

func TestRevision_InvalidMetaSchemaBeforeValidAfter(t *testing.T) {
	ctx := createTestContext(t)
	bad := models.Category{Name: "bad", MetaSchema: `{ not valid json ][`}
	if err := ctx.db.Create(&bad).Error; err != nil {
		t.Fatal(err)
	}
	if err := updateCategory(ctx, bad.ID, func(e *query_models.CategoryEditor) { e.MetaSchema = `{"type":"object"}` }); err != nil {
		t.Fatalf("repairing an invalid schema must save: %v", err)
	}
	if got := revisionSummary(listRevisionRows(t, ctx, models.TemplateCarrierCategory, bad.ID)); got != "1:baseline 2:api" {
		t.Fatalf("revisions = %q", got)
	}
	diff, err := ctx.DiffTemplateRevisions(models.TemplateCarrierCategory, bad.ID, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Slots) != 1 || diff.Slots[0].Slot != "MetaSchema" {
		t.Fatalf("diff slots = %+v", diff.Slots)
	}
	if h := diff.Slots[0].Fields[0].Hunks; h[0].Op != models.DiffOpDelete || h[0].Lines[0] != `{ not valid json ][` {
		t.Errorf("the malformed side must diff as raw text: %+v", h)
	}
}

func TestRevision_RestoreWholeRecordsForwardAndClearsMissingKeys(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c", CustomHeader: "v1 header"})
	if err := updateCategory(ctx, cat.ID, func(e *query_models.CategoryEditor) {
		e.CustomHeader = "v2 header"
		e.CustomSidebar = "v2 sidebar"
	}); err != nil {
		t.Fatal(err)
	}

	// Revision #1 is rewritten to look like one taken before CustomSidebar
	// existed: the key is simply absent.
	old, _ := models.ParseTemplateSnapshot(listRevisionRows(t, ctx, models.TemplateCarrierCategory, cat.ID)[0].Snapshot)
	delete(old, "CustomSidebar")
	raw, _ := old.Marshal()
	if err := ctx.db.Model(&models.TemplateRevision{}).Where("carrier_id = ? AND number = 1", cat.ID).Update("snapshot", string(raw)).Error; err != nil {
		t.Fatal(err)
	}

	summary, err := ctx.RestoreTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Number != 3 || summary.Source != models.RevisionSourceRestore || summary.RestoredFrom == nil || *summary.RestoredFrom != 1 {
		t.Errorf("restore summary = %+v", summary)
	}
	got, _ := ctx.GetCategory(cat.ID)
	if got.CustomHeader != "v1 header" || got.CustomSidebar != "" {
		t.Errorf("after restore header=%q sidebar=%q, want v1 header and a cleared sidebar", got.CustomHeader, got.CustomSidebar)
	}
	if got := revisionSummary(listRevisionRows(t, ctx, models.TemplateCarrierCategory, cat.ID)); got != "1:create 2:api 3:restore" {
		t.Errorf("restore moves forward, revisions = %q", got)
	}
}

func TestRevision_IdenticalRestoreStillRecords(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c", CustomHeader: "h"})
	if _, err := ctx.RestoreTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, nil, true); err != nil {
		t.Fatal(err)
	}
	if got := revisionSummary(listRevisionRows(t, ctx, models.TemplateCarrierCategory, cat.ID)); got != "1:create 2:restore" {
		t.Errorf("identical restore = %q, want one restore", got)
	}
}

func TestRevision_RestoreWithNoSectionConfigRendersLikeTheSnapshot(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c"})
	if err := updateCategory(ctx, cat.ID, func(e *query_models.CategoryEditor) { e.SectionConfig = `{"tags":false}` }); err != nil {
		t.Fatal(err)
	}
	// #1 has no section config. The editor mapping would "keep current" and so
	// never restore it; restore applies the exact state.
	if _, err := ctx.RestoreTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, nil, true); err != nil {
		t.Fatal(err)
	}
	got, _ := ctx.GetCategory(cat.ID)
	cfg := models.ResolveGroupSectionConfig(&got.SectionConfig)
	if !cfg.Tags {
		t.Errorf("restoring a revision with no section config must show tags again, SectionConfig=%q", got.SectionConfig)
	}
}

func TestRevision_OneSlotRestoreLeavesTheOthersCurrent(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{
		Name: "c", CustomHeader: "h1", CustomHeaderCSS: "h1css", CustomSidebar: "s1",
	})
	if err := updateCategory(ctx, cat.ID, func(e *query_models.CategoryEditor) {
		e.CustomHeader, e.CustomHeaderCSS, e.CustomSidebar = "h2", "h2css", "s2"
	}); err != nil {
		t.Fatal(err)
	}
	slot := "CustomHeader"
	if _, err := ctx.RestoreTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, &slot, false); err != nil {
		t.Fatal(err)
	}
	got, _ := ctx.GetCategory(cat.ID)
	if got.CustomHeader != "h1" || got.CustomHeaderCSS != "h2css" || got.CustomSidebar != "s2" {
		t.Errorf("without CSS: header=%q css=%q sidebar=%q", got.CustomHeader, got.CustomHeaderCSS, got.CustomSidebar)
	}
	if _, err := ctx.RestoreTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, &slot, true); err != nil {
		t.Fatal(err)
	}
	got, _ = ctx.GetCategory(cat.ID)
	if got.CustomHeaderCSS != "h1css" || got.CustomSidebar != "s2" {
		t.Errorf("with CSS: css=%q sidebar=%q", got.CustomHeaderCSS, got.CustomSidebar)
	}
	bogus := "Name"
	if _, err := ctx.RestoreTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, &bogus, true); !errors.Is(err, ErrTemplateRevisionInvalid) {
		t.Errorf("restoring an untracked field = %v, want ErrTemplateRevisionInvalid", err)
	}
}

func TestRevision_RefusedRestoreWritesNothing(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c", CustomHeader: "current"})

	// A revision whose raw MetaSchema no longer validates.
	snap := models.TemplateSnapshot{"CustomHeader": "old", "MetaSchema": `{ not valid json ][`}
	raw, _ := snap.Marshal()
	if err := ctx.db.Create(&models.TemplateRevision{
		CarrierType: models.TemplateCarrierCategory, CarrierID: cat.ID, Number: 2, Snapshot: raw, Source: models.RevisionSourceAPI,
	}).Error; err != nil {
		t.Fatal(err)
	}

	_, err := ctx.RestoreTemplateRevision(models.TemplateCarrierCategory, cat.ID, 2, nil, true)
	if !errors.Is(err, ErrTemplateRevisionInvalid) || !strings.Contains(err.Error(), "MetaSchema") {
		t.Fatalf("err = %v, want ErrTemplateRevisionInvalid naming MetaSchema", err)
	}
	got, _ := ctx.GetCategory(cat.ID)
	if got.CustomHeader != "current" {
		t.Errorf("a refused restore changed the carrier: %q", got.CustomHeader)
	}
	if n := len(listRevisionRows(t, ctx, models.TemplateCarrierCategory, cat.ID)); n != 2 {
		t.Errorf("a refused restore recorded a revision (%d rows)", n)
	}
}

func TestRevision_RestoreWritesAuditLogDetails(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c"})
	if err := updateCategory(ctx, cat.ID, func(e *query_models.CategoryEditor) { e.CustomHeader = "x" }); err != nil {
		t.Fatal(err)
	}
	if _, err := ctx.RestoreTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, nil, true); err != nil {
		t.Fatal(err)
	}
	var entry models.LogEntry
	if err := ctx.db.Where("message LIKE ?", "Restored template revision%").First(&entry).Error; err != nil {
		t.Fatalf("no audit entry: %v", err)
	}
	var details map[string]float64
	_ = json.Unmarshal(entry.Details, &details)
	if details["fromRevision"] != 1 || details["toRevision"] != 3 {
		t.Errorf("details = %v", details)
	}
}

func TestRevision_DeleteRemovesRevisions(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c"})
	rc, _ := ctx.CreateResourceCategory(&query_models.ResourceCategoryCreator{Name: "rc"})
	nt, _ := ctx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{Name: "nt"})
	other := mustCategory(t, ctx, query_models.CategoryCreator{Name: "keep"})

	if err := ctx.DeleteCategory(cat.ID); err != nil {
		t.Fatal(err)
	}
	if err := ctx.DeleteResourceCategory(rc.ID); err != nil {
		t.Fatal(err)
	}
	if err := ctx.DeleteNoteType(nt.ID); err != nil {
		t.Fatal(err)
	}
	var left []models.TemplateRevision
	ctx.db.Find(&left)
	if len(left) != 1 || left[0].CarrierID != other.ID {
		t.Errorf("revisions left after deletes = %+v, want only the surviving carrier's", left)
	}
}

func TestRevision_LabelValidation(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c"})

	s, err := ctx.LabelTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, "  before redesign ")
	if err != nil || s.Label != "before redesign" {
		t.Fatalf("label = %+v, %v", s, err)
	}
	if _, err := ctx.LabelTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, strings.Repeat("é", 201)); !errors.Is(err, ErrTemplateRevisionInvalid) {
		t.Errorf("201 characters = %v", err)
	}
	if _, err := ctx.LabelTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, strings.Repeat("é", 200)); err != nil {
		t.Errorf("200 multi-byte characters must fit: %v", err)
	}
	if s, err := ctx.LabelTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1, ""); err != nil || s.Label != "" {
		t.Errorf("clearing = %+v, %v", s, err)
	}
	if _, err := ctx.LabelTemplateRevision(models.TemplateCarrierCategory, cat.ID, 9, "x"); !errors.Is(err, ErrTemplateRevisionNotFound) {
		t.Errorf("missing revision = %v", err)
	}
}

func TestRevision_SourceFollowsTheIngress(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c"})

	cases := []struct {
		name   string
		accept string
		header string
		want   string
	}{
		{"browser form", "text/html,application/xhtml+xml", "", models.RevisionSourceForm},
		{"mr cli", "application/json", "mr", models.RevisionSourceCLI},
		{"plain api", "application/json", "", models.RevisionSourceAPI},
		{"no accept header", "", "", models.RevisionSourceAPI},
		{"html wins over the cli header", "text/html", "mr", models.RevisionSourceForm},
	}
	for i, tc := range cases {
		req := httptest.NewRequest("POST", "/v1/category", nil)
		if tc.accept != "" {
			req.Header.Set("Accept", tc.accept)
		}
		if tc.header != "" {
			req.Header.Set("X-Mahresources-Client", tc.header)
		}
		scoped := ctx.WithRequest(req).(*MahresourcesContext)
		hdr := string(rune('a' + i))
		if err := updateCategory(scoped, cat.ID, func(e *query_models.CategoryEditor) { e.CustomHeader = hdr }); err != nil {
			t.Fatal(err)
		}
		revs := listRevisionRows(t, ctx, models.TemplateCarrierCategory, cat.ID)
		if got := revs[len(revs)-1].Source; got != tc.want {
			t.Errorf("%s: source = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestRevision_PluginWritesAreStampedPlugin(t *testing.T) {
	ctx := createTestContext(t)
	adapter := &pluginDBAdapter{ctx: ctx}
	_, writer := adapter.BindInvocation(nil)
	w := writer.(*pluginDBAdapter)

	created, err := w.CreateCategory(map[string]any{"name": "plug", "custom_header": "h"})
	if err != nil {
		t.Fatal(err)
	}
	id := uint(created["id"].(float64))
	if _, err := w.PatchCategory(id, map[string]any{"custom_header": "patched"}); err != nil {
		t.Fatal(err)
	}
	revs := listRevisionRows(t, ctx, models.TemplateCarrierCategory, id)
	if len(revs) != 2 || revs[1].Source != models.RevisionSourcePlugin {
		t.Errorf("revisions = %q, want the patch recorded as plugin", revisionSummary(revs))
	}

	// The actor-bound branch builds its context through WithPrincipal; it must
	// stamp the source too.
	bound := adapter.boundContext(nil)
	if bound.revisionSource != models.RevisionSourcePlugin {
		t.Errorf("actor-zero branch source = %q", bound.revisionSource)
	}
}

func TestRevision_ReadsAndWritesNeedTheCarriersEditCapability(t *testing.T) {
	ctx := newRoleCapabilityTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c"})
	rc, _ := ctx.CreateResourceCategory(&query_models.ResourceCategoryCreator{Name: "rc"})
	nt, _ := ctx.CreateOrUpdateNoteType(&query_models.NoteTypeEditor{Name: "nt"})

	carriers := []struct {
		typ       string
		id        uint
		adminOnly bool
	}{
		{models.TemplateCarrierCategory, cat.ID, true},
		{models.TemplateCarrierResourceCategory, rc.ID, true},
		{models.TemplateCarrierNoteType, nt.ID, false},
	}
	principals := []struct {
		name   string
		p      *auth.Principal
		admin  bool
		editor bool
	}{
		{"admin", &auth.Principal{UserID: 1, Role: models.RoleAdmin}, true, true},
		{"editor", &auth.Principal{UserID: 2, Role: models.RoleEditor}, false, true},
		{"unscoped user", &auth.Principal{UserID: 3, Role: models.RoleUser}, false, false},
		{"scoped user", &auth.Principal{UserID: 4, Role: models.RoleUser, ScopeGroupID: ptrUint(1)}, false, false},
		{"guest", &auth.Principal{UserID: 5, Role: models.RoleGuest, ScopeGroupID: ptrUint(1)}, false, false},
	}
	for _, c := range carriers {
		for _, pr := range principals {
			allowed := pr.editor
			if c.adminOnly {
				allowed = pr.admin
			}
			bound := ctx.WithPrincipal(pr.p)
			ops := map[string]func() error{
				"list":    func() error { _, err := bound.ListTemplateRevisions(c.typ, c.id); return err },
				"get":     func() error { _, err := bound.GetTemplateRevision(c.typ, c.id, 1); return err },
				"diff":    func() error { _, err := bound.DiffTemplateRevisions(c.typ, c.id, 1, nil); return err },
				"label":   func() error { _, err := bound.LabelTemplateRevision(c.typ, c.id, 1, "x"); return err },
				"restore": func() error { _, err := bound.RestoreTemplateRevision(c.typ, c.id, 1, nil, true); return err },
			}
			for op, run := range ops {
				err := run()
				if allowed && err != nil {
					t.Errorf("%s %s %s: refused: %v", pr.name, c.typ, op, err)
				}
				if !allowed && !errors.Is(err, ErrRoleCapability) {
					t.Errorf("%s %s %s: err = %v, want ErrRoleCapability", pr.name, c.typ, op, err)
				}
			}
		}
	}
}

func TestRevision_SectionConfigSnapshotKeepsRawText(t *testing.T) {
	ctx := createTestContext(t)
	cat := mustCategory(t, ctx, query_models.CategoryCreator{Name: "c", SectionConfig: `{"ownEntities":null}`})
	d, err := ctx.GetTemplateRevision(models.TemplateCarrierCategory, cat.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.Snapshot["SectionConfig"] != `{"ownEntities":null}` {
		t.Errorf("snapshot must keep the raw text, got %v", d.Snapshot["SectionConfig"])
	}
	resolved := types.JSON(d.Editor["SectionConfig"].(string))
	if strings.Contains(string(resolved), `"ownEntities":null`) || !strings.Contains(string(resolved), `"ownNotes":true`) {
		t.Errorf("editor view must be the resolver's output, got %s", resolved)
	}
}
