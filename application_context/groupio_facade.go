package application_context

import (
	"context"
	"io"

	"mahresources/download_queue"
	"mahresources/groupio"
)

// Facade over the groupio package, which owns group import/export. It exists so
// that extracting ~5,900 LOC out of this package changed no call site: the six
// methods keep their original signatures and the aliases below keep the DTO
// type identities.

// The externally-referenced seam types. Aliases (=), not defined types, so
// application_context.ImportPlan and groupio.ImportPlan are the *same* type and
// every existing reference compiles untouched.
//
// This list was derived by searching `application_context.<symbol>` across the
// repo for every exported symbol defined in the four moved files. Seven further
// exported symbols (ConflictSummary, DanglingRefPlan, DecisionKeyFor,
// ImportMappings, ImportPlanCounts, MappingAlternative, SeriesMapping) have no
// external references and are reached only through field access, which alias
// identity already covers. Re-run that search before adding to the seam; an
// incomplete list breaks the build in cmd/mr, not here.
type (
	ImportPlan        = groupio.ImportPlan
	ImportDecisions   = groupio.ImportDecisions
	ImportApplyResult = groupio.ImportApplyResult
	ExportRequest     = groupio.ExportRequest
	ExportEstimate    = groupio.ExportEstimate
	ReporterFn        = groupio.ReporterFn
	ProgressEvent     = groupio.ProgressEvent

	// Constructed directly by cmd/mr/commands/group_import.go and the handler
	// unit tests, rather than only received.
	ImportPlanItem   = groupio.ImportPlanItem
	MappingEntry     = groupio.MappingEntry
	MappingAction    = groupio.MappingAction
	DanglingAction   = groupio.DanglingAction
	ShellGroupAction = groupio.ShellGroupAction
)

// groupioScope adapts MahresourcesContext to groupio.ScopeResolver without
// exporting two more methods on an already-wide struct. Both delegates read
// this context's principal and db, so a derived context resolves against its
// own scope.
type groupioScope struct{ ctx *MahresourcesContext }

func (g groupioScope) SubtreeGroupIDs(rootID uint) ([]uint, error) {
	return g.ctx.collectSubtreeGroupIDs(rootID)
}

func (g groupioScope) VisibleGroupIDs(ids []uint) map[uint]bool {
	return g.ctx.visibleGroupIDs(ids)
}

// groupioDeps rebuilds the per-call dependencies every time, so it always
// reflects THIS context's db — the transactional and/or subtree-scoped handle on
// a derived copy. Never cache the result.
//
// This is the whole trick. WithTransaction and WithPrincipal shallow-copy the
// struct and swap only db; a groupio.Service field is copied verbatim, but it
// holds nothing but filesystems, and the handle is read here at call time. A
// Service that had captured db at construction would silently run outside the
// transaction and outside the subtree. See groupio's package comment.
func (ctx *MahresourcesContext) groupioDeps() groupio.Deps {
	return groupio.Deps{DB: ctx.db, Scope: groupioScope{ctx: ctx}}
}

// EstimateExport walks the requested scope and returns counts without
// touching file bytes. Used by /v1/groups/export/estimate.
func (ctx *MahresourcesContext) EstimateExport(req *ExportRequest) (*ExportEstimate, error) {
	return ctx.groupio.EstimateExport(ctx.groupioDeps(), req)
}

// StreamExport writes the export archive for req to dst.
func (ctx *MahresourcesContext) StreamExport(jobCtx context.Context, req *ExportRequest, dst io.Writer, report ReporterFn) error {
	return ctx.groupio.StreamExport(ctx.groupioDeps(), jobCtx, req, dst, report)
}

// ParseImport reads a staged import tar and produces a plan for review.
func (ctx *MahresourcesContext) ParseImport(cancelCtx context.Context, jobID, tarPath string) (*ImportPlan, error) {
	return ctx.groupio.ParseImport(ctx.groupioDeps(), cancelCtx, jobID, tarPath)
}

// ApplyImport executes a reviewed import plan.
func (ctx *MahresourcesContext) ApplyImport(cancelCtx context.Context, parseJobID string, decisions *ImportDecisions, sink download_queue.ProgressSink) (*ImportApplyResult, error) {
	return ctx.groupio.ApplyImport(ctx.groupioDeps(), cancelCtx, parseJobID, decisions, sink)
}

// LoadImportPlan reads back a previously persisted import plan.
func (ctx *MahresourcesContext) LoadImportPlan(jobID string) (*ImportPlan, error) {
	return ctx.groupio.LoadImportPlan(ctx.groupioDeps(), jobID)
}

// DeleteImportFiles removes a parse job's staged tar and plan.
func (ctx *MahresourcesContext) DeleteImportFiles(jobID string) error {
	return ctx.groupio.DeleteImportFiles(ctx.groupioDeps(), jobID)
}
