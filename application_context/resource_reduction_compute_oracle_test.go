package application_context

// The in-memory clustering the streaming compute replaced, kept as the oracle the
// differential test compares it against (design D14): the Clusters are built in Go
// slices and maps, and landed through storeLandComputedPlan. Nothing outside tests
// calls any of it.

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gorm.io/gorm/clause"

	"mahresources/constants"
	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/mrql"
)

// builtReductionPlan is what the in-memory clustering produced, ready to land: the
// fresh Clusters (carried ones are copied by the landing itself), the coverage and
// the settings the run used, and the version the plan is about.
type builtReductionPlan struct {
	reduction *models.ResourceReduction
	version   uint
	fresh     []*models.ReductionCluster
	coverage  models.ReductionCoverage
	settings  *models.ReductionSettings
}

// computeReductionPlan does the work: resolve the Extent, cluster it, holding out
// every Resource a carried judgement already claims.
func (ctx *MahresourcesContext) computeReductionPlan(jobCtx context.Context, reductionID uint, progress download_queue.ProgressSink) (*builtReductionPlan, error) {
	// Read unscoped by owner: this runs for the Reduction the request already
	// authorized, and the principal's *data* scope still applies through the db
	// handle. Re-applying the owner predicate here would fail under the auth-off
	// super-user, whose id the job does not carry.
	reduction, err := ctx.loadReductionForUpdate(reductionID, nil, false)
	if err != nil {
		return nil, err
	}
	// The version this plan is *about*. Everything below describes the Extent,
	// the Winner Rule and the decisions as they stand right now, so the landing
	// has to lose if any of them moved.
	built := &builtReductionPlan{reduction: reduction, version: reduction.Version}

	storedExtent, err := ctx.storedReductionExtent(reduction)
	if err != nil {
		return nil, err
	}
	rule := DecodeWinnerRule(reduction.WinnerRule)

	if progress != nil {
		progress.SetPhase("resolving the Extent")
	}
	extent, err := ctx.resolveReductionExtent(storedExtent)
	if err != nil {
		return nil, err
	}
	extent.ExcludeExternalResources = reduction.ExcludeExternalResources
	if err := jobCtx.Err(); err != nil {
		return nil, err
	}

	if progress != nil {
		progress.SetPhase("measuring coverage")
	}
	if built.coverage, err = ctx.reductionCoverage(extent); err != nil {
		return nil, err
	}

	// A judgement already made is never rearranged by a later compute. Frozen
	// Clusters carry over whole and their members are held out of the pool, so
	// growing the Extent can add Clusters but cannot move a Resource out of one
	// the reviewer has acted on.
	//
	// An *applied* Cluster is different, and deliberately: its Losers no longer
	// exist and its Winner goes back into the pool as an ordinary candidate. A
	// claim that is not confirmed is a keep decision in flight (or stranded by a
	// crash): its Resources stay out of the pool until it settles.
	excluded, err := ctx.reductionStore().carriedResourceIDs(reduction)
	if err != nil {
		return nil, err
	}

	if progress != nil {
		progress.SetPhase("Identical Resources")
	}
	identical, err := ctx.clusterIdentical(extent, rule, excluded, func(done, total int64) {
		if progress != nil {
			progress.SetPhaseProgress(done, total)
		}
	})
	if err != nil {
		return nil, err
	}
	if err := jobCtx.Err(); err != nil {
		return nil, err
	}

	// Every Resource an Identical Cluster claims — Winner and Losers alike —
	// leaves the pool before the perceptual tier runs. Byte-identical images
	// decode to identical pixels and are therefore also stored as distance-zero
	// pairs, so without this the same Resources would appear in two Clusters with
	// different defaults and possibly different Winners. When both apply, take the
	// fact.
	built.fresh = identical
	for _, cluster := range identical {
		for _, member := range cluster.Members {
			excluded[member.ResourceID] = true
		}
	}

	if reduction.MatchingMode != models.MatchingModeIdenticalOnly {
		if progress != nil {
			progress.SetPhase("Near-Identical Resources")
		}
		near, nearErr := ctx.clusterNearIdentical(jobCtx, extent, rule, excluded, func(done, total int64) {
			if progress != nil {
				progress.SetPhaseProgress(done, total)
			}
		})
		if nearErr != nil {
			return nil, nearErr
		}
		built.fresh = append(built.fresh, near...)
	}

	ensureDistinctClusterIDs(built.fresh)
	// What this plan was computed with, so the page can say when the settings have
	// since changed. The matching mode and rule are the normalised ones actually
	// used, not whatever the row's column happened to hold.
	built.settings = &models.ReductionSettings{
		MatchingMode:    normalizeMatchingMode(reduction.MatchingMode),
		WinnerRule:      rule,
		ExcludeExternal: reduction.ExcludeExternalResources,
	}
	return built, nil
}

