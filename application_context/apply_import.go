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
	"time"

	"github.com/spf13/afero"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

	if decisions.GUIDCollisionPolicy == "" {
		decisions.GUIDCollisionPolicy = "merge"
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
		skippedM2M:         make(map[string]bool),
		blobPaths:          make(map[string]string),
		previewData:        make(map[string][]byte),
		createdResourceIDs: make(map[string]bool),
		// RetrySafe defaults true — Phase 1 collection flips it false for
		// legacy pre-GUID archives that would duplicate groups/notes on retry.
		result: &ImportApplyResult{RetrySafe: true},
	}

	collector, err := state.collectAndWriteBlobs(tarPath)
	if err != nil {
		// Phase 1 failure: no DB rows created yet, nothing to clean up.
		return nil, err
	}
	state.collector = collector
	state.evaluateRetrySafety()

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

	// skippedM2M holds export IDs of entities whose existing rows must NOT
	// receive M2M wiring (tags, related entities, group relations) from the
	// archive. Populated by GUID-skip, hash-skip, and manifest-only-missing-bytes
	// branches; honored by applyM2MLinks.
	skippedM2M map[string]bool

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

// evaluateRetrySafety decides whether replaying this plan against a DB that
// might already hold rows from a prior partial run is safe. Unsafe when:
//
//   - any group, note, or resource in the archive lacks a GUID (legacy
//     pre-GUID format or mixed-format archive): group/note names aren't
//     uniquely indexed, and a GUID-less resource falls into the hash
//     collision branch of applyOneResource which either silently drops
//     M2M wiring (skip policy) or inserts a duplicate row (non-skip
//     policies);
//
//   - GUIDCollisionPolicy == "skip": on retry, the rows created by the
//     first run now trigger the GUID collision path and land in the skip
//     branches, which add them to skippedM2M. applyM2MLinks then drops
//     the archive's tags / related entities / group relations for those
//     rows, so the retry would silently complete with an incomplete
//     graph.
//
// The handler reads result.RetrySafe to decide whether to restore the
// consumed plan for a retry; false means the user must re-upload.
func (s *applyState) evaluateRetrySafety() {
	if s.decisions.GUIDCollisionPolicy == "skip" {
		s.result.RetrySafe = false
		return
	}
	for _, gp := range s.collector.groups {
		if gp.GUID == "" {
			s.result.RetrySafe = false
			return
		}
	}
	for _, np := range s.collector.notes {
		if np.GUID == "" {
			s.result.RetrySafe = false
			return
		}
	}
	for _, rp := range s.collector.resources {
		if rp.GUID == "" {
			s.result.RetrySafe = false
			return
		}
	}
}

