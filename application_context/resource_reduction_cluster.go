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
func buildCluster(tier string, candidates []clusterCandidate, rule []string) *models.ReductionCluster {
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

	winner := kept[0]
	runnerUp := kept[1]
	decidedBy, margin, undecided := models.DecidingCriterion(rule, winner.WinnerCandidate, runnerUp.WinnerCandidate)

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

// loadClusterCandidates reads the Resources behind a set of ids and decorates
// them with what the Winner Rule needs.
func (ctx *MahresourcesContext) loadClusterCandidates(ids []uint, rule []string, extent *ReductionExtent) ([]clusterCandidate, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var resources []*models.Resource
	for _, chunk := range chunkUints(ids, idChunk) {
		var batch []*models.Resource
		if err := ctx.db.Where("resources.id IN ?", chunk).Find(&batch).Error; err != nil {
			return nil, fmt.Errorf("loading Cluster candidates: %w", err)
		}
		resources = append(resources, batch...)
	}
	if len(resources) == 0 {
		return nil, nil
	}

	loaded := make([]uint, 0, len(resources))
	for _, r := range resources {
		loaded = append(loaded, r.ID)
	}

	inExtent, err := ctx.containsResources(extent, loaded)
	if err != nil {
		return nil, err
	}

	// Only when the rule asks. Three GROUP BY queries over the candidate set are
	// cheap next to the clustering itself, but they are not free, and the default
	// rule never consults them.
	var associations map[uint]int
	if ruleUsesAssociations(rule) {
		associations, err = ctx.countResourceAssociations(loaded)
		if err != nil {
			return nil, err
		}
	}

	out := make([]clusterCandidate, 0, len(resources))
	for _, r := range resources {
		out = append(out, clusterCandidate{
			WinnerCandidate: models.WinnerCandidate{Resource: r, Associations: associations[r.ID]},
			InExtent:        inExtent[r.ID],
		})
	}
	return out, nil
}

func ruleUsesAssociations(rule []string) bool {
	for _, c := range rule {
		if c == models.WinnerCriterionAssociationsDesc || c == models.WinnerCriterionAssociationsAsc {
			return true
		}
	}
	return false
}

// countResourceAssociations totals the tags, notes and related groups each
// Resource holds — the "someone curated this copy" signal the Winner Rule can be
// asked to prefer.
func (ctx *MahresourcesContext) countResourceAssociations(ids []uint) (map[uint]int, error) {
	counts := map[uint]int{}

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
		build func(chunk []uint) *gorm.DB
	}{
		{"tags", func(chunk []uint) *gorm.DB {
			return ctx.db.Table("resource_tags").
				Select("resource_id, count(*) AS total").
				Where("resource_id IN ?", chunk).
				Group("resource_id")
		}},
		{"notes", func(chunk []uint) *gorm.DB {
			return ctx.db.Model(&models.Note{}).
				Joins("INNER JOIN resource_notes rn ON rn.note_id = notes.id").
				Select("rn.resource_id AS resource_id, count(*) AS total").
				Where("rn.resource_id IN ?", chunk).
				Group("rn.resource_id")
		}},
		{"groups", func(chunk []uint) *gorm.DB {
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
				counts[row.ResourceID] += row.Total
			}
		}
	}
	return counts, nil
}

