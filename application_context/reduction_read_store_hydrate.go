package application_context

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"mahresources/contracts"
	"mahresources/models"
)

// Hydration of one review page from the plan tables (design D8, D13): the page's
// Clusters, their Winners and a bounded share of their members, with the narrow
// Resource projection. Nothing here reads more than the page names.

// reductionMemberCursor is a keyset position in a Cluster's members, (position, id).
// The zero cursor reads from the start.
type reductionMemberCursor struct {
	Position int
	ID       uint
}

func startOfMembers() reductionMemberCursor { return reductionMemberCursor{Position: -1} }

func (c reductionMemberCursor) String() string {
	if c.Position < 0 {
		return ""
	}
	return strconv.Itoa(c.Position) + "." + strconv.FormatUint(uint64(c.ID), 10)
}

// parseReductionMemberCursor reads a cursor String produced; "" is the start.
func parseReductionMemberCursor(s string) (reductionMemberCursor, error) {
	if s == "" {
		return startOfMembers(), nil
	}
	pos, id, ok := strings.Cut(s, ".")
	p, err1 := strconv.Atoi(pos)
	i, err2 := strconv.ParseUint(id, 10, 32)
	if !ok || err1 != nil || err2 != nil || p < 0 {
		return startOfMembers(), fmt.Errorf("not a member cursor: %q", s)
	}
	return reductionMemberCursor{Position: p, ID: uint(i)}, nil
}

// memberPage reads up to limit members of a Cluster after the cursor, in
// (position, id) order.
func (s *reductionStore) memberPage(clusterID uint, after reductionMemberCursor, limit int) ([]models.ReductionMemberRow, error) {
	var rows []models.ReductionMemberRow
	err := s.db.Where(`cluster_id = ? AND ("position" > ? OR ("position" = ? AND id > ?))`, clusterID, after.Position, after.Position, after.ID).
		Order(`"position", id`).Limit(limit).Find(&rows).Error
	return rows, err
}

// smallClusterMembers reads every member of several small Clusters at once.
func (s *reductionStore) smallClusterMembers(clusterIDs []uint) (map[uint][]models.ReductionMemberRow, error) {
	out := make(map[uint][]models.ReductionMemberRow, len(clusterIDs))
	for _, chunk := range chunkUints(clusterIDs, 100) {
		var rows []models.ReductionMemberRow
		if err := s.db.Where("cluster_id IN ?", chunk).Order(`cluster_id, "position", id`).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			out[row.ClusterID] = append(out[row.ClusterID], row)
		}
	}
	return out, nil
}

// winnerMember reads the Winner's own member row, which a promotion can leave
// beyond the first members.
func (s *reductionStore) winnerMember(clusterID, winnerID uint) (*models.ReductionMemberRow, error) {
	var rows []models.ReductionMemberRow
	if err := s.db.Where("cluster_id = ? AND resource_id = ?", clusterID, winnerID).Order(`"position", id`).Limit(1).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// ejectedCounts counts the ejected members of the named Clusters.
func (s *reductionStore) ejectedCounts(clusterIDs []uint) (map[uint]int, error) {
	out := map[uint]int{}
	for _, chunk := range chunkUints(clusterIDs, idChunk) {
		var rows []struct {
			ClusterID uint
			N         int64
		}
		if err := s.db.Raw(`SELECT cluster_id, COALESCE(SUM(CASE WHEN ejected THEN 1 ELSE 0 END), 0) AS n FROM reduction_members WHERE cluster_id IN ? GROUP BY cluster_id`, chunk).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			out[row.ClusterID] = int(row.N)
		}
	}
	return out, nil
}

// narrowResourceRow is the review's Resource projection: no meta, own_meta or
// description, and the display text bounded.
type narrowResourceRow struct {
	ID                 uint
	CreatedAt          time.Time
	Name               []byte
	OriginalName       []byte
	ContentType        []byte
	Hash               string
	Width              uint
	Height             uint
	FileSize           int64
	OwnerId            *uint
	ResourceCategoryId uint
	SeriesID           *uint
}

