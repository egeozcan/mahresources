package application_context

import (
	"bytes"
	"fmt"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mahresources/models"
	"mahresources/mrql"
)

// neighbourPageSize is how many pairs one neighbour statement reads per direction
// (design D5): 400, or the chunk size when a test shrinks it.
func neighbourPageSize() int { return min(400, computeChunk) }

// nearTier groups the staged Extent by perceptual distance, greedy star, seeded in
// Winner Rule order (ADR 0002). The seeds are staged with their byte keys and read
// back in key order, so the order never lives in Go.
//
// Perceptual similarity is not transitive, so connected components would let one
// bridging image drag unrelated photographs into a Cluster. Instead the first
// available seed claims every available Resource within threshold *of itself*,
// nearest first, up to the member cap: every proposed deletion is justified by a
// stored pair between that exact Loser and that exact Winner.
func (run *reductionStream) nearTier() error {
	total, err := run.stageSeeds()
	if err != nil || total == 0 {
		return err
	}
	if err := run.rankTiedSeeds(); err != nil {
		return err
	}
	// Read only now: a library with nothing to match never needs the similarity settings.
	run.threshold = run.ctx.similarityThreshold()
	_, run.aThreshold = run.ctx.similarityThresholds()
	var (
		cursor *seedCursor
		done   int64
	)
	for {
		if err := run.jobCtx.Err(); err != nil {
			return err
		}
		seeds, err := run.seedPage(cursor)
		if err != nil {
			return err
		}
		if len(seeds) == 0 {
			return nil
		}
		for _, seed := range seeds {
			// Re-checked per seed: an earlier Cluster of this very pass may have claimed it.
			ok, err := run.isAvailable(seed.ResourceID)
			if err != nil {
				return err
			}
			if ok {
				id := seed.ResourceID
				if err := run.insertBuilt(func() (*pendingCluster, error) { return run.buildStar(id) }); err != nil {
					return err
				}
			}
			done++
			run.report(done, total)
		}
		last := seeds[len(seeds)-1]
		cursor = &seedCursor{KeyPrefix: last.KeyPrefix, TieRank: last.TieRank, ResourceID: last.ResourceID}
	}
}

// seedRow is one staged seed as the star pass reads it.
type seedRow struct {
	ResourceID uint
	KeyPrefix  []byte
	TieRank    int64
}

// seedCursor is the keyset position in the seed order: (key_prefix, tie_rank,
// resource_id), the columns of idx_rseed_rank in order.
type seedCursor struct {
	KeyPrefix  []byte
	TieRank    int64
	ResourceID uint
}

// seedPage reads the next computeChunk seeds after the cursor, in Winner Rule order,
// straight off idx_rseed_rank: no sort sits between the index and the LIMIT, however
// many seeds share a key prefix.
func (run *reductionStream) seedPage(after *seedCursor) ([]seedRow, error) {
	sql := `SELECT resource_id, key_prefix, tie_rank FROM reduction_seeds WHERE reduction_id = ? AND generation = ?`
	args := []any{run.r.ID, run.generation}
	if after != nil {
		sql += ` AND (key_prefix, tie_rank, resource_id) > ( ?, ?, ?)`
		args = append(args, after.KeyPrefix, after.TieRank, after.ResourceID)
	}
	var seeds []seedRow
	err := run.s.db.Raw(sql+` ORDER BY key_prefix, tie_rank, resource_id LIMIT ?`, append(args, computeChunk)...).Scan(&seeds).Error
	return seeds, err
}

