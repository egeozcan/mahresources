package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
)

// The table-backed review reader (design D8, D10), beside the JSON one in
// resource_reduction_review.go and not wired to a route: storeGetReductionReview
// builds the same contracts.ReductionReview, from the plan tables, in bounded
// memory. The provider switches over once the writers do.
//
// Everything the review states is one read snapshot at one plan generation: the
// Reduction row first (version, plan_schema, plan_generation), then counts,
// figures, token and the page at that generation.

// ErrReductionUpgrading answers a Reduction whose plan is still the legacy JSON
// document: the migration has not moved it to tables yet (design D8, HTTP 503).
var ErrReductionUpgrading = errors.New("this Resource Reduction is being upgraded; try again shortly")

const (
	// reductionDisplayTextBytes bounds every text field that crosses into Go on
	// the review and list paths (names, original names, content types).
	reductionDisplayTextBytes = 1024
	// reductionSmallCluster is the size up to which a Cluster always renders in full.
	reductionSmallCluster = 12
	// reductionDefaultPageMemberBudget is the default of reductionPageMemberBudget.
	reductionDefaultPageMemberBudget = 2000
	// reductionNoGeneration is the generation a Reduction with no landed plan reads
	// at: none, so nothing matches.
	reductionNoGeneration = "none:no-plan-landed"
)

// reductionPageMemberBudget is the members the Clusters over reductionSmallCluster of one
// page share. One past the budget shows its Winner and a members cursor. The environment
// variable REDUCTION_PAGE_MEMBER_BUDGET sets it: it exists so the browser tests can reach
// a Cluster cut at the budget without uploading two thousand files, and a deployment has
// no reason to set it.
var reductionPageMemberBudget = pageMemberBudgetFromEnv(os.Getenv("REDUCTION_PAGE_MEMBER_BUDGET"))

// SetReductionPageMemberBudgetForTest sets the page member budget and returns the restore.
func SetReductionPageMemberBudgetForTest(n int) (restore func()) {
	old := reductionPageMemberBudget
	reductionPageMemberBudget = n
	return func() { reductionPageMemberBudget = old }
}

// pageMemberBudgetFromEnv is the budget a REDUCTION_PAGE_MEMBER_BUDGET value names; anything
// that is not a positive number leaves the default.
func pageMemberBudgetFromEnv(raw string) int {
	if n, err := strconv.Atoi(raw); err == nil && n > 0 {
		return n
	}
	return reductionDefaultPageMemberBudget
}

// reductionGenerationOf is the generation a Reduction's plan is read at.
func reductionGenerationOf(r *models.ResourceReduction) string {
	if r.PlanGeneration == nil || *r.PlanGeneration == "" {
		return reductionNoGeneration
	}
	return *r.PlanGeneration
}

// boundedText is a SQL expression for the first reductionDisplayTextBytes bytes of
// a text column, as bytes (BLOB on SQLite, bytea on Postgres), so a multibyte
// character cut in half never reaches a text type in the driver. Go trims the
// remainder to a rune boundary (trimDisplayText).
func (s *reductionStore) boundedText(expr string) string {
	if s.postgres() {
		return "substring(convert_to(" + expr + ", 'UTF8') from 1 for " + strconv.Itoa(reductionDisplayTextBytes) + ")"
	}
	return "substr(CAST(" + expr + " AS BLOB), 1, " + strconv.Itoa(reductionDisplayTextBytes) + ")"
}

// trimDisplayText turns bounded bytes into text, dropping an incomplete rune the
// byte cut left at the end. A value shorter than the bound is returned untouched.
func trimDisplayText(b []byte) string {
	if len(b) < reductionDisplayTextBytes {
		return string(b)
	}
	for i := 1; i < utf8.UTFMax && i <= len(b); i++ {
		if utf8.RuneStart(b[len(b)-i]) {
			if !utf8.FullRune(b[len(b)-i:]) {
				b = b[:len(b)-i]
			}
			break
		}
	}
	return string(b)
}

