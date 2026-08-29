package application_context

import (
	"strings"
	"testing"
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
