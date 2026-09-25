package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
)

// This file holds the read side of the control plane: the one query constructor
// every Job read goes through, the listing that paginates it, and the bounded
// views those reads return. Command filters compose exact per-Kind database
// selectors into that same visible query before a page or summary is formed.
//
// The constructor is the point of the file. A Job is reachable through a list, a
// detail read, a timeline join, an output lookup, a lineage join, the resumable
// event scan and an aggregate, and every one of those has to agree about which
// Jobs the asker may see — otherwise a hidden Job is counted in a summary the
// listing does not show, or an event arrives for a Job the timeline refuses.
// There is one place the predicate is written, and no read adds a second.

// jobQuery is the constructor: a query over jobs carrying the shared visibility
// predicate and nothing else. Callers add their own predicates on top.
//
// A Kind that needs stricter inspection than the base rule does not get a second
// predicate here. The rule has to compile to a durable column — which is what
// `visibility_class` is, fixed by the registered Kind at acceptance and read by
// this one predicate — because a read that narrowed in Go would answer its page
// from one set and its counts from another. Adapters may narrow through the
// fields they set at acceptance; they may never widen past this.
func jobQuery(db *gorm.DB, access Access) *gorm.DB {
	return visibleTo(db, access)
}

// visibleJobIDs is the same predicate as a subquery, for the reads that join
// onto something else — events, outputs, links — rather than reading Jobs
// themselves. Joining through it is what keeps a hidden Job's events out of the
// resumable stream without a second spelling of who may see what.
func visibleJobIDs(db *gorm.DB, access Access) *gorm.DB {
	return jobQuery(db.Model(&models.Job{}), access).Select("jobs.id")
}

// requireVisibleJob reports whether the asker may read one Job, answering
// ErrNotFound when they may not. A hidden Job and a missing one are the same
// answer on purpose: an unauthorized probe must not be able to tell them apart.
func requireVisibleJob(db *gorm.DB, access Access, jobID string) error {
	if strings.TrimSpace(jobID) == "" {
		return fmt.Errorf("%w: empty job id", ErrNotFound)
	}
	var present int64
	err := jobQuery(db.Model(&models.Job{}), access).Where("jobs.id = ?", jobID).Count(&present).Error
	if err != nil {
		return fmt.Errorf("jobs: resolve visible job: %w", err)
	}
	if present == 0 {
		return fmt.Errorf("%w: %s", ErrNotFound, jobID)
	}
	return nil
}

// List returns one bounded, newest-first page of the Jobs the asker may see.
//
// Ordering is accepted_at then identity, and the cursor is that pair, because
// accepted_at is not unique: a page boundary drawn on the instant alone would
// either repeat or skip every Job accepted in the same tick.
func (s *Service) List(deps Deps, access Access, filter Filter, cursor Cursor, limit int) (Page, error) {
	// One row beyond the page is read so the answer says whether there is a next
	// page without a second query, and without ever reporting a next page that
	// turns out to be empty.
	if err := validateCursor(cursor); err != nil {
		return Page{}, err
	}
	rows, size, err := s.readListRows(deps, access, filter, limit, false, true, func(query *gorm.DB) *gorm.DB {
		return continueAfter(query, cursor)
	})
	if err != nil {
		return Page{}, fmt.Errorf("jobs: list: %w", err)
	}

	hasNext := len(rows) > size
	if hasNext {
		rows = rows[:size]
	}
	page := Page{Jobs: make([]Snapshot, 0, len(rows))}
	for _, row := range rows {
		page.Jobs = append(page.Jobs, viewerSnapshot(row, access))
	}
	if hasNext {
		page.Next = cursorOf(rows[len(rows)-1])
	}
	// A page that did not start at the top has one before it. Its first row is
	// where ListBefore walks back from; if everything newer has since gone,
	// ListBefore answers the first page rather than an empty one. A page that
	// came back empty — its rows dismissed, or moved out of the filter — walks
	// back from its own cursor, so the reader is never stranded without a way to
	// the rows that remain before it.
	if cursor.ID != "" {
		if len(rows) > 0 {
			page.Prev = cursorOf(rows[0])
		} else {
			prev := cursor
			page.Prev = &prev
		}
	}
	return s.finishPage(deps, access, page)
}

// ListBefore returns the page immediately newer than a cursor, still ordered
// newest first: the page a reader goes back to with Previous.
//
// When the rows newer than the cursor no longer fill a page — the reader walked
// forward with a smaller page size, or Jobs were removed — the answer is the
// listing's first page rather than a short slice of the top, so the page a
// reader lands on is always one the forward walk would also have shown.
func (s *Service) ListBefore(deps Deps, access Access, filter Filter, before Cursor, limit int) (Page, error) {
	if before.ID == "" {
		return s.List(deps, access, filter, Cursor{}, limit)
	}
	if err := validateCursor(before); err != nil {
		return Page{}, err
	}
	rows, size, err := s.readListRows(deps, access, filter, limit, false, false, func(query *gorm.DB) *gorm.DB {
		return continueBefore(query, before)
	})
	if err != nil {
		return Page{}, fmt.Errorf("jobs: list before: %w", err)
	}
	if len(rows) <= size {
		return s.List(deps, access, filter, Cursor{}, limit)
	}
	rows = rows[:size]
	slices.Reverse(rows)

	page := Page{Jobs: make([]Snapshot, 0, len(rows))}
	for _, row := range rows {
		page.Jobs = append(page.Jobs, viewerSnapshot(row, access))
	}
	page.Prev = cursorOf(rows[0])

	// Whether anything is older than this page is asked rather than assumed: the
	// cursor's own row may be gone, and a Next that opens an empty page is the
	// answer List promises never to give.
	last := *cursorOf(rows[len(rows)-1])
	older, _, err := s.readListRows(deps, access, filter, limit, true, true, func(query *gorm.DB) *gorm.DB {
		return continueAfter(query, last)
	})
	if err != nil {
		return Page{}, fmt.Errorf("jobs: list before: %w", err)
	}
	if len(older) > 0 {
		page.Next = &last
	}
	return s.finishPage(deps, access, page)
}

// CountByState counts the visible Jobs matching a filter, grouped by state, with
// no summary window. It answers the same question List does, so a count shown
// beside a link is the number of rows the link opens; Summary's window would make
// an old failure that still needs attention disappear from the count while
// remaining in the list.
func (s *Service) CountByState(deps Deps, access Access, filter Filter) (map[string]int64, error) {
	branches := listBranches(filter)
	queries := make([]*gorm.DB, 0, len(branches))
	for _, branch := range branches {
		query, _, err := s.listQuery(deps, access, branch, Cursor{}, 0)
		if err != nil {
			return nil, err
		}
		queries = append(queries, query)
	}
	if len(queries) == 1 {
		return countByColumn(queries[0], "state")
	}
	// The branches are disjoint, so their counts add — in one statement, so both
	// are counted from one snapshot and a Job that moved between them in the
	// meantime is not counted twice.
	grouped := func(query *gorm.DB) *gorm.DB {
		return query.Select("jobs.state AS value, COUNT(*) AS count").Group("jobs.state")
	}
	var rows []struct {
		Value string
		Count int64
	}
	if err := deps.DB.Raw("SELECT value, SUM(count) AS count FROM (SELECT value, count FROM (?) AS a UNION ALL SELECT value, count FROM (?) AS b) AS branches GROUP BY value",
		grouped(queries[0]), grouped(queries[1])).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("jobs: aggregate by state: %w", err)
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Value] = row.Count
	}
	return counts, nil
}

