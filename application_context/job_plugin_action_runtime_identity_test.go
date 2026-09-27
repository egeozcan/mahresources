package application_context

import (
	"encoding/json"
	"strings"
	"testing"

	"mahresources/jobs"
	"mahresources/models"
	"mahresources/plugin_system"
)

// The runtime identity a plugin-action Job records names the host, the boot
// session and the process. It is what reconciliation reads to prove a closure's
// process gone, and nothing a reader of the Job may see: not the sanitized
// summary every owner is shown, and not the text the search box matches.
func TestAPluginActionJobShowsNoRuntimeIdentity(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	identity := plugin_system.CurrentRuntimeIdentity()

	_, registered, err := ctx.RunPluginActionAsync(nil, pluginActionTestPlugin, "async-work", 1, nil, "")
	if err != nil {
		t.Fatalf("run the action: %v", err)
	}
	ref, err := (&pluginActionHostJobs{ctx: ctx}).StartClosureJob(plugin_system.ClosureJobRequest{
		PluginName: pluginActionTestPlugin, Label: "closure work",
	})
	if err != nil || ref == nil {
		t.Fatalf("start a closure job: %v, %v", ref, err)
	}

	for _, id := range []string{registered, ref.JobID} {
		snap, err := ctx.JobService().Get(ctx.jobDeps(), jobs.Access{Administrator: true}, id)
		if err != nil {
			t.Fatalf("read %s: %v", id, err)
		}
		var summary map[string]any
		if err := json.Unmarshal(snap.Summary, &summary); err != nil {
			t.Fatalf("decode the summary of %s: %v", id, err)
		}
		if _, present := summary["runtime"]; present {
			t.Fatalf("the summary of %s carries the runtime identity: %s", id, snap.Summary)
		}
		if strings.Contains(string(snap.Summary), identity.BootSession) || strings.Contains(string(snap.Summary), identity.Host) {
			t.Fatalf("the summary of %s names this process: %s", id, snap.Summary)
		}
	}

	for _, term := range []string{identity.BootSession, identity.Host} {
		page, err := ctx.JobService().List(ctx.jobDeps(), jobs.Access{Administrator: true},
			jobs.Filter{Kinds: []string{JobKindPluginAction}, Search: term}, jobs.Cursor{}, 0)
		if err != nil {
			t.Fatalf("search %q: %v", term, err)
		}
		if len(page.Jobs) != 0 {
			t.Fatalf("searching %q finds %d plugin-action jobs", term, len(page.Jobs))
		}
	}

	recorded, err := ctx.JobService().OriginRuntime(ctx.jobDeps(), ref.JobID)
	if err != nil {
		t.Fatalf("read the closure's recorded runtime: %v", err)
	}
	if recorded != identity.String() {
		t.Fatalf("the closure recorded runtime %q, want this process %q", recorded, identity.String())
	}
}

// A summary written by an earlier release still carries the identity, and it is
// served and searched until something rewrites it. The scrub moves it onto the
// Job's internal column, where reconciliation still finds it, and is idempotent.
func TestTheRuntimeIdentityIsScrubbedFromStoredPluginActionSummaries(t *testing.T) {
	exerciseTheRuntimeIdentityScrub(t, newPluginActionJobContext(t))
}

