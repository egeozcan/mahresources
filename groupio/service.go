// Package groupio owns the group export and import subsystem: planning an
// export, streaming it to a tar, parsing an import tar into a plan, and
// applying that plan.
//
// It is deliberately stateless with respect to the database handle. Every entry
// point takes the caller's *gorm.DB rather than capturing one at construction,
// because transaction membership, subtree scope, and actor attribution all ride
// on that handle:
//
//   - application_context.WithTransaction swaps in a *gorm.DB bound to the
//     transaction. A captured handle would run every write outside it, so a
//     failed import would leave partial rows behind instead of rolling back.
//   - application_context.WithPrincipal swaps in a handle whose context carries
//     the subtree allow-list and acting user id, which the GORM callbacks read.
//     A captured handle would see neither, so a scoped principal's writes would
//     escape their subtree and stamp no creator.
//
// Both failures are silent and security-relevant. See
// docs/plans/2026-07-28-application-context-decomposition.md section 2.
//
// Only fs and alt live on Service: they are set once at construction and never
// rebound by any of the context derivation methods.
package groupio

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/afero"
	"gorm.io/gorm"

	"mahresources/download_queue"
	"mahresources/models"
)

// Service holds the process-lifetime filesystem handles. Construct one per
// process; it carries no request, transaction, or principal state.
type Service struct {
	fs  afero.Fs
	alt map[string]afero.Fs
}

// NewService returns a Service over the main filesystem and the alternative
// filesystems keyed by storage location.
func NewService(fs afero.Fs, alt map[string]afero.Fs) *Service {
	return &Service{fs: fs, alt: alt}
}

// ScopeResolver is the sliver of principal state a bare *gorm.DB cannot carry.
// The GORM scope callbacks do not fire on raw SQL, nor on tables scopeColumn()
// does not map (group_relations in particular), so those paths must consult the
// principal's subtree explicitly.
type ScopeResolver interface {
	// SubtreeGroupIDs flattens a group's subtree.
	SubtreeGroupIDs(rootID uint) ([]uint, error)
	// VisibleGroupIDs filters candidate group IDs down to those the current
	// principal may see. Fail-closed: a scoped principal with no resolvable
	// allow-list gets the empty set, not everything.
	VisibleGroupIDs(ids []uint) map[uint]bool
}

// Deps is the per-call input. Rebuild it for every call — never cache one — so
// that a transactional or principal-derived handle is picked up rather than the
// handle that happened to be current when the Service was built.
type Deps struct {
	DB    *gorm.DB      // carries transaction membership, subtree scope, and actor
	Scope ScopeResolver // the principal-derived bits the handle cannot carry
}

// opCtx binds the immutable Service to one call's Deps. It exists for exactly
// as long as the call, which is what makes handle capture structurally
// impossible: there is nowhere durable to capture it to.
type opCtx struct {
	*Service
	db    *gorm.DB
	scope ScopeResolver
}

// op builds the per-call binding. Scope must be non-nil; the export planner and
// the import applier both consult it.
func (s *Service) op(d Deps) *opCtx {
	return &opCtx{Service: s, db: d.DB, scope: d.Scope}
}

// --- Public entry points ---
//
// Each binds the caller's Deps to the Service for the duration of one call and
// no longer. Signatures mirror the former MahresourcesContext methods exactly,
// with Deps prepended, so the application_context facade is a pure delegation.

// EstimateExport walks the requested scope and returns counts without touching
// file bytes.
func (s *Service) EstimateExport(d Deps, req *ExportRequest) (*ExportEstimate, error) {
	return s.op(d).EstimateExport(req)
}

// StreamExport writes the export archive to dst, reporting progress.
func (s *Service) StreamExport(d Deps, jobCtx context.Context, req *ExportRequest, dst io.Writer, report ReporterFn) error {
	return s.op(d).StreamExport(jobCtx, req, dst, report)
}

// ParseImport reads an import tar and produces a plan for review.
func (s *Service) ParseImport(d Deps, cancelCtx context.Context, jobID, tarPath string) (*ImportPlan, error) {
	return s.op(d).ParseImport(cancelCtx, jobID, tarPath)
}

// ApplyImport executes a reviewed plan.
func (s *Service) ApplyImport(d Deps, cancelCtx context.Context, parseJobID string, decisions *ImportDecisions, sink download_queue.ProgressSink) (*ImportApplyResult, error) {
	return s.op(d).ApplyImport(cancelCtx, parseJobID, decisions, sink)
}

// LoadImportPlan reads back a persisted plan.
func (s *Service) LoadImportPlan(d Deps, jobID string) (*ImportPlan, error) {
	return s.op(d).LoadImportPlan(jobID)
}

// DeleteImportFiles removes a parse job's staged tar and plan.
func (s *Service) DeleteImportFiles(d Deps, jobID string) error {
	return s.op(d).DeleteImportFiles(jobID)
}

// GetFsForStorageLocation resolves a resource's storage location to a
// filesystem. Ported from application_context.MahresourcesContext, whose
// version reads the same two immutable fields.
func (s *Service) GetFsForStorageLocation(storageLocation *string) (afero.Fs, error) {
	if storageLocation != nil {
		altFs, ok := s.alt[*storageLocation]
		if !ok {
			return nil, fmt.Errorf("alt fs '%v' is not attached", *storageLocation)
		}
		return altFs, nil
	}
	return s.fs, nil
}

// GetSeriesBySlug looks up a series by its unique slug.
//
// application_context's version also does Preload("Resources", pageLimit). The
// sole caller here (resolveSeriesInfo) reads only ID and Name, so the preload
// was dead weight; dropping it avoids depending on pageLimit and changes
// nothing observable.
func (ctx *opCtx) GetSeriesBySlug(slug string) (*models.Series, error) {
	var series models.Series
	return &series, ctx.db.Where("slug = ?", slug).First(&series).Error
}

// Association helpers, copied rather than moved from
// application_context/associations.go. BuildAssociationSlicePtr, tagPtrFromID,
// and groupPtrFromID are also used by group_crud_context.go, which is not
// moving, so relocating them would leave application_context uncompilable.
// They are a one-line generic and four three-line constructors over models/
// only, so a copy is cheaper than a shared leaf package.

func buildAssociationSlicePtr[T any](ids []uint, factory func(uint) *T) []*T {
	result := make([]*T, len(ids))
	for i, id := range ids {
		result[i] = factory(id)
	}
	return result
}

func tagPtrFromID(id uint) *models.Tag           { return &models.Tag{ID: id} }
func groupPtrFromID(id uint) *models.Group       { return &models.Group{ID: id} }
func notePtrFromID(id uint) *models.Note         { return &models.Note{ID: id} }
func resourcePtrFromID(id uint) *models.Resource { return &models.Resource{ID: id} }
