package application_context

import (
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/mrql"
)

// Mass-edit typed refusals. Matched by errors.Is in
// server/api_handlers/error_status.go BEFORE the substring scan, because their
// wordings would otherwise be claimed by the generic patterns there.
var (
	// ErrMassEditSetChanged refuses a filter-targeted edit whose re-count does
	// not match the count the caller confirmed. The request is well formed and
	// the world moved, so it is a conflict (409), not a bad request.
	ErrMassEditSetChanged = errors.New("the set of entities matching this filter changed since the count was confirmed")
	// ErrMassEditTooLarge refuses a resolved set above the deployment ceiling.
	// The ceiling is a lock-duration budget, not a memory budget: on SQLite the
	// write lock is held for the whole edit, and a six-figure edit would hold it
	// for minutes. Raised with the count and the ceiling named.
	ErrMassEditTooLarge = errors.New("mass edit set exceeds the configured ceiling")
	// ErrMassEditOwnershipCycle refuses a group re-parent whose new owner sits
	// below one of the targets. Refused, never repaired: the cycle IS the
	// primary effect of what was asked, nothing has been destroyed yet, and
	// NULLing an owner silently evicts a whole subtree from its parent — and,
	// for a scoped principal, from its own visibility forever.
	ErrMassEditOwnershipCycle = errors.New("this mass edit would create an ownership cycle")
	// ErrMassEditOwnerClearScoped refuses owner-clearing for a group-limited
	// principal. An owner-less row is in nobody's subtree, so clearing evicts
	// the rows from the caller's own visibility permanently and cannot be
	// undone by them. An authorization answer (403), not a validation failure.
	ErrMassEditOwnerClearScoped = errors.New("clearing the owner is not available to a group-limited principal")
)

// defaultMaxMassEditEntities bounds one mass edit when no operator value is
// configured.
//
// The ceiling is a lock-duration budget, not a memory budget: one mass edit
// wraps every op and every chunk in a single transaction, and on SQLite that
// means the database's write lock is held for the whole of it. A six-figure
// edit would hold the writer for minutes.
const defaultMaxMassEditEntities = 10_000

// massEditChunkSize bounds how many ids one mass-edit write statement names.
//
// Chunking is a bind-count strategy, not a transaction strategy — every chunk
// runs inside the same transaction. 500 keeps the statement well under SQLite's
// SQLITE_MAX_VARIABLE_NUMBER and Postgres's 65535, which matters because the
// scope callback appends the whole subtree allow-list to every GORM-modelled
// statement: the effective bind count is chunk + subtree.
//
// A var rather than a const so tests can lower it and exercise the boundaries.
var massEditChunkSize = 500

// Engine placeholder ceilings. SQLite's SQLITE_MAX_VARIABLE_NUMBER defaults to
// 32766 (3.32+), Postgres's bind limit is 65535; the margin absorbs the chunk,
// the statement's fixed predicates and engine variance.
const (
	sqliteParamLimit = 32766
	pgParamLimit     = 65535
	paramLimitMargin = 512
)

// massEditParamLimit is the engine's placeholder ceiling, read from the LIVE
// dialector rather than the configured DbType: a directly constructed context
// with an empty DbType sits on the SQLite dialector and must get SQLite's
// smaller budget.
func (ctx *MahresourcesContext) massEditParamLimit() int {
	if ctx.db.Dialector.Name() == "postgres" {
		return pgParamLimit
	}
	return sqliteParamLimit
}

// massEditBindBudget is how many ids one request may bind into a single
// statement of the described shape, after the request's scope allow-list (when
// the statement carries it — the scope callback appends it to every scoped
// statement), the target chunk, and a margin for the statement's fixed
// predicates. Enforced at PARSE time, so a DryRun and the confirmed submit
// fail identically instead of the submit failing at apply time.
//
// allowListBound must be true only for statements that actually bind the
// allow-list — scoped, non-tag statements through the GORM layer. Raw DELETEs
// and the tag family's global statements never carry it.
// massEditBindBudgetOverride, when >= 0, replaces the computed budget. It is
// a TEST seam for the exhausted-budget regression; production leaves it -1.
var massEditBindBudgetOverride = -1

