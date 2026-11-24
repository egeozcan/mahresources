package application_context

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// reductionStore is the one door to a Reduction's plan tables (design D2).
//
// Its methods take a *models.ResourceReduction the caller has already
// authorized, never a bare id from a request, and a generation: every read is of
// one generation of Cluster rows.
//
// It runs on unscopedDB() of the caller's handle, which keeps the actor, the
// deadline and the connection or transaction while dropping the scope
// callback's flattened allow-list, and applies scope itself: a confined
// principal's visibility is one recursive subtree CTE bound once (mrql.
// SubtreeCTEDefinition), never an IN list that grows with the subtree.
//
// It is built per call from the context (extracted services take the handle per
// call, never at construction).
type reductionStore struct {
	db    *gorm.DB
	scope reductionScope
	// settingsProbe caches, per Reduction, what the D12 migration read of a legacy
	// plan's settings, so they are parsed once and not once per Cluster.
	settingsProbe map[uint]legacySettingsProbe
	// outcomeBatch, when set, is stamped on a Cluster a reconciliation makes stale:
	// it is that apply batch's outcome (design D10).
	outcomeBatch string
}

// reductionMemberCap is the most members one Cluster loads or shows (design D6).
const reductionMemberCap = models.MaxClusterMembers

// reductionRowChunk is the most rows one store statement inserts.
const reductionRowChunk = 500

// storedCluster is a Cluster row with its members.
type storedCluster struct {
	Row     models.ReductionClusterRow
	Members []models.ReductionMemberRow
}

// deriveClusterColumns computes the derived columns from the Cluster's own
// members (design D3). It is the one writer of them, apart from the SQL paths
// that maintain gone_members.
//
//   - member_count and loser_count: a Loser is a non-ejected member that is not the Winner
//   - needs_attention: the page's curation markers, lossy, oversized or keeps-smaller
//   - total_size: the members' file sizes at compute, ejected ones included
//   - gone_members: non-ejected members whose Resource no longer exists
//   - fingerprint: sha256 of key | winner@hash | sorted loser@hash | fixed_seed, the
//     per-Cluster component of the apply token
func deriveClusterColumns(c *storedCluster) {
	row := &c.Row
	row.MemberCount = len(c.Members)
	row.LoserCount = 0
	row.TotalSize = 0
	row.GoneMembers = 0
	var winnerHash string
	var losers []string
	for i := range c.Members {
		m := &c.Members[i]
		row.TotalSize += m.FileSize
		if !m.Ejected && m.Gone {
			row.GoneMembers++
		}
		if m.ResourceID == row.WinnerID {
			winnerHash = m.Hash
		} else if !m.Ejected {
			row.LoserCount++
		}
		if !m.Ejected && m.ResourceID != row.WinnerID {
			losers = append(losers, strconv.FormatUint(uint64(m.ResourceID), 10)+"@"+m.Hash)
		}
	}
	row.NeedsAttention = lossyFieldCount(row.Lossy) > 0 || row.Oversized || row.KeepsSmaller
	sort.Strings(losers)
	fixed := "0"
	if row.FixedSeed {
		fixed = "1"
	}
	sum := sha256.Sum256([]byte(row.Key + "|" + strconv.FormatUint(uint64(row.WinnerID), 10) + "@" + winnerHash + "|" + strings.Join(losers, ",") + "|" + fixed))
	row.Fingerprint = hex.EncodeToString(sum[:])
}

// lossyFieldCount is how many field names a lossy column names.
func lossyFieldCount(lossy []byte) int {
	if len(lossy) == 0 {
		return 0
	}
	var names []string
	if err := json.Unmarshal(lossy, &names); err != nil {
		return 0
	}
	return len(names)
}

// tokenDigest streams the apply token: a fingerprint at a time, in key order, so
// nothing is held but the hash state (300,000 eligible Clusters would otherwise
// keep about 19 MB of fingerprints).
type tokenDigest struct{ h hash.Hash }

func newTokenDigest() *tokenDigest { return &tokenDigest{h: sha256.New()} }

func (d *tokenDigest) add(fingerprint string) {
	d.h.Write([]byte(fingerprint))
	d.h.Write([]byte{'\n'})
}

// sum is the opaque token clients carry back: the first 32 hex digits.
func (d *tokenDigest) sum() string { return hex.EncodeToString(d.h.Sum(nil))[:32] }

// applyTokenOf is the digest an apply is bound to: the stored fingerprints of the
// eligible Clusters, in key order (design D10). Opaque to clients.
func applyTokenOf(fingerprints []string) string {
	d := newTokenDigest()
	for _, fp := range fingerprints {
		d.add(fp)
	}
	return d.sum()
}

