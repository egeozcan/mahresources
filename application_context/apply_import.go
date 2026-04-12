package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path"
	"path/filepath"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"mahresources/archive"
	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/types"
)

const applyBatchSize = 500

// ApplyImport executes an import plan with user decisions. Two-phase:
// Phase 1 walks the tar (collect metadata + write blobs + buffer previews).
// Phase 2 creates DB entities in batched transactions from the collected data.
// Single-shot: caller must delete the plan file after enqueue to prevent re-apply.
func (ctx *MahresourcesContext) ApplyImport(
	cancelCtx context.Context,
	parseJobID string,
	decisions *ImportDecisions,
	sink download_queue.ProgressSink,
) (*ImportApplyResult, error) {
	plan, err := ctx.LoadImportPlan(parseJobID)
	if err != nil {
		return nil, fmt.Errorf("load plan: %w", err)
	}
	if err := plan.ValidateForApply(decisions); err != nil {
		return nil, err
	}

	tarPath := filepath.Join("_imports", parseJobID+".tar")

	// --- Phase 1: collect entity payloads + write blobs + buffer previews ---
	sink.SetPhase("reading archive")
	state := &applyState{
		ctx:                ctx,
		plan:               plan,
		decisions:          decisions,
		sink:               sink,
		cancelCtx:          cancelCtx,
		idMap:              make(map[string]uint),
		blobPaths:          make(map[string]string),
		previewData:        make(map[string][]byte),
		createdResourceIDs: make(map[string]bool),
		result:             &ImportApplyResult{},
	}

	collector, err := state.collectAndWriteBlobs(tarPath)
	if err != nil {
		// Phase 1 failure: no DB rows created yet, nothing to clean up.
		return nil, err
	}
	state.collector = collector

	// --- Phase 2: create DB entities in batched transactions ---
	// From this point on, every error return includes state.result so the
	// caller can persist the created-ID lists for partial-failure cleanup.

	if err := cancelCtx.Err(); err != nil {
		return state.result, err
	}

	sink.SetPhase("resolving schema definitions")
	if err := state.applySchemaDefDecisions(); err != nil {
		return state.result, fmt.Errorf("schema defs: %w", err)
	}

	if err := cancelCtx.Err(); err != nil {
		return state.result, err
	}

	sink.SetPhase("resolving series")
	if err := state.applySeries(); err != nil {
		return state.result, fmt.Errorf("series: %w", err)
	}

	sink.SetPhase("creating groups")
	if err := state.applyGroups(); err != nil {
		return state.result, fmt.Errorf("groups: %w", err)
	}

	if err := cancelCtx.Err(); err != nil {
		return state.result, err
	}

	// Resources in batches of 500 — each batch is one transaction covering
	// the resource row, version rows, CurrentVersionID, and preview rows.
	sink.SetPhase("creating resources")
	if err := state.applyResources(); err != nil {
		return state.result, fmt.Errorf("resources: %w", err)
	}

	if err := cancelCtx.Err(); err != nil {
		return state.result, err
	}

	// Notes in batches of 500
	sink.SetPhase("creating notes")
	if err := state.applyNotes(); err != nil {
		return state.result, fmt.Errorf("notes: %w", err)
	}

	if err := cancelCtx.Err(); err != nil {
		return state.result, err
	}

	sink.SetPhase("wiring relationships")
	if err := state.applyM2MLinks(); err != nil {
		return state.result, fmt.Errorf("m2m links: %w", err)
	}
	if err := state.applyDanglingDecisions(); err != nil {
		return state.result, fmt.Errorf("dangling refs: %w", err)
	}

	sink.SetPhase("completed")
	state.result.Warnings = append(plan.Warnings, state.result.Warnings...)
	return state.result, nil
}