// listBranches is the filter as the listing reads it: itself, or — when the
// partial token sits beside other states that do not already include it — two
// filters, the other states and the partial token alone. They are disjoint (the
// first cannot hold a succeeded Job), each is one indexed seek, and List merges
// them by the keyset. As one ordinary predicate neither spelling seeks both: a
// bare OR lets the planner walk the accepted-order index filtering as it goes,
// and state IN (..., succeeded) reads every succeeded entry to test its phase —
// tens of milliseconds at 400,000 Jobs for a handful of matches, against
// microseconds for either branch alone. The listing combines the branches in one
// statement (readListRows); every other reader gets their ids as one predicate
// (applyFilter).
func listBranches(filter Filter) []Filter {
	partial := false
	var others []string
	for _, token := range filter.States {
		if token == FilterStatePartial {
			partial = true
			continue
		}
		others = append(others, token)
	}
	if !partial || len(others) == 0 || slices.Contains(others, string(StateSucceeded)) {
		return []Filter{filter}
	}
	rest, partialOnly := filter, filter
	rest.States = others
	partialOnly.States = []string{FilterStatePartial}
	return []Filter{rest, partialOnly}
}

// readListRows reads one keyset-bounded run of a listing, newest first when desc,
// oldest first otherwise: at most size+1 rows, or one when probe asks only
// whether any row is there. position applies the keyset bound to each branch.
//
// With two branches (listBranches) each is ordered and limited on its own index
// and the two are combined in ONE statement. Separate statements would read two
// snapshots, and a Job moving from one branch to the other between them — a
// running Job succeeding as partial — would be returned twice.
func (s *Service) readListRows(deps Deps, access Access, filter Filter, limit int, probe, desc bool, position func(*gorm.DB) *gorm.DB) ([]models.Job, int, error) {
	query, size, union, err := s.listRowsQuery(deps, access, filter, limit, probe, desc, position)
	if err != nil {
		return nil, 0, err
	}
	var rows []models.Job
	if union {
		err = query.Scan(&rows).Error
	} else {
		err = query.Find(&rows).Error
	}
	return rows, size, err
}

// listRowsQuery builds the statement readListRows runs, so a test can explain
// exactly what the listing executes. union reports whether it is the raw
// two-branch statement (read with Scan) rather than a query-builder one.
func (s *Service) listRowsQuery(deps Deps, access Access, filter Filter, limit int, probe, desc bool, position func(*gorm.DB) *gorm.DB) (*gorm.DB, int, bool, error) {
	order := "jobs.accepted_at DESC, jobs.id DESC"
	outer := "accepted_at DESC, id DESC"
	if !desc {
		order, outer = "jobs.accepted_at ASC, jobs.id ASC", "accepted_at ASC, id ASC"
	}
	branches := listBranches(filter)
	queries := make([]*gorm.DB, 0, len(branches))
	size, take := 0, 1
	for _, branch := range branches {
		query, branchSize, err := s.listQuery(deps, access, branch, Cursor{}, limit)
		if err != nil {
			return nil, 0, false, err
		}
		size = branchSize
		if !probe {
			take = size + 1
		}
		queries = append(queries, position(query.Session(&gorm.Session{})).Order(order).Limit(take))
	}
	if len(queries) == 1 {
		return queries[0], size, false, nil
	}
	return deps.DB.Raw("SELECT * FROM (?) AS a UNION ALL SELECT * FROM (?) AS b ORDER BY "+outer+" LIMIT ?",
		queries[0], queries[1], take), size, true, nil
}

// listQuery validates one listing question and builds its filtered, visible,
// unordered query. List, ListBefore and CountByState share it so that none of
// them can answer a different set than the others.
func (s *Service) listQuery(deps Deps, access Access, filter Filter, cursor Cursor, limit int) (*gorm.DB, int, error) {
	if err := validateFilter(filter); err != nil {
		return nil, 0, err
	}
	size, err := pageSize(limit)
	if err != nil {
		return nil, 0, err
	}
	if err := validateCursor(cursor); err != nil {
		return nil, 0, err
	}
	query, err := applyFilter(jobQuery(deps.DB.Model(&models.Job{}), access), access, filter)
	if err != nil {
		return nil, 0, err
	}
	if filter.Command != "" {
		query, err = s.applyCommandFilter(query, deps, access, filter.Command)
		if err != nil {
			return nil, 0, err
		}
	}
	return query, size, nil
}

// finishPage fills the per-viewer projections a page carries.
func (s *Service) finishPage(deps Deps, access Access, page Page) (Page, error) {
	if err := s.fillReplayAvailability(deps, page.Jobs); err != nil {
		return Page{}, err
	}
	if err := fillViewerPinState(deps.DB, access, page.Jobs); err != nil {
		return Page{}, err
	}
	return page, nil
}

func cursorOf(row models.Job) *Cursor {
	return &Cursor{AcceptedAt: row.AcceptedAt, ID: row.ID}
}

// fillViewerPinState projects one viewer's pin preference onto a bounded set of
// snapshots with one query. Pinning belongs to the viewer, so callers must not
// infer it from retention state or another user's preference row.
func fillViewerPinState(db *gorm.DB, access Access, snapshots []Snapshot) error {
	for i := range snapshots {
		snapshots[i].Pinned = false
	}
	if access.UserID == 0 || len(snapshots) == 0 {
		return nil
	}

	ids := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		ids = append(ids, snapshot.ID)
	}
	var pinnedIDs []string
	if err := db.Model(&models.JobPreference{}).
		Where("user_id = ? AND pinned_at IS NOT NULL AND job_id IN ?", access.UserID, ids).
		Pluck("job_id", &pinnedIDs).Error; err != nil {
		return fmt.Errorf("jobs: read viewer pin state: %w", err)
	}
	pinned := make(map[string]bool, len(pinnedIDs))
	for _, id := range pinnedIDs {
		pinned[id] = true
	}
	for i := range snapshots {
		snapshots[i].Pinned = pinned[snapshots[i].ID]
	}
	return nil
}

// viewerSnapshotWithPin projects one stored Job for a viewer and attaches that
// viewer's pin state. Refusal responses use it so a stale command cannot replace
// a correct badge with the zero value.
func viewerSnapshotWithPin(db *gorm.DB, access Access, job models.Job) (Snapshot, error) {
	snapshots := []Snapshot{viewerSnapshot(job, access)}
	if err := fillViewerPinState(db, access, snapshots); err != nil {
		return Snapshot{}, err
	}
	return snapshots[0], nil
}

// fillReplayAvailability answers each listed Job's replay question from one
// query for the whole page. A listing must not spend a database round trip per
// row — a page of two hundred would be two hundred queries — and it must not
// decrypt anything either: the answer is carried by the envelope's key ID and
// the registered Kind versions.
func (s *Service) fillReplayAvailability(deps Deps, jobs []Snapshot) error {
	ids := make([]string, 0, len(jobs))
	for _, job := range jobs {
		ids = append(ids, job.ID)
	}
	if len(ids) == 0 {
		return nil
	}

	var envelopes []models.JobReplayEnvelope
	if err := deps.DB.Where("job_id IN ?", ids).Find(&envelopes).Error; err != nil {
		return fmt.Errorf("jobs: load replay envelopes: %w", err)
	}
	byJob := make(map[string]models.JobReplayEnvelope, len(envelopes))
	for _, envelope := range envelopes {
		byJob[envelope.JobID] = envelope
	}

	keys := replayKeys(deps)
	now := deps.now()
	for i := range jobs {
		if jobs[i].ReplayClass == ReplayClassNonReplayable {
			jobs[i].ReplayAvailability = ReplayAvailabilityNone
			continue
		}
		envelope, ok := byJob[jobs[i].ID]
		if !ok {
			jobs[i].ReplayAvailability = ReplayUnreadable
			continue
		}
		jobs[i].ReplayAvailability = s.replayAvailabilityFrom(keys, envelope, now)
	}
	return nil
}

