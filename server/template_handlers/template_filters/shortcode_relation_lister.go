package template_filters

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"time"

	"mahresources/application_context"
	"mahresources/models"
	"mahresources/shortcodes"
)

// RelationListerContext is what [relations] needs from the application: the
// edge listing, the far-group details a block body may read, the timeout that
// bounds them, and the logger for their failures.
type RelationListerContext interface {
	IsGroupVisibleForTemplateRelations(groupID uint) (bool, error)
	ListGroupRelationsForTemplate(context.Context, shortcodes.RelationsRequest) ([]shortcodes.GroupRelationEntry, error)
	LoadRelatedGroupDetails(context.Context, []uint) ([]models.Group, error)
	MRQLQueryTimeout() time.Duration
	Logger() *application_context.Logger
}

// relationCall is one lookup of the page cache: done closes when entries and
// err are set.
type relationCall struct {
	done    chan struct{}
	entries []shortcodes.GroupRelationEntry
	err     error
}

// BuildRelationLister returns the [relations] lister for one render, or nil
// when appCtx is missing (a nil interface or a typed-nil pointer), which makes
// [relations] render its "unavailable" comment.
//
// Each statement takes one unit of the page query budget: the edge listing
// always, and the far-group details when the request asks for them. Identical
// requests on the page are served from a small cache and cost nothing; the
// slots of one page may render concurrently, so an identical request waits for
// the one in flight rather than spending budget on a second. Only successes
// are cached. Each lookup runs under the MRQL query timeout.
func BuildRelationLister(appCtx RelationListerContext) shortcodes.RelationLister {
	if appCtx == nil {
		return nil
	}
	if v := reflect.ValueOf(appCtx); v.Kind() == reflect.Ptr && v.IsNil() {
		return nil
	}
	var mu sync.Mutex
	calls := map[shortcodes.RelationsRequest]*relationCall{}
	return func(reqCtx context.Context, req shortcodes.RelationsRequest) ([]shortcodes.GroupRelationEntry, error) {
		if ok, err := appCtx.IsGroupVisibleForTemplateRelations(req.GroupID); !ok {
			return nil, err
		}
		mu.Lock()
		if c, ok := calls[req]; ok {
			mu.Unlock()
			select {
			case <-c.done:
				return c.entries, c.err
			case <-reqCtx.Done():
				return nil, reqCtx.Err()
			}
		}
		c := &relationCall{done: make(chan struct{})}
		calls[req] = c
		mu.Unlock()

		c.entries, c.err = lookupRelations(reqCtx, appCtx, req)
		if c.err != nil {
			mu.Lock()
			delete(calls, req)
			mu.Unlock()
		}
		close(c.done)
		return c.entries, c.err
	}
}

// chargeRelationStatement takes one unit of the page budget for one statement.
func chargeRelationStatement(reqCtx context.Context, appCtx RelationListerContext) error {
	budget := shortcodes.QueryBudgetFrom(reqCtx)
	if budget == nil || budget.Allow() {
		return nil
	}
	if budget.MarkExceeded() {
		logPageQueryBudgetExceeded(appCtx, budget.Limit())
	}
	return fmt.Errorf("%w: inline query budget exceeded (%d per page); refine templates or raise -mrql-page-query-budget",
		shortcodes.ErrRelationsBudget, budget.Limit())
}

// lookupRelations runs the edge statement and, when asked, the details
// statement, each charged to the page budget, under one MRQL timeout.
func lookupRelations(reqCtx context.Context, appCtx RelationListerContext, req shortcodes.RelationsRequest) ([]shortcodes.GroupRelationEntry, error) {
	if err := chargeRelationStatement(reqCtx, appCtx); err != nil {
		return nil, err
	}
	// WithTimeout keeps an earlier parent deadline, such as the deferred
	// endpoint's.
	bounded, cancel := context.WithTimeout(reqCtx, appCtx.MRQLQueryTimeout())
	defer cancel()
	entries, err := appCtx.ListGroupRelationsForTemplate(bounded, req)
	if err == nil && req.Details && len(entries) > 0 {
		if err := chargeRelationStatement(reqCtx, appCtx); err != nil {
			return nil, err
		}
		err = mergeRelatedGroupDetails(bounded, appCtx, entries)
	}
	if err != nil {
		if !errors.Is(err, shortcodes.ErrRelationsUnavailable) {
			appCtx.Logger().Warning(models.LogActionSystem, "shortcode", nil, "",
				fmt.Sprintf("[relations] lookup for group %d failed: %v", req.GroupID, err),
				map[string]interface{}{"groupId": req.GroupID, "direction": req.Direction, "type": req.Type})
		}
		return nil, err
	}
	return entries, nil
}

// mergeRelatedGroupDetails loads the far groups' Description and Meta once per
// distinct group and writes them onto every entry that names it.
func mergeRelatedGroupDetails(ctx context.Context, appCtx RelationListerContext, entries []shortcodes.GroupRelationEntry) error {
	seen := make(map[uint]bool, len(entries))
	ids := make([]uint, 0, len(entries))
	for _, e := range entries {
		if !seen[e.GroupID] {
			seen[e.GroupID] = true
			ids = append(ids, e.GroupID)
		}
	}
	groups, err := appCtx.LoadRelatedGroupDetails(ctx, ids)
	if err != nil {
		return err
	}
	byID := make(map[uint]*models.Group, len(groups))
	for i := range groups {
		byID[groups[i].ID] = &groups[i]
	}
	for i := range entries {
		if g := byID[entries[i].GroupID]; g != nil {
			entries[i].GroupDescription = g.Description
			if len(g.Meta) > 0 {
				entries[i].GroupMeta = []byte(g.Meta)
			}
		}
	}
	return nil
}
