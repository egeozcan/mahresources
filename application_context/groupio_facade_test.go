package application_context

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/spf13/afero"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"mahresources/archive"
	"mahresources/auth"
	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/types"
)

// Tests for the transaction, scope, and actor semantics of the import/export
// facade. They must live in package application_context: they need
// WithPrincipal, WithTransaction, and the callbacks registerScopeCallbacks
// installs, and application_context imports groupio, so a groupio test
// importing back would be a cycle.
//
// Every helper below is defined locally on purpose. The obvious ones
// (createGUIDIsolatedContext, buildDefaultDecisions, mustCreateGroup, noopSink)
// live in apply_import_test.go and export_context_test.go, both of which move
// to groupio — depending on them would make this file straddle the seam, which
// is the exact defect the export_scope_test.go split had to undo.

// facadeSink satisfies download_queue.ProgressSink without pulling in the
// moving noopSink.
type facadeSink struct{}

func (facadeSink) SetPhase(string)               {}
func (facadeSink) SetPhaseProgress(int64, int64) {}
func (facadeSink) UpdateProgress(int64, int64)   {}
func (facadeSink) AppendWarning(string)          {}
func (facadeSink) SetResultPath(string)          {}

// createFacadeContext builds a context on a DB private to this test, through
// NewMahresourcesContext so the scope and create-stamp callbacks are installed.
// A bare gorm.Open would register neither, and every assertion here would pass
// against a context that enforced nothing.
func createFacadeContext(t *testing.T, name string) *MahresourcesContext {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(testSQLiteMemoryDSN(name)), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.Query{}, &models.Resource{}, &models.ResourceVersion{}, &models.Note{},
		&models.Tag{}, &models.Group{}, &models.Category{}, &models.NoteType{}, &models.TemplateRevision{},
		&models.Preview{}, &models.GroupRelation{}, &models.GroupRelationType{},
		&models.ImageHash{}, &models.ResourceSimilarity{}, &models.LogEntry{},
		&models.ResourceCategory{}, &models.Series{}, &models.NoteBlock{}, &models.PluginKV{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	config := &MahresourcesConfig{DbType: constants.DbTypeSqlite}
	sqlDB, _ := db.DB()
	ctx := NewMahresourcesContext(afero.NewMemMapFs(), db, sqlx.NewDb(sqlDB, "sqlite3"), config)

	defaultRC := &models.ResourceCategory{Name: "Default", Description: "Default resource category."}
	defaultRC.ID = 1
	db.FirstOrCreate(defaultRC, 1)
	return ctx
}

// facadeGUID reads one row's GUID, failing the test if it is absent — a blank
// GUID would make every "GUID not in archive" assertion pass for free.
func facadeGUID(t *testing.T, ctx *MahresourcesContext, model any, id uint) string {
	t.Helper()
	var guids []string
	if err := ctx.db.Model(model).Where("id = ?", id).Pluck("guid", &guids).Error; err != nil {
		t.Fatalf("read guid for %T %d: %v", model, id, err)
	}
	if len(guids) != 1 || guids[0] == "" {
		t.Fatalf("expected exactly one non-empty guid for %T %d, got %v", model, id, guids)
	}
	return guids[0]
}

func mustCreateFacade(t *testing.T, ctx *MahresourcesContext, v any) {
	t.Helper()
	if err := ctx.db.Create(v).Error; err != nil {
		t.Fatalf("create %T: %v", v, err)
	}
}

// facadeDecisions accepts every suggestion, dropping dangling refs. A local
// copy of the same shape buildDefaultDecisions builds.
func facadeDecisions(plan *ImportPlan) *ImportDecisions {
	d := &ImportDecisions{
		ResourceCollisionPolicy: "skip",
		MappingActions:          map[string]MappingAction{},
		DanglingActions:         map[string]DanglingAction{},
		ShellGroupActions:       map[string]ShellGroupAction{},
	}
	for _, group := range [][]MappingEntry{
		plan.Mappings.Categories, plan.Mappings.NoteTypes, plan.Mappings.ResourceCategories,
		plan.Mappings.Tags, plan.Mappings.GroupRelationTypes,
	} {
		for _, entry := range group {
			action := entry.Suggestion
			if action == "" {
				action = "create"
			}
			d.MappingActions[entry.DecisionKey] = MappingAction{
				Include: true, Action: action, DestinationID: entry.DestinationID,
			}
		}
	}
	for _, dr := range plan.DanglingRefs {
		d.DanglingActions[dr.ID] = DanglingAction{Action: "drop"}
	}
	return d
}

// (1) Handle propagation. The facade must read the caller's *gorm.DB at call
// time, never a handle captured at construction. Transaction membership,
// subtree scope, and actor identity all ride on that handle, so a captured one
// silently escapes all three.
//
// This is NOT a nesting test. GORM's Begin switches on Statement.ConnPool;
// inside WithTransaction that pool is a *sql.Tx, which satisfies neither
// TxBeginner nor ConnPoolBeginner, so it returns ErrInvalidTransaction. Since
// apply_import.go opens its batches with s.ctx.db.Begin(), a nested ApplyImport
// would abort at the first batch and prove nothing — and ApplyImport is nested
// nowhere in production. The assertion is propagation, not nesting.
func TestGroupioFacade_ObservesDerivedHandleNotSingleton(t *testing.T) {
	t.Run("transaction", func(t *testing.T) {
		ctx := createFacadeContext(t, "facade_tx")

		// A group that exists only inside the transaction. Only the tx handle
		// can see it; the singleton handle is a different connection.
		errRollback := errors.New("rollback")
		err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
			g := &models.Group{Name: "tx-only-group"}
			if err := txCtx.db.Create(g).Error; err != nil {
				return err
			}

			// Direct: the deps handed to the service must BE the transactional
			// handle. Asserted by identity because it is the actual invariant;
			// the behavioural check below can be satisfied accidentally by
			// SQLite shared-cache visibility.
			if got := txCtx.groupioDeps().DB; got != txCtx.db {
				t.Errorf("groupioDeps() inside a transaction handed the service %p, want the "+
					"transactional handle %p — its writes would escape the transaction",
					got, txCtx.db)
			}

			est, err := txCtx.EstimateExport(&ExportRequest{
				RootGroupIDs: []uint{g.ID},
				Scope:        archive.ExportScope{Subtree: true},
			})
			if err != nil {
				return err
			}
			if est.Counts.Groups != 1 {
				t.Errorf("export inside a transaction saw %d groups, want 1 — the facade did not "+
					"observe the transactional handle", est.Counts.Groups)
			}
			return errRollback
		})
		if err != nil && !errors.Is(err, errRollback) {
			t.Fatalf("transaction: %v", err)
		}

		// The rollback took effect, confirming the group really was tx-only and
		// the assertion above was not reading a committed row.
		var count int64
		ctx.db.Model(&models.Group{}).Where("name = ?", "tx-only-group").Count(&count)
		if count != 0 {
			t.Fatalf("tx-only group survived rollback (%d rows); the sub-test proved nothing", count)
		}
	})

	t.Run("principal", func(t *testing.T) {
		ctx := createFacadeContext(t, "facade_principal")

		root := &models.Group{Name: "fp-root"}
		mustCreateFacade(t, ctx, root)
		outside := &models.Group{Name: "fp-outside"}
		mustCreateFacade(t, ctx, outside)

		// An M2M edge, deliberately not a typed GroupRelation. The relation BFS
		// is confined through Deps.Scope, which resolves against ctx.db and so
		// stays correct even when Deps.DB is wrong — a GroupRelation fixture
		// here would pass against a facade holding a captured handle. The M2M
		// branch is guarded by the GORM callbacks, which read Deps.DB, so it is
		// the branch that actually detects the wrong handle.
		if err := ctx.db.Model(root).Association("RelatedGroups").Append(outside); err != nil {
			t.Fatalf("relate groups: %v", err)
		}

		req := fullBFSRequest(root.ID)

		// Direct: the deps must carry this derived context's handle.
		scopedCtx := ctx.WithPrincipal(scopedGuest(root.ID))
		if got := scopedCtx.groupioDeps().DB; got != scopedCtx.db {
			t.Errorf("groupioDeps() on a principal-derived context handed the service %p, want the "+
				"derived handle %p — scope would not be enforced", got, scopedCtx.db)
		}

		// Control: on the singleton handle the edge is followed.
		ctrl, err := ctx.EstimateExport(req)
		if err != nil {
			t.Fatalf("control EstimateExport: %v", err)
		}
		if ctrl.Counts.Groups != 2 {
			t.Fatalf("control invalid: singleton export saw %d groups, want 2", ctrl.Counts.Groups)
		}

		// On the derived handle it must not be. A facade holding the singleton
		// would return the control's answer here.
		est, err := scopedCtx.EstimateExport(req)
		if err != nil {
			t.Fatalf("scoped EstimateExport: %v", err)
		}
		if est.Counts.Groups != 1 {
			t.Errorf("export through a principal-derived context saw %d groups, want 1 — the facade "+
				"used the singleton handle, so scope was not enforced", est.Counts.Groups)
		}
	})
}