// validateFilter checks a filter before anything is read, so a question this
// release cannot answer is refused rather than answered as "no matches".
func validateFilter(filter Filter) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidFilter, fmt.Sprintf(format, args...))
	}

	for _, state := range filter.States {
		if state != FilterStatePartial && !State(state).Valid() {
			return invalid("unknown state %q", state)
		}
	}
	for _, kind := range filter.Kinds {
		if strings.TrimSpace(kind) == "" || len(kind) > MaxKindBytes {
			return invalid("kind filter %q is empty or over its ceiling", kind)
		}
	}
	for _, origin := range filter.Origins {
		if strings.TrimSpace(origin) == "" || len(origin) > MaxOriginBytes {
			return invalid("origin filter %q is empty or over its ceiling", origin)
		}
	}
	for _, relation := range []struct{ name, value string }{
		{"relationship", filter.Relationship},
		{"inboundRelationship", filter.InboundRelationship},
		{"noInboundRelationship", filter.NoInboundRelationship},
	} {
		if relation.value != "" && !knownLinkType(relation.value) {
			return invalid("%s %q is not one of %s", relation.name, relation.value, strings.Join(LinkTypes, ", "))
		}
	}
	if filter.AcceptedAfter != nil && filter.AcceptedBefore != nil &&
		filter.AcceptedAfter.After(*filter.AcceptedBefore) {
		return invalid("the accepted window ends before it starts")
	}
	if filter.Command != "" {
		if strings.TrimSpace(filter.Command) == "" {
			return invalid("command key is empty")
		}
		if len(filter.Command) > MaxCommandKeyBytes {
			return invalid("command key is %d bytes, over the %d-byte ceiling", len(filter.Command), MaxCommandKeyBytes)
		}
	}
	return nil
}

// ValidateFilter checks one list or summary filter before it is accepted or read.
// Kind adapters that persist a filtered request can use the same validation at
// submission time rather than accepting work that will fail only when dispatched.
func ValidateFilter(filter Filter) error { return validateFilter(filter) }

// applyFilter adds the filter's predicates to a visible-Jobs query. The asker is
// passed in because the preference dimensions are the asker's own rows: a
// dismissal belongs to one viewer's list, never to the Job.
func applyFilter(db *gorm.DB, access Access, filter Filter) (*gorm.DB, error) {
	if branches := listBranches(filter); len(branches) == 2 {
		// The partial token beside other states, as one predicate (Summary, and
		// every reader other than the listing, which reads the branches itself):
		// the union of the two branches' ids, each branch the whole filter — the
		// asker's visibility, the summary window, every other dimension — so each
		// is an indexed seek bounded as tightly as a single-state filter is. A
		// branch that carried only its state would materialize every historical
		// match before an outer window threw most of them away.
		fresh := db.Session(&gorm.Session{NewDB: true})
		ids := make([]*gorm.DB, 0, 2)
		for _, branch := range branches {
			query, err := applyFilter(visibleTo(fresh.Model(&models.Job{}), access), access, branch)
			if err != nil {
				return nil, err
			}
			ids = append(ids, query.Select("jobs.id"))
		}
		return db.Where("jobs.id IN (SELECT id FROM (?) AS r UNION ALL SELECT id FROM (?) AS p)", ids[0], ids[1]), nil
	}
	if len(filter.States) > 0 {
		db = applyStateFilter(db, filter.States)
	}
	if len(filter.Kinds) > 0 {
		db = db.Where("jobs.kind IN ?", filter.Kinds)
	}
	if len(filter.Origins) > 0 {
		db = db.Where("jobs.origin IN ?", filter.Origins)
	}
	if filter.OwnerID != nil {
		db = db.Where("jobs.owner_user_id = ?", *filter.OwnerID)
	}
	if filter.ActorID != nil {
		db = db.Where("jobs.actor_user_id = ?", *filter.ActorID)
	}
	if filter.AcceptedAfter != nil {
		db = db.Where("jobs.accepted_at >= ?", filter.AcceptedAfter.UTC())
	}
	if filter.AcceptedBefore != nil {
		db = db.Where("jobs.accepted_at <= ?", filter.AcceptedBefore.UTC())
	}
	if filter.Relationship != "" {
		db = db.Where("EXISTS (?)", linkedJobs(db, access, filter.Relationship, "from_job_id", "to_job_id"))
	}
	if filter.InboundRelationship != "" {
		db = db.Where("EXISTS (?)", linkedJobs(db, access, filter.InboundRelationship, "to_job_id", "from_job_id"))
	}
	if filter.NoInboundRelationship != "" {
		db = db.Where("NOT EXISTS (?)", linkedJobs(db, access, filter.NoInboundRelationship, "to_job_id", "from_job_id"))
	}
	if filter.Pinned != nil {
		db = db.Where(preferencePredicate("pinned_at", *filter.Pinned), access.UserID)
	}
	if filter.Dismissed != nil {
		db = db.Where(preferencePredicate("dismissed_at", *filter.Dismissed), access.UserID)
	}
	if filter.Search != "" {
		db = applySearch(db, filter.Search)
	}
	return db, nil
}

// applyStateFilter narrows to the named states. FilterStatePartial is succeeded
// with the partial phase. Beside other states it never reaches here as a mixed
// list — applyFilter reads that as two branches — so what remains is the token
// alone, or with succeeded, which already includes every partial Job.
func applyStateFilter(db *gorm.DB, tokens []string) *gorm.DB {
	states := make([]string, 0, len(tokens))
	partial := false
	for _, token := range tokens {
		if token == FilterStatePartial {
			partial = true
			continue
		}
		states = append(states, token)
	}
	if partial && len(states) == 0 {
		return db.Where("jobs.state = ? AND jobs.phase = ?", string(StateSucceeded), PhasePartial)
	}
	return db.Where("jobs.state IN ?", states)
}

// linkedJobs is the correlated subquery behind the three lineage filters: the
// links of one type whose near column is the outer Job and whose far endpoint
// the asker may see. Relationship reads a link from its FROM end (the successor
// a Retry, Continue or Repeat created, or the parent of a child stage); the
// inbound filters read it from its TO end (the Job that was retried or repeated,
// or a child stage). The columns are the relation's own spelling, so a filter
// and a write agree about what "this Job is a retry of that one" points at.
//
// The far endpoint is filtered by the same visibility rule every other read
// uses, because a relation is a fact about two Jobs: matching on the link row
// alone made a visible Job whose only relative was hidden answer "yes" to "does
// this have a relative", which is the existence of the hidden relationship
// published as a filter result — and counted as one in an aggregate. Under NOT
// EXISTS the same rule makes a Job whose only successor is hidden read as not
// followed. Lineage already drops those relatives; the filters agree with it.
//
// Each link is checked against its own far Job rather than against "IN (every
// Job the asker may see)", which materializes that whole set — all of them, for
// an administrator — before one page is read. The far Job is its own FROM clause
// so visibleTo's unqualified columns resolve to it, and it is built on a fresh
// statement so it carries the visibility rule and nothing else: inherited
// conditions would ask the far endpoint to satisfy the asker's own filters,
// which would hide relations rather than authorize them.
func linkedJobs(db *gorm.DB, access Access, linkType, nearColumn, farColumn string) *gorm.DB {
	fresh := db.Session(&gorm.Session{NewDB: true})
	far := visibleTo(fresh.Table("jobs AS far"), access).
		Select("1").Where("far.id = l." + farColumn)
	return fresh.Table("job_links AS l").Select("1").
		Where("l.type = ? AND l."+nearColumn+" = jobs.id", linkType).
		Where("EXISTS (?)", far)
}