// clusterIdentical groups the Extent by content hash.
//
// The empty hash is excluded, and that exclusion is load-bearing rather than
// tidy: resources.hash is a plain string and "" is a live value in this schema,
// so a GROUP BY would collapse every hashless Resource into one Cluster —
// arriving checked, exempt from the size cap, and proposing to delete files that
// have nothing whatever to do with each other. Hashless Resources are reported in
// the coverage line instead.
//
// A Cluster's members are every Resource sharing the hash, not only the ones in
// the Extent, so the better copy sitting elsewhere is visible. buildCluster is
// what keeps exactly one of those and never lets it lose.
func (ctx *MahresourcesContext) clusterIdentical(extent *ReductionExtent, rule []string, excluded map[uint]bool, report func(done, total int64)) ([]*models.ReductionCluster, error) {
	hashes := map[string]bool{}
	err := ctx.extentResourceIDs(extent, func(ids []uint) error {
		var found []string
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.id IN ?", ids).
			Where("resources.hash <> ''").
			Distinct().
			Pluck("resources.hash", &found).Error; err != nil {
			return err
		}
		for _, h := range found {
			if h != "" {
				hashes[h] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collecting the Extent's content hashes: %w", err)
	}
	if len(hashes) == 0 {
		return nil, nil
	}

	hashList := make([]string, 0, len(hashes))
	for h := range hashes {
		hashList = append(hashList, h)
	}
	sort.Strings(hashList)

	var clusters []*models.ReductionCluster
	done := int64(0)
	total := int64(len(hashList))
	for start := 0; start < len(hashList); start += idChunk {
		end := start + idChunk
		if end > len(hashList) {
			end = len(hashList)
		}
		chunk := hashList[start:end]

		var rows []struct {
			ID   uint
			Hash string
		}
		// Find, not Scan: Scan runs the Row callback chain, which carries no
		// subtree filter, so this would offer candidates from outside the caller's
		// subtree. loadClusterCandidates would drop them again — it uses Find — but
		// a Cluster is not the place to rely on a second line.
		if err := ctx.db.Model(&models.Resource{}).
			Select("resources.id, resources.hash").
			Where("resources.hash IN ?", chunk).
			Find(&rows).Error; err != nil {
			return nil, fmt.Errorf("collecting Identical candidates: %w", err)
		}

		byHash := map[string][]uint{}
		for _, row := range rows {
			if excluded[row.ID] {
				continue
			}
			byHash[row.Hash] = append(byHash[row.Hash], row.ID)
		}

		for _, hash := range chunk {
			ids := byHash[hash]
			if len(ids) < 2 {
				continue
			}
			candidates, err := ctx.loadClusterCandidates(ids, rule, extent)
			if err != nil {
				return nil, err
			}
			// Re-checked against the rows as loaded, not trusted from the grouping
			// query above. A version upload landing between the two rewrites
			// resources.hash, and a Cluster built from the stale grouping would carry
			// members whose recorded hashes differ — labelled Identical, arriving
			// checked, and passing apply's revalidation, because each member is
			// validated against its own snapshot rather than against the others.
			// Byte-identity is the one thing this tier is allowed to assert.
			sharing := make([]clusterCandidate, 0, len(candidates))
			for _, candidate := range candidates {
				if candidate.Resource.Hash == hash {
					sharing = append(sharing, candidate)
				}
			}
			if cluster := buildCluster(models.ReductionTierIdentical, sharing, rule); cluster != nil {
				clusters = append(clusters, cluster)
			}
		}

		done += int64(len(chunk))
		if report != nil {
			report(done, total)
		}
	}
	return clusters, nil
}

// reductionCoverage reports how much of the Extent could actually be examined, so
// "no repeats found" stays distinguishable from "nothing was hashed".
func (ctx *MahresourcesContext) reductionCoverage(extent *ReductionExtent) (models.ReductionCoverage, error) {
	size, err := ctx.countExtentResources(extent)
	if err != nil {
		return models.ReductionCoverage{}, err
	}
	coverage := models.ReductionCoverage{ExtentSize: size}
	eligible := hashableContentTypes()

	err = ctx.extentResourceIDs(extent, func(ids []uint) error {
		var withHash int64
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.id IN ?", ids).
			Where("resources.hash <> ''").
			Count(&withHash).Error; err != nil {
			return err
		}
		coverage.ContentHashed += int(withHash)

		var eligibleCount int64
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.id IN ?", ids).
			Where("resources.content_type IN ?", eligible).
			Count(&eligibleCount).Error; err != nil {
			return err
		}
		coverage.PerceptualEligible += int(eligibleCount)

		var hashed int64
		if err := ctx.db.Model(&models.ImageHash{}).
			Where("image_hashes.resource_id IN ?", ids).
			Count(&hashed).Error; err != nil {
			return err
		}
		coverage.PerceptualHashed += int(hashed)
		return nil
	})
	return coverage, err
}

func hashableContentTypes() []string {
	out := make([]string, 0, len(hash_worker.HashableContentTypes))
	for ct := range hash_worker.HashableContentTypes {
		out = append(out, ct)
	}
	sort.Strings(out)
	return out
}
