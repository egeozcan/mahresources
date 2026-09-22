package jobs

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mahresources/constants"
	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
)

// The exact values a replay fixture must never leak: the query string of a URL,
// a Cookie, an Authorization header, and a value a plugin supplied. They are
// deliberately not secret-shaped (no random-looking uppercase runs) so an
// assertion that scans for them cannot pass by accident.
const (
	fixtureQuerySecret  = "url-query-secret-8f1c"
	fixtureCookieSecret = "session-cookie-secret-2b7d"
	fixtureAuthSecret   = "bearer-token-secret-5a3e"
	fixturePluginSecret = "plugin-secret-value-9c4f"
)

// fixtureReplayInput is one Kind's validated input, carrying every shape of
// secret the Red list names in one payload.
func fixtureReplayInput() json.RawMessage {
	return json.RawMessage(`{
  "url": "https://files.example.test/media/clip.mp4?token=` + fixtureQuerySecret + `",
  "headers": {"Cookie": "sid=` + fixtureCookieSecret + `", "Authorization": "Bearer ` + fixtureAuthSecret + `"},
  "pluginSecret": "` + fixturePluginSecret + `"
}`)
}

// fixtureReplayCodec is a Kind owning its own redaction and encoding, which is
// the only place a Kind's input may become searchable text.
func fixtureReplayCodec() ReplayCodec {
	return ReplayCodec{
		Sanitize: func(input json.RawMessage) (json.RawMessage, error) {
			var parsed struct {
				URL     string            `json:"url"`
				Headers map[string]string `json:"headers"`
			}
			if err := json.Unmarshal(input, &parsed); err != nil {
				return nil, err
			}
			parsedURL, err := url.Parse(parsed.URL)
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]any{
				"scheme":  parsedURL.Scheme,
				"host":    parsedURL.Host,
				"name":    path.Base(parsedURL.Path),
				"headers": len(parsed.Headers),
			})
		},
		Encode: func(input json.RawMessage) (json.RawMessage, error) {
			return input, nil
		},
		Decode: func(payload json.RawMessage, kindVersion uint) (json.RawMessage, error) {
			return payload, nil
		},
		Migrate: func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			return payload, nil
		},
	}
}

// newReplayDeps opens the file-backed SQLite database the replay tests write
// envelopes into and reports its path, so a test can also assert what is *not*
// in the bytes on disk.
func newReplayDeps(t *testing.T) (Deps, string) {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "jobs.db")
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, dsn, "", 0)
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("underlying database: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := db.AutoMigrate(jobCoreTables()...); err != nil {
		t.Fatalf("migrate job core: %v", err)
	}
	return Deps{DB: db}, dsn
}

// replayKeyFromSeed derives a deterministic 32-byte key from a readable seed,
// so an assertion about which key sealed an envelope is about that key rather
// than about a random value a test cannot name.
func replayKeyFromSeed(t *testing.T, seed string) ReplayKey {
	t.Helper()
	sum := sha256.Sum256([]byte(seed))
	key, err := NewReplayKey(sum[:])
	if err != nil {
		t.Fatalf("build key from seed %q: %v", seed, err)
	}
	return key
}

// replayKeyringFromSeeds builds a keyring whose active key is the first seed.
func replayKeyringFromSeeds(t *testing.T, seeds ...string) *Keyring {
	t.Helper()
	keys := make([]ReplayKey, 0, len(seeds))
	for _, seed := range seeds {
		keys = append(keys, replayKeyFromSeed(t, seed))
	}
	ring, err := NewKeyring(keys...)
	if err != nil {
		t.Fatalf("build keyring: %v", err)
	}
	return ring
}

// fixtureReplaySecrets is every value the assertions scan for.
func fixtureReplaySecrets() []string {
	return []string{fixtureQuerySecret, fixtureCookieSecret, fixtureAuthSecret, fixturePluginSecret}
}

func requireSecretsAbsent(t *testing.T, what string, blob string) {
	t.Helper()
	for _, secret := range fixtureReplaySecrets() {
		if strings.Contains(blob, secret) {
			t.Errorf("%s contains the exact secret %q", what, secret)
		}
	}
}

// requireSecretsAbsentFromDisk asserts the database file and the SQLite
// write-ahead log hold no plaintext replay value. Anchoring on the file rather
// than on a stored column is the point: it catches a value that reached the
// database through some path the test did not think to read.
func requireSecretsAbsentFromDisk(t *testing.T, dsn string) {
	t.Helper()
	for _, suffix := range []string{"", "-wal", "-journal"} {
		data, err := os.ReadFile(dsn + suffix)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("read %s%s: %v", dsn, suffix, err)
		}
		requireSecretsAbsent(t, fmt.Sprintf("%s%s", filepath.Base(dsn), suffix), string(data))
	}
}