// findSchemaDefByGUID looks up a schema-def row by GUID so retries after a
// partial apply reuse rows created on a prior run instead of hitting the
// unique constraint. Returns (id, true) on match, (0, false) otherwise.
// guid == nil (fresh import) short-circuits to (0, false).
func (s *applyState) findSchemaDefByGUID(model any, guid *string) (uint, bool) {
	if guid == nil || *guid == "" {
		return 0, false
	}
	var id uint
	err := s.ctx.db.Model(model).Where("guid = ?", *guid).Limit(1).Pluck("id", &id).Error
	if err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

// findSchemaDefByName is the name-based fallback for retry idempotency when
// the archive carried no GUID (e.g., schema defs turned off at export time
// so ParseImport synthesized name-only "create" mappings). A previous run
// already inserted a row with this name and a locally-generated GUID; on
// retry the unique-name index would otherwise fail the create. Also used
// for NoteType which has no unique-name index but still must not be
// silently duplicated on a retry (duplicate would poison resolveNoteTypeID).
func (s *applyState) findSchemaDefByName(model any, name string) (uint, bool) {
	if name == "" {
		return 0, false
	}
	var id uint
	err := s.ctx.db.Model(model).Where("name = ?", name).Limit(1).Pluck("id", &id).Error
	if err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

// findGRTByCompositeKey is GRT's analog of findSchemaDefByName. GRT's
// uniqueness is the composite (name, from_category_id, to_category_id),
// so name alone is insufficient. Used on retry when a prior partial apply
// already inserted this GRT without a GUID we can key on.
func (s *applyState) findGRTByCompositeKey(grt *models.GroupRelationType) (uint, bool) {
	if grt.Name == "" {
		return 0, false
	}
	q := s.ctx.db.Model(&models.GroupRelationType{}).Where("name = ?", grt.Name)
	if grt.FromCategoryId != nil {
		q = q.Where("from_category_id = ?", *grt.FromCategoryId)
	} else {
		q = q.Where("from_category_id IS NULL")
	}
	if grt.ToCategoryId != nil {
		q = q.Where("to_category_id = ?", *grt.ToCategoryId)
	} else {
		q = q.Where("to_category_id IS NULL")
	}
	var id uint
	if err := q.Limit(1).Pluck("id", &id).Error; err != nil || id == 0 {
		return 0, false
	}
	return id, true
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
		if ok && action.Action == "guid_rename" {
			if action.RenameTo == "" {
				return fmt.Errorf("guid_rename action for %s requires rename_to", entry.DecisionKey)
			}
			if err := s.ctx.db.Model(&models.Category{}).
				Where("id = ?", entry.GUIDMatchID).
				Update("name", action.RenameTo).Error; err != nil {
				return fmt.Errorf("guid_rename category %s: %w", entry.DecisionKey, err)
			}
			s.idMap[entry.DecisionKey] = entry.GUIDMatchID
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = entry.GUIDMatchID
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
				if def.GUID != "" {
					guid := def.GUID
					cat.GUID = &guid
				}
				if def.SectionConfig != nil {
					sc, _ := json.Marshal(def.SectionConfig)
					cat.SectionConfig = sc
				}
			}
		}
		// Idempotency: a row with this GUID or unique name may already exist
		// from a prior partial apply that was retried. Reuse it rather than
		// hitting the unique constraint. Name is the fallback for archives
		// that shipped no GUID (schema defs off at export).
		if id, found := s.findSchemaDefByGUID(&models.Category{}, cat.GUID); found {
			s.idMap[entry.DecisionKey] = id
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = id
			}
			continue
		}
		if id, found := s.findSchemaDefByName(&models.Category{}, cat.Name); found {
			s.idMap[entry.DecisionKey] = id
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = id
			}
			continue
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
		if ok && action.Action == "guid_rename" {
			if action.RenameTo == "" {
				return fmt.Errorf("guid_rename action for %s requires rename_to", entry.DecisionKey)
			}
			if err := s.ctx.db.Model(&models.NoteType{}).
				Where("id = ?", entry.GUIDMatchID).
				Update("name", action.RenameTo).Error; err != nil {
				return fmt.Errorf("guid_rename note type %s: %w", entry.DecisionKey, err)
			}
			s.idMap[entry.DecisionKey] = entry.GUIDMatchID
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = entry.GUIDMatchID
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
				if def.GUID != "" {
					guid := def.GUID
					nt.GUID = &guid
				}
				if def.SectionConfig != nil {
					sc, _ := json.Marshal(def.SectionConfig)
					nt.SectionConfig = sc
				}
			}
		}
		// Idempotency: reuse an existing GUID-matched or name-matched row
		// on retry. NoteType has no unique-name index so the bug from
		// missing this fallback is silent duplication rather than a
		// constraint failure; resolveNoteTypeID would then repoint notes
		// at the duplicate.
		if id, found := s.findSchemaDefByGUID(&models.NoteType{}, nt.GUID); found {
			s.idMap[entry.DecisionKey] = id
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = id
			}
			continue
		}
		if id, found := s.findSchemaDefByName(&models.NoteType{}, nt.Name); found {
			s.idMap[entry.DecisionKey] = id
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = id
			}
			continue
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
		if ok && action.Action == "guid_rename" {
			if action.RenameTo == "" {
				return fmt.Errorf("guid_rename action for %s requires rename_to", entry.DecisionKey)
			}
			if err := s.ctx.db.Model(&models.ResourceCategory{}).
				Where("id = ?", entry.GUIDMatchID).
				Update("name", action.RenameTo).Error; err != nil {
				return fmt.Errorf("guid_rename resource category %s: %w", entry.DecisionKey, err)
			}
			s.idMap[entry.DecisionKey] = entry.GUIDMatchID
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = entry.GUIDMatchID
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
				if def.GUID != "" {
					guid := def.GUID
					rc.GUID = &guid
				}
				if def.SectionConfig != nil {
					sc, _ := json.Marshal(def.SectionConfig)
					rc.SectionConfig = sc
				}
			}
		}
		// Idempotency: reuse an existing GUID- or name-matched row on retry.
		if id, found := s.findSchemaDefByGUID(&models.ResourceCategory{}, rc.GUID); found {
			s.idMap[entry.DecisionKey] = id
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = id
			}
			continue
		}
		if id, found := s.findSchemaDefByName(&models.ResourceCategory{}, rc.Name); found {
			s.idMap[entry.DecisionKey] = id
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = id
			}
			continue
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
		if ok && action.Action == "guid_rename" {
			if action.RenameTo == "" {
				return fmt.Errorf("guid_rename action for %s requires rename_to", entry.DecisionKey)
			}
			if err := s.ctx.db.Model(&models.Tag{}).
				Where("id = ?", entry.GUIDMatchID).
				Update("name", action.RenameTo).Error; err != nil {
				return fmt.Errorf("guid_rename tag %s: %w", entry.DecisionKey, err)
			}
			s.idMap[entry.DecisionKey] = entry.GUIDMatchID
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = entry.GUIDMatchID
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
				if def.GUID != "" {
					guid := def.GUID
					tag.GUID = &guid
				}
				if def.Meta != nil {
					m, _ := json.Marshal(def.Meta)
					tag.Meta = m
				}
			}
		}
		// Idempotency: reuse an existing GUID- or name-matched row on retry.
		if id, found := s.findSchemaDefByGUID(&models.Tag{}, tag.GUID); found {
			s.idMap[entry.DecisionKey] = id
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = id
			}
			continue
		}
		if id, found := s.findSchemaDefByName(&models.Tag{}, tag.Name); found {
			s.idMap[entry.DecisionKey] = id
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = id
			}
			continue
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
		if ok && action.Action == "guid_rename" {
			if action.RenameTo == "" {
				return fmt.Errorf("guid_rename action for %s requires rename_to", entry.DecisionKey)
			}
			if err := s.ctx.db.Model(&models.GroupRelationType{}).
				Where("id = ?", entry.GUIDMatchID).
				Update("name", action.RenameTo).Error; err != nil {
				return fmt.Errorf("guid_rename group relation type %s: %w", entry.DecisionKey, err)
			}
			s.idMap[entry.DecisionKey] = entry.GUIDMatchID
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = entry.GUIDMatchID
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
				if def.GUID != "" {
					guid := def.GUID
					grt.GUID = &guid
				}
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
		// Idempotency: reuse an existing GUID- or composite-key matched
		// row on retry. GRT's unique constraint is composite
		// (name + from_category_id + to_category_id), so name alone is
		// insufficient.
		if id, found := s.findSchemaDefByGUID(&models.GroupRelationType{}, grt.GUID); found {
			s.idMap[entry.DecisionKey] = id
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = id
			}
			continue
		}
		if id, found := s.findGRTByCompositeKey(&grt); found {
			s.idMap[entry.DecisionKey] = id
			if entry.SourceExportID != "" {
				s.idMap[entry.SourceExportID] = id
			}
			continue
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
		// Idempotency: a row with this slug may already exist from a
		// prior partial apply that was retried. Reuse its ID rather
		// than hitting the unique slug constraint.
		var existing models.Series
		if err := s.ctx.db.Where("slug = ?", series.Slug).First(&existing).Error; err == nil {
			s.idMap[sm.ExportID] = existing.ID
			continue
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

			// Shell group handling: check ShellGroupActions
			if gp.Shell && s.decisions.ShellGroupActions != nil {
				if action, ok := s.decisions.ShellGroupActions[item.ExportID]; ok {
					if action.Action == "map_to_existing" && action.DestinationID != nil {
						s.idMap[item.ExportID] = *action.DestinationID
						s.result.MappedShellGroups++
						if err := walk(item.Children); err != nil {
							return err
						}
						continue
					}
				}
			}

			// GUID collision handling
			if gp.GUID != "" {
				var existing models.Group
				if err := s.ctx.db.Where("guid = ?", gp.GUID).First(&existing).Error; err == nil {
					s.idMap[item.ExportID] = existing.ID

					switch s.decisions.GUIDCollisionPolicy {
					case "skip":
						s.skippedM2M[item.ExportID] = true
						if err := walk(item.Children); err != nil {
							return err
						}
						continue
					case "merge":
						if err := s.mergeGroup(&existing, gp); err != nil {
							return fmt.Errorf("merge group %q: %w", gp.Name, err)
						}
					case "replace":
						if err := s.replaceGroup(&existing, gp); err != nil {
							return fmt.Errorf("replace group %q: %w", gp.Name, err)
						}
					}

					if err := walk(item.Children); err != nil {
						return err
					}
					continue
				}
			}

			g := models.Group{
				Name:        gp.Name,
				Description: gp.Description,
			}
			if gp.GUID != "" {
				guid := gp.GUID
				g.GUID = &guid
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
			if gp.Shell {
				s.result.CreatedShellGroups++
			}

			// Recurse into children
			if err := walk(item.Children); err != nil {
				return err
			}
		}
		return nil
	}

	return walk(s.plan.Items)
}