// applyState holds all mutable state for the apply operation. Methods on this
// struct implement each phase of the apply pipeline.
type applyState struct {
	ctx       *MahresourcesContext
	collector *importDataCollector
	plan      *ImportPlan
	decisions *ImportDecisions
	sink      download_queue.ProgressSink
	cancelCtx context.Context

	// idMap tracks export_id -> destination DB ID for all created/mapped entities.
	// Also stores decision_key -> destination ID for schema def mappings.
	idMap map[string]uint

	// blobPaths: hash -> filesystem path (computed during collection, written during walk)
	blobPaths map[string]string

	// previewData: preview_export_id -> raw bytes (buffered from tar, consumed during
	// resource batch creation). Cleared entry-by-entry after use to free memory.
	previewData map[string][]byte

	// createdResourceIDs: tracks export IDs of resources that were actually created
	// (not skipped via hash collision). Only created resources receive previews/versions.
	createdResourceIDs map[string]bool

	result *ImportApplyResult
}

// collectAndWriteBlobs walks the tar once. Entity metadata is collected into the
// returned importDataCollector. Blob files are written to disk. Preview bytes are
// buffered into state.previewData. If a blob write fails (disk full, permissions),
// the error is returned before any DB rows exist — clean failure.
func (s *applyState) collectAndWriteBlobs(tarPath string) (*importDataCollector, error) {
	f, err := s.ctx.fs.Open(tarPath)
	if err != nil {
		return nil, fmt.Errorf("open tar: %w", err)
	}
	defer f.Close()

	r, err := archive.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("new reader: %w", err)
	}
	defer r.Close()

	if _, err := r.ReadManifest(); err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	v := &collectBlobVisitor{
		collector: &importDataCollector{
			groups:    make(map[string]*archive.GroupPayload),
			notes:     make(map[string]*archive.NotePayload),
			resources: make(map[string]*archive.ResourcePayload),
			series:    make(map[string]*archive.SeriesPayload),
		},
		fs:          s.ctx.fs,
		cancelCtx:   s.cancelCtx,
		blobPaths:   s.blobPaths,
		previewData: s.previewData,
	}

	if err := r.Walk(v); err != nil {
		return nil, fmt.Errorf("walk tar: %w", err)
	}

	return v.collector, nil
}

// collectBlobVisitor collects entity metadata, writes blobs, buffers previews.
type collectBlobVisitor struct {
	collector       *importDataCollector
	fs              afero.Fs
	cancelCtx       context.Context
	blobPaths       map[string]string // hash -> path (populated during walk)
	previewData     map[string][]byte // preview_export_id -> bytes
	hashContentType map[string]string // built lazily on first blob
	hashMapBuilt    bool
}

func (v *collectBlobVisitor) OnGroup(p *archive.GroupPayload) error   { return v.collector.OnGroup(p) }
func (v *collectBlobVisitor) OnNote(p *archive.NotePayload) error     { return v.collector.OnNote(p) }
func (v *collectBlobVisitor) OnResource(p *archive.ResourcePayload) error {
	return v.collector.OnResource(p)
}
func (v *collectBlobVisitor) OnSeries(p *archive.SeriesPayload) error { return v.collector.OnSeries(p) }
func (v *collectBlobVisitor) OnCategoryDefs(d []archive.CategoryDef) error {
	return v.collector.OnCategoryDefs(d)
}
func (v *collectBlobVisitor) OnNoteTypeDefs(d []archive.NoteTypeDef) error {
	return v.collector.OnNoteTypeDefs(d)
}
func (v *collectBlobVisitor) OnResourceCategoryDefs(d []archive.ResourceCategoryDef) error {
	return v.collector.OnResourceCategoryDefs(d)
}
func (v *collectBlobVisitor) OnTagDefs(d []archive.TagDef) error { return v.collector.OnTagDefs(d) }
func (v *collectBlobVisitor) OnGroupRelationTypeDefs(d []archive.GroupRelationTypeDef) error {
	return v.collector.OnGroupRelationTypeDefs(d)
}