// (2b) Scoped export, asserted on archive contents rather than status codes.
//
// The guest is scoped to scopeRoot but exports from exportRoot, a strict subset
// of its scope. That gap is what makes the positive assertion mean something:
// sibling/sibRes/sibNote are inside the guest's scope but outside the export
// subtree, so Phase A cannot seed them and only BFS traversal can bring them
// in. With the guest scoped to the export root itself, Subtree:true pre-seeds
// every in-scope entity and the positive assertion passes even with traversal
// switched off entirely.
//
// Both traversal branches are exercised: they are independently gated on
// Scope.RelatedM2M and Scope.GroupRelations, whose JSON zero value is false, so
// a request setting only relatedDepth exercises neither.
func TestScopedExport_ArchiveConfinedAcrossBothTraversalKinds(t *testing.T) {
	ctx := createFacadeContext(t, "facade_2b")

	scopeRoot := &models.Group{Name: "b2b-scoperoot"}
	mustCreateFacade(t, ctx, scopeRoot)
	exportRoot := &models.Group{Name: "b2b-exportroot", OwnerId: &scopeRoot.ID}
	mustCreateFacade(t, ctx, exportRoot)

	// In scope, outside the export subtree: reachable only by traversal.
	sibling := &models.Group{Name: "b2b-sibling-visible", OwnerId: &scopeRoot.ID}
	mustCreateFacade(t, ctx, sibling)
	sibRes := &models.Resource{Name: "b2b-sibres-visible", OwnerId: &sibling.ID, Hash: "aaaa1111", FileSize: 3}
	mustCreateFacade(t, ctx, sibRes)
	sibNote := &models.Note{Name: "b2b-sibnote-visible", OwnerId: &sibling.ID}
	mustCreateFacade(t, ctx, sibNote)

	// Out of scope entirely: must never appear.
	outside := &models.Group{Name: "b2b-outside-secret"}
	mustCreateFacade(t, ctx, outside)
	outRes := &models.Resource{Name: "b2b-outres-secret", OwnerId: &outside.ID, Hash: "bbbb2222", FileSize: 5}
	mustCreateFacade(t, ctx, outRes)
	outNote := &models.Note{Name: "b2b-outnote-secret", OwnerId: &outside.ID}
	mustCreateFacade(t, ctx, outNote)

	// M2M edges of all three kinds, each crossing the boundary and each with an
	// in-scope counterpart.
	assoc := ctx.db.Model(exportRoot)
	if err := assoc.Association("RelatedGroups").Append(sibling, outside); err != nil {
		t.Fatalf("relate groups: %v", err)
	}
	if err := ctx.db.Model(exportRoot).Association("RelatedResources").Append(sibRes, outRes); err != nil {
		t.Fatalf("relate resources: %v", err)
	}
	if err := ctx.db.Model(exportRoot).Association("RelatedNotes").Append(sibNote, outNote); err != nil {
		t.Fatalf("relate notes: %v", err)
	}

	// Typed GroupRelation edges, likewise.
	rt := &models.GroupRelationType{Name: "b2b-type"}
	mustCreateFacade(t, ctx, rt)
	mustCreateFacade(t, ctx, &models.GroupRelation{FromGroupId: &exportRoot.ID, ToGroupId: &sibling.ID, RelationTypeId: &rt.ID})
	mustCreateFacade(t, ctx, &models.GroupRelation{FromGroupId: &exportRoot.ID, ToGroupId: &outside.ID, RelationTypeId: &rt.ID})

	req := fullBFSRequest(exportRoot.ID)

	// (3) Unscoped control: the edges ARE followable, so the scoped run's
	// absences below are enforcement and not a no-op traversal.
	var ctrlBuf bytes.Buffer
	if err := ctx.StreamExport(context.Background(), req, &ctrlBuf, func(ev ProgressEvent) {}); err != nil {
		t.Fatalf("control StreamExport: %v", err)
	}
	for _, name := range []string{"b2b-outside-secret", "b2b-outres-secret", "b2b-outnote-secret"} {
		if !bytes.Contains(ctrlBuf.Bytes(), []byte(name)) {
			t.Fatalf("control invalid: unscoped export did not reach %q, so the scoped assertions "+
				"would prove nothing", name)
		}
	}

	var buf bytes.Buffer
	if err := ctx.WithPrincipal(scopedGuest(scopeRoot.ID)).
		StreamExport(context.Background(), req, &buf, func(ev ProgressEvent) {}); err != nil {
		t.Fatalf("scoped StreamExport: %v", err)
	}

	// (1) Positive traversal: in-scope targets outside the export subtree are
	// present, so the BFS genuinely ran under scope.
	for _, name := range []string{"b2b-sibling-visible", "b2b-sibres-visible", "b2b-sibnote-visible"} {
		if !bytes.Contains(buf.Bytes(), []byte(name)) {
			t.Errorf("in-scope traversal target %q missing from the scoped archive; confinement must "+
				"filter by visibility, not disable the traversal", name)
		}
	}

	// (2) Negative confinement: nothing out of scope, anywhere in the tar. The
	// manifest is written from plan state before payloads load, so checking only
	// payload files would miss a leaked name or GUID.
	for _, secret := range []string{"b2b-outside-secret", "b2b-outres-secret", "b2b-outnote-secret"} {
		if bytes.Contains(buf.Bytes(), []byte(secret)) {
			t.Errorf("out-of-scope name %q present in the scoped export archive", secret)
		}
	}
	guids := map[string]string{
		"group":    facadeGUID(t, ctx, &models.Group{}, outside.ID),
		"resource": facadeGUID(t, ctx, &models.Resource{}, outRes.ID),
		"note":     facadeGUID(t, ctx, &models.Note{}, outNote.ID),
	}
	for kind, g := range guids {
		if bytes.Contains(buf.Bytes(), []byte(g)) {
			t.Errorf("out-of-scope %s GUID %q present in the scoped export archive", kind, g)
		}
		// The unscoped control must contain it, or the check above is trivially
		// satisfied by a GUID that never reaches any archive.
		if !bytes.Contains(ctrlBuf.Bytes(), []byte(g)) {
			t.Errorf("control invalid: unscoped archive lacks the %s GUID %q, so its absence from the "+
				"scoped archive proves nothing", kind, g)
		}
	}
}

