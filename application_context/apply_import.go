package application_context

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path"
	"path/filepath"

	"github.com/spf13/afero"
	"mahresources/archive"
	"mahresources/download_queue"
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

// Stubs — implemented in subsequent tasks. Each returns nil so the test
// compiles and runs (it will fail on assertions, not compilation).
func (s *applyState) applySchemaDefDecisions() error { return nil }
func (s *applyState) applySeries() error             { return nil }
func (s *applyState) applyGroups() error             { return nil }
func (s *applyState) applyResources() error          { return nil }
func (s *applyState) applyNotes() error              { return nil }
func (s *applyState) applyM2MLinks() error           { return nil }
func (s *applyState) applyDanglingDecisions() error  { return nil }