// insertCluster writes a Cluster and its members under the Reduction and
// generation, deriving the derived columns first. Members go in chunks of at
// most reductionRowChunk rows per statement, in one transaction with the
// Cluster row.
func (s *reductionStore) insertCluster(r *models.ResourceReduction, generation string, c *storedCluster) error {
	if len(c.Members) == 0 {
		return errors.New("a Cluster is written with its members, and none was given")
	}
	c.Row.ID = 0
	c.Row.ReductionID = r.ID
	c.Row.Generation = generation
	for i := range c.Members {
		c.Members[i].ID = 0
		c.Members[i].ReductionID = r.ID
		c.Members[i].Generation = generation
		c.Members[i].Position = i
	}
	deriveClusterColumns(c)
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&c.Row).Error; err != nil {
			return err
		}
		for i := range c.Members {
			c.Members[i].ClusterID = c.Row.ID
		}
		if len(c.Members) == 0 {
			return nil
		}
		return tx.CreateInBatches(c.Members, reductionRowChunk).Error
	})
}

// reviewFilter is today's filterReviewClusters as a WHERE fragment over the
// effective state.
func reviewFilter(query *query_models.ResourceReductionReviewQuery) (string, []any) {
	if query == nil {
		return "", nil
	}
	var parts []string
	var args []any
	status := map[string]bool{}
	for _, v := range query.Status {
		status[v] = true
	}
	if len(status) > 0 && !status["all"] {
		eff := effectiveState("c")
		var matches []string
		switch {
		case status["open"]:
			matches = append(matches, eff+" = 'open'")
		case status["reviewed"]:
			matches = append(matches, eff+" = 'open' AND c.reviewed")
		}
		if status[models.ReductionClusterSkipped] {
			matches = append(matches, eff+" = 'skipped'")
		}
		if status[models.ReductionClusterApplied] {
			matches = append(matches, eff+" = 'applied' AND c.merged")
		}
		if status[models.ReductionClusterStale] {
			matches = append(matches, eff+" = 'stale'")
		}
		if len(matches) == 0 {
			parts = append(parts, "(1 = 0)")
		} else {
			parts = append(parts, "(("+strings.Join(matches, ") OR (")+"))")
		}
	}
	if len(query.Tier) > 0 {
		parts = append(parts, "c.tier IN ?")
		args = append(args, query.Tier)
	}
	if query.NeedsAttention {
		parts = append(parts, "c.needs_attention")
	}
	switch query.Approval {
	case "approved":
		parts = append(parts, approved("c"))
	case "unapproved":
		parts = append(parts, "NOT "+approved("c"))
	}
	if len(parts) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(parts, " AND "), args
}

// ---- reads ------------------------------------------------------------------------

// reviewClusterColumns is every column of a Cluster row, with the stale reason read
// as at most 1,024 characters: a row written before the write-side bound, or a
// plugin veto reason, must not put a megabyte on each Cluster of a review page.
// TestReviewClusterColumnsCoverTheModel keeps it equal to the model's columns.
const reviewClusterColumns = `c.id, c.reduction_id, c.generation, c."key", c.position, c.tier, c.state, c.winner_id, c.checked,
 c.reviewed, c.oversized, c.decided_by, c.margin, c.undecided, c.keeps_smaller, c.read_only, c.lossy,
 SUBSTR(c.stale_reason, 1, 1024) AS stale_reason, c.merged, c.claim_id, c.applied_at, c.member_count, c.loser_count,
 c.needs_attention, c.fingerprint, c.total_size, c.gone_members, c.truncated, c.restore_seq, c.apply_batch,
 c.approved_winner, c.approved_seq, c.outcome_batch, c.bulk_op, c.seed_resource_id, c.fixed_seed`

// pageClusters is one page of the review's Clusters: filtered, sorted and
// paginated in SQL, with the total the filter matches. Sort is "copies" (the
// default), "size" or "computed", ties by key, as sortReviewClusters orders them.
func (s *reductionStore) pageClusters(r *models.ResourceReduction, generation string, query *query_models.ResourceReductionReviewQuery, offset, limit int) ([]models.ReductionClusterRow, int, error) {
	prefix, where, args := s.base(r, generation)
	filter, filterArgs := reviewFilter(query)
	args = append(args, filterArgs...)

	var total int64
	if err := s.db.Raw(prefix+"SELECT COUNT(*) FROM reduction_clusters c WHERE "+where+filter, args...).Scan(&total).Error; err != nil {
		return nil, 0, err
	}
	order := `c.member_count DESC, c."key"`
	if query != nil {
		switch query.Sort {
		case "size":
			order = `c.total_size DESC, c."key"`
		case "computed":
			order = `c.position, c."key"`
		}
	}
	var rows []models.ReductionClusterRow
	pageArgs := append(append([]any(nil), args...), limit, offset)
	if err := s.db.Raw(prefix+"SELECT "+reviewClusterColumns+" FROM reduction_clusters c WHERE "+where+filter+" ORDER BY "+order+" LIMIT ? OFFSET ?", pageArgs...).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, int(total), nil
}

