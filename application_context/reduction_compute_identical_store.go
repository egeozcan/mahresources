package application_context

import (
	"container/heap"
	"sort"
	"strings"

	"mahresources/models"
)

// candidateHeap keeps the best `max` candidates by the Winner Rule: its root is the
// worst one kept, so a better arrival replaces it in O(log max).
type candidateHeap struct {
	items []clusterCandidate
	rule  []string
	max   int
}

func (h *candidateHeap) Len() int { return len(h.items) }
func (h *candidateHeap) Less(i, j int) bool {
	return models.CompareByWinnerRule(h.rule, h.items[i].WinnerCandidate, h.items[j].WinnerCandidate) > 0
}
func (h *candidateHeap) Swap(i, j int) { h.items[i], h.items[j] = h.items[j], h.items[i] }
func (h *candidateHeap) Push(x any)    { h.items = append(h.items, x.(clusterCandidate)) }
func (h *candidateHeap) Pop() any {
	last := h.items[len(h.items)-1]
	h.items = h.items[:len(h.items)-1]
	return last
}

func (h *candidateHeap) offer(c clusterCandidate) {
	if len(h.items) < h.max {
		heap.Push(h, c)
		return
	}
	if models.CompareByWinnerRule(h.rule, c.WinnerCandidate, h.items[0].WinnerCandidate) < 0 {
		h.items[0] = c
		heap.Fix(h, 0)
	}
}

// sorted is the kept candidates, best first.
func (h *candidateHeap) sorted() []clusterCandidate {
	out := append([]clusterCandidate(nil), h.items...)
	sort.SliceStable(out, func(i, j int) bool {
		return models.CompareByWinnerRule(h.rule, out[i].WinnerCandidate, out[j].WinnerCandidate) < 0
	})
	return out
}

// identicalTier groups the staged Extent by content hash (design D5). The empty
// hash is excluded, and that exclusion is load-bearing: "" is a live value in this
// schema, so grouping on it would collapse every hashless Resource into one
// Cluster arriving checked and proposing to delete files that have nothing to do
// with each other.
//
// The staged hashes are read by rows on (reduction, generation, hash), a chunk at a
// time, each chunk starting past the last hash of the one before. A hash is
// nominated when it repeats inside the chunk or a twin exists anywhere (one batched
// probe, unscoped, only to nominate; the holders are read again through the
// principal's subtree), and each nominated hash then builds its Cluster.
func (run *reductionStream) identicalTier() error {
	last := ""
	var done int64
	for {
		if err := run.jobCtx.Err(); err != nil {
			return err
		}
		var hashes []string
		if err := run.s.db.Raw(`SELECT hash FROM reduction_extent WHERE reduction_id = ? AND generation = ? AND hash > ? AND hash <> '' ORDER BY hash LIMIT ?`,
			run.r.ID, run.generation, last, computeChunk).Scan(&hashes).Error; err != nil {
			return err
		}
		if len(hashes) == 0 {
			return nil
		}
		last = hashes[len(hashes)-1]
		done += int64(len(hashes))

		seen := make(map[string]int, len(hashes))
		var order []string
		for _, h := range hashes {
			if seen[h] == 0 {
				order = append(order, h)
			}
			seen[h]++
		}
		var singles []string
		nominated := map[string]bool{}
		for _, h := range order {
			if seen[h] >= 2 {
				nominated[h] = true
			} else {
				singles = append(singles, h)
			}
		}
		twins, err := run.twinHashes(singles)
		if err != nil {
			return err
		}
		var list []string
		for _, h := range order {
			if nominated[h] || twins[h] {
				list = append(list, h)
			}
		}
		if computeAfterNominateHook != nil {
			computeAfterNominateHook(list)
		}
		for _, h := range list {
			if err := run.insertBuilt(func() (*pendingCluster, error) { return run.buildIdentical(h) }); err != nil {
				return err
			}
		}
		run.report(done, int64(run.extentSize))
	}
}