func (v *collectBlobVisitor) OnBlob(hash string, body io.Reader, size int64) error {
	if err := v.cancelCtx.Err(); err != nil {
		return err
	}
	// Lazily build hash->contentType from collected resource payloads
	if !v.hashMapBuilt {
		v.hashMapBuilt = true
		v.hashContentType = make(map[string]string)
		for _, r := range v.collector.resources {
			if r.Hash != "" {
				v.hashContentType[r.Hash] = r.ContentType
			}
			for _, ver := range r.Versions {
				if ver.Hash != "" {
					v.hashContentType[ver.Hash] = ver.ContentType
				}
			}
		}
	}

	ct := v.hashContentType[hash]
	loc := importBlobLocation(hash, ct)
	v.blobPaths[hash] = loc

	// Skip if already on disk (content-addressed, idempotent)
	if _, err := v.fs.Stat(loc); err == nil {
		return nil
	}
	if err := v.fs.MkdirAll(filepath.Dir(loc), 0755); err != nil {
		return fmt.Errorf("create blob dir: %w", err)
	}
	f, err := v.fs.Create(loc)
	if err != nil {
		return fmt.Errorf("create blob %s: %w", hash, err)
	}
	_, copyErr := io.Copy(f, body)
	f.Close()
	if copyErr != nil {
		return fmt.Errorf("write blob %s: %w", hash, copyErr)
	}
	return nil
}

func (v *collectBlobVisitor) OnPreview(previewExportID string, body io.Reader, size int64) error {
	if err := v.cancelCtx.Err(); err != nil {
		return err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return fmt.Errorf("read preview %s: %w", previewExportID, err)
	}
	v.previewData[previewExportID] = data
	return nil
}

// --- Helpers used by all apply phases ---

// importBlobLocation computes the filesystem path for a blob based on hash
// and content type. Uses "resources/" folder to match the normal upload path.
func importBlobLocation(hash, contentType string) string {
	ext := ""
	if exts, _ := mime.ExtensionsByType(contentType); len(exts) > 0 {
		ext = exts[0]
	}
	return path.Join("resources", hash+ext)
}

func (s *applyState) isExcluded(exportID string) bool {
	for _, id := range s.decisions.ExcludedItems {
		if id == exportID {
			return true
		}
	}
	return false
}

func (s *applyState) resolveTagIDs(refs []archive.TagRef) []uint {
	ids := make([]uint, 0, len(refs))
	for _, tr := range refs {
		if tr.Ref != "" {
			if destID, ok := s.idMap[tr.Ref]; ok {
				ids = append(ids, destID)
				continue
			}
		}
		key := DecisionKeyFor("tag", MappingEntry{SourceKey: tr.Name})
		if destID, ok := s.idMap[key]; ok {
			ids = append(ids, destID)
		}
	}
	return ids
}

