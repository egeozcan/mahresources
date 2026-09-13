package plugin_system

import (
	"context"
	"sync/atomic"
	"time"

	lua "github.com/yuin/gopher-lua"
)

// docsPreviewContextKey carries the deliberately isolated execution state used
// to render a block's documentation preview. It is never derived from a page
// request: a preview must not inherit its viewer's principal, database access,
// transaction, or cancellation-owned work.
type docsPreviewContextKey struct{}

type docsPreviewState struct {
	requestedHostData atomic.Bool
}

// valuesStrippedContext preserves a page request's cancellation and deadline
// without preserving any of its values. In particular, it does not carry the
// visitor principal, plugin access decision, or request-scoped data into Lua.
type valuesStrippedContext struct {
	request context.Context
}

func (ctx valuesStrippedContext) Deadline() (time.Time, bool) { return ctx.request.Deadline() }
func (ctx valuesStrippedContext) Done() <-chan struct{}       { return ctx.request.Done() }
func (ctx valuesStrippedContext) Err() error                  { return ctx.request.Err() }
func (valuesStrippedContext) Value(any) any                   { return nil }

func newDocsPreviewContext(request context.Context) context.Context {
	if request == nil {
		request = context.Background()
	}
	return context.WithValue(valuesStrippedContext{request: request}, docsPreviewContextKey{}, &docsPreviewState{})
}

func docsPreviewStateFromContext(ctx context.Context) *docsPreviewState {
	if ctx == nil {
		return nil
	}
	preview, _ := ctx.Value(docsPreviewContextKey{}).(*docsPreviewState)
	return preview
}

func isDocsPreviewContext(ctx context.Context) bool {
	return docsPreviewStateFromContext(ctx) != nil
}

func docsPreviewRequestedHostData(ctx context.Context) bool {
	preview := docsPreviewStateFromContext(ctx)
	return preview != nil && preview.requestedHostData.Load()
}

func (pm *PluginManager) isDocsPreview(L *lua.LState) bool {
	return isDocsPreviewContext(pm.luaContext(L))
}

// markDocsPreviewHostData records that a renderer attempted to access host
// state or start host work. Its output cannot honestly represent a block that
// can render from defaults alone, so the documentation page omits it.
func (pm *PluginManager) markDocsPreviewHostData(L *lua.LState) bool {
	preview := docsPreviewStateFromContext(pm.luaContext(L))
	if preview == nil {
		return false
	}
	preview.requestedHostData.Store(true)
	return true
}

// refuseDocsPreview stops a stateful or host-data operation from running while
// documentation is rendering a synthetic block. Pure Lua, JSON, formatting,
// and image-data transformations remain available.
func (pm *PluginManager) refuseDocsPreview(L *lua.LState, operation string) bool {
	if !pm.markDocsPreviewHostData(L) {
		return false
	}
	L.RaiseError("%s is unavailable in documentation previews", operation)
	return true
}