// (3) Actor attribution. Import runs on a WithPrincipal-derived context, so the
// acting user rides the db context and the create-stamp callback must attribute
// every row it writes. Route denial targets group-limited principals; the
// unscoped users, editors, and admins who actually import still get stamped.
func TestImport_StampsActingUserOnCreatedRows(t *testing.T) {
	srcCtx := createFacadeContext(t, "facade_attr_src")

	root := &models.Group{Name: "attr-root"}
	mustCreateFacade(t, srcCtx, root)
	child := &models.Group{Name: "attr-child", OwnerId: &root.ID}
	mustCreateFacade(t, srcCtx, child)
	mustCreateFacade(t, srcCtx, &models.Note{Name: "attr-note", OwnerId: &child.ID})

	var tarBuf bytes.Buffer
	if err := srcCtx.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedNotes: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true, Tags: true},
	}, &tarBuf, func(ev ProgressEvent) {}); err != nil {
		t.Fatalf("export: %v", err)
	}

	// A separate DB, so the import creates rows instead of resolving GUID
	// collisions against the source's (which would keep the original stamp and
	// make the assertion vacuous).
	dstCtx := createFacadeContext(t, "facade_attr_dst")

	jobID := "facade-attr-001"
	tarPath := filepath.Join("_imports", jobID+".tar")
	if err := dstCtx.fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(dstCtx.fs, tarPath, tarBuf.Bytes(), 0644); err != nil {
		t.Fatalf("stage tar: %v", err)
	}

	const actorID uint = 4242
	importer := dstCtx.WithPrincipal(&auth.Principal{UserID: actorID, Role: models.RoleEditor})

	plan, err := importer.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("ParseImport: %v", err)
	}
	if _, err := importer.ApplyImport(context.Background(), jobID, importPlanPathFor(jobID), facadeDecisions(plan), facadeSink{}); err != nil {
		t.Fatalf("ApplyImport: %v", err)
	}

	// Read back on the unscoped handle; the importer's is fine too but this
	// keeps the assertion independent of the scope callbacks.
	assertStamped := func(model any, label string) {
		t.Helper()
		var total, stamped int64
		dstCtx.db.Model(model).Count(&total)
		if total == 0 {
			t.Fatalf("no %s rows were created, so attribution is untested", label)
		}
		dstCtx.db.Model(model).Where("created_by_user_id = ?", actorID).Count(&stamped)
		if stamped != total {
			t.Errorf("%s: %d of %d rows stamped with the acting user %d; import writes must be attributed",
				label, stamped, total, actorID)
		}
	}
	assertStamped(&models.Group{}, "groups")
	assertStamped(&models.Note{}, "notes")
}