// TestReplayAcceptSealsInputAndPublishesOnlyTheSanitizedSummary is the redaction
// seam: what a Kind hands over is stored as ciphertext, and the only text that
// leaves acceptance is the summary the Kind's own sanitizer produced. The
// fixture carries one of every secret shape the contract names, so a snapshot,
// an event, the stored row or the bytes on disk carrying any of them is a leak
// rather than a formatting difference.
func TestReplayAcceptSealsInputAndPublishesOnlyTheSanitizedSummary(t *testing.T) {
	deps, dsn := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	ring := replayKeyringFromSeeds(t, "primary-key-material")
	deps.Replay = &ReplayConfig{Keys: ring}

	accepted := time.Date(2032, 1, 2, 3, 4, 5, 0, time.UTC)
	deps.Now = func() time.Time { return accepted }

	snap, err := svc.Accept(deps, Acceptance{
		Kind:        "remote-download",
		KindVersion: 1,
		State:       StateQueued,
		Origin:      "api",
		Title:       "clip.mp4",
		Replay:      ReplayInput{Input: fixtureReplayInput()},
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	if snap.ReplayClass != ReplayClassReplayable {
		t.Errorf("replay class = %q, want %q", snap.ReplayClass, ReplayClassReplayable)
	}
	if snap.ReplayAvailability != ReplayAvailable {
		t.Errorf("replay availability = %q, want %q", snap.ReplayAvailability, ReplayAvailable)
	}

	var summary map[string]any
	if err := json.Unmarshal(snap.Summary, &summary); err != nil {
		t.Fatalf("accepted summary is not JSON: %v (%s)", err, snap.Summary)
	}
	if summary["host"] != "files.example.test" || summary["name"] != "clip.mp4" {
		t.Errorf("summary = %s, want the sanitizer's own output", snap.Summary)
	}
	if _, present := summary["url"]; present {
		t.Errorf("the summary carries the raw URL: %s", snap.Summary)
	}

	// The sealed row: bound to the Job, Kind and version, and never the input.
	envelope := replayEnvelopeRow(t, deps, snap.ID)
	if envelope.Kind != "remote-download" || envelope.KindVersion != 1 {
		t.Errorf("envelope kind = %s v%d, want remote-download v1", envelope.Kind, envelope.KindVersion)
	}
	if envelope.SchemaVersion != ReplayEnvelopeSchemaVersion {
		t.Errorf("envelope schema version = %d, want %d", envelope.SchemaVersion, ReplayEnvelopeSchemaVersion)
	}
	if envelope.KeyID != ring.ActiveKeyID() {
		t.Errorf("envelope key id = %q, want the active %q", envelope.KeyID, ring.ActiveKeyID())
	}
	if len(envelope.Ciphertext) == 0 || len(envelope.Nonce) == 0 {
		t.Fatalf("envelope is not sealed: %+v", envelope)
	}
	requireSecretsAbsent(t, "the sealed ciphertext", string(envelope.Ciphertext))
	for _, secret := range fixtureReplaySecrets() {
		if strings.Contains(string(envelope.Ciphertext), secret) {
			t.Fatalf("the ciphertext contains %q", secret)
		}
	}
	if envelope.ExpiresAt != nil {
		t.Errorf("a queued Job's envelope must not carry an expiry, got %v", envelope.ExpiresAt)
	}

	// Every ordinary view of the Job, and the bytes on disk.
	requireSecretsAbsent(t, "the accepted snapshot", fmt.Sprintf("%+v", snap))
	requireSecretsAbsent(t, "the accepted snapshot JSON", mustJSON(t, snap))
	requireSecretsAbsent(t, "the stored Job row", mustJSON(t, jobRow(t, deps, snap.ID)))
	requireSecretsAbsent(t, "the stored envelope row", mustJSON(t, envelope))
	for _, event := range jobEvents(t, deps, snap.ID) {
		requireSecretsAbsent(t, "a Job event", mustJSON(t, event))
	}
	requireSecretsAbsentFromDisk(t, dsn)
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	return string(encoded)
}

// replayEnvelopeRow reads the stored envelope directly: what is on disk is the
// assertion, not what the Service's own reads happen to expose.
func replayEnvelopeRow(t *testing.T, deps Deps, jobID string) models.JobReplayEnvelope {
	t.Helper()
	var envelope models.JobReplayEnvelope
	if err := deps.DB.Where("job_id = ?", jobID).First(&envelope).Error; err != nil {
		t.Fatalf("load replay envelope for %s: %v", jobID, err)
	}
	return envelope
}

// acceptFixtureReplayJob accepts one replayable Job with the fixture input and
// returns its snapshot. Every test below starts from the same acceptance so the
// difference under assertion is the one the test makes.
func acceptFixtureReplayJob(t *testing.T, svc *Service, deps Deps) Snapshot {
	t.Helper()
	snap, err := svc.Accept(deps, Acceptance{
		Kind:        "remote-download",
		KindVersion: 1,
		State:       StateQueued,
		Origin:      "api",
		Title:       "clip.mp4",
		Replay:      ReplayInput{Input: fixtureReplayInput()},
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}
	return snap
}

// TestReplayOpenReturnsTheInputTheJobWasAcceptedWith is the round trip: the
// sealed bytes are the Kind's own input again, at the version it was written
// for, with no migration involved.
func TestReplayOpenReturnsTheInputTheJobWasAcceptedWith(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}

	snap := acceptFixtureReplayJob(t, svc, deps)

	opened, err := svc.OpenReplay(deps, Access{Administrator: true}, snap.ID)
	if err != nil {
		t.Fatalf("OpenReplay: %v", err)
	}
	if opened.JobID != snap.ID || opened.Kind != "remote-download" || opened.KindVersion != 1 {
		t.Errorf("opened = %+v, want the accepted Job's own identity", opened)
	}
	if opened.SchemaVersion != ReplayEnvelopeSchemaVersion {
		t.Errorf("opened schema version = %d, want %d", opened.SchemaVersion, ReplayEnvelopeSchemaVersion)
	}
	if opened.MigratedFrom != nil {
		t.Errorf("nothing should have been migrated, got from v%d", *opened.MigratedFrom)
	}
	var got, want any
	if err := json.Unmarshal(opened.Input, &got); err != nil {
		t.Fatalf("opened input is not JSON: %v (%s)", err, opened.Input)
	}
	if err := json.Unmarshal(fixtureReplayInput(), &want); err != nil {
		t.Fatalf("fixture input is not JSON: %v", err)
	}
	if mustJSON(t, got) != mustJSON(t, want) {
		t.Errorf("opened input = %s, want %s", opened.Input, fixtureReplayInput())
	}
}

// TestReplayOpenRefusesTamperedMovedAndRekindedEnvelopes is the binding the
// associated data buys: ciphertext is readable only as the exact Job, Kind and
// Kind version it was sealed for.
func TestReplayOpenRefusesTamperedMovedAndRekindedEnvelopes(t *testing.T) {
	cases := []struct {
		name string
		// tamper alters the stored rows and returns the Job whose envelope is
		// then opened. It always opens a Job that was accepted with input, so a
		// failure is authentication rather than absence.
		tamper func(t *testing.T, deps Deps, svc *Service, jobID string) string
	}{
		{
			name: "a flipped ciphertext bit",
			tamper: func(t *testing.T, deps Deps, _ *Service, jobID string) string {
				envelope := replayEnvelopeRow(t, deps, jobID)
				tampered := append([]byte(nil), envelope.Ciphertext...)
				tampered[0] ^= 0x01
				if err := deps.DB.Model(&models.JobReplayEnvelope{}).
					Where("job_id = ?", jobID).Update("ciphertext", tampered).Error; err != nil {
					t.Fatalf("tamper ciphertext: %v", err)
				}
				return jobID
			},
		},
		{
			name: "a replaced nonce",
			tamper: func(t *testing.T, deps Deps, _ *Service, jobID string) string {
				envelope := replayEnvelopeRow(t, deps, jobID)
				nonce := append([]byte(nil), envelope.Nonce...)
				nonce[0] ^= 0xff
				if err := deps.DB.Model(&models.JobReplayEnvelope{}).
					Where("job_id = ?", jobID).Update("nonce", nonce).Error; err != nil {
					t.Fatalf("replace nonce: %v", err)
				}
				return jobID
			},
		},
		{
			name: "a row moved to another Job",
			tamper: func(t *testing.T, deps Deps, svc *Service, jobID string) string {
				other := acceptFixtureReplayJob(t, svc, deps)
				if err := deps.DB.Where("job_id = ?", other.ID).
					Delete(&models.JobReplayEnvelope{}).Error; err != nil {
					t.Fatalf("clear the other Job's envelope: %v", err)
				}
				if err := deps.DB.Model(&models.JobReplayEnvelope{}).
					Where("job_id = ?", jobID).Update("job_id", other.ID).Error; err != nil {
					t.Fatalf("move envelope: %v", err)
				}
				return other.ID
			},
		},
		{
			name: "a rewritten Kind version",
			tamper: func(t *testing.T, deps Deps, _ *Service, jobID string) string {
				if err := deps.DB.Model(&models.JobReplayEnvelope{}).
					Where("job_id = ?", jobID).Update("kind_version", 2).Error; err != nil {
					t.Fatalf("rewrite kind version: %v", err)
				}
				return jobID
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			deps, _ := newReplayDeps(t)
			svc := NewService()
			if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
				t.Fatalf("register codec: %v", err)
			}
			deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}
			snap := acceptFixtureReplayJob(t, svc, deps)

			openID := testCase.tamper(t, deps, svc, snap.ID)

			if _, err := svc.OpenReplay(deps, Access{Administrator: true}, openID); !errors.Is(err, ErrReplayCorrupt) {
				t.Fatalf("OpenReplay of a tampered envelope = %v, want ErrReplayCorrupt", err)
			}
		})
	}
}

// TestReplayOpenRefusesAnEnvelopeWhoseKeyThisProcessDoesNotHold proves the two
// failures stay distinguishable: a process that simply does not hold the key is
// told that, rather than being told the bytes are corrupt.
func TestReplayOpenRefusesAnEnvelopeWhoseKeyThisProcessDoesNotHold(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}
	snap := acceptFixtureReplayJob(t, svc, deps)

	otherProcess := deps
	otherProcess.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "someone-elses-key")}

	if _, err := svc.OpenReplay(otherProcess, Access{Administrator: true}, snap.ID); !errors.Is(err, ErrReplayKeyUnavailable) {
		t.Fatalf("OpenReplay with the wrong keyring = %v, want ErrReplayKeyUnavailable", err)
	}
	if snapshot := svc.snapshotFor(otherProcess, Access{Administrator: true}, jobRow(t, deps, snap.ID)); snapshot.ReplayAvailability != ReplayUnreadable {
		t.Fatalf("availability without the key = %q, want %q", snapshot.ReplayAvailability, ReplayUnreadable)
	}
}

// TestReplayOpenHonoursVisibility keeps the hidden/missing indistinguishability
// the rest of the module has: an unreadable envelope is no more discoverable
// than an unreadable Job.
func TestReplayOpenHonoursVisibility(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}

	snap, err := svc.Accept(deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(41), ActorUserID: uintPtr(41),
		Replay: ReplayInput{Input: fixtureReplayInput()},
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	if _, err := svc.OpenReplay(deps, Access{UserID: 42}, snap.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("OpenReplay for a non-owner = %v, want ErrNotFound", err)
	}
	if _, err := svc.OpenReplay(deps, Access{UserID: 41}, snap.ID); err != nil {
		t.Fatalf("OpenReplay for the owner: %v", err)
	}
	if _, err := svc.OpenReplay(deps, Access{UserID: 41}, "0193f0a0-0000-7000-8000-000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("OpenReplay for an unknown Job = %v, want ErrNotFound", err)
	}
}

// TestReplayRotationReadsOldEnvelopesAndSealsWithTheActiveKey is the rotation
// contract: the key that seals is the first of the list, and every key after it
// is there so history stays readable.
func TestReplayRotationReadsOldEnvelopesAndSealsWithTheActiveKey(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	before := replayKeyringFromSeeds(t, "primary-key-material")
	deps.Replay = &ReplayConfig{Keys: before}
	old := acceptFixtureReplayJob(t, svc, deps)

	rotated := replayKeyringFromSeeds(t, "the-new-active-key", "primary-key-material")
	if rotated.ActiveKeyID() == before.ActiveKeyID() {
		t.Fatal("the rotation test did not actually change the active key")
	}
	after := deps
	after.Replay = &ReplayConfig{Keys: rotated}

	if _, err := svc.OpenReplay(after, Access{Administrator: true}, old.ID); err != nil {
		t.Fatalf("OpenReplay of an envelope sealed by the retired key: %v", err)
	}
	fresh := acceptFixtureReplayJob(t, svc, after)
	if got := replayEnvelopeRow(t, deps, fresh.ID).KeyID; got != rotated.ActiveKeyID() {
		t.Fatalf("a new envelope was sealed with %q, want the active key %q", got, rotated.ActiveKeyID())
	}
	if got := replayEnvelopeRow(t, deps, old.ID).KeyID; got != before.ActiveKeyID() {
		t.Fatalf("the old envelope's key id changed to %q", got)
	}
}