// rankTiedSeeds gives the seeds that share a key prefix (two keys equal in their
// first 512 bytes, names alike up to there) a tie_rank: their position in sort_key
// order among those that share it. A seed with a prefix of its own keeps rank 0.
// The star pass then walks (key_prefix, tie_rank, resource_id), which the index
// serves, where ordering ties by sort_key would sort the whole tied group again for
// every page.
//
// Bound: ties are found while the seeds are walked by keyset in chunks, comparing
// each seed's key_prefix with the one before it (across a page boundary too), so no
// discovery statement reads more than one chunk. A tied group is ranked once the
// walk has passed its last seed, so the ranking never moves a row ahead of the
// cursor. The ranking statement ranks its group in the database (one window sort)
// and writes one row per seed of that group, so a group of G seeds is one statement
// of G rows, the one place a statement exceeds a chunk. A group needs G Resources
// whose Winner Rule keys agree on 512 bytes (identical criteria and a name that long
// in common), which is unusual, and G is at most the Extent.
func (run *reductionStream) rankTiedSeeds() error {
	var (
		cursor *seedCursor
		prev   []byte // the key_prefix of the seed before
		tied   []byte // the open group's prefix, once two seeds in a row shared it
		isTied bool
	)
	flush := func() error {
		if !isTied {
			return nil
		}
		g := tied
		isTied, tied = false, nil
		return run.s.fenced(run.r.ID, run.generation, func(tx *gorm.DB) error {
			return tx.Exec(`UPDATE reduction_seeds SET tie_rank = ranked.rn
				FROM (SELECT resource_id, ROW_NUMBER() OVER (ORDER BY sort_key, resource_id) AS rn FROM reduction_seeds
					WHERE reduction_id = ? AND generation = ? AND key_prefix = ?) ranked
				WHERE reduction_seeds.reduction_id = ? AND reduction_seeds.generation = ? AND reduction_seeds.resource_id = ranked.resource_id`,
				run.r.ID, run.generation, g, run.r.ID, run.generation).Error
		})
	}
	for {
		if err := run.jobCtx.Err(); err != nil {
			return err
		}
		seeds, err := run.seedPage(cursor)
		if err != nil {
			return err
		}
		if len(seeds) == 0 {
			return flush()
		}
		for _, seed := range seeds {
			if prev != nil && bytes.Equal(prev, seed.KeyPrefix) {
				isTied, tied = true, seed.KeyPrefix
			} else if err := flush(); err != nil {
				return err
			}
			prev = seed.KeyPrefix
		}
		last := seeds[len(seeds)-1]
		cursor = &seedCursor{KeyPrefix: last.KeyPrefix, TieRank: last.TieRank, ResourceID: last.ResourceID}
	}
}

// stageSeeds inserts, for each staged chunk of Resources that carry a perceptual
// hash, appear in a stored pair and are available, the Winner Rule's byte key. It
// returns how many seeds were staged.
func (run *reductionStream) stageSeeds() (int64, error) {
	if run.s.scope.mode == scopeDeny {
		return 0, nil
	}
	var total int64
	after := uint(0)
	for {
		if err := run.jobCtx.Err(); err != nil {
			return total, err
		}
		var hi uint
		if err := run.s.db.Raw(`SELECT COALESCE(MAX(resource_id), 0) FROM
			(SELECT resource_id FROM reduction_extent WHERE reduction_id = ? AND generation = ? AND resource_id > ? ORDER BY resource_id LIMIT ?) c`,
			run.r.ID, run.generation, after, computeChunk).Scan(&hi).Error; err != nil {
			return total, err
		}
		if hi == 0 {
			return total, nil
		}
		prefix, pargs := run.s.with()
		sel, sargs := run.candidateSelect()
		avail, aargs := run.available("r.id")
		args := append(append(pargs, sargs...), run.r.ID, run.generation, after, hi)
		var rows []candidateRow
		if err := run.s.db.Raw(prefix+`SELECT `+sel+` FROM reduction_extent e JOIN resources r ON r.id = e.resource_id
			WHERE e.reduction_id = ? AND e.generation = ? AND e.resource_id > ? AND e.resource_id <= ?`+run.visibleOwner("r")+`
			AND EXISTS (SELECT 1 FROM image_hashes ih WHERE ih.resource_id = r.id)
			AND (EXISTS (SELECT 1 FROM resource_similarities p1 WHERE p1.resource_id1 = r.id)
				OR EXISTS (SELECT 1 FROM resource_similarities p2 WHERE p2.resource_id2 = r.id))
			AND `+avail+` ORDER BY r.id`, append(args, aargs...)...).Scan(&rows).Error; err != nil {
			return total, err
		}
		after = hi
		if len(rows) == 0 {
			continue
		}
		cands := make([]clusterCandidate, len(rows))
		for i, row := range rows {
			cands[i] = row.candidate()
		}
		if err := run.withAssociations(cands); err != nil {
			return total, err
		}
		seeds := make([]models.ReductionSeed, len(rows))
		for i, row := range rows {
			key := winnerSortKey(run.rule, winnerKeyCandidate{WinnerCandidate: cands[i].WinnerCandidate, HasDescription: row.HasDescription})
			seeds[i] = models.ReductionSeed{ReductionID: run.r.ID, Generation: run.generation, ResourceID: row.ID, KeyPrefix: winnerKeyPrefix(key), SortKey: key}
		}
		err := run.s.fenced(run.r.ID, run.generation, func(tx *gorm.DB) error {
			return tx.Clauses(clause.OnConflict{DoNothing: true}).CreateInBatches(seeds, reductionRowChunk).Error
		})
		if err != nil {
			return total, err
		}
		total += int64(len(seeds))
	}
}