// pendingBlobCopy describes a blob copy that must run AFTER the surrounding
// transaction commits, so a rolled-back tx never leaves a partially written
// or truncated file behind. The destination is resolved through the
// resource's storage backend (alt-fs aware), not bare ctx.fs.
type pendingBlobCopy struct {
	src             string
	dst             string
	storageLocation *string
	resourceName    string
}

// batchAccumulator buffers result additions within a batch transaction. Only
// merged into the main ImportApplyResult after a successful tx.Commit(), so a
// rolled-back batch never leaks uncommitted IDs into the persisted cleanup payload.
type batchAccumulator struct {
	createdResources   int
	createdResourceIDs []uint
	createdVersions    int
	createdPreviews    int
	createdNotes       int
	createdNoteIDs     []uint
	pendingBlobCopies  []pendingBlobCopy
}

func (b *batchAccumulator) mergeInto(r *ImportApplyResult) {
	r.CreatedResources += b.createdResources
	r.CreatedResourceIDs = append(r.CreatedResourceIDs, b.createdResourceIDs...)
	r.CreatedVersions += b.createdVersions
	r.CreatedPreviews += b.createdPreviews
	r.CreatedNotes += b.createdNotes
	r.CreatedNoteIDs = append(r.CreatedNoteIDs, b.createdNoteIDs...)
}

// performPendingBlobCopies runs the deferred blob copies for a successfully
// committed batch. The DB rows are already committed by the time we get here,
// so any failure leaves DB and disk out of sync. Returning an error makes the
// import fail loudly, signalling the user to re-run (the GUID-replace path is
// idempotent). copyBlob itself writes via temp+rename so a mid-write failure
// never corrupts the live file.
func (s *applyState) performPendingBlobCopies(copies []pendingBlobCopy) error {
	for _, c := range copies {
		if err := s.copyBlob(c); err != nil {
			return fmt.Errorf("post-commit blob copy for resource %q failed (DB metadata already updated; fix the storage backend and re-apply the import — the GUID-replace path is idempotent and will finish the blob copy): %w", c.resourceName, err)
		}
	}
	return nil
}

// importTempBlobSuffix is appended to the destination path while staging a
// replacement blob. The final `Rename` is atomic on POSIX filesystems and on
// Afero's MemMapFs, so the live file is never observed mid-write.
const importTempBlobSuffix = ".mrimport.tmp"

// copyBlob stages the new blob bytes on the resource's actual storage
// backend (alt-fs aware) and atomically renames them into place. Any failure
// — including a missing alt fs, a disk-full mid-write, or a failed Close —
// leaves the live file untouched.
func (s *applyState) copyBlob(c pendingBlobCopy) error {
	dstFs, err := s.ctx.GetFsForStorageLocation(c.storageLocation)
	if err != nil {
		return fmt.Errorf("resolve storage backend: %w", err)
	}

	srcFile, err := s.ctx.fs.Open(c.src)
	if err != nil {
		return fmt.Errorf("open replacement blob: %w", err)
	}
	defer srcFile.Close()

	tmpPath := c.dst + importTempBlobSuffix
	dstFile, err := dstFs.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create staging blob %q: %w", tmpPath, err)
	}

	if _, copyErr := io.Copy(dstFile, srcFile); copyErr != nil {
		dstFile.Close()
		_ = dstFs.Remove(tmpPath)
		return fmt.Errorf("write staging blob: %w", copyErr)
	}
	if closeErr := dstFile.Close(); closeErr != nil {
		_ = dstFs.Remove(tmpPath)
		return fmt.Errorf("close staging blob: %w", closeErr)
	}

	if err := dstFs.Rename(tmpPath, c.dst); err != nil {
		_ = dstFs.Remove(tmpPath)
		return fmt.Errorf("rename staging blob %q -> %q: %w", tmpPath, c.dst, err)
	}
	return nil
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

		// Buffer per-batch result additions so a rolled-back batch doesn't
		// leak uncommitted IDs into the persisted cleanup payload.
		batchResult := &batchAccumulator{}

		tx := s.ctx.db.Begin()
		for _, entry := range batch {
			if err := s.applyOneResource(tx, entry.exportID, entry.payload, batchResult); err != nil {
				tx.Rollback()
				return fmt.Errorf("batch %d, resource %s: %w", batchNum, entry.exportID, err)
			}
		}
		if err := tx.Commit().Error; err != nil {
			return fmt.Errorf("commit batch %d: %w", batchNum, err)
		}

		// Merge only after successful commit.
		batchResult.mergeInto(s.result)

		// Now that the tx is durable, run any deferred blob copies. Any
		// failure here leaves DB and on-disk state out of sync — fail the
		// import so the user is signalled to retry (the GUID-replace path
		// is idempotent).
		if err := s.performPendingBlobCopies(batchResult.pendingBlobCopies); err != nil {
			return fmt.Errorf("batch %d: %w", batchNum, err)
		}

		s.sink.SetPhaseProgress(int64(batchEnd), int64(len(entries)))
	}
	return nil
}

