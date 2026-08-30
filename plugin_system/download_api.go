package plugin_system

import (
	"fmt"
	"math"
	"time"

	lua "github.com/yuin/gopher-lua"
)

const (
	// DownloadMaxDeferral bounds one-shot deferred host downloads. Longer-lived,
	// unattended work belongs to mah.schedule and its own capability.
	DownloadMaxDeferral = 30 * 24 * time.Hour

	// downloadSubmitStartAtOption is the typed bridge marker mah.download.submit
	// adds after validating `start_at` or `delay`. It stays unexported so callers
	// use the typed helpers below rather than sharing a magic map key.
	downloadSubmitStartAtOption = "__mah_download_start_at"
)

// SetDownloadSubmitStartAt stores a parsed deferred start in the bridge options
// map. Lua cannot manufacture the time.Time value, so direct option names remain
// ordinary resource options.
func SetDownloadSubmitStartAt(opts map[string]any, startAt time.Time) {
	if opts == nil {
		return
	}
	opts[downloadSubmitStartAtOption] = startAt
}

// DownloadSubmitStartAt returns the parsed deferred start carried through the
// mah.download.submit bridge.
func DownloadSubmitStartAt(opts map[string]any) (time.Time, bool) {
	if opts == nil {
		return time.Time{}, false
	}
	startAt, ok := opts[downloadSubmitStartAtOption].(time.Time)
	return startAt, ok
}

// DownloadSubmitter enqueues a download the host performs.
//
// Declared here and implemented by application_context, the same seam shape as
// KVStore and HistoryRecorder: this package must not reach up into the layer
// that owns the queue.
//
// pluginName travels with the submission because it selects the egress policy
// every fetch that download makes runs under -- including a retry replayed
// months later in another process. The host resolves it to a policy per
// attempt; a plugin that has since been disabled cannot fetch, which is what
// disabling it means.
type DownloadSubmitter interface {
	SubmitDownload(pluginName string, actorUserID uint, url string, opts map[string]any) (map[string]any, error)
}

// SetDownloadSubmitter wires the queue. Unset, mah.download.submit reports that
// the queue is unavailable rather than silently doing nothing.
func (pm *PluginManager) SetDownloadSubmitter(d DownloadSubmitter) {
	pm.downloadSubmitter.Store(d)
}

func (pm *PluginManager) getDownloadSubmitter() DownloadSubmitter {
	v := pm.downloadSubmitter.Load()
	if v == nil {
		return nil
	}
	return v.(DownloadSubmitter)
}

// registerDownloadModule installs mah.download.
//
// Gated on db:write, and deliberately not on a name of its own. The power is
// the one create_resource_from_url already grants -- fetch a URL of the
// plugin's choosing into the library -- differing only in whether the caller
// waits for it. It is not "jobs" either: jobs means *plugin code* runs in the
// background, and nothing of the plugin's runs here at all. The capability's
// own label already reads "...and fetch a URL of its choosing into it".
//
// What it buys is the two things a synchronous fetch cannot give: the VM lock
// is not held for the length of a transfer, and the work is not bounded by
// MaxAsyncJobDuration. A plugin that wants to know how it ended listens for
// after_job_completed, which carries the created resource.
func (pm *PluginManager) registerDownloadModule(L *lua.LState, mahMod *lua.LTable, pluginNamePtr *string, egress NetworkPolicy) {
	mod := L.NewTable()

	// mah.download.submit(url, options) -> table or (nil, error)
	mod.RawSetString("submit", L.NewFunction(func(L *lua.LState) int {
		// A queued download outlives the transaction that would be waiting for
		// it, and a transaction held open across a network fetch is what this
		// refusal exists for everywhere else it appears.
		if pm.inTransaction(L) {
			L.Push(lua.LNil)
			L.Push(lua.LString(refusedInTransaction("mah.download.submit", whyItWaits)))
			return 2
		}

		// The same liveness question querierFor asks for every mah.db call. An
		// async job can outlive a disable, and a VM revoked while it slept must
		// not go on writing -- least of all after the operator was told the
		// plugin was off.
		if !pm.stateIsLive(L) {
			L.Push(lua.LNil)
			L.Push(lua.LString("this plugin is no longer enabled"))
			return 2
		}

		submitter := pm.getDownloadSubmitter()
		if submitter == nil {
			L.Push(lua.LNil)
			L.Push(lua.LString("the download queue is not available"))
			return 2
		}

		url := L.CheckString(1)
		// Layer (a) at the call site, as create_resource_from_url checks it
		// before creating anything. The transfer is policed again when it runs,
		// so this is not the protection -- it is the difference between telling
		// the plugin now and letting it discover the refusal minutes later in a
		// history row it has to go looking for.
		if err := checkEgressHost(egress, url); err != nil {
			L.Push(lua.LNil)
			L.Push(lua.LString(err.Error()))
			return 2
		}

		opts := make(map[string]any)
		if optTbl := L.OptTable(2, nil); optTbl != nil {
			checkEntityIDOpts(L, 2, optTbl)
			startAt, scheduled, err := parseDownloadSubmitDeferral(optTbl, time.Now())
			if err != nil {
				L.Push(lua.LNil)
				L.Push(lua.LString(err.Error()))
				return 2
			}
			opts = luaTableToGoMap(optTbl)
			delete(opts, "start_at")
			delete(opts, "delay")
			if scheduled {
				SetDownloadSubmitStartAt(opts, startAt)
			}
		}

		name := ""
		if pluginNamePtr != nil {
			name = *pluginNamePtr
		}

		job, err := submitter.SubmitDownload(name, pm.actorFor(L), url, opts)
		if err != nil {
			logEgressRefusal(err, name, "GET", url)
			L.Push(lua.LNil)
			// Sanitized like every other fetch refusal reaching Lua: our own
			// message and Go's *net.OpError prefix both carry the address a
			// name resolved to, and handing that back turns each refusal into
			// one line of an internal DNS map.
			L.Push(lua.LString(egressErrorForPlugin(err)))
			return 2
		}
		L.Push(goToLuaTable(L, job))
		return 1
	}))

	mahMod.RawSetString("download", mod)
}