// preferencePredicate is one viewer-preference predicate over the asker's own
// rows: a Job the asker has (or has not) pinned or dismissed. A viewer with no
// user id owns no preferences, which the comparison answers for free — no
// preference row names user 0.
func preferencePredicate(column string, want bool) string {
	clause := "SELECT 1 FROM job_preferences p WHERE p.job_id = jobs.id AND p.user_id = ? AND p." + column + " IS NOT NULL"
	if want {
		return "EXISTS (" + clause + ")"
	}
	return "NOT EXISTS (" + clause + ")"
}

// applySearch narrows a listing to the bounded sanitized text a viewer may read:
// the Job's identity, title, sanitized summary and sanitized failure message,
// and the labels of its outputs.
//
// Two things are deliberately out of reach. The replay envelope is another
// table, and its contents are ciphertext — a search can never reach them. The
// failure's diagnostic reference is on this table but is a protected
// administrator-facing pointer rather than a message for a person, so the box
// that searches what a viewer can read does not search it: naming it would turn
// an internal path into an oracle.
func applySearch(db *gorm.DB, term string) *gorm.DB {
	if database_scopes.LikeTermIsUnmatchable(term) {
		// A term no stored text can hold: a NUL, or bytes that are not valid
		// UTF-8 (which PostgreSQL refuses inside a text parameter outright).
		// "Matches nothing" is both the safe answer and the true one.
		return db.Where("1 = 0")
	}
	pattern, escape := database_scopes.LikePattern(term)
	operator := database_scopes.GetLikeOperator(db)
	return db.Where(
		"(jobs.id "+operator+" ?"+escape+
			" OR jobs.title "+operator+" ?"+escape+
			" OR COALESCE(CAST(jobs.summary AS TEXT), '') "+operator+" ?"+escape+
			" OR jobs.failure_message "+operator+" ?"+escape+
			" OR EXISTS (SELECT 1 FROM job_outputs o WHERE o.job_id = jobs.id AND o.label "+operator+" ?"+escape+"))",
		pattern, pattern, pattern, pattern, pattern,
	)
}

// continueAfter applies a keyset position. A zero cursor is the start of the
// listing; a cursor naming an instant but no identity is refused by
// validateCursor, because it names a boundary that cannot be walked.
func continueAfter(db *gorm.DB, cursor Cursor) *gorm.DB {
	if cursor.ID == "" {
		return db
	}
	return db.Where("(jobs.accepted_at < ? OR (jobs.accepted_at = ? AND jobs.id < ?))",
		cursor.AcceptedAt.UTC(), cursor.AcceptedAt.UTC(), cursor.ID)
}

// continueBefore applies a keyset position in the other direction: the rows
// newer than the cursor, which ListBefore reads oldest first.
func continueBefore(db *gorm.DB, cursor Cursor) *gorm.DB {
	return db.Where("(jobs.accepted_at > ? OR (jobs.accepted_at = ? AND jobs.id > ?))",
		cursor.AcceptedAt.UTC(), cursor.AcceptedAt.UTC(), cursor.ID)
}

// validateCursor checks a keyset position. The zero value means "from the
// start"; anything else has to name both halves of the key, because the instant
// alone is not unique and a position without an identity has no defined page
// boundary.
func validateCursor(cursor Cursor) error {
	if cursor.ID == "" && cursor.AcceptedAt.IsZero() {
		return nil
	}
	if cursor.ID == "" || cursor.AcceptedAt.IsZero() {
		return fmt.Errorf("%w: a cursor needs both the accepted instant and the job id", ErrInvalidCursor)
	}
	return nil
}

// pageSize resolves a requested page size, refusing one beyond the ceiling
// rather than quietly serving a smaller page than was asked for.
func pageSize(limit int) (int, error) {
	switch {
	case limit == 0:
		return DefaultPageSize, nil
	case limit < 0:
		return 0, fmt.Errorf("%w: %d", ErrInvalidPage, limit)
	case limit > MaxPageSize:
		return 0, fmt.Errorf("%w: %d is over the %d-job ceiling", ErrInvalidPage, limit, MaxPageSize)
	}
	return limit, nil
}

// applyCommandFilter narrows the already-authorized, ordinary-filter query with
// exact selector subqueries. A dynamic advertisement cannot safely be applied
// after pagination or during an unbounded Go scan, so every registered adapter
// must answer this database query contract before command-filter reads are
// enabled.
func (s *Service) applyCommandFilter(base *gorm.DB, deps Deps, access Access, key string) (*gorm.DB, error) {
	if query, handled, err := s.applyHostOnlyCommandFilter(base, deps, access, key); handled || err != nil {
		return query, err
	}

	ctx := context.Background()
	if deps.DB != nil && deps.DB.Statement != nil && deps.DB.Statement.Context != nil {
		ctx = deps.DB.Statement.Context
	}
	selectors := deps.DB.Session(&gorm.Session{NewDB: true}).Model(&models.Job{}).
		Select("jobs.id").Where("1 = 0")
	for _, registration := range s.Registrations() {
		selector, ok := registration.Adapter.(CommandFilterAdapter)
		if !ok {
			return nil, fmt.Errorf("%w: %s v%d has no Kind command selector",
				ErrCommandFilterUnavailable, registration.Definition.Kind, registration.Definition.KindVersion)
		}
		kindQuery := base.Session(&gorm.Session{}).
			Where("jobs.kind = ? AND jobs.kind_version = ?", registration.Definition.Kind, registration.Definition.KindVersion)
		selected, supported, err := selector.SelectCommandJobs(ctx, CommandFilterRequest{
			Deps: deps, Access: access, Key: key, Jobs: kindQuery,
		})
		if err != nil {
			return nil, fmt.Errorf("jobs: select %s command candidates: %w", registration.Definition.Kind, err)
		}
		if !supported {
			if selected != nil {
				return nil, fmt.Errorf("%w: %s v%d returned a selector for an unsupported key",
					ErrCommandFilterUnavailable, registration.Definition.Kind, registration.Definition.KindVersion)
			}
			continue
		}
		if selected == nil {
			return nil, fmt.Errorf("%w: %s v%d returned no selector for %q",
				ErrCommandFilterUnavailable, registration.Definition.Kind, registration.Definition.KindVersion, key)
		}
		// These wrapper predicates are owned by the Service. A selector can only
		// narrow one registered Kind/version and cannot replace its shared read
		// predicate or any request filter.
		kindSelection := deps.DB.Session(&gorm.Session{NewDB: true}).Model(&models.Job{}).
			Select("jobs.id").
			Where("jobs.kind = ? AND jobs.kind_version = ?", registration.Definition.Kind, registration.Definition.KindVersion).
			Where("jobs.id IN (?)", selected)
		selectors = selectors.Or("jobs.id IN (?)", kindSelection)
	}
	query := base.Where("jobs.id IN (?)", selectors)
	return s.applyCommandHostNarrowing(query, deps, key)
}

// applyHostOnlyCommandFilter handles keys the host alone advertises. They do
// not need an adapter selector, because Adapter.Commands entries for these keys
// are ignored by advertisedCommands.
func (s *Service) applyHostOnlyCommandFilter(base *gorm.DB, deps Deps, access Access, key string) (*gorm.DB, bool, error) {
	if !isHostOnlyCommandKey(key) {
		return base, false, nil
	}
	switch key {
	case CommandDismiss:
		if access.UserID == 0 {
			return base.Where("1 = 0"), true, nil
		}
		return base.Where("jobs.state IN ?", terminalJobStates()), true, nil
	case CommandPin, CommandUnpin:
		if access.UserID == 0 {
			return base.Where("1 = 0"), true, nil
		}
		return base, true, nil
	case CommandPinLineage:
		if access.UserID == 0 {
			return base.Where("1 = 0"), true, nil
		}
		return base, true, nil
	case CommandForget:
		query, err := s.applyReplayAvailableFilter(base, deps)
		if err != nil {
			return nil, true, err
		}
		return query.Where("jobs.state IN ?", terminalJobStates()), true, nil
	default:
		return nil, true, fmt.Errorf("%w: host key %q has no query selector", ErrCommandFilterUnavailable, key)
	}
}

