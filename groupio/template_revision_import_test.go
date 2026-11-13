package groupio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"gorm.io/gorm"

	"mahresources/archive"
	"mahresources/models"
)

// newRevisionImportCtx is createGUIDIsolatedContext with a database name that is
// unique per invocation. The shared-cache memory database lives as long as any
// connection to it is open, so a fixed name collides with its own previous run
// under -count=2; the pool is closed on cleanup so the database goes away.
func newRevisionImportCtx(t *testing.T, name string) *opCtx {
	t.Helper()
	ctx := createGUIDIsolatedContext(t, fmt.Sprintf("%s_%d", name, time.Now().UnixNano()))
	t.Cleanup(func() {
		if sqlDB, err := ctx.db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return ctx
}

// exportSchemaDefs exports a small tree that carries one Category, one Note
// Type and one Resource Category definition, and returns the archive.
func exportSchemaDefs(t *testing.T, name string) []byte {
	t.Helper()
	src := newRevisionImportCtx(t, name+"_src")

	cat := &models.Category{Name: "RevCat", CustomHeader: "<h1>cat</h1>"}
	nt := &models.NoteType{Name: "RevNoteType", CustomSidebar: "side"}
	rc := &models.ResourceCategory{Name: "RevRC", CustomSummary: "sum"}
	for _, m := range []any{cat, nt, rc} {
		if err := src.db.Create(m).Error; err != nil {
			t.Fatal(err)
		}
	}
	root := mustCreateGroup(t, src, "RevRoot", nil)
	src.db.Model(root).Update("category_id", cat.ID)
	res := mustCreateResource(t, src, "rev.txt", &root.ID, []byte("REV"))
	src.db.Model(res).Update("resource_category_id", rc.ID)
	note := mustCreateNote(t, src, "RevNote", &root.ID)
	src.db.Model(note).Update("note_type_id", nt.ID)

	var buf bytes.Buffer
	if err := src.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true, OwnedNotes: true},
		Fidelity:     archive.ExportFidelity{ResourceBlobs: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true},
	}, &buf, func(ev ProgressEvent) {}); err != nil {
		t.Fatalf("export: %v", err)
	}
	return buf.Bytes()
}

func applyArchive(t *testing.T, dst *opCtx, jobID string, tar []byte) error {
	t.Helper()
	tarPath := filepath.Join("_imports", jobID+".tar")
	dst.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dst.fs, tarPath, tar, 0644)
	plan, err := dst.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	_, err = dst.ApplyImport(context.Background(), jobID, importPlanPath(jobID), buildDefaultDecisions(plan), noopSink{})
	return err
}

func revisionsOf(t *testing.T, db *gorm.DB, carrierType, name string) []models.TemplateRevision {
	t.Helper()
	var id uint
	table := map[string]string{
		models.TemplateCarrierCategory:         "categories",
		models.TemplateCarrierNoteType:         "note_types",
		models.TemplateCarrierResourceCategory: "resource_categories",
	}[carrierType]
	if err := db.Table(table).Where("name = ?", name).Select("id").Scan(&id).Error; err != nil || id == 0 {
		return nil
	}
	var revs []models.TemplateRevision
	db.Where("carrier_type = ? AND carrier_id = ?", carrierType, id).Order("number").Find(&revs)
	return revs
}

func TestApplyImport_CreatedCarriersGetAnImportRevision_ReusedGetNone(t *testing.T) {
	tar := exportSchemaDefs(t, "rev_import")
	dst := newRevisionImportCtx(t, "rev_import_dst")

	if err := applyArchive(t, dst, "rev-import-1", tar); err != nil {
		t.Fatalf("apply: %v", err)
	}
	for carrierType, name := range map[string]string{
		models.TemplateCarrierCategory:         "RevCat",
		models.TemplateCarrierNoteType:         "RevNoteType",
		models.TemplateCarrierResourceCategory: "RevRC",
	} {
		revs := revisionsOf(t, dst.db, carrierType, name)
		if len(revs) != 1 || revs[0].Number != 1 || revs[0].Source != models.RevisionSourceImport {
			t.Errorf("%s %s: revisions = %+v, want exactly #1 import", carrierType, name, revs)
			continue
		}
		snap, _ := models.ParseTemplateSnapshot(revs[0].Snapshot)
		if carrierType == models.TemplateCarrierCategory && snap["CustomHeader"] != "<h1>cat</h1>" {
			t.Errorf("import revision snapshot lost the imported content: %v", snap["CustomHeader"])
		}
	}

	// A second import reuses the carriers by GUID and must not touch their history.
	if err := applyArchive(t, dst, "rev-import-2", tar); err != nil {
		t.Fatalf("second apply: %v", err)
	}
	var total int64
	dst.db.Model(&models.TemplateRevision{}).Count(&total)
	if total != 3 {
		t.Errorf("template_revisions after re-import = %d, want 3 (reused carriers record nothing)", total)
	}
}

// fakeCarrierAuthority refuses the carrier types in deny and records which
// carrier types it was asked to validate, failing those in invalid.
type fakeCarrierAuthority struct {
	deny      map[string]bool
	invalid   map[string]bool
	validated []string
}

func (f *fakeCarrierAuthority) AuthorTemplateCarrier(carrierType string) error {
	if f.deny[carrierType] {
		return errors.New("denied")
	}
	return nil
}

func (f *fakeCarrierAuthority) ValidateTemplateCarrier(carrierType string, snap models.TemplateSnapshot) error {
	f.validated = append(f.validated, carrierType)
	if f.invalid[carrierType] {
		return errors.New("bad payload")
	}
	return nil
}

