package plugin_system

import (
	"strings"
	"testing"
	"time"
)

// schedulePlugin writes and enables a plugin whose init() body is `body`.
func schedulePlugin(t *testing.T, name, body string) (*PluginManager, error) {
	t.Helper()
	return enablingPlugin(t, t.TempDir(), name, `
plugin = { api_version = 1, name = "`+name+`", version = "1.0",
           description = "declares a schedule", capabilities = { "schedule" } }
function init()
`+body+`
end
`)
}

func TestScheduleRegistersAndIsReadableByTheBridge(t *testing.T) {
	pm, err := schedulePlugin(t, "poller", `
    mah.schedule({ id = "feed", every = "15m", handler = function(job_id) end })
`)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}

	regs := pm.DeclaredSchedules("poller")
	if len(regs) != 1 {
		t.Fatalf("got %d schedules, want 1", len(regs))
	}
	got := regs[0]
	if got.ScheduleID != "feed" {
		t.Errorf("ScheduleID = %q, want \"feed\"", got.ScheduleID)
	}
	if got.Every != 15*time.Minute {
		t.Errorf("Every = %s, want 15m", got.Every)
	}
	if got.EverySeconds != 900 {
		t.Errorf("EverySeconds = %d, want 900", got.EverySeconds)
	}
	// Unstated overlap is "skip". It is the policy that cannot double-run, so it
	// is the one a plugin gets without asking.
	if got.Overlap != ScheduleOverlapSkip {
		t.Errorf("Overlap = %q, want %q", got.Overlap, ScheduleOverlapSkip)
	}
	if !pm.ScheduleIsRegistered("poller", "feed") {
		t.Error("ScheduleIsRegistered says no for a schedule that just registered")
	}
	if pm.ScheduleIsRegistered("poller", "renamed") {
		t.Error("ScheduleIsRegistered says yes for an id nothing declared")
	}
}

// Every wrong shape is refused by name at load, rather than defaulted. A
// validator that guards the good shape and lets the rest fall through to a zero
// value makes a typo mean the opposite of what its author wrote — here, a
// mistyped `every` would read as zero and become "as often as the host allows".
func TestScheduleRefusesEveryMalformedDeclaration(t *testing.T) {
	cases := []struct {
		name, body, wantErr string
	}{
		{"no id", `mah.schedule({ every = "15m", handler = function() end })`, "id must be a string"},
		{"id with a slash", `mah.schedule({ id = "a/b", every = "15m", handler = function() end })`, "letters, digits"},
		{"no every", `mah.schedule({ id = "x", handler = function() end })`, "every must be a duration"},
		{"every is a number", `mah.schedule({ id = "x", every = 900, handler = function() end })`, "every must be a duration"},
		{"every unparseable", `mah.schedule({ id = "x", every = "soon", handler = function() end })`, "is not a duration"},
		{"every too short", `mah.schedule({ id = "x", every = "1s", handler = function() end })`, "below the"},
		{"no handler", `mah.schedule({ id = "x", every = "15m" })`, "handler must be a function"},
		{"handler is a string", `mah.schedule({ id = "x", every = "15m", handler = "go" })`, "handler must be a function"},
		{"unknown overlap", `mah.schedule({ id = "x", every = "15m", overlap = "queue", handler = function() end })`, "must be"},
		{"overlap is a boolean", `mah.schedule({ id = "x", every = "15m", overlap = true, handler = function() end })`, "overlap must be a string"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pm, err := schedulePlugin(t, "bad", "    "+tc.body)
			if err == nil {
				t.Fatalf("loaded cleanly; the schedule would never run as written")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not name the problem (want %q)", err, tc.wantErr)
			}
			if len(pm.DeclaredSchedules("bad")) != 0 {
				t.Error("a refused registration left a schedule behind")
			}
		})
	}
}

// The id is half of a database unique key, so two declarations of one id are a
// conflict the row cannot represent, not a last-one-wins.
func TestScheduleRefusesADuplicateID(t *testing.T) {
	_, err := schedulePlugin(t, "dupe", `
    mah.schedule({ id = "feed", every = "15m", handler = function() end })
    mah.schedule({ id = "feed", every = "30m", handler = function() end })
`)
	if err == nil {
		t.Fatal("two schedules registered under one id; the durable row cannot hold both")
	}
	if !strings.Contains(err.Error(), "already registered") {
		t.Errorf("error %q does not say the id is taken", err)
	}
}

