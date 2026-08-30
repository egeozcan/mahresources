package plugin_system

import (
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	lua "github.com/yuin/gopher-lua"
)

type scheduledRecordingSubmitter struct {
	mu          sync.Mutex
	calls       int
	pluginName  string
	actorUserID uint
	url         string
	opts        map[string]any
}

func (r *scheduledRecordingSubmitter) SubmitDownload(pluginName string, actorUserID uint, url string, opts map[string]any) (map[string]any, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	r.pluginName = pluginName
	r.actorUserID = actorUserID
	r.url = url
	r.opts = opts
	if startAt, ok := DownloadSubmitStartAt(opts); ok {
		return map[string]any{
			"scheduled":    true,
			"scheduled_id": 42,
			"start_at":     startAt.Unix(),
		}, nil
	}
	return map[string]any{"id": "job-1", "url": url, "status": "pending"}, nil
}

func (r *scheduledRecordingSubmitter) snapshot() (int, map[string]any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, r.opts
}

func newScheduledSubmitTestState(t *testing.T) (*lua.LState, *scheduledRecordingSubmitter) {
	t.Helper()
	pm := mustEnable(t, t.TempDir(), "dl", `
plugin = { name = "dl", version = "1.0", api_version = 1, capabilities = { "db:write" } }
function init() end
`)
	sink := &scheduledRecordingSubmitter{}
	pm.SetDownloadSubmitter(sink)
	return stateForPlugin(t, pm, "dl"), sink
}

func TestDownloadSubmitStartAtSchedulesWithoutReturningAJobID(t *testing.T) {
	L, sink := newScheduledSubmitTestState(t)
	startAt := time.Now().Add(2 * time.Hour).Unix()
	if err := L.DoString(`
__job, __err = mah.download.submit("https://example.invalid/v.mp4", { start_at = ` + strconvFormatInt(startAt) + ` })
`); err != nil {
		t.Fatalf("mah.download.submit: %v", err)
	}
	if errVal := L.GetGlobal("__err"); errVal != lua.LNil {
		t.Fatalf("submit reported %v", errVal)
	}
	job := L.GetGlobal("__job").(*lua.LTable)
	if got := job.RawGetString("scheduled"); got != lua.LBool(true) {
		t.Fatalf("scheduled = %v, want true", got)
	}
	if got := job.RawGetString("scheduled_id"); got != lua.LNumber(42) {
		t.Fatalf("scheduled_id = %v, want 42", got)
	}
	if got := job.RawGetString("id"); got != lua.LNil {
		t.Fatalf("deferred submit returned job id %v; no queue job exists yet", got)
	}
	calls, opts := sink.snapshot()
	if calls != 1 {
		t.Fatalf("submitter calls = %d, want 1 bridge call", calls)
	}
	start, ok := DownloadSubmitStartAt(opts)
	if !ok || start.Unix() != startAt {
		t.Fatalf("bridge start_at = %v/%v, want unix %d", start, ok, startAt)
	}
}

func TestDownloadSubmitStartAtAcceptsFarFutureAbsoluteTimes(t *testing.T) {
	L, sink := newScheduledSubmitTestState(t)
	startAt := time.Now().Add(60 * 24 * time.Hour).Unix()
	if err := L.DoString(`
__job, __err = mah.download.submit("https://example.invalid/v.mp4", { start_at = ` + strconvFormatInt(startAt) + ` })
`); err != nil {
		t.Fatalf("mah.download.submit: %v", err)
	}
	if errVal := L.GetGlobal("__err"); errVal != lua.LNil {
		t.Fatalf("submit reported %v", errVal)
	}
	_, opts := sink.snapshot()
	start, ok := DownloadSubmitStartAt(opts)
	if !ok || start.Unix() != startAt {
		t.Fatalf("bridge start_at = %v/%v, want unix %d", start, ok, startAt)
	}
}

func TestDownloadSubmitDelaySchedulesFromNow(t *testing.T) {
	L, sink := newScheduledSubmitTestState(t)
	before := time.Now().Add(2 * time.Hour)
	if err := L.DoString(`
__job, __err = mah.download.submit("https://example.invalid/v.mp4", { delay = "2h" })
`); err != nil {
		t.Fatalf("mah.download.submit: %v", err)
	}
	if errVal := L.GetGlobal("__err"); errVal != lua.LNil {
		t.Fatalf("submit reported %v", errVal)
	}
	_, opts := sink.snapshot()
	start, ok := DownloadSubmitStartAt(opts)
	if !ok {
		t.Fatal("delay did not reach the bridge as a scheduled start")
	}
	after := time.Now().Add(2 * time.Hour)
	if start.Before(before.Add(-time.Second)) || start.After(after.Add(time.Second)) {
		t.Fatalf("delay start = %s, want around [%s,%s]", start, before, after)
	}
}

func TestDownloadSubmitWithoutDeferralKeepsImmediateShape(t *testing.T) {
	L, sink := newScheduledSubmitTestState(t)
	if err := L.DoString(`
__job, __err = mah.download.submit("https://example.invalid/v.mp4", { name = "v.mp4" })
`); err != nil {
		t.Fatalf("mah.download.submit: %v", err)
	}
	if errVal := L.GetGlobal("__err"); errVal != lua.LNil {
		t.Fatalf("submit reported %v", errVal)
	}
	job := L.GetGlobal("__job").(*lua.LTable)
	if got := job.RawGetString("id"); got != lua.LString("job-1") {
		t.Fatalf("id = %v, want immediate job id", got)
	}
	_, opts := sink.snapshot()
	if _, ok := DownloadSubmitStartAt(opts); ok {
		t.Fatal("immediate submit carried a scheduled start marker")
	}
}

func TestDownloadSubmitRejectsInvalidDeferralOptions(t *testing.T) {
	cases := []struct {
		name string
		lua  string
		want string
	}{
		{"both", `{ start_at = 4102444800, delay = "1h" }`, "start_at and delay"},
		{"bad delay type", `{ delay = 3 }`, "delay"},
		{"bad delay string", `{ delay = "soon" }`, "delay"},
		{"negative delay", `{ delay = "-1s" }`, "delay"},
		{"too long delay", `{ delay = "721h" }`, "30 days"},
		{"past start_at", `{ start_at = 1 }`, "start_at"},
		{"bad start_at type", `{ start_at = "tomorrow" }`, "start_at"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			L, sink := newScheduledSubmitTestState(t)
			if err := L.DoString(`
__job, __err = mah.download.submit("https://example.invalid/v.mp4", ` + tc.lua + `)
`); err != nil {
				t.Fatalf("mah.download.submit: %v", err)
			}
			if L.GetGlobal("__job") != lua.LNil {
				t.Fatalf("invalid deferral returned job %v", L.GetGlobal("__job"))
			}
			errVal := L.GetGlobal("__err")
			if errVal == lua.LNil || !strings.Contains(strings.ToLower(errVal.String()), strings.ToLower(tc.want)) {
				t.Fatalf("error = %v, want to contain %q", errVal, tc.want)
			}
			if calls, _ := sink.snapshot(); calls != 0 {
				t.Fatalf("invalid deferral reached submitter %d times", calls)
			}
		})
	}
}

func strconvFormatInt(n int64) string { return strconv.FormatInt(n, 10) }