// applySchemaDefDecisions resolves categories, note types, resource categories,
// tags, and group relation types according to the user's mapping decisions.
// For each entry: "map" reuses an existing DB row, "create" inserts a new one.
// IDs are stored in s.idMap keyed by both DecisionKey and SourceExportID.
func (s *applyState) applySchemaDefDecisions() error {
	// --- Categories ---
	for _, entry := range s.plan.Mappings.Categories {
		action, ok := s.decisions.MappingActions[entry.DecisionKey]
		if ok && action.Action == "map" && action.DestinationID != nil {
			s.idMap[entry.DecisionKey] = *action.DestinationID
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = *action.DestinationID
			}
			continue
		}
		if ok && !action.Include {
			continue
		}
		// Create new category
		cat := models.Category{Name: entry.SourceKey}
		if entry.HasPayload {
			if def := findCategoryDef(s.collector.categoryDefs, entry.SourceExportID); def != nil {
				cat.Name = def.Name
				cat.Description = def.Description
				cat.CustomHeader = def.CustomHeader
				cat.CustomSidebar = def.CustomSidebar
				cat.CustomSummary = def.CustomSummary
				cat.CustomAvatar = def.CustomAvatar
				cat.CustomMRQLResult = def.CustomMRQLResult
				cat.MetaSchema = def.MetaSchema
				if def.SectionConfig != nil {
					sc, _ := json.Marshal(def.SectionConfig)
					cat.SectionConfig = sc
				}
			}
		}
		if err := s.ctx.db.Create(&cat).Error; err != nil {
			return fmt.Errorf("create category %q: %w", cat.Name, err)
		}
		s.idMap[entry.DecisionKey] = cat.ID
		if entry.SourceExportID != "" {
			s.idMap[entry.SourceExportID] = cat.ID
		}
		s.result.CreatedCategories++
	}

	// --- Note Types ---
	for _, entry := range s.plan.Mappings.NoteTypes {
		action, ok := s.decisions.MappingActions[entry.DecisionKey]
		if ok && action.Action == "map" && action.DestinationID != nil {
			s.idMap[entry.DecisionKey] = *action.DestinationID
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = *action.DestinationID
			}
			continue
		}
		if ok && !action.Include {
			continue
		}
		nt := models.NoteType{Name: entry.SourceKey}
		if entry.HasPayload {
			if def := findNoteTypeDef(s.collector.noteTypeDefs, entry.SourceExportID); def != nil {
				nt.Name = def.Name
				nt.Description = def.Description
				nt.CustomHeader = def.CustomHeader
				nt.CustomSidebar = def.CustomSidebar
				nt.CustomSummary = def.CustomSummary
				nt.CustomAvatar = def.CustomAvatar
				nt.CustomMRQLResult = def.CustomMRQLResult
				nt.MetaSchema = def.MetaSchema
				if def.SectionConfig != nil {
					sc, _ := json.Marshal(def.SectionConfig)
					nt.SectionConfig = sc
				}
			}
		}
		if err := s.ctx.db.Create(&nt).Error; err != nil {
			return fmt.Errorf("create note type %q: %w", nt.Name, err)
		}
		s.idMap[entry.DecisionKey] = nt.ID
		if entry.SourceExportID != "" {
			s.idMap[entry.SourceExportID] = nt.ID
		}
		s.result.CreatedNoteTypes++
	}

	// --- Resource Categories ---
	for _, entry := range s.plan.Mappings.ResourceCategories {
		action, ok := s.decisions.MappingActions[entry.DecisionKey]
		if ok && action.Action == "map" && action.DestinationID != nil {
			s.idMap[entry.DecisionKey] = *action.DestinationID
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = *action.DestinationID
			}
			continue
		}
		if ok && !action.Include {
			continue
		}
		rc := models.ResourceCategory{Name: entry.SourceKey}
		if entry.HasPayload {
			if def := findResourceCategoryDef(s.collector.resourceCategoryDefs, entry.SourceExportID); def != nil {
				rc.Name = def.Name
				rc.Description = def.Description
				rc.CustomHeader = def.CustomHeader
				rc.CustomSidebar = def.CustomSidebar
				rc.CustomSummary = def.CustomSummary
				rc.CustomAvatar = def.CustomAvatar
				rc.CustomMRQLResult = def.CustomMRQLResult
				rc.MetaSchema = def.MetaSchema
				rc.AutoDetectRules = def.AutoDetectRules
				if def.SectionConfig != nil {
					sc, _ := json.Marshal(def.SectionConfig)
					rc.SectionConfig = sc
				}
			}
		}
		if err := s.ctx.db.Create(&rc).Error; err != nil {
			return fmt.Errorf("create resource category %q: %w", rc.Name, err)
		}
		s.idMap[entry.DecisionKey] = rc.ID
		if entry.SourceExportID != "" {
			s.idMap[entry.SourceExportID] = rc.ID
		}
		s.result.CreatedResourceCategories++
	}

	// --- Tags ---
	for _, entry := range s.plan.Mappings.Tags {
		action, ok := s.decisions.MappingActions[entry.DecisionKey]
		if ok && action.Action == "map" && action.DestinationID != nil {
			s.idMap[entry.DecisionKey] = *action.DestinationID
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = *action.DestinationID
			}
			continue
		}
		if ok && !action.Include {
			continue
		}
		tag := models.Tag{Name: entry.SourceKey}
		if entry.HasPayload {
			if def := findTagDef(s.collector.tagDefs, entry.SourceExportID); def != nil {
				tag.Name = def.Name
				tag.Description = def.Description
				if def.Meta != nil {
					m, _ := json.Marshal(def.Meta)
					tag.Meta = m
				}
			}
		}
		if err := s.ctx.db.Create(&tag).Error; err != nil {
			return fmt.Errorf("create tag %q: %w", tag.Name, err)
		}
		s.idMap[entry.DecisionKey] = tag.ID
		if entry.SourceExportID != "" {
			s.idMap[entry.SourceExportID] = tag.ID
		}
		s.result.CreatedTags++
	}

	// --- Group Relation Types (first pass: create rows) ---
	// We track which entries were created so the second pass can wire BackRelationId.
	type grtCreated struct {
		entry MappingEntry
		id    uint
	}
	var createdGRTs []grtCreated

	for _, entry := range s.plan.Mappings.GroupRelationTypes {
		action, ok := s.decisions.MappingActions[entry.DecisionKey]
		if ok && action.Action == "map" && action.DestinationID != nil {
			s.idMap[entry.DecisionKey] = *action.DestinationID
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = *action.DestinationID
			}
			continue
		}
		if ok && !action.Include {
			continue
		}
		grt := models.GroupRelationType{Name: entry.SourceKey}
		if entry.HasPayload {
			if def := findGRTDef(s.collector.grtDefs, entry.SourceExportID); def != nil {
				grt.Name = def.Name
				grt.Description = def.Description
				// Resolve FromCategoryId via already-mapped categories
				if def.FromCategoryName != "" {
					fromKey := DecisionKeyFor("category", MappingEntry{SourceKey: def.FromCategoryName})
					if catID, ok := s.idMap[fromKey]; ok {
						grt.FromCategoryId = &catID
					} else if def.FromCategoryRef != "" {
						if catID, ok := s.idMap[def.FromCategoryRef]; ok {
							grt.FromCategoryId = &catID
						}
					}
				}
				// Resolve ToCategoryId via already-mapped categories
				if def.ToCategoryName != "" {
					toKey := DecisionKeyFor("category", MappingEntry{SourceKey: def.ToCategoryName})
					if catID, ok := s.idMap[toKey]; ok {
						grt.ToCategoryId = &catID
					} else if def.ToCategoryRef != "" {
						if catID, ok := s.idMap[def.ToCategoryRef]; ok {
							grt.ToCategoryId = &catID
						}
					}
				}
			}
		}
		if err := s.ctx.db.Create(&grt).Error; err != nil {
			return fmt.Errorf("create GRT %q: %w", grt.Name, err)
		}
		s.idMap[entry.DecisionKey] = grt.ID
		if entry.SourceExportID != "" {
			s.idMap[entry.SourceExportID] = grt.ID
		}
		createdGRTs = append(createdGRTs, grtCreated{entry: entry, id: grt.ID})
		s.result.CreatedGRTs++
	}

	// --- Group Relation Types (second pass: wire BackRelationId) ---
	for _, cg := range createdGRTs {
		def := findGRTDef(s.collector.grtDefs, cg.entry.SourceExportID)
		if def == nil || def.BackRelationRef == "" {
			continue
		}
		backID, ok := s.idMap[def.BackRelationRef]
		if !ok {
			continue
		}
		if err := s.ctx.db.Model(&models.GroupRelationType{}).Where("id = ?", cg.id).
			Update("back_relation_id", backID).Error; err != nil {
			return fmt.Errorf("wire GRT back-relation %d -> %d: %w", cg.id, backID, err)
		}
	}

	return nil
}