// applyCommandHostNarrowing mirrors commandHonorable after the adapter's exact
// selector. These durable host conditions are shared by advertisement and
// execution, so a SQL filter cannot announce a command the Service itself would
// refuse on a visible Job.
func (s *Service) applyCommandHostNarrowing(query *gorm.DB, deps Deps, key string) (*gorm.DB, error) {
	switch key {
	case CommandCancel:
		return query.Where("jobs.state NOT IN ?", terminalJobStates()), nil
	case CommandPause:
		return query.Where("jobs.state = ?", StateRunning).
			Where("(jobs.control_intent IS NULL OR jobs.control_intent <> ?)", ControlIntentCancel), nil
	case CommandResume:
		return query.Where("jobs.state IN ?", []State{StatePaused, StateBlocked}).
			Where("(jobs.control_intent IS NULL OR jobs.control_intent <> ?)", ControlIntentCancel).
			Where("NOT EXISTS (SELECT 1 FROM job_claims c WHERE c.job_id = jobs.id AND c.state IN ?)", unresolvedClaimStates()), nil
	case CommandRetry:
		query = query.Where("jobs.state IN ?", []State{StateFailed, StateCancelled, StateInterrupted}).
			Where("NOT EXISTS (SELECT 1 FROM job_links l WHERE l.type = ? AND l.to_job_id = jobs.id)", string(LinkRetryOf))
		return s.applyReplayAvailableFilter(query, deps)
	case CommandContinue:
		// Continue moves the same linear chain Retry does, so the successor
		// predicate is the Retry one; only the starting state differs. The Kind
		// narrows further (its adapter selects only Jobs it declared unfinished).
		query = query.Where("jobs.state = ?", StateSucceeded).
			Where("NOT EXISTS (SELECT 1 FROM job_links l WHERE l.type = ? AND l.to_job_id = jobs.id)", string(LinkRetryOf))
		return s.applyReplayAvailableFilter(query, deps)
	case CommandRepeat:
		query = query.Where("jobs.state = ?", StateSucceeded)
		return s.applyReplayAvailableFilter(query, deps)
	default:
		return query, nil
	}
}

func (s *Service) applyReplayAvailableFilter(query *gorm.DB, deps Deps) (*gorm.DB, error) {
	keyring := replayKeys(deps)
	if keyring == nil || len(keyring.keys) == 0 {
		return query.Where("1 = 0"), nil
	}
	keyIDs := make([]string, 0, len(keyring.keys))
	for id := range keyring.keys {
		keyIDs = append(keyIDs, id)
	}

	s.replayMu.Lock()
	codecs := make([]replayCodecKey, 0, len(s.replayCodecs))
	for key := range s.replayCodecs {
		codecs = append(codecs, key)
	}
	s.replayMu.Unlock()
	if len(codecs) == 0 {
		return query.Where("1 = 0"), nil
	}
	codecPredicates := make([]string, 0, len(codecs))
	codecArgs := make([]any, 0, 2*len(codecs))
	for _, codec := range codecs {
		codecPredicates = append(codecPredicates, "(e.kind = ? AND e.kind_version = ?)")
		codecArgs = append(codecArgs, codec.kind, codec.version)
	}
	args := []any{deps.now(), keyIDs}
	args = append(args, codecArgs...)
	query = query.Where("jobs.replay_class = ?", ReplayClassReplayable).
		Where("EXISTS (SELECT 1 FROM job_replay_envelopes e WHERE e.job_id = jobs.id AND e.purged_at IS NULL AND (e.expires_at IS NULL OR e.expires_at > ?) AND e.key_id IN ? AND ("+strings.Join(codecPredicates, " OR ")+"))", args...)
	return query, nil
}

func terminalJobStates() []State {
	return []State{StateSucceeded, StateFailed, StateCancelled, StateInterrupted}
}

// knownLinkType reports whether a spelling is one of the lineage relations.
func knownLinkType(value string) bool {
	for _, candidate := range LinkTypes {
		if value == candidate {
			return true
		}
	}
	return false
}

// SetPreference records one viewer's relationship to one Job: dismissed from
// their default list, pinned, or both.
//
// It is a viewer's row, not a fact about the Job, so it changes what *they* see
// and nothing about what happened. Pinning is the exception in reach rather than
// in ownership: it exempts the Job's metadata and events from ordinary retention
// for everybody, which is why it is bounded per viewer — an unbounded pin list
// would be unbounded history — and why it exempts nothing else (an artifact has
// its own expiry, and a relative is a different Job).
//
// A hidden Job is refused exactly as a missing one, and the request is refused
// outright when the principal has no user to belong to: a preference on nobody's
// list cannot be answered. A viewer whose account has been deleted is refused the
// same way, on the durable tombstone their deletion left rather than on a read of
// the account row — see lockPinAdmission.
func (s *Service) SetPreference(deps Deps, access Access, request PreferenceRequest) error {
	if access.UserID == 0 {
		return fmt.Errorf("%w: a preference belongs to a user, and this principal has none", ErrInvalidPreference)
	}
	if request.Dismissed == nil && request.Pinned == nil {
		return fmt.Errorf("%w: the request names neither a dismissal nor a pin", ErrInvalidPreference)
	}
	if err := requireVisibleJob(deps.DB, access, request.JobID); err != nil {
		return err
	}

	now := deps.now()
	return deps.DB.Transaction(func(tx *gorm.DB) error {
		// Three guards are taken before anything is decided, because every decision
		// this write makes is a read another writer can invalidate: whether the
		// viewer still exists, the pin limit — a count of the viewer's rows — and
		// the Job's existence, which retention removes on its own schedule.
		//
		// The per-viewer admission row is inserted first — a write, so SQLite's
		// writer lock is taken before anything is read — and locked for the rest
		// of the transaction where the engine has row locks at all. It is also the
		// durable answer to whether this viewer exists: the account deletion takes
		// the same row and tombstones it in the transaction that sweeps their
		// preferences, so a request authenticated before that deletion and writing
		// after it finds the tombstone rather than an account nobody asked about
		// — and the fence is what keeps the two from committing in either order.
		deleted, err := lockPinAdmission(tx, access.UserID, now)
		if err != nil {
			return err
		}
		if deleted {
			return fmt.Errorf("%w: this viewer's account has been deleted", ErrViewerDeleted)
		}
		// The Job's own row is locked where the engine can: retention's delete
		// takes the same row, so a Job removed between the check above and this
		// write is seen by the recheck below rather than by a race.
		if err := lockPreferenceTarget(tx, access, request.JobID); err != nil {
			return err
		}

		// The row is created on first use and replaced afterwards, because a
		// preference is the pair (job, user) rather than a log of changes.
		preference := models.JobPreference{JobID: request.JobID, UserID: access.UserID, CreatedAt: now, UpdatedAt: now}
		if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "job_id"}, {Name: "user_id"}}, DoNothing: true}).
			Create(&preference).Error; err != nil {
			return fmt.Errorf("jobs: create preference: %w", err)
		}

		// The Job is asked again inside the transaction that writes about it: a
		// preference row referencing a Job nobody can read would outlive the Job
		// it is about — these tables carry no foreign keys, so nothing else would
		// notice.
		if err := requireVisibleJob(tx, access, request.JobID); err != nil {
			return err
		}

		stored := map[string]any{"updated_at": now}
		if request.Pinned != nil {
			if *request.Pinned {
				if err := enforcePinLimit(tx, access.UserID, request.JobID, pinLimit(deps)); err != nil {
					return err
				}
				stored["pinned_at"] = now
			} else {
				stored["pinned_at"] = nil
			}
		}
		if request.Dismissed != nil {
			if *request.Dismissed {
				stored["dismissed_at"] = now
			} else {
				stored["dismissed_at"] = nil
			}
		}
		if err := tx.Model(&models.JobPreference{}).
			Where("job_id = ? AND user_id = ?", request.JobID, access.UserID).
			Updates(stored).Error; err != nil {
			return fmt.Errorf("jobs: store preference: %w", err)
		}

		// A row that answers nothing is dropped rather than kept as evidence of
		// a view that no longer exists.
		var row models.JobPreference
		if err := tx.Where("job_id = ? AND user_id = ?", request.JobID, access.UserID).First(&row).Error; err != nil {
			return fmt.Errorf("jobs: read preference: %w", err)
		}
		if row.PinnedAt == nil && row.DismissedAt == nil {
			if err := tx.Where("job_id = ? AND user_id = ?", request.JobID, access.UserID).
				Delete(&models.JobPreference{}).Error; err != nil {
				return fmt.Errorf("jobs: clear preference: %w", err)
			}
		}
		return nil
	})
}