// base64Key encodes 32 bytes as the JOB_REPLAY_KEY grammar spells them.
func base64Key(seed byte) string {
	raw := make([]byte, replayKeyBytes)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// TestReplayKeysParseTheEnvironmentGrammar covers the one input an operator
// writes by hand: one or more comma-separated base64-encoded 32-byte keys, most
// recent first, with the first active and the rest kept for reading history.
func TestReplayKeysParseTheEnvironmentGrammar(t *testing.T) {
	parsed, err := ParseReplayKeys(" " + base64Key(1) + "," + base64Key(9) + " ")
	if err != nil {
		t.Fatalf("ParseReplayKeys: %v", err)
	}
	if len(parsed) != 2 {
		t.Fatalf("parsed %d keys, want 2", len(parsed))
	}
	ring, err := NewKeyring(parsed...)
	if err != nil {
		t.Fatalf("NewKeyring: %v", err)
	}
	if ring.ActiveKeyID() != parsed[0].ID {
		t.Errorf("active key = %q, want the first listed %q", ring.ActiveKeyID(), parsed[0].ID)
	}
	if !ring.HasKey(parsed[1].ID) {
		t.Errorf("the second key is not available for reading old envelopes")
	}

	// Every malformed shape is refused by name rather than silently skipped: a
	// typo in a rotation list must not mean "that key is simply not there",
	// which is discovered when a Retry fails months later.
	shortKey := base64.StdEncoding.EncodeToString([]byte("too short"))
	for _, bad := range []string{
		"not base64!",
		shortKey,
		base64Key(1) + "," + base64Key(1),
		base64Key(1) + ", ",
		",",
	} {
		if _, err := ParseReplayKeys(bad); !errors.Is(err, ErrInvalidReplayKey) {
			t.Errorf("ParseReplayKeys(%q) = %v, want ErrInvalidReplayKey", bad, err)
		}
	}
}

// TestReplayKeyringRefusesAPostgresDeploymentWithoutAKey is the startup refusal
// the contract turns on: PostgreSQL may have several processes and hosts writing
// one database, so a key this process generated is a key the next one will not
// have.
func TestReplayKeyringRefusesAPostgresDeploymentWithoutAKey(t *testing.T) {
	if _, err := LoadReplayKeyring(ReplayKeyConfig{Dialect: constants.DbTypePosgres}); !errors.Is(err, ErrReplayKeyRequired) {
		t.Fatalf("postgres without a key = %v, want ErrReplayKeyRequired", err)
	}
	if _, err := LoadReplayKeyring(ReplayKeyConfig{Dialect: constants.DbTypePosgres, Keys: base64Key(3)}); err != nil {
		t.Fatalf("postgres with an explicit key: %v", err)
	}

	// A persistent deployment with nowhere to keep a private key file is the
	// same refusal, for the same reason.
	if _, err := LoadReplayKeyring(ReplayKeyConfig{Dialect: constants.DbTypeSqlite}); !errors.Is(err, ErrReplayKeyRequired) {
		t.Fatalf("persistent sqlite with no data root = %v, want ErrReplayKeyRequired", err)
	}

	// An in-memory database is the one deployment that may generate a key of its
	// own: its envelopes die with the process that wrote them.
	first, err := LoadReplayKeyring(ReplayKeyConfig{Dialect: constants.DbTypeSqlite, Ephemeral: true})
	if err != nil {
		t.Fatalf("ephemeral keyring: %v", err)
	}
	second, err := LoadReplayKeyring(ReplayKeyConfig{Dialect: constants.DbTypeSqlite, Ephemeral: true})
	if err != nil {
		t.Fatalf("second ephemeral keyring: %v", err)
	}
	if first.ActiveKeyID() == second.ActiveKeyID() {
		t.Error("an ephemeral keyring must be generated per boot, not shared")
	}
}

// TestReplayKeyringFileIsPrivateAndStableAcrossRestarts is the single-process
// deployment's key: created 0600 under the data root, and the same key on the
// next start, because a different one would leave every stored envelope
// unreadable.
func TestReplayKeyringFileIsPrivateAndStableAcrossRestarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), JobReplayKeyFileName)
	cfg := ReplayKeyConfig{Dialect: constants.DbTypeSqlite, KeyFilePath: path}

	first, err := LoadReplayKeyring(cfg)
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %v, want 0600", info.Mode().Perm())
	}

	// A restart must find the same key.
	second, err := LoadReplayKeyring(cfg)
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if second.ActiveKeyID() != first.ActiveKeyID() {
		t.Fatalf("a restart generated a different key: %s then %s", first.ActiveKeyID(), second.ActiveKeyID())
	}

	// A file whose permissions were widened — by a backup restore, an umask, an
	// rsync — is tightened rather than trusted.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("widen key file: %v", err)
	}
	third, err := LoadReplayKeyring(cfg)
	if err != nil {
		t.Fatalf("load after widening: %v", err)
	}
	info, err = os.Stat(path)
	if err != nil {
		t.Fatalf("stat key file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode after load = %v, want 0600", info.Mode().Perm())
	}
	if third.ActiveKeyID() != first.ActiveKeyID() {
		t.Fatalf("tightening the file changed the key: %s then %s", first.ActiveKeyID(), third.ActiveKeyID())
	}

	// The file holds the same grammar JOB_REPLAY_KEY accepts, so an operator can
	// promote it verbatim when the deployment becomes a multi-process one.
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read key file: %v", err)
	}
	promoted, err := LoadReplayKeyring(ReplayKeyConfig{Dialect: constants.DbTypePosgres, Keys: strings.TrimSpace(string(contents))})
	if err != nil {
		t.Fatalf("promote the key file to the environment grammar: %v", err)
	}
	if promoted.ActiveKeyID() != first.ActiveKeyID() {
		t.Fatalf("promoted key = %s, want %s", promoted.ActiveKeyID(), first.ActiveKeyID())
	}

	// A file that is not a key is refused rather than replaced: overwriting it
	// would make every envelope written under the real key unreadable.
	if err := os.WriteFile(path, []byte("this is not a key\n"), 0o600); err != nil {
		t.Fatalf("corrupt key file: %v", err)
	}
	if _, err := LoadReplayKeyring(cfg); !errors.Is(err, ErrInvalidReplayKey) {
		t.Fatalf("load of a corrupt key file = %v, want ErrInvalidReplayKey", err)
	}
}

// advanceReplayJob walks one Job one step through its lifecycle and returns the
// version a caller must name to move it again.
//
// Running is the one step no transition may make: entering running is what a claim
// does, and the Job this helper is given is not necessarily the Job a claim would
// pick. So the step is written rather than transitioned, by startTestExecution.
// Every other step is made as the execution that owns the Job — the token is read
// from the row — which is what the fence on a write that leaves running is about.
func advanceReplayJob(t *testing.T, svc *Service, deps Deps, snap Snapshot, to State) Snapshot {
	t.Helper()
	if to == StateRunning {
		return startTestExecution(t, deps, snap)
	}
	transition := Transition{
		JobID: snap.ID, ExpectedVersion: snap.Version, To: to,
		ExecutionToken: executionTokenOf(t, deps, snap.ID),
	}
	if to == StateFailed {
		transition.Failure = &Failure{Code: "test-failure", Class: FailureClassInternal, Message: "the executor gave up"}
	}
	next, err := svc.Transition(deps, transition)
	if err != nil {
		t.Fatalf("Transition %s -> %s: %v", snap.State, to, err)
	}
	return next
}

// executionTokenOf reads the token that owns a Job right now: what a write from
// running is fenced by, and what a test acting as the Job's execution names.
func executionTokenOf(t *testing.T, deps Deps, jobID string) string {
	t.Helper()
	return jobRow(t, deps, jobID).ExecutionToken
}

