package application_context

import (
	"strings"
	"testing"

	"mahresources/models"
	"mahresources/models/query_models"
)

// mah.download.submit end-to-end, against a real PluginManager and a real
// download manager, because what is being built is a chain: Lua → the seam →
// the queue → the policy the transfer runs under. A fake at any link would let
// the test pass while a plugin's download ran unpoliced.

// The hosts here are under .invalid, which is reserved and never resolves, so
// the worker these tests enqueue onto fails at DNS without a packet leaving the
// machine. A loopback test server cannot stand in: the plugin egress layer
// denies private addresses for every plugin, manifest or not.

// TestPluginDownloadSubmitEnqueuesAJob is the ordinary case. The plugin returns
// immediately with a job id, which is the entire point: a synchronous fetch
// holds the VM lock for the length of the transfer and, inside an async job,
// dies at MaxAsyncJobDuration.
func TestPluginDownloadSubmitEnqueuesAJob(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{
		"downloader": `
plugin = { name = "downloader", version = "1.0", api_version = 1,
           capabilities = { "db:write", "inject" },
           network = { "example.invalid" } }
function init()
    mah.inject("page_bottom", function(c)
        local job, err = mah.download.submit("https://example.invalid/clip.m3u8", { name = "clip" })
        if not job then return "error=" .. tostring(err) end
        return "id=" .. tostring(job.id) .. " status=" .. tostring(job.status)
    end)
end
`,
	})

	out := runSlot(ctx, "page_bottom")
	if !strings.Contains(out, "id=") || strings.Contains(out, "error=") {
		t.Fatalf("submit reported %q, want a job id", out)
	}
	if !strings.Contains(out, "status=pending") && !strings.Contains(out, "status=downloading") {
		t.Errorf("submit reported %q, want a queued status", out)
	}

	jobs := ctx.DownloadManager().GetJobs()
	if len(jobs) != 1 {
		t.Fatalf("%d jobs queued, want 1", len(jobs))
	}
	// The origin is what selects the egress policy on every attempt, including
	// a retry replayed after a restart.
	if got := jobs[0].PluginName(); got != "downloader" {
		t.Errorf("the queued job names plugin %q, want \"downloader\" — without it the transfer runs under the host policy", got)
	}
}

// TestPluginDownloadSubmitRefusesAHostOutsideTheNetworkList checks layer (a) at
// the call site. The transfer is policed again when it runs, so this is not the
// protection; it is the difference between telling the plugin now and letting
// it find out minutes later in a history row it has to go looking for.
func TestPluginDownloadSubmitRefusesAHostOutsideTheNetworkList(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{
		"confined": `
plugin = { name = "confined", version = "1.0", api_version = 1,
           capabilities = { "db:write", "inject" },
           network = { "allowed.example" } }
function init()
    mah.inject("page_bottom", function(c)
        local job, err = mah.download.submit("https://elsewhere.example/x.mp4")
        if job then return "submitted" end
        return "refused"
    end)
end
`,
	})

	if out := runSlot(ctx, "page_bottom"); out != "refused" {
		t.Fatalf("submit reported %q, want a refusal", out)
	}
	if jobs := ctx.DownloadManager().GetJobs(); len(jobs) != 0 {
		t.Errorf("%d jobs were queued despite the refusal", len(jobs))
	}
}

// TestPluginDownloadSubmitIsRefusedInsideATransaction. A queued download
// outlives the transaction, and a transaction held open across a fetch is what
// this refusal exists for everywhere else it appears.
func TestPluginDownloadSubmitIsRefusedInsideATransaction(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{
		"txn": `
plugin = { name = "txn", version = "1.0", api_version = 1,
           capabilities = { "db:write", "inject" },
           network = { "example.invalid" } }
function init()
    mah.inject("page_bottom", function(c)
        local report = "no-refusal"
        mah.db.transaction(function()
            local job, err = mah.download.submit("https://example.invalid/a.mp4")
            if job then report = "submitted" else report = tostring(err) end
        end)
        return report
    end)
end
`,
	})

	out := runSlot(ctx, "page_bottom")
	if strings.Contains(out, "submitted") || out == "no-refusal" {
		t.Fatalf("submit inside a transaction reported %q, want a refusal", out)
	}
	if !strings.Contains(out, "transaction") {
		t.Errorf("the refusal %q does not say what is wrong", out)
	}
	if jobs := ctx.DownloadManager().GetJobs(); len(jobs) != 0 {
		t.Errorf("%d jobs were queued from inside a transaction", len(jobs))
	}
}

// TestPluginDownloadSubmitRefusesATargetOutsideTheCallersScope is the hole a
// review found. The queue's worker runs unscoped by design -- it binds
// attribution and nothing else -- and /v1/download/submit compensates by
// validating the targets against the submitting principal before enqueueing.
// This path did not, so a plugin's Lua running on a confined user's own write
// could plant a resource in a group that user cannot see.
func TestPluginDownloadSubmitRefusesATargetOutsideTheCallersScope(t *testing.T) {
	ctx := newTwoPluginContext(t, map[string]string{
		"scoped-downloader": `
plugin = { name = "scoped-downloader", version = "1.0", api_version = 1,
           capabilities = { "db:write", "hooks", "inject" },
           network = { "example.invalid" } }
local report = "not-run"
function init()
    mah.on("after_note_create", function(data)
        local job, err = mah.download.submit("https://example.invalid/x.mp4",
                                             { owner_id = tonumber(data.description) })
        if job then report = "submitted" else report = tostring(err) end
        return data
    end)
    mah.inject("page_top", function(c) return report end)
end
`,
	})

	principal, inside := scopeProbeFixture(t, ctx)
	outside := &models.Group{Name: "far-away"}
	if err := ctx.db.Create(outside).Error; err != nil {
		t.Fatal(err)
	}
	scoped := ctx.WithPrincipal(principal)

	submitTargeting := func(owner uint) string {
		t.Helper()
		if _, err := scoped.CreateOrUpdateNote(&query_models.NoteEditor{
			NoteCreator: query_models.NoteCreator{
				Name: "trigger", OwnerId: inside.ID, Description: itoa(owner),
			},
		}); err != nil {
			t.Fatalf("confined user could not create a note in its own subtree: %v", err)
		}
		return runSlot(ctx, "page_top")
	}

	// The control: the same call inside the caller's own subtree is allowed, so
	// the refusal below is about scope rather than about the call never working.
	if got := submitTargeting(inside.ID); got != "submitted" {
		t.Fatalf("a download into the caller's own subtree reported %q", got)
	}
	queued := len(ctx.DownloadManager().GetJobs())

	if got := submitTargeting(outside.ID); got == "submitted" {
		t.Fatalf("a caller confined to %q submitted a download owned by a group outside it", inside.Name)
	}
	if now := len(ctx.DownloadManager().GetJobs()); now != queued {
		t.Errorf("%d jobs are queued, want %d — the out-of-scope submission was enqueued anyway", now, queued)
	}
}