func (ctx *MahresourcesContext) massEditBindBudget(allowListBound bool, extraBinds int) int {
	if massEditBindBudgetOverride >= 0 {
		return massEditBindBudgetOverride
	}
	budget := ctx.massEditParamLimit() - paramLimitMargin
	if allowListBound {
		sf := scopeFromContext(ctx.db.Statement.Context)
		if sf != nil {
			budget -= len(sf.allowed)
		}
	}
	budget -= extraBinds
	if budget < 0 {
		budget = 0
	}
	return budget
}

// massEditBetweenCountAndPluck, when non-nil, runs between the filter's Count
// and the keyset Pluck. It is a TEST seam for the TOCTOU regression test —
// production code never sets it.
var massEditBetweenCountAndPluck func()

// massEditPluckPage bounds one keyset page of filter-target resolution.
// Offset over millions is O(n²) and drifts if anything commits between pages;
// keyset on the primary key is neither.
var massEditPluckPage = 1000

// massEditHookIDThreshold caps when the before-mass-edit hook payload carries
// the full id list. Above it, plugins get the count only.
const massEditHookIDThreshold = 100

// MaxMassEditEntities is how many entities one mass edit may change.
//
// Zero means "not configured" and selects the default, never "refuse
// everything": every api_test and every programmatic embed builds a
// MahresourcesConfig{} whose zero value was never a decision.
func (ctx *MahresourcesContext) MaxMassEditEntities() int {
	if n := ctx.Config.MaxMassEditEntities; n > 0 {
		return n
	}
	return defaultMaxMassEditEntities
}

func (ctx *MahresourcesContext) MassEditResources(q *query_models.MassEditQuery) (*contracts.MassEditResult, error) {
	return ctx.massEdit(massEditResourceSpec, q)
}

func (ctx *MahresourcesContext) MassEditNotes(q *query_models.MassEditQuery) (*contracts.MassEditResult, error) {
	return ctx.massEdit(massEditNoteSpec, q)
}

func (ctx *MahresourcesContext) MassEditGroups(q *query_models.MassEditQuery) (*contracts.MassEditResult, error) {
	return ctx.massEdit(massEditGroupSpec, q)
}

// massEditOp is one parsed operation of a mass edit. validate runs once, inside
// the transaction, before any op applies; apply runs per chunk.
// massEditLockReq is one row-lock requirement of a parsed mass edit: the named
// far-endpoint (or target) rows must exist, be visible to the caller, and stay
// that way until the transaction ends. Requirements are UNIONED BY MODEL in
// lockMassEditInputs, never taken role by role — two edits with mirrored roles
// (A targets group 1 and references group 2, B the reverse) would otherwise
// take the same two row locks in opposite orders and deadlock on Postgres.
type massEditLockReq struct {
	key   string // model identity: "group" | "note" | "resource" | "tag"
	model any
	what  string // the not-found message noun: "groups"
	ids   []uint
	// missingErr, when set, is returned verbatim when THIS requirement's ids
	// fail the model phase — the owner requirement says "owner group not
	// found", the exact wording EditResource uses.
	missingErr error
}

// massEditOp is one parsed operation of a mass edit. Its lock requirements are
// satisfied in one canonical phase before anything applies; check runs after
// the locks are held for read-only semantic conditions (the group cycle); apply
// runs per chunk.
type massEditOp struct {
	name string
	// lockReqs names the rows this op's writes depend on.
	lockReqs []massEditLockReq
	// check is a read-only semantic condition evaluated after locking, inside
	// the transaction. It receives the resolved target ids.
	check func(txCtx *MahresourcesContext, ids []uint) error
	// apply writes one chunk and reports how many rows it touched — join rows
	// for relation ops, entity rows for owner and meta ops.
	apply func(txCtx *MahresourcesContext, chunk []uint) (int64, error)
}

// massEditModelOrder is the fixed order model classes are locked in. It must
// be the same for every mass edit (no role-based partitioning) and consistent
// with the upload path's association validation order (groups, notes, tags) —
// locks taken in a globally consistent order cannot deadlock.
var massEditModelOrder = map[string]int{"group": 0, "note": 1, "resource": 2, "tag": 3}