// startTestExecution establishes a Job in the state a claim leaves it in: running,
// owned by a fresh execution token, with the started event a claim records.
//
// A test whose subject is the lifecycle itself claims its Job — that is the only
// way in, and the refusals are tested where they belong. A test whose subject is
// something else needs a running Job it can name, which a claim cannot give it: Claim
// takes the oldest waiting Job of a Kind, and these jobs share Kinds. So the helper
// writes what a claim leaves, exactly as seedJob writes a state the lifecycle could
// otherwise only reach by walking to it, and it derives the row through the
// lifecycle's own applyTransition so the timestamps and durations a test goes on to
// assert are the ones an admitted execution would have.
//
// The claim row itself is not written. These Jobs are fixtures for what their own
// state, timeline and outputs do next, and a held claim row would make every one of
// them protected from the retention paths the tests exist to exercise.
func startTestExecution(t *testing.T, deps Deps, snap Snapshot) Snapshot {
	t.Helper()
	job := jobRow(t, deps, snap.ID)
	now := deps.now()
	next, updates := applyTransition(job, Transition{To: StateRunning}, deps.retention(), now)
	token := types.NewUUIDv7()
	updates["execution_token"] = token

	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.Job{}).
			Where("id = ? AND version = ? AND state = ?", job.ID, job.Version, job.State).
			Updates(updates)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("job %s moved while it was being started", job.ID)
		}
		sequence, err := nextEventSequence(tx, job.ID)
		if err != nil {
			return err
		}
		event := newEvent(job.ID, sequence, next.Version, EventStarted, nil, true, now)
		return tx.Create(&event).Error
	})
	if err != nil {
		t.Fatalf("start the execution of %s: %v", job.ID, err)
	}
	next.ExecutionToken = token
	return snapshot(next)
}

// TestReplayExpiryStartsAtTerminalCompletionNotAcceptance is the retention
// anchor: a Job that ran for a month must not lose the input it still needed,
// so the window opens when the Job ends, not when it was accepted.
func TestReplayExpiryStartsAtTerminalCompletionNotAcceptance(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	accepted := time.Date(2033, 5, 6, 7, 8, 9, 0, time.UTC)
	clock := accepted
	deps.Now = func() time.Time { return clock }
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material"), Retention: 2 * time.Hour}

	snap := acceptFixtureReplayJob(t, svc, deps)
	if envelope := replayEnvelopeRow(t, deps, snap.ID); envelope.ExpiresAt != nil {
		t.Fatalf("a queued Job's envelope already has a deadline: %v", envelope.ExpiresAt)
	}

	running := advanceReplayJob(t, svc, deps, snap, StateRunning)
	clock = clock.Add(30 * 24 * time.Hour) // a long run, well past the window
	if envelope := replayEnvelopeRow(t, deps, snap.ID); envelope.ExpiresAt != nil {
		t.Fatalf("a running Job's envelope acquired a deadline: %v", envelope.ExpiresAt)
	}
	if availability := svc.snapshotFor(deps, Access{Administrator: true}, jobRow(t, deps, snap.ID)).ReplayAvailability; availability != ReplayAvailable {
		t.Fatalf("availability after a month of running = %q, want %q", availability, ReplayAvailable)
	}
	if _, err := svc.OpenReplay(deps, Access{Administrator: true}, snap.ID); err != nil {
		t.Fatalf("a running Job's input must still be readable: %v", err)
	}

	paused := advanceReplayJob(t, svc, deps, running, StatePaused)
	blocked := advanceReplayJob(t, svc, deps, paused, StateBlocked)
	requeued := advanceReplayJob(t, svc, deps, blocked, StateQueued)
	finished := advanceReplayJob(t, svc, deps, requeued, StateRunning)
	clock = clock.Add(time.Hour)
	terminal := advanceReplayJob(t, svc, deps, finished, StateFailed)

	envelope := replayEnvelopeRow(t, deps, snap.ID)
	if envelope.ExpiresAt == nil {
		t.Fatal("a finished Job's envelope has no replay deadline")
	}
	want := clock.Add(2 * time.Hour)
	if !envelope.ExpiresAt.Equal(want) {
		t.Fatalf("replay deadline = %v, want finished_at + retention = %v", envelope.ExpiresAt, want)
	}
	if envelope.ExpiresAt.Sub(accepted) <= 2*time.Hour {
		t.Fatalf("the deadline was measured from acceptance: %v", envelope.ExpiresAt)
	}

	// Before the deadline it is still readable; after it, the Job's history is
	// unaffected and only the input is gone.
	if terminal.State != StateFailed {
		t.Fatalf("terminal state = %s", terminal.State)
	}
	clock = want.Add(-time.Minute)
	if _, err := svc.OpenReplay(deps, Access{Administrator: true}, snap.ID); err != nil {
		t.Fatalf("OpenReplay just before the deadline: %v", err)
	}
	clock = want.Add(time.Minute)
	if availability := svc.snapshotFor(deps, Access{Administrator: true}, jobRow(t, deps, snap.ID)).ReplayAvailability; availability != ReplayExpired {
		t.Fatalf("availability after the deadline = %q, want %q", availability, ReplayExpired)
	}
	if _, err := svc.OpenReplay(deps, Access{Administrator: true}, snap.ID); !errors.Is(err, ErrReplayExpired) {
		t.Fatalf("OpenReplay after the deadline = %v, want ErrReplayExpired", err)
	}

	// The sweep is what makes the bytes actually go, and it says so durably.
	purged, err := svc.PurgeExpiredReplay(deps, 10)
	if err != nil {
		t.Fatalf("PurgeExpiredReplay: %v", err)
	}
	if purged != 1 {
		t.Fatalf("purged %d envelopes, want 1", purged)
	}
	envelope = replayEnvelopeRow(t, deps, snap.ID)
	if envelope.PurgedAt == nil || envelope.PurgeReason != models.JobReplayPurgeExpired {
		t.Fatalf("expired envelope = %+v, want a durable expiry marker", envelope)
	}
	if envelope.Ciphertext != nil || envelope.Nonce != nil {
		t.Fatalf("the purged envelope still holds bytes: %+v", envelope)
	}
	if availability := svc.snapshotFor(deps, Access{Administrator: true}, jobRow(t, deps, snap.ID)).ReplayAvailability; availability != ReplayExpired {
		t.Fatalf("availability after the sweep = %q, want %q", availability, ReplayExpired)
	}
}

// TestReplayPurgeSweepIsBoundedAndExemptsNonterminalEnvelopes is the retention
// rule: a sweep may only take finished Jobs' input, and a bounded batch taken
// out of a large backlog may not touch work that is still owed an execution.
func TestReplayPurgeSweepIsBoundedAndExemptsNonterminalEnvelopes(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	start := time.Date(2034, 7, 8, 9, 10, 11, 0, time.UTC)
	clock := start
	deps.Now = func() time.Time { return clock }
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material"), Retention: time.Hour}

	// Three finished Jobs whose window has passed, one that is still running,
	// and one that finished but whose window has not.
	expired := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		snap := acceptFixtureReplayJob(t, svc, deps)
		running := advanceReplayJob(t, svc, deps, snap, StateRunning)
		advanceReplayJob(t, svc, deps, running, StateFailed)
		expired = append(expired, snap.ID)
	}
	running := acceptFixtureReplayJob(t, svc, deps)
	advanceReplayJob(t, svc, deps, running, StateRunning)

	// Finished later, so its own window is still open when the sweep runs.
	clock = start.Add(90 * time.Minute)
	recent := acceptFixtureReplayJob(t, svc, deps)
	advanceReplayJob(t, svc, deps, advanceReplayJob(t, svc, deps, recent, StateRunning), StateSucceeded)

	// A nonterminal envelope that somehow carries a deadline is still not
	// sweepable: the state, not the timestamp, is what exempts it. Nothing
	// stamps one — the assertion is that the sweep would not trust one either.
	poisoned := acceptFixtureReplayJob(t, svc, deps)
	advanceReplayJob(t, svc, deps, poisoned, StateRunning)
	if err := deps.DB.Model(&models.JobReplayEnvelope{}).
		Where("job_id = ?", poisoned.ID).
		Update("expires_at", start.Add(-time.Hour)).Error; err != nil {
		t.Fatalf("force a deadline on a running Job: %v", err)
	}

	clock = start.Add(2 * time.Hour)
	purged, err := svc.PurgeExpiredReplay(deps, 2)
	if err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if purged != 2 {
		t.Fatalf("first sweep purged %d, want the bounded 2", purged)
	}
	purged, err = svc.PurgeExpiredReplay(deps, 10)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if purged != 1 {
		t.Fatalf("second sweep purged %d, want the remaining 1", purged)
	}
	purged, err = svc.PurgeExpiredReplay(deps, 10)
	if err != nil {
		t.Fatalf("third sweep: %v", err)
	}
	if purged != 0 {
		t.Fatalf("third sweep purged %d, want 0: the sweep is not idempotent", purged)
	}

	for _, jobID := range expired {
		if envelope := replayEnvelopeRow(t, deps, jobID); envelope.PurgedAt == nil {
			t.Fatalf("finished Job %s kept its replay input", jobID)
		}
	}
	if envelope := replayEnvelopeRow(t, deps, running.ID); envelope.PurgedAt != nil {
		t.Fatalf("a running Job's replay input was swept: %+v", envelope)
	}
	if envelope := replayEnvelopeRow(t, deps, poisoned.ID); envelope.PurgedAt != nil {
		t.Fatalf("a running Job with a past deadline was swept: %+v", envelope)
	}
	if _, err := svc.OpenReplay(deps, Access{Administrator: true}, running.ID); err != nil {
		t.Fatalf("a running Job's input must survive the sweep: %v", err)
	}
	if envelope := replayEnvelopeRow(t, deps, recent.ID); envelope.PurgedAt != nil {
		t.Fatalf("a finished Job inside its window was swept: %+v", envelope)
	}
}