// twinHashes is which of the values some other pair of Resources also holds: two
// distinct rows with the hash exist, found by index probes that stop at the second.
func (run *reductionStream) twinHashes(values []string) (map[string]bool, error) {
	out := map[string]bool{}
	const batch = 250
	for from := 0; from < len(values); from += batch {
		part := values[from:min(from+batch, len(values))]
		args := make([]any, len(part))
		sel := make([]string, len(part))
		for i, v := range part {
			args[i] = v
			sel[i] = "SELECT ?"
			if i == 0 {
				sel[i] = "SELECT CAST(? AS TEXT) AS hash"
			}
		}
		var found []string
		if err := run.s.db.Raw(`SELECT v.hash FROM (`+strings.Join(sel, " UNION ALL ")+`) v WHERE EXISTS (
				SELECT 1 FROM resources a WHERE a.hash = v.hash AND EXISTS (SELECT 1 FROM resources b WHERE b.hash = v.hash AND b.id > a.id))`, args...).Scan(&found).Error; err != nil {
			return nil, err
		}
		for _, h := range found {
			out[h] = true
		}
	}
	return out, nil
}

// buildIdentical builds the Cluster of one content hash. The holders are streamed by
// id in chunks with the narrow projection, the hash checked again on every loaded
// row (a version upload between the grouping and the load rewrites resources.hash,
// and byte-identity is the one thing this tier may assert), the unavailable ones
// dropped in SQL. Two bounded heaps hold the best 200 holders inside the Extent and
// the single best outsider; the outsider joins only if it beats every insider,
// taking the 200th one's place. truncated counts the insiders left out.
func (run *reductionStream) buildIdentical(hash string) (*pendingCluster, error) {
	insiders := &candidateHeap{rule: run.rule, max: models.MaxClusterMembers}
	var outsider *clusterCandidate
	insiderTotal := 0
	after := uint(0)
	for {
		if err := run.jobCtx.Err(); err != nil {
			return nil, err
		}
		prefix, pargs := run.s.with()
		sel, sargs := run.candidateSelect()
		avail, aargs := run.available("r.id")
		args := append(append(append(pargs, sargs...), hash, after), aargs...)
		var rows []candidateRow
		if err := run.s.db.Raw(prefix+`SELECT `+sel+` FROM resources r WHERE r.hash = ? AND r.id > ?`+run.visibleOwner("r")+` AND `+avail+` ORDER BY r.id LIMIT ?`,
			append(args, computeChunk)...).Scan(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			break
		}
		after = rows[len(rows)-1].ID
		cands := make([]clusterCandidate, 0, len(rows))
		for _, row := range rows {
			if row.Hash != hash {
				continue
			}
			cands = append(cands, row.candidate())
		}
		if err := run.withAssociations(cands); err != nil {
			return nil, err
		}
		for _, c := range cands {
			switch {
			case c.InExtent:
				insiderTotal++
				insiders.offer(c)
			case run.r.ExcludeExternalResources:
			default:
				if outsider == nil || models.CompareByWinnerRule(run.rule, c.WinnerCandidate, outsider.WinnerCandidate) < 0 {
					kept := c
					outsider = &kept
				}
			}
		}
	}
	kept := insiders.sorted()
	if len(kept) == 0 {
		return nil, nil
	}
	candidates := kept
	if outsider != nil && models.CompareByWinnerRule(run.rule, outsider.WinnerCandidate, kept[0].WinnerCandidate) < 0 {
		if len(kept) == models.MaxClusterMembers {
			kept = kept[:len(kept)-1]
		}
		candidates = append(append([]clusterCandidate(nil), kept...), *outsider)
	}
	cluster := buildCluster(models.ReductionTierIdentical, candidates, run.rule, 0)
	if cluster == nil {
		return nil, nil
	}
	inCluster := 0
	for _, m := range cluster.Members {
		if m.InExtent {
			inCluster++
		}
	}
	return &pendingCluster{stored: storedStreamCluster(cluster, insiderTotal-inCluster, 0, false), hash: hash}, nil
}