// Withholding the capability withholds the function itself, rather than leaving
// it installed and failing at the call. That is how every other capability
// works: an ungranted one is a key that is never set.
func TestScheduleIsWithheldWithoutTheCapability(t *testing.T) {
	_, err := enablingPlugin(t, t.TempDir(), "ungranted", `
plugin = { api_version = 1, name = "ungranted", version = "1.0",
           description = "wants to schedule without asking", capabilities = { "hooks" } }
function init()
    mah.schedule({ id = "feed", every = "15m", handler = function() end })
end
`)
	if err == nil {
		t.Fatal("a plugin scheduled work without declaring the capability for it")
	}
	// "non-function object" is what calling an unset key looks like from Lua, and
	// it is the shape that matters: the function is absent, not present-and-
	// refusing. An ungranted capability is a key that is never set, so there is no
	// call site anywhere that has to remember to check.
	if !strings.Contains(err.Error(), "non-function object") {
		t.Errorf("error %q is not the shape of an uninstalled key; mah.schedule may be "+
			"installed and merely refusing, which is a check somebody can forget", err)
	}
}

// Disabling a plugin takes its schedules with it. That is the join the durable
// row depends on: a row whose plugin is gone has no live registration, and a row
// with no live registration is never run.
func TestDisablingAPluginRemovesItsSchedules(t *testing.T) {
	pm, err := schedulePlugin(t, "poller", `
    mah.schedule({ id = "feed", every = "15m", handler = function(job_id) end })
`)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	if !pm.ScheduleIsRegistered("poller", "feed") {
		t.Fatal("precondition: the schedule did not register")
	}

	if err := pm.DisablePlugin("poller"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if pm.ScheduleIsRegistered("poller", "feed") {
		t.Fatal("a disabled plugin's schedule is still registered, so its row would still be run")
	}
	if len(pm.DeclaredSchedules("poller")) != 0 {
		t.Fatal("DeclaredSchedules still reports a disabled plugin's work")
	}
	if len(pm.AllDeclaredSchedules()) != 0 {
		t.Fatal("AllDeclaredSchedules still reports a disabled plugin's work")
	}
}

// A tick holds a database claim on the schedule row for the whole run, so it
// must never park indefinitely waiting for a job slot: an unbounded wait makes
// the claim's lifetime unbounded, and a claim with no bounded lifetime cannot
// have an expiry that means anything. The failure that produces is a schedule
// unavailable for the length of the claim TTL, silently, under load.
//
// executeAsyncJob's own acquire is a blocking send with no escape, which is why
// the scheduler goes through executeAsyncJobWithin instead. Reporting ran=false
// having touched nothing is the contract the caller depends on to hand the claim
// back.
func TestRunScheduleGivesUpRatherThanWaitingForeverForAJobSlot(t *testing.T) {
	pm, err := schedulePlugin(t, "poller", `
    mah.schedule({ id = "feed", every = "15m", handler = function(job_id) end })
`)
	if err != nil {
		t.Fatalf("enable: %v", err)
	}
	regs := pm.DeclaredSchedules("poller")
	if len(regs) != 1 {
		t.Fatalf("got %d schedules, want 1", len(regs))
	}

	// Fill the async budget, exactly as a plugin running its full job allowance
	// would. Released through t.Cleanup so a failure cannot wedge the package.
	for i := 0; i < maxConcurrentActions; i++ {
		pm.actionSemaphore <- struct{}{}
	}
	t.Cleanup(func() {
		for i := 0; i < maxConcurrentActions; i++ {
			<-pm.actionSemaphore
		}
	})

	jobsBefore := len(pm.actionJobs)

	// Run it off the test goroutine and wait with a bound. If the acquire ever
	// goes back to being unbounded, this fails in five seconds naming the reason
	// rather than parking until `go test` aborts the whole package -- which would
	// destroy every other test's result to report this one.
	type outcome struct {
		jobID string
		ran   bool
		err   error
	}
	results := make(chan outcome, 1)
	go func() {
		jobID, ran, err := pm.RunSchedule(regs[0], 1, 50*time.Millisecond, true)
		results <- outcome{jobID, ran, err}
	}()

	var got outcome
	select {
	case got = <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("RunSchedule did not return within 5s against a 50ms bound: it is waiting for a " +
			"job slot without a deadline, so a caller's claim would be held for as long as the " +
			"budget stays full and the claim's expiry would mean nothing")
	}

	if got.ran {
		t.Fatal("RunSchedule reported that it ran with every job slot taken")
	}
	if got.err != nil {
		t.Fatalf("a full job budget is not an error, it is 'not this tick': %v", got.err)
	}
	if got.jobID != "" {
		t.Errorf("a job id was handed back for a run that never started: %q", got.jobID)
	}

	// Nothing touched: a job entry left behind would sit in the panel forever
	// claiming to be pending, for work that never ran.
	pm.actionJobsMu.Lock()
	jobsAfter := len(pm.actionJobs)
	pm.actionJobsMu.Unlock()
	if jobsAfter != jobsBefore {
		t.Errorf("a refused dispatch left %d job entries behind (was %d)", jobsAfter, jobsBefore)
	}
}
