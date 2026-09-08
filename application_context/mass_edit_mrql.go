package application_context

import (
	"context"
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"mahresources/mrql"
	"sort"
	"strings"
)

// Resolve the executed MRQL result before selecting its entity-type subset.
// Adding type=resource to a mixed query would change its global LIMIT/OFFSET.
func (ctx *MahresourcesContext) resolveMassEditMRQL(spec massEditSpec, q *query_models.MassEditQuery) ([]uint, int64, error) {
	if q.ExpectedCount == nil && !q.DryRun {
		return nil, 0, fmt.Errorf("an expected count is required for MRQL mass edit")
	}
	parsed, err := mrql.Parse(q.MRQLQuery)
	if err != nil {
		return nil, 0, err
	}
	var params map[string]any
	if q.MRQLParams != "" {
		if err := json.Unmarshal([]byte(q.MRQLParams), &params); err != nil {
			return nil, 0, fmt.Errorf("invalid MRQL parameters: %w", err)
		}
	}
	if err := mrql.BindParams(parsed, params); err != nil {
		return nil, 0, err
	}
	if err := mrql.Validate(parsed); err != nil {
		return nil, 0, err
	}
	reqCtx := context.Background()
	if ctx.currentRequest != nil {
		reqCtx = ctx.currentRequest.Context()
	}
	// Bound the complete target resolution, including every bucket page.
	reqCtx, cancel := context.WithTimeout(reqCtx, ctx.mrqlQueryTimeout())
	defer cancel()
	seen := map[uint]bool{}
	collect := func(items any) {
		switch rows := items.(type) {
		case []models.Resource:
			if spec.entity == "resource" {
				for _, r := range rows {
					seen[r.ID] = true
				}
			}
		case []models.Note:
			if spec.entity == "note" {
				for _, n := range rows {
					seen[n.ID] = true
				}
			}
		case []models.Group:
			if spec.entity == "group" {
				for _, g := range rows {
					seen[g.ID] = true
				}
			}
		}
	}
	if parsed.GroupBy != nil {
		if len(parsed.GroupBy.Aggregates) > 0 {
			return nil, 0, fmt.Errorf("aggregate rows cannot be mass edited")
		}
		parsed.EntityType = mrql.ExtractEntityType(parsed)
		if parsed.EntityType.String() != spec.entity {
			return nil, 0, fmt.Errorf("query does not return %s entities", spec.entity)
		}
		parsed.BucketLimit = mrql.MaxBuckets
		for {
			result, err := ctx.ExecuteMRQLGrouped(reqCtx, parsed)
			if err != nil {
				return nil, 0, err
			}
			// A continuation warning is recoverable by following NextOffset. Any
			// other incomplete result must not become an edit target.
			for _, warning := range result.Warnings {
				continuation := strings.Contains(warning, "bucket queries; continue") || strings.HasPrefix(warning, "Results truncated at ")
				if !continuation || result.NextOffset == nil || *result.NextOffset <= max(0, parsed.Offset) {
					return nil, 0, fmt.Errorf("cannot mass edit incomplete MRQL results: %s", warning)
				}
			}
			for _, bucket := range result.Groups {
				collect(bucket.Items)
			}
			if len(seen) > ctx.MaxMassEditEntities() {
				return nil, 0, ErrMassEditTooLarge
			}
			if result.NextOffset == nil {
				break
			}
			if *result.NextOffset <= parsed.Offset {
				return nil, 0, fmt.Errorf("MRQL pagination did not advance")
			}
			parsed.Offset = *result.NextOffset
		}
	} else {
		var result *MRQLResult
		if q.MRQLSnapshot != "" {
			result, err = ctx.ResolveMRQLSnapshot(reqCtx, q.MRQLQuery, params, q.MRQLSnapshot)
		} else {
			for _, order := range parsed.OrderBy {
				if order.Random {
					return nil, 0, fmt.Errorf("%w: RANDOM() mass edit requires the executed result snapshot", ErrInvalidMRQLSnapshot)
				}
			}
			result, err = ctx.ExecuteMRQLParsed(reqCtx, parsed, 0, 0)
		}
		if err != nil {
			return nil, 0, err
		}
		if len(result.Warnings) > 0 {
			return nil, 0, fmt.Errorf("cannot mass edit incomplete MRQL results: %s", result.Warnings[0])
		}
		collect(result.Resources)
		collect(result.Notes)
		collect(result.Groups)
	}
	if len(seen) > ctx.MaxMassEditEntities() {
		return nil, 0, ErrMassEditTooLarge
	}
	matched := int64(len(seen))
	if !q.DryRun && matched != int64(*q.ExpectedCount) {
		return nil, 0, ErrMassEditSetChanged
	}
	ids := make([]uint, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids, matched, nil
}