// applyOneResource creates a single resource row plus its versions, wires
// CurrentVersionID, and creates preview rows — all within the caller's tx.
func (s *applyState) applyOneResource(tx *gorm.DB, exportID string, rp *archive.ResourcePayload, batch *batchAccumulator) error {
	// (a0) GUID collision takes precedence over hash collision
	if rp.GUID != "" {
		var existing models.Resource
		if err := tx.Where("guid = ?", rp.GUID).First(&existing).Error; err == nil {
			s.idMap[exportID] = existing.ID

			switch s.decisions.GUIDCollisionPolicy {
			case "skip":
				s.skippedM2M[exportID] = true
				return nil
			case "merge":
				return s.mergeResource(tx, &existing, rp)
			case "replace":
				return s.replaceResource(tx, &existing, rp, batch)
			default:
				return s.mergeResource(tx, &existing, rp)
			}
		}
	}

	// (a) Skip-on-hash collision
	if rp.Hash != "" {
		var existing models.Resource
		if err := tx.Where("hash = ?", rp.Hash).First(&existing).Error; err == nil {
			if s.decisions.ResourceCollisionPolicy == "skip" {
				s.idMap[exportID] = existing.ID
				s.skippedM2M[exportID] = true
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
			s.skippedM2M[exportID] = true
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
	if rp.GUID != "" {
		guid := rp.GUID
		r.GUID = &guid
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
	batch.createdResources++
	batch.createdResourceIDs = append(batch.createdResourceIDs, r.ID)

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
		batch.createdVersions++
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
		batch.createdPreviews++
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
// mergeResource updates an existing resource's non-blob scalar fields (incoming wins),
// deep-merges meta/ownMeta, resolves owner and resource category, and unions M2M tags.
// Blob-derived metadata (ContentType, Width, Height, FileSize, ContentCategory) and
// blob-coupled fields (Hash, Location, blob bytes, versions, previews) are NOT updated —
// they stay in sync with the existing blob.
func (s *applyState) mergeResource(tx *gorm.DB, existing *models.Resource, rp *archive.ResourcePayload) error {
	// Non-blob scalars: incoming wins
	updates := map[string]any{
		"name":              rp.Name,
		"original_name":     rp.OriginalName,
		"original_location": rp.OriginalLocation,
		"description":       rp.Description,
		"category":          rp.Category,
		"updated_at":        time.Now(),
	}

	// ResourceCategory
	if rp.ResourceCategoryRef != "" {
		if rcID, ok := s.idMap[rp.ResourceCategoryRef]; ok {
			updates["resource_category_id"] = rcID
		}
	} else if rp.ResourceCategoryName != "" {
		rcKey := DecisionKeyFor("resource_category", MappingEntry{SourceKey: rp.ResourceCategoryName})
		if rcID, ok := s.idMap[rcKey]; ok {
			updates["resource_category_id"] = rcID
		}
	}

	// Owner
	if rp.OwnerRef != "" {
		if ownerID, ok := s.idMap[rp.OwnerRef]; ok {
			updates["owner_id"] = ownerID
		}
	}

	// OwnMeta: deep merge
	if rp.OwnMeta != nil {
		existingOwnMeta := jsonToMap(existing.OwnMeta)
		merged := types.DeepMergeJSON(existingOwnMeta, rp.OwnMeta)
		m, _ := json.Marshal(merged)
		updates["own_meta"] = types.JSON(m)
	}

	// Meta: deep merge
	if rp.Meta != nil {
		existingMeta := jsonToMap(existing.Meta)
		merged := types.DeepMergeJSON(existingMeta, rp.Meta)
		m, _ := json.Marshal(merged)
		updates["meta"] = types.JSON(m)
	}

	// Blob-derived metadata (ContentType, Width, Height, FileSize, ContentCategory):
	// NOT updated — stays in sync with kept blob.

	// Blob-coupled fields (Hash, Location, etc.): NOT updated.
	if rp.Hash != "" && rp.Hash != existing.Hash {
		s.result.Warnings = append(s.result.Warnings,
			fmt.Sprintf("Resource %q: GUID merge kept existing blob (hash %s), incoming has different hash %s", rp.Name, existing.Hash, rp.Hash))
	}

	if err := tx.Model(existing).Updates(updates).Error; err != nil {
		return err
	}

	// M2M: union tags
	for _, tr := range rp.Tags {
		tagID := s.resolveTagID(tr)
		if tagID == 0 {
			continue
		}
		tx.Exec(
			"INSERT INTO resource_tags (resource_id, tag_id) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM resource_tags WHERE resource_id = ? AND tag_id = ?)",
			existing.ID, tagID, existing.ID, tagID,
		)
	}

	return nil
}

// replaceResource overwrites an existing resource's non-blob scalar fields and meta.
// When the archive contains a blob for this resource, blob-derived metadata and
// blob-coupled fields (Hash, Location, blob bytes, versions, previews) are also replaced.
// When no blob is present in the archive, those fields are kept and a warning is emitted.
// Tags are cleared and reset to the incoming set.
func (s *applyState) replaceResource(tx *gorm.DB, existing *models.Resource, rp *archive.ResourcePayload, batch *batchAccumulator) error {
	hasBlobInArchive := s.blobPaths[rp.Hash] != ""

	// Non-blob scalars always updated
	updates := map[string]any{
		"name":              rp.Name,
		"original_name":     rp.OriginalName,
		"original_location": rp.OriginalLocation,
		"description":       rp.Description,
		"category":          rp.Category,
		"updated_at":        time.Now(),
	}

	// ResourceCategory
	if rp.ResourceCategoryRef != "" {
		if rcID, ok := s.idMap[rp.ResourceCategoryRef]; ok {
			updates["resource_category_id"] = rcID
		}
	} else if rp.ResourceCategoryName != "" {
		rcKey := DecisionKeyFor("resource_category", MappingEntry{SourceKey: rp.ResourceCategoryName})
		if rcID, ok := s.idMap[rcKey]; ok {
			updates["resource_category_id"] = rcID
		}
	}

	// Owner
	if rp.OwnerRef != "" {
		if ownerID, ok := s.idMap[rp.OwnerRef]; ok {
			updates["owner_id"] = ownerID
		}
	}

	// Meta: incoming replaces entirely
	if rp.Meta != nil {
		m, _ := json.Marshal(rp.Meta)
		updates["meta"] = types.JSON(m)
	}
	if rp.OwnMeta != nil {
		m, _ := json.Marshal(rp.OwnMeta)
		updates["own_meta"] = types.JSON(m)
	}

	if hasBlobInArchive {
		// Full replace: blob-derived metadata + blob-coupled fields
		updates["hash"] = rp.Hash
		updates["hash_type"] = rp.HashType
		updates["file_size"] = rp.FileSize
		updates["content_type"] = rp.ContentType
		updates["content_category"] = rp.ContentCategory
		updates["width"] = rp.Width
		updates["height"] = rp.Height

		// Defer the blob copy until after the tx commits so a rollback never
		// leaves the existing file truncated or half-written. Resolve the
		// destination filesystem from the resource's storage backend so
		// attached (alt-fs) resources are written through the right Afero fs.
		blobSrc := s.blobPaths[rp.Hash]
		if existing.Location != "" && blobSrc != "" {
			batch.pendingBlobCopies = append(batch.pendingBlobCopies, pendingBlobCopy{
				src:             blobSrc,
				dst:             existing.GetCleanLocation(),
				storageLocation: existing.StorageLocation,
				resourceName:    rp.Name,
			})
		}

		// Delete existing versions and previews
		tx.Where("resource_id = ?", existing.ID).Delete(&models.ResourceVersion{})
		tx.Where("resource_id = ?", existing.ID).Delete(&models.Preview{})

		// Track replaced resource so callers can find it (same semantics as createdResourceIDs)
		batch.createdResourceIDs = append(batch.createdResourceIDs, existing.ID)

		// Create incoming versions
		for _, vp := range rp.Versions {
			vLoc := s.blobPaths[vp.Hash]
			if vLoc == "" {
				vLoc = importBlobLocation(vp.Hash, vp.ContentType)
			}
			ver := models.ResourceVersion{
				ResourceID:    existing.ID,
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
			batch.createdVersions++
		}

		// Wire CurrentVersionID
		if rp.CurrentVersionRef != "" {
			if cvID, ok := s.idMap[rp.CurrentVersionRef]; ok {
				if err := tx.Model(existing).Update("current_version_id", cvID).Error; err != nil {
					return fmt.Errorf("wire current_version_id: %w", err)
				}
			}
		}

		// Create incoming previews
		for _, pp := range rp.Previews {
			data, ok := s.previewData[pp.PreviewExportID]
			if !ok || len(data) == 0 {
				continue
			}
			preview := models.Preview{
				ResourceId:  &existing.ID,
				Data:        data,
				Width:       pp.Width,
				Height:      pp.Height,
				ContentType: pp.ContentType,
			}
			if err := tx.Create(&preview).Error; err != nil {
				return fmt.Errorf("create preview %s: %w", pp.PreviewExportID, err)
			}
			delete(s.previewData, pp.PreviewExportID)
			batch.createdPreviews++
		}
	} else {
		// No blob in archive — keep existing file, warn
		s.result.Warnings = append(s.result.Warnings,
			fmt.Sprintf("Resource %q: blob not present in archive, keeping existing file", rp.Name))
	}

	if err := tx.Model(existing).Updates(updates).Error; err != nil {
		return err
	}

	// M2M: clear existing, set incoming
	tx.Exec("DELETE FROM resource_tags WHERE resource_id = ?", existing.ID)
	for _, tr := range rp.Tags {
		tagID := s.resolveTagID(tr)
		if tagID == 0 {
			continue
		}
		tx.Exec(
			"INSERT INTO resource_tags (resource_id, tag_id) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM resource_tags WHERE resource_id = ? AND tag_id = ?)",
			existing.ID, tagID, existing.ID, tagID,
		)
	}

	return nil
}

// noteEntry pairs an export ID with its note payload for ordered iteration.
type noteEntry struct {
	exportID string
	payload  *archive.NotePayload
}

// applyNotes creates note rows in batches of applyBatchSize. Each batch
// runs inside a single DB transaction covering the note row and its blocks.
func (s *applyState) applyNotes() error {
	// Build ordered list, excluding notes whose owner was excluded or unmapped.
	var entries []noteEntry
	for exportID, np := range s.collector.notes {
		if s.isExcluded(exportID) {
			continue
		}
		// Owner must be resolvable (mapped or absent). If OwnerRef is set but
		// not in idMap, the owner group was excluded — skip this note.
		if np.OwnerRef != "" {
			if _, ok := s.idMap[np.OwnerRef]; !ok {
				continue
			}
		}
		entries = append(entries, noteEntry{exportID: exportID, payload: np})
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

		batchResult := &batchAccumulator{}

		tx := s.ctx.db.Begin()
		for _, entry := range batch {
			if err := s.applyOneNote(tx, entry.exportID, entry.payload, batchResult); err != nil {
				tx.Rollback()
				return fmt.Errorf("batch %d, note %s: %w", batchNum, entry.exportID, err)
			}
		}
		if err := tx.Commit().Error; err != nil {
			return fmt.Errorf("commit note batch %d: %w", batchNum, err)
		}

		batchResult.mergeInto(s.result)

		s.sink.SetPhaseProgress(int64(batchEnd), int64(len(entries)))
	}
	return nil
}

// applyOneNote creates a single note row plus its blocks within the caller's tx.
func (s *applyState) applyOneNote(tx *gorm.DB, exportID string, np *archive.NotePayload, batch *batchAccumulator) error {
	// GUID collision handling
	if np.GUID != "" {
		var existing models.Note
		if err := tx.Where("guid = ?", np.GUID).First(&existing).Error; err == nil {
			s.idMap[exportID] = existing.ID

			switch s.decisions.GUIDCollisionPolicy {
			case "skip":
				s.skippedM2M[exportID] = true
				return nil
			case "merge":
				if err := s.mergeNote(tx, &existing, np); err != nil {
					return fmt.Errorf("merge note %q: %w", np.Name, err)
				}
			case "replace":
				if err := s.replaceNote(tx, &existing, np); err != nil {
					return fmt.Errorf("replace note %q: %w", np.Name, err)
				}
			}
			return nil
		}
	}

	n := models.Note{
		Name:        np.Name,
		Description: np.Description,
		CreatedAt:   np.CreatedAt,
		UpdatedAt:   np.UpdatedAt,
		StartDate:   np.StartDate,
		EndDate:     np.EndDate,
	}
	if np.GUID != "" {
		guid := np.GUID
		n.GUID = &guid
	}

	// Resolve OwnerRef
	if np.OwnerRef != "" {
		if ownerID, ok := s.idMap[np.OwnerRef]; ok {
			n.OwnerId = &ownerID
		}
	}

	// Resolve NoteType: try NoteTypeRef first, then NoteTypeName lookup
	if np.NoteTypeRef != "" {
		if ntID, ok := s.idMap[np.NoteTypeRef]; ok {
			n.NoteTypeId = &ntID
		}
	}
	if n.NoteTypeId == nil && np.NoteTypeName != "" {
		ntKey := DecisionKeyFor("note_type", MappingEntry{SourceKey: np.NoteTypeName})
		if ntID, ok := s.idMap[ntKey]; ok {
			n.NoteTypeId = &ntID
		}
	}

	// Marshal Meta
	if np.Meta != nil {
		m, err := json.Marshal(np.Meta)
		if err != nil {
			return fmt.Errorf("marshal note meta: %w", err)
		}
		n.Meta = types.JSON(m)
	}

	if err := tx.Create(&n).Error; err != nil {
		return fmt.Errorf("create note %q: %w", n.Name, err)
	}

	s.idMap[exportID] = n.ID
	batch.createdNotes++
	batch.createdNoteIDs = append(batch.createdNoteIDs, n.ID)

	// Create NoteBlock rows
	for _, bp := range np.Blocks {
		block := models.NoteBlock{
			NoteID:   n.ID,
			Type:     bp.Type,
			Position: bp.Position,
		}

		// Content defaults to {} if nil
		if bp.Content != nil {
			c, err := json.Marshal(bp.Content)
			if err != nil {
				return fmt.Errorf("marshal block content: %w", err)
			}
			block.Content = types.JSON(c)
		} else {
			block.Content = types.JSON([]byte("{}"))
		}

		// State defaults to {} if nil
		if bp.State != nil {
			st, err := json.Marshal(bp.State)
			if err != nil {
				return fmt.Errorf("marshal block state: %w", err)
			}
			block.State = types.JSON(st)
		} else {
			block.State = types.JSON([]byte("{}"))
		}

		if err := tx.Create(&block).Error; err != nil {
			return fmt.Errorf("create note block (note %q, pos %s): %w", n.Name, bp.Position, err)
		}
	}

	return nil
}

// applyM2MLinks wires all many-to-many relationships for groups, resources,
// and notes. Tags, RelatedGroups/Resources/Notes, and typed GroupRelation rows.
func (s *applyState) applyM2MLinks() error {
	// --- Groups: Tags, RelatedGroups, RelatedResources, RelatedNotes, GroupRelations ---
	for exportID, gp := range s.collector.groups {
		if s.skippedM2M[exportID] {
			continue // skip policy: existing row must not be modified
		}
		destID, ok := s.idMap[exportID]
		if !ok {
			continue // group was excluded or not created
		}
		group := models.Group{ID: destID}

		// Tags
		tagIDs := s.resolveTagIDs(gp.Tags)
		if len(tagIDs) > 0 {
			tags := BuildAssociationSlicePtr(tagIDs, TagPtrFromID)
			if err := s.ctx.db.Model(&group).Association("Tags").Append(tags); err != nil {
				return fmt.Errorf("group %s tags: %w", exportID, err)
			}
		}

		// RelatedGroups
		relGroupIDs := s.resolveRefIDs(gp.RelatedGroups)
		if len(relGroupIDs) > 0 {
			groups := BuildAssociationSlicePtr(relGroupIDs, GroupPtrFromID)
			if err := s.ctx.db.Model(&group).Association("RelatedGroups").Append(groups); err != nil {
				return fmt.Errorf("group %s related groups: %w", exportID, err)
			}
		}

		// RelatedResources
		relResIDs := s.resolveRefIDs(gp.RelatedResources)
		if len(relResIDs) > 0 {
			resources := BuildAssociationSlicePtr(relResIDs, ResourcePtrFromID)
			if err := s.ctx.db.Model(&group).Association("RelatedResources").Append(resources); err != nil {
				return fmt.Errorf("group %s related resources: %w", exportID, err)
			}
		}

		// RelatedNotes
		relNoteIDs := s.resolveRefIDs(gp.RelatedNotes)
		if len(relNoteIDs) > 0 {
			notes := BuildAssociationSlicePtr(relNoteIDs, NotePtrFromID)
			if err := s.ctx.db.Model(&group).Association("RelatedNotes").Append(notes); err != nil {
				return fmt.Errorf("group %s related notes: %w", exportID, err)
			}
		}

		// GroupRelation rows (typed relationships with ToRef in scope)
		for _, rel := range gp.Relationships {
			if rel.ToRef == "" {
				continue // dangling — handled by applyDanglingDecisions
			}
			toID, ok := s.idMap[rel.ToRef]
			if !ok {
				continue // target not created
			}
			grtID := s.resolveGRTID(rel)
			if grtID == 0 {
				continue // relation type not resolved
			}
			gr := models.GroupRelation{
				FromGroupId:    &destID,
				ToGroupId:      &toID,
				RelationTypeId: &grtID,
				Name:           rel.Name,
				Description:    rel.Description,
			}
			if err := s.ctx.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&gr).Error; err != nil {
				return fmt.Errorf("group %s relation to %s: %w", exportID, rel.ToRef, err)
			}
		}
	}

	// --- Resources: Tags, Groups, Notes ---
	for exportID, rp := range s.collector.resources {
		if s.skippedM2M[exportID] {
			continue // skip / hash-skip / missing-bytes policy: leave existing rows alone
		}
		destID, ok := s.idMap[exportID]
		if !ok {
			continue
		}
		resource := models.Resource{ID: destID}

		// Tags
		tagIDs := s.resolveTagIDs(rp.Tags)
		if len(tagIDs) > 0 {
			tags := BuildAssociationSlicePtr(tagIDs, TagPtrFromID)
			if err := s.ctx.db.Model(&resource).Association("Tags").Append(tags); err != nil {
				return fmt.Errorf("resource %s tags: %w", exportID, err)
			}
		}

		// Groups (m2m)
		groupIDs := s.resolveRefIDs(rp.Groups)
		if len(groupIDs) > 0 {
			groups := BuildAssociationSlicePtr(groupIDs, GroupPtrFromID)
			if err := s.ctx.db.Model(&resource).Association("Groups").Append(groups); err != nil {
				return fmt.Errorf("resource %s groups: %w", exportID, err)
			}
		}

		// Notes (m2m)
		noteIDs := s.resolveRefIDs(rp.Notes)
		if len(noteIDs) > 0 {
			notes := BuildAssociationSlicePtr(noteIDs, NotePtrFromID)
			if err := s.ctx.db.Model(&resource).Association("Notes").Append(notes); err != nil {
				return fmt.Errorf("resource %s notes: %w", exportID, err)
			}
		}
	}

	// --- Notes: Tags, Resources, Groups ---
	for exportID, np := range s.collector.notes {
		if s.skippedM2M[exportID] {
			continue // skip policy: leave existing row alone
		}
		destID, ok := s.idMap[exportID]
		if !ok {
			continue
		}
		note := models.Note{ID: destID}

		// Tags
		tagIDs := s.resolveTagIDs(np.Tags)
		if len(tagIDs) > 0 {
			tags := BuildAssociationSlicePtr(tagIDs, TagPtrFromID)
			if err := s.ctx.db.Model(&note).Association("Tags").Append(tags); err != nil {
				return fmt.Errorf("note %s tags: %w", exportID, err)
			}
		}

		// Resources (m2m)
		resIDs := s.resolveRefIDs(np.Resources)
		if len(resIDs) > 0 {
			resources := BuildAssociationSlicePtr(resIDs, ResourcePtrFromID)
			if err := s.ctx.db.Model(&note).Association("Resources").Append(resources); err != nil {
				return fmt.Errorf("note %s resources: %w", exportID, err)
			}
		}

		// Groups (m2m)
		groupIDs := s.resolveRefIDs(np.Groups)
		if len(groupIDs) > 0 {
			groups := BuildAssociationSlicePtr(groupIDs, GroupPtrFromID)
			if err := s.ctx.db.Model(&note).Association("Groups").Append(groups); err != nil {
				return fmt.Errorf("note %s groups: %w", exportID, err)
			}
		}
	}

	return nil
}

// resolveRefIDs converts a slice of export ID references to destination DB IDs.
// References that are not in the idMap (excluded or not created) are skipped.
func (s *applyState) resolveRefIDs(refs []string) []uint {
	ids := make([]uint, 0, len(refs))
	for _, ref := range refs {
		if destID, ok := s.idMap[ref]; ok {
			ids = append(ids, destID)
		}
	}
	return ids
}

// resolveGRTID resolves the GroupRelationType ID for a relationship payload.
// Tries TypeRef -> idMap first, fallback to composite decision key.
func (s *applyState) resolveGRTID(rel archive.GroupRelationPayload) uint {
	if rel.TypeRef != "" {
		if id, ok := s.idMap[rel.TypeRef]; ok {
			return id
		}
	}
	// Composite key fallback: grt:TypeName|FromCategoryName|ToCategoryName
	if rel.TypeName != "" {
		key := DecisionKeyFor("grt", MappingEntry{
			SourceKey:        rel.TypeName,
			FromCategoryName: rel.FromCategoryName,
			ToCategoryName:   rel.ToCategoryName,
		})
		if id, ok := s.idMap[key]; ok {
			return id
		}
	}
	return 0
}

// applyDanglingDecisions processes dangling reference decisions (map or drop).
// For "map" actions, it creates the appropriate m2m link or GroupRelation row
// pointing to the user-chosen destination entity.
func (s *applyState) applyDanglingDecisions() error {
	for _, dr := range s.plan.DanglingRefs {
		action, ok := s.decisions.DanglingActions[dr.ID]
		if !ok || action.Action == "drop" {
			continue
		}
		// action.Action == "map"
		fromID, ok := s.idMap[dr.FromExportID]
		if !ok {
			continue // source entity was excluded
		}
		if action.DestinationID == nil {
			continue // no destination specified
		}
		destID := *action.DestinationID

		switch dr.Kind {
		case "related_group":
			group := models.Group{ID: fromID}
			target := &models.Group{ID: destID}
			if err := s.ctx.db.Model(&group).Association("RelatedGroups").Append(target); err != nil {
				return fmt.Errorf("dangling %s related_group %d->%d: %w", dr.ID, fromID, destID, err)
			}

		case "related_resource":
			group := models.Group{ID: fromID}
			target := &models.Resource{ID: destID}
			if err := s.ctx.db.Model(&group).Association("RelatedResources").Append(target); err != nil {
				return fmt.Errorf("dangling %s related_resource %d->%d: %w", dr.ID, fromID, destID, err)
			}

		case "related_note":
			group := models.Group{ID: fromID}
			target := &models.Note{ID: destID}
			if err := s.ctx.db.Model(&group).Association("RelatedNotes").Append(target); err != nil {
				return fmt.Errorf("dangling %s related_note %d->%d: %w", dr.ID, fromID, destID, err)
			}

		case "group_relation":
			grtID := s.resolveGRTForDanglingRef(dr)
			if grtID == 0 {
				s.result.Warnings = append(s.result.Warnings,
					fmt.Sprintf("dangling ref %s: could not resolve group relation type, skipping", dr.ID))
				continue
			}
			gr := models.GroupRelation{
				FromGroupId:    &fromID,
				ToGroupId:      &destID,
				RelationTypeId: &grtID,
			}
			if err := s.ctx.db.Clauses(clause.OnConflict{DoNothing: true}).Create(&gr).Error; err != nil {
				return fmt.Errorf("dangling %s group_relation %d->%d: %w", dr.ID, fromID, destID, err)
			}
		}
	}
	return nil
}

// resolveGRTForDanglingRef finds the GroupRelationType ID for a dangling
// group_relation reference by scanning the owning group's relationship
// payloads for the matching DanglingRef.
func (s *applyState) resolveGRTForDanglingRef(dr DanglingRefPlan) uint {
	fromGroup, ok := s.collector.groups[dr.FromExportID]
	if !ok {
		return 0
	}
	for _, rel := range fromGroup.Relationships {
		if rel.DanglingRef != dr.ID {
			continue
		}
		// Found the matching relationship — resolve its type
		if rel.TypeRef != "" {
			if id, ok := s.idMap[rel.TypeRef]; ok {
				return id
			}
		}
		// Composite key fallback
		if rel.TypeName != "" {
			key := DecisionKeyFor("grt", MappingEntry{
				SourceKey:        rel.TypeName,
				FromCategoryName: rel.FromCategoryName,
				ToCategoryName:   rel.ToCategoryName,
			})
			if id, ok := s.idMap[key]; ok {
				return id
			}
		}
		break
	}
	return 0
}

// --- GUID collision helpers ---

// resolveCategoryID looks up a category ID from CategoryRef or CategoryName.
func (s *applyState) resolveCategoryID(catRef, catName string) *uint {
	if catRef != "" {
		if catID, ok := s.idMap[catRef]; ok {
			return &catID
		}
	}
	if catName != "" {
		catKey := DecisionKeyFor("category", MappingEntry{SourceKey: catName})
		if catID, ok := s.idMap[catKey]; ok {
			return &catID
		}
	}
	return nil
}

// resolveTagID resolves a single TagRef to a destination DB tag ID.
func (s *applyState) resolveTagID(tr archive.TagRef) uint {
	if tr.Ref != "" {
		if id, ok := s.idMap[tr.Ref]; ok {
			return id
		}
	}
	tagKey := DecisionKeyFor("tag", MappingEntry{SourceKey: tr.Name})
	if id, ok := s.idMap[tagKey]; ok {
		return id
	}
	return 0
}

// unionGroupTags adds any tags from gp not already on the group (union semantics).
func (s *applyState) unionGroupTags(groupID uint, gp *archive.GroupPayload) error {
	for _, tr := range gp.Tags {
		tagID := s.resolveTagID(tr)
		if tagID == 0 {
			continue
		}
		s.ctx.db.Exec(
			"INSERT INTO group_tags (group_id, tag_id) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM group_tags WHERE group_id = ? AND tag_id = ?)",
			groupID, tagID, groupID, tagID,
		)
	}
	return nil
}

// mergeGroup updates an existing group's scalars (incoming wins), deep-merges
// meta, resolves owner and category, and unions tags.
func (s *applyState) mergeGroup(existing *models.Group, gp *archive.GroupPayload) error {
	updates := map[string]any{
		"name":        gp.Name,
		"description": gp.Description,
		"updated_at":  time.Now(),
	}
	if gp.URL != "" {
		parsed, err := url.Parse(gp.URL)
		if err == nil {
			u := types.URL(*parsed)
			updates["url"] = &u
		}
	}

	// Meta: deep merge (incoming keys overwrite, existing keys preserved)
	if gp.Meta != nil {
		existingMeta := jsonToMap(existing.Meta)
		merged := types.DeepMergeJSON(existingMeta, gp.Meta)
		m, _ := json.Marshal(merged)
		updates["meta"] = types.JSON(m)
	}

	// Owner
	if gp.OwnerRef != "" {
		if ownerID, ok := s.idMap[gp.OwnerRef]; ok {
			updates["owner_id"] = ownerID
		}
	}

	// Category
	catID := s.resolveCategoryID(gp.CategoryRef, gp.CategoryName)
	if catID != nil {
		updates["category_id"] = *catID
	}

	if err := s.ctx.db.Model(existing).Updates(updates).Error; err != nil {
		return err
	}

	return s.unionGroupTags(existing.ID, gp)
}

// replaceGroup overwrites an existing group's scalars and meta entirely and
// resets its tags to the incoming set.
func (s *applyState) replaceGroup(existing *models.Group, gp *archive.GroupPayload) error {
	updates := map[string]any{
		"name":        gp.Name,
		"description": gp.Description,
		"updated_at":  time.Now(),
	}
	if gp.URL != "" {
		parsed, err := url.Parse(gp.URL)
		if err == nil {
			u := types.URL(*parsed)
			updates["url"] = &u
		}
	} else {
		updates["url"] = nil
	}

	// Meta: incoming replaces entirely
	if gp.Meta != nil {
		m, _ := json.Marshal(gp.Meta)
		updates["meta"] = types.JSON(m)
	} else {
		updates["meta"] = types.JSON("null")
	}

	if gp.OwnerRef != "" {
		if ownerID, ok := s.idMap[gp.OwnerRef]; ok {
			updates["owner_id"] = ownerID
		}
	}

	catID := s.resolveCategoryID(gp.CategoryRef, gp.CategoryName)
	if catID != nil {
		updates["category_id"] = *catID
	}

	if err := s.ctx.db.Model(existing).Updates(updates).Error; err != nil {
		return err
	}

	// Tags: clear all existing, set incoming
	s.ctx.db.Exec("DELETE FROM group_tags WHERE group_id = ?", existing.ID)
	return s.unionGroupTags(existing.ID, gp)
}

// resolveNoteTypeID resolves a NoteType ID from NoteTypeRef or NoteTypeName.
func (s *applyState) resolveNoteTypeID(np *archive.NotePayload) *uint {
	if np.NoteTypeRef != "" {
		if id, ok := s.idMap[np.NoteTypeRef]; ok {
			return &id
		}
	}
	if np.NoteTypeName != "" {
		ntKey := DecisionKeyFor("note_type", MappingEntry{SourceKey: np.NoteTypeName})
		if id, ok := s.idMap[ntKey]; ok {
			return &id
		}
	}
	return nil
}

// unionNoteTags adds tags from np not already on the note (union semantics).
func (s *applyState) unionNoteTags(noteID uint, np *archive.NotePayload) {
	for _, tr := range np.Tags {
		tagID := s.resolveTagID(tr)
		if tagID == 0 {
			continue
		}
		s.ctx.db.Exec(
			"INSERT INTO note_tags (note_id, tag_id) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM note_tags WHERE note_id = ? AND tag_id = ?)",
			noteID, tagID, noteID, tagID,
		)
	}
}

// mergeNote updates an existing note's scalars (incoming wins), deep-merges
// meta, resolves NoteTypeId/OwnerId, and unions M2M (tags, resources, groups).
// Existing blocks are preserved.
func (s *applyState) mergeNote(tx *gorm.DB, existing *models.Note, np *archive.NotePayload) error {
	updates := map[string]any{
		"name":        np.Name,
		"description": np.Description,
		"updated_at":  time.Now(),
		"start_date":  np.StartDate,
		"end_date":    np.EndDate,
	}

	// Meta: deep merge
	if np.Meta != nil {
		existingMeta := jsonToMap(existing.Meta)
		merged := types.DeepMergeJSON(existingMeta, np.Meta)
		m, _ := json.Marshal(merged)
		updates["meta"] = types.JSON(m)
	}

	// Owner
	if np.OwnerRef != "" {
		if ownerID, ok := s.idMap[np.OwnerRef]; ok {
			updates["owner_id"] = ownerID
		}
	}

	// NoteType
	if ntID := s.resolveNoteTypeID(np); ntID != nil {
		updates["note_type_id"] = *ntID
	}

	if err := tx.Model(existing).Updates(updates).Error; err != nil {
		return err
	}

	// M2M: union tags
	s.unionNoteTags(existing.ID, np)

	// M2M: union resources
	for _, ref := range np.Resources {
		if resID, ok := s.idMap[ref]; ok {
			tx.Exec(
				"INSERT INTO resource_notes (resource_id, note_id) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM resource_notes WHERE resource_id = ? AND note_id = ?)",
				resID, existing.ID, resID, existing.ID,
			)
		}
	}

	// M2M: union groups
	for _, ref := range np.Groups {
		if grpID, ok := s.idMap[ref]; ok {
			tx.Exec(
				"INSERT INTO groups_related_notes (group_id, note_id) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM groups_related_notes WHERE group_id = ? AND note_id = ?)",
				grpID, existing.ID, grpID, existing.ID,
			)
		}
	}

	return nil
}

// replaceNote overwrites an existing note's scalars and meta, clears+resets
// M2M, and replaces blocks with the incoming set.
func (s *applyState) replaceNote(tx *gorm.DB, existing *models.Note, np *archive.NotePayload) error {
	updates := map[string]any{
		"name":        np.Name,
		"description": np.Description,
		"updated_at":  time.Now(),
		"start_date":  np.StartDate,
		"end_date":    np.EndDate,
	}

	// Meta: incoming replaces entirely
	if np.Meta != nil {
		m, _ := json.Marshal(np.Meta)
		updates["meta"] = types.JSON(m)
	} else {
		updates["meta"] = types.JSON("null")
	}

	// Owner
	if np.OwnerRef != "" {
		if ownerID, ok := s.idMap[np.OwnerRef]; ok {
			updates["owner_id"] = ownerID
		}
	}

	// NoteType
	if ntID := s.resolveNoteTypeID(np); ntID != nil {
		updates["note_type_id"] = *ntID
	}

	if err := tx.Model(existing).Updates(updates).Error; err != nil {
		return err
	}

	// Tags: clear all existing, set incoming
	tx.Exec("DELETE FROM note_tags WHERE note_id = ?", existing.ID)
	s.unionNoteTags(existing.ID, np)

	// Resources: clear all existing, set incoming
	tx.Exec("DELETE FROM resource_notes WHERE note_id = ?", existing.ID)
	for _, ref := range np.Resources {
		if resID, ok := s.idMap[ref]; ok {
			tx.Exec(
				"INSERT INTO resource_notes (resource_id, note_id) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM resource_notes WHERE resource_id = ? AND note_id = ?)",
				resID, existing.ID, resID, existing.ID,
			)
		}
	}

	// Groups: clear all existing, set incoming
	tx.Exec("DELETE FROM groups_related_notes WHERE note_id = ?", existing.ID)
	for _, ref := range np.Groups {
		if grpID, ok := s.idMap[ref]; ok {
			tx.Exec(
				"INSERT INTO groups_related_notes (group_id, note_id) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM groups_related_notes WHERE group_id = ? AND note_id = ?)",
				grpID, existing.ID, grpID, existing.ID,
			)
		}
	}

	// Blocks: delete existing, create incoming
	if err := tx.Where("note_id = ?", existing.ID).Delete(&models.NoteBlock{}).Error; err != nil {
		return fmt.Errorf("delete existing blocks: %w", err)
	}
	for _, bp := range np.Blocks {
		block := models.NoteBlock{
			NoteID:   existing.ID,
			Type:     bp.Type,
			Position: bp.Position,
		}
		if bp.Content != nil {
			c, err := json.Marshal(bp.Content)
			if err != nil {
				return fmt.Errorf("marshal block content: %w", err)
			}
			block.Content = types.JSON(c)
		} else {
			block.Content = types.JSON([]byte("{}"))
		}
		if bp.State != nil {
			st, err := json.Marshal(bp.State)
			if err != nil {
				return fmt.Errorf("marshal block state: %w", err)
			}
			block.State = types.JSON(st)
		} else {
			block.State = types.JSON([]byte("{}"))
		}
		if err := tx.Create(&block).Error; err != nil {
			return fmt.Errorf("create replaced note block (note %q, pos %s): %w", np.Name, bp.Position, err)
		}
	}

	return nil
}

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
