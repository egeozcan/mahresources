package plugin_system

import (
	"os"
	"strconv"
	"sync"
	"testing"
	"time"
)

// This file pins the host-Job seam: what plugin_system reports into a durable
// Job, and what it does when the callback behind one stops existing.
//
// The host is a recording double here, and that is the point of the seam: the
// package's whole contract is five calls and one method that accepts a closure
// Job, and everything the control plane does with them lives on the other side.

// recordingHostJobs is a HostJobs implementation that records what it is asked.
type recordingHostJobs struct {
	mu       sync.Mutex
	starts   []closureStart
	ref      *HostJobRef
	startErr error
}

type closureStart struct {
	PluginName  string
	Label       string
	ActorUserID uint
	ParentJobID string
}

func (h *recordingHostJobs) StartClosureJob(pluginName, label string, actorUserID uint, parentJobID string) (*HostJobRef, error) {
	h.mu.Lock()
	h.starts = append(h.starts, closureStart{pluginName, label, actorUserID, parentJobID})
	h.mu.Unlock()
	if h.startErr != nil {
		return nil, h.startErr
	}
	return h.ref, nil
}

func (h *recordingHostJobs) closures() []closureStart {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]closureStart(nil), h.starts...)
}

// recordingSink records the reports one execution makes.
type recordingSink struct {
	mu        sync.Mutex
	started   int
	completed int
	failed    int
	lost      []string
	progress  int
	message   string
}

func (s *recordingSink) Started(string) { s.bump(&s.started) }
func (s *recordingSink) Progress(int, string) {
	s.mu.Lock()
	s.progress++
	s.mu.Unlock()
}
func (s *recordingSink) Completed(message string, _ map[string]any) {
	s.mu.Lock()
	s.completed++
	s.message = message
	s.mu.Unlock()
}
func (s *recordingSink) Failed(string) { s.bump(&s.failed) }
func (s *recordingSink) CallbackLost(reason string) {
	s.mu.Lock()
	s.lost = append(s.lost, reason)
	s.mu.Unlock()
}

func (s *recordingSink) bump(field *int) {
	s.mu.Lock()
	*field++
	s.mu.Unlock()
}

func (s *recordingSink) counts() (int, int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.started, s.completed, s.failed, s.progress
}