// lockPinAdmission takes one viewer's admission fence, creating it on first use,
// and reports whether that fence is tombstoned — the durable answer to "has this
// viewer been deleted".
//
// A count is not a guard: two admissions for one viewer at the limit's edge read
// the same count, and nothing about the rows they are inserting conflicts — they
// are different Jobs. Holding one row per viewer across the count is what makes
// the second admission see the first one's committed pin. The same row is what
// the account deletion takes before it sweeps that viewer's preferences, so an
// admission and a deletion of one viewer can only ever be ordered one way.
//
// It is deliberately not the viewer's account row: this is a lock with an
// identity rather than a fact about the account, so it exists in every
// deployment, including the no-auth one where the acting principal's id is the
// root account's, and it puts no new lock edge between retention and user
// administration. The tombstone it also carries is a fact about the viewer, and
// it is written where the account layer deletes one rather than derived from the
// account row here.
func lockPinAdmission(tx *gorm.DB, userID uint, now time.Time) (bool, error) {
	guard := models.JobPinGuard{UserID: userID, CreatedAt: now, UpdatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&guard).Error; err != nil {
		return false, fmt.Errorf("jobs: open pin admission: %w", err)
	}
	// The row is read back in both dialects, because it now carries an answer
	// rather than being a lock alone. On SQLite that read is safe for the module's
	// usual reason — the insert above took the writer lock, so nothing can commit
	// between the two — and on an engine with row locks it is what holds the fence
	// for the rest of the transaction.
	query := tx.Where("user_id = ?", userID)
	if tx.Dialector.Name() != "sqlite" {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	var held models.JobPinGuard
	if err := query.First(&held).Error; err != nil {
		return false, fmt.Errorf("jobs: lock pin admission: %w", err)
	}
	return held.DeletedAt != nil, nil
}

// DeleteViewerPreferences removes one viewer's preference rows and leaves a
// durable tombstone on that viewer's admission fence, so no preference can be
// admitted for them afterwards.
//
// It is a pair rather than a DELETE for the reason the fence exists. A
// preference is written by an admission that serializes on this viewer's guard
// row, and a deletion that only deletes can lose that race: a request
// authenticated before the deletion whose write lands after it inserts a row
// whose viewer no longer exists — and if it is a pin, the Job is exempt from
// retention for everybody, forever, with nobody left who could unpin it. Taking
// the fence first, tombstones included, leaves exactly two orders: either the
// admission committed first and this call sweeps what it wrote, or this call
// committed first and the admission is refused by ErrViewerDeleted.
//
// The caller holds the transaction. It runs inside the account deletion, so the
// tombstone, the swept rows and the removal of the account commit together — and
// it is the first thing that transaction does with any of the viewer's rows,
// because an admission waiting on this fence must never be waiting behind a
// transaction that is itself waiting for a row the admission holds.
func DeleteViewerPreferences(tx *gorm.DB, userID uint, now time.Time) error {
	guard := models.JobPinGuard{UserID: userID, CreatedAt: now, UpdatedAt: now}
	if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&guard).Error; err != nil {
		return fmt.Errorf("jobs: open the deleted viewer's preference fence: %w", err)
	}
	if err := tx.Model(&models.JobPinGuard{}).Where("user_id = ?", userID).
		Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error; err != nil {
		return fmt.Errorf("jobs: tombstone the deleted viewer's preference fence: %w", err)
	}
	if err := tx.Where("user_id = ?", userID).Delete(&models.JobPreference{}).Error; err != nil {
		return fmt.Errorf("jobs: delete the deleted viewer's preferences: %w", err)
	}
	return nil
}

// lockPreferenceTarget locks the Job a preference is about, where the engine has
// row locks at all. It resolves the Job through the shared visibility predicate,
// so a Job the asker may not see is a refusal here exactly as it is above.
func lockPreferenceTarget(tx *gorm.DB, access Access, jobID string) error {
	if tx.Dialector.Name() == "sqlite" {
		return nil
	}
	var job models.Job
	err := jobQuery(tx.Model(&models.Job{}), access).Where("jobs.id = ?", jobID).
		Clauses(clause.Locking{Strength: "UPDATE"}).First(&job).Error
	if err != nil {
		if isNotFound(err) {
			return fmt.Errorf("%w: %s", ErrNotFound, jobID)
		}
		return fmt.Errorf("jobs: lock preference target: %w", err)
	}
	return nil
}

// enforcePinLimit refuses a pin that would take a viewer past the deployment's
// limit. A Job the viewer has already pinned is not counted against itself:
// re-pinning something is the same request, not a second slot.
func enforcePinLimit(tx *gorm.DB, userID uint, jobID string, limit int) error {
	if limit <= 0 {
		return nil
	}
	var pinned int64
	if err := tx.Model(&models.JobPreference{}).
		Where("user_id = ? AND pinned_at IS NOT NULL AND job_id <> ?", userID, jobID).
		Count(&pinned).Error; err != nil {
		return fmt.Errorf("jobs: count pinned jobs: %w", err)
	}
	if pinned >= int64(limit) {
		return fmt.Errorf("%w: %d jobs are already pinned and the limit is %d", ErrPinLimitReached, pinned, limit)
	}
	return nil
}

// pinLimit resolves the deployment's per-viewer pin limit, with the default
// applied when nothing configured one.
func pinLimit(deps Deps) int {
	if deps.PinLimit <= 0 {
		return DefaultPinLimit
	}
	return deps.PinLimit
}

// Timeline returns one Job's events in order, from a sequence onwards, bounded.
//
// Sequencing is the Job's own: a timeline is read by one viewer about one Job,
// so it needs no delivery order — that belongs to the stream, which has to
// order facts from different Jobs. The cursor is exclusive, so a client that has
// consumed an event never sees it twice.
func (s *Service) Timeline(deps Deps, access Access, jobID string, afterSequence uint64, limit int) ([]Event, error) {
	size, err := eventPageSize(limit)
	if err != nil {
		return nil, err
	}
	if err := requireVisibleJob(deps.DB, access, jobID); err != nil {
		return nil, err
	}

	var rows []models.JobEvent
	err = deps.DB.Where("job_id = ? AND sequence > ?", jobID, afterSequence).
		Order("sequence ASC").Limit(size).Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("jobs: read timeline: %w", err)
	}
	return eventsOf(rows), nil
}

