package application_context

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"mahresources/hash_worker"
	"mahresources/models"
)

// oversizedNearClusterSize is where a Near-Identical Cluster stops arriving
// checkable and has to be expanded before it can be acted on.
//
// It bounds the damage a chained perceptual match can do behind one checkbox.
// Greedy star already refuses transitive inference — every proposed deletion has
// a stored pair to its own Winner — but a hub image genuinely within threshold of
// three hundred others is one click from destroying all of them, and the review
// is this feature's only safety mechanism.
//
// It deliberately does not apply to the Identical tier, where a large Cluster is
// merely numerous: fifty copies of one file are fifty copies of one file, and
// byte-identity is a fact rather than a guess.
const oversizedNearClusterSize = 12

// clusterCandidate is one Resource being considered for a Cluster, together with
// what the Winner Rule needs and the row does not carry.
type clusterCandidate struct {
	models.WinnerCandidate
	InExtent bool
	Distance *uint8
}

// buildCluster turns a set of candidates into a Cluster, or returns nil when
// there is nothing to propose.
//
// The out-of-Extent rule is applied here, and it is the one thing in this file
// that is a decision rather than a mechanism. D7 says a Resource outside the
// Extent may win and may never lose, which leaves two cases the design did not
// spell out and greedy star produces trivially:
//
//   - Two out-of-Extent members. Both cannot win and neither may lose, so the
//     Cluster as computed is unsatisfiable. The best of them by the Winner Rule
//     wins and the rest are not part of the Cluster at all.
//   - An out-of-Extent member that is *worse* than the best in-Extent one. It is
//     not the better copy sitting elsewhere that the design wanted to surface, it
//     cannot lose, and forcing it to win would merge the reviewer's own Resources
//     into something they did not select. It is not part of the Cluster either.
//
// So a Cluster holds at most one out-of-Extent member, always as its Winner, and
// "nothing outside the Extent is ever destroyed" is structural rather than a rule
// apply has to remember.
//
// fixedSeed, when nonzero, names the candidate a full Near star keeps whatever the
// Winner Rule says: the seed every other member was matched to (design D5). Its
// justification is then the seed itself, no margin, never undecided.
func buildCluster(tier string, candidates []clusterCandidate, rule []string, fixedSeed uint) *models.ReductionCluster {
	if len(candidates) < 2 {
		return nil
	}

	sorted := make([]clusterCandidate, len(candidates))
	copy(sorted, candidates)
	sort.SliceStable(sorted, func(i, j int) bool {
		return models.CompareByWinnerRule(rule, sorted[i].WinnerCandidate, sorted[j].WinnerCandidate) < 0
	})

	// The best member overall decides whether an out-of-Extent Resource is the
	// better copy the reviewer should see, because "better" here means exactly
	// "wins under the Winner Rule".
	kept := make([]clusterCandidate, 0, len(sorted))
	outsideTaken := false
	for _, candidate := range sorted {
		if candidate.InExtent {
			kept = append(kept, candidate)
			continue
		}
		// An out-of-Extent member is kept only if nothing has been kept yet,
		// which — the list being sorted best-first — is exactly "it beats every
		// in-Extent member".
		if len(kept) == 0 && !outsideTaken {
			outsideTaken = true
			kept = append(kept, candidate)
		}
	}

	// One member left in the Extent is nothing to collapse.
	inExtentCount := 0
	for _, candidate := range kept {
		if candidate.InExtent {
			inExtentCount++
		}
	}
	if len(kept) < 2 || inExtentCount == 0 {
		return nil
	}

	var decidedBy, margin string
	var undecided bool
	if fixedSeed != 0 {
		// The seed leads, the rest keep their Winner Rule order.
		for i := range kept {
			if kept[i].Resource.ID == fixedSeed {
				seed := kept[i]
				copy(kept[1:i+1], kept[:i])
				kept[0] = seed
				break
			}
		}
		if kept[0].Resource.ID != fixedSeed {
			return nil
		}
		decidedBy = models.SeedCriterion
	}
	winner := kept[0]
	runnerUp := kept[1]
	if fixedSeed == 0 {
		decidedBy, margin, undecided = models.DecidingCriterion(rule, winner.WinnerCandidate, runnerUp.WinnerCandidate)
	}

	members := make([]*models.ReductionMember, 0, len(kept))
	for _, candidate := range kept {
		member := &models.ReductionMember{
			ResourceID: candidate.Resource.ID,
			Hash:       candidate.Resource.Hash,
			InExtent:   candidate.InExtent,
		}
		if candidate.Resource.ID != winner.Resource.ID {
			member.Distance = candidate.Distance
		}
		members = append(members, member)
	}

	cluster := &models.ReductionCluster{
		ID:        clusterID(tier, members),
		Tier:      tier,
		WinnerID:  winner.Resource.ID,
		Members:   members,
		DecidedBy: decidedBy,
		Margin:    margin,
		Undecided: undecided,
		State:     models.ReductionClusterOpen,
	}
	cluster.Lossy = lossyFields(kept, winner.Resource.ID)
	cluster.KeepsSmaller = keepsSmaller(kept, winner.Resource.ID, rule, cluster.DecidedBy)
	applyTierDefaults(cluster)
	return cluster
}