// carriedResourceIDs is the pool a compute holds out: the members of every Cluster
// the landing will carry over that still claims them. An applied Cluster claims its
// members only until its merge is confirmed; read-only history claims only the
// copies a reviewer ejected (an explicit "don't remove" must not return in an
// auto-checked Cluster); any other frozen Cluster claims all of them.
func (s *reductionStore) carriedResourceIDs(r *models.ResourceReduction) (map[uint]bool, error) {
	out := map[uint]bool{}
	if r.PlanGeneration == nil {
		return out, nil
	}
	var after uint
	for {
		var rows []struct{ ID, ResourceID uint }
		if err := s.db.Raw(`SELECT m.id AS id, m.resource_id AS resource_id FROM reduction_members m JOIN reduction_clusters c ON c.id = m.cluster_id
			WHERE c.reduction_id = ? AND c.generation = ? AND m.id > ? AND (
				(c.state = 'applied' AND NOT c.merged)
				OR (c.state <> 'applied' AND c.read_only AND m.ejected)
				OR (c.state <> 'applied' AND NOT c.read_only AND (c.reviewed OR c.state IN ('skipped', 'stale'))))
			ORDER BY m.id LIMIT ?`, r.ID, *r.PlanGeneration, after, reductionRowChunk*10).Scan(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return out, nil
		}
		for _, row := range rows {
			out[row.ResourceID] = true
		}
		after = rows[len(rows)-1].ID
	}
}

// ensureDistinctClusterIDs guarantees no two Clusters in one plan answer to the
// same id.
//
// The id is derived from the tier and the member ids, which is what keeps a
// frozen Cluster addressable across recomputes — and it means a *carried* Cluster
// and a *fresh* one over the same members collide exactly. That is not
// hypothetical: an applied Cluster is carried forward while its members return to
// the pool, so a crash between an apply's claim and its merge leaves the Losers
// alive to re-cluster into the same set. findCluster takes the first match, so
// every override and every apply on the fresh Cluster would be refused as already
// settled, permanently.
//
// Numbering the collision rather than changing the derivation keeps the ordinary
// case stable, which is the property the derivation exists for.
func ensureDistinctClusterIDs(clusters []*models.ReductionCluster) {
	seen := make(map[string]bool, len(clusters))
	for _, cluster := range clusters {
		if !seen[cluster.ID] {
			seen[cluster.ID] = true
			continue
		}
		for suffix := 2; ; suffix++ {
			candidate := fmt.Sprintf("%s-%d", cluster.ID, suffix)
			if !seen[candidate] {
				cluster.ID = candidate
				seen[candidate] = true
				break
			}
		}
	}
}

// loadClusterCandidates reads the Resources behind a set of ids and decorates
// them with what the Winner Rule needs.
func (ctx *MahresourcesContext) loadClusterCandidates(ids []uint, rule []string, extent *ReductionExtent) ([]clusterCandidate, error) {
	return ctx.loadCandidateColumns(ids, rule, extent, nil)
}

// loadRankingCandidates is loadClusterCandidates reduced to the columns the
// Winner Rule compares, for ordering a pool too large to hold in full: the
// Near-Identical seed pool of a whole-library Extent is every paired image in it,
// and a full Resource row (description, meta, own meta) runs to kilobytes. A
// candidate loaded this way ranks exactly like a full one but is never a Cluster
// member: members are loaded whole.
func (ctx *MahresourcesContext) loadRankingCandidates(ids []uint, rule []string, extent *ReductionExtent) ([]clusterCandidate, error) {
	columns := []string{
		"resources.id", "resources.width", "resources.height", "resources.file_size",
		"resources.created_at", "resources.updated_at", "resources.name", "resources.content_type",
	}
	if contains(rule, models.WinnerCriterionHasDescription) {
		columns = append(columns, "resources.description")
	}
	return ctx.loadCandidateColumns(ids, rule, extent, columns)
}