// exportTemplateCarriers exports a tree that references one Category, one Note
// Type and one Resource Category, each carrying template payload, and returns
// the archive. The carriers are inserted with db.Create so the save validation
// is skipped and an invalid schema or rule set can be put in an archive.
func exportTemplateCarriers(t *testing.T, name, categorySchema, rules string) []byte {
	t.Helper()
	src := createFacadeContext(t, name+"_src")

	cat := &models.Category{Name: "GateCat", Description: "cat desc", CustomHeader: "<h1>x</h1>",
		CustomCSS: "body{}", MetaSchema: categorySchema, MetadataIndexes: `[{"key":"score","kind":"numeric"}]`}
	nt := &models.NoteType{Name: "GateNoteType", Description: "nt desc", CustomSidebar: "<b>side</b>",
		CustomCSS: "body{}", ApplyTemplatesToShares: true}
	rc := &models.ResourceCategory{Name: "GateRC", Description: "rc desc", CustomHeader: "<h1>rc</h1>",
		CustomCSS: "body{}", AutoDetectRules: rules}
	for _, m := range []any{cat, nt, rc} {
		mustCreateFacade(t, src, m)
	}
	root := &models.Group{Name: "gate-root", CategoryId: &cat.ID}
	mustCreateFacade(t, src, root)
	mustCreateFacade(t, src, &models.Note{Name: "gate-note", OwnerId: &root.ID, NoteTypeId: &nt.ID})
	mustCreateFacade(t, src, &models.Resource{Name: "gate-res", OwnerId: &root.ID, Hash: "cafe0100",
		FileSize: 3, ResourceCategoryId: rc.ID})

	var buf bytes.Buffer
	if err := src.StreamExport(context.Background(), &ExportRequest{
		RootGroupIDs: []uint{root.ID},
		Scope:        archive.ExportScope{Subtree: true, OwnedResources: true, OwnedNotes: true},
		SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true},
	}, &buf, func(ev ProgressEvent) {}); err != nil {
		t.Fatalf("export: %v", err)
	}
	return buf.Bytes()
}

