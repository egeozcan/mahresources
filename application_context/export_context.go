package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"time"

	"gorm.io/gorm"
	"mahresources/archive"
	"mahresources/models"
	"mahresources/models/types"
)

// ExportRequest is the input to EstimateExport / StreamExport. Comes from
// either the HTTP API (decoded from JSON) or the CLI command.
type ExportRequest struct {
	RootGroupIDs []uint                   `json:"rootGroupIds"`
	Scope        archive.ExportScope      `json:"scope"`
	Fidelity     archive.ExportFidelity   `json:"fidelity"`
	SchemaDefs   archive.ExportSchemaDefs `json:"schemaDefs"`
	Gzip         bool                     `json:"gzip"`
}

// ExportEstimate is the result of EstimateExport. Cheap to compute — no
// blob bytes are read, no tar is written.
type ExportEstimate struct {
	Counts         archive.Counts `json:"counts"`
	UniqueBlobs    int            `json:"uniqueBlobs"`
	EstimatedBytes int64          `json:"estimatedBytes"`
	DanglingByKind map[string]int `json:"danglingByKind"`
}

// EstimateExport walks the requested scope and returns counts without
// touching file bytes. Used by /v1/groups/export/estimate.
func (ctx *MahresourcesContext) EstimateExport(req *ExportRequest) (*ExportEstimate, error) {
	if len(req.RootGroupIDs) == 0 {
		return nil, fmt.Errorf("export: at least one root group required")
	}

	plan, err := ctx.buildExportPlan(req)
	if err != nil {
		return nil, err
	}

	est := &ExportEstimate{
		Counts: archive.Counts{
			Groups:    len(plan.groupIDs),
			Notes:     len(plan.noteIDs),
			Resources: len(plan.resourceIDs),
			Series:    len(plan.seriesIDs),
		},
		UniqueBlobs:    len(plan.uniqueHashes),
		EstimatedBytes: plan.totalBytes,
		DanglingByKind: countDanglingByKind(plan.dangling),
	}
	return est, nil
}

func countDanglingByKind(refs []archive.DanglingRef) map[string]int {
	out := map[string]int{}
	for _, r := range refs {
		out[r.Kind]++
	}
	return out
}

// exportPlan is the internal planning struct produced by buildExportPlan.
type exportPlan struct {
	req *ExportRequest

	groupIDs    []uint
	noteIDs     []uint
	resourceIDs []uint
	seriesIDs   []uint

	groupExportID    map[uint]string
	noteExportID     map[uint]string
	resourceExportID map[uint]string
	seriesExportID   map[uint]string

	categoryExportID         map[uint]string
	noteTypeExportID         map[uint]string
	resourceCategoryExportID map[uint]string
	tagExportID              map[uint]string
	grtExportID              map[uint]string

	dangling     []archive.DanglingRef
	danglingNext int

	uniqueHashes map[string]bool
	totalBytes   int64

	warnings []string
	// missingBlobs is the set of hashes whose backing file was not found during
	// the pre-scan phase. writeResourceBlob consults this to skip re-attempts.
	missingBlobs map[string]bool
}

// buildExportPlan walks the DB starting from req.RootGroupIDs, collecting all
// in-scope entities, and assigns deterministic export IDs (g0001, r0042, etc.)
// in insertion order. Task 9 extends it with dangling reference detection.
func (ctx *MahresourcesContext) buildExportPlan(req *ExportRequest) (*exportPlan, error) {
	plan := &exportPlan{
		req:                      req,
		groupExportID:            map[uint]string{},
		noteExportID:             map[uint]string{},
		resourceExportID:         map[uint]string{},
		seriesExportID:           map[uint]string{},
		categoryExportID:         map[uint]string{},
		noteTypeExportID:         map[uint]string{},
		resourceCategoryExportID: map[uint]string{},
		tagExportID:              map[uint]string{},
		grtExportID:              map[uint]string{},
		uniqueHashes:             map[string]bool{},
	}

	// Phase A: collect group IDs in scope.
	groupSet := map[uint]bool{}
	for _, rootID := range req.RootGroupIDs {
		if req.Scope.Subtree {
			ids, err := ctx.collectSubtreeGroupIDs(rootID)
			if err != nil {
				return nil, fmt.Errorf("collectSubtreeGroupIDs(%d): %w", rootID, err)
			}
			for _, id := range ids {
				groupSet[id] = true
			}
		} else {
			groupSet[rootID] = true
		}
	}
	for id := range groupSet {
		plan.groupIDs = append(plan.groupIDs, id)
	}
	sortAscUint(plan.groupIDs)
	for _, id := range plan.groupIDs {
		plan.groupExportID[id] = fmt.Sprintf("g%04d", len(plan.groupExportID)+1)
	}

	// Phase B: collect resources owned by in-scope groups.
	if req.Scope.OwnedResources {
		resources, err := ctx.findResourcesByOwner(plan.groupIDs)
		if err != nil {
			return nil, err
		}
		for _, r := range resources {
			plan.resourceIDs = append(plan.resourceIDs, r.ID)
			plan.resourceExportID[r.ID] = fmt.Sprintf("r%04d", len(plan.resourceExportID)+1)
			if r.Hash != "" && !plan.uniqueHashes[r.Hash] {
				plan.uniqueHashes[r.Hash] = true
				plan.totalBytes += r.FileSize
			}
		}
	}

	// Phase C: collect notes owned by in-scope groups.
	if req.Scope.OwnedNotes {
		notes, err := ctx.findNotesByOwner(plan.groupIDs)
		if err != nil {
			return nil, err
		}
		for _, n := range notes {
			plan.noteIDs = append(plan.noteIDs, n.ID)
			plan.noteExportID[n.ID] = fmt.Sprintf("n%04d", len(plan.noteExportID)+1)
		}
	}

	// Phase D: collect series referenced by in-scope resources.
	if req.Fidelity.ResourceSeries && len(plan.resourceIDs) > 0 {
		seriesIDs, err := ctx.findSeriesForResources(plan.resourceIDs)
		if err != nil {
			return nil, err
		}
		for _, sid := range seriesIDs {
			plan.seriesIDs = append(plan.seriesIDs, sid)
			plan.seriesExportID[sid] = fmt.Sprintf("s%04d", len(plan.seriesExportID)+1)
		}
	}

	// Phase E: detect dangling references (m2m / GroupRelations / Series siblings).
	if err := ctx.collectDanglingRefs(plan); err != nil {
		return nil, err
	}

	return plan, nil
}

func (ctx *MahresourcesContext) findResourcesByOwner(groupIDs []uint) ([]models.Resource, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	var resources []models.Resource
	if err := ctx.db.Where("owner_id IN ?", groupIDs).Order("id").Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}

func (ctx *MahresourcesContext) findNotesByOwner(groupIDs []uint) ([]models.Note, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}
	var notes []models.Note
	if err := ctx.db.Where("owner_id IN ?", groupIDs).Order("id").Find(&notes).Error; err != nil {
		return nil, err
	}
	return notes, nil
}