// reductionClusterExtra is what the page needs to say about a Cluster that
// contracts.ReductionClusterView has no field for yet; the provider that wires
// this reader carries it onto the view.
type reductionClusterExtra struct {
	// MemberCount is every stored member; MembersShown how many the page carries.
	MemberCount  int
	MembersShown int
	// Truncated counts eligible copies the compute's member cap left out (D6). Always
	// zero for a scoped viewer: omitted copies may lie outside their scope.
	Truncated int
	// MoreMembers is true when members were left out of this page, and MembersAfter
	// the cursor to ask for the next ones from ("" is from the start). A Cluster past
	// the page budget shows only its Winner and has MembersAfter "".
	MoreMembers  bool
	MembersAfter string
}

// reductionReviewExtras is the review's side output, keyed by Cluster key.
type reductionReviewExtras struct {
	Clusters map[string]reductionClusterExtra
}

// loadReductionForStoreRead reads a Reduction under the visibility predicate with
// the ordinary projection (never the legacy plan and extent documents).
func (ctx *MahresourcesContext) loadReductionForStoreRead(id uint, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	var row models.ResourceReduction
	err := ctx.db.Select(ctx.reductionStore().boundedReductionColumns()).
		Scopes(database_scopes.ResourceReductionQuery(reductionQueryFor(ownerUserID, ownerRestricted), true)).
		Where("id = ?", id).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrReductionNotFound
	}
	if err != nil {
		return nil, err
	}
	row.Name = trimDisplayText([]byte(row.Name))
	return &row, nil
}

// boundedReductionColumns is reductionColumns with the name cut to
// reductionDisplayTextBytes in SQL, so a name stored at its full length never
// crosses into Go; trimDisplayText finishes the cut at a rune boundary.
func (s *reductionStore) boundedReductionColumns() string {
	return strings.Replace(reductionColumns, " name, ", " "+s.boundedText("name")+" AS name, ", 1)
}