// TestReplayForgetRefusesNonterminalWorkAndPurgesFinishedWork is the purge rule:
// input is execution-required until the Job is done, and only then may an owner
// or an administrator take it away — atomically, with a marker that says so.
func TestReplayForgetRefusesNonterminalWorkAndPurgesFinishedWork(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	clock := time.Date(2035, 8, 9, 10, 11, 12, 0, time.UTC)
	deps.Now = func() time.Time { return clock }
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material"), Retention: 24 * time.Hour}

	owner := Access{UserID: 77}
	snap, err := svc.Accept(deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(77), ActorUserID: uintPtr(77),
		Replay: ReplayInput{Input: fixtureReplayInput()},
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	// Nonterminal: the input is still owed to an execution, so a purge would
	// leave a Job that can neither run nor be recovered.
	if _, err := svc.ForgetReplay(deps, owner, snap.ID); !errors.Is(err, ErrReplayExecutionRequired) {
		t.Fatalf("Forget of a queued Job = %v, want ErrReplayExecutionRequired", err)
	}
	if envelope := replayEnvelopeRow(t, deps, snap.ID); envelope.PurgedAt != nil || envelope.Ciphertext == nil {
		t.Fatalf("a refused Forget changed the envelope: %+v", envelope)
	}

	running := advanceReplayJob(t, svc, deps, snap, StateRunning)
	if _, err := svc.ForgetReplay(deps, owner, running.ID); !errors.Is(err, ErrReplayExecutionRequired) {
		t.Fatalf("Forget of a running Job = %v, want ErrReplayExecutionRequired", err)
	}
	clock = clock.Add(time.Minute)
	terminal := advanceReplayJob(t, svc, deps, running, StateCancelled)

	// Visibility: a Job the caller may not see is not found, and that includes
	// somebody else's finished Job.
	if _, err := svc.ForgetReplay(deps, Access{UserID: 78}, terminal.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Forget by a non-owner = %v, want ErrNotFound", err)
	}
	if envelope := replayEnvelopeRow(t, deps, snap.ID); envelope.PurgedAt != nil {
		t.Fatal("a refused Forget purged the envelope")
	}

	forgotten, err := svc.ForgetReplay(deps, owner, terminal.ID)
	if err != nil {
		t.Fatalf("Forget: %v", err)
	}
	if forgotten.ReplayAvailability != ReplayForgotten {
		t.Fatalf("availability after Forget = %q, want %q", forgotten.ReplayAvailability, ReplayForgotten)
	}
	envelope := replayEnvelopeRow(t, deps, snap.ID)
	if envelope.PurgedAt == nil || !envelope.PurgedAt.Equal(clock) {
		t.Fatalf("purge marker = %v, want %v", envelope.PurgedAt, clock)
	}
	if envelope.PurgeReason != models.JobReplayPurgeForgotten {
		t.Fatalf("purge reason = %q, want %q", envelope.PurgeReason, models.JobReplayPurgeForgotten)
	}
	if envelope.Ciphertext != nil || envelope.Nonce != nil {
		t.Fatalf("the forgotten envelope still holds bytes: %+v", envelope)
	}
	if _, err := svc.OpenReplay(deps, owner, terminal.ID); !errors.Is(err, ErrReplayForgotten) {
		t.Fatalf("OpenReplay after Forget = %v, want ErrReplayForgotten", err)
	}

	// Idempotent, and it does not rewrite the reason it was purged for.
	if _, err := svc.ForgetReplay(deps, owner, terminal.ID); err != nil {
		t.Fatalf("a second Forget: %v", err)
	}

	// A Job with no envelope at all has nothing to forget.
	noInput, err := svc.Accept(deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(77), ActorUserID: uintPtr(77),
		Replay: ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("Accept a non-replayable Job: %v", err)
	}
	if noInput.ReplayAvailability != ReplayAvailabilityNone {
		t.Fatalf("a non-replayable Job's availability = %q, want none", noInput.ReplayAvailability)
	}
	finishedNoInput := advanceReplayJob(t, svc, deps, advanceReplayJob(t, svc, deps, noInput, StateRunning), StateSucceeded)
	if _, err := svc.ForgetReplay(deps, owner, finishedNoInput.ID); !errors.Is(err, ErrReplayAbsent) {
		t.Fatalf("Forget of a Job with no input = %v, want ErrReplayAbsent", err)
	}
	if _, err := svc.OpenReplay(deps, owner, finishedNoInput.ID); !errors.Is(err, ErrReplayAbsent) {
		t.Fatalf("OpenReplay of non-replayable work = %v, want ErrReplayAbsent", err)
	}
}

// TestReplayAcceptanceRefusesInputItCannotSealOrSummarize is the fail-closed
// boundary: acceptance is the last moment at which refusing costs nothing, so
// every reason the input could not be stored properly — no codec, no key, a
// codec that cannot summarize it, work that declared itself non-replayable —
// refuses there rather than leaving a Job whose Retry silently cannot work.
func TestReplayAcceptanceRefusesInputItCannotSealOrSummarize(t *testing.T) {
	deps, _ := newReplayDeps(t)
	ring := replayKeyringFromSeeds(t, "primary-key-material")
	deps.Replay = &ReplayConfig{Keys: ring}

	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	acceptance := func() Acceptance {
		return Acceptance{
			Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
			Replay: ReplayInput{Input: fixtureReplayInput()},
		}
	}

	cases := []struct {
		name string
		// build returns the acceptance and the deps for one refusal.
		build func(t *testing.T) (Deps, Acceptance)
		want  error
	}{
		{
			name: "no codec is registered for the Kind",
			build: func(t *testing.T) (Deps, Acceptance) {
				return deps, acceptance()
			},
			want: ErrReplayCodecUnregistered,
		},
		{
			name: "the process holds no replay key",
			build: func(t *testing.T) (Deps, Acceptance) {
				keyless := deps
				keyless.Replay = nil
				return keyless, acceptance()
			},
			want: ErrReplayKeyRequired,
		},
		{
			name: "work that declared non-replayable input was given some",
			build: func(t *testing.T) (Deps, Acceptance) {
				request := acceptance()
				request.Replay.NonReplayable = true
				return deps, request
			},
			want: ErrInvalidAcceptance,
		},
		{
			name: "the request named its own summary as well",
			build: func(t *testing.T) (Deps, Acceptance) {
				request := acceptance()
				request.Summary = json.RawMessage(`{"host":"elsewhere.example"}`)
				return deps, request
			},
			want: ErrInvalidAcceptance,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			refusalDeps, request := testCase.build(t)
			// The no-codec case needs a service without the registration; every
			// other case uses the fixture codec.
			accepting := svc
			if errors.Is(testCase.want, ErrReplayCodecUnregistered) {
				accepting = NewService()
			}
			if _, err := accepting.Accept(refusalDeps, request); !errors.Is(err, testCase.want) {
				t.Fatalf("Accept = %v, want %v", err, testCase.want)
			}
			var stored int64
			if err := deps.DB.Model(&models.Job{}).Count(&stored).Error; err != nil {
				t.Fatalf("count jobs: %v", err)
			}
			if stored != 0 {
				t.Fatalf("a refused acceptance stored %d Jobs", stored)
			}
		})
	}

	// A codec whose own hooks cannot produce storable bytes is a Kind bug, and
	// it is refused at the boundary rather than stored.
	broken := fixtureReplayCodec()
	broken.Sanitize = func(json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage("not json"), nil
	}
	brokenSvc := NewService()
	if err := brokenSvc.RegisterReplayCodec("remote-download", 1, broken); err != nil {
		t.Fatalf("register broken codec: %v", err)
	}
	if _, err := brokenSvc.Accept(deps, acceptance()); !errors.Is(err, ErrInvalidReplay) {
		t.Fatalf("Accept with a broken sanitizer = %v, want ErrInvalidReplay", err)
	}

	// Registration is per (Kind, version) and refused twice: two codecs for one
	// version would make "what is this input" depend on initialization order.
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); !errors.Is(err, ErrInvalidReplayCodec) {
		t.Fatalf("duplicate registration = %v, want ErrInvalidReplayCodec", err)
	}
	if err := svc.RegisterReplayCodec("remote-download", 2, ReplayCodec{}); !errors.Is(err, ErrInvalidReplayCodec) {
		t.Fatalf("registration without hooks = %v, want ErrInvalidReplayCodec", err)
	}
}

