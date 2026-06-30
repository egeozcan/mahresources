package application_context

import (
	"sort"

	"mahresources/models/query_models"
	"mahresources/server/interfaces"
)

// Tunable ranking parameters for GetSuggestedTags. Kept as named constants so
// the heuristic can be adjusted in one place; not exposed as flags for v1.
const (
	// suggestedTagsDefaultLimit is the number of suggestions returned when the
	// caller passes a non-positive limit. The chip row renders 5-8.
	suggestedTagsDefaultLimit = 8
	// suggestedTagsMaxSimilar caps how many perceptual-hash-similar resources we
	// aggregate tags from, bounding work for high-fan-out resources.
	suggestedTagsMaxSimilar = 50
	// Weights blending the two sources. Similar-resource tags are more
	// contextually specific than group-popular tags, so they weigh more.
	suggestedTagWeightSimilar = 0.6
	suggestedTagWeightGroup   = 0.4
)

// suggestAccumulator tracks per-tag scoring across the two sources.
type suggestAccumulator struct {
	name       string
	simFreq    float64
	groupCount float64
	fromSim    bool
	fromGroup  bool
}

// GetSuggestedTags ranks tag suggestions for a resource by unioning two
// already-computed signals: tags on perceptual-hash-similar resources, and the
// most-used tags in the resource's owner group. Tags already on the resource
// are excluded. Results are ordered by blended score descending (tiebreak by
// name ascending) and capped to limit.
//
// Access control: GetResource runs through the (possibly scoped) db, so an
// out-of-subtree or missing id returns the underlying record-not-found error —
// this is the primary guard. The two source queries are likewise scoped, so a
// confined principal can only ever receive suggestions derived from resources
// inside its subtree.
func (ctx *MahresourcesContext) GetSuggestedTags(resourceId uint, limit int) ([]interfaces.SuggestedTag, error) {
	if limit <= 0 {
		limit = suggestedTagsDefaultLimit
	}

	res, err := ctx.GetResource(resourceId)
	if err != nil {
		return nil, err
	}

	// Exclude tags already on the resource.
	excluded := make(map[uint]struct{}, len(res.Tags))
	for _, t := range res.Tags {
		excluded[t.ID] = struct{}{}
	}

	acc := make(map[uint]*suggestAccumulator)
	ensure := func(id uint, name string) *suggestAccumulator {
		a := acc[id]
		if a == nil {
			a = &suggestAccumulator{name: name}
			acc[id] = a
		} else if a.name == "" {
			a.name = name
		}
		return a
	}

	// Source A: tags on perceptual-hash-similar resources. Degrade gracefully —
	// a missing similarity table or unprocessed resource simply yields nothing.
	var maxSimFreq float64
	if sims, simErr := ctx.GetSimilarResources(resourceId); simErr == nil {
		if len(sims) > suggestedTagsMaxSimilar {
			sims = sims[:suggestedTagsMaxSimilar]
		}
		for _, sr := range sims {
			if sr == nil {
				continue
			}
			for _, t := range sr.Tags {
				if t == nil {
					continue
				}
				if _, skip := excluded[t.ID]; skip {
					continue
				}
				a := ensure(t.ID, t.Name)
				a.simFreq++
				a.fromSim = true
				if a.simFreq > maxSimFreq {
					maxSimFreq = a.simFreq
				}
			}
		}
	}

	// Source B: most-used tags in the owner group.
	var maxGroupCount float64
	if res.OwnerId != nil {
		if pop, popErr := ctx.GetPopularResourceTags(&query_models.ResourceSearchQuery{OwnerId: *res.OwnerId}); popErr == nil {
			for _, p := range pop {
				if _, skip := excluded[p.Id]; skip {
					continue
				}
				a := ensure(p.Id, p.Name)
				a.groupCount = float64(p.Count)
				a.fromGroup = true
				if a.groupCount > maxGroupCount {
					maxGroupCount = a.groupCount
				}
			}
		}
	}

	suggestions := make([]interfaces.SuggestedTag, 0, len(acc))
	for id, a := range acc {
		var score float64
		if maxSimFreq > 0 {
			score += suggestedTagWeightSimilar * (a.simFreq / maxSimFreq)
		}
		if maxGroupCount > 0 {
			score += suggestedTagWeightGroup * (a.groupCount / maxGroupCount)
		}

		sources := make([]string, 0, 2)
		if a.fromSim {
			sources = append(sources, "similar")
		}
		if a.fromGroup {
			sources = append(sources, "group")
		}

		suggestions = append(suggestions, interfaces.SuggestedTag{
			ID:      id,
			Name:    a.name,
			Score:   score,
			Sources: sources,
		})
	}

	// Highest score first; deterministic tiebreak by name then id.
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
