package api_handlers

import (
	"math"
	"testing"

	"mahresources/mrql"
)

func TestApplyGroupedPaginationSaturatesOverflow(t *testing.T) {
	query := &mrql.Query{
		GroupBy:     &mrql.GroupByClause{},
		Limit:       -1,
		Offset:      -1,
		BucketLimit: -1,
	}
	applyGroupedPagination(query, 2, 2, math.MaxInt, 0)
	if query.Offset != math.MaxInt {
		t.Fatalf("bucket offset = %d, want saturated %d", query.Offset, math.MaxInt)
	}
}
