package application_context

import (
	"math"
	"sort"

	"gorm.io/gorm"
	"mahresources/contracts"
)

// Scores are evidence-sensitive ranking heuristics, not probabilities. Missing
// sources contribute zero; their weights are never redistributed.
const (
	suggestedTagsDefaultLimit      = 8
	suggestedTagsMaxSimilar        = 50
	suggestedTagsSourceLimit       = 20
	suggestedTagWeightSimilar      = 0.5
	suggestedTagWeightCooccurrence = 0.3
	suggestedTagWeightGroup        = 0.2
	suggestedTagSimilarityPrior    = 2.0
	suggestedTagPopulationPrior    = 5.0
	suggestedTagLocalSupport       = 5
)

type suggestAccumulator struct {
	name       string
	simWeight  float64
	coCount    float64
	groupCount float64
}

// suggestionResources explicitly scopes the population: Scan and SQL subqueries
// do not run the ORM Query callbacks. Membership counts each resource only once,
// including when it shares several seed tags. Empty seeds select tagged resources.
func (ctx *MahresourcesContext) suggestionResources(target uint, owner *uint, seeds []uint) *gorm.DB {
	db := ctx.db.Table("resources").Where("resources.id <> ?", target)
	if sf := scopeFromContext(ctx.db.Statement.Context); sf != nil {
		if len(sf.allowed) == 0 {
			db = db.Where("1 = 0")
		} else {
			db = db.Where("resources.owner_id IN ?", sf.allowed)
		}
	}
	if owner != nil {
		db = db.Where("resources.owner_id = ?", *owner)
	}
	if len(seeds) > 0 {
		// Starting with the tag index avoids a correlated probe of every resource
		// when a small seed set matches only a fraction of a large library.
		db = db.Where("resources.id IN (SELECT resource_id FROM resource_tags WHERE tag_id IN ?)", seeds)
	} else {
		db = db.Where("EXISTS (SELECT 1 FROM resource_tags WHERE resource_id = resources.id)")
	}
	return db
}

type suggestionPopulation struct {
	owner *uint
	seeds []uint
	total int64
}

func (ctx *MahresourcesContext) suggestionPopulation(target uint, owner *uint, seeds []uint) (*suggestionPopulation, error) {
	pop := &suggestionPopulation{owner: owner, seeds: seeds}
	err := ctx.suggestionResources(target, owner, seeds).Count(&pop.total).Error
	return pop, err
}

// suggestionTagCounts either selects the top eligible candidates, or fills in
// counts for the entire candidate union. Both paths aggregate in SQL, never per tag.
func (ctx *MahresourcesContext) suggestionTagCounts(target uint, pop *suggestionPopulation, excluded, candidates []uint) ([]PopularTag, error) {
	var counts []PopularTag
	// Aggregate before joining names: joining tags inside the aggregation lets
	// SQLite cross every tag with every matching resource (millions of probes
	// for common seeds). The grouped subquery bounds name lookups to actual tags.
	aggregate := ctx.suggestionResources(target, pop.owner, pop.seeds).
		Joins("JOIN resource_tags st ON st.resource_id = resources.id").
		Select("st.tag_id AS id, COUNT(*) AS count").
		Group("st.tag_id")
	if len(excluded) > 0 {
		aggregate = aggregate.Where("st.tag_id NOT IN ?", excluded)
	}
	if candidates != nil {
		aggregate = aggregate.Where("st.tag_id IN ?", candidates)
	}
	db := ctx.db.Table("(?) AS counts", aggregate).
		Joins("JOIN tags t ON t.id = counts.id").
		Select("counts.id AS id, t.name AS name, counts.count AS count")
	if candidates == nil {
		db = db.Order("counts.count DESC, t.name ASC, t.id ASC").Limit(suggestedTagsSourceLimit)
	}
	return counts, db.Scan(&counts).Error
}