// applySeries resolves series references: reuse existing or create new.
func (s *applyState) applySeries() error {
	for _, sm := range s.plan.SeriesInfo {
		if sm.Action == "reuse_existing" && sm.DestID != nil {
			s.idMap[sm.ExportID] = *sm.DestID
			s.result.ReusedSeries++
			s.result.Warnings = append(s.result.Warnings,
				fmt.Sprintf("reused existing series %q (slug=%s, id=%d)", sm.Name, sm.Slug, *sm.DestID))
			continue
		}
		// Create new series
		sp := s.collector.series[sm.ExportID]
		series := models.Series{
			Name: sm.Name,
			Slug: sm.Slug,
		}
		if sp != nil {
			series.Name = sp.Name
			series.Slug = sp.Slug
			if sp.Meta != nil {
				m, _ := json.Marshal(sp.Meta)
				series.Meta = m
			}
		}
		if err := s.ctx.db.Create(&series).Error; err != nil {
			return fmt.Errorf("create series %q: %w", series.Name, err)
		}
		s.idMap[sm.ExportID] = series.ID
		s.result.CreatedSeries++
	}
	return nil
}

// applyGroups creates groups in topological order (depth-first walk of the
// item tree) so that parent groups are always created before their children.
func (s *applyState) applyGroups() error {
	var walk func(items []ImportPlanItem) error
	walk = func(items []ImportPlanItem) error {
		for _, item := range items {
			if item.Kind != "group" {
				continue
			}
			if s.isExcluded(item.ExportID) {
				continue
			}

			gp, ok := s.collector.groups[item.ExportID]
			if !ok {
				s.result.Warnings = append(s.result.Warnings,
					fmt.Sprintf("group %s referenced in plan but not found in archive", item.ExportID))
				continue
			}

			g := models.Group{
				Name:        gp.Name,
				Description: gp.Description,
			}

			// Meta
			if gp.Meta != nil {
				m, _ := json.Marshal(gp.Meta)
				g.Meta = m
			}

			// URL
			if gp.URL != "" {
				parsed, err := url.Parse(gp.URL)
				if err == nil {
					u := types.URL(*parsed)
					g.URL = &u
				}
			}

			// Owner: resolve from OwnerRef (parent in the archive), fall back
			// to decisions.ParentGroupID for root groups.
			if gp.OwnerRef != "" {
				if ownerID, ok := s.idMap[gp.OwnerRef]; ok {
					g.OwnerId = &ownerID
				}
			} else if s.decisions.ParentGroupID != nil {
				g.OwnerId = s.decisions.ParentGroupID
			}

			// Category: try CategoryRef first, then CategoryName lookup
			if gp.CategoryRef != "" {
				if catID, ok := s.idMap[gp.CategoryRef]; ok {
					g.CategoryId = &catID
				}
			}
			if g.CategoryId == nil && gp.CategoryName != "" {
				catKey := DecisionKeyFor("category", MappingEntry{SourceKey: gp.CategoryName})
				if catID, ok := s.idMap[catKey]; ok {
					g.CategoryId = &catID
				}
			}

			if err := s.ctx.db.Create(&g).Error; err != nil {
				return fmt.Errorf("create group %q: %w", g.Name, err)
			}
			s.idMap[item.ExportID] = g.ID
			s.result.CreatedGroupIDs = append(s.result.CreatedGroupIDs, g.ID)
			s.result.CreatedGroups++

			// Recurse into children
			if err := walk(item.Children); err != nil {
				return err
			}
		}
		return nil
	}

	return walk(s.plan.Items)
}