// TestReplayMigrationDecodesAnEnvelopeFromARetiredKindVersion is what the
// Migrate hook exists for: a Job accepted under v1 stays replayable after the
// Kind moves to v2 and stops registering v1 at all.
func TestReplayMigrationDecodesAnEnvelopeFromARetiredKindVersion(t *testing.T) {
	deps, _ := newReplayDeps(t)
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}

	v1 := NewService()
	if err := v1.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register v1: %v", err)
	}
	oldJob := acceptFixtureReplayJob(t, v1, deps)

	// The v2 codec is the only one this release knows: it migrates the v1 input
	// into v2's shape and decodes that.
	seenFrom, seenTo := uint(0), uint(0)
	v2 := NewService()
	if err := v2.RegisterReplayCodec("remote-download", 2, ReplayCodec{
		Sanitize: func(input json.RawMessage) (json.RawMessage, error) { return input, nil },
		Encode:   func(input json.RawMessage) (json.RawMessage, error) { return input, nil },
		Migrate: func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			seenFrom, seenTo = fromVersion, toVersion
			var parsed map[string]any
			if err := json.Unmarshal(payload, &parsed); err != nil {
				return nil, err
			}
			parsed["migrated"] = true
			return json.Marshal(parsed)
		},
		Decode: func(payload json.RawMessage, kindVersion uint) (json.RawMessage, error) {
			if kindVersion != 2 {
				t.Errorf("Decode was handed v%d, want the version it was registered for", kindVersion)
			}
			return payload, nil
		},
	}); err != nil {
		t.Fatalf("register v2: %v", err)
	}

	opened, err := v2.OpenReplay(deps, Access{Administrator: true}, oldJob.ID)
	if err != nil {
		t.Fatalf("OpenReplay across a Kind version bump: %v", err)
	}
	if seenFrom != 1 || seenTo != 2 {
		t.Fatalf("Migrate was asked for %d -> %d, want 1 -> 2", seenFrom, seenTo)
	}
	if opened.MigratedFrom == nil || *opened.MigratedFrom != 1 {
		t.Fatalf("MigratedFrom = %v, want 1", opened.MigratedFrom)
	}
	var migrated map[string]any
	if err := json.Unmarshal(opened.Input, &migrated); err != nil {
		t.Fatalf("migrated input is not JSON: %v", err)
	}
	if migrated["migrated"] != true {
		t.Fatalf("the migration did not run: %s", opened.Input)
	}

	// A Kind nothing is registered for is a distinct refusal, not corruption.
	stranger := NewService()
	if _, err := stranger.OpenReplay(deps, Access{Administrator: true}, oldJob.ID); !errors.Is(err, ErrReplayCodecUnregistered) {
		t.Fatalf("OpenReplay with no codec = %v, want ErrReplayCodecUnregistered", err)
	}
	// And it is not a key problem either.
	if _, err := v2.OpenReplay(Deps{DB: deps.DB}, Access{Administrator: true}, oldJob.ID); !errors.Is(err, ErrReplayKeyUnavailable) {
		t.Fatalf("OpenReplay without a keyring = %v, want ErrReplayKeyUnavailable", err)
	}
}

// TestReplayBlockedSeparatesNonterminalFromTerminalDecodeFailures is the policy
// the plan states once rather than per Kind: work that cannot read its required
// input must be blocked, while finished work merely loses Retry and Repeat.
func TestReplayBlockedSeparatesNonterminalFromTerminalDecodeFailures(t *testing.T) {
	unreadable := []error{
		ErrReplayKeyUnavailable, ErrReplayCodecUnregistered, ErrReplayCorrupt, ErrReplayDecodeFailed,
	}
	for _, err := range unreadable {
		for _, state := range AllStates {
			wrapped := fmt.Errorf("jobs: wrapping: %w", err)
			if got, want := ReplayBlocked(state, wrapped), !state.Terminal(); got != want {
				t.Errorf("ReplayBlocked(%s, %v) = %v, want %v", state, err, got, want)
			}
		}
	}
	// Input that is absent, expired or forgotten is not a decode failure: there
	// is nothing to block on, and the Job's state describes what happened.
	for _, err := range []error{ErrReplayAbsent, ErrReplayExpired, ErrReplayForgotten, ErrNotFound, ErrVersionConflict} {
		if ReplayBlocked(StateQueued, err) {
			t.Errorf("ReplayBlocked(queued, %v) = true, want false", err)
		}
	}
	if ReplayBlocked(StateQueued, nil) {
		t.Error("ReplayBlocked(queued, nil) = true, want false")
	}
}

// TestReplayKeyFilePublishNeverReplacesAnExistingKey is the split-brain guard:
// two processes starting against one database must end up with one key, and the
// loser of the publish race must adopt the winner's key rather than keep the one
// it generated. A key file that was replaced would leave every envelope the
// other process wrote unreadable.
func TestReplayKeyFilePublishNeverReplacesAnExistingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), JobReplayKeyFileName)
	first := replayKeyFromSeed(t, "the-process-that-won-the-race")
	published, err := publishReplayKeyFile(path, first, syncReplayKeyDirectory)
	if err != nil {
		t.Fatalf("publish the first key: %v", err)
	}
	if published.ID != first.ID {
		t.Fatalf("publishing into a free name returned %s, want %s", published.ID, first.ID)
	}

	loser := replayKeyFromSeed(t, "the-process-that-lost-the-race")
	published, err = publishReplayKeyFile(path, loser, syncReplayKeyDirectory)
	if err != nil {
		t.Fatalf("publish the second key: %v", err)
	}
	if published.ID != first.ID {
		t.Fatalf("the losing publish returned %s, want the winner's %s", published.ID, first.ID)
	}

	ring, err := LoadReplayKeyring(ReplayKeyConfig{Dialect: constants.DbTypeSqlite, KeyFilePath: path})
	if err != nil {
		t.Fatalf("load after the race: %v", err)
	}
	if ring.ActiveKeyID() != first.ID {
		t.Fatalf("the key file holds %s, want the winner's %s", ring.ActiveKeyID(), first.ID)
	}

	// A file holding several keys is refused rather than read as its first: the
	// one key a single-process deployment keeps is not a rotation list.
	if err := os.WriteFile(path, []byte(base64Key(1)+"\n"+base64Key(2)+"\n"), 0o600); err != nil {
		t.Fatalf("write a two-key file: %v", err)
	}
	if _, err := LoadReplayKeyring(ReplayKeyConfig{Dialect: constants.DbTypeSqlite, KeyFilePath: path}); !errors.Is(err, ErrInvalidReplayKey) {
		t.Fatalf("load of a two-key file = %v, want ErrInvalidReplayKey", err)
	}
}

// TestReplayMalformedStoredNoncesFailClosed covers the one stored value that
// reaches a primitive which does not return an error: AES-GCM panics on a nonce
// that is not its own size, so a truncated, NULL or oversized nonce in the row
// would take the runtime goroutine down instead of being reported as corruption.
//
// The envelope table is a durable store that a partial migration, a truncating
// copy or a hostile write can all reach, and the refusal has to be the same
// refusal tampered ciphertext gets: ErrReplayCorrupt, with nothing run.
func TestReplayMalformedStoredNoncesFailClosed(t *testing.T) {
	cases := []struct {
		name  string
		nonce any
	}{
		{name: "a NULL nonce", nonce: nil},
		{name: "an empty nonce", nonce: []byte{}},
		{name: "a truncated nonce", nonce: []byte{1, 2, 3, 4}},
		{name: "an oversized nonce", nonce: bytes.Repeat([]byte{7}, 32)},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			deps, _ := newReplayDeps(t)
			svc := NewService()
			if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
				t.Fatalf("register codec: %v", err)
			}
			deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}
			adapter := registerTestAdapter(t, svc, Definition{
				Kind: "remote-download", KindVersion: 1, Restorable: true,
			})
			snap := acceptFixtureReplayJob(t, svc, deps)

			if err := deps.DB.Model(&models.JobReplayEnvelope{}).
				Where("job_id = ?", snap.ID).Updates(map[string]any{"nonce": testCase.nonce}).Error; err != nil {
				t.Fatalf("store a malformed nonce: %v", err)
			}

			if _, err := svc.OpenReplay(deps, Access{Administrator: true}, snap.ID); !errors.Is(err, ErrReplayCorrupt) {
				t.Fatalf("OpenReplay with a malformed nonce = %v, want ErrReplayCorrupt", err)
			}

			// The same row through dispatch: the refusal blocks the Job rather
			// than handing an executor input nobody can open.
			if _, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
				Kind: "remote-download", KindVersion: 1, Claimant: "runtime-a",
			}); err == nil {
				t.Fatalf("Claim of a Job whose envelope cannot be opened returned no error (ok=%v)", ok)
			}
			if adapter.dispatchedCount() != 0 {
				t.Fatalf("a Job with a malformed envelope reached the adapter %d times", adapter.dispatchedCount())
			}
			if stored := jobRow(t, deps, snap.ID); stored.State != string(StateBlocked) {
				t.Fatalf("state = %s, want blocked", stored.State)
			}
		})
	}
}

