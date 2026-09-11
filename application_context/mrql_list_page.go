package application_context

import (
	"context"
	"mahresources/models"
	"mahresources/mrql"
)

type mrqlCardPageKey struct{}
type mrqlCardPage struct{ size, itemOffset int }

// WithMRQLCardPage bounds materialized bucket cards, with an item continuation
// inside the first bucket. It leaves the authored per-bucket LIMIT unchanged.
func WithMRQLCardPage(parent context.Context, size, itemOffset int) context.Context {
	return context.WithValue(parent, mrqlCardPageKey{}, mrqlCardPage{min(100, max(1, size)), max(0, itemOffset)})
}

// ExecuteMRQLListPage pages within the authored bounds. Single-entity pages push
// LIMIT/OFFSET into SQL; mixed pages merge only identities and sort fields using
// the same ordering as ordinary execution. Only the display page is hydrated.
// Random samples use the separate snapshot path.
func (ctx *MahresourcesContext) ExecuteMRQLListPage(parent context.Context, query *mrql.Query, page, size int) (*MRQLResult, int, int, error) {
	parsed := *query
	defaultApplied := parsed.Limit < 0
	if defaultApplied {
		parsed.Limit = min(ctx.defaultMRQLLimit(), MaxMRQLInteractiveLimit)
	}
	if err := validateMRQLExecutionBounds(&parsed, interactiveMRQLPolicy); err != nil {
		return nil, 0, 0, err
	}
	appliedLimit := parsed.Limit
	opts, deny, err := ctx.mrqlQueryTranslateOptions(&parsed)
	if err != nil {
		return nil, 0, 0, err
	}
	entityType := mrql.ExtractEntityType(&parsed)
	result := &MRQLResult{EntityType: entityType.String(), DefaultLimitApplied: defaultApplied, AppliedLimit: appliedLimit}
	if deny {
		return result, 0, 1, nil
	}
	reqCtx, cancel := context.WithTimeout(parent, ctx.mrqlQueryTimeout())
	defer cancel()
	base := max(0, parsed.Offset)
	var total int
	if entityType == mrql.EntityUnspecified {
		// Keep the original branch bounds and Unicode-aware merge: replacing
		// them with database collation changes which rows LIMIT selects and
		// would make cards disagree with query-wide Mass Edit and exports.
		result, err = ctx.executeCrossEntity(reqCtx, &parsed, opts, interactiveMRQLPolicy, true)
		if err != nil {
			return nil, 0, 0, err
		}
		total = len(result.Order)
	} else {
		matched, err := ctx.countCrossEntity(reqCtx, &parsed, opts, entityType)
		if err != nil {
			return nil, 0, 0, err
		}
		total = int(min(int64(appliedLimit), max(0, matched-int64(base))))
	}
	size = min(100, max(1, size))
	page = min(max(1, page), max(1, (total+size-1)/size))
	start, end := (page-1)*size, min(page*size, total)
	if entityType == mrql.EntityUnspecified {
		result.Order = result.Order[start:end]
		result.Resources, result.Notes, result.Groups = nil, nil, nil
		for _, identity := range result.Order {
			switch identity.EntityType {
			case "resource":
				result.Resources = append(result.Resources, models.Resource{ID: identity.ID})
			case "note":
				result.Notes = append(result.Notes, models.Note{ID: identity.ID})
			case "group":
				result.Groups = append(result.Groups, models.Group{ID: identity.ID})
			}
		}
	} else if total > 0 {
		parsed.Offset, parsed.Limit = base+start, end-start
		// The authored offset was validated above. Internal pages may advance
		// within its already-authorized LIMIT beyond the public offset ceiling.
		pagePolicy := interactiveMRQLPolicy
		pagePolicy.maxOffset += pagePolicy.maxLimit
		result, err = ctx.executeMRQLParsed(reqCtx, &parsed, 0, 0, pagePolicy)
		if err != nil {
			return nil, 0, 0, err
		}
	}
	result.DefaultLimitApplied, result.AppliedLimit = defaultApplied, appliedLimit
	return result, total, page, nil
}