// resEntry pairs an export ID with its resource payload for ordered iteration.
type resEntry struct {
	exportID string
	payload  *archive.ResourcePayload
}

// applyResources creates resource rows in batches of applyBatchSize. Each batch
// runs inside a single DB transaction covering the resource row, version rows,
// CurrentVersionID wiring, and preview rows.
func (s *applyState) applyResources() error {
	// Build ordered list, excluding resources whose owner was excluded or unmapped.
	var entries []resEntry
	for exportID, rp := range s.collector.resources {
		if s.isExcluded(exportID) {
			continue
		}
		// Owner must be resolvable (mapped or absent). If OwnerRef is set but
		// not in idMap, the owner group was excluded — skip this resource.
		if rp.OwnerRef != "" {
			if _, ok := s.idMap[rp.OwnerRef]; !ok {
				continue
			}
		}
		entries = append(entries, resEntry{exportID: exportID, payload: rp})
	}

	for batchStart := 0; batchStart < len(entries); batchStart += applyBatchSize {
		if err := s.cancelCtx.Err(); err != nil {
			return err
		}

		batchEnd := batchStart + applyBatchSize
		if batchEnd > len(entries) {
			batchEnd = len(entries)
		}
		batch := entries[batchStart:batchEnd]
		batchNum := batchStart/applyBatchSize + 1

		tx := s.ctx.db.Begin()
		for _, entry := range batch {
			if err := s.applyOneResource(tx, entry.exportID, entry.payload); err != nil {
				tx.Rollback()
				return fmt.Errorf("batch %d, resource %s: %w", batchNum, entry.exportID, err)
			}
		}
		if err := tx.Commit().Error; err != nil {
			return fmt.Errorf("commit batch %d: %w", batchNum, err)
		}

		s.sink.SetPhaseProgress(int64(batchEnd), int64(len(entries)))
	}
	return nil
}