// TestReplayAcceptanceRequiresTheExactKindVersionCodec is the sealing half of
// versioned envelopes: what is sealed must be encoded by the codec registered
// for *that* Kind version, because the envelope records that version and every
// later read — including a migration — trusts the label.
//
// A fallback to the newest registered version would run v2's encoder and store
// the result under a v1 label, and OpenReplay would then migrate bytes that are
// already v2 as though they were v1.
func TestReplayAcceptanceRequiresTheExactKindVersionCodec(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 2, fixtureReplayCodec()); err != nil {
		t.Fatalf("register the v2 codec: %v", err)
	}
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}

	_, err := svc.Accept(deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		Replay: ReplayInput{Input: fixtureReplayInput()},
	})
	if !errors.Is(err, ErrReplayCodecUnregistered) {
		t.Fatalf("accepting v1 with only a v2 codec registered = %v, want ErrReplayCodecUnregistered", err)
	}
	if rows := countRows(t, deps, &models.Job{}, "kind_version = ?", 1); rows != 0 {
		t.Fatalf("a refused acceptance stored %d Job rows", rows)
	}
	if rows := countRows(t, deps, &models.JobReplayEnvelope{}, "kind_version = ?", 1); rows != 0 {
		t.Fatalf("a refused acceptance stored %d envelopes", rows)
	}
}

// TestReplayAcceptanceRequiresAnEnvelopeOrANonReplayableClass pins the
// acceptance contract §3 states: a Job is persisted with a replay envelope or
// with an explicit non-replayable classification, never with neither.
//
// The class defaults to replayable, so "no input supplied" used to be stored as
// replayable work with no envelope — a Job that can never be replayed and whose
// dispatch ran with incomplete input.
func TestReplayAcceptanceRequiresAnEnvelopeOrANonReplayableClass(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}

	_, err := svc.Accept(deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), ActorUserID: uintPtr(7),
	})
	if !errors.Is(err, ErrInvalidAcceptance) {
		t.Fatalf("accepting replayable work with no envelope = %v, want ErrInvalidAcceptance", err)
	}
	if rows := countRows(t, deps, &models.Job{}, "origin = ?", "api"); rows != 0 {
		t.Fatalf("a refused acceptance stored %d Job rows", rows)
	}

	// The other honest answer: work whose input cannot be replayed says so.
	declared, err := svc.Accept(deps, Acceptance{
		Kind: "remote-download", KindVersion: 1, State: StateQueued, Origin: "api",
		OwnerUserID: uintPtr(7), ActorUserID: uintPtr(7),
		Replay: ReplayInput{NonReplayable: true},
	})
	if err != nil {
		t.Fatalf("Accept with an explicit non-replayable class: %v", err)
	}
	if declared.ReplayClass != ReplayClassNonReplayable {
		t.Fatalf("replay class = %q, want %q", declared.ReplayClass, ReplayClassNonReplayable)
	}
	if rows := countRows(t, deps, &models.JobReplayEnvelope{}, "job_id = ?", declared.ID); rows != 0 {
		t.Fatalf("non-replayable acceptance stored %d envelopes", rows)
	}
}

// TestReplayableJobWithoutItsEnvelopeIsBlockedRatherThanRun is the dispatch half
// of the same contract. Whatever removed the envelope — a purge, a migration
// that could not convert it, a restore from a partial backup — the Job's durable
// class still says its input is replayable, and running it with no input at all
// is the one thing §3 forbids outright.
func TestReplayableJobWithoutItsEnvelopeIsBlockedRatherThanRun(t *testing.T) {
	deps, _ := newReplayDeps(t)
	svc := NewService()
	if err := svc.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
		t.Fatalf("register codec: %v", err)
	}
	deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}
	adapter := registerTestAdapter(t, svc, Definition{
		Kind: "remote-download", KindVersion: 1, Restorable: true,
	})
	snap := acceptFixtureReplayJob(t, svc, deps)

	if err := deps.DB.Where("job_id = ?", snap.ID).Delete(&models.JobReplayEnvelope{}).Error; err != nil {
		t.Fatalf("remove the envelope: %v", err)
	}

	if _, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
		Kind: "remote-download", KindVersion: 1, Claimant: "runtime-a",
	}); err == nil {
		t.Fatalf("Claim of a replayable Job with no envelope returned no error (ok=%v)", ok)
	}
	if adapter.dispatchedCount() != 0 {
		t.Fatalf("a replayable Job ran with no envelope %d times", adapter.dispatchedCount())
	}
	if stored := jobRow(t, deps, snap.ID); stored.State != string(StateBlocked) {
		t.Fatalf("state = %s, want blocked", stored.State)
	}
}

// TestReplayPostDecodeValidationBlocksTheClaimAndReleasesItsCapacity is §5's
// "nonterminal work that cannot decode its required input becomes `blocked`" read
// as covering the whole decode boundary rather than the codec's error return.
//
// A codec can succeed and still produce input no executor may run with: empty, not
// JSON, or over the size ceiling. That is a decode failure in the sense the rule
// means, and it was classified as a plain validation error — so the claim this
// process had just committed stayed committed: the Job sat `running` under a token
// with an execution that never started, and the capacity it was admitted under
// stayed occupied until somebody's lease expired.
func TestReplayPostDecodeValidationBlocksTheClaimAndReleasesItsCapacity(t *testing.T) {
	cases := []struct {
		name    string
		decoded func() json.RawMessage
	}{
		{name: "an empty payload", decoded: func() json.RawMessage { return nil }},
		{name: "a payload that is not JSON", decoded: func() json.RawMessage { return json.RawMessage("not json") }},
		{
			name: "a payload over the size ceiling",
			decoded: func() json.RawMessage {
				return json.RawMessage(`{"oversized":"` + strings.Repeat("a", MaxReplayPayloadBytes) + `"}`)
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			deps, _ := newReplayDeps(t)
			svc := NewService()
			codec := fixtureReplayCodec()
			codec.Decode = func(json.RawMessage, uint) (json.RawMessage, error) { return testCase.decoded(), nil }
			if err := svc.RegisterReplayCodec("remote-download", 1, codec); err != nil {
				t.Fatalf("register codec: %v", err)
			}
			deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}
			adapter := registerTestAdapter(t, svc, Definition{
				Kind: "remote-download", KindVersion: 1, Restorable: true,
			})
			snap := acceptFixtureReplayJob(t, svc, deps)

			_, ok, err := svc.Claim(context.Background(), deps, ClaimRequest{
				Kind: "remote-download", KindVersion: 1, Claimant: "runtime-a",
				Capacity: []CapacityRef{{Group: CapacityGroupGlobal, Limit: 1}},
			})
			if err == nil {
				t.Fatalf("Claim of a Job whose decoded input cannot be run returned no error (ok=%v)", ok)
			}
			if adapter.dispatchedCount() != 0 {
				t.Fatalf("a Job with unusable input reached the adapter %d times", adapter.dispatchedCount())
			}

			stored := jobRow(t, deps, snap.ID)
			if stored.State != string(StateBlocked) {
				t.Fatalf("state = %s, want blocked", stored.State)
			}
			if stored.ExecutionToken != "" {
				t.Fatalf("the blocked Job still carries the claim's execution token %q", stored.ExecutionToken)
			}
			if claim := claimRow(t, deps, snap.ID); claim.State != models.JobClaimStateReleased {
				t.Fatalf("claim state = %s, want released once the Job was blocked", claim.State)
			}
			if held := capacityCount(t, deps, CapacityGroupGlobal); held != 0 {
				t.Fatalf("capacity rows = %d, want none: a Job that never ran may not hold the slot it was admitted under", held)
			}
		})
	}
}

