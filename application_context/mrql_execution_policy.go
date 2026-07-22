package application_context

import (
	"fmt"
	"math"

	"mahresources/mrql"
)

const (
	MaxMRQLInteractiveLimit  = 10_000
	MaxMRQLInteractiveOffset = 10_000
	MaxMRQLExportLimit       = 10_000
	MaxMRQLExportOffset      = 10_000
)

// MRQLExecutionLimitError is a client-visible pagination safety error.
type MRQLExecutionLimitError struct {
	Field string
	Value int
	Max   int
}

func (e *MRQLExecutionLimitError) Error() string {
	return fmt.Sprintf("MRQL %s %d exceeds maximum %d", e.Field, e.Value, e.Max)
}

type mrqlExecutionPolicy struct {
	maxLimit  int
	maxOffset int
}

var (
	interactiveMRQLPolicy = mrqlExecutionPolicy{maxLimit: MaxMRQLInteractiveLimit, maxOffset: MaxMRQLInteractiveOffset}
	exportMRQLPolicy      = mrqlExecutionPolicy{maxLimit: MaxMRQLExportLimit, maxOffset: MaxMRQLExportOffset}
)

func validateMRQLExecutionBounds(q *mrql.Query, policy mrqlExecutionPolicy) error {
	if q.Limit > policy.maxLimit {
		return &MRQLExecutionLimitError{Field: "limit", Value: q.Limit, Max: policy.maxLimit}
	}
	if q.Offset > policy.maxOffset {
		return &MRQLExecutionLimitError{Field: "offset", Value: q.Offset, Max: policy.maxOffset}
	}
	if q.BucketLimit > mrql.MaxBuckets {
		return &MRQLExecutionLimitError{Field: "bucket limit", Value: q.BucketLimit, Max: mrql.MaxBuckets}
	}
	return nil
}

// crossEntityWindow returns the global pagination window and the per-entity
// fetch cap used before results are merged and globally paginated in Go.
func crossEntityWindow(q *mrql.Query, defaultLimit int, policy mrqlExecutionPolicy) (globalLimit, globalOffset, perEntityCap int, err error) {
	if err = validateMRQLExecutionBounds(q, policy); err != nil {
		return 0, 0, 0, err
	}
	globalLimit = min(defaultLimit, policy.maxLimit)
	if q.Limit >= 0 {
		globalLimit = q.Limit
	}
	if q.Offset >= 0 {
		globalOffset = q.Offset
	}
	if globalOffset > math.MaxInt-globalLimit {
		return 0, 0, 0, &MRQLExecutionLimitError{Field: "offset + limit", Value: math.MaxInt, Max: policy.maxOffset + policy.maxLimit}
	}
	return globalLimit, globalOffset, globalOffset + globalLimit, nil
}

// ValidateMRQLFlatExportBounds validates export pagination without executing the
// query. It is used by the browser download preflight so limit errors remain
// visible before switching to the native streaming download path.
func (ctx *MahresourcesContext) ValidateMRQLFlatExportBounds(q *mrql.Query, limit, page int) error {
	clone := *q
	if limit > 0 {
		clone.Limit = limit
	}
	if page >= 1 {
		effectiveLimit := clone.Limit
		if effectiveLimit < 0 {
			effectiveLimit = min(ctx.defaultMRQLLimit(), exportMRQLPolicy.maxLimit)
		}
		offset, err := checkedPageOffset(page, effectiveLimit, exportMRQLPolicy.maxOffset)
		if err != nil {
			return err
		}
		clone.Offset = offset
	}
	if clone.Limit < 0 {
		clone.Limit = min(ctx.defaultMRQLLimit(), exportMRQLPolicy.maxLimit)
	}
	return validateMRQLExecutionBounds(&clone, exportMRQLPolicy)
}

// ValidateMRQLGroupedExportBounds validates an already paginated grouped query
// without materializing buckets or aggregate rows.
func (ctx *MahresourcesContext) ValidateMRQLGroupedExportBounds(q *mrql.Query) error {
	clone := *q
	if clone.Limit < 0 {
		clone.Limit = min(ctx.defaultMRQLLimit(), exportMRQLPolicy.maxLimit)
	}
	return validateMRQLExecutionBounds(&clone, exportMRQLPolicy)
}

func checkedPageOffset(page, pageSize, maxOffset int) (int, error) {
	if page < 1 {
		return 0, nil
	}
	multiplier := page - 1
	if pageSize < 0 || multiplier > math.MaxInt/max(1, pageSize) {
		return 0, &MRQLExecutionLimitError{Field: "offset", Value: math.MaxInt, Max: maxOffset}
	}
	offset := multiplier * pageSize
	if offset > maxOffset {
		return 0, &MRQLExecutionLimitError{Field: "offset", Value: offset, Max: maxOffset}
	}
	return offset, nil
}
