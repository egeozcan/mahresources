package plugin_system

import (
	"context"

	lua "github.com/yuin/gopher-lua"
)

// MediaProcessor is the host's media surface: read what a file contains, take a
// frame out of it, cut a clip from it.
//
// Declared here and implemented by application_context, which owns ffmpeg. This
// package stays free of process execution, and a plugin never names a path or
// an ffmpeg argument -- only a resource id and a number.
//
// It is deliberately *not* a separate wiring seam. The implementation is the
// same object BindInvocation returns, so every call is already bound to the
// acting principal: a group-limited caller cannot probe or cut a resource
// outside its subtree, and that follows from the handle rather than from a
// check somebody has to remember. A seam of its own would have been a second
// path to the same files with none of that.
type MediaProcessor interface {
	ProbeMedia(ctx context.Context, resourceID uint) (map[string]any, error)
	// FrameDataURI returns a JPEG "data:" URI, the shape mah.image already
	// speaks, so a frame composes with it without a conversion step.
	FrameDataURI(ctx context.Context, resourceID uint, atSeconds float64, maxWidth int) (string, error)
	TrimVideoClip(ctx context.Context, resourceID uint, start, end, comment string) error
}

// mediaProcessorFor returns the media surface bound to this call's principal,
// or nil when the host provides none.
//
// It reaches through querierFor rather than through a setter of its own, which
// is what makes the binding automatic: querierFor is the function the whole
// mah.db surface goes through, and a provider that cannot also process media
// simply answers nil here.
func (pm *PluginManager) mediaProcessorFor(L *lua.LState) MediaProcessor {
	q := pm.querierFor(L)
	if q == nil {
		return nil
	}
	mp, ok := q.(MediaProcessor)
	if !ok {
		return nil
	}
	return mp
}

// registerMediaModule installs mah.media.
//
// Its own capability, not part of "image". mah.image transforms bytes the
// plugin already holds and touches nothing else; these read files out of the
// user's library and spend an ffmpeg process doing it. Folding them together
// would widen every plugin already consented to image transforms into one that
// can read video out of the library, which is exactly the widening
// CompareGrants exists to report.
func (pm *PluginManager) registerMediaModule(L *lua.LState, mahMod *lua.LTable) {
	mod := L.NewTable()

	unavailable := func(L *lua.LState) int {
		L.Push(lua.LNil)
		L.Push(lua.LString("media processing is not available"))
		return 2
	}

	// mah.media.probe(resource_id) -> table or (nil, error)
	// The ffprobe document: `format` and `streams`, nested as ffprobe writes
	// them.
	mod.RawSetString("probe", L.NewFunction(func(L *lua.LState) int {
		mp := pm.mediaProcessorFor(L)
		if mp == nil {
			return unavailable(L)
		}
		id := checkEntityID(L, 1)
		out, err := mp.ProbeMedia(pm.luaContext(L), id)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(goToLuaTable(L, out))
		return 1
	}))

	// mah.media.extract_frame(resource_id, at_seconds, max_width) -> data_uri or (nil, error)
	mod.RawSetString("extract_frame", L.NewFunction(func(L *lua.LState) int {
		mp := pm.mediaProcessorFor(L)
		if mp == nil {
			return unavailable(L)
		}
		id := checkEntityID(L, 1)
		at := float64(L.OptNumber(2, 0))
		// 0 means "leave it at the video's own size", which is the honest
		// default for a frame nobody said anything about.
		width := L.OptInt(3, 0)

		uri, err := mp.FrameDataURI(pm.luaContext(L), id, at, width)
		if err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		L.Push(lua.LString(uri))
		return 1
	}))

	// mah.media.trim(resource_id, start, end, comment) -> true or (nil, error)
	// start and end are "SS", "MM:SS" or "HH:MM:SS", the same spellings the
	// trim UI accepts. The clip becomes a new version of the resource, which is
	// what the existing trim does; a plugin that wants a separate resource can
	// read the version back and create one.
	mod.RawSetString("trim", L.NewFunction(func(L *lua.LState) int {
		mp := pm.mediaProcessorFor(L)
		if mp == nil {
			return unavailable(L)
		}
		// A trim writes a new version, and it runs ffmpeg over the whole clip
		// while doing it. Inside a transaction that is the database's write
		// lock held for the length of a transcode.
		if pm.inTransaction(L) {
			L.Push(lua.LNil)
			L.Push(lua.LString(refusedInTransaction("mah.media.trim", whyItWaits)))
			return 2
		}
		id := checkEntityID(L, 1)
		start := L.CheckString(2)
		end := L.CheckString(3)
		comment := L.OptString(4, "")

		if err := mp.TrimVideoClip(pm.luaContext(L), id, start, end, comment); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}
		InvalidateMRQLCache(pm.luaContext(L))
		L.Push(lua.LTrue)
		return 1
	}))

	mahMod.RawSetString("media", mod)
}
