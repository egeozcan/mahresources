package application_context

import (
	"testing"
	"time"

	"mahresources/jobs"
)

// The replay retention is the one replay setting that is runtime-editable: the
// key is not, because a key an administrator could change from /admin/settings
// is a key that can render every stored envelope unreadable in one click. These
// tests pin both halves of that split at the accessor the Job control plane's
// facade reads.

// TestJobReplayRetentionDefaultsAndHonoursOverrides proves the accessor answers
// from the live settings, and that a context with no settings service or a
// missing value still answers with the seven-day default rather than zero — for
// execution-required input, a published 0 would read as "expire on write".
func TestJobReplayRetentionDefaultsAndHonoursOverrides(t *testing.T) {
	if defaultJobReplayRetention != 168*time.Hour {
		t.Fatalf("default replay retention = %v, want seven days", defaultJobReplayRetention)
	}

	// A bare context: every api_test and programmatic embed looks like this.
	bare := &MahresourcesContext{}
	if got := bare.JobReplayRetention(); got != defaultJobReplayRetention {
		t.Fatalf("retention without settings = %v, want %v", got, defaultJobReplayRetention)
	}

	// A settings service that has never been given the key is the same answer,
	// not a panic: getRaw returns no value rather than a typed zero.
	db := newTestDB(t)
	partial := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), map[string]any{})
	if err := partial.Load(); err != nil {
		t.Fatalf("load settings: %v", err)
	}
	withoutKey := &MahresourcesContext{settings: partial}
	if got := withoutKey.JobReplayRetention(); got != defaultJobReplayRetention {
		t.Fatalf("retention without a value = %v, want %v", got, defaultJobReplayRetention)
	}

	// And an operator's override wins, without a restart.
	settings := NewRuntimeSettings(db, &stubLogger{}, buildSpecs(), BuildDefaultsFromConfig(&MahresourcesConfig{}))
	if err := settings.Load(); err != nil {
		t.Fatalf("load settings: %v", err)
	}
	withKey := &MahresourcesContext{settings: settings}
	if got := withKey.JobReplayRetention(); got != defaultJobReplayRetention {
		t.Fatalf("boot default = %v, want %v", got, defaultJobReplayRetention)
	}
	if err := settings.Set(KeyJobReplayRetention, (48 * time.Hour).String(), "shorter window", "127.0.0.1"); err != nil {
		t.Fatalf("set %s: %v", KeyJobReplayRetention, err)
	}
	if got := withKey.JobReplayRetention(); got != 48*time.Hour {
		t.Fatalf("retention after the override = %v, want 48h", got)
	}

	// The key itself is deliberately absent from the registry, so no runtime
	// write can reach it.
	if err := settings.Set("job_replay_key", "anything", "try it", "127.0.0.1"); err == nil {
		t.Fatal("the replay key must not be a runtime-editable setting")
	}
}

// TestJobReplayKeyringIsInstalledOnceAndSharedByEveryContextCopy proves the
// accessor is the seam the facade reads, and that the shallow copies
// WithPrincipal/WithRequest make all see the installed keyring — a copy that
// lost it would silently start refusing to seal on a scoped request.
func TestJobReplayKeyringIsInstalledOnceAndSharedByEveryContextCopy(t *testing.T) {
	ring, err := jobs.LoadReplayKeyring(jobs.ReplayKeyConfig{
		Dialect:   "SQLITE",
		Ephemeral: true,
		Keys:      "bWFocmVzb3VyY2VzLXJlcGxheS1rZXktMzJieXRlcyE=",
	})
	if err != nil {
		t.Fatalf("build keyring: %v", err)
	}

	ctx := &MahresourcesContext{}
	if ctx.JobReplayKeyring() != nil {
		t.Fatal("a context that was never given a keyring must hold none")
	}
	ctx.SetJobReplayKeyring(ring)
	if ctx.JobReplayKeyring() != ring {
		t.Fatal("the installed keyring is not the one the accessor returns")
	}

	// WithPrincipal/WithRequest/WithTransaction all shallow-copy the struct, which
	// is what makes the keyring reach a scoped request; a copy that built its own
	// would refuse to seal there.
	copied := *ctx
	if copied.JobReplayKeyring() != ring {
		t.Fatal("a copy of the context lost the keyring")
	}
}
