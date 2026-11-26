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

// BuildAdjacentResolver returns the [adjacent] resolver for one page render.
// Identical lookups on the page are served from a small cache. The slots of
// one page may render concurrently against one context, so a lookup holds the
// lock from the cache check to the store: an identical lookup waits for the
// first one's result rather than spending budget on a second. Each other lookup takes one unit of the page query budget and runs
// under the MRQL query timeout. Only successes are cached.
func BuildAdjacentResolver(appCtx AdjacentContext) shortcodes.AdjacentResolver {
	var mu sync.Mutex
	cache := map[adjacentCacheKey]shortcodes.AdjacentResult{}
	return func(reqCtx context.Context, q shortcodes.AdjacentQuery) (shortcodes.AdjacentResult, error) {
		key := adjacentCacheKey{q.EntityType, q.EntityID, q.Series}
		mu.Lock()
		defer mu.Unlock()
		if cached, ok := cache[key]; ok {
			return cached, nil
		}
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
		cache[key] = result
		return result, nil
	}
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