// lockMassEditInputs unions the edit's lock requirements by model, then locks
// and verifies each model's ids in one validateAndLockIDs call per model,
// models in massEditModelOrder, ids ascending within a model.
//
// Every requirement runs through the scoped db, so a group-limited principal
// cannot name an out-of-subtree target or far endpoint, and every row is held
// to the end of the transaction: on Postgres READ COMMITTED a plain Count is
// check-then-act, and the raw-Exec writes that follow have no foreign keys to
// catch what a concurrent delete leaves behind.
func (ctx *MahresourcesContext) lockMassEditInputs(spec massEditSpec, ops []massEditOp, targetIDs []uint) error {
	byModel := map[string]*massEditLockReq{}
	var order []string
	add := func(key string, model any, what string, ids []uint) {
		if len(ids) == 0 {
			return
		}
		req, ok := byModel[key]
		if !ok {
			req = &massEditLockReq{key: key, model: model, what: what}
			byModel[key] = req
			order = append(order, key)
		}
		req.ids = append(req.ids, ids...)
	}

	add(spec.entity, spec.model(), spec.noun+"s", targetIDs)
	for _, op := range ops {
		for _, req := range op.lockReqs {
			add(req.key, req.model, req.what, req.ids)
		}
	}

	sort.Slice(order, func(i, j int) bool { return massEditModelOrder[order[i]] < massEditModelOrder[order[j]] })
	for _, key := range order {
		req := byModel[key]
		// Deduplicate and sort the WHOLE union before chunking: lockIDs sorts
		// only within a chunk, and unsorted union order would let two requests
		// lock high- and low-id chunks in opposite orders and deadlock.
		req.ids = deduplicateUints(req.ids)
		sort.Slice(req.ids, func(i, j int) bool { return req.ids[i] < req.ids[j] })
		// Ascending chunks: the union can exceed the bind ceiling once the
		// scope callback appends the whole subtree to the statement, and the
		// chunk boundaries stay ascending so the global lock order is
		// preserved.
		var missing []uint
		for _, chunk := range chunkUints(req.ids, massEditChunkSize) {
			found, err := lockIDs(ctx.db, req.model, chunk)
			if err != nil {
				return err
			}
			missing = append(missing, setDifference(chunk, found)...)
		}
		if len(missing) > 0 {
			// Attribute the failure to a requirement WITHOUT re-querying —
			// re-running the lock clause here could acquire fresh,
			// out-of-order row locks on the failure path.
			for _, op := range ops {
				for _, r := range op.lockReqs {
					if r.key != key {
						continue
					}
					// The first requirement whose ids overlap the missing set
					// owns the failure.
					if !intersects(missing, r.ids) {
						continue
					}
					if r.missingErr != nil {
						return r.missingErr
					}
					return fmt.Errorf("one or more %s not found", r.what)
				}
			}
			return fmt.Errorf("one or more %ss not found", key)
		}
	}
	return nil
}

func intersects(a []uint, b []uint) bool {
	set := make(map[uint]bool, len(a))
	for _, v := range a {
		set[v] = true
	}
	for _, v := range b {
		if set[v] {
			return true
		}
	}
	return false
}

func setDifference(a []uint, b []uint) []uint {
	remove := make(map[uint]bool, len(b))
	for _, v := range b {
		remove[v] = true
	}
	var out []uint
	for _, v := range a {
		if !remove[v] {
			out = append(out, v)
		}
	}
	return out
}

// massEditSpec describes one entity's mass edit: the three entry points differ
// only in this descriptor and in which ops parseOps accepts.
type massEditSpec struct {
	entity     string // wire name in results and hooks: "resource"
	noun       string // singular, for messages: "resource"
	plural     string
	entityType string // search-cache invalidation: EntityTypeResource
	table      string // "resources"
	idColumn   string // the entity's own column in its join tables: "resource_id"
	// model returns a zero GORM model of the entity, for the scoped Count gate,
	// the owner update and the keyset pluck.
	model    func() any
	parseOps func(spec massEditSpec, ctx *MahresourcesContext, q *query_models.MassEditQuery) ([]massEditOp, error)
}

var massEditResourceSpec = massEditSpec{
	entity:     "resource",
	noun:       "resource",
	plural:     "resources",
	entityType: EntityTypeResource,
	table:      "resources",
	idColumn:   "resource_id",
	model:      func() any { return &models.Resource{} },
	parseOps:   parseResourceMassEditOps,
}

