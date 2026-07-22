package shortcodes

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// QueryBudget bounds how many distinct MRQL queries one page render may execute
// via inline [mrql] shortcodes. Custom* templates render once per card, so a
// summary containing an entity-scoped [mrql] runs one query per card — on a
// list page of thousands that is a real load source. The budget is a per-page
// object threaded on the request context (alongside the MRQL result cache and
// partial resolver): identical queries are served from a small result cache
// (free), and each cache *miss* consumes one unit of budget. Once exhausted,
// further misses are refused so the executor can render an error box instead of
// running the query.
//
// A limit of 0 disables the budget entirely (unlimited queries) while still
// deduping via the cache, matching the historical unbounded behaviour.
type QueryBudget struct {
	mu          sync.Mutex
	limit       int
	count       int
	cache       map[string]*QueryResult
	exceeded    bool
	cacheHits   int
	cacheMisses int
	executions  int
}

// QueryBudgetStats is a read-only snapshot for diagnostics and benchmarks.
type QueryBudgetStats struct {
	Limit       int  `json:"limit"`
	Executions  int  `json:"executions"`
	CacheHits   int  `json:"cacheHits"`
	CacheMisses int  `json:"cacheMisses"`
	Exceeded    bool `json:"exceeded"`
}

type queryBudgetKey struct{}

// WithQueryBudget attaches a fresh per-page query budget with the given limit
// (0 disables the cap). Callers build it once per page render.
func WithQueryBudget(ctx context.Context, limit int) context.Context {
	return context.WithValue(ctx, queryBudgetKey{}, &QueryBudget{
		limit: limit,
		cache: make(map[string]*QueryResult),
	})
}

// QueryBudgetFrom returns the budget carried on ctx, or nil when none is
// attached (contexts that don't wire one run unbudgeted and uncached).
func QueryBudgetFrom(ctx context.Context) *QueryBudget {
	if ctx == nil {
		return nil
	}
	b, _ := ctx.Value(queryBudgetKey{}).(*QueryBudget)
	return b
}

// Limit returns the configured budget (0 = disabled).
func (b *QueryBudget) Limit() int { return b.limit }

// Stats returns a concurrency-safe snapshot without exposing cached results.
func (b *QueryBudget) Stats() QueryBudgetStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return QueryBudgetStats{
		Limit: b.limit, Executions: b.executions, CacheHits: b.cacheHits,
		CacheMisses: b.cacheMisses, Exceeded: b.exceeded,
	}
}

// Allow records one cache-miss execution against the budget. It returns false
// when the budget is already spent (a limit>0 that count has reached), in which
// case the caller must not execute the query. A limit of 0 always allows.
func (b *QueryBudget) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.limit > 0 {
		if b.count >= b.limit {
			return false
		}
		b.count++
	}
	b.executions++
	return true
}

// MarkExceeded returns true exactly once per budget — the first time the budget
// is reported exhausted — so the caller logs a single warning per page render.
func (b *QueryBudget) MarkExceeded() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.exceeded {
		return false
	}
	b.exceeded = true
	return true
}

// Lookup returns a private copy of a cached result for (query, opts), or false.
func (b *QueryBudget) Lookup(query string, opts QueryOptions) (*QueryResult, bool) {
	key := budgetCacheKey(query, opts)
	b.mu.Lock()
	defer b.mu.Unlock()
	r, ok := b.cache[key]
	if !ok {
		b.cacheMisses++
		return nil, false
	}
	b.cacheHits++
	return cloneQueryResult(r), true
}

// Store caches a private copy of result under (query, opts). Storing a copy (and
// returning copies from Lookup) keeps callers that mutate their result — e.g.
// [mrql] stamping a per-item block template — from corrupting the shared entry.
func (b *QueryBudget) Store(query string, opts QueryOptions, result *QueryResult) {
	if result == nil {
		return
	}
	key := budgetCacheKey(query, opts)
	b.mu.Lock()
	defer b.mu.Unlock()
	b.cache[key] = cloneQueryResult(result)
}

// BudgetedExecutor wraps base so that, when the request context carries a
// QueryBudget, identical queries are served from the budget's cache (free) and
// each cache miss is charged against the budget. Once the budget is spent, a
// miss returns a budget error without calling base — the [mrql] handler renders
// it as the standard error box. onExceeded (may be nil) fires exactly once per
// page render so the caller can log a single warning. With no budget on the
// context (contexts that don't wire one), base is called directly, unbudgeted
// and uncached.
func BudgetedExecutor(base QueryExecutor, onExceeded func(limit int)) QueryExecutor {
	return func(reqCtx context.Context, query string, opts QueryOptions) (*QueryResult, error) {
		budget := QueryBudgetFrom(reqCtx)
		if budget == nil {
			return base(reqCtx, query, opts)
		}
		if cached, ok := budget.Lookup(query, opts); ok {
			return cached, nil
		}
		if !budget.Allow() {
			if budget.MarkExceeded() && onExceeded != nil {
				onExceeded(budget.Limit())
			}
			return nil, fmt.Errorf(
				"inline query budget exceeded (%d per page); refine templates or raise -mrql-page-query-budget",
				budget.Limit())
		}
		result, err := base(reqCtx, query, opts)
		if err != nil {
			return nil, err
		}
		budget.Store(query, opts, result)
		return result, nil
	}
}

// budgetCacheKey builds a deterministic key from the executor inputs. Two calls
// with identical inputs produce identical results within a page, so keying on
// the raw inputs (before saved-name/scope resolution) is sufficient for dedup.
func budgetCacheKey(query string, opts QueryOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%s|%d|%d|%d|%d|%t",
		query, opts.SavedName, opts.ScopeGroupID, opts.Limit, opts.Buckets, len(opts.Params), opts.WantTotal)
	if len(opts.Params) > 0 {
		keys := make([]string, 0, len(opts.Params))
		for k := range opts.Params {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "|%q=%q", k, opts.Params[k])
		}
	}
	return b.String()
}

// cloneQueryResult makes a shallow copy with fresh Items/Rows/Groups slices so
// per-item mutation on the returned result can't alias the cached copy. Entity
// pointers inside items are shared (template rendering never mutates them).
func cloneQueryResult(r *QueryResult) *QueryResult {
	if r == nil {
		return nil
	}
	cp := *r
	if r.Items != nil {
		cp.Items = append([]QueryResultItem(nil), r.Items...)
	}
	if r.Rows != nil {
		cp.Rows = append([]map[string]any(nil), r.Rows...)
	}
	if r.Groups != nil {
		cp.Groups = make([]QueryResultGroup, len(r.Groups))
		for i := range r.Groups {
			cp.Groups[i] = r.Groups[i]
			cp.Groups[i].Items = append([]QueryResultItem(nil), r.Groups[i].Items...)
		}
	}
	return &cp
}
