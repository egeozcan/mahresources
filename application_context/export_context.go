package application_context

import (
	"fmt"

	"mahresources/archive"
	"mahresources/models"
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
	ref.ID = fmt.Sprintf("d%04d", p.danglingNext)
	p.dangling = append(p.dangling, ref)
}