// loadCluster reads one Cluster by key with its members (at most
// reductionMemberCap, in position order). A Cluster the principal may not fully
// see is not found, the same answer an unknown key gets.
func (s *reductionStore) loadCluster(r *models.ResourceReduction, generation, key string) (*storedCluster, error) {
	prefix, where, args := s.base(r, generation)
	var rows []models.ReductionClusterRow
	if err := s.db.Raw(prefix+`SELECT `+reviewClusterColumns+` FROM reduction_clusters c WHERE `+where+` AND c."key" = ?`, append(args, key)...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	members, err := s.loadMembers(rows[0].ID, reductionMemberCap)
	if err != nil {
		return nil, err
	}
	return &storedCluster{Row: rows[0], Members: members}, nil
}

// loadMembers reads a Cluster's first members in position order.
func (s *reductionStore) loadMembers(clusterID uint, limit int) ([]models.ReductionMemberRow, error) {
	var members []models.ReductionMemberRow
	err := s.db.Where("cluster_id = ?", clusterID).Order("position, id").Limit(limit).Find(&members).Error
	return members, err
}

// tabCounts counts the Clusters of each status tab over the ones the principal
// may fully see, reading a Cluster whose member was deleted as stale.
func (s *reductionStore) tabCounts(r *models.ResourceReduction, generation string) (contracts.ReductionTabCounts, error) {
	prefix, where, args := s.base(r, generation)
	var rows []struct {
		State  string
		Merged bool
		N      int64
	}
	if err := s.db.Raw(prefix+"SELECT "+effectiveState("c")+" AS state, c.merged AS merged, COUNT(*) AS n FROM reduction_clusters c WHERE "+where+
		" GROUP BY "+effectiveState("c")+", c.merged", args...).Scan(&rows).Error; err != nil {
		return contracts.ReductionTabCounts{}, err
	}
	var counts contracts.ReductionTabCounts
	for _, row := range rows {
		n := int(row.N)
		switch row.State {
		case models.ReductionClusterOpen:
			counts.Open += n
		case models.ReductionClusterSkipped:
			counts.Skipped += n
		case models.ReductionClusterStale:
			counts.Stale += n
		case models.ReductionClusterApplied:
			if row.Merged {
				counts.Applied += n
			} else {
				counts.Merging += n
			}
		}
	}
	return counts, nil
}

// reductionStoreFigures are the counts the review states about the whole plan:
// what is checked, what an apply would do, what the bulk buttons target.
type reductionStoreFigures struct {
	Checked       int
	CheckedLosers int
	Apply         contracts.ReductionApplyFigures
	Bulk          contracts.ReductionBulkFigures
}

// figures counts over the actionable Clusters the principal may fully see, across
// the whole generation (apply acts on all of it, not on the page).
func (s *reductionStore) figures(r *models.ResourceReduction, generation string) (reductionStoreFigures, error) {
	prefix, where, args := s.base(r, generation)
	count := func(cond string) string {
		return "COALESCE(SUM(CASE WHEN " + cond + " THEN 1 ELSE 0 END), 0)"
	}
	sum := func(cond string) string {
		return "COALESCE(SUM(CASE WHEN " + cond + " THEN c.loser_count ELSE 0 END), 0)"
	}
	app := approved("c")
	var row struct {
		IdenticalOpen, IdenticalApprovable   int64
		Checked, CheckedLosers               int64
		IdenticalClusters, IdenticalLosers   int64
		NearClusters, NearLosers, KeepsSmall int64
	}
	sql := prefix + "SELECT " +
		count("c.tier = 'identical'") + " AS identical_open, " +
		count("c.tier = 'identical' AND NOT "+app+" AND NOT c.needs_attention AND c.loser_count > 0") + " AS identical_approvable, " +
		count(app) + " AS checked, " +
		sum(app) + " AS checked_losers, " +
		count(app+" AND c.tier = 'identical'") + " AS identical_clusters, " +
		sum(app+" AND c.tier = 'identical'") + " AS identical_losers, " +
		count(app+" AND c.tier = 'near'") + " AS near_clusters, " +
		sum(app+" AND c.tier = 'near'") + " AS near_losers, " +
		count(app+" AND c.keeps_smaller") + " AS keeps_small " +
		"FROM reduction_clusters c WHERE " + where + " AND " + actionable("c")
	if err := s.db.Raw(sql, args...).Scan(&row).Error; err != nil {
		return reductionStoreFigures{}, err
	}
	var out reductionStoreFigures
	out.Bulk.IdenticalOpen = int(row.IdenticalOpen)
	out.Bulk.IdenticalApprovable = int(row.IdenticalApprovable)
	out.Checked = int(row.Checked)
	out.CheckedLosers = int(row.CheckedLosers)
	out.Apply.Identical.Clusters = int(row.IdenticalClusters)
	out.Apply.Identical.Losers = int(row.IdenticalLosers)
	out.Apply.Near.Clusters = int(row.NearClusters)
	out.Apply.Near.Losers = int(row.NearLosers)
	out.Apply.KeepsSmaller = int(row.KeepsSmall)
	return out, nil
}

// eligibleChunk is how many eligible Clusters one keyset step reads.
const eligibleChunk = 500

// eligibleClusters streams the Clusters an apply may act on, in key order, a
// chunk of narrow rows at a time: actionable, checked with a Loser to remove,
// unclaimed, and fully visible. The fingerprints digest into the apply token.
func (s *reductionStore) eligibleClusters(r *models.ResourceReduction, generation string, fn func([]models.ReductionClusterRow) error) error {
	prefix, where, args := s.base(r, generation)
	last := ""
	for {
		var rows []models.ReductionClusterRow
		stepArgs := append(append([]any(nil), args...), last, eligibleChunk)
		if err := s.db.Raw(prefix+`SELECT c.id, c."key", c.winner_id, c.loser_count, c.fingerprint FROM reduction_clusters c WHERE `+where+
			" AND "+actionable("c")+" AND "+approved("c")+` AND c.claim_id = '' AND c."key" > ? ORDER BY c."key" LIMIT ?`, stepArgs...).Scan(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		if err := fn(rows); err != nil {
			return err
		}
		last = rows[len(rows)-1].Key
		if len(rows) < eligibleChunk {
			return nil
		}
	}
}

// applyEligibility is the token over the eligible Clusters with their counts.
func (s *reductionStore) applyEligibility(r *models.ResourceReduction, generation string) (token string, clusters, losers int, err error) {
	digest := newTokenDigest()
	err = s.eligibleClusters(r, generation, func(rows []models.ReductionClusterRow) error {
		for _, row := range rows {
			digest.add(row.Fingerprint)
			clusters++
			losers += row.LoserCount
		}
		return nil
	})
	return digest.sum(), clusters, losers, err
}

// settleableClaims counts the Clusters settle-only could release for this
// principal: claimed and not merged, fully visible, with no live apply or settle
// owning them. Filters do not enter into it.
func (s *reductionStore) settleableClaims(r *models.ResourceReduction, generation string, now time.Time) (int, error) {
	// A live apply or a live settle owns the claims; the write path refuses both.
	if r.ApplyLeaseUntil > now.UnixNano() || (r.SettleOp != nil && r.SettleLeaseUntil > now.UnixNano()) {
		return 0, nil
	}
	prefix, where, args := s.base(r, generation)
	var n int64
	if err := s.db.Raw(prefix+"SELECT COUNT(*) FROM reduction_clusters c WHERE "+where+" AND c.state = 'applied' AND NOT c.merged", args...).Scan(&n).Error; err != nil {
		return 0, err
	}
	return int(n), nil
}

// lastApplyCountsVisible reports whether the last apply's counts may be shown:
// the record names a batch, exactly the counted number of Clusters carry it, and
// none of them is withheld. A record that counted Clusters but carries no batch
// (it predates the ids) cannot be checked, so it is not. A record that counted
// none and names no batch has nothing to show or hide; one that names a batch is
// still checked, since Clusters stamped with it that it did not count are outcomes
// the numbers leave out.
func (s *reductionStore) lastApplyCountsVisible(r *models.ResourceReduction, generation, batch string, counted int) (bool, error) {
	if batch == "" {
		return counted == 0, nil
	}
	prefix, args := s.with()
	var row struct {
		N        int64
		Withheld int64
	}
	args = append(args, r.ID, generation, batch)
	if err := s.db.Raw(prefix+"SELECT COUNT(*) AS n, COALESCE(SUM(CASE WHEN "+s.withheld("c")+" THEN 1 ELSE 0 END), 0) AS withheld "+
		"FROM reduction_clusters c WHERE c.reduction_id = ? AND c.generation = ? AND c.outcome_batch = ?", args...).Scan(&row).Error; err != nil {
		return false, err
	}
	return int(row.N) == counted && row.Withheld == 0, nil
}