// storeGetReductionReview is GetReductionReview over the plan tables. One read
// snapshot, the same contracts.ReductionReview, plus the per-Cluster extras the
// contract cannot carry yet.
func (ctx *MahresourcesContext) storeGetReductionReview(id uint, ownerUserID *uint, ownerRestricted bool, page int, query *query_models.ResourceReductionReviewQuery) (*contracts.ReductionReview, *reductionReviewExtras, error) {
	var review *contracts.ReductionReview
	var extras *reductionReviewExtras
	err := ctx.withReadSnapshot(func(snap *MahresourcesContext) error {
		var err error
		review, extras, err = snap.storeReviewInSnapshot(id, ownerUserID, ownerRestricted, page, query)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return review, extras, nil
}

type storedLastApply struct {
	models.ReductionLastApply
	BatchID string `json:"batchId"`
}

func decodeStoredJSON(raw []byte, into any) (bool, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return false, nil
	}
	return true, json.Unmarshal(raw, into)
}

func (ctx *MahresourcesContext) storeReviewInSnapshot(id uint, ownerUserID *uint, ownerRestricted bool, page int, query *query_models.ResourceReductionReviewQuery) (*contracts.ReductionReview, *reductionReviewExtras, error) {
	reduction, err := ctx.loadReductionForStoreRead(id, ownerUserID, ownerRestricted)
	if err != nil {
		return nil, nil, err
	}
	if reduction.PlanSchema == 0 {
		return nil, nil, ErrReductionUpgrading
	}
	generation := reductionGenerationOf(reduction)
	store := ctx.reductionStore()
	scoped := ctx.isScopedPrincipal()
	now := time.Now() // one reading for the lease checks, so running and settle agree

	if page < 1 {
		page = 1
	}
	pageSize := ReductionClustersPerPage
	if query != nil {
		pageSize = normalizeReviewPageSize(query.PageSize)
	}

	tabs, err := store.tabCounts(reduction, generation)
	if err != nil {
		return nil, nil, err
	}
	figures, err := store.figures(reduction, generation)
	if err != nil {
		return nil, nil, err
	}
	token, _, _, err := store.applyEligibility(reduction, generation)
	if err != nil {
		return nil, nil, err
	}
	settleable, err := store.settleableClaims(reduction, generation, now)
	if err != nil {
		return nil, nil, err
	}
	rows, clusterCount, err := store.pageClusters(reduction, generation, withDistinctTiers(query), (page-1)*pageSize, pageSize)
	if err != nil {
		return nil, nil, err
	}
	views, extras, err := ctx.hydrateReviewPage(store, reduction, rows, scoped)
	if err != nil {
		return nil, nil, err
	}

	figures.Apply.Identical.KeepAsVersion = reduction.KeepAsVersionIdentical
	figures.Apply.Near.KeepAsVersion = reduction.KeepAsVersionNear

	// The coverage, the drift report and the selection preview.
	var coverage models.ReductionCoverage
	if _, err := decodeStoredJSON(reduction.Coverage, &coverage); err != nil {
		return nil, nil, err
	}
	sel, err := store.selectionSummary(reduction)
	if err != nil {
		return nil, nil, err
	}
	entered, enteredCapped := 0, false
	if reduction.ComputedAt != nil {
		if entered, enteredCapped, err = store.enteredSince(reduction, *reduction.ComputedAt); err != nil {
			return nil, nil, err
		}
	}
	// Coverage answers a question about the whole Extent, so only a principal who
	// can see all of it is shown it (the JSON reader's rule, kept).
	coverageTrusted := !scoped && sel.resources == sel.storedResources && sel.groups == sel.storedGroups
	extentSize := coverage.ExtentSize
	if !coverageTrusted {
		coverage = models.ReductionCoverage{}
		extentSize = 0
	}

	var settings *models.ReductionSettings
	{
		var s models.ReductionSettings
		if ok, err := decodeStoredJSON(reduction.PlanSettings, &s); err != nil {
			return nil, nil, err
		} else if ok {
			settings = &s
		}
	}

	var lastView *contracts.ReductionLastApplyView
	{
		var rec storedLastApply
		if ok, err := decodeStoredJSON(reduction.LastApply, &rec); err != nil {
			return nil, nil, err
		} else if ok {
			lastView = describeStoredLastApply(reduction, &rec.ReductionLastApply, now)
			// The counts are shown to anyone only with evidence (design D10): the
			// record names a batch, exactly that many Clusters carry it, and (for a
			// scoped viewer) none of them is withheld. Without it the counts are
			// unavailable, for an administrator too: a record whose Clusters cannot
			// be accounted for is not a number to act on.
			visible, err := store.lastApplyCountsVisible(reduction, generation, rec.BatchID, rec.Applied+rec.Stale)
			if err != nil {
				return nil, nil, err
			}
			if !visible {
				lastView.Applied, lastView.Removed, lastView.Stale = 0, 0, 0
				lastView.CountsHidden = true
			}
		}
	}

	return &contracts.ReductionReview{
		SelectionGroups:     sel.groupRefs,
		SelectionResources:  sel.resourceRefs,
		MoreGroups:          sel.moreGroups,
		MoreResources:       sel.moreResources,
		Reduction:           reduction,
		Status:              EffectiveReductionStatus(reduction),
		Coverage:            coverage,
		CoverageTrusted:     coverageTrusted,
		Clusters:            views,
		ClusterCount:        clusterCount,
		CheckedCount:        figures.Checked,
		CheckedLoserCount:   figures.CheckedLosers,
		Page:                page,
		PageSize:            pageSize,
		SelectedResources:   sel.resources,
		SelectedGroups:      sel.groups,
		ExtentSize:          extentSize,
		EnteredSinceCompute: entered,
		EnteredSinceCapped:  enteredCapped,
		ExtentAll:           reduction.ExtentAll,
		TabCounts:           tabs,
		ApplyFigures:        figures.Apply,
		ApplyToken:          token,
		SettleableClaims:    settleable,
		Bulk:                figures.Bulk,
		SettingsChanged:     reductionSettingsChanged(reduction, settings),
		LastApply:           lastView,
	}, extras, nil
}

// reductionSelection is the review's view of what was selected: counts as the
// current principal sees them, the first names, and the stored totals the
// coverage trust rule compares against.
type reductionSelectionSummary struct {
	resources, groups             int
	storedResources, storedGroups int
	resourceRefs, groupRefs       []contracts.ReductionNamedRef
	moreResources, moreGroups     int
}

// selectionSummary counts and names the Reduction's explicit selection through
// the principal's subtree: a Resource or Group outside it is neither named nor
// counted (design D5, D8). A whole-library Reduction names nothing, but a selection
// kept from before it was widened is still counted, as the JSON reader counts it.
func (s *reductionStore) selectionSummary(r *models.ResourceReduction) (*reductionSelectionSummary, error) {
	out := &reductionSelectionSummary{}
	if s.scope.mode == scopeDeny {
		return out, nil
	}
	prefix, args := s.with()
	resVisible, groupVisible := "", ""
	if s.scope.mode == scopeSubtree {
		resVisible = " AND x.owner_id IN " + subtreeIDs
		groupVisible = " AND x.id IN " + subtreeIDs
	}
	count := func(sql string, extra ...any) (int, error) {
		var n int64
		err := s.db.Raw(sql, extra...).Scan(&n).Error
		return int(n), err
	}
	var err error
	if out.storedResources, err = count(`SELECT COUNT(*) FROM reduction_extent_resources WHERE reduction_id = ?`, r.ID); err != nil {
		return nil, err
	}
	if out.storedGroups, err = count(`SELECT COUNT(*) FROM reduction_extent_groups WHERE reduction_id = ?`, r.ID); err != nil {
		return nil, err
	}
	if out.resources, err = count(prefix+`SELECT COUNT(*) FROM reduction_extent_resources e JOIN resources x ON x.id = e.resource_id WHERE e.reduction_id = ?`+resVisible,
		append(append([]any{}, args...), r.ID)...); err != nil {
		return nil, err
	}
	if out.groups, err = count(prefix+`SELECT COUNT(*) FROM reduction_extent_groups e JOIN groups x ON x.id = e.group_id WHERE e.reduction_id = ?`+groupVisible,
		append(append([]any{}, args...), r.ID)...); err != nil {
		return nil, err
	}
	named := func(sql string, extra ...any) ([]contracts.ReductionNamedRef, error) {
		var rows []struct {
			ID   uint
			Name []byte
		}
		if err := s.db.Raw(sql, extra...).Scan(&rows).Error; err != nil {
			return nil, err
		}
		refs := make([]contracts.ReductionNamedRef, 0, len(rows))
		for _, row := range rows {
			refs = append(refs, contracts.ReductionNamedRef{ID: row.ID, Name: trimDisplayText(row.Name)})
		}
		return refs, nil
	}
	if r.ExtentAll {
		return out, nil
	}
	out.moreResources = max(0, out.resources-reductionSelectionNamesShown)
	out.moreGroups = max(0, out.groups-reductionSelectionNamesShown)
	if out.resources > 0 {
		if out.resourceRefs, err = named(prefix+`SELECT x.id AS id, `+s.boundedText("x.name")+` AS name FROM reduction_extent_resources e JOIN resources x ON x.id = e.resource_id
			WHERE e.reduction_id = ?`+resVisible+` ORDER BY x.id LIMIT ?`, append(append([]any{}, args...), r.ID, reductionSelectionNamesShown)...); err != nil {
			return nil, err
		}
	}
	if out.groups > 0 {
		if out.groupRefs, err = named(prefix+`SELECT x.id AS id, `+s.boundedText("x.name")+` AS name FROM reduction_extent_groups e JOIN groups x ON x.id = e.group_id
			WHERE e.reduction_id = ?`+groupVisible+` ORDER BY x.id LIMIT ?`, append(append([]any{}, args...), r.ID, reductionSelectionNamesShown)...); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// enteredSince counts the Resources that joined the selection after the given
// instant, the drift figure: the explicit ones, the ones a selected Group (or any
// descendant) owns or is related to, or everything for a whole-library Reduction,
// each once and only as the principal may see it.
func (s *reductionStore) enteredSince(r *models.ResourceReduction, since time.Time) (int, bool, error) {
	return s.enteredSinceWithin(r, since, reductionArrivalCap)
}

// enteredSinceWithin is enteredSince examining at most limit arrivals. Past it the
// count is a floor, not a figure: capped is true and n is how many of the first limit
// arrivals belong to the selection.
func (s *reductionStore) enteredSinceWithin(r *models.ResourceReduction, since time.Time, limit int) (int, bool, error) {
	if s.scope.mode == scopeDeny {
		return 0, false, nil
	}
	prefix, args := s.with()
	scopeCond := ""
	if s.scope.mode == scopeSubtree {
		scopeCond = " AND x.owner_id IN " + subtreeIDs
	}
	where := "x.created_at > ?" + scopeCond
	queryArgs := []any{since}
	if r.ExtentAll {
		var n int64
		if err := s.db.Raw(prefix+"SELECT COUNT(*) FROM resources x WHERE "+where, append(args, queryArgs...)...).Scan(&n).Error; err != nil {
			return 0, false, err
		}
		return int(n), false, nil
	}

	// Walk the arrivals (an indexed range on created_at) in keyset chunks and test each
	// chunk's Extent membership. One statement over the whole predicate would have SQLite
	// materialise every relationship row of every selected Group, however old, before
	// looking at a single arrival.
	anchor := "SELECT g.id FROM groups g WHERE g.id IN (SELECT group_id FROM reduction_extent_groups WHERE reduction_id = ?)"
	if s.scope.mode == scopeSubtree {
		anchor += " AND g.id IN " + subtreeIDs
	}
	extTree := "ext_tree(id) AS (" + anchor + " UNION SELECT g.id FROM groups g INNER JOIN ext_tree et ON g.owner_id = et.id)"
	memberPrefix := "WITH RECURSIVE " + extTree + " "
	memberArgs := []any{r.ID}
	if prefix != "" {
		memberPrefix = strings.TrimSuffix(prefix, " ") + ", " + extTree + " "
		memberArgs = append(append([]any{}, args...), r.ID)
	}
	const member = ` AND (x.id IN (SELECT resource_id FROM reduction_extent_resources WHERE reduction_id = ?)
		OR x.owner_id IN (SELECT id FROM ext_tree)
		OR EXISTS (SELECT 1 FROM groups_related_resources grr WHERE grr.resource_id = x.id AND grr.group_id IN (SELECT id FROM ext_tree)))`

	// The cursor is the arrival's created_at as the database holds it. SQLite keeps a time
	// as text and compares text, so a cursor scanned into a time.Time and bound back is
	// rewritten (10:00+01:00 returns as 09:00+00:00) and sorts before the stored value:
	// the same chunk would come back forever. The text is read and bound as stored.
	type arrival struct {
		ID   uint
		Raw  string
		Time time.Time
	}
	cursorColumns := "x.created_at AS time"
	if !s.postgres() {
		cursorColumns = "CAST(x.created_at AS TEXT) AS raw"
	}
	total, walked := 0, 0
	var cursor *arrival
	for iteration := 0; ; iteration++ {
		if iteration > reductionArrivalMaxChunks {
			return 0, false, fmt.Errorf("the arrivals walk exceeded %d chunks", reductionArrivalMaxChunks)
		}
		// The first chunk starts after the instant; every later one seeks straight
		// past the cursor on (created_at, id) and states nothing else about created_at,
		// because a second range bound on the leading column lets the planner range
		// over everything after the instant and discard the rows before the cursor,
		// which is quadratic in the arrivals.
		scan := "x.created_at > ?" + scopeCond
		scanArgs := []any{since}
		if cursor != nil {
			scan = "(x.created_at, x.id) > (?, ?)" + scopeCond
			if s.postgres() {
				scanArgs = []any{cursor.Time, cursor.ID}
			} else {
				scanArgs = []any{cursor.Raw, cursor.ID}
			}
		}
		// One past the cap is read so a library with exactly the cap is not called over it.
		rows := min(reductionArrivalChunk, limit-walked+1)
		var chunk []arrival
		if err := s.db.Raw(prefix+"SELECT x.id AS id, "+cursorColumns+" FROM resources x WHERE "+scan+" ORDER BY x.created_at, x.id LIMIT ?",
			append(append(append([]any{}, args...), scanArgs...), rows)...).Scan(&chunk).Error; err != nil {
			return 0, false, err
		}
		if len(chunk) == 0 {
			return total, false, nil
		}
		last := chunk[len(chunk)-1]
		over := walked+len(chunk) > limit
		if over {
			chunk = chunk[:limit-walked]
		}
		if len(chunk) > 0 {
			ids := make([]uint, len(chunk))
			for i, row := range chunk {
				ids[i] = row.ID
			}
			var n int64
			if err := s.db.Raw(memberPrefix+"SELECT COUNT(*) FROM resources x WHERE x.id IN ?"+member, append(append([]any{}, memberArgs...), ids, r.ID)...).Scan(&n).Error; err != nil {
				return 0, false, err
			}
			total += int(n)
		}
		walked += len(chunk)
		if over {
			return total, true, nil
		}
		if rows < reductionArrivalChunk || len(chunk) < rows {
			return total, false, nil
		}
		if cursor != nil && last.ID == cursor.ID && last.Raw == cursor.Raw && last.Time.Equal(cursor.Time) {
			return 0, false, fmt.Errorf("the arrivals walk did not advance past resource %d", last.ID)
		}
		cursor = &last
	}
}

// reductionArrivalCap is how many arrivals the drift report examines. Each chunk tests
// membership against the Extent, whose Group subtree can be tens of thousands of rows,
// so a library that took a million uploads since the plan was computed would pay that
// per chunk. Past the cap the page says "N+" instead of a figure.
var reductionArrivalCap = 10000

// reductionArrivalMaxChunks bounds the walk: a chunk that never advances fails loudly
// instead of spinning. A var so a test can lower it.
var reductionArrivalMaxChunks = 1 << 24

// reductionArrivalChunk is how many arrivals one membership test covers. A variable so a
// test can cross chunk boundaries with a few rows.
var reductionArrivalChunk = 500

// describeStoredLastApply is the stored apply record as the page shows it. Whether
// it is running is read from the leases, not a clock on its start: a worker that
// died leaves a record with no finish, and its lease expires (10 minutes) long
// before any start-time ceiling, so the live lease says running and an expired one
// says interrupted, which is what shows the recovery control.
func describeStoredLastApply(r *models.ResourceReduction, last *models.ReductionLastApply, now time.Time) *contracts.ReductionLastApplyView {
	view := describeLastApply(last)
	unfinished := last.FinishedAt == nil
	nanos := now.UnixNano()
	live := r.ApplyLeaseUntil > nanos || (r.SettleOp != nil && r.SettleLeaseUntil > nanos)
	view.Running = unfinished && live
	view.Interrupted = unfinished && !live
	return view
}

// withDistinctTiers returns the query with its tier filter's values reduced to the
// distinct ones, and every value no Cluster can hold collapsed to one (it still
// matches nothing). A request that repeats a value would otherwise bind each copy.
func withDistinctTiers(q *query_models.ResourceReductionReviewQuery) *query_models.ResourceReductionReviewQuery {
	if q == nil || len(q.Tier) < 2 {
		return q
	}
	seen := map[string]bool{}
	unknown := false
	var tiers []string
	for _, tier := range q.Tier {
		switch {
		case tier == models.ReductionTierIdentical || tier == models.ReductionTierNear:
			if !seen[tier] {
				seen[tier] = true
				tiers = append(tiers, tier)
			}
		case !unknown:
			unknown = true
			tiers = append(tiers, tier)
		}
	}
	cp := *q
	cp.Tier = tiers
	return &cp
}