// loadNarrowResources reads the named Resources with the narrow projection,
// filtered to the principal's subtree. A missing id is a Resource that was
// deleted or that the principal may not see.
func (s *reductionStore) loadNarrowResources(ids []uint) (map[uint]*models.Resource, error) {
	out := make(map[uint]*models.Resource, len(ids))
	if s.scope.mode == scopeDeny {
		return out, nil
	}
	prefix, args := s.with()
	visible := ""
	if s.scope.mode == scopeSubtree {
		visible = " AND r.owner_id IN " + subtreeIDs
	}
	cols := "r.id, r.created_at, r.hash, r.width, r.height, r.file_size, r.owner_id, r.resource_category_id, r.series_id, " +
		s.boundedText("r.name") + " AS name, " + s.boundedText("r.original_name") + " AS original_name, " +
		s.boundedText("r.content_type") + " AS content_type"
	for _, chunk := range chunkUints(dedupeUints(ids), idChunk) {
		var rows []narrowResourceRow
		if err := s.db.Raw(prefix+"SELECT "+cols+" FROM resources r WHERE r.id IN ?"+visible, append(append([]any{}, args...), chunk)...).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			out[row.ID] = &models.Resource{
				ID: row.ID, CreatedAt: row.CreatedAt, Name: trimDisplayText(row.Name), OriginalName: trimDisplayText(row.OriginalName),
				ContentType: trimDisplayText(row.ContentType), Hash: row.Hash, Width: row.Width, Height: row.Height, FileSize: row.FileSize,
				OwnerId: row.OwnerId, ResourceCategoryId: row.ResourceCategoryId, SeriesID: row.SeriesID,
			}
		}
	}
	return out, nil
}

