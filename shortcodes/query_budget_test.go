package shortcodes

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBudgetedExecutorNoBudgetPassesThrough(t *testing.T) {
	calls := 0
	base := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		calls++
		return &QueryResult{Mode: "flat"}, nil
	}
	exec := BudgetedExecutor(base, nil)

	// No budget on the context: every call reaches base, none are cached.
	for i := 0; i < 3; i++ {
		r, err := exec(context.Background(), "resources", QueryOptions{})
		assert.NoError(t, err)
		assert.NotNil(t, r)
	}
	assert.Equal(t, 3, calls)
}

func TestBudgetedExecutorLimitAndCache(t *testing.T) {
	calls := 0
	base := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		calls++
		return &QueryResult{Mode: "flat"}, nil
	}
	var exceededAt []int
	exec := BudgetedExecutor(base, func(limit int) { exceededAt = append(exceededAt, limit) })

	ctx := WithQueryBudget(context.Background(), 2)

	// Two distinct queries: both execute (misses), budget now spent.
	_, err := exec(ctx, "resources", QueryOptions{})
	assert.NoError(t, err)
	_, err = exec(ctx, "notes", QueryOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 2, calls)

	// Third distinct query exceeds the budget: error, base not called.
	_, err = exec(ctx, "groups", QueryOptions{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "inline query budget exceeded (2 per page)")
	assert.Equal(t, 2, calls)

	// A repeat of an earlier query is a cache hit: free, base not called again.
	r, err := exec(ctx, "resources", QueryOptions{})
	assert.NoError(t, err)
	assert.NotNil(t, r)
	assert.Equal(t, 2, calls)

	// onExceeded fired exactly once per page render despite the miss.
	assert.Equal(t, []int{2}, exceededAt)
}

func TestBudgetedExecutorZeroLimitDisablesButStillCaches(t *testing.T) {
	calls := 0
	base := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		calls++
		return &QueryResult{Mode: "flat"}, nil
	}
	exec := BudgetedExecutor(base, nil)
	ctx := WithQueryBudget(context.Background(), 0)

	// Limit 0 = unbounded, but identical queries still dedupe via the cache.
	for i := 0; i < 5; i++ {
		_, err := exec(ctx, "resources", QueryOptions{})
		assert.NoError(t, err)
	}
	assert.Equal(t, 1, calls)

	// A distinct query is a separate miss but never budget-limited.
	_, err := exec(ctx, "notes", QueryOptions{})
	assert.NoError(t, err)
	assert.Equal(t, 2, calls)
}

func TestBudgetErroringQueryNotCached(t *testing.T) {
	calls := 0
	base := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		calls++
		return nil, fmt.Errorf("boom")
	}
	exec := BudgetedExecutor(base, nil)
	ctx := WithQueryBudget(context.Background(), 5)

	_, err := exec(ctx, "resources", QueryOptions{})
	assert.Error(t, err)
	// A failed query is not cached, so a repeat re-executes (and re-charges).
	_, err = exec(ctx, "resources", QueryOptions{})
	assert.Error(t, err)
	assert.Equal(t, 2, calls)
}

func TestCloneQueryResultIsolatesItemMutation(t *testing.T) {
	base := func(ctx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		return &QueryResult{
			Mode:  "flat",
			Items: []QueryResultItem{{EntityType: "resource", EntityID: 1, CustomMRQLResult: "orig"}},
		}, nil
	}
	exec := BudgetedExecutor(base, nil)
	ctx := WithQueryBudget(context.Background(), 5)

	// First call executes and caches a copy; mutate the returned result.
	r1, _ := exec(ctx, "resources", QueryOptions{})
	r1.Items[0].CustomMRQLResult = "mutated"

	// Second call is a cache hit and must be unaffected by r1's mutation.
	r2, _ := exec(ctx, "resources", QueryOptions{})
	assert.Equal(t, "orig", r2.Items[0].CustomMRQLResult)
}

func TestBudgetCacheKeyDistinguishesParamsAndScope(t *testing.T) {
	base := WithQueryBudget(context.Background(), 10)
	b := QueryBudgetFrom(base)

	b.Store("q", QueryOptions{ScopeGroupID: 1}, &QueryResult{Mode: "a"})
	b.Store("q", QueryOptions{ScopeGroupID: 2}, &QueryResult{Mode: "b"})

	r1, ok1 := b.Lookup("q", QueryOptions{ScopeGroupID: 1})
	r2, ok2 := b.Lookup("q", QueryOptions{ScopeGroupID: 2})
	assert.True(t, ok1)
	assert.True(t, ok2)
	assert.Equal(t, "a", r1.Mode)
	assert.Equal(t, "b", r2.Mode)

	_, ok3 := b.Lookup("q", QueryOptions{ScopeGroupID: 3})
	assert.False(t, ok3)
}