// neighbour is one stored pair as the star reads it.
type neighbour struct {
	ResourceID uint
	Distance   int
	InExtent   bool
}

// pairStream reads one direction of a seed's stored pairs in (distance, id) order on
// the expression index, a page at a time, continuing by keyset.
type pairStream struct {
	run         *reductionStream
	seed        uint
	self, other string
	buf         []neighbour
	pos         int
	lastDist    int
	lastID      uint
	done        bool
}

func (p *pairStream) head(insidersOnly bool) (*neighbour, error) {
	for p.pos >= len(p.buf) {
		if p.done {
			return nil, nil
		}
		rows, err := p.fetch(insidersOnly)
		if err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			p.done = true
			return nil, nil
		}
		p.buf, p.pos = rows, 0
		last := rows[len(rows)-1]
		p.lastDist, p.lastID = last.Distance, last.ResourceID
	}
	return &p.buf[p.pos], nil
}

func (p *pairStream) fetch(insidersOnly bool) ([]neighbour, error) {
	run := p.run
	dist := mrql.SimilarPairDistance("s")
	within := mrql.SimilarPairPredicate("s", run.threshold, run.aThreshold)
	prefix, pargs := run.s.with()
	avail, aargs := run.available("s." + p.other)
	inExtent := inStagedExtent("s." + p.other)
	sql := prefix + `SELECT s.` + p.other + ` AS resource_id, ` + dist + ` AS distance, ` + inExtent + ` AS in_extent
		FROM resource_similarities s
		WHERE s.` + p.self + ` = ? AND s.` + p.other + ` <> ? AND ` + within + `
		AND (` + dist + ` > ? OR (` + dist + ` = ? AND s.` + p.other + ` > ?))
		AND EXISTS (SELECT 1 FROM image_hashes ih WHERE ih.resource_id = s.` + p.other + `)
		AND EXISTS (SELECT 1 FROM resources r WHERE r.id = s.` + p.other + run.visibleOwner("r") + `)
		AND ` + avail
	args := append(append(pargs, run.r.ID, run.generation, p.seed, p.seed, p.lastDist, p.lastDist, p.lastID), aargs...)
	if insidersOnly {
		sql += ` AND ` + inExtent
		args = append(args, run.r.ID, run.generation)
	}
	sql += ` ORDER BY ` + dist + `, s.` + p.other + ` LIMIT ?`
	var rows []neighbour
	if err := run.s.db.Raw(sql, append(args, neighbourPageSize())...).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("reading similarity pairs for resource %d: %w", p.seed, err)
	}
	return rows, nil
}