// largestLoser is the Loser with the most pixels, the first in member order among
// equals: the copy the keeps-smaller warning compares against and its fix button
// promotes. Found in SQL so a Cluster whose members are not all on the page still
// names it.
func (s *reductionStore) largestLoser(clusterID, winnerID uint) (*models.Resource, error) {
	prefix, args := s.with()
	visible := ""
	if s.scope.mode == scopeSubtree {
		visible = " AND r.owner_id IN " + subtreeIDs
	} else if s.scope.mode == scopeDeny {
		return nil, nil
	}
	var rows []struct {
		ID            uint
		Width, Height uint
	}
	if err := s.db.Raw(prefix+`SELECT r.id AS id, r.width AS width, r.height AS height FROM reduction_members m JOIN resources r ON r.id = m.resource_id
		WHERE m.cluster_id = ? AND NOT m.ejected AND m.resource_id <> ?`+visible+` ORDER BY r.width * r.height DESC, m."position", m.id LIMIT 1`,
		append(append([]any{}, args...), clusterID, winnerID)...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &models.Resource{ID: rows[0].ID, Width: rows[0].Width, Height: rows[0].Height}, nil
}

// clusterOfRow is the JSON plan's view of a Cluster row, with the unscoped reading
// of a pending Cluster whose member was deleted: stale, with the reason, and
// unchecked (the effective state every count and filter uses).
func clusterOfRow(row *models.ReductionClusterRow) (*models.ReductionCluster, error) {
	c := &models.ReductionCluster{
		ID: row.Key, Tier: row.Tier, WinnerID: row.WinnerID, DecidedBy: row.DecidedBy, Margin: row.Margin,
		Undecided: row.Undecided, KeepsSmaller: row.KeepsSmaller, State: row.State, Checked: row.Checked,
		Reviewed: row.Reviewed, Oversized: row.Oversized, StaleReason: row.StaleReason, AppliedAt: row.AppliedAt,
		Merged: row.Merged, ClaimID: row.ClaimID,
	}
	if ok, err := decodeStoredJSON(row.Lossy, &c.Lossy); err != nil {
		return nil, err
	} else if !ok {
		c.Lossy = nil
	}
	if (row.State == models.ReductionClusterOpen || row.State == models.ReductionClusterSkipped) && row.GoneMembers > 0 {
		c.State = models.ReductionClusterStale
		c.StaleReason = staleReasonDeleted
		c.Checked = false
	}
	return c, nil
}

// storeStateLabel is reductionStateLabel for a Cluster whose members are not all
// loaded: approval is judged on the stored Loser count, not on the members at hand.
func storeStateLabel(c *models.ReductionCluster, loserCount int) string {
	switch c.State {
	case models.ReductionClusterSkipped:
		return "Skipped"
	case models.ReductionClusterApplied:
		if !c.Merged {
			return "Merging"
		}
		return "Applied"
	case models.ReductionClusterStale:
		return "Stale"
	}
	if c.Checked && loserCount > 0 {
		return "Approved"
	}
	if c.Reviewed {
		return "Not approved"
	}
	return "Open"
}

// storeDecidedByLabel names the deciding criterion; a full Near star's seed is
// kept for being the copy the others were matched to, not for a rule criterion.
func storeDecidedByLabel(decidedBy string) string {
	if decidedBy == "seed" {
		return "kept: the copy the others were matched to"
	}
	return models.WinnerCriterionLabel(decidedBy)
}

func memberOfRow(m *models.ReductionMemberRow) *models.ReductionMember {
	out := &models.ReductionMember{ResourceID: m.ResourceID, Hash: m.Hash, InExtent: m.InExtent, Ejected: m.Ejected, EjectedReason: m.EjectedReason}
	if m.Distance != nil {
		d := uint8(max(0, min(*m.Distance, 255)))
		out.Distance = &d
	}
	return out
}

// hydrateReviewPage builds the page's views. Clusters of up to reductionSmallCluster
// members always render in full; larger ones share reductionPageMemberBudget
// members, and one past the budget shows its Winner and a members cursor.
func (ctx *MahresourcesContext) hydrateReviewPage(store *reductionStore, r *models.ResourceReduction, rows []models.ReductionClusterRow, scoped bool) ([]contracts.ReductionClusterView, *reductionReviewExtras, error) {
	extras := &reductionReviewExtras{Clusters: make(map[string]reductionClusterExtra, len(rows))}
	if len(rows) == 0 {
		return []contracts.ReductionClusterView{}, extras, nil
	}

	// Which Clusters get their members, and how.
	full := make([]bool, len(rows))
	var smallIDs []uint
	budget := reductionPageMemberBudget
	for i := range rows {
		switch {
		case rows[i].MemberCount <= reductionSmallCluster:
			full[i] = true
			smallIDs = append(smallIDs, rows[i].ID)
		case min(rows[i].MemberCount, reductionMemberCap) <= budget:
			full[i] = true
			budget -= min(rows[i].MemberCount, reductionMemberCap)
		}
	}
	small, err := store.smallClusterMembers(smallIDs)
	if err != nil {
		return nil, nil, err
	}

	members := make([][]models.ReductionMemberRow, len(rows))
	more := make([]bool, len(rows))
	after := make([]string, len(rows))
	for i := range rows {
		row := &rows[i]
		switch {
		case !full[i]:
			more[i] = true
		case row.MemberCount <= reductionSmallCluster:
			members[i] = small[row.ID]
		default:
			page, err := store.memberPage(row.ID, startOfMembers(), reductionMemberCap)
			if err != nil {
				return nil, nil, err
			}
			members[i] = page
			if row.MemberCount > len(page) && len(page) > 0 {
				more[i] = true
				last := page[len(page)-1]
				after[i] = reductionMemberCursor{Position: last.Position, ID: last.ID}.String()
			}
		}
		// The Winner is always on the card, whatever the page of members holds.
		found := false
		for _, m := range members[i] {
			if m.ResourceID == row.WinnerID {
				found = true
				break
			}
		}
		if !found {
			w, err := store.winnerMember(row.ID, row.WinnerID)
			if err != nil {
				return nil, nil, err
			}
			if w != nil {
				members[i] = append([]models.ReductionMemberRow{*w}, members[i]...)
			}
		}
	}

	var ids []uint
	for i := range rows {
		ids = append(ids, rows[i].WinnerID)
		for _, m := range members[i] {
			ids = append(ids, m.ResourceID)
		}
	}
	resources, err := store.loadNarrowResources(ids)
	if err != nil {
		return nil, nil, err
	}
	associations, err := ctx.countResourceAssociationKinds(resourceIDsOf(resources))
	if err != nil {
		return nil, nil, err
	}
	var partial []uint
	for i := range rows {
		if len(members[i]) < rows[i].MemberCount {
			partial = append(partial, rows[i].ID)
		}
	}
	ejected, err := store.ejectedCounts(partial)
	if err != nil {
		return nil, nil, err
	}

	views := make([]contracts.ReductionClusterView, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		cluster, err := clusterOfRow(row)
		if err != nil {
			return nil, nil, err
		}
		view := contracts.ReductionClusterView{
			ReductionCluster: cluster,
			Winner:           resources[row.WinnerID],
			DecidedByLabel:   storeDecidedByLabel(row.DecidedBy),
			StateLabel:       storeStateLabel(cluster, row.LoserCount),
			LoserCount:       row.LoserCount,
			Position:         len(views) + 1,
			NeedsAttention:   row.NeedsAttention,
		}
		leftOut := 0
		for k := range members[i] {
			m := &members[i][k]
			member := memberOfRow(m)
			cluster.Members = append(cluster.Members, member)
			if m.Changed {
				cluster.ChangedMembers = append(cluster.ChangedMembers, m.ResourceID)
			}
			if m.Ejected {
				leftOut++
			}
			if row.Merged && m.OwnerAtMerge != 0 && member.IsLoser(row.WinnerID) {
				if cluster.LoserOwners == nil {
					cluster.LoserOwners = map[uint]uint{}
				}
				cluster.LoserOwners[m.ResourceID] = m.OwnerAtMerge
			}
			view.Members = append(view.Members, contracts.ReductionMemberView{
				ReductionMember: member,
				Resource:        resources[m.ResourceID],
				IsWinner:        m.ResourceID == row.WinnerID,
				IsLoser:         member.IsLoser(row.WinnerID),
				Changed:         m.Changed,
				DistanceLabel:   reductionDistanceLabel(member.Distance),
				TagCount:        associations[m.ResourceID].Tags,
				NoteCount:       associations[m.ResourceID].Notes,
				GroupCount:      associations[m.ResourceID].Groups,
			})
		}
		if len(members[i]) < row.MemberCount {
			leftOut = ejected[row.ID]
		}
		view.LeftOutCount = leftOut
		if row.State == models.ReductionClusterApplied && row.Merged {
			view.MergedCount = row.LoserCount
		}
		view.WinnerChanged = containsUint(cluster.ChangedMembers, row.WinnerID) ||
			(!scoped && cluster.State == models.ReductionClusterStale && resources[row.WinnerID] == nil)
		if row.KeepsSmaller {
			kept := resources[row.WinnerID]
			if kept != nil {
				largest, err := store.largestLoser(row.ID, row.WinnerID)
				if err != nil {
					return nil, nil, err
				}
				if largest != nil && pixelsOf(largest) > pixelsOf(kept) {
					view.KeptDimensions = fmt.Sprintf("%d×%d", kept.Width, kept.Height)
					view.LargerDimensions = fmt.Sprintf("%d×%d", largest.Width, largest.Height)
					view.LargestMemberID = largest.ID
				}
			}
		}
		views = append(views, view)

		extra := reductionClusterExtra{MemberCount: row.MemberCount, MembersShown: len(members[i]), MoreMembers: more[i], MembersAfter: after[i]}
		if !scoped {
			extra.Truncated = row.Truncated
		}
		extras.Clusters[row.Key] = extra
	}
	return views, extras, nil
}