// PublishedEvents returns the committed, visible events a subscriber has not
// consumed yet, in delivery order.
//
// Delivery sequence is the correctness mechanism of the resumable stream: it is
// assigned by the post-commit publisher, so it describes the order events became
// durable rather than the order they were allocated. A wake-up is an
// optimization on top of it; this cursor is what makes a reconnect whole.
func (s *Service) PublishedEvents(deps Deps, access Access, afterDelivery uint64, limit int) ([]Event, error) {
	size, err := eventPageSize(limit)
	if err != nil {
		return nil, err
	}
	var rows []models.JobEvent
	err = deps.DB.
		Where("delivery_sequence IS NOT NULL AND delivery_sequence > ?", afterDelivery).
		Where("job_id IN (?)", visibleJobIDs(deps.DB, access)).
		Order("delivery_sequence ASC").Limit(size).Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("jobs: read published events: %w", err)
	}
	return eventsOf(rows), nil
}

// Outputs returns one visible Job's typed outputs.
//
// The Job's visibility is the gate and the output is then authorized on its own:
// visibility of a Job is not a capability token for what it produced, so an
// artifact route re-checks availability and authorization when it opens one.
func (s *Service) Outputs(deps Deps, access Access, jobID string) ([]Output, error) {
	if err := requireVisibleJob(deps.DB, access, jobID); err != nil {
		return nil, err
	}
	var rows []models.JobOutput
	if err := deps.DB.Where("job_id = ?", jobID).Order("key ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("jobs: read outputs: %w", err)
	}
	outputs := make([]Output, 0, len(rows))
	for _, row := range rows {
		outputs = append(outputs, outputView(row))
	}
	return outputs, nil
}

// Lineage returns one visible Job's relatives, one hop out, each authorized
// independently: a link grants no right over the Job it reaches.
//
// The reads are deliberately not a walk. A chain is followed by reading each
// Job's own lineage, which is what makes every step a fresh authorization
// decision and keeps a hidden Job in the middle of a chain invisible rather
// than merely unnamed.
func (s *Service) Lineage(deps Deps, access Access, jobID string) (Lineage, error) {
	snap, err := s.Get(deps, access, jobID)
	if err != nil {
		return Lineage{}, err
	}
	lineage := Lineage{Job: snap}

	outgoing, err := visibleLinks(deps.DB, access, "from_job_id = ?", jobID)
	if err != nil {
		return Lineage{}, err
	}
	incoming, err := visibleLinks(deps.DB, access, "to_job_id = ?", jobID)
	if err != nil {
		return Lineage{}, err
	}

	for _, link := range outgoing {
		switch link.row.Type {
		case string(LinkRetryOf), string(LinkRepeatOf):
			// FromJobID is the successor, so an outgoing retry/repeat link names
			// an ancestor of this Job.
			lineage.Ancestors = append(lineage.Ancestors, link.other)
		case string(LinkParentChild):
			lineage.Children = append(lineage.Children, link.other)
		}
	}
	for _, link := range incoming {
		switch link.row.Type {
		case string(LinkRetryOf), string(LinkRepeatOf):
			lineage.Successors = append(lineage.Successors, link.other)
		case string(LinkParentChild):
			lineage.Parents = append(lineage.Parents, link.other)
		}
	}
	return lineage, nil
}

// relativeLink is one link together with the far endpoint, if the asker may see
// it. A link whose far endpoint is hidden is dropped rather than reported with a
// placeholder, because "there is something here you may not see" is itself a
// leak.
type relativeLink struct {
	row   models.JobLink
	other Snapshot
}

// visibleLinks reads the links touching one Job in one direction and resolves
// the far endpoints through the shared predicate, in one query for the far Job
// rows: the hidden ones simply do not come back.
func visibleLinks(db *gorm.DB, access Access, condition string, jobID string) ([]relativeLink, error) {
	var links []models.JobLink
	if err := db.Where(condition, jobID).Find(&links).Error; err != nil {
		return nil, fmt.Errorf("jobs: read lineage links: %w", err)
	}
	if len(links) == 0 {
		return nil, nil
	}

	ids := make([]string, 0, len(links))
	seen := make(map[string]bool, len(links))
	for _, link := range links {
		other := link.ToJobID
		if other == jobID {
			other = link.FromJobID
		}
		if !seen[other] {
			seen[other] = true
			ids = append(ids, other)
		}
	}

	var rows []models.Job
	if err := jobQuery(db.Model(&models.Job{}), access).Where("jobs.id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("jobs: read lineage relatives: %w", err)
	}
	visible := make(map[string]Snapshot, len(rows))
	snapshots := make([]Snapshot, 0, len(rows))
	for _, row := range rows {
		snapshots = append(snapshots, viewerSnapshot(row, access))
	}
	if err := fillViewerPinState(db, access, snapshots); err != nil {
		return nil, err
	}
	for _, snapshot := range snapshots {
		visible[snapshot.ID] = snapshot
	}

	out := make([]relativeLink, 0, len(links))
	for _, link := range links {
		other := link.ToJobID
		if other == jobID {
			other = link.FromJobID
		}
		if snap, ok := visible[other]; ok {
			out = append(out, relativeLink{row: link, other: snap})
		}
	}
	// Newest relative first, the order every listing uses, so a lineage list and
	// a listing of the same Jobs read the same way. It is a presentation order
	// over one Job's relations rather than a filter, so it sorts here rather than
	// asking the database for a shape the link rows do not have.
	slices.SortStableFunc(out, func(a, b relativeLink) int {
		if cmp := b.other.AcceptedAt.Compare(a.other.AcceptedAt); cmp != 0 {
			return cmp
		}
		return strings.Compare(b.other.ID, a.other.ID)
	})
	return out, nil
}

// eventsOf projects stored event rows onto the bounded public view.
func eventsOf(rows []models.JobEvent) []Event {
	events := make([]Event, 0, len(rows))
	for _, row := range rows {
		events = append(events, Event{
			ID:               row.ID,
			JobID:            row.JobID,
			Sequence:         row.Sequence,
			JobVersion:       row.JobVersion,
			Type:             row.Type,
			Detail:           json.RawMessage(row.Detail),
			ReservedHost:     row.ReservedHost,
			DeliverySequence: row.DeliverySequence,
			CreatedAt:        row.CreatedAt,
		})
	}
	return events
}

// eventPageSize resolves a requested event page size, refusing one beyond the
// ceiling rather than serving less than was asked for.
func eventPageSize(limit int) (int, error) {
	switch {
	case limit == 0:
		return DefaultEventPageSize, nil
	case limit < 0:
		return 0, fmt.Errorf("%w: %d", ErrInvalidPage, limit)
	case limit > MaxEventPageSize:
		return 0, fmt.Errorf("%w: %d is over the %d-event ceiling", ErrInvalidPage, limit, MaxEventPageSize)
	}
	return limit, nil
}

// Summary reports the bounded aggregate over one filter and one window, under
// the same visibility predicate and the same filter the listing uses.
//
// Sharing them is the whole design: an aggregate computed from a different set
// than the listing shows is how a page announces "3 failed" beside two rows, and
// post-filtering in Go cannot fix it because the counts are the thing that was
// computed. Every number here therefore comes from a query built by
// applyFilter — never from a page of rows — and none of them touch a replay
// envelope, a verbose log or an event payload.
func (s *Service) Summary(deps Deps, access Access, filter Filter, window time.Duration) (Summary, error) {
	if err := validateFilter(filter); err != nil {
		return Summary{}, err
	}
	window, err := summaryWindow(window)
	if err != nil {
		return Summary{}, err
	}
	now := deps.now()

	to := now
	if filter.AcceptedBefore != nil && filter.AcceptedBefore.Before(to) {
		to = filter.AcceptedBefore.UTC()
	}
	from := now.Add(-window)
	if filter.AcceptedAfter != nil && filter.AcceptedAfter.After(from) {
		from = filter.AcceptedAfter.UTC()
	}
	return s.summaryRange(deps, access, filter, window, from, to)
}