// importFacadeArchive stages tar and runs parse then apply on importer,
// accepting every suggestion as the review UI's defaults do.
func importFacadeArchive(t *testing.T, importer *MahresourcesContext, jobID string, tar []byte) (*ImportApplyResult, error) {
	t.Helper()
	tarPath := filepath.Join("_imports", jobID+".tar")
	if err := importer.fs.MkdirAll("_imports", 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := afero.WriteFile(importer.fs, tarPath, tar, 0644); err != nil {
		t.Fatalf("stage tar: %v", err)
	}
	plan, err := importer.ParseImport(context.Background(), jobID, tarPath)
	if err != nil {
		t.Fatalf("ParseImport: %v", err)
	}
	return importer.ApplyImport(context.Background(), jobID, importPlanPathFor(jobID), facadeDecisions(plan), facadeSink{})
}

// Issue #100. Group import is capWrite, but a Category or Resource Category's
// templates, schema and auto-detect rules are admin-only content and a Note
// Type's are editor-only. Import creates the rows at capWrite and keeps their
// payload only at the carrier's edit role; whatever it keeps passes the save
// path's validation.
func TestImport_CarrierTemplatesNeedTheCarriersEditRole(t *testing.T) {
	const schema = `{"type":"object","properties":{"x":{"type":"string"}}}`
	const rules = `{"contentTypes":["image/png"],"priority":1000000}`
	tar := exportTemplateCarriers(t, "carrier_gate", schema, rules)

	type kept struct{ category, noteType, resourceCategory bool }
	for _, tc := range []struct {
		name      string
		principal *auth.Principal
		keep      kept
	}{
		{"user", &auth.Principal{UserID: 501, Role: models.RoleUser}, kept{}},
		{"editor", &auth.Principal{UserID: 502, Role: models.RoleEditor}, kept{noteType: true}},
		{"admin", &auth.Principal{UserID: 503, Role: models.RoleAdmin}, kept{true, true, true}},
		{"no_principal", nil, kept{true, true, true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dst := createFacadeContext(t, "carrier_gate_dst_"+tc.name)
			importer := dst
			if tc.principal != nil {
				importer = dst.WithPrincipal(tc.principal)
			}
			result, err := importFacadeArchive(t, importer, "carrier-gate-"+tc.name, tar)
			if err != nil {
				t.Fatalf("ApplyImport: %v", err)
			}

			var cat models.Category
			var nt models.NoteType
			var rc models.ResourceCategory
			if err := dst.db.Where("name = ?", "GateCat").First(&cat).Error; err != nil {
				t.Fatalf("category row was not created: %v", err)
			}
			if err := dst.db.Where("name = ?", "GateNoteType").First(&nt).Error; err != nil {
				t.Fatalf("note type row was not created: %v", err)
			}
			if err := dst.db.Where("name = ?", "GateRC").First(&rc).Error; err != nil {
				t.Fatalf("resource category row was not created: %v", err)
			}

			check := func(carrierType string, carrier any, id uint, name, desc, gotDesc, metadataIndexes string, keep bool) {
				t.Helper()
				if gotDesc != desc {
					t.Errorf("%s description = %q, want %q: the row itself is capWrite", name, gotDesc, desc)
				}
				warned := false
				for _, w := range result.Warnings {
					if strings.Contains(w, name) && strings.Contains(w, "without its templates") {
						warned = true
					}
				}
				var revs []models.TemplateRevision
				dst.db.Where("carrier_type = ? AND carrier_id = ?", carrierType, id).Order("number").Find(&revs)
				if len(revs) != 1 {
					t.Fatalf("%s has %d revisions, want the import revision #1", name, len(revs))
				}
				revSnap, err := models.ParseTemplateSnapshot(revs[0].Snapshot)
				if err != nil {
					t.Fatal(err)
				}
				snap := models.NewTemplateSnapshot(carrierType, carrier)
				if keep {
					if models.TemplateSnapshotsEqual(carrierType, snap, models.TemplateSnapshot{}) {
						t.Errorf("%s lost its templates although this role may author them", name)
					}
					if warned {
						t.Errorf("%s: unexpected stripped-templates warning %v", name, result.Warnings)
					}
					return
				}
				for field, v := range snap {
					if v != "" && v != false {
						t.Errorf("%s.%s = %v, want empty: this role may not author it", name, field, v)
					}
				}
				for field, v := range revSnap {
					if v != "" && v != false {
						t.Errorf("%s revision #1 %s = %v, want empty: the payload must not reach history", name, field, v)
					}
				}
				if metadataIndexes != "" {
					t.Errorf("%s.MetadataIndexes = %q, want empty", name, metadataIndexes)
				}
				if !warned {
					t.Errorf("%s was stripped without a warning; warnings = %v", name, result.Warnings)
				}
			}
			check(models.TemplateCarrierCategory, &cat, cat.ID, "GateCat", "cat desc", cat.Description, cat.MetadataIndexes, tc.keep.category)
			check(models.TemplateCarrierNoteType, &nt, nt.ID, "GateNoteType", "nt desc", nt.Description, nt.MetadataIndexes, tc.keep.noteType)
			check(models.TemplateCarrierResourceCategory, &rc, rc.ID, "GateRC", "rc desc", rc.Description, rc.MetadataIndexes, tc.keep.resourceCategory)

			if tc.keep.noteType && !nt.ApplyTemplatesToShares {
				t.Errorf("note type lost ApplyTemplatesToShares although this role may author it")
			}

			// The rules are what gives an imported Resource Category its reach:
			// they claim every upload that names no category.
			want := dst.DefaultResourceCategoryID
			if tc.keep.resourceCategory {
				want = rc.ID
			}
			if got := dst.detectResourceCategory("image/png", 10, 10, 100); got != want {
				t.Errorf("an upload of image/png was detected as category %d, want %d", got, want)
			}
		})
	}
}

// Whatever an import keeps passes the same validation as a save: an archive
// cannot carry rules or a schema the editor would refuse, even for an admin.
func TestImport_CarrierTemplatesAreValidatedLikeASave(t *testing.T) {
	const okSchema = `{"type":"object"}`
	const okRules = `{"contentTypes":["image/png"]}`
	for _, tc := range []struct {
		name, schema, rules, carrier string
		model                        any
	}{
		{"invalid_rules", okSchema, `{"bogus":1}`, "GateRC", &models.ResourceCategory{}},
		{"external_schema_ref", `{"$ref":"file:///etc/passwd"}`, okRules, "GateCat", &models.Category{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tar := exportTemplateCarriers(t, "carrier_validate_"+tc.name, tc.schema, tc.rules)
			dst := createFacadeContext(t, "carrier_validate_dst_"+tc.name)
			admin := dst.WithPrincipal(&auth.Principal{UserID: 601, Role: models.RoleAdmin})

			_, err := importFacadeArchive(t, admin, "carrier-validate-"+tc.name, tar)
			if err == nil || !strings.Contains(err.Error(), tc.carrier) {
				t.Fatalf("ApplyImport err = %v, want a refusal naming %s", err, tc.carrier)
			}
			var n int64
			dst.db.Model(tc.model).Where("name = ?", tc.carrier).Count(&n)
			if n != 0 {
				t.Errorf("%s was created with content the save path refuses", tc.carrier)
			}
		})
	}
}

// A SectionConfig is payload even when the section config resolver reads it as
// the defaults: a config that does not unmarshal, unknown keys, or restated
// defaults all resolve to the defaults, but the raw column is what the edit
// page hands to sectionConfigForm.js. A carrier whose only payload is such a
// config must still be stripped below its edit role, with the warning.
func TestImport_SectionConfigOnlyCarriersAreStrippedBelowTheEditRole(t *testing.T) {
	for _, tc := range []struct{ name, sectionConfig string }{
		{"proto_key", `{"__proto__":{"polluted":"yes"}}`},
		{"wrong_type", `{"tags":false,"timestamps":"x","evil":"<script>alert(1)</script>"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := createFacadeContext(t, "sc_only_src_"+tc.name)
			sc := types.JSON(tc.sectionConfig)
			cat := &models.Category{Name: "ScCat", SectionConfig: sc}
			nt := &models.NoteType{Name: "ScNoteType", SectionConfig: sc}
			rc := &models.ResourceCategory{Name: "ScRC", SectionConfig: sc}
			for _, m := range []any{cat, nt, rc} {
				mustCreateFacade(t, src, m)
			}
			root := &models.Group{Name: "sc-root", CategoryId: &cat.ID}
			mustCreateFacade(t, src, root)
			mustCreateFacade(t, src, &models.Note{Name: "sc-note", OwnerId: &root.ID, NoteTypeId: &nt.ID})
			mustCreateFacade(t, src, &models.Resource{Name: "sc-res", OwnerId: &root.ID, Hash: "cafe0101",
				FileSize: 3, ResourceCategoryId: rc.ID})
			var buf bytes.Buffer
			if err := src.StreamExport(context.Background(), &ExportRequest{
				RootGroupIDs: []uint{root.ID},
				Scope:        archive.ExportScope{Subtree: true, OwnedResources: true, OwnedNotes: true},
				SchemaDefs:   archive.ExportSchemaDefs{CategoriesAndTypes: true},
			}, &buf, func(ev ProgressEvent) {}); err != nil {
				t.Fatalf("export: %v", err)
			}

			dst := createFacadeContext(t, "sc_only_dst_"+tc.name)
			user := dst.WithPrincipal(&auth.Principal{UserID: 701, Role: models.RoleUser})
			result, err := importFacadeArchive(t, user, "sc-only-"+tc.name, buf.Bytes())
			if err != nil {
				t.Fatalf("ApplyImport: %v", err)
			}

			for _, c := range []struct {
				carrierType, name, table string
			}{
				{models.TemplateCarrierCategory, "ScCat", "categories"},
				{models.TemplateCarrierNoteType, "ScNoteType", "note_types"},
				{models.TemplateCarrierResourceCategory, "ScRC", "resource_categories"},
			} {
				var row struct {
					ID            uint
					SectionConfig *string
				}
				if err := dst.db.Table(c.table).Select("id, section_config").Where("name = ?", c.name).Take(&row).Error; err != nil {
					t.Fatalf("%s row was not created: %v", c.name, err)
				}
				if row.SectionConfig != nil && *row.SectionConfig != "" {
					t.Errorf("%s.SectionConfig = %q, want empty: a user may not author it", c.name, *row.SectionConfig)
				}
				var revs []models.TemplateRevision
				dst.db.Where("carrier_type = ? AND carrier_id = ?", c.carrierType, row.ID).Find(&revs)
				if len(revs) != 1 {
					t.Fatalf("%s has %d revisions, want the import revision #1", c.name, len(revs))
				}
				revSnap, err := models.ParseTemplateSnapshot(revs[0].Snapshot)
				if err != nil {
					t.Fatal(err)
				}
				if v, _ := revSnap["SectionConfig"].(string); v != "" {
					t.Errorf("%s revision #1 SectionConfig = %q, want empty", c.name, v)
				}
				warned := false
				for _, w := range result.Warnings {
					if strings.Contains(w, c.name) && strings.Contains(w, "without its templates") {
						warned = true
					}
				}
				if !warned {
					t.Errorf("%s was imported without a stripped-templates warning; warnings = %v", c.name, result.Warnings)
				}
			}
		})
	}
}

// (4) Begin() inherits the derived context. apply_import.go opens its batch
// transactions with s.ctx.db.Begin() rather than WithTransaction, so if Begin
// dropped the scope filter or the acting user, every batched write would run
// unscoped and unattributed.
func TestScopedHandle_BeginInheritsScopeAndActor(t *testing.T) {
	ctx := createFacadeContext(t, "facade_begin")

	root := &models.Group{Name: "begin-root"}
	mustCreateFacade(t, ctx, root)
	outside := &models.Group{Name: "begin-outside"}
	mustCreateFacade(t, ctx, outside)

	guest := scopedGuest(root.ID)
	scoped := ctx.WithPrincipal(guest)

	// Control: the derived handle itself carries both values.
	if sf := scopeFromContext(scoped.db.Statement.Context); sf == nil {
		t.Fatal("control invalid: the principal-derived handle carries no scope filter")
	}
	if id, ok := actingUserFromContext(scoped.db.Statement.Context); !ok || id != guest.UserID {
		t.Fatalf("control invalid: derived handle acting user = (%d,%v), want %d", id, ok, guest.UserID)
	}

	tx := scoped.db.Begin()
	if tx.Error != nil {
		t.Fatalf("Begin: %v", tx.Error)
	}
	defer tx.Rollback()

	sf := scopeFromContext(tx.Statement.Context)
	if sf == nil {
		t.Fatal("Begin() dropped the scope filter; batched import writes would run unscoped")
	}
	if len(sf.allowed) == 0 || sf.allowed[0] != root.ID {
		t.Errorf("Begin() carried allow-list %v, want it to contain the scope root %d", sf.allowed, root.ID)
	}
	if id, ok := actingUserFromContext(tx.Statement.Context); !ok || id != guest.UserID {
		t.Errorf("Begin() carried acting user (%d,%v), want %d; batched writes would not be attributed",
			id, ok, guest.UserID)
	}

	// Behavioural confirmation: the callbacks actually fire on the tx handle.
	var groups []models.Group
	if err := tx.Find(&groups).Error; err != nil {
		t.Fatalf("query through tx: %v", err)
	}
	for _, g := range groups {
		if g.ID == outside.ID {
			t.Errorf("a query through the transaction returned out-of-subtree group %d; the scope "+
				"callback did not fire on the Begin()-derived handle", outside.ID)
		}
	}
	if len(groups) != 1 {
		t.Errorf("query through tx returned %d groups, want 1 (the scope root)", len(groups))
	}
}