var massEditNoteSpec = massEditSpec{
	entity:     "note",
	noun:       "note",
	plural:     "notes",
	entityType: EntityTypeNote,
	table:      "notes",
	idColumn:   "note_id",
	model:      func() any { return &models.Note{} },
	parseOps:   parseNoteMassEditOps,
}

var massEditGroupSpec = massEditSpec{
	entity:     "group",
	noun:       "group",
	plural:     "groups",
	entityType: EntityTypeGroup,
	table:      "groups",
	idColumn:   "group_id",
	model:      func() any { return &models.Group{} },
	parseOps:   parseGroupMassEditOps,
}

// massEdit is the one write path all three entities share.
//
// All-or-nothing: one WithTransaction wraps every op and every chunk. Chunking
// is a bind-count strategy, not a transaction strategy — the reason is specific
// to filter mode, where partial success is unrecoverable: a reader told "7,431
// of 9,802 succeeded" cannot express the remainder as a filter, because the
// successful edits changed the filter's own result set.
func (ctx *MahresourcesContext) massEdit(spec massEditSpec, q *query_models.MassEditQuery) (*contracts.MassEditResult, error) {
	ops, err := spec.parseOps(spec, ctx, q)
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return nil, fmt.Errorf("at least one operation is required")
	}

	ids, matched, err := ctx.resolveMassEditTargets(spec, q)
	if err != nil {
		return nil, err
	}

	result := &contracts.MassEditResult{
		Entity:   spec.entity,
		Matched:  matched,
		Affected: int64(len(ids)),
		DryRun:   q.DryRun,
	}
	opNames := make([]string, 0, len(ops))
	for _, op := range ops {
		opNames = append(opNames, op.name)
		result.Ops = append(result.Ops, contracts.MassEditOpResult{Op: op.name})
	}

	if q.DryRun {
		// DryRun resolves the target set and echoes the parsed ops, committing
		// nothing. It is what the confirmation dialog's "this will change 4,211
		// resources" is built on. It does NOT run the transaction-scoped
		// validators (far endpoints, owner scope, group cycles) — those run on
		// the real submit, and the UI documents the probe as a count, not a
		// guarantee.
		return result, nil
	}

	if _, hookErr := ctx.RunBeforePluginHooks("before_mass_edit", beforeMassEditHookPayload(spec, q, opNames, ids)); hookErr != nil {
		return nil, hookErr
	}

	err = ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		// A group re-parent participates in the tree-mutation protocol BEFORE
		// any row lock is taken: the cycle check reads an ancestor chain that
		// a concurrent UpdateGroup could be extending, and the advisory lock
		// (shared with UpdateGroup's re-parent branch) serialises them. First
		// operation in the transaction — an advisory lock taken after row
		// locks could invert against a transaction holding it and waiting for
		// those rows.
		if spec.entity == "group" && q.OwnerOp == "set" {
			if err := txCtx.lockGroupTreeMutation(txCtx.db); err != nil {
				return err
			}
		}

		// Phase 1 — lock and verify every row the edit depends on. Requirements
		// are UNIONED BY MODEL and taken in one fixed model order, ids ascending
		// within each model: a global total order that two mass edits cannot
		// traverse in opposite directions, whichever roles the same rows play in
		// them (target here, far endpoint there). Targets are locked so a
		// concurrent re-parent cannot move a row out of the caller's subtree
		// before the raw join-table statements run; far endpoints are locked so
		// a concurrent delete cannot leave a dangling join row behind (Postgres
		// creates no join-table foreign keys). The model order matches the
		// upload path's association validation (groups, notes, tags) to avoid
		// cross-feature inversions.
		if err := txCtx.lockMassEditInputs(spec, ops, ids); err != nil {
			return err
		}

		// Phase 2 — read-only semantic conditions, after the locks are held: the
		// group re-parent cycle check walks ancestry that the lock phase cannot
		// meaningfully freeze.
		for _, op := range ops {
			if op.check != nil {
				if err := op.check(txCtx, ids); err != nil {
					return err
				}
			}
		}

		// Phase 3 — apply. Ops OUTER, chunks INNER, and the owner op is parsed
		// last: re-parenting changes subtree membership, and the scope allow-list
		// on the db context is a snapshot from request start, so no scoped read
		// after the owner UPDATE may be expected to see fresh membership.
		for _, op := range ops {
			for _, chunk := range chunkUints(ids, massEditChunkSize) {
				n, err := op.apply(txCtx, chunk)
				if err != nil {
					return err
				}
				addMassEditRows(result, op.name, n)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Meta and owner changes change what search returns; the join-table writes
	// fire SQLite's FTS triggers themselves, but the cache must still go.
	ctx.InvalidateSearchCacheByType(spec.entityType)

	logFields := map[string]interface{}{
		"matched":  result.Matched,
		"affected": result.Affected,
		"ops":      opNames,
	}
	if q.Target == "filter" {
		logFields["filter"] = q.Filter
	}
	ctx.Logger().Info(models.LogActionUpdate, spec.noun, nil, "", "Mass edited "+spec.plural, logFields)

	ctx.RunAfterPluginHooks("after_mass_edit", map[string]any{
		"entity":   spec.entity,
		"matched":  float64(result.Matched),
		"affected": float64(result.Affected),
		"ops":      opNames,
	})

	return result, nil
}

// resolveMassEditTargets resolves the target set. Selection mode uses the
// explicit ids; filter mode re-runs the list page's own query server-side.
func (ctx *MahresourcesContext) resolveMassEditTargets(spec massEditSpec, q *query_models.MassEditQuery) ([]uint, int64, error) {
	switch q.Target {
	case "", "ids":
		ids := deduplicateUints(q.ID)
		if len(ids) == 0 {
			return nil, 0, fmt.Errorf("at least one %s ID is required", spec.noun)
		}
		// Ascending, like the filter mode's keyset pluck: the target rows are
		// locked chunk by chunk, and two transactions taking the same row locks
		// in opposite orders deadlock on Postgres.
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		// The ceiling is a lock-duration budget, not a filter-mode rule: an
		// explicit selection holds the same write lock a filter-resolved set
		// does.
		if int64(len(ids)) > int64(ctx.MaxMassEditEntities()) {
			return nil, 0, fmt.Errorf("%w: %d %s ids exceeds the ceiling of %d",
				ErrMassEditTooLarge, len(ids), spec.noun, ctx.MaxMassEditEntities())
		}
		return ids, int64(len(ids)), nil

	case "filter":
		// An EMPTY filter is legitimate: it means every entity visible to the
		// caller — the same set the unfiltered list page shows — and the scope
		// predicate confines it exactly as it confines the list. ExpectedCount
		// (and the confirm handshake) still gates the write.
		//
		// DryRun is what produces the count the confirmation dialog shows, so it
		// must work without an ExpectedCount; the real edit may not. The field
		// is a POINTER because zero is a legitimate confirmed count (an empty
		// set over an empty filter) that must be told apart from "omitted".
		if q.ExpectedCount == nil && !q.DryRun {
			return nil, 0, fmt.Errorf("an expected count is required when targeting every matching %s", spec.noun)
		}

		// The decoded filter's id lists ride the Count and every Pluck page as
		// bind parameters (some scopes bind a list twice), BEFORE any ceiling
		// on the matched set is known. Bound their cost here, so an oversized
		// filter is the same typed 400 as every other budget refusal instead
		// of a driver error.
		if cost := spec.filterBindCost(q.Filter); cost > ctx.massEditBindBudget(true, massEditChunkSize+64) {
			return nil, 0, fmt.Errorf("%w: the filter's id lists exceed the database parameter budget of %d",
				ErrMassEditTooLarge, ctx.massEditBindBudget(true, massEditChunkSize+64))
		}

		matched, err := ctx.countMassEditTargets(spec, q.Filter)
		if err != nil {
			return nil, 0, err
		}

		// Exact match, answered with a conflict. The number in the confirmation
		// dialog is the number the server acts on, or nobody acts.
		if !q.DryRun && q.ExpectedCount != nil && matched != int64(*q.ExpectedCount) {
			return nil, 0, ErrMassEditSetChanged
		}

		// Refused with the count and the ceiling named, never truncated.
		if matched > int64(ctx.MaxMassEditEntities()) {
			return nil, 0, fmt.Errorf("%w: %d %s matches exceeds the ceiling of %d",
				ErrMassEditTooLarge, matched, spec.noun, ctx.MaxMassEditEntities())
		}

		// Test seam: a deterministic place for the world to move between the
		// Count above and the Pluck below. Production leaves it nil — the two
		// queries are as adjacent as the transaction boundary allows.
		if massEditBetweenCountAndPluck != nil {
			massEditBetweenCountAndPluck()
		}

		ids, err := ctx.pluckMassEditTargetsWithinCap(spec, q.Filter, ctx.MaxMassEditEntities())
		if err != nil {
			return nil, 0, err
		}
		// The world can move between the Count above and the Pluck here — an
		// insert, a delete, a re-parent. The materialized id set is what the
		// ops will touch, so IT is authoritative: anything other than the
		// confirmed count is the same conflict an ExpectedCount mismatch is.
		if int64(len(ids)) != matched {
			return nil, 0, ErrMassEditSetChanged
		}
		return ids, matched, nil

	default:
		return nil, 0, fmt.Errorf("target must be \"ids\" or \"filter\"")
	}
}

// countMassEditTargets counts the filter's matches with the entity's own list
// scope, ignoreSort=true, exactly as GetResourceCount and its siblings build
// it. A bad MRQL expression fails here and never widens: applyMRQLFilter
// returns *MRQLFilterError, and there is no fall-through to the unfiltered set.
func (ctx *MahresourcesContext) countMassEditTargets(spec massEditSpec, filter string) (int64, error) {
	mrqlExpr, scope, err := spec.massEditScope(filter, ctx.db)
	if err != nil {
		return 0, err
	}

	var count int64
	db := ctx.db.Scopes(scope).Model(spec.model())
	db, err = ctx.applyMRQLFilter(db, spec.mrqlEntity(), mrqlExpr)
	if err != nil {
		return 0, err
	}
	return count, db.Count(&count).Error
}

// pluckMassEditTargets pages the matched ids out by keyset on the primary key,
// rebuilding the scoped query per page.
//
// Pluck, never Scan: Pluck runs the Query callback chain, so scopeReadCallback
// appends the subtree predicate; Scan does not. Every scoped read in this
// feature rests on that one method choice, and a well-meaning "optimisation"
// to Scan hands a group-limited principal the whole database.
func (ctx *MahresourcesContext) pluckMassEditTargets(spec massEditSpec, filter string) ([]uint, error) {
	return ctx.pluckMassEditTargetsWithinCap(spec, filter, 0)
}

// pluckMassEditTargetsWithinCap stops plucking once the resolved set passes
// the cap: materializing ten million ids before refusing them is its own
// denial of service. cap <= 0 means unbounded (tests). The caller re-checks
// the final length against the confirmed count and the ceiling either way.
func (ctx *MahresourcesContext) pluckMassEditTargetsWithinCap(spec massEditSpec, filter string, cap int) ([]uint, error) {
	mrqlExpr, scope, err := spec.massEditScope(filter, ctx.db)
	if err != nil {
		return nil, err
	}

	var ids []uint
	var cursor uint
	for {
		db := ctx.db.Scopes(scope).Model(spec.model()).
			Where(spec.table+".id > ?", cursor).
			Order(spec.table + ".id").
			Limit(massEditPluckPage)
		db, err := ctx.applyMRQLFilter(db, spec.mrqlEntity(), mrqlExpr)
		if err != nil {
			return nil, err
		}
		var page []uint
		if err := db.Pluck(spec.table+".id", &page).Error; err != nil {
			return nil, err
		}
		if len(page) == 0 {
			break
		}
		ids = append(ids, page...)
		cursor = page[len(page)-1]
		if len(page) < massEditPluckPage {
			break
		}
		if cap > 0 && int64(len(ids)) > int64(cap) {
			break
		}
	}
	return ids, nil
}

// massEditScope decodes the raw filter query string into the entity's real
// search DTO and returns its MRQL expression and list scope (ignoreSort=true) —
// the same scope GetResourceCount and its siblings build. The decode cannot
// live in server/api_handlers — its decoder is package-private and
// application_context may not import server/.
//
// MetaQuery survives the decode through query_models.FillMetaQueryFromValues:
// the field needs hand-parsing because gorilla/schema converters never fire for
// slice fields, and a dropped MetaQuery would widen the target set beyond what
// the reader was shown.
func (spec *massEditSpec) massEditScope(filter string, base *gorm.DB) (string, func(db *gorm.DB) *gorm.DB, error) {
	switch spec.entity {
	case "note":
		nq, err := query_models.DecodeNoteFilter(filter)
		if err != nil {
			return "", nil, err
		}
		return nq.MRQL, database_scopes.NoteQuery(nq, true, base), nil
	case "group":
		gq, err := query_models.DecodeGroupFilter(filter)
		if err != nil {
			return "", nil, err
		}
		return gq.MRQL, database_scopes.GroupQuery(gq, true, base), nil
	default:
		rq, err := query_models.DecodeResourceFilter(filter)
		if err != nil {
			return "", nil, err
		}
		return rq.MRQL, database_scopes.ResourceQuery(rq, true, base), nil
	}
}

// filterBindCost counts the request-controlled id lists a decoded filter will
// bind. The multiplier is deliberately loose (some scopes bind a list twice,
// e.g. a group filter's Groups feed both a direct and a parent-scope
// predicate); the budget's margin absorbs the imprecision.
func (spec *massEditSpec) filterBindCost(filter string) int {
	switch spec.entity {
	case "note":
		nq, err := query_models.DecodeNoteFilter(filter)
		if err != nil {
			return 0
		}
		return 2 * (len(nq.Ids) + len(nq.Tags) + len(nq.Groups) + len(nq.NoteTypeIds) + len(nq.MetaQuery))
	case "group":
		gq, err := query_models.DecodeGroupFilter(filter)
		if err != nil {
			return 0
		}
		return 2 * (len(gq.Ids) + len(gq.Tags) + len(gq.Groups) + len(gq.Notes) + len(gq.Resources) + len(gq.Categories) + len(gq.MetaQuery))
	default:
		rq, err := query_models.DecodeResourceFilter(filter)
		if err != nil {
			return 0
		}
		return 2 * (len(rq.Ids) + len(rq.Tags) + len(rq.Groups) + len(rq.Notes) + len(rq.ContentTypes) + len(rq.MetaQuery))
	}
}

func (spec *massEditSpec) mrqlEntity() mrql.EntityType {
	switch spec.entity {
	case "note":
		return mrql.EntityNote
	case "group":
		return mrql.EntityGroup
	default:
		return mrql.EntityResource
	}
}

// countRefsInScope resolves the named far-endpoint ids through the scoped db
// and refuses any that did not resolve — and on Postgres it LOCKS the rows it
// resolved (validateAndLockIDs): the join-table inserts that follow are raw
// Exec, Postgres migrations create no foreign keys, so a far row deleted
// concurrently after a plain Count would leave a dangling join row behind.
// The lock holds it in place until this transaction ends.
func countRefsInScope(txCtx *MahresourcesContext, model any, ids []uint, what string) error {
	return validateAndLockIDs(txCtx.db, model, ids, what)
}

// addMassEditRows accumulates rowsAffected into the result's op entry.
func addMassEditRows(result *contracts.MassEditResult, op string, n int64) {
	for i := range result.Ops {
		if result.Ops[i].Op == op {
			result.Ops[i].RowsAffected += n
			return
		}
	}
}

// beforeMassEditHookPayload builds the before_mass_edit payload. Veto-only:
// field rewrites are not honoured, and there are no fields to rewrite anyway.
func beforeMassEditHookPayload(spec massEditSpec, q *query_models.MassEditQuery, opNames []string, ids []uint) map[string]any {
	payload := map[string]any{
		"entity": spec.entity,
		"count":  float64(len(ids)),
		"ops":    opNames,
	}
	if q.Target == "filter" {
		payload["target"] = "filter"
		payload["filter"] = q.Filter
	}
	if len(ids) <= massEditHookIDThreshold {
		hookIDs := make([]any, len(ids))
		for i, id := range ids {
			hookIDs[i] = float64(id)
		}
		payload["ids"] = hookIDs
	}
	return payload
}