func parseDownloadSubmitDeferral(tbl *lua.LTable, now time.Time) (time.Time, bool, error) {
	if tbl == nil {
		return time.Time{}, false, nil
	}
	startVal := tbl.RawGetString("start_at")
	delayVal := tbl.RawGetString("delay")
	if startVal != lua.LNil && delayVal != lua.LNil {
		return time.Time{}, false, fmt.Errorf("start_at and delay are mutually exclusive")
	}
	if delayVal != lua.LNil {
		delayStr, ok := delayVal.(lua.LString)
		if !ok {
			return time.Time{}, false, fmt.Errorf("delay must be a duration string such as \"2h\", got %s", delayVal.Type())
		}
		delay, err := time.ParseDuration(string(delayStr))
		if err != nil {
			return time.Time{}, false, fmt.Errorf("delay %q is not a duration: %v", string(delayStr), err)
		}
		if delay < 0 {
			return time.Time{}, false, fmt.Errorf("delay must not be negative, got %q", string(delayStr))
		}
		if delay > DownloadMaxDeferral {
			return time.Time{}, false, fmt.Errorf("delay %q exceeds the 30 days maximum", string(delayStr))
		}
		return now.Add(delay), true, nil
	}
	if startVal == lua.LNil {
		return time.Time{}, false, nil
	}
	startNum, ok := startVal.(lua.LNumber)
	if !ok {
		return time.Time{}, false, fmt.Errorf("start_at must be a unix seconds number, got %s", startVal.Type())
	}
	startAt, err := downloadStartAtFromUnixSeconds(float64(startNum))
	if err != nil {
		return time.Time{}, false, err
	}
	// Refuse past absolute times rather than accepting a row that will fire on
	// the next tick: a typo should fail at submit time, not become "run now".
	if startAt.Before(now) {
		return time.Time{}, false, fmt.Errorf("start_at must not be in the past")
	}
	if startAt.After(now.Add(DownloadMaxDeferral)) {
		return time.Time{}, false, fmt.Errorf("start_at exceeds the 30 days maximum")
	}
	return startAt, true, nil
}

func downloadStartAtFromUnixSeconds(seconds float64) (time.Time, error) {
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) {
		return time.Time{}, fmt.Errorf("start_at must be a finite unix seconds number")
	}
	if seconds < 0 {
		return time.Time{}, fmt.Errorf("start_at must not be negative")
	}
	whole, frac := math.Modf(seconds)
	const maxUnixSeconds = float64(1<<63 - 1)
	if whole > maxUnixSeconds {
		return time.Time{}, fmt.Errorf("start_at is too large")
	}
	return time.Unix(int64(whole), int64(frac*float64(time.Second))).UTC(), nil
}