// GetSuggestedTags combines distance-weighted neighbors, tag co-occurrence, and
// owner-group popularity. Every denominator includes the full evidence population,
// independent of candidate limits and already-applied-tag exclusion.
func (ctx *MahresourcesContext) GetSuggestedTags(resourceId uint, limit int) ([]contracts.SuggestedTag, error) {
	if limit <= 0 {
		limit = suggestedTagsDefaultLimit
	}
	res, err := ctx.GetResource(resourceId)
	if err != nil {
		return nil, err
	}
	excluded := make(map[uint]bool, len(res.Tags))
	seedIDs := make([]uint, 0, len(res.Tags))
	for _, t := range res.Tags {
		if t != nil && !excluded[t.ID] {
			excluded[t.ID] = true
			seedIDs = append(seedIDs, t.ID)
		}
	}
	sort.Slice(seedIDs, func(i, j int) bool { return seedIDs[i] < seedIDs[j] })
	acc := make(map[uint]*suggestAccumulator)
	ensure := func(id uint, name string) *suggestAccumulator {
		if acc[id] == nil {
			acc[id] = &suggestAccumulator{name: name}
		}
		return acc[id]
	}

	var neighborWeight float64
	if sims, err := ctx.getSimilarResourcesLimited(resourceId, suggestedTagsMaxSimilar); err == nil {
		for _, sr := range sims {
			if sr == nil || sr.ID == resourceId {
				continue
			}
			seen := make(map[uint]bool)
			weight := 0.25 // exact dHash fallback has no comparable distance
			if sr.SimilarityDistance != nil {
				weight = math.Exp2(-float64(*sr.SimilarityDistance) / 3)
			}
			for _, tag := range sr.Tags {
				if tag == nil || seen[tag.ID] {
					continue
				}
				seen[tag.ID] = true
				if !excluded[tag.ID] {
					ensure(tag.ID, tag.Name).simWeight += weight
				}
			}
			if len(seen) > 0 {
				neighborWeight += weight
			}
		}
	}

	var group, co *suggestionPopulation
	if res.OwnerId != nil {
		if pop, err := ctx.suggestionPopulation(resourceId, res.OwnerId, nil); err == nil {
			group = pop
		}
	}
	if len(seedIDs) > 0 {
		pop, err := ctx.suggestionPopulation(resourceId, res.OwnerId, seedIDs)
		// An error must not widen scope: fallback requires a successful local count.
		if err == nil && res.OwnerId != nil && pop.total < suggestedTagLocalSupport {
			pop, err = ctx.suggestionPopulation(resourceId, nil, seedIDs)
		}
		if err == nil {
			co = pop
		}
	}

	// Keep candidates provisional until the source's full count query succeeds.
	// A failed source must not leave zero-evidence candidates in the result.
	candidates := make(map[uint]struct{}, len(acc))
	for id := range acc {
		candidates[id] = struct{}{}
	}
	for _, source := range []**suggestionPopulation{&group, &co} {
		if *source == nil {
			continue
		}
		counts, err := ctx.suggestionTagCounts(resourceId, *source, seedIDs, nil)
		if err != nil {
			*source = nil
			continue
		}
		for _, c := range counts {
			candidates[c.Id] = struct{}{}
		}
	}
	candidateIDs := make([]uint, 0, len(candidates))
	for id := range candidates {
		candidateIDs = append(candidateIDs, id)
	}
	sort.Slice(candidateIDs, func(i, j int) bool { return candidateIDs[i] < candidateIDs[j] })
	if len(candidateIDs) > 0 {
		for _, source := range []struct {
			pop  *suggestionPopulation
			isCo bool
		}{{group, false}, {co, true}} {
			if source.pop == nil {
				continue
			}
			counts, err := ctx.suggestionTagCounts(resourceId, source.pop, seedIDs, candidateIDs)
			if err != nil {
				continue
			}
			for _, c := range counts {
				a := ensure(c.Id, c.Name)
				if source.isCo {
					a.coCount = float64(c.Count)
				} else {
					a.groupCount = float64(c.Count)
				}
			}
		}
	}

	suggestions := make([]contracts.SuggestedTag, 0, len(acc))
	for id, a := range acc {
		score := suggestedTagWeightSimilar * a.simWeight / (neighborWeight + suggestedTagSimilarityPrior)
		sources := make([]string, 0, 3)
		if a.simWeight > 0 {
			sources = append(sources, "similar")
		}
		if a.coCount > 0 {
			score += suggestedTagWeightCooccurrence * a.coCount / (float64(co.total) + suggestedTagPopulationPrior)
			sources = append(sources, "cooccurrence")
		}
		if a.groupCount > 0 {
			score += suggestedTagWeightGroup * a.groupCount / (float64(group.total) + suggestedTagPopulationPrior)
			sources = append(sources, "group")
		}
		suggestions = append(suggestions, contracts.SuggestedTag{ID: id, Name: a.name, Score: score, Sources: sources})
	}
	sort.Slice(suggestions, func(i, j int) bool {
		if suggestions[i].Score != suggestions[j].Score {
			return suggestions[i].Score > suggestions[j].Score
		}
		if suggestions[i].Name != suggestions[j].Name {
			return suggestions[i].Name < suggestions[j].Name
		}
		return suggestions[i].ID < suggestions[j].ID
	})
	if len(suggestions) > limit {
		suggestions = suggestions[:limit]
	}
	return suggestions, nil
}
