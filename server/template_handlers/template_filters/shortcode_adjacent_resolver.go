package template_filters

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/shortcodes"
)

var _ AdjacentContext = (*application_context.MahresourcesContext)(nil)

// AdjacentContext is what [adjacent] needs from the application: the sibling
// lookup, the timeout that bounds it, and the logger for its failures.
type AdjacentContext interface {
	AdjacentEntities(context.Context, shortcodes.AdjacentQuery) (shortcodes.AdjacentResult, error)
	MRQLQueryTimeout() time.Duration
	Logger() *application_context.Logger
}

type adjacentCacheKey struct {
	entityType string
	id         uint
	series     bool
}

// adjacentCall is one lookup of the page cache: done closes when result and
// err are set.
type adjacentCall struct {
	done   chan struct{}
	result shortcodes.AdjacentResult
	err    error
}

// BuildAdjacentResolver returns the [adjacent] resolver for one page render.
// Identical lookups on the page are served from a small cache, and each other
// lookup takes one unit of the page query budget and runs under the MRQL query
// timeout. The slots of one page may render concurrently against one context:
// distinct lookups run in parallel, and an identical lookup waits for the one
// in flight (or its own context) rather than spending budget on a second.
// Only successes are cached.
func BuildAdjacentResolver(appCtx AdjacentContext) shortcodes.AdjacentResolver {
	var mu sync.Mutex
	calls := map[adjacentCacheKey]*adjacentCall{}
	return func(reqCtx context.Context, q shortcodes.AdjacentQuery) (shortcodes.AdjacentResult, error) {
		key := adjacentCacheKey{q.EntityType, q.EntityID, q.Series}
		mu.Lock()
		if c, ok := calls[key]; ok {
			mu.Unlock()
			select {
			case <-c.done:
				return c.result, c.err
			case <-reqCtx.Done():
				return shortcodes.AdjacentResult{}, reqCtx.Err()
			}
		}
		c := &adjacentCall{done: make(chan struct{})}
		calls[key] = c
		mu.Unlock()

		c.result, c.err = lookupAdjacent(reqCtx, appCtx, q)
		if c.err != nil {
			mu.Lock()
			delete(calls, key)
			mu.Unlock()
		}
		close(c.done)
		return c.result, c.err
	}
}

// lookupAdjacent spends one unit of the page budget and runs one lookup under
// the MRQL query timeout.
func lookupAdjacent(reqCtx context.Context, appCtx AdjacentContext, q shortcodes.AdjacentQuery) (shortcodes.AdjacentResult, error) {
	if budget := shortcodes.QueryBudgetFrom(reqCtx); budget != nil && !budget.Allow() {
		if budget.MarkExceeded() {
			logPageQueryBudgetExceeded(appCtx, budget.Limit())
		}
		return shortcodes.AdjacentResult{}, fmt.Errorf(
			"%w: inline query budget exceeded (%d per page); refine templates or raise -mrql-page-query-budget",
			shortcodes.ErrAdjacentBudget, budget.Limit())
	}
	// WithTimeout keeps an earlier parent deadline, such as the deferred
	// endpoint's.
	bounded, cancel := context.WithTimeout(reqCtx, appCtx.MRQLQueryTimeout())
	defer cancel()
	result, err := appCtx.AdjacentEntities(bounded, q)
	if err != nil {
		appCtx.Logger().Warning(models.LogActionSystem, "shortcode", nil, "",
			fmt.Sprintf("[adjacent] lookup for %s %d failed: %v", q.EntityType, q.EntityID, err),
			map[string]interface{}{"entityType": q.EntityType, "entityId": q.EntityID, "series": q.Series})
		return shortcodes.AdjacentResult{}, err
	}
	return result, nil
}

// WithAdjacentResolverFor installs the [adjacent] resolver on reqCtx when
// appCtx can answer it, and returns reqCtx unchanged otherwise. Call it only
// on a surface whose appCtx is bound with WithPrincipal, so the ORM subtree
// scope is on its handle; a WithMRQLPrincipal binding carries none.
func WithAdjacentResolverFor(reqCtx context.Context, appCtx any) context.Context {
	adjacent, ok := appCtx.(AdjacentContext)
	if !ok || adjacent == nil {
		return reqCtx
	}
	if v := reflect.ValueOf(adjacent); v.Kind() == reflect.Ptr && v.IsNil() {
		return reqCtx
	}
	return shortcodes.WithAdjacentResolver(reqCtx, BuildAdjacentResolver(adjacent))
}
