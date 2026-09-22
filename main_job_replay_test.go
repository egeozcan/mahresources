package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mahresources/constants"
	"mahresources/jobs"
)

// TestJobReplayKeyConfigFollowsTheDeployment pins the boot facts the keyring is
// loaded from. They differ per deployment for reasons that only make sense
// together: PostgreSQL may have several processes and hosts writing one
// database, an in-memory database has nothing that outlives the process, and a
// persistent single-process deployment keeps a private key file beside its data.
func TestJobReplayKeyConfigFollowsTheDeployment(t *testing.T) {
	t.Setenv("JOB_REPLAY_KEY", base64ReplayKey())

	persistent := jobReplayKeyConfig("/srv/mahresources/files", constants.DbTypeSqlite, false)
	if persistent.Dialect != constants.DbTypeSqlite || persistent.Ephemeral {
		t.Errorf("persistent sqlite config = %+v", persistent)
	}
	if persistent.KeyFilePath != filepath.Join("/srv/mahresources/files", jobs.JobReplayKeyFileName) {
		t.Errorf("key file path = %q, want it under the data root", persistent.KeyFilePath)
	}
	if persistent.Keys != base64ReplayKey() {
		t.Errorf("keys = %q, want the environment's value", persistent.Keys)
	}

	// A deployment with no data root has nowhere to keep a private key file,
	// which is a refusal rather than a per-boot key: the database outlives the
	// process even when the filesystem does not.
	rootless := jobReplayKeyConfig("", constants.DbTypeSqlite, false)
	if rootless.KeyFilePath != "" {
		t.Errorf("key file path = %q, want none for a deployment with no data root", rootless.KeyFilePath)
	}
	if _, err := jobs.LoadReplayKeyring(jobs.ReplayKeyConfig{Dialect: constants.DbTypeSqlite}); !errors.Is(err, jobs.ErrReplayKeyRequired) {
		t.Errorf("loading without a data root = %v, want ErrReplayKeyRequired", err)
	}

	t.Setenv("JOB_REPLAY_KEY", "")
	inMemory := jobReplayKeyConfig("", constants.DbTypeSqlite, true)
	if !inMemory.Ephemeral {
		t.Error("an in-memory database must be reported as ephemeral")
	}
	if _, err := jobs.LoadReplayKeyring(inMemory); err != nil {
		t.Errorf("an in-memory deployment may generate its own key: %v", err)
	}
}

// TestJobReplayKeyIsRequiredForPostgresBeforeTheContextIsBuilt is the startup
// refusal, driven through the configuration main.go actually derives rather than
// through a hand-built struct: a PostgreSQL deployment that could accept durable
// secret work with no key it will still hold refuses to boot, and the same
// deployment configured properly boots.
func TestJobReplayKeyIsRequiredForPostgresBeforeTheContextIsBuilt(t *testing.T) {
	t.Setenv("JOB_REPLAY_KEY", "")

	cfg := jobReplayKeyConfig("/srv/mahresources/files", constants.DbTypePosgres, false)
	if _, err := jobs.LoadReplayKeyring(cfg); !errors.Is(err, jobs.ErrReplayKeyRequired) {
		t.Fatalf("postgres without a key = %v, want ErrReplayKeyRequired", err)
	}

	// The message has to name the fix: an operator meets this only at boot.
	if _, err := jobs.LoadReplayKeyring(cfg); err == nil || !strings.Contains(err.Error(), "JOB_REPLAY_KEY") {
		t.Fatalf("refusal = %v, want it to name JOB_REPLAY_KEY", err)
	}

	key := base64ReplayKey()
	t.Setenv("JOB_REPLAY_KEY", key)
	ring, err := jobs.LoadReplayKeyring(jobReplayKeyConfig("/srv/mahresources/files", constants.DbTypePosgres, false))
	if err != nil {
		t.Fatalf("postgres with a key: %v", err)
	}
	if ring.ActiveKeyID() == "" {
		t.Fatal("a configured keyring has no active key")
	}
}

// TestJobReplayKeyFileIsCreatedUnderTheDataRoot proves the persistent
// single-process path main.go points at is a real, private file, and that a
// restart reads the same key rather than minting a new one.
func TestJobReplayKeyFileIsCreatedUnderTheDataRoot(t *testing.T) {
	t.Setenv("JOB_REPLAY_KEY", "")
	root := t.TempDir()

	first, err := jobs.LoadReplayKeyring(jobReplayKeyConfig(root, constants.DbTypeSqlite, false))
	if err != nil {
		t.Fatalf("first boot: %v", err)
	}
	path := filepath.Join(root, jobs.JobReplayKeyFileName)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %v, want 0600", info.Mode().Perm())
	}

	second, err := jobs.LoadReplayKeyring(jobReplayKeyConfig(root, constants.DbTypeSqlite, false))
	if err != nil {
		t.Fatalf("second boot: %v", err)
	}
	if second.ActiveKeyID() != first.ActiveKeyID() {
		t.Fatalf("a restart minted a different key: %s then %s", first.ActiveKeyID(), second.ActiveKeyID())
	}
}

// base64ReplayKey is a fixed 32-byte key in the JOB_REPLAY_KEY grammar. It is a
// test value, and it is spelled here rather than generated so the test asserts
// the grammar an operator writes.
func base64ReplayKey() string {
	// 32 bytes, base64: "mahresources-replay-key-32bytes" padded to length.
	return "bWFocmVzb3VyY2VzLXJlcGxheS1rZXktMzJieXRlcyE="
}