func exerciseTheRuntimeIdentityScrub(t *testing.T, ctx *MahresourcesContext) {
	t.Helper()
	legacyRuntime := plugin_system.CurrentRuntimeIdentity().Host + "/boot-from-before/4242"
	accepted, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
		Origin: "plugin", Title: "written by an earlier release", Replay: jobs.ReplayInput{NonReplayable: true},
		Summary: json.RawMessage(`{"subtype":"closure-start-job","plugin":"` + pluginActionTestPlugin +
			`","runtime":"` + legacyRuntime + `","noTerminalHook":true}`),
	})
	if err != nil {
		t.Fatalf("accept the legacy-shaped job: %v", err)
	}
	other, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: "remote-download", KindVersion: 1, State: jobs.StateQueued, Origin: "api", Title: "another kind",
		Replay: jobs.ReplayInput{NonReplayable: true}, Summary: json.RawMessage(`{"host":"example.com","runtime":"kept"}`),
	})
	if err != nil {
		t.Fatalf("accept a job of another kind: %v", err)
	}

	scrubbed, err := ctx.ScrubPluginActionRuntimeFromSummaries()
	if err != nil || scrubbed != 1 {
		t.Fatalf("scrub = %d, %v; want one job", scrubbed, err)
	}
	var row models.Job
	if err := ctx.db.Where("id = ?", accepted.ID).First(&row).Error; err != nil {
		t.Fatalf("read the scrubbed job: %v", err)
	}
	var summary map[string]any
	if err := json.Unmarshal(row.Summary, &summary); err != nil {
		t.Fatalf("decode the scrubbed summary: %v", err)
	}
	if _, present := summary["runtime"]; present {
		t.Fatalf("the scrubbed summary still carries the runtime: %s", row.Summary)
	}
	if summary["noTerminalHook"] != true || summary["subtype"] != "closure-start-job" {
		t.Fatalf("the scrub lost the rest of the summary: %s", row.Summary)
	}
	if row.OriginRuntime != legacyRuntime {
		t.Fatalf("the scrub left origin runtime %q, want %q", row.OriginRuntime, legacyRuntime)
	}
	if row.Version != accepted.Version {
		t.Fatalf("the scrub moved the job's version from %d to %d", accepted.Version, row.Version)
	}

	var untouched models.Job
	if err := ctx.db.Where("id = ?", other.ID).First(&untouched).Error; err != nil {
		t.Fatalf("read the other job: %v", err)
	}
	if !strings.Contains(string(untouched.Summary), `"runtime"`) {
		t.Fatalf("the scrub rewrote another kind's summary: %s", untouched.Summary)
	}

	again, err := ctx.ScrubPluginActionRuntimeFromSummaries()
	if err != nil || again != 0 {
		t.Fatalf("a second scrub = %d, %v; want nothing left to do", again, err)
	}
}

// The identity is stored as its own string and read back unchanged, whatever
// fields it carries, so a later release that adds one to it (a per-process
// nonce) does not lose it here and turn a live process's own work into work of
// an unknown one.
func TestTheOriginRuntimeIsStoredOpaque(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	identity := "a-rather-long-host-name.internal.example/45D21A7C-39C3-4CBC-B5B9-F3C23F9680DC/4242/8d3c1f0e9b7a6d5c"
	accepted := acceptClosureJobForTest(t, ctx, identity)
	recorded, err := ctx.JobService().OriginRuntime(ctx.jobDeps(), accepted.ID)
	if err != nil || recorded != identity {
		t.Fatalf("the origin runtime read back as %q, %v; want %q", recorded, err, identity)
	}
}

// A closure accepted by an older process running beside this one still carries
// its identity in the summary until the next start scrubs it; adoption reads it
// there rather than treating the closure as one nobody can account for.
func TestAdoptionReadsTheRuntimeOfARowTheScrubHasNotReached(t *testing.T) {
	ctx := newPluginActionJobContext(t)
	gone := goneRuntimeIdentityForTest()
	accepted, err := ctx.JobService().Accept(ctx.jobDeps(), jobs.Acceptance{
		Kind: JobKindPluginAction, KindVersion: jobPluginActionKindVersion, State: jobs.StateQueued,
		Origin: "plugin", Title: "written by an older process", Replay: jobs.ReplayInput{NonReplayable: true},
		Summary: json.RawMessage(`{"subtype":"closure-start-job","plugin":"` + pluginActionTestPlugin + `","runtime":"` + gone + `"}`),
	})
	if err != nil {
		t.Fatalf("accept the older process's closure: %v", err)
	}
	withdrawn := waitForJobState(t, ctx, accepted.ID, "the orphaned closure to be withdrawn", func(s jobs.Snapshot) bool {
		return s.State.Terminal()
	})
	if withdrawn.State != jobs.StateCancelled || lastEventType(t, ctx, accepted.ID) != "not-started" {
		t.Fatalf("an older process's orphaned closure ended %s (last event %q), want withdrawn as never started",
			withdrawn.State, lastEventType(t, ctx, accepted.ID))
	}
}