func applyArchiveResult(t *testing.T, dst *opCtx, jobID string, tar []byte) (*ImportApplyResult, error) {
	t.Helper()
	tarPath := filepath.Join("_imports", jobID+".tar")
	dst.fs.MkdirAll("_imports", 0755)
	afero.WriteFile(dst.fs, tarPath, tar, 0644)
	plan, err := dst.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return dst.ApplyImport(context.Background(), jobID, importPlanPath(jobID), buildDefaultDecisions(plan), noopSink{})
}

// A carrier whose template payload the principal may not author is created
// bare, before its import revision is taken, with a warning; one it may author
// keeps its payload and is validated. (Issue #100; the role answers come from
// application_context, so a fake stands in for them here.)
func TestApplyImport_CarrierPayloadFollowsTheCarrierAuthority(t *testing.T) {
	tar := exportSchemaDefs(t, "carrier_authority")
	dst := newRevisionImportCtx(t, "carrier_authority_dst")
	fake := &fakeCarrierAuthority{deny: map[string]bool{
		models.TemplateCarrierCategory:         true,
		models.TemplateCarrierResourceCategory: true,
	}}
	dst.carriers = fake

	result, err := applyArchiveResult(t, dst, "carrier-authority-1", tar)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	var cat models.Category
	var nt models.NoteType
	var rc models.ResourceCategory
	dst.db.Where("name = ?", "RevCat").First(&cat)
	dst.db.Where("name = ?", "RevNoteType").First(&nt)
	dst.db.Where("name = ?", "RevRC").First(&rc)
	if cat.ID == 0 || nt.ID == 0 || rc.ID == 0 {
		t.Fatalf("a carrier row was not created: cat=%d nt=%d rc=%d", cat.ID, nt.ID, rc.ID)
	}
	if cat.CustomHeader != "" || rc.CustomSummary != "" {
		t.Errorf("denied carriers kept their templates: cat=%q rc=%q", cat.CustomHeader, rc.CustomSummary)
	}
	if nt.CustomSidebar != "side" {
		t.Errorf("allowed note type lost its template: %q", nt.CustomSidebar)
	}
	snap, _ := models.ParseTemplateSnapshot(revisionsOf(t, dst.db, models.TemplateCarrierCategory, "RevCat")[0].Snapshot)
	if snap["CustomHeader"] != "" {
		t.Errorf("the stripped template reached the import revision: %v", snap["CustomHeader"])
	}
	for _, name := range []string{"RevCat", "RevRC"} {
		found := false
		for _, w := range result.Warnings {
			if strings.Contains(w, name) {
				found = true
			}
		}
		if !found {
			t.Errorf("no warning names stripped carrier %s: %v", name, result.Warnings)
		}
	}
	if len(fake.validated) != 1 || fake.validated[0] != models.TemplateCarrierNoteType {
		t.Errorf("validated %v, want only the note type the principal may author", fake.validated)
	}
}

// A field the authority refuses is cleared with a warning
// (TestImport_CarrierTemplatesAreValidatedLikeASave). A payload it still
// refuses once every refused field is cleared fails the apply, and the carrier
// is not created: what is stored always passes.
func TestApplyImport_InvalidCarrierPayloadFailsTheApply(t *testing.T) {
	tar := exportSchemaDefs(t, "carrier_invalid")
	dst := newRevisionImportCtx(t, "carrier_invalid_dst")
	dst.carriers = &fakeCarrierAuthority{invalid: map[string]bool{models.TemplateCarrierResourceCategory: true}}

	_, err := applyArchiveResult(t, dst, "carrier-invalid-1", tar)
	if err == nil || !strings.Contains(err.Error(), "RevRC") {
		t.Fatalf("apply err = %v, want a refusal naming RevRC", err)
	}
	var n int64
	dst.db.Model(&models.ResourceCategory{}).Where("name = ?", "RevRC").Count(&n)
	if n != 0 {
		t.Errorf("the invalid resource category was created")
	}
}

// If the revision insert fails after the carrier insert, neither may commit:
// otherwise a retry would reuse the carrier by GUID and never record it. The
// failure is injected by a GORM callback between the two inserts, not by a race.
func TestApplyImport_FailureBetweenCarrierAndRevisionLeavesNeither(t *testing.T) {
	tar := exportSchemaDefs(t, "rev_atomic")
	dst := newRevisionImportCtx(t, "rev_atomic_dst")

	boom := errors.New("injected revision insert failure")
	const cbName = "test:fail_revision_insert"
	if err := dst.db.Callback().Create().Before("gorm:create").Register(cbName, func(tx *gorm.DB) {
		if tx.Statement.Schema != nil && tx.Statement.Schema.Table == "template_revisions" {
			_ = tx.AddError(boom)
		}
	}); err != nil {
		t.Fatal(err)
	}

	err := applyArchive(t, dst, "rev-atomic-1", tar)
	if err == nil || !strings.Contains(err.Error(), "injected revision insert failure") {
		t.Fatalf("apply err = %v, want the injected failure", err)
	}
	var carriers int64
	dst.db.Model(&models.Category{}).Where("name = ?", "RevCat").Count(&carriers)
	if carriers != 0 {
		t.Fatalf("the Category committed without its revision (%d rows)", carriers)
	}

	// Retry with the fault removed: the carrier is created, with its revision.
	if err := dst.db.Callback().Create().Remove(cbName); err != nil {
		t.Fatal(err)
	}
	if err := applyArchive(t, dst, "rev-atomic-2", tar); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if revs := revisionsOf(t, dst.db, models.TemplateCarrierCategory, "RevCat"); len(revs) != 1 || revs[0].Source != models.RevisionSourceImport {
		t.Errorf("after retry revisions = %+v", revs)
	}
}