// applyTierDefaults sets the starting checked state, which is where the friction
// sits: byte-identity is a fact and arrives checked, perceptual similarity is a
// guess and does not. An oversized Near-Identical Cluster arrives unchecked
// whatever the tier default says.
func applyTierDefaults(cluster *models.ReductionCluster) {
	if cluster.Tier == models.ReductionTierIdentical {
		cluster.Checked = true
		return
	}
	cluster.Oversized = len(cluster.Members) > oversizedNearClusterSize
	cluster.Checked = false
}

// lossyFields names what a merge would silently discard: MergeResources keeps the
// Winner's own Description, Series and ResourceCategory and drops every Loser's,
// so a Loser holding one the Winner lacks is the curated copy about to be thrown
// away.
func lossyFields(candidates []clusterCandidate, winnerID uint) []string {
	var winner *models.Resource
	for i := range candidates {
		if candidates[i].Resource.ID == winnerID {
			winner = candidates[i].Resource
			break
		}
	}
	if winner == nil {
		return nil
	}

	var lossy []string
	losesDescription := strings.TrimSpace(winner.Description) == ""
	losesSeries := winner.SeriesID == nil
	// The default ResourceCategory is id 1 and every Resource has one, so "the
	// Winner has none" is not expressible. What matters is a Loser filed under a
	// *different* category, which the merge drops.
	winnerCategory := winner.ResourceCategoryId

	for i := range candidates {
		candidate := candidates[i].Resource
		if candidate.ID == winnerID {
			continue
		}
		if losesDescription && strings.TrimSpace(candidate.Description) != "" && !contains(lossy, "description") {
			lossy = append(lossy, "description")
		}
		if losesSeries && candidate.SeriesID != nil && !contains(lossy, "series") {
			lossy = append(lossy, "series")
		}
		if candidate.ResourceCategoryId != winnerCategory && !contains(lossy, "resource category") {
			lossy = append(lossy, "resource category")
		}
	}
	return lossy
}

// keepsSmaller reports whether the Winner has fewer pixels than a copy that would
// be removed.
//
// Pixels only, because a smaller *file* at the same resolution is usually the
// better copy, and comparing sizes would flag every Cluster under the "Smallest
// file" preset. A copy with no known resolution cannot be compared, so it never
// raises the flag on either side.
//
// Suppressed when the rule's first criterion is pixels_asc, since the reviewer
// asked for the smaller copy. Not suppressed once the reviewer has promoted a
// member by hand (decidedBy is manual): that mis-click is exactly what this
// exists to catch.
func keepsSmaller(candidates []clusterCandidate, winnerID uint, rule []string, decidedBy string) bool {
	if decidedBy != models.ManualCriterion && len(rule) > 0 && rule[0] == models.WinnerCriterionPixelsAsc {
		return false
	}
	var winner *models.Resource
	for i := range candidates {
		if candidates[i].Resource.ID == winnerID {
			winner = candidates[i].Resource
			break
		}
	}
	if winner == nil || pixelsOf(winner) == 0 {
		return false
	}
	for i := range candidates {
		other := candidates[i].Resource
		if other.ID != winnerID && pixelsOf(other) > pixelsOf(winner) {
			return true
		}
	}
	return false
}

func pixelsOf(r *models.Resource) uint64 {
	return uint64(r.Width) * uint64(r.Height)
}