func (ctx *MahresourcesContext) loadCandidateColumns(ids []uint, rule []string, extent *ReductionExtent, columns []string) ([]clusterCandidate, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	var resources []*models.Resource
	for _, chunk := range chunkUints(ids, idChunk) {
		var batch []*models.Resource
		q := ctx.db.Where("resources.id IN ?", chunk)
		if columns != nil {
			q = q.Select(columns)
		}
		if err := q.Find(&batch).Error; err != nil {
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
		// Filter before winner selection and near-match claiming so an external
		// match cannot displace an in-Extent Winner or split its Cluster.
		if extent.ExcludeExternalResources && !inExtent[r.ID] {
			continue
		}
		out = append(out, clusterCandidate{
			WinnerCandidate: models.WinnerCandidate{Resource: r, Associations: associations[r.ID]},
			InExtent:        inExtent[r.ID],
		})
	}
	return out, nil
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
		found, err := ctx.repeatedHashesIn(ids)
		if err != nil {
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
			if cluster := buildCluster(models.ReductionTierIdentical, sharing, rule, 0); cluster != nil {
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

// repeatedHashesIn is the content hashes of the named Resources that at least one
// other Resource also holds. Only those can form an Identical Cluster, and keeping
// every distinct hash instead held one string per Resource of a whole-library
// Extent in memory: millions, almost all of them unique.
//
// The other holder is looked up unscoped, which is safe: it only nominates the
// hash, and the candidates for it are read again through the scoped handle, so a
// twin outside the caller's subtree never becomes a member.
func (ctx *MahresourcesContext) repeatedHashesIn(ids []uint) ([]string, error) {
	var found []string
	err := ctx.db.Model(&models.Resource{}).
		Where("resources.id IN ?", ids).
		Where("resources.hash <> ''").
		Where("EXISTS (SELECT 1 FROM resources twin WHERE twin.hash = resources.hash AND twin.id <> resources.id)").
		Distinct().
		Pluck("resources.hash", &found).Error
	return found, err
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

// clusterNearIdentical groups the Extent by perceptual distance, greedy star,
// seeded in Winner Rule order. See ADR 0002.
//
// Perceptual similarity is not transitive: A within threshold of B and B of C
// says nothing about A and C, so connected components over the pair table lets
// one bridging image drag unrelated photographs into a single Cluster, and every
// deletion it proposed would rest on a transitive inference rather than a stored
// pair. Instead the candidates are sorted best-first by the Winner Rule and
// walked: the first unclaimed Resource becomes a seed and claims every unclaimed
// Resource within threshold *of itself*.
//
// What that buys is the property this feature is graded on — every proposed
// deletion is justified by a stored pair between that exact Loser and that exact
// Winner. The seed order being the Winner Rule is why the best copy claims its
// neighbours rather than an arbitrary one doing so.
//
// Fragmentation is the accepted cost: a genuine run of near-identicals splits
// across two Clusters when no single member is within threshold of all the rest.
func (ctx *MahresourcesContext) clusterNearIdentical(jobCtx context.Context, extent *ReductionExtent, rule []string, excluded map[uint]bool, report func(done, total int64)) ([]*models.ReductionCluster, error) {
	seedIDs, err := ctx.perceptualExtentIDs(extent, excluded)
	if err != nil {
		return nil, err
	}
	// One seed is enough. D7 lets a Cluster reach outside the Extent precisely so
	// the better copy sitting elsewhere is visible, and an Extent of one Resource
	// with a stored pair to a Resource outside it is exactly that case — the
	// question "is there already a better copy of this?" is the one a reviewer
	// asks about a single photograph.
	if len(seedIDs) == 0 {
		return nil, nil
	}

	seeds, err := ctx.loadRankingCandidates(seedIDs, rule, extent)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(seeds, func(i, j int) bool {
		return models.CompareByWinnerRule(rule, seeds[i].WinnerCandidate, seeds[j].WinnerCandidate) < 0
	})

	threshold := ctx.similarityThreshold()
	claimed := map[uint]bool{}
	var clusters []*models.ReductionCluster

	for i, seed := range seeds {
		if err := jobCtx.Err(); err != nil {
			return nil, err
		}
		if report != nil {
			report(int64(i+1), int64(len(seeds)))
		}
		if claimed[seed.Resource.ID] || excluded[seed.Resource.ID] {
			continue
		}

		neighbours, err := ctx.similarWithin(seed.Resource.ID, threshold)
		if err != nil {
			return nil, err
		}
		if len(neighbours) == 0 {
			continue
		}

		// A neighbour whose ImageHash row is gone has had its file replaced and not
		// yet been re-hashed — OnResourceFileChanged deletes the row and re-queues
		// the work — while its stored pairs, which carry no foreign key to
		// image_hashes, survive describing content it no longer holds. Those pairs
		// are the justification for a deletion, so a Cluster must not be built on
		// one. (The wider defect is pre-existing and named as out of scope in the
		// plan: once the worker re-hashes, the stale pairs are still there. What the
		// review protects against then is what it always did — the reviewer looking
		// at two images that are not alike.)
		hashed, err := ctx.perceptuallyHashed(pairIDs(neighbours))
		if err != nil {
			return nil, err
		}

		distances := map[uint]uint8{}
		neighbourIDs := make([]uint, 0, len(neighbours))
		for _, pair := range neighbours {
			if claimed[pair.ResourceID] || excluded[pair.ResourceID] || pair.ResourceID == seed.Resource.ID {
				continue
			}
			if !hashed[pair.ResourceID] {
				continue
			}
			// A neighbour reached twice at different distances keeps the closest,
			// which is the one the pair table would answer with.
			if existing, seen := distances[pair.ResourceID]; seen && existing <= pair.Distance {
				continue
			}
			if _, seen := distances[pair.ResourceID]; !seen {
				neighbourIDs = append(neighbourIDs, pair.ResourceID)
			}
			distances[pair.ResourceID] = pair.Distance
		}
		if len(neighbourIDs) == 0 {
			continue
		}

		// The neighbours are loaded rather than taken from `seeds`, because a
		// neighbour may sit outside the Extent — which is exactly the better copy
		// elsewhere the reviewer needs to see, and it is never a seed.
		//
		// The seed is loaded again with them: it was ranked from the Winner Rule's
		// columns alone, and a Cluster member needs the whole row. One that has
		// gone since the pool was read simply forms no Cluster.
		members, err := ctx.loadClusterCandidates(append(neighbourIDs, seed.Resource.ID), rule, extent)
		if err != nil {
			return nil, err
		}
		seedFound := false
		for i := range members {
			if members[i].Resource.ID == seed.Resource.ID {
				seedFound = true
				continue
			}
			d := distances[members[i].Resource.ID]
			members[i].Distance = &d
		}
		if !seedFound {
			continue
		}

		cluster := buildCluster(models.ReductionTierNear, members, rule, 0)
		if cluster == nil {
			continue
		}

		// The out-of-Extent rule can move the Winner off the seed, and ADR 0002's
		// pair-justification holds only while the Winner *is* the seed: the
		// members were chosen for being within threshold of the seed, and nothing
		// says they are within threshold of an outsider that displaced it. The
		// amendment says every change of Winner re-checks, so this is the same
		// re-check promote does, applied at the one other place a Winner moves.
		// Leaving it to apply would mean proposing a deletion the ADR forbids and
		// only refusing it at the end.
		if cluster.WinnerID != seed.Resource.ID {
			if err := ctx.rejustifyAgainstWinner(cluster); err != nil {
				return nil, err
			}
			if len(cluster.LoserIDs()) == 0 {
				continue
			}
			if err := ctx.refreshLossy(cluster, rule); err != nil {
				return nil, err
			}
		}

		// Claimed on the strength of being in the Cluster, so nothing is proposed
		// twice. The seed is claimed whether or not it ended as the Winner: an
		// out-of-Extent member may have displaced it, and it is still spoken for.
		claimed[seed.Resource.ID] = true
		for _, member := range cluster.Members {
			claimed[member.ResourceID] = true
		}
		clusters = append(clusters, cluster)
	}
	return clusters, nil
}

// pairIDs is the neighbour half of a set of stored pairs.
func pairIDs(pairs []similarPair) []uint {
	ids := make([]uint, 0, len(pairs))
	for _, pair := range pairs {
		ids = append(ids, pair.ResourceID)
	}
	return ids
}

// perceptuallyHashed reports which of the given Resources currently carry a
// perceptual hash.
func (ctx *MahresourcesContext) perceptuallyHashed(ids []uint) (map[uint]bool, error) {
	out := map[uint]bool{}
	for _, chunk := range chunkUints(dedupeUints(ids), idChunk) {
		var found []uint
		if err := ctx.db.Model(&models.ImageHash{}).
			Where("image_hashes.resource_id IN ?", chunk).
			Pluck("image_hashes.resource_id", &found).Error; err != nil {
			return nil, fmt.Errorf("checking perceptual hashes: %w", err)
		}
		for _, id := range found {
			out[id] = true
		}
	}
	return out, nil
}

// perceptualExtentIDs is the seed pool: the Extent's Resources that carry a
// perceptual hash and appear in at least one stored pair, minus anything a frozen
// Cluster or the Identical tier already claimed. An image with no stored pair row
// has no neighbour to find, and leaving it out keeps a whole-library pool to the
// paired images rather than every hashed one. This is a coarse cut, not the read
// threshold: a pair the threshold or the aHash guard rejects still admits its
// images, and similarWithin then finds nothing for them, as before.
func (ctx *MahresourcesContext) perceptualExtentIDs(extent *ReductionExtent, excluded map[uint]bool) ([]uint, error) {
	var ids []uint
	err := ctx.extentResourceIDs(extent, func(chunk []uint) error {
		var hashed []uint
		if err := ctx.db.Model(&models.ImageHash{}).
			Where("image_hashes.resource_id IN ?", chunk).
			Where("(EXISTS (SELECT 1 FROM resource_similarities s WHERE s.resource_id1 = image_hashes.resource_id) OR "+
				"EXISTS (SELECT 1 FROM resource_similarities s WHERE s.resource_id2 = image_hashes.resource_id))").
			Pluck("image_hashes.resource_id", &hashed).Error; err != nil {
			return err
		}
		for _, id := range hashed {
			if !excluded[id] {
				ids = append(ids, id)
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("collecting perceptually hashed Resources: %w", err)
	}
	return ids, nil
}

// similarWithin returns the stored pairs for one Resource within the read
// threshold, probed from both directions because the pair table stores each edge
// once with the lower id first.
//
// This is NOT scoped, and it is important to be exact about why that is safe.
// The GORM scope callback keys on the statement's own table, and scopeColumn maps
// only groups, resources and notes — so a query whose model is ResourceSimilarity
// carries no subtree filter however it joins. What confines the result is the
// step after it: every id returned here is looked up through loadClusterCandidates,
// whose model *is* Resource, and a neighbour outside the reviewer's subtree simply
// does not come back and never becomes a member. The pair re-check at apply is
// likewise a membership test over ids that were already scope-checked.
//
// The zero-pairs d_hash equality fallback the detail page's similar-resources
// panel carries is deliberately not inherited. It answers a different question —
// "show me something when the pair table is empty" — and here an empty pair table
// means the coverage line should say so, not that the clustering should invent
// edges.
func (ctx *MahresourcesContext) similarWithin(resourceID uint, threshold int) ([]similarPair, error) {
	return ctx.similarWithinLocking(resourceID, threshold, false)
}

// similarWithinLocking is similarWithin with an optional row lock on the pairs it
// reads.
//
// Locking matters in exactly one place: the merge's precondition, where the pair
// is the justification for a deletion about to happen. Locking the Resource rows
// does not lock resource_similarities, so a similarity recompute can delete the
// very edge the precondition just verified and commit before the merge runs — and
// ADR 0002's guarantee would then be true up to the delete rather than through it.
// Postgres-only, like every other lock here: SQLite serializes writers and rejects
// the clause.
func (ctx *MahresourcesContext) similarWithinLocking(resourceID uint, threshold int, lock bool) ([]similarPair, error) {
	// The pair qualifies by the rule every read of the pair table applies, the
	// aHash guard included, so a Reduction never proposes a match the similar list
	// and SIMILAR TO would reject.
	_, aThreshold := ctx.similarityThresholds()
	distance := mrql.SimilarPairDistance("resource_similarities")
	within := mrql.SimilarPairPredicate("resource_similarities", threshold, aThreshold)

	var out []similarPair
	for _, direction := range []struct{ self, other string }{
		{"resource_id1", "resource_id2"},
		{"resource_id2", "resource_id1"},
	} {
		var rows []similarPair
		query := ctx.db.Model(&models.ResourceSimilarity{})
		if lock && ctx.Config.DbType != constants.DbTypeSqlite {
			// OF the pair table specifically: the joined resources rows are already
			// locked by the merge, and naming them again is noise.
			query = query.Clauses(clause.Locking{Strength: "UPDATE", Table: clause.Table{Name: "resource_similarities"}})
		}
		if err := query.
			Joins("INNER JOIN resources ON resources.id = resource_similarities."+direction.other).
			Select("resource_similarities."+direction.other+" AS resource_id, "+distance+" AS distance").
			Where("resource_similarities."+direction.self+" = ?", resourceID).
			Where(within).
			Scan(&rows).Error; err != nil {
			return nil, fmt.Errorf("reading similarity pairs for resource %d: %w", resourceID, err)
		}
		out = append(out, rows...)
	}
	return out, nil
}

// ReductionExtent is a Reduction's Extent, resolved.
//
// Group ids are expanded through their descendants here rather than at rest,
// because a Group's descendants and its contents both change and D23 makes
// re-scanning an explicit act. This runs once per compute.
type ReductionExtent struct {
	// ExcludeExternalResources confines clustering candidates to this Extent.
	ExcludeExternalResources bool
	// ResourceIDs are the Resources named explicitly — filtered, like everything
	// here, to what the current principal may see.
	ResourceIDs map[uint]bool
	// GroupIDs are the selected Groups and every descendant of each.
	GroupIDs map[uint]bool
	// SelectedGroups is how many of the *named* Groups the principal may see, as
	// opposed to how many Groups the expansion reached. The page reports what was
	// selected, and after somebody's subtree shrinks that number has to shrink
	// with it: an exact count of Groups they can no longer open is a fact about
	// those Groups.
	SelectedGroups int
	// All is the whole library as the principal can see it. Nothing is listed: the
	// scoped handle decides what "everything" is, on every use.
	All bool
}

// Empty reports an Extent that names nothing.
func (e *ReductionExtent) Empty() bool {
	return !e.All && len(e.ResourceIDs) == 0 && len(e.GroupIDs) == 0
}

// resolveReductionExtent expands the stored Extent against the current database
// and the current principal.
//
// Scoping is the db handle's, not this function's: ctx.db carries the principal's
// subtree filter through the GORM callback, so a confined reviewer's Extent
// silently loses the Resources and Groups outside their subtree rather than this
// code having to remember to filter. That is also why a Reduction is re-resolved
// on every compute and every render instead of being expanded once and stored: a
// scoped user's subtree can change after a Reduction is created.
func (ctx *MahresourcesContext) resolveReductionExtent(stored models.ResourceReductionExtent) (*ReductionExtent, error) {
	extent := &ReductionExtent{
		ResourceIDs: map[uint]bool{},
		GroupIDs:    map[uint]bool{},
		All:         stored.All,
	}

	// Explicit Resource ids, filtered to the ones that still exist and that this
	// principal may see. A deleted Resource simply leaves the Extent.
	for _, chunk := range chunkUints(stored.ResourceIDs, idChunk) {
		var found []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.id IN ?", chunk).
			Pluck("resources.id", &found).Error; err != nil {
			return nil, fmt.Errorf("resolving the Extent's Resources: %w", err)
		}
		for _, id := range found {
			extent.ResourceIDs[id] = true
		}
	}

	// Groups, each expanded through its own subtree. A Reduction over a parent
	// Group covers the subtree the reviewer actually means.
	for _, root := range stored.GroupIDs {
		if !ctx.GroupVisible(root) {
			continue
		}
		ids, err := ctx.collectSubtreeGroupIDs(root)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			// GroupVisible short-circuits to true for an unscoped principal, so a
			// Group deleted since it was selected passes that check and reaches
			// here. Its own subtree is empty, which is how the deletion shows.
			continue
		}
		for _, id := range ids {
			extent.GroupIDs[id] = true
		}
		extent.SelectedGroups++
	}

	return extent, nil
}

// extentResourceIDs streams every Resource id in the Extent through fn, in
// chunks, so a wide Extent never becomes one enormous slice.
//
// A Resource is in a Group's Extent if the Group owns it or is related to it.
// Both, because both are how the app files a Resource under a Group and the
// resources list shows both — an Extent that covered only one would silently miss
// half of what "everything filed under Holidays" means.
func (ctx *MahresourcesContext) extentResourceIDs(extent *ReductionExtent, fn func(ids []uint) error) error {
	if extent.All {
		// Everything the handle can see, in id-keyset pages: no OFFSET to rescan on a
		// library of millions, and no set of seen ids to hold, because each id is
		// visited exactly once. Pluck runs the scope callback, so a confined
		// principal's "everything" is their subtree.
		return ctx.walkVisibleResourceIDs(fn)
	}
	seen := map[uint]bool{}
	emit := func(ids []uint) error {
		fresh := make([]uint, 0, len(ids))
		for _, id := range ids {
			if !seen[id] {
				seen[id] = true
				fresh = append(fresh, id)
			}
		}
		if len(fresh) == 0 {
			return nil
		}
		return fn(fresh)
	}

	for _, chunk := range chunkUints(mapKeys(extent.ResourceIDs), idChunk) {
		if err := emit(chunk); err != nil {
			return err
		}
	}

	if len(extent.GroupIDs) == 0 {
		return nil
	}
	groupIDs := mapKeys(extent.GroupIDs)
	for _, groupChunk := range chunkUints(groupIDs, idChunk) {
		var owned []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.owner_id IN ?", groupChunk).
			Pluck("resources.id", &owned).Error; err != nil {
			return fmt.Errorf("resolving the Extent's owned Resources: %w", err)
		}
		for _, c := range chunkUints(owned, idChunk) {
			if err := emit(c); err != nil {
				return err
			}
		}

		var related []uint
		if err := ctx.db.Model(&models.Resource{}).
			Joins("INNER JOIN groups_related_resources grr ON grr.resource_id = resources.id").
			Where("grr.group_id IN ?", groupChunk).
			Pluck("resources.id", &related).Error; err != nil {
			return fmt.Errorf("resolving the Extent's related Resources: %w", err)
		}
		for _, c := range chunkUints(related, idChunk) {
			if err := emit(c); err != nil {
				return err
			}
		}
	}
	return nil
}

// countExtentResources is how many distinct Resources the Extent holds.
//
// This walks the whole Extent, so it is deliberately called once per compute and
// never on a page render. A Reduction over a top-level Group in a library of
// millions reaches millions of Resources, and paying that per page view is the
// difference between a review surface and an outage. What the page shows instead
// is the figure the last compute recorded, which is also the figure its coverage
// line is about.
func (ctx *MahresourcesContext) countExtentResources(extent *ReductionExtent) (int, error) {
	if extent.All {
		var count int64
		if err := ctx.db.Model(&models.Resource{}).Count(&count).Error; err != nil {
			return 0, fmt.Errorf("counting the library: %w", err)
		}
		return int(count), nil
	}
	total := 0
	err := ctx.extentResourceIDs(extent, func(ids []uint) error {
		total += len(ids)
		return nil
	})
	return total, err
}

// extentArrivalsSince counts the Resources that entered the Extent after the
// given instant — the drift figure the page reports so a Group-scoped Reduction
// can be seen going out of date rather than surprising the reviewer.
//
// Filtered on created_at first and deduplicated over ids rather than summed over
// counts. The filter is what keeps this cheap: however wide the Extent, only the
// Resources created since the last compute are ever materialised, and a Resource
// that is both named explicitly and owned by a named Group must be counted once.
func (ctx *MahresourcesContext) extentArrivalsSince(extent *ReductionExtent, since time.Time) (int, error) {
	if extent.All {
		var count int64
		if err := ctx.db.Model(&models.Resource{}).Where("resources.created_at > ?", since).Count(&count).Error; err != nil {
			return 0, err
		}
		return int(count), nil
	}
	arrivals := map[uint]bool{}
	collect := func(ids []uint) {
		for _, id := range ids {
			arrivals[id] = true
		}
	}

	for _, chunk := range chunkUints(mapKeys(extent.ResourceIDs), idChunk) {
		var found []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.id IN ?", chunk).
			Where("resources.created_at > ?", since).
			Pluck("resources.id", &found).Error; err != nil {
			return 0, err
		}
		collect(found)
	}

	for _, groupChunk := range chunkUints(mapKeys(extent.GroupIDs), idChunk) {
		var owned []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.owner_id IN ?", groupChunk).
			Where("resources.created_at > ?", since).
			Pluck("resources.id", &owned).Error; err != nil {
			return 0, err
		}
		collect(owned)

		var related []uint
		if err := ctx.db.Model(&models.Resource{}).
			Joins("INNER JOIN groups_related_resources grr ON grr.resource_id = resources.id").
			Where("grr.group_id IN ?", groupChunk).
			Where("resources.created_at > ?", since).
			Pluck("resources.id", &related).Error; err != nil {
			return 0, err
		}
		collect(related)
	}

	return len(arrivals), nil
}

// containsResources reports which of the given Resource ids are inside the
// Extent. Asked of the Cluster candidates rather than of the whole library, so
// the answer costs one query per chunk of candidates however wide the Extent is.
func (ctx *MahresourcesContext) containsResources(extent *ReductionExtent, candidates []uint) (map[uint]bool, error) {
	inside := map[uint]bool{}
	if extent.All {
		// Everything the handle can see is inside; asking the database is what makes
		// a candidate it cannot see fall outside, rather than assuming.
		for _, chunk := range chunkUints(candidates, idChunk) {
			var found []uint
			if err := ctx.db.Model(&models.Resource{}).
				Where("resources.id IN ?", chunk).
				Pluck("resources.id", &found).Error; err != nil {
				return nil, err
			}
			for _, id := range found {
				inside[id] = true
			}
		}
		return inside, nil
	}
	remaining := make([]uint, 0, len(candidates))
	for _, id := range candidates {
		if extent.ResourceIDs[id] {
			inside[id] = true
			continue
		}
		remaining = append(remaining, id)
	}
	if len(remaining) == 0 || len(extent.GroupIDs) == 0 {
		return inside, nil
	}

	groupIDs := mapKeys(extent.GroupIDs)
	for _, candidateChunk := range chunkUints(remaining, idChunk) {
		for _, groupChunk := range chunkUints(groupIDs, idChunk) {
			var owned []uint
			if err := ctx.db.Model(&models.Resource{}).
				Where("resources.id IN ?", candidateChunk).
				Where("resources.owner_id IN ?", groupChunk).
				Pluck("resources.id", &owned).Error; err != nil {
				return nil, err
			}
			for _, id := range owned {
				inside[id] = true
			}

			var related []uint
			if err := ctx.db.Model(&models.Resource{}).
				Joins("INNER JOIN groups_related_resources grr ON grr.resource_id = resources.id").
				Where("resources.id IN ?", candidateChunk).
				Where("grr.group_id IN ?", groupChunk).
				Pluck("resources.id", &related).Error; err != nil {
				return nil, err
			}
			for _, id := range related {
				inside[id] = true
			}
		}
	}
	return inside, nil
}

// walkVisibleResourceIDs streams every Resource id the handle can see through fn,
// idChunk at a time, ascending by id.
func (ctx *MahresourcesContext) walkVisibleResourceIDs(fn func(ids []uint) error) error {
	var last uint
	for {
		var ids []uint
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.id > ?", last).
			Order("resources.id ASC").
			Limit(idChunk).
			Pluck("resources.id", &ids).Error; err != nil {
			return fmt.Errorf("walking the library: %w", err)
		}
		if len(ids) == 0 {
			return nil
		}
		if err := fn(ids); err != nil {
			return err
		}
		last = ids[len(ids)-1]
	}
}

// rejustifyAgainstWinner ejects every Loser with no stored pair to the current
// Winner within threshold, and records the distance of the ones that keep their
// place.
func (ctx *MahresourcesContext) rejustifyAgainstWinner(cluster *models.ReductionCluster) error {
	pairs, err := ctx.similarWithin(cluster.WinnerID, ctx.similarityThreshold())
	if err != nil {
		return err
	}
	closest := map[uint]uint8{}
	for _, pair := range pairs {
		if existing, seen := closest[pair.ResourceID]; seen && existing <= pair.Distance {
			continue
		}
		closest[pair.ResourceID] = pair.Distance
	}

	for _, member := range cluster.Members {
		if member.ResourceID == cluster.WinnerID {
			member.Distance = nil
			continue
		}
		// An outsider may never lose: whatever its pair state to the new Winner, it
		// stays out of the merge proposal. Relabelling its ejection no-pair-to-winner
		// would be the restore path's own trigger — the next promotion whose Winner
		// does have a surviving pair to the outsider would then read the relabel as
		// "a previous promotion's lapse" and re-arm it into a Loser.
		if !member.InExtent {
			continue
		}
		distance, paired := closest[member.ResourceID]
		if !paired {
			// Left untouched by the ejection itself — that is what makes ejection
			// safe — but out of this Cluster, and labelled so the reviewer can see
			// that their promotion is what moved it.
			member.Ejected = true
			member.EjectedReason = models.EjectReasonNoPairToWinner
			member.Distance = nil
			continue
		}
		if member.Ejected && member.EjectedReason == models.EjectReasonNoPairToWinner {
			// A previous promotion ejected it for want of a pair to a different
			// Winner. It has one to this Winner, so the reason has lapsed.
			member.Ejected = false
			member.EjectedReason = ""
		}
		d := distance
		member.Distance = &d
	}
	return nil
}

// refreshLossy recomputes the curation warnings after a change of Winner or of
// membership: which fields a merge would now discard, and whether it would now
// discard a higher-resolution copy, are properties of the Winner and the
// remaining members, not of the Cluster. The rule is what tells a deliberately
// smaller Winner (pixels_asc first) from a mistaken one.
func (ctx *MahresourcesContext) refreshLossy(cluster *models.ReductionCluster, rule []string) error {
	ids := make([]uint, 0, len(cluster.Members))
	for _, member := range cluster.Members {
		if !member.Ejected {
			ids = append(ids, member.ResourceID)
		}
	}
	resources, err := ctx.loadResourcesByID(ids)
	if err != nil {
		return err
	}
	candidates := make([]clusterCandidate, 0, len(ids))
	for _, id := range ids {
		if r := resources[id]; r != nil {
			candidates = append(candidates, clusterCandidate{WinnerCandidate: models.WinnerCandidate{Resource: r}})
		}
	}
	cluster.Lossy = lossyFields(candidates, cluster.WinnerID)
	cluster.KeepsSmaller = keepsSmaller(candidates, cluster.WinnerID, rule, cluster.DecidedBy)
	return nil
}

// storeLandComputedPlan lands a finished compute: the Clusters it built in memory
// (fresh), the coverage and the settings it ran with. Every staging transaction
// begins with the fence, so a compute that expired, was superseded or whose
// Reduction was deleted writes nothing more.
//
//  1. its own generation's rows are deleted first (an earlier attempt's);
//  2. the carried Clusters are copied from the plan generation: Cluster rows 500 per
//     statement with every column, then member rows by keyset on (source cluster,
//     position) 500 per statement, remapped by key, with `gone` taken from the
//     share-locked Resources;
//  3. each fresh Cluster is inserted in a transaction of its own;
//  4. one landing transaction: the version compare-and-set, the job id, a live lease,
//     the generation, coverage, settings and status;
//  5. the generation's staging rows are deleted, and stale generations swept.
func (ctx *MahresourcesContext) storeLandComputedPlan(r *models.ResourceReduction, jobID, generation string, version uint, fresh []*models.ReductionCluster, coverage models.ReductionCoverage, settings *models.ReductionSettings) error {
	return ctx.landComputedPlan(r, jobID, generation, version, fresh, coverage, settings, true)
}