// applyOneResource creates a single resource row plus its versions, wires
// CurrentVersionID, and creates preview rows — all within the caller's tx.
func (s *applyState) applyOneResource(tx *gorm.DB, exportID string, rp *archive.ResourcePayload) error {
	// (a) Skip-on-hash collision
	if rp.Hash != "" {
		var existing models.Resource
		if err := tx.Where("hash = ?", rp.Hash).First(&existing).Error; err == nil {
			if s.decisions.ResourceCollisionPolicy == "skip" {
				s.idMap[exportID] = existing.ID
				// Do NOT mark as created — skipped resources get no previews/versions
				s.result.SkippedByHash++
				return nil
			}
		}
	}

	// (a') Manifest-only missing bytes
	if rp.BlobMissing || (rp.BlobRef == "" && rp.Hash != "") {
		// Check if hash exists on destination
		var existing models.Resource
		if err := tx.Where("hash = ?", rp.Hash).First(&existing).Error; err == nil {
			// Reuse existing
			s.idMap[exportID] = existing.ID
			s.result.SkippedByHash++
			return nil
		}
		// No existing resource with this hash — skip with warning
		s.result.SkippedMissingBytes++
		s.result.Warnings = append(s.result.Warnings,
			fmt.Sprintf("resource %s (%s): blob missing and no existing resource with hash %s", exportID, rp.Name, rp.Hash))
		return nil
	}

	// (b) Blob location
	loc := s.blobPaths[rp.Hash]
	if loc == "" {
		loc = importBlobLocation(rp.Hash, rp.ContentType)
	}

	// (c) Create resource row
	r := models.Resource{
		Name:             rp.Name,
		OriginalName:     rp.OriginalName,
		OriginalLocation: rp.OriginalLocation,
		Hash:             rp.Hash,
		HashType:         rp.HashType,
		Location:         loc,
		Description:      rp.Description,
		Width:            rp.Width,
		Height:           rp.Height,
		FileSize:         rp.FileSize,
		Category:         rp.Category,
		ContentType:      rp.ContentType,
		ContentCategory:  rp.ContentCategory,
		CreatedAt:        rp.CreatedAt,
		UpdatedAt:        rp.UpdatedAt,
	}

	// Resolve OwnerRef
	if rp.OwnerRef != "" {
		if ownerID, ok := s.idMap[rp.OwnerRef]; ok {
			r.OwnerId = &ownerID
		}
	}

	// Resolve ResourceCategory
	r.ResourceCategoryId = s.resolveResourceCategoryID(rp)

	// Resolve SeriesRef
	if rp.SeriesRef != "" {
		if seriesID, ok := s.idMap[rp.SeriesRef]; ok {
			r.SeriesID = &seriesID
		}
	}

	// Marshal Meta
	if rp.Meta != nil {
		m, err := json.Marshal(rp.Meta)
		if err != nil {
			return fmt.Errorf("marshal meta: %w", err)
		}
		r.Meta = types.JSON(m)
	}

	// Marshal OwnMeta
	if rp.OwnMeta != nil {
		m, err := json.Marshal(rp.OwnMeta)
		if err != nil {
			return fmt.Errorf("marshal own_meta: %w", err)
		}
		r.OwnMeta = types.JSON(m)
	}

	if err := tx.Create(&r).Error; err != nil {
		return fmt.Errorf("create resource: %w", err)
	}

	s.idMap[exportID] = r.ID
	s.createdResourceIDs[exportID] = true
	s.result.CreatedResources++
	s.result.CreatedResourceIDs = append(s.result.CreatedResourceIDs, r.ID)

	// (d) Resource versions
	for _, vp := range rp.Versions {
		vLoc := s.blobPaths[vp.Hash]
		if vLoc == "" {
			vLoc = importBlobLocation(vp.Hash, vp.ContentType)
		}

		ver := models.ResourceVersion{
			ResourceID:    r.ID,
			VersionNumber: vp.VersionNumber,
			Hash:          vp.Hash,
			HashType:      vp.HashType,
			FileSize:      vp.FileSize,
			ContentType:   vp.ContentType,
			Width:         vp.Width,
			Height:        vp.Height,
			Location:      vLoc,
			Comment:       vp.Comment,
			CreatedAt:     vp.CreatedAt,
		}
		if err := tx.Create(&ver).Error; err != nil {
			return fmt.Errorf("create version %s: %w", vp.VersionExportID, err)
		}
		s.idMap[vp.VersionExportID] = ver.ID
		s.result.CreatedVersions++
	}

	// (e) Wire CurrentVersionID
	if rp.CurrentVersionRef != "" {
		if cvID, ok := s.idMap[rp.CurrentVersionRef]; ok {
			if err := tx.Model(&r).Update("current_version_id", cvID).Error; err != nil {
				return fmt.Errorf("wire current_version_id: %w", err)
			}
		}
	}

	// (f) Previews
	for _, pp := range rp.Previews {
		data, ok := s.previewData[pp.PreviewExportID]
		if !ok || len(data) == 0 {
			continue
		}
		preview := models.Preview{
			ResourceId:  &r.ID,
			Data:        data,
			Width:       pp.Width,
			Height:      pp.Height,
			ContentType: pp.ContentType,
		}
		if err := tx.Create(&preview).Error; err != nil {
			return fmt.Errorf("create preview %s: %w", pp.PreviewExportID, err)
		}
		// Free memory after use
		delete(s.previewData, pp.PreviewExportID)
		s.result.CreatedPreviews++
	}

	return nil
}

