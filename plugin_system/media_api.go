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
	// TrimVideoClipToResource files the clip as a resource of its own and
	// returns it, leaving the source untouched.
	TrimVideoClipToResource(ctx context.Context, resourceID uint, start, end, name string) (map[string]any, error)
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
		// Refused inside a transaction for the same reason trim is, and it is
		// not a lesser case: this waits up to a minute for a video slot, may
		// copy the file, and then runs a process -- all while the caller's
		// transaction holds the database's write lock. A read that takes a
		// minute is as bad as a write that does.
		if pm.inTransaction(L) {
			L.Push(lua.LNil)
			L.Push(lua.LString(refusedInTransaction("mah.media.probe", whyItWaits)))
			return 2
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
		// Refused inside a transaction for the same reason trim is, and it is
		// not a lesser case: this waits up to a minute for a video slot, may
		// copy the file, and then runs a process -- all while the caller's
		// transaction holds the database's write lock. A read that takes a
		// minute is as bad as a write that does.
		if pm.inTransaction(L) {
			L.Push(lua.LNil)
			L.Push(lua.LString(refusedInTransaction("mah.media.extract_frame", whyItWaits)))
			return 2
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

	// mah.media.trim(resource_id, start, end, options_or_comment) -> true | resource | (nil, error)
	//
	// start and end are "SS", "MM:SS" or "HH:MM:SS", the same spellings the trim
	// UI accepts.
	//
	// The fourth argument is a table, or a string for the comment. `into`
	// chooses what the clip becomes:
	//
	//   "version"  (default) a new version of the resource, replacing its
	//              current content -- what the trim button does, and what you
	//              want when the clip *is* the thing.
	//   "resource" a resource of its own, source untouched -- what you want
	//              when the source is a recording you are pulling clips out of.
	//              Returns the new resource table rather than true.
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

		// A string fourth argument stays the comment it always was, so the
		// simple call keeps working unchanged.
		into, comment, name := "version", "", ""
		switch arg := L.Get(4).(type) {
		case *lua.LTable:
			if v, ok := arg.RawGetString("into").(lua.LString); ok {
				into = string(v)
			}
			if v, ok := arg.RawGetString("comment").(lua.LString); ok {
				comment = string(v)
			}
			if v, ok := arg.RawGetString("name").(lua.LString); ok {
				name = string(v)
			}
		case lua.LString:
			comment = string(arg)
		case *lua.LNilType:
		default:
			L.ArgError(4, "expected a table or a comment string")
		}

		switch into {
		case "version":
			if err := mp.TrimVideoClip(pm.luaContext(L), id, start, end, comment); err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			InvalidateMRQLCache(pm.luaContext(L))
			L.Push(lua.LTrue)
			return 1
		case "resource":
			res, err := mp.TrimVideoClipToResource(pm.luaContext(L), id, start, end, name)
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			InvalidateMRQLCache(pm.luaContext(L))
			L.Push(goToLuaTable(L, res))
			return 1
		default:
			// Named rather than silently defaulted: a typo that quietly
			// replaced a two-hour recording with a ten-second clip is not
			// something the author would find out about in time.
			L.Push(lua.LNil)
			L.Push(lua.LString(`into must be "version" or "resource"`))
			return 2
		}
	}))

	mahMod.RawSetString("media", mod)
}