// SummaryRange reports the same filtered, visible aggregate as Summary over an
// explicit historical interval. Summary is intentionally capped at
// MaxSummaryWindow for interactive requests; this method is for a durable export
// whose accepted input fixes the dates it will analyze.
func (s *Service) SummaryRange(deps Deps, access Access, filter Filter, from, to time.Time) (Summary, error) {
	if err := validateFilter(filter); err != nil {
		return Summary{}, err
	}
	if from.IsZero() || to.IsZero() || !from.Before(to) {
		return Summary{}, fmt.Errorf("%w: an explicit range needs a start before its end", ErrInvalidWindow)
	}
	from, to = from.UTC(), to.UTC()
	window := to.Sub(from)
	if filter.AcceptedAfter != nil && filter.AcceptedAfter.After(from) {
		from = filter.AcceptedAfter.UTC()
	}
	if filter.AcceptedBefore != nil && filter.AcceptedBefore.Before(to) {
		to = filter.AcceptedBefore.UTC()
	}

	return s.summaryRange(deps, access, filter, window, from, to)
}

// summaryRange is the shared query path for interactive and exported summaries.
// It writes the accepted interval into the ordinary filter so every aggregate
// dimension uses exactly the same visibility and filter predicates as listing.
func (s *Service) summaryRange(deps Deps, access Access, filter Filter, window time.Duration, from, to time.Time) (Summary, error) {
	scoped := filter
	scoped.AcceptedAfter = &from
	scoped.AcceptedBefore = &to
	if from.After(to) {
		return Summary{Window: window, From: from, To: to, ByState: map[string]int64{}, ByKind: map[string]int64{}}, nil
	}
	if filter.Command != "" {
		var summary Summary
		err := deps.DB.Transaction(func(tx *gorm.DB) error {
			commandDeps := deps
			commandDeps.DB = tx
			base, err := applyFilter(jobQuery(tx.Model(&models.Job{}), access), access, scoped)
			if err != nil {
				return err
			}
			base, err = s.applyCommandFilter(base, commandDeps, access, filter.Command)
			if err != nil {
				return err
			}
			summary, err = summarizeQuery(base, window, from, to)
			return err
		})
		if err != nil {
			return Summary{}, err
		}
		return summary, nil
	}

	base, err := applyFilter(jobQuery(deps.DB.Model(&models.Job{}), access), access, scoped)
	if err != nil {
		return Summary{}, err
	}
	return summarizeQuery(base, window, from, to)
}

func summarizeQuery(base *gorm.DB, window time.Duration, from, to time.Time) (Summary, error) {
	summary := Summary{
		Window: window, From: from, To: to,
		ByState: map[string]int64{}, ByKind: map[string]int64{},
	}

	total, err := countByColumn(base, "state")
	if err != nil {
		return Summary{}, err
	}
	for state, count := range total {
		summary.ByState[state] = count
		summary.Total += count
	}
	byKind, err := countByColumn(base, "kind")
	if err != nil {
		return Summary{}, err
	}
	for kind, count := range byKind {
		summary.ByKind[kind] = count
	}
	summary.Succeeded = summary.ByState[string(StateSucceeded)]
	summary.Failed = summary.ByState[string(StateFailed)]
	for _, state := range AllStates {
		if state.Terminal() {
			summary.Terminal += summary.ByState[string(state)]
		}
	}
	if summary.Terminal > 0 {
		summary.SuccessRate = float64(summary.Succeeded) / float64(summary.Terminal)
	}

	summary.Queue, err = durationStats(base, "queue_duration", "")
	if err != nil {
		return Summary{}, err
	}
	summary.Run, err = durationStats(base, "running_duration", "started_at IS NOT NULL")
	if err != nil {
		return Summary{}, err
	}
	summary.Failures, err = failureClassCounts(base)
	if err != nil {
		return Summary{}, err
	}
	return summary, nil
}

// summaryWindow resolves the analysis window, defaulting an unset one and
// refusing a longer one than an interactive question may scan.
func summaryWindow(window time.Duration) (time.Duration, error) {
	if window <= 0 {
		return DefaultSummaryWindow, nil
	}
	if window > MaxSummaryWindow {
		return 0, fmt.Errorf("%w: %v is over the %v ceiling", ErrInvalidWindow, window, MaxSummaryWindow)
	}
	return window, nil
}

// countByColumn groups a filtered Job query by one normalized column.
func countByColumn(base *gorm.DB, column string) (map[string]int64, error) {
	var rows []struct {
		Value string
		Count int64
	}
	if err := base.Session(&gorm.Session{}).
		Select(column + " AS value, COUNT(*) AS count").
		Group(column).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("jobs: aggregate by %s: %w", column, err)
	}
	counts := make(map[string]int64, len(rows))
	for _, row := range rows {
		counts[row.Value] = row.Count
	}
	return counts, nil
}

// durationStats reads the median and the 95th percentile of one duration column
// as two bounded seeks.
//
// It deliberately does not aggregate in Go: the percentiles are answered by the
// engine ordering the filtered set and taking one row at an offset, so no page
// of durations is materialized in the process and the window is the only thing
// bounding the work.
func durationStats(base *gorm.DB, column, extra string) (DurationStats, error) {
	var stats DurationStats
	query := base.Session(&gorm.Session{})
	if extra != "" {
		query = query.Where(extra)
	}

	var count int64
	if err := query.Count(&count).Error; err != nil {
		return DurationStats{}, fmt.Errorf("jobs: count %s: %w", column, err)
	}
	if count == 0 {
		return DurationStats{}, nil
	}

	for _, seek := range []struct {
		into  *time.Duration
		index int64
	}{
		{&stats.Median, medianIndex(count)},
		{&stats.P95, p95Index(count)},
	} {
		var duration int64
		err := query.Session(&gorm.Session{}).
			Select(column).
			Order(column + " ASC").
			Limit(1).Offset(int(seek.index)).
			Scan(&duration).Error
		if err != nil {
			return DurationStats{}, fmt.Errorf("jobs: read %s percentile: %w", column, err)
		}
		*seek.into = time.Duration(duration)
	}
	return stats, nil
}

// medianIndex is the position of the median in an ascending set of n values.
func medianIndex(n int64) int64 { return (n - 1) / 2 }

// p95Index is the position of the 95th percentile in an ascending set of n
// values, clamped into the set.
func p95Index(n int64) int64 {
	index := int64(math.Ceil(0.95*float64(n))) - 1
	if index < 0 {
		return 0
	}
	if index > n-1 {
		return n - 1
	}
	return index
}

// failureClassCounts groups settled failures by their bounded classification,
// commonest first. The class is a taxonomy rather than an error message, which
// is what makes it safe to aggregate and useful to look at.
func failureClassCounts(base *gorm.DB) ([]FailureClassCount, error) {
	var rows []struct {
		Class string
		Count int64
	}
	err := base.Session(&gorm.Session{}).
		Where("jobs.failure_class <> ''").
		Select("jobs.failure_class AS class, COUNT(*) AS count").
		Group("jobs.failure_class").
		Order("count DESC, class ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("jobs: aggregate failure classes: %w", err)
	}
	failures := make([]FailureClassCount, 0, len(rows))
	for _, row := range rows {
		failures = append(failures, FailureClassCount{Class: row.Class, Count: row.Count})
	}
	return failures, nil
}