// resolveResourceCategoryID resolves the resource category for a resource payload.
// Tries ResourceCategoryRef -> idMap, then ResourceCategoryName -> DecisionKeyFor -> idMap,
// fallback to 1.
func (s *applyState) resolveResourceCategoryID(rp *archive.ResourcePayload) uint {
	// Try ref first
	if rp.ResourceCategoryRef != "" {
		if id, ok := s.idMap[rp.ResourceCategoryRef]; ok {
			return id
		}
	}
	// Try name via decision key
	if rp.ResourceCategoryName != "" {
		key := DecisionKeyFor("resource_category", MappingEntry{SourceKey: rp.ResourceCategoryName})
		if id, ok := s.idMap[key]; ok {
			return id
		}
	}
	return 1
}
func (s *applyState) applyNotes() error              { return nil }
func (s *applyState) applyM2MLinks() error           { return nil }
func (s *applyState) applyDanglingDecisions() error  { return nil }

// --- Schema def lookup helpers ---

func findCategoryDef(defs []archive.CategoryDef, exportID string) *archive.CategoryDef {
	for i := range defs {
		if defs[i].ExportID == exportID {
			return &defs[i]
		}
	}
	return nil
}

func findNoteTypeDef(defs []archive.NoteTypeDef, exportID string) *archive.NoteTypeDef {
	for i := range defs {
		if defs[i].ExportID == exportID {
			return &defs[i]
		}
	}
	return nil
}

func findResourceCategoryDef(defs []archive.ResourceCategoryDef, exportID string) *archive.ResourceCategoryDef {
	for i := range defs {
		if defs[i].ExportID == exportID {
			return &defs[i]
		}
	}
	return nil
}

func findTagDef(defs []archive.TagDef, exportID string) *archive.TagDef {
	for i := range defs {
		if defs[i].ExportID == exportID {
			return &defs[i]
		}
	}
	return nil
}

func findGRTDef(defs []archive.GroupRelationTypeDef, exportID string) *archive.GroupRelationTypeDef {
	for i := range defs {
		if defs[i].ExportID == exportID {
			return &defs[i]
		}
	}
	return nil
}