// starNeighbours merges both directions of a seed's pairs, nearest first, keeping
// at most the member cap of insiders (a Resource inside the Extent) and of
// outsiders. A neighbour reached twice keeps the closer distance: the first arrival
// is the closer, and only kept ids are remembered, so the set stays bounded.
func (run *reductionStream) starNeighbours(seed uint) (insiders, outsiders []neighbour, err error) {
	streams := []*pairStream{
		{run: run, seed: seed, self: "resource_id1", other: "resource_id2", lastDist: -1},
		{run: run, seed: seed, self: "resource_id2", other: "resource_id1", lastDist: -1},
	}
	kept := map[uint]bool{}
	for len(insiders) < models.MaxClusterMembers {
		insidersOnly := run.r.ExcludeExternalResources || len(outsiders) >= models.MaxClusterMembers
		var best *pairStream
		var bestHead *neighbour
		for _, st := range streams {
			head, herr := st.head(insidersOnly)
			if herr != nil {
				return nil, nil, herr
			}
			if head == nil {
				continue
			}
			if bestHead == nil || head.Distance < bestHead.Distance || (head.Distance == bestHead.Distance && head.ResourceID < bestHead.ResourceID) {
				best, bestHead = st, head
			}
		}
		if best == nil {
			break
		}
		n := *bestHead
		best.pos++
		if kept[n.ResourceID] {
			continue
		}
		switch {
		case n.InExtent:
			kept[n.ResourceID] = true
			insiders = append(insiders, n)
		case !insidersOnly:
			kept[n.ResourceID] = true
			outsiders = append(outsiders, n)
		}
	}
	return insiders, outsiders, nil
}

// buildStar builds the Cluster one seed claims.
func (run *reductionStream) buildStar(seed uint) (*pendingCluster, error) {
	ok, err := run.isAvailable(seed)
	if err != nil || !ok {
		return nil, err
	}
	insiders, outsiders, err := run.starNeighbours(seed)
	if err != nil {
		return nil, err
	}
	if len(insiders) == 0 && len(outsiders) == 0 {
		return nil, nil
	}

	// A star whose insider query reached its cap is full: whether or not anything
	// was left out, its seed is its Winner for good and it admits no outsider.
	insiderCap := models.MaxClusterMembers - 1
	full := len(insiders) >= insiderCap
	truncated := 0
	if len(insiders) > insiderCap {
		truncated = 1
		insiders = insiders[:insiderCap]
	}
	distances := make(map[uint]uint8, len(insiders)+1)
	ids := []uint{seed}
	for _, n := range insiders {
		distances[n.ResourceID] = uint8(n.Distance)
		ids = append(ids, n.ResourceID)
	}

	// Among the nearest outsiders the Winner Rule picks one. It joins only a star
	// that is not full, and never displaces an insider.
	if !full && len(outsiders) > 0 {
		outIDs := make([]uint, len(outsiders))
		for i, n := range outsiders {
			outIDs[i] = n.ResourceID
			distances[n.ResourceID] = uint8(n.Distance)
		}
		cands, err := run.loadCandidates(outIDs)
		if err != nil {
			return nil, err
		}
		if len(cands) > 0 {
			best := cands[0]
			for _, c := range cands[1:] {
				if models.CompareByWinnerRule(run.rule, c.WinnerCandidate, best.WinnerCandidate) < 0 {
					best = c
				}
			}
			ids = append(ids, best.Resource.ID)
		}
	}

	members, err := run.loadCandidates(ids)
	if err != nil {
		return nil, err
	}
	seedFound := false
	for i := range members {
		if members[i].Resource.ID == seed {
			seedFound = true
			continue
		}
		d := distances[members[i].Resource.ID]
		members[i].Distance = &d
	}
	if !seedFound {
		return nil, nil
	}
	fixed := uint(0)
	if full {
		fixed = seed
	}
	cluster := buildCluster(models.ReductionTierNear, members, run.rule, fixed)
	if cluster == nil {
		return nil, nil
	}
	stored := storedStreamCluster(cluster, truncated, seed, full)

	// The out-of-Extent rule can move the Winner off the seed, and ADR 0002's
	// pair-justification holds only while the Winner is the seed: re-check every
	// Loser against the new Winner by its named pair, the way promote does.
	if cluster.WinnerID != seed {
		if err := rejustifyStored(run.s, stored, run.threshold, run.aThreshold); err != nil {
			return nil, err
		}
		losers := 0
		for _, m := range stored.Members {
			if !m.Ejected && m.ResourceID != stored.Row.WinnerID {
				losers++
			}
		}
		if losers == 0 {
			return nil, nil
		}
		if err := run.s.refreshLossy(stored, run.rule); err != nil {
			return nil, err
		}
	}
	return &pendingCluster{stored: stored}, nil
}