func contains(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

// clusterID is stable across recomputes for the same membership, so a frozen
// Cluster keeps its identity and the page's per-Cluster controls keep addressing
// the same thing after a reload.
func clusterID(tier string, members []*models.ReductionMember) string {
	ids := make([]string, 0, len(members))
	for _, m := range members {
		ids = append(ids, strconv.FormatUint(uint64(m.ResourceID), 10))
	}
	sort.Strings(ids)
	sum := sha1.Sum([]byte(tier + ":" + strings.Join(ids, ",")))
	return tier + "-" + hex.EncodeToString(sum[:])[:16]
}

func ruleUsesAssociations(rule []string) bool {
	for _, c := range rule {
		if c == models.WinnerCriterionAssociationsDesc || c == models.WinnerCriterionAssociationsAsc {
			return true
		}
	}
	return false
}

// resourceAssociationCounts is how many tags, notes and related Groups one
// Resource holds.
type resourceAssociationCounts struct {
	Tags, Notes, Groups int
}

// countResourceAssociations totals the tags, notes and related groups each
// Resource holds — the "someone curated this copy" signal the Winner Rule can be
// asked to prefer.
func (ctx *MahresourcesContext) countResourceAssociations(ids []uint) (map[uint]int, error) {
	byKind, err := ctx.countResourceAssociationKinds(ids)
	if err != nil {
		return nil, err
	}
	counts := make(map[uint]int, len(byKind))
	for id, c := range byKind {
		counts[id] = c.Tags + c.Notes + c.Groups
	}
	return counts, nil
}

// countResourceAssociationKinds counts each Resource's tags, notes and related
// Groups separately, which is what a review card shows.
func (ctx *MahresourcesContext) countResourceAssociationKinds(ids []uint) (map[uint]resourceAssociationCounts, error) {
	counts := map[uint]resourceAssociationCounts{}

	// Two things make these counts respect the caller's subtree, and both are
	// necessary. The far side is the query's *model*, so the GORM scope callback
	// has a table it maps — counting the join table directly would include Notes
	// and Groups outside the subtree, letting a Winner be chosen on relationships
	// the reviewer cannot see and then printing the exact totals in the margin the
	// page renders. And each one finishes with Find rather than Scan: Scan runs
	// GORM's *Row* callback chain, on which the scope callback is not registered,
	// so it returns unfiltered rows however the query is built. See
	// TestScanBypassesTheSubtreeScopeCallback.
	//
	// Tags carry no owner and are global by design, so that one is counted on the
	// join table.
	sources := []struct {
		name  string
		add   func(c *resourceAssociationCounts, n int)
		build func(chunk []uint) *gorm.DB
	}{
		{"tags", func(c *resourceAssociationCounts, n int) { c.Tags += n }, func(chunk []uint) *gorm.DB {
			return ctx.db.Table("resource_tags").
				Select("resource_id, count(*) AS total").
				Where("resource_id IN ?", chunk).
				Group("resource_id")
		}},
		{"notes", func(c *resourceAssociationCounts, n int) { c.Notes += n }, func(chunk []uint) *gorm.DB {
			return ctx.db.Model(&models.Note{}).
				Joins("INNER JOIN resource_notes rn ON rn.note_id = notes.id").
				Select("rn.resource_id AS resource_id, count(*) AS total").
				Where("rn.resource_id IN ?", chunk).
				Group("rn.resource_id")
		}},
		{"groups", func(c *resourceAssociationCounts, n int) { c.Groups += n }, func(chunk []uint) *gorm.DB {
			return ctx.db.Model(&models.Group{}).
				Joins("INNER JOIN groups_related_resources grr ON grr.group_id = groups.id").
				Select("grr.resource_id AS resource_id, count(*) AS total").
				Where("grr.resource_id IN ?", chunk).
				Group("grr.resource_id")
		}},
	}

	for _, source := range sources {
		for _, chunk := range chunkUints(ids, idChunk) {
			var rows []struct {
				ResourceID uint
				Total      int
			}
			if err := source.build(chunk).Find(&rows).Error; err != nil {
				return nil, fmt.Errorf("counting %s associations: %w", source.name, err)
			}
			for _, row := range rows {
				c := counts[row.ResourceID]
				source.add(&c, row.Total)
				counts[row.ResourceID] = c
			}
		}
	}
	return counts, nil
}

func hashableContentTypes() []string {
	out := make([]string, 0, len(hash_worker.HashableContentTypes))
	for ct := range hash_worker.HashableContentTypes {
		out = append(out, ct)
	}
	sort.Strings(out)
	return out
}