// TestAnAsyncActionReportsIntoItsHostJob is the seam's primary contract: the
// execution the host accepted is the one the plugin's own reports land on, and
// the id the in-memory projection answers to is the handle the host minted.
func TestAnAsyncActionReportsIntoItsHostJob(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "host-plugin", `
plugin = { name = "host-plugin", version = "1.0", api_version = 1, capabilities = { "actions", "jobs" } }

function work(ctx)
    mah.job_progress(ctx.job_id, 40, "working")
    mah.job_complete(ctx.job_id, { message = "all done" })
end

function init()
    mah.action({ id = "work", label = "Work", entity = "resource", async = true, handler = work })
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("plugin manager: %v", err)
	}
	defer pm.Close()

	sink := &recordingSink{}
	pm.SetHostJobs(&recordingHostJobs{ref: &HostJobRef{JobID: "job-1", Handle: "handle-1", Sink: sink}})
	if err := pm.EnablePlugin("host-plugin"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	jobID, err := pm.RunActionAsyncForHost(
		&HostJobRef{JobID: "job-1", Handle: "handle-1", Sink: sink},
		nil, "host-plugin", "work", 5, nil, "")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if jobID != "handle-1" {
		t.Fatalf("the execution is listed as %q, want the host's handle", jobID)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		started, completed, failed, progress := sink.counts()
		if completed > 0 {
			if started == 0 {
				t.Fatalf("the host was told the job completed before it started")
			}
			if failed != 0 {
				t.Fatalf("a successful action reported %d failures", failed)
			}
			if progress == 0 {
				t.Fatal("the host was told nothing about the plugin's progress")
			}
			if sink.message != "all done" {
				t.Fatalf("the completion message is %q, want the plugin's own", sink.message)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("the action never reported its completion to the host job")
}

// TestStartJobAcceptsAHostJobAndNamesItsParent pins the two facts a nested
// mah.start_job has to carry: the host accepts the Job before the closure runs,
// and the Job it names as the parent is the execution that asked for it.
func TestStartJobAcceptsAHostJobAndNamesItsParent(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "nested-plugin", `
plugin = { name = "nested-plugin", version = "1.0", api_version = 1, capabilities = { "actions", "jobs" } }

function work(ctx)
    mah.start_job("child work", function(job_id) mah.job_complete(job_id, { message = "child done" }) end)
    mah.job_complete(ctx.job_id, { message = "parent done" })
end

function init()
    mah.action({ id = "work", label = "Work", entity = "resource", async = true, handler = work })
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("plugin manager: %v", err)
	}
	defer pm.Close()

	childSink := &recordingSink{}
	hosts := &recordingHostJobs{ref: &HostJobRef{JobID: "child-job", Handle: "child-handle", Sink: childSink}}
	pm.SetHostJobs(hosts)
	if err := pm.EnablePlugin("nested-plugin"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	parentSink := &recordingSink{}
	if _, err := pm.RunActionAsyncForHost(
		&HostJobRef{JobID: "parent-job", Handle: "parent-handle", Sink: parentSink},
		nil, "nested-plugin", "work", 5, nil, ""); err != nil {
		t.Fatalf("run the parent: %v", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if closures := hosts.closures(); len(closures) > 0 {
			if closures[0].ParentJobID != "parent-job" {
				t.Fatalf("the child job records parent %q, want the action that started it", closures[0].ParentJobID)
			}
			if closures[0].Label != "child work" {
				t.Fatalf("the child job is labelled %q, want the plugin's own label", closures[0].Label)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("mah.start_job accepted no host job")
}

// TestClosingTheManagerReportsEveryLostCallback is the graceful-shutdown proof:
// stopping the VMs is a fact this process establishes about its own callbacks,
// and every execution still in flight has to be told that it can never finish.
func TestClosingTheManagerReportsEveryLostCallback(t *testing.T) {
	dir := t.TempDir()
	writePlugin(t, dir, "slow-plugin", `
plugin = { name = "slow-plugin", version = "1.0", api_version = 1, capabilities = { "actions" } }

function work(ctx)
    mah.sleep(0.5)
    mah.job_complete(ctx.job_id, { message = "done" })
end

function init()
    mah.action({ id = "work", label = "Work", entity = "resource", async = true, handler = work })
end
`)
	pm, err := NewPluginManager(dir)
	if err != nil {
		t.Fatalf("plugin manager: %v", err)
	}
	if err := pm.EnablePlugin("slow-plugin"); err != nil {
		t.Fatalf("enable: %v", err)
	}

	sink := &recordingSink{}
	if _, err := pm.RunActionAsyncForHost(
		&HostJobRef{JobID: "slow-job", Handle: "slow-handle", Sink: sink},
		nil, "slow-plugin", "work", 5, nil, ""); err != nil {
		t.Fatalf("run: %v", err)
	}
	// The handler sleeps, so it is certainly still running when the manager is
	// closed: this is the state a deployment is in when it stops with work in
	// flight, not a race.
	pm.Close()

	sink.mu.Lock()
	lost := append([]string(nil), sink.lost...)
	sink.mu.Unlock()
	if len(lost) != 1 {
		t.Fatalf("the host was told about %d lost callbacks, want 1", len(lost))
	}
	if lost[0] != "plugin-runtime-stopping" {
		t.Fatalf("the loss reason is %q, want the shutdown reason", lost[0])
	}
}

// TestRuntimeIdentityLiveness_isConservative pins what can be proved about a
// process from its recorded identity, because every answer here decides whether
// non-restorable work is interrupted or left blocked for a person.
func TestRuntimeIdentityLiveness_isConservative(t *testing.T) {
	current := CurrentRuntimeIdentity()
	if current.Host == "" {
		t.Skip("this host has no name to compare against")
	}

	cases := []struct {
		name     string
		identity RuntimeIdentity
		want     RuntimeLiveness
	}{
		{"this process", current, RuntimeAlive},
		{"another host", RuntimeIdentity{Host: current.Host + "-elsewhere", BootSession: current.BootSession, PID: current.PID}, RuntimeUnknown},
		{"a boot that ended", RuntimeIdentity{Host: current.Host, BootSession: current.BootSession + "-old", PID: current.PID}, RuntimeGone},
		{"no boot session recorded", RuntimeIdentity{Host: current.Host, PID: current.PID}, RuntimeUnknown},
	}
	for _, tc := range cases {
		if got := tc.identity.Liveness(); got != tc.want {
			t.Errorf("%s: liveness = %v, want %v", tc.name, got, tc.want)
		}
	}

	// A pid that cannot exist in this boot is gone; one that is certainly taken
	// (this process's own) is alive.
	gone := RuntimeIdentity{Host: current.Host, BootSession: current.BootSession, PID: 1 << 30}
	if got := gone.Liveness(); got != RuntimeGone {
		t.Errorf("a pid no process holds: liveness = %v, want gone", got)
	}
}

func TestRuntimeIdentityRoundTrips(t *testing.T) {
	identity := RuntimeIdentity{Host: "host-a", BootSession: "boot-b", PID: 4242}
	parsed, ok := ParseRuntimeIdentity(identity.String())
	if !ok || parsed != identity {
		t.Fatalf("round trip of %q gave %+v ok=%v", identity.String(), parsed, ok)
	}
	for _, bad := range []string{"", "host", "host/boot", "host/boot/notanumber", "host/boot/0", "/boot/1"} {
		if _, ok := ParseRuntimeIdentity(bad); ok {
			t.Errorf("ParseRuntimeIdentity(%q) was accepted", bad)
		}
	}
	pid, err := strconv.Atoi(strconv.Itoa(os.Getpid()))
	if err != nil || pid != os.Getpid() {
		t.Fatal("pid sanity check failed")
	}
}