func (ctx *MahresourcesContext) findSeriesForResources(resourceIDs []uint) ([]uint, error) {
	if len(resourceIDs) == 0 {
		return nil, nil
	}
	var ids []uint
	if err := ctx.db.Model(&models.Resource{}).
		Where("id IN ? AND series_id IS NOT NULL", resourceIDs).
		Distinct("series_id").
		Pluck("series_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

// sortAscUint sorts a uint slice ascending in-place (insertion sort — fine for
// the small slices we generate during plan-building).
func sortAscUint(s []uint) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// collectDanglingRefs populates plan.dangling with cross-subtree edge references.
// It detects three categories:
//
//  1. m2m RelatedGroups / RelatedResources / RelatedNotes on in-scope groups
//     that point outside the export scope (only when Scope.RelatedM2M is set).
//
//  2. Typed GroupRelation rows whose ToGroup is outside scope
//     (only when Scope.GroupRelations is set).
//
//  3. Resources that share a series_id with in-scope resources but are
//     themselves not in scope (only when Fidelity.ResourceSeries is set and
//     there are in-scope series).
func (ctx *MahresourcesContext) collectDanglingRefs(plan *exportPlan) error {
	req := plan.req

	// Build fast-lookup sets.
	resourceSet := make(map[uint]bool, len(plan.resourceIDs))
	for _, id := range plan.resourceIDs {
		resourceSet[id] = true
	}
	noteSet := make(map[uint]bool, len(plan.noteIDs))
	for _, id := range plan.noteIDs {
		noteSet[id] = true
	}

	// --- Sub-detection 1: m2m RelatedGroups / RelatedResources / RelatedNotes ---
	if req.Scope.RelatedM2M && len(plan.groupIDs) > 0 {
		var groups []models.Group
		if err := ctx.db.
			Preload("RelatedGroups").
			Preload("RelatedResources").
			Preload("RelatedNotes").
			Where("id IN ?", plan.groupIDs).
			Order("id").
			Find(&groups).Error; err != nil {
			return fmt.Errorf("preload m2m for dangling detection: %w", err)
		}

		for _, g := range groups {
			fromRef := plan.groupExportID[g.ID]

			for _, rg := range g.RelatedGroups {
				if !plan.isGroupInScope(rg.ID) {
					plan.appendDangling(archive.DanglingRef{
						Kind: archive.DanglingKindRelatedGroup,
						From: fromRef,
						ToStub: archive.DanglingStub{
							SourceID: rg.ID,
							Name:     rg.Name,
							Reason:   "out_of_scope",
						},
					})
				}
			}

			for _, rr := range g.RelatedResources {
				if !resourceSet[rr.ID] {
					plan.appendDangling(archive.DanglingRef{
						Kind: archive.DanglingKindRelatedResource,
						From: fromRef,
						ToStub: archive.DanglingStub{
							SourceID: rr.ID,
							Name:     rr.Name,
							Reason:   "out_of_scope",
						},
					})
				}
			}

			for _, rn := range g.RelatedNotes {
				if !noteSet[rn.ID] {
					plan.appendDangling(archive.DanglingRef{
						Kind: archive.DanglingKindRelatedNote,
						From: fromRef,
						ToStub: archive.DanglingStub{
							SourceID: rn.ID,
							Name:     rn.Name,
							Reason:   "out_of_scope",
						},
					})
				}
			}
		}
	}

	// --- Sub-detection 2: Typed GroupRelations ---
	if req.Scope.GroupRelations && len(plan.groupIDs) > 0 {
		var relations []models.GroupRelation
		if err := ctx.db.
			Preload("ToGroup").
			Preload("RelationType").
			Where("from_group_id IN ?", plan.groupIDs).
			Order("id").
			Find(&relations).Error; err != nil {
			return fmt.Errorf("load GroupRelations for dangling detection: %w", err)
		}

		for _, rel := range relations {
			if rel.ToGroupId == nil || rel.ToGroup == nil {
				continue
			}
			if !plan.isGroupInScope(*rel.ToGroupId) {
				typeName := ""
				if rel.RelationType != nil {
					typeName = rel.RelationType.Name
				}
				fromRef := ""
				if rel.FromGroupId != nil {
					fromRef = plan.groupExportID[*rel.FromGroupId]
				}
				plan.appendDangling(archive.DanglingRef{
					Kind:             archive.DanglingKindGroupRelation,
					From:             fromRef,
					RelationTypeName: typeName,
					ToStub: archive.DanglingStub{
						SourceID: rel.ToGroup.ID,
						Name:     rel.ToGroup.Name,
						Reason:   "out_of_scope",
					},
				})
			}
		}
	}

	// --- Sub-detection 3: Series siblings ---
	if req.Fidelity.ResourceSeries && len(plan.seriesIDs) > 0 {
		var sibs []models.Resource
		if err := ctx.db.
			Where("series_id IN ? AND id NOT IN ?", plan.seriesIDs, plan.resourceIDs).
			Order("id").
			Find(&sibs).Error; err != nil {
			return fmt.Errorf("load series siblings for dangling detection: %w", err)
		}

		// Build a map series_id -> first in-scope resource export ID by querying
		// all in-scope resources that have a series_id in a single batch.
		seriesFromRef := make(map[uint]string)
		if len(plan.resourceIDs) > 0 {
			type idSeriesRow struct {
				ID       uint
				SeriesID *uint
			}
			var rows []idSeriesRow
			if err := ctx.db.Model(&models.Resource{}).
				Select("id, series_id").
				Where("id IN ? AND series_id IS NOT NULL", plan.resourceIDs).
				Order("id").
				Scan(&rows).Error; err != nil {
				return fmt.Errorf("load resource series IDs for dangling detection: %w", err)
			}
			for _, row := range rows {
				if row.SeriesID != nil {
					if _, seen := seriesFromRef[*row.SeriesID]; !seen {
						seriesFromRef[*row.SeriesID] = plan.resourceExportID[row.ID]
					}
				}
			}
		}

		for _, sib := range sibs {
			if sib.SeriesID == nil {
				continue
			}
			fromRef := seriesFromRef[*sib.SeriesID]
			plan.appendDangling(archive.DanglingRef{
				Kind: archive.DanglingKindResourceSeriesSib,
				From: fromRef,
				ToStub: archive.DanglingStub{
					SourceID: sib.ID,
					Name:     sib.Name,
					Reason:   "out_of_scope",
				},
			})
		}
	}

	return nil
}

// isGroupInScope reports whether id is in the plan's group set.
func (p *exportPlan) isGroupInScope(id uint) bool {
	_, ok := p.groupExportID[id]
	return ok
}

// appendDangling adds a DanglingRef to the plan, assigning it a unique ID.
func (p *exportPlan) appendDangling(ref archive.DanglingRef) {
	p.danglingNext++
	ref.ID = fmt.Sprintf("dr%04d", p.danglingNext)
	p.dangling = append(p.dangling, ref)
}

// ────────────────────────────────────────────────────────────────────────────
// StreamExport public API
// ────────────────────────────────────────────────────────────────────────────

// ProgressEvent is emitted by the ReporterFn during StreamExport to let callers
// track progress. At least one of the fields will be non-zero.
type ProgressEvent struct {
	// Phase is the current high-level phase name (e.g. "groups", "resources").
	Phase string
	// PhaseCurrent / PhaseTotal are the within-phase item counters.
	PhaseCurrent int
	PhaseTotal   int
	// BytesWritten is the cumulative number of bytes written to the tar so far.
	BytesWritten int64
	// Warning is set when a non-fatal problem was encountered (e.g. missing blob).
	Warning string
}

// ReporterFn is a callback that receives live ProgressEvent values during
// StreamExport. It must be non-blocking; slow callbacks will stall the export.
type ReporterFn func(ev ProgressEvent)

// blobReadInfo carries the information needed to open and stream a resource blob.
type blobReadInfo struct {
	resourceExportID string
	hash             string
	size             int64
	location         string
	storageLocation  *string
}

// StreamExport walks the export plan built from req, writes a complete tar
// archive to dst, and sends live ProgressEvent values to report. Honors
// jobCtx cancellation.
func (ctx *MahresourcesContext) StreamExport(
	jobCtx context.Context,
	req *ExportRequest,
	dst io.Writer,
	report ReporterFn,
) error {
	if len(req.RootGroupIDs) == 0 {
		return fmt.Errorf("export: at least one root group required")
	}

	// Step 1: build the plan (walks the DB, assigns export IDs).
	plan, err := ctx.buildExportPlan(req)
	if err != nil {
		return fmt.Errorf("build export plan: %w", err)
	}

	// Step 2: collect schema-def IDs (category, note-type, resource-category, tag, grt).
	if err := ctx.collectSchemaDefIDs(plan); err != nil {
		return fmt.Errorf("collect schema def IDs: %w", err)
	}

	// Step 2b: pre-scan blobs to populate plan.warnings before writing the manifest.
	// The manifest must be written first, so warnings from missing blobs must be
	// detected here (not during streaming) so they can be included in the manifest.
	if req.Fidelity.ResourceBlobs && len(plan.resourceIDs) > 0 {
		if err := ctx.preScanBlobs(plan, report); err != nil {
			return fmt.Errorf("pre-scan blobs: %w", err)
		}
	}

	// Step 3: build the dangling-lookup helpers we'll need in loadGroupPayload.
	// key: (fromGroupID, toGroupID, rtID) → dangling.ID string
	type relKey struct{ from, to, rt uint }
	danglingByRel := map[relKey]string{}
	for _, d := range plan.dangling {
		if d.Kind == archive.DanglingKindGroupRelation {
			// We can't reconstruct the three-key tuple from a DanglingRef alone
			// because ToStub only carries SourceID (the to-group ID). We'll fall
			// back to a two-key lookup (from export ID, to source ID) below.
			_ = d
		}
	}
	// Build a fast map: from-group export ID + to-group source ID → dangling.ID
	danglingByFromTo := map[string]string{} // key = fromExportID + ":" + toSourceIDstr
	for _, d := range plan.dangling {
		if d.Kind == archive.DanglingKindGroupRelation {
			key := fmt.Sprintf("%s:%d", d.From, d.ToStub.SourceID)
			if _, exists := danglingByFromTo[key]; !exists {
				danglingByFromTo[key] = d.ID
			}
		}
	}
	_ = danglingByRel // silence unused warning (not actually used after refactor)

	// Step 4: open the archive writer.
	w, err := archive.NewWriter(dst, req.Gzip)
	if err != nil {
		return fmt.Errorf("create archive writer: %w", err)
	}

	// Write the manifest now so it is the first tar entry.
	manifest := plan.toManifest(req, ctx)
	if err := w.WriteManifest(manifest); err != nil {
		return fmt.Errorf("write manifest: %w", err)
	}
	report(ProgressEvent{Phase: "manifest", BytesWritten: w.BytesWritten()})

	// Check cancellation.
	if err := jobCtx.Err(); err != nil {
		return err
	}

	// Step 5: schema defs (categories, note types, resource categories, tags, grts).
	if req.SchemaDefs.CategoriesAndTypes {
		if err := ctx.writeCategoryDefs(w, plan); err != nil {
			return fmt.Errorf("write category defs: %w", err)
		}
		if err := ctx.writeNoteTypeDefs(w, plan); err != nil {
			return fmt.Errorf("write note type defs: %w", err)
		}
		if err := ctx.writeResourceCategoryDefs(w, plan); err != nil {
			return fmt.Errorf("write resource category defs: %w", err)
		}
	}
	if req.SchemaDefs.Tags {
		if err := ctx.writeTagDefs(w, plan); err != nil {
			return fmt.Errorf("write tag defs: %w", err)
		}
	}
	if req.SchemaDefs.GroupRelationTypes {
		if err := ctx.writeGroupRelationTypeDefs(w, plan); err != nil {
			return fmt.Errorf("write group relation type defs: %w", err)
		}
	}
	report(ProgressEvent{Phase: "schema_defs", BytesWritten: w.BytesWritten()})

	// Step 6: series.
	if len(plan.seriesIDs) > 0 {
		if err := ctx.writeSeries(w, plan); err != nil {
			return fmt.Errorf("write series: %w", err)
		}
		report(ProgressEvent{Phase: "series", PhaseTotal: len(plan.seriesIDs), PhaseCurrent: len(plan.seriesIDs), BytesWritten: w.BytesWritten()})
	}

	// Step 7: groups.
	for i, id := range plan.groupIDs {
		if err := jobCtx.Err(); err != nil {
			return err
		}
		p, err := ctx.loadGroupPayload(id, plan, danglingByFromTo)
		if err != nil {
			return fmt.Errorf("load group %d: %w", id, err)
		}
		if err := w.WriteGroup(p); err != nil {
			return fmt.Errorf("write group %d: %w", id, err)
		}
		report(ProgressEvent{Phase: "groups", PhaseCurrent: i + 1, PhaseTotal: len(plan.groupIDs), BytesWritten: w.BytesWritten()})
	}

	// Step 8: notes.
	for i, id := range plan.noteIDs {
		if err := jobCtx.Err(); err != nil {
			return err
		}
		p, err := ctx.loadNotePayload(id, plan)
		if err != nil {
			return fmt.Errorf("load note %d: %w", id, err)
		}
		if err := w.WriteNote(p); err != nil {
			return fmt.Errorf("write note %d: %w", id, err)
		}
		report(ProgressEvent{Phase: "notes", PhaseCurrent: i + 1, PhaseTotal: len(plan.noteIDs), BytesWritten: w.BytesWritten()})
	}

	// Step 9: resources (payload JSON + optional blob + optional previews).
	for i, id := range plan.resourceIDs {
		if err := jobCtx.Err(); err != nil {
			return err
		}
		p, blobInfo, err := ctx.loadResourcePayload(id, plan)
		if err != nil {
			return fmt.Errorf("load resource %d: %w", id, err)
		}
		if err := w.WriteResource(p); err != nil {
			return fmt.Errorf("write resource %d: %w", id, err)
		}
		// Write blob if requested and available.
		if req.Fidelity.ResourceBlobs && blobInfo != nil {
			if err := ctx.writeResourceBlob(w, blobInfo, plan, report); err != nil {
				return fmt.Errorf("write blob for resource %d: %w", id, err)
			}
		}
		// Write previews if requested.
		if req.Fidelity.ResourcePreviews && p != nil {
			for _, prev := range p.Previews {
				// Load preview bytes from DB.
				var dbPreview models.Preview
				if err := ctx.db.Where("resource_id = ? AND width = ? AND height = ?", id, prev.Width, prev.Height).First(&dbPreview).Error; err == nil && len(dbPreview.Data) > 0 {
					if err := w.WritePreview(prev.PreviewExportID, dbPreview.Data); err != nil {
						return fmt.Errorf("write preview %s: %w", prev.PreviewExportID, err)
					}
				}
			}
		}
		report(ProgressEvent{Phase: "resources", PhaseCurrent: i + 1, PhaseTotal: len(plan.resourceIDs), BytesWritten: w.BytesWritten()})
	}

	// Step 10: finalize.
	if err := w.Close(); err != nil {
		return fmt.Errorf("close archive writer: %w", err)
	}

	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// collectSchemaDefIDs
// ────────────────────────────────────────────────────────────────────────────

// collectSchemaDefIDs performs SELECT-only queries to discover which schema
// definition rows are referenced by in-scope entities, and assigns them export
// IDs in the plan maps.
func (ctx *MahresourcesContext) collectSchemaDefIDs(plan *exportPlan) error {
	req := plan.req

	// Categories (from groups).
	if req.SchemaDefs.CategoriesAndTypes && len(plan.groupIDs) > 0 {
		var catIDs []uint
		if err := ctx.db.Model(&models.Group{}).
			Where("id IN ? AND category_id IS NOT NULL", plan.groupIDs).
			Distinct("category_id").
			Pluck("category_id", &catIDs).Error; err != nil {
			return fmt.Errorf("pluck category IDs: %w", err)
		}
		sortAscUint(catIDs)
		for _, id := range catIDs {
			if _, ok := plan.categoryExportID[id]; !ok {
				plan.categoryExportID[id] = fmt.Sprintf("c%04d", len(plan.categoryExportID)+1)
			}
		}
	}

	// NoteTypes (from notes).
	if req.SchemaDefs.CategoriesAndTypes && len(plan.noteIDs) > 0 {
		var ntIDs []uint
		if err := ctx.db.Model(&models.Note{}).
			Where("id IN ? AND note_type_id IS NOT NULL", plan.noteIDs).
			Distinct("note_type_id").
			Pluck("note_type_id", &ntIDs).Error; err != nil {
			return fmt.Errorf("pluck note type IDs: %w", err)
		}
		sortAscUint(ntIDs)
		for _, id := range ntIDs {
			if _, ok := plan.noteTypeExportID[id]; !ok {
				plan.noteTypeExportID[id] = fmt.Sprintf("nt%04d", len(plan.noteTypeExportID)+1)
			}
		}
	}

	// ResourceCategories (from resources).
	if req.SchemaDefs.CategoriesAndTypes && len(plan.resourceIDs) > 0 {
		var rcIDs []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("id IN ?", plan.resourceIDs).
			Distinct("resource_category_id").
			Pluck("resource_category_id", &rcIDs).Error; err != nil {
			return fmt.Errorf("pluck resource category IDs: %w", err)
		}
		sortAscUint(rcIDs)
		for _, id := range rcIDs {
			if id == 0 {
				continue
			}
			if _, ok := plan.resourceCategoryExportID[id]; !ok {
				plan.resourceCategoryExportID[id] = fmt.Sprintf("rc%04d", len(plan.resourceCategoryExportID)+1)
			}
		}
	}

	// Tags (from groups, notes, resources via join tables).
	if req.SchemaDefs.Tags {
		tagSet := map[uint]bool{}

		if len(plan.groupIDs) > 0 {
			var ids []uint
			if err := ctx.db.Table("group_tags").
				Where("group_id IN ?", plan.groupIDs).
				Distinct("tag_id").
				Pluck("tag_id", &ids).Error; err != nil {
				return fmt.Errorf("pluck group tag IDs: %w", err)
			}
			for _, id := range ids {
				tagSet[id] = true
			}
		}
		if len(plan.noteIDs) > 0 {
			var ids []uint
			if err := ctx.db.Table("note_tags").
				Where("note_id IN ?", plan.noteIDs).
				Distinct("tag_id").
				Pluck("tag_id", &ids).Error; err != nil {
				return fmt.Errorf("pluck note tag IDs: %w", err)
			}
			for _, id := range ids {
				tagSet[id] = true
			}
		}
		if len(plan.resourceIDs) > 0 {
			var ids []uint
			if err := ctx.db.Table("resource_tags").
				Where("resource_id IN ?", plan.resourceIDs).
				Distinct("tag_id").
				Pluck("tag_id", &ids).Error; err != nil {
				return fmt.Errorf("pluck resource tag IDs: %w", err)
			}
			for _, id := range ids {
				tagSet[id] = true
			}
		}
		tagIDs := keysOfUintBoolMap(tagSet)
		sortAscUint(tagIDs)
		for _, id := range tagIDs {
			if _, ok := plan.tagExportID[id]; !ok {
				plan.tagExportID[id] = fmt.Sprintf("t%04d", len(plan.tagExportID)+1)
			}
		}
	}

	// GroupRelationTypes (from GroupRelation rows with from-group in scope).
	if req.SchemaDefs.GroupRelationTypes && len(plan.groupIDs) > 0 {
		var rtIDs []uint
		if err := ctx.db.Model(&models.GroupRelation{}).
			Where("from_group_id IN ? AND relation_type_id IS NOT NULL", plan.groupIDs).
			Distinct("relation_type_id").
			Pluck("relation_type_id", &rtIDs).Error; err != nil {
			return fmt.Errorf("pluck group relation type IDs: %w", err)
		}
		sortAscUint(rtIDs)
		for _, id := range rtIDs {
			if _, ok := plan.grtExportID[id]; !ok {
				plan.grtExportID[id] = fmt.Sprintf("grt%04d", len(plan.grtExportID)+1)
			}
		}
	}

	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// preScanBlobs
// ────────────────────────────────────────────────────────────────────────────

// preScanBlobs checks whether each unique blob hash in the plan can actually
// be opened. Missing blobs are recorded in plan.warnings (and emitted via
// report) so they appear in the manifest — which must be written before any
// blob streaming starts.
func (ctx *MahresourcesContext) preScanBlobs(plan *exportPlan, report ReporterFn) error {
	if plan.missingBlobs == nil {
		plan.missingBlobs = map[string]bool{}
	}

	type row struct {
		ID              uint
		Hash            string
		Location        string
		StorageLocation *string
	}
	var rows []row
	if err := ctx.db.Model(&models.Resource{}).
		Select("id, hash, location, storage_location").
		Where("id IN ? AND hash != ''", plan.resourceIDs).
		Scan(&rows).Error; err != nil {
		return err
	}

	checked := map[string]bool{} // deduplicate by hash
	for _, r := range rows {
		if r.Hash == "" || checked[r.Hash] {
			continue
		}
		checked[r.Hash] = true

		fs, err := ctx.GetFsForStorageLocation(r.StorageLocation)
		if err != nil {
			msg := fmt.Sprintf("blob missing for resource %d (%s): storage location error: %v", r.ID, r.Hash, err)
			plan.warnings = append(plan.warnings, msg)
			plan.missingBlobs[r.Hash] = true
			report(ProgressEvent{Warning: msg})
			continue
		}
		cleanLoc := models.Resource{Location: r.Location}.GetCleanLocation()
		if _, statErr := fs.Stat(cleanLoc); statErr != nil {
			msg := fmt.Sprintf("blob missing for resource %d (%s): %v", r.ID, r.Hash, statErr)
			plan.warnings = append(plan.warnings, msg)
			plan.missingBlobs[r.Hash] = true
			report(ProgressEvent{Warning: msg})
		}
	}
	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// Schema-def writers
// ────────────────────────────────────────────────────────────────────────────

func (ctx *MahresourcesContext) writeCategoryDefs(w *archive.Writer, plan *exportPlan) error {
	ids := keysOfUintMap(plan.categoryExportID)
	if len(ids) == 0 {
		return w.WriteCategoryDefs([]archive.CategoryDef{})
	}
	var rows []models.Category
	if err := ctx.db.Where("id IN ?", ids).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	defs := make([]archive.CategoryDef, 0, len(rows))
	for _, row := range rows {
		defs = append(defs, archive.CategoryDef{
			ExportID:         plan.categoryExportID[row.ID],
			SourceID:         row.ID,
			Name:             row.Name,
			Description:      row.Description,
			CustomHeader:     row.CustomHeader,
			CustomSidebar:    row.CustomSidebar,
			CustomSummary:    row.CustomSummary,
			CustomAvatar:     row.CustomAvatar,
			CustomMRQLResult: row.CustomMRQLResult,
			MetaSchema:       row.MetaSchema,
			SectionConfig:    jsonToMap(row.SectionConfig),
		})
	}
	return w.WriteCategoryDefs(defs)
}

func (ctx *MahresourcesContext) writeNoteTypeDefs(w *archive.Writer, plan *exportPlan) error {
	ids := keysOfUintMap(plan.noteTypeExportID)
	if len(ids) == 0 {
		return w.WriteNoteTypeDefs([]archive.NoteTypeDef{})
	}
	var rows []models.NoteType
	if err := ctx.db.Where("id IN ?", ids).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	defs := make([]archive.NoteTypeDef, 0, len(rows))
	for _, row := range rows {
		defs = append(defs, archive.NoteTypeDef{
			ExportID:         plan.noteTypeExportID[row.ID],
			SourceID:         row.ID,
			Name:             row.Name,
			Description:      row.Description,
			CustomHeader:     row.CustomHeader,
			CustomSidebar:    row.CustomSidebar,
			CustomSummary:    row.CustomSummary,
			CustomAvatar:     row.CustomAvatar,
			CustomMRQLResult: row.CustomMRQLResult,
			MetaSchema:       row.MetaSchema,
			SectionConfig:    jsonToMap(row.SectionConfig),
		})
	}
	return w.WriteNoteTypeDefs(defs)
}

func (ctx *MahresourcesContext) writeResourceCategoryDefs(w *archive.Writer, plan *exportPlan) error {
	ids := keysOfUintMap(plan.resourceCategoryExportID)
	if len(ids) == 0 {
		return w.WriteResourceCategoryDefs([]archive.ResourceCategoryDef{})
	}
	var rows []models.ResourceCategory
	if err := ctx.db.Where("id IN ?", ids).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	defs := make([]archive.ResourceCategoryDef, 0, len(rows))
	for _, row := range rows {
		defs = append(defs, archive.ResourceCategoryDef{
			CategoryDef: archive.CategoryDef{
				ExportID:         plan.resourceCategoryExportID[row.ID],
				SourceID:         row.ID,
				Name:             row.Name,
				Description:      row.Description,
				CustomHeader:     row.CustomHeader,
				CustomSidebar:    row.CustomSidebar,
				CustomSummary:    row.CustomSummary,
				CustomAvatar:     row.CustomAvatar,
				CustomMRQLResult: row.CustomMRQLResult,
				MetaSchema:       row.MetaSchema,
				SectionConfig:    jsonToMap(row.SectionConfig),
			},
			AutoDetectRules: row.AutoDetectRules,
		})
	}
	return w.WriteResourceCategoryDefs(defs)
}

func (ctx *MahresourcesContext) writeTagDefs(w *archive.Writer, plan *exportPlan) error {
	ids := keysOfUintMap(plan.tagExportID)
	if len(ids) == 0 {
		return w.WriteTagDefs([]archive.TagDef{})
	}
	var rows []models.Tag
	if err := ctx.db.Where("id IN ?", ids).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	defs := make([]archive.TagDef, 0, len(rows))
	for _, row := range rows {
		defs = append(defs, archive.TagDef{
			ExportID:    plan.tagExportID[row.ID],
			SourceID:    row.ID,
			Name:        row.Name,
			Description: row.Description,
			Meta:        jsonToMap(row.Meta),
		})
	}
	return w.WriteTagDefs(defs)
}

func (ctx *MahresourcesContext) writeGroupRelationTypeDefs(w *archive.Writer, plan *exportPlan) error {
	ids := keysOfUintMap(plan.grtExportID)
	if len(ids) == 0 {
		return w.WriteGroupRelationTypeDefs([]archive.GroupRelationTypeDef{})
	}
	var rows []models.GroupRelationType
	if err := ctx.db.
		Preload("FromCategory").
		Preload("ToCategory").
		Where("id IN ?", ids).
		Order("id").
		Find(&rows).Error; err != nil {
		return err
	}
	defs := make([]archive.GroupRelationTypeDef, 0, len(rows))
	for _, row := range rows {
		def := archive.GroupRelationTypeDef{
			ExportID:    plan.grtExportID[row.ID],
			SourceID:    row.ID,
			Name:        row.Name,
			Description: row.Description,
		}
		if row.FromCategory != nil {
			def.FromCategoryName = row.FromCategory.Name
			if ref, ok := plan.categoryExportID[row.FromCategory.ID]; ok {
				def.FromCategoryRef = ref
			}
		}
		if row.ToCategory != nil {
			def.ToCategoryName = row.ToCategory.Name
			if ref, ok := plan.categoryExportID[row.ToCategory.ID]; ok {
				def.ToCategoryRef = ref
			}
		}
		if row.BackRelationId != nil {
			if ref, ok := plan.grtExportID[*row.BackRelationId]; ok {
				def.BackRelationRef = ref
			}
		}
		defs = append(defs, def)
	}
	return w.WriteGroupRelationTypeDefs(defs)
}

// ────────────────────────────────────────────────────────────────────────────
// writeSeries
// ────────────────────────────────────────────────────────────────────────────

func (ctx *MahresourcesContext) writeSeries(w *archive.Writer, plan *exportPlan) error {
	if len(plan.seriesIDs) == 0 {
		return nil
	}
	var rows []models.Series
	if err := ctx.db.Where("id IN ?", plan.seriesIDs).Order("id").Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		p := &archive.SeriesPayload{
			ExportID: plan.seriesExportID[row.ID],
			SourceID: row.ID,
			Name:     row.Name,
			Slug:     row.Slug,
			Meta:     jsonToMap(row.Meta),
		}
		if err := w.WriteSeries(p); err != nil {
			return err
		}
	}
	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// loadGroupPayload
// ────────────────────────────────────────────────────────────────────────────

func (ctx *MahresourcesContext) loadGroupPayload(
	id uint,
	plan *exportPlan,
	danglingByFromTo map[string]string,
) (*archive.GroupPayload, error) {
	var g models.Group
	if err := ctx.db.
		Preload("Tags").
		Preload("Category").
		Preload("RelatedGroups").
		Preload("RelatedResources").
		Preload("RelatedNotes").
		Preload("Relationships.RelationType").
		Preload("Relationships.ToGroup").
		First(&g, id).Error; err != nil {
		return nil, err
	}

	p := &archive.GroupPayload{
		ExportID:         plan.groupExportID[g.ID],
		SourceID:         g.ID,
		Name:             g.Name,
		Description:      g.Description,
		Meta:             jsonToMap(g.Meta),
		CreatedAt:        g.CreatedAt,
		UpdatedAt:        g.UpdatedAt,
		Tags:             make([]archive.TagRef, 0, len(g.Tags)),
		RelatedGroups:    []string{},
		RelatedResources: []string{},
		RelatedNotes:     []string{},
		Relationships:    []archive.GroupRelationPayload{},
	}

	if g.URL != nil {
		u := url.URL(*g.URL)
		p.URL = u.String()
	}
	if g.OwnerId != nil {
		if ref, ok := plan.groupExportID[*g.OwnerId]; ok {
			p.OwnerRef = ref
		}
	}
	if g.Category != nil {
		p.CategoryName = g.Category.Name
		if ref, ok := plan.categoryExportID[g.Category.ID]; ok {
			p.CategoryRef = ref
		}
	}

	for _, tag := range g.Tags {
		tr := archive.TagRef{Name: tag.Name}
		if ref, ok := plan.tagExportID[tag.ID]; ok {
			tr.Ref = ref
		}
		p.Tags = append(p.Tags, tr)
	}

	for _, rg := range g.RelatedGroups {
		if ref, ok := plan.groupExportID[rg.ID]; ok {
			p.RelatedGroups = append(p.RelatedGroups, ref)
		}
		// out-of-scope related groups are tracked as dangling refs, not listed here
	}
	for _, rr := range g.RelatedResources {
		if ref, ok := plan.resourceExportID[rr.ID]; ok {
			p.RelatedResources = append(p.RelatedResources, ref)
		}
	}
	for _, rn := range g.RelatedNotes {
		if ref, ok := plan.noteExportID[rn.ID]; ok {
			p.RelatedNotes = append(p.RelatedNotes, ref)
		}
	}

	// Typed GroupRelations.
	for _, rel := range g.Relationships {
		grp := archive.GroupRelationPayload{
			Name:        rel.Name,
			Description: rel.Description,
		}
		if rel.RelationType != nil {
			grp.TypeName = rel.RelationType.Name
			if ref, ok := plan.grtExportID[rel.RelationType.ID]; ok {
				grp.TypeRef = ref
			}
		}
		if rel.ToGroupId != nil {
			if toRef, ok := plan.groupExportID[*rel.ToGroupId]; ok {
				grp.ToRef = toRef
			} else {
				// Out of scope — look up the dangling ref ID.
				fromRef := plan.groupExportID[g.ID]
				key := fmt.Sprintf("%s:%d", fromRef, *rel.ToGroupId)
				if dID, ok := danglingByFromTo[key]; ok {
					grp.DanglingRef = dID
				}
			}
		}
		p.Relationships = append(p.Relationships, grp)
	}

	return p, nil
}

// ────────────────────────────────────────────────────────────────────────────
// loadNotePayload
// ────────────────────────────────────────────────────────────────────────────

func (ctx *MahresourcesContext) loadNotePayload(id uint, plan *exportPlan) (*archive.NotePayload, error) {
	var n models.Note
	if err := ctx.db.
		Preload("Tags").
		Preload("Resources").
		Preload("Groups").
		Preload("NoteType").
		Preload("Blocks", func(db *gorm.DB) *gorm.DB {
			return db.Order("position ASC")
		}).
		First(&n, id).Error; err != nil {
		return nil, err
	}

	p := &archive.NotePayload{
		ExportID:    plan.noteExportID[n.ID],
		SourceID:    n.ID,
		Name:        n.Name,
		Description: n.Description,
		Meta:        jsonToMap(n.Meta),
		StartDate:   n.StartDate,
		EndDate:     n.EndDate,
		CreatedAt:   n.CreatedAt,
		UpdatedAt:   n.UpdatedAt,
		Tags:        make([]archive.TagRef, 0, len(n.Tags)),
		Resources:   []string{},
		Groups:      []string{},
		Blocks:      []archive.NoteBlockPayload{},
	}

	if n.OwnerId != nil {
		if ref, ok := plan.groupExportID[*n.OwnerId]; ok {
			p.OwnerRef = ref
		}
	}
	if n.NoteType != nil {
		p.NoteTypeName = n.NoteType.Name
		if ref, ok := plan.noteTypeExportID[n.NoteType.ID]; ok {
			p.NoteTypeRef = ref
		}
	}

	for _, tag := range n.Tags {
		tr := archive.TagRef{Name: tag.Name}
		if ref, ok := plan.tagExportID[tag.ID]; ok {
			tr.Ref = ref
		}
		p.Tags = append(p.Tags, tr)
	}
	for _, r := range n.Resources {
		if ref, ok := plan.resourceExportID[r.ID]; ok {
			p.Resources = append(p.Resources, ref)
		}
	}
	for _, g := range n.Groups {
		if ref, ok := plan.groupExportID[g.ID]; ok {
			p.Groups = append(p.Groups, ref)
		}
	}
	for _, b := range n.Blocks {
		p.Blocks = append(p.Blocks, archive.NoteBlockPayload{
			Type:     b.Type,
			Position: b.Position,
			Content:  jsonToMap(b.Content),
			State:    jsonToMap(b.State),
		})
	}

	return p, nil
}

// ────────────────────────────────────────────────────────────────────────────
// loadResourcePayload
// ────────────────────────────────────────────────────────────────────────────

func (ctx *MahresourcesContext) loadResourcePayload(
	id uint,
	plan *exportPlan,
) (*archive.ResourcePayload, *blobReadInfo, error) {
	var r models.Resource
	if err := ctx.db.
		Preload("Tags").
		Preload("Notes").
		Preload("Groups").
		Preload("ResourceCategory").
		Preload("Series").
		Preload("CurrentVersion").
		Preload("Versions").
		Preload("Previews").
		First(&r, id).Error; err != nil {
		return nil, nil, err
	}

	exportID := plan.resourceExportID[r.ID]

	p := &archive.ResourcePayload{
		ExportID:         exportID,
		SourceID:         r.ID,
		Name:             r.Name,
		OriginalName:     r.OriginalName,
		OriginalLocation: r.OriginalLocation,
		Hash:             r.Hash,
		HashType:         r.HashType,
		FileSize:         r.FileSize,
		ContentType:      r.ContentType,
		ContentCategory:  r.ContentCategory,
		Width:            r.Width,
		Height:           r.Height,
		Description:      r.Description,
		Category:         r.Category,
		Meta:             jsonToMap(r.Meta),
		OwnMeta:          jsonToMap(r.OwnMeta),
		CreatedAt:        r.CreatedAt,
		UpdatedAt:        r.UpdatedAt,
		Tags:             make([]archive.TagRef, 0, len(r.Tags)),
		Groups:           []string{},
		Notes:            []string{},
		Versions:         []archive.ResourceVersionPayload{},
		Previews:         []archive.PreviewPayload{},
	}

	if r.OwnerId != nil {
		if ref, ok := plan.groupExportID[*r.OwnerId]; ok {
			p.OwnerRef = ref
		}
	}
	if r.ResourceCategory != nil {
		p.ResourceCategoryName = r.ResourceCategory.Name
		if ref, ok := plan.resourceCategoryExportID[r.ResourceCategory.ID]; ok {
			p.ResourceCategoryRef = ref
		}
	}
	if r.Series != nil {
		if ref, ok := plan.seriesExportID[r.Series.ID]; ok {
			p.SeriesRef = ref
		}
	}
	if r.CurrentVersionID != nil && r.CurrentVersion != nil {
		p.CurrentVersionRef = fmt.Sprintf("v%04d", r.CurrentVersion.VersionNumber)
	}

	for _, tag := range r.Tags {
		tr := archive.TagRef{Name: tag.Name}
		if ref, ok := plan.tagExportID[tag.ID]; ok {
			tr.Ref = ref
		}
		p.Tags = append(p.Tags, tr)
	}
	for _, g := range r.Groups {
		if ref, ok := plan.groupExportID[g.ID]; ok {
			p.Groups = append(p.Groups, ref)
		}
	}
	for _, n := range r.Notes {
		if ref, ok := plan.noteExportID[n.ID]; ok {
			p.Notes = append(p.Notes, ref)
		}
	}

	// Versions (only when fidelity flag is set or we have a current version ref).
	for i, v := range r.Versions {
		vp := archive.ResourceVersionPayload{
			VersionExportID: fmt.Sprintf("v%04d", v.VersionNumber),
			VersionNumber:   v.VersionNumber,
			Hash:            v.Hash,
			HashType:        v.HashType,
			FileSize:        v.FileSize,
			ContentType:     v.ContentType,
			Width:           v.Width,
			Height:          v.Height,
			Comment:         v.Comment,
			CreatedAt:       v.CreatedAt,
		}
		if plan.req.Fidelity.ResourceVersions {
			vp.BlobRef = v.Hash
		}
		p.Versions = append(p.Versions, vp)
		_ = i
	}

	// Previews.
	for i, prev := range r.Previews {
		prevID := fmt.Sprintf("%s_p%d", exportID, i+1)
		p.Previews = append(p.Previews, archive.PreviewPayload{
			PreviewExportID: prevID,
			Width:           prev.Width,
			Height:          prev.Height,
			ContentType:     prev.ContentType,
		})
	}

	// Build blob info (used by writeResourceBlob).
	var blob *blobReadInfo
	if r.Hash != "" {
		p.BlobRef = r.Hash
		blob = &blobReadInfo{
			resourceExportID: exportID,
			hash:             r.Hash,
			size:             r.FileSize,
			location:         r.GetCleanLocation(),
			storageLocation:  r.StorageLocation,
		}
	}

	return p, blob, nil
}

// ────────────────────────────────────────────────────────────────────────────
// writeResourceBlob
// ────────────────────────────────────────────────────────────────────────────

func (ctx *MahresourcesContext) writeResourceBlob(
	w *archive.Writer,
	info *blobReadInfo,
	plan *exportPlan,
	report ReporterFn,
) error {
	if w.HasBlob(info.hash) {
		return nil // already written by a prior resource
	}
	// Skip blobs that were flagged as missing during the pre-scan phase.
	// Warnings were already recorded at that point; don't duplicate them.
	if plan.missingBlobs != nil && plan.missingBlobs[info.hash] {
		return nil
	}

	fs, err := ctx.GetFsForStorageLocation(info.storageLocation)
	if err != nil {
		// Shouldn't normally happen post-pre-scan, but be defensive.
		msg := fmt.Sprintf("blob missing for resource %s: storage location error: %v", info.resourceExportID, err)
		plan.warnings = append(plan.warnings, msg)
		report(ProgressEvent{Warning: msg})
		return nil
	}

	f, err := fs.Open(info.location)
	if err != nil {
		// Shouldn't normally happen post-pre-scan, but be defensive.
		msg := fmt.Sprintf("blob missing for resource %s (%s): %v", info.resourceExportID, info.hash, err)
		plan.warnings = append(plan.warnings, msg)
		report(ProgressEvent{Warning: msg})
		return nil
	}
	defer f.Close()

	if err := w.WriteBlob(info.hash, f, info.size); err != nil {
		return fmt.Errorf("WriteBlob %s: %w", info.hash, err)
	}
	report(ProgressEvent{Phase: "blobs", BytesWritten: w.BytesWritten()})
	return nil
}

// ────────────────────────────────────────────────────────────────────────────
// toManifest
// ────────────────────────────────────────────────────────────────────────────

// toManifest converts the fully-built exportPlan into the archive.Manifest
// that will be written as the first tar entry.
func (p *exportPlan) toManifest(req *ExportRequest, ctx *MahresourcesContext) *archive.Manifest {
	// Count blobs written — we track unique hashes in plan.uniqueHashes already.
	blobCount := len(p.uniqueHashes)

	// Count previews: sum of previews across all resources (we need a DB count).
	// For simplicity we use the plan's resourceIDs and query once.
	var previewCount int64
	if req.Fidelity.ResourcePreviews && len(p.resourceIDs) > 0 {
		ctx.db.Model(&models.Preview{}).Where("resource_id IN ?", p.resourceIDs).Count(&previewCount)
	}

	// Count versions.
	var versionCount int64
	if req.Fidelity.ResourceVersions && len(p.resourceIDs) > 0 {
		ctx.db.Model(&models.ResourceVersion{}).Where("resource_id IN ?", p.resourceIDs).Count(&versionCount)
	}

	m := &archive.Manifest{
		SchemaVersion: archive.SchemaVersion,
		CreatedAt:     time.Now().UTC(),
		CreatedBy:     "mahresources",
		ExportOptions: archive.ExportOptions{
			Scope:      req.Scope,
			Fidelity:   req.Fidelity,
			SchemaDefs: req.SchemaDefs,
			Gzip:       req.Gzip,
		},
		Counts: archive.Counts{
			Groups:    len(p.groupIDs),
			Notes:     len(p.noteIDs),
			Resources: len(p.resourceIDs),
			Series:    len(p.seriesIDs),
			Blobs:     blobCount,
			Previews:  int(previewCount),
			Versions:  int(versionCount),
		},
		Dangling: p.dangling,
		Warnings: p.warnings,
	}

	// Build Roots slice from req.RootGroupIDs → export IDs.
	m.Roots = make([]string, 0, len(req.RootGroupIDs))
	for _, id := range req.RootGroupIDs {
		if ref, ok := p.groupExportID[id]; ok {
			m.Roots = append(m.Roots, ref)
		}
	}

	// Build Entries.
	// Groups — we need group names, load them minimally.
	type nameRow struct {
		ID   uint
		Name string
	}
	if len(p.groupIDs) > 0 {
		var rows []nameRow
		ctx.db.Model(&models.Group{}).Select("id, name").Where("id IN ?", p.groupIDs).Scan(&rows)
		for _, row := range rows {
			m.Entries.Groups = append(m.Entries.Groups, archive.GroupEntry{
				ExportID: p.groupExportID[row.ID],
				Name:     row.Name,
				SourceID: row.ID,
				Path:     "groups/" + p.groupExportID[row.ID] + ".json",
			})
		}
	}
	if len(p.noteIDs) > 0 {
		var rows []nameRow
		ctx.db.Model(&models.Note{}).Select("id, name, owner_id").Where("id IN ?", p.noteIDs).Scan(&rows)
		for _, row := range rows {
			m.Entries.Notes = append(m.Entries.Notes, archive.NoteEntry{
				ExportID: p.noteExportID[row.ID],
				Name:     row.Name,
				SourceID: row.ID,
				Path:     "notes/" + p.noteExportID[row.ID] + ".json",
			})
		}
	}
	if len(p.resourceIDs) > 0 {
		type resRow struct {
			ID      uint
			Name    string
			OwnerId *uint
			Hash    string
		}
		var rows []resRow
		ctx.db.Model(&models.Resource{}).Select("id, name, owner_id, hash").Where("id IN ?", p.resourceIDs).Scan(&rows)
		for _, row := range rows {
			ownerRef := ""
			if row.OwnerId != nil {
				ownerRef = p.groupExportID[*row.OwnerId]
			}
			m.Entries.Resources = append(m.Entries.Resources, archive.ResourceEntry{
				ExportID: p.resourceExportID[row.ID],
				Name:     row.Name,
				SourceID: row.ID,
				Owner:    ownerRef,
				Hash:     row.Hash,
				Path:     "resources/" + p.resourceExportID[row.ID] + ".json",
			})
		}
	}
	if len(p.seriesIDs) > 0 {
		var rows []nameRow
		ctx.db.Model(&models.Series{}).Select("id, name").Where("id IN ?", p.seriesIDs).Scan(&rows)
		for _, row := range rows {
			m.Entries.Series = append(m.Entries.Series, archive.SeriesEntry{
				ExportID: p.seriesExportID[row.ID],
				Name:     row.Name,
				SourceID: row.ID,
				Path:     "series/" + p.seriesExportID[row.ID] + ".json",
			})
		}
	}

	// Build SchemaDefs index.
	if req.SchemaDefs.CategoriesAndTypes && len(p.categoryExportID) > 0 {
		catIDs := keysOfUintMap(p.categoryExportID)
		sortAscUint(catIDs)
		var rows []nameRow
		ctx.db.Model(&models.Category{}).Select("id, name").Where("id IN ?", catIDs).Scan(&rows)
		for _, row := range rows {
			m.SchemaDefs.Categories = append(m.SchemaDefs.Categories, archive.SchemaDefEntry{
				ExportID: p.categoryExportID[row.ID],
				Name:     row.Name,
				SourceID: row.ID,
				Path:     "schemas/categories.json",
			})
		}
	}
	if req.SchemaDefs.CategoriesAndTypes && len(p.noteTypeExportID) > 0 {
		ntIDs := keysOfUintMap(p.noteTypeExportID)
		sortAscUint(ntIDs)
		var rows []nameRow
		ctx.db.Model(&models.NoteType{}).Select("id, name").Where("id IN ?", ntIDs).Scan(&rows)
		for _, row := range rows {
			m.SchemaDefs.NoteTypes = append(m.SchemaDefs.NoteTypes, archive.SchemaDefEntry{
				ExportID: p.noteTypeExportID[row.ID],
				Name:     row.Name,
				SourceID: row.ID,
				Path:     "schemas/note_types.json",
			})
		}
	}
	if req.SchemaDefs.CategoriesAndTypes && len(p.resourceCategoryExportID) > 0 {
		rcIDs := keysOfUintMap(p.resourceCategoryExportID)
		sortAscUint(rcIDs)
		var rows []nameRow
		ctx.db.Model(&models.ResourceCategory{}).Select("id, name").Where("id IN ?", rcIDs).Scan(&rows)
		for _, row := range rows {
			m.SchemaDefs.ResourceCategories = append(m.SchemaDefs.ResourceCategories, archive.SchemaDefEntry{
				ExportID: p.resourceCategoryExportID[row.ID],
				Name:     row.Name,
				SourceID: row.ID,
				Path:     "schemas/resource_categories.json",
			})
		}
	}
	if req.SchemaDefs.Tags && len(p.tagExportID) > 0 {
		tagIDs := keysOfUintMap(p.tagExportID)
		sortAscUint(tagIDs)
		var rows []nameRow
		ctx.db.Model(&models.Tag{}).Select("id, name").Where("id IN ?", tagIDs).Scan(&rows)
		for _, row := range rows {
			m.SchemaDefs.Tags = append(m.SchemaDefs.Tags, archive.SchemaDefEntry{
				ExportID: p.tagExportID[row.ID],
				Name:     row.Name,
				SourceID: row.ID,
				Path:     "schemas/tags.json",
			})
		}
	}
	if req.SchemaDefs.GroupRelationTypes && len(p.grtExportID) > 0 {
		grtIDs := keysOfUintMap(p.grtExportID)
		sortAscUint(grtIDs)
		var rows []nameRow
		ctx.db.Model(&models.GroupRelationType{}).Select("id, name").Where("id IN ?", grtIDs).Scan(&rows)
		for _, row := range rows {
			m.SchemaDefs.GroupRelationTypes = append(m.SchemaDefs.GroupRelationTypes, archive.SchemaDefEntry{
				ExportID: p.grtExportID[row.ID],
				Name:     row.Name,
				SourceID: row.ID,
				Path:     "schemas/group_relation_types.json",
			})
		}
	}

	return m
}

// ────────────────────────────────────────────────────────────────────────────
// Small helpers
// ────────────────────────────────────────────────────────────────────────────

// keysOfUintMap returns the keys of a map[uint]string as a []uint, sorted
// ascending for deterministic iteration.
func keysOfUintMap(m map[uint]string) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// keysOfUintBoolMap is like keysOfUintMap but for map[uint]bool (e.g. tag sets).
func keysOfUintBoolMap(m map[uint]bool) []uint {
	out := make([]uint, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// jsonToMap converts a types.JSON field to map[string]any. Returns an empty
// map on nil, empty, or "null" input — never returns nil.
func jsonToMap(j types.JSON) map[string]any {
	if len(j) == 0 {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(j), &m); err != nil || m == nil {
		return map[string]any{}
	}
	return m
}

// Ensure unused imports are used (gorm.DB needed for Preload callback closure).
var _ = time.Now
