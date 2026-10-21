package template_filters

import (
	"sync"

	"github.com/flosch/pongo2/v4"
)

// metaScopeMemo remembers a MetaScopeResolver's answers for one page render.
//
// Every shortcode slot builds a meta context that carries its entity's owner's
// parent and root group, and both are database walks up the ownership chain:
// the root walk issues one query per level. A list page renders a slot per card,
// usually for a handful of owners, so without this a page of 50 cards made 100 to
// 150 sequential queries for the same few answers. A page is short-lived, so an
// ownership change during the render is not a concern.
type metaScopeMemo struct {
	MetaScopeResolver

	mu     sync.Mutex
	parent map[uint]uint
	root   map[uint]uint
}

func newMetaScopeMemo(resolver MetaScopeResolver) *metaScopeMemo {
	return &metaScopeMemo{MetaScopeResolver: resolver, parent: map[uint]uint{}, root: map[uint]uint{}}
}

func (m *metaScopeMemo) ResolveParentScopeID(groupID uint) uint {
	return m.remember(m.parent, groupID, m.MetaScopeResolver.ResolveParentScopeID)
}

func (m *metaScopeMemo) ResolveRootScopeID(groupID uint) uint {
	return m.remember(m.root, groupID, m.MetaScopeResolver.ResolveRootScopeID)
}

func (m *metaScopeMemo) remember(answers map[uint]uint, groupID uint, resolve func(uint) uint) uint {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := answers[groupID]; ok {
		return id
	}
	id := resolve(groupID)
	answers[groupID] = id
	return id
}

// pageMetaScopeResolver returns the page's memo around appCtx, creating it on
// the first slot of the render. It returns nil when appCtx is nil, so callers
// keep the sentinel scope they fall back to without a context.
func pageMetaScopeResolver(ctx *pongo2.ExecutionContext, appCtx MetaScopeResolver) MetaScopeResolver {
	if appCtx == nil {
		return nil
	}
	if memo, ok := ctx.Public["_metaScopeMemo"].(*metaScopeMemo); ok {
		return memo
	}
	memo := newMetaScopeMemo(appCtx)
	ctx.Public["_metaScopeMemo"] = memo
	return memo
}
