package application_context

import (
	"context"
	"testing"

	"gorm.io/gorm"
)

// The scope callback binds the caller's whole subtree next to each chunk, so
// a large subtree must shrink the chunk to stay under the bind limit
// (review round 8).
func TestScopedChunkLeavesRoomForTheAllowList(t *testing.T) {
	txFor := func(allowed int) *gorm.DB {
		ids := make([]uint, allowed)
		ctx := context.WithValue(context.Background(), scopeCtxKey{}, &scopeFilter{allowed: ids})
		return &gorm.DB{Statement: &gorm.Statement{Context: ctx}}
	}
	if got := scopedChunk(&gorm.DB{Statement: &gorm.Statement{Context: context.Background()}}); got != linkChunk {
		t.Errorf("unscoped chunk = %d, want %d", got, linkChunk)
	}
	if got := scopedChunk(txFor(10)); got != linkChunk {
		t.Errorf("small subtree chunk = %d, want %d", got, linkChunk)
	}
	if got := scopedChunk(txFor(31_900)); got != bindBudget-31_900 {
		t.Errorf("large subtree chunk = %d, want %d", got, bindBudget-31_900)
	}
	if got := scopedChunk(txFor(40_000)); got != 1 {
		t.Errorf("oversized subtree chunk = %d, want 1", got)
	}
}