// TestReplayCodecFailuresNeverQuoteTheDecryptedInput is §5's "raw payloads never
// enter searchable summaries or user-visible Job Events" applied to the one
// channel that was left open: the codec's own error.
//
// A codec's Decode and Migrate hooks run on the decrypted bytes, and the errors a
// real one produces are the errors a parser produces — which quote their input. A
// URL with its query string, a Cookie header, an Authorization header and a plugin
// value all reached the caller that way, and the dispatch runtime logs that error
// verbatim, so a secret that was stored encrypted was written to the log in clear.
func TestReplayCodecFailuresNeverQuoteTheDecryptedInput(t *testing.T) {
	secrets := []string{fixtureQuerySecret, fixtureCookieSecret, fixtureAuthSecret, fixturePluginSecret}

	// Decode: the codec reports what it could not parse, quoting its input.
	t.Run("a failure decoding the current version", func(t *testing.T) {
		deps, _ := newReplayDeps(t)
		svc := NewService()
		codec := fixtureReplayCodec()
		codec.Decode = func(payload json.RawMessage, _ uint) (json.RawMessage, error) {
			return nil, fmt.Errorf("cannot decode payload %s", payload)
		}
		if err := svc.RegisterReplayCodec("remote-download", 1, codec); err != nil {
			t.Fatalf("register codec: %v", err)
		}
		deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}
		snap := acceptFixtureReplayJob(t, svc, deps)

		_, err := svc.OpenReplay(deps, Access{Administrator: true}, snap.ID)
		if !errors.Is(err, ErrReplayDecodeFailed) {
			t.Fatalf("OpenReplay = %v, want ErrReplayDecodeFailed", err)
		}
		for _, secret := range secrets {
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("the decode failure quoted a secret: %v", err)
			}
		}
	})

	// Migrate: the same, on the read path an envelope written at a retired Kind
	// version takes.
	t.Run("a failure migrating a retired version", func(t *testing.T) {
		deps, _ := newReplayDeps(t)
		original := NewService()
		if err := original.RegisterReplayCodec("remote-download", 1, fixtureReplayCodec()); err != nil {
			t.Fatalf("register v1: %v", err)
		}
		deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}
		snap := acceptFixtureReplayJob(t, original, deps)

		v2 := NewService()
		codec := fixtureReplayCodec()
		codec.Migrate = func(payload json.RawMessage, _, _ uint) (json.RawMessage, error) {
			return nil, fmt.Errorf("cannot upgrade payload %s", payload)
		}
		if err := v2.RegisterReplayCodec("remote-download", 2, codec); err != nil {
			t.Fatalf("register v2: %v", err)
		}

		_, err := v2.OpenReplay(deps, Access{Administrator: true}, snap.ID)
		if !errors.Is(err, ErrReplayDecodeFailed) {
			t.Fatalf("OpenReplay = %v, want ErrReplayDecodeFailed", err)
		}
		for _, secret := range secrets {
			if strings.Contains(err.Error(), secret) {
				t.Fatalf("the migration failure quoted a secret: %v", err)
			}
		}
	})
}

// TestReplayAcceptanceCodecFailuresNeverQuoteTheSubmittedInput is the same §5
// boundary on the write side of the envelope.
//
// Sanitize and Encode run on the input the caller *submitted* — the one place a
// URL query string, a Cookie header, an Authorization header and a plugin value
// are all present in the clear — and a codec that cannot read its input reports
// what it could not read. Service.Accept returns that error to its caller and the
// runtime logs it, so carrying the codec's own text is exactly the leak the read
// side already refuses: a secret that is about to be encrypted written out in
// plaintext instead.
func TestReplayAcceptanceCodecFailuresNeverQuoteTheSubmittedInput(t *testing.T) {
	accept := func(t *testing.T, codec ReplayCodec, svc *Service, deps Deps) error {
		t.Helper()
		if err := svc.RegisterReplayCodec("remote-download", 1, codec); err != nil {
			t.Fatalf("register codec: %v", err)
		}
		deps.Replay = &ReplayConfig{Keys: replayKeyringFromSeeds(t, "primary-key-material")}
		_, err := svc.Accept(deps, Acceptance{
			Kind:        "remote-download",
			KindVersion: 1,
			State:       StateQueued,
			Origin:      "api",
			Title:       "clip.mp4",
			Replay:      ReplayInput{Input: fixtureReplayInput()},
		})
		return err
	}

	// Sanitize: the codec reports what it could not summarize, quoting its input.
	t.Run("a failure sanitizing the input", func(t *testing.T) {
		deps, _ := newReplayDeps(t)
		svc := NewService()
		codec := fixtureReplayCodec()
		codec.Sanitize = func(input json.RawMessage) (json.RawMessage, error) {
			return nil, fmt.Errorf("cannot summarize %s", input)
		}

		err := accept(t, codec, svc, deps)
		if !errors.Is(err, ErrInvalidReplay) {
			t.Fatalf("Accept = %v, want ErrInvalidReplay", err)
		}
		requireSecretsAbsent(t, "the sanitize failure", err.Error())
		if !strings.Contains(err.Error(), "remote-download v1") {
			t.Errorf("the refusal = %q, want the Kind and version a reader acts on", err)
		}
		if jobs := countRows(t, deps, &models.Job{}, "1 = 1"); jobs != 0 {
			t.Fatalf("a refused acceptance stored %d Jobs", jobs)
		}
	})

	// Encode: the same, on the hook that produces the bytes that get encrypted.
	t.Run("a failure encoding the input", func(t *testing.T) {
		deps, _ := newReplayDeps(t)
		svc := NewService()
		codec := fixtureReplayCodec()
		codec.Encode = func(input json.RawMessage) (json.RawMessage, error) {
			return nil, fmt.Errorf("cannot encode payload %s", input)
		}

		err := accept(t, codec, svc, deps)
		if !errors.Is(err, ErrInvalidReplay) {
			t.Fatalf("Accept = %v, want ErrInvalidReplay", err)
		}
		requireSecretsAbsent(t, "the encode failure", err.Error())
		if !strings.Contains(err.Error(), "remote-download v1") {
			t.Errorf("the refusal = %q, want the Kind and version a reader acts on", err)
		}
		if envelopes := countRows(t, deps, &models.JobReplayEnvelope{}, "1 = 1"); envelopes != 0 {
			t.Fatalf("a refused acceptance stored %d envelopes", envelopes)
		}
	})
}

// TestReplayKeyFilePublicationMakesTheNameDurable is the boundary a key file
// exists for: the key that sealed committed envelopes must still be there after a
// power failure, or the next boot generates a different one and every envelope
// written under the first becomes unreadable.
//
// Syncing the *file* before publishing makes its contents durable and says nothing
// about the directory entry that now names it: the bytes can survive the crash and
// the name be lost. So the publication flushes the containing directory after the
// exclusive link — both when it won the race and when it adopted another process's
// key, because in the second case the winner's entry is the one this process now
// depends on. A directory that cannot be flushed is a refusal, and startup then
// stops before anything is accepted: a key nobody can promise will survive is the
// failure this file exists to prevent.
func TestReplayKeyFilePublicationMakesTheNameDurable(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, JobReplayKeyFileName)
	key := replayKeyFromSeed(t, "the-key-that-must-survive")

	t.Run("the containing directory is flushed after the name exists", func(t *testing.T) {
		var flushed []string
		recording := func(dir string) error {
			flushed = append(flushed, dir)
			return nil
		}
		published, err := publishReplayKeyFile(path, key, recording)
		if err != nil {
			t.Fatalf("publish: %v", err)
		}
		if published.ID != key.ID {
			t.Fatalf("published key = %s, want %s", published.ID, key.ID)
		}
		if len(flushed) != 1 || flushed[0] != directory {
			t.Fatalf("flushed %v, want the key file's own directory %s once", flushed, directory)
		}
		// The entry really is there, so the flush was about a name that exists.
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("the published key file is missing: %v", err)
		}
	})

	t.Run("a directory that cannot be flushed refuses the publication", func(t *testing.T) {
		second := filepath.Join(t.TempDir(), JobReplayKeyFileName)
		broken := func(string) error { return errors.New("the directory could not be flushed") }
		if _, err := publishReplayKeyFile(second, key, broken); err == nil {
			t.Fatal("a key whose name could not be made durable was accepted")
		}
	})

	t.Run("a competing publisher flushes the directory it adopted the key from", func(t *testing.T) {
		contestedDir := t.TempDir()
		contested := filepath.Join(contestedDir, JobReplayKeyFileName)
		winner := replayKeyFromSeed(t, "the-process-that-won-the-race")
		if _, err := publishReplayKeyFile(contested, winner, func(string) error { return nil }); err != nil {
			t.Fatalf("publish the winner's key: %v", err)
		}
		var flushed []string
		loser := replayKeyFromSeed(t, "the-process-that-lost-the-race")
		published, err := publishReplayKeyFile(contested, loser, func(dir string) error {
			flushed = append(flushed, dir)
			return nil
		})
		if err != nil {
			t.Fatalf("publish the losing key: %v", err)
		}
		if published.ID != winner.ID {
			t.Fatalf("the losing publish returned %s, want the winner's %s", published.ID, winner.ID)
		}
		if len(flushed) != 1 || flushed[0] != contestedDir {
			t.Fatalf("flushed %v, want the directory the adopted key was published into", flushed)
		}
	})

	t.Run("a publication that cannot be written reports the failure", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "absent", JobReplayKeyFileName)
		if _, err := publishReplayKeyFile(missing, key, func(string) error { return nil }); err == nil {
			t.Fatal("a key file was published into a directory that does not exist")
		}
	})
}
