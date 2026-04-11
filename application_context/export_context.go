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
			rows, err := ctx.GetGroupTreeDown(rootID, 100, 5000)
			if err != nil {
				return nil, fmt.Errorf("GetGroupTreeDown(%d): %w", rootID, err)
			}
			for _, row := range rows {
				groupSet[row.ID] = true
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
