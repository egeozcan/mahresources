package jobs

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"mahresources/constants"
	"mahresources/models"

	"gorm.io/gorm"
)

// This file holds replay input: the key material a deployment seals it with, the
// Kind-owned codec that turns a Kind's input into a summary and into bytes, the
// sealing itself, and the purge rules that end an envelope's life.
//
// Replay input is the one thing this module stores that a reader must never see.
// So it is stored twice over in two shapes: a bounded sanitized summary on the
// Job (searchable, and the only text), and an AES-256-GCM envelope in its own
// table (opaque, never returned by an ordinary read). Nothing here writes a
// plaintext secret anywhere — not a column, not an event, not a log line — and
// acceptance refuses to store input at all when this process holds no key.

// ReplayEnvelopeSchemaVersion is the envelope encoding this release writes. It is
// stored with each envelope so a later release can tell what it is reading; it is
// not the Kind's input version, which is stored beside it and is what a Kind's
// Migrate hook moves between.
const ReplayEnvelopeSchemaVersion uint = 1

// MaxReplayPayloadBytes bounds one sealed input. It is a refusal, not a
// truncation: an envelope nobody can seal whole is one nobody can replay.
const MaxReplayPayloadBytes = 1 << 20

// JobReplayKeyFileName is the private key file a persistent single-process
// deployment keeps under its data root. It holds the same base64 form
// JOB_REPLAY_KEY accepts, so an operator can promote it verbatim when the
// deployment becomes a multi-process one.
const JobReplayKeyFileName = "_job_replay_key"

// replayKeyBytes is AES-256. A key of any other length is refused rather than
// stretched: silently deriving a key from a short passphrase is how a deployment
// ends up believing it has 256 bits.
const replayKeyBytes = 32

// ReplayKey is one key with the non-secret identity its envelopes carry.
type ReplayKey struct {
	// ID is a SHA-256 fingerprint of the key material. It is stored on every
	// envelope and is safe to show: it identifies which key to use without
	// disclosing anything that could be used to decrypt.
	ID string
	// Key is the 32-byte key material. It never leaves this package except
	// through the AEAD.
	Key []byte
}

// NewReplayKey builds a key from raw 32-byte material, deriving its fingerprint.
func NewReplayKey(raw []byte) (ReplayKey, error) {
	if len(raw) != replayKeyBytes {
		return ReplayKey{}, fmt.Errorf("%w: a replay key is %d bytes, got %d",
			ErrInvalidReplayKey, replayKeyBytes, len(raw))
	}
	key := make([]byte, replayKeyBytes)
	copy(key, raw)
	sum := sha256.Sum256(key)
	return ReplayKey{ID: hex.EncodeToString(sum[:]), Key: key}, nil
}

// GenerateReplayKey returns a fresh random key, for the two deployments that
// have nothing to keep one in: an in-memory database, and the first start of a
// persistent one that has no key file yet.
func GenerateReplayKey() (ReplayKey, error) {
	raw := make([]byte, replayKeyBytes)
	if _, err := rand.Read(raw); err != nil {
		return ReplayKey{}, fmt.Errorf("jobs: generate replay key: %w", err)
	}
	return NewReplayKey(raw)
}

// ParseReplayKeys reads a JOB_REPLAY_KEY value: one or more comma-separated
// base64-encoded 32-byte keys, most recent first. The first is the active key;
// every other one is decrypt-only, which is what makes rotation possible without
// re-encrypting history.
func ParseReplayKeys(spec string) ([]ReplayKey, error) {
	fields := strings.Split(spec, ",")
	keys := make([]ReplayKey, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for index, field := range fields {
		trimmed := strings.TrimSpace(field)
		if trimmed == "" {
			return nil, fmt.Errorf("%w: entry %d is empty", ErrInvalidReplayKey, index+1)
		}
		raw, err := base64.StdEncoding.DecodeString(trimmed)
		if err != nil {
			return nil, fmt.Errorf("%w: entry %d is not base64: %v", ErrInvalidReplayKey, index+1, err)
		}
		key, err := NewReplayKey(raw)
		if err != nil {
			return nil, fmt.Errorf("entry %d: %w", index+1, err)
		}
		if _, duplicate := seen[key.ID]; duplicate {
			return nil, fmt.Errorf("%w: entry %d repeats key %s", ErrInvalidReplayKey, index+1, key.ID)
		}
		seen[key.ID] = struct{}{}
		keys = append(keys, key)
	}
	return keys, nil
}

// Keyring is the set of keys a process can decrypt with, and the single key it
// seals with: the first of them. It is immutable once built, so it can be shared
// by every caller in the process.
type Keyring struct {
	active ReplayKey
	keys   map[string]ReplayKey
}

// NewKeyring builds a keyring whose active key is the first one. It needs at
// least one key: an empty keyring would read as "seal with nothing", and the
// only safe answer to "which key should seal this" is a refusal.
func NewKeyring(keys ...ReplayKey) (*Keyring, error) {
	if len(keys) == 0 {
		return nil, fmt.Errorf("%w: a keyring needs at least one key", ErrInvalidReplayKey)
	}
	byID := make(map[string]ReplayKey, len(keys))
	for _, key := range keys {
		if len(key.Key) != replayKeyBytes {
			return nil, fmt.Errorf("%w: key %s is %d bytes, want %d",
				ErrInvalidReplayKey, key.ID, len(key.Key), replayKeyBytes)
		}
		if _, duplicate := byID[key.ID]; duplicate {
			return nil, fmt.Errorf("%w: key %s is listed twice", ErrInvalidReplayKey, key.ID)
		}
		byID[key.ID] = key
	}
	return &Keyring{active: keys[0], keys: byID}, nil
}

// ActiveKeyID is the fingerprint of the key new envelopes are sealed with.
func (k *Keyring) ActiveKeyID() string {
	if k == nil {
		return ""
	}
	return k.active.ID
}

// HasKey reports whether this process holds the key one envelope names. It is
// what a snapshot reads to answer "can this input be opened here", without
// decrypting anything to find out.
func (k *Keyring) HasKey(id string) bool {
	if k == nil {
		return false
	}
	_, ok := k.keys[id]
	return ok
}

// ReplayKeyConfig is the deployment a keyring is being loaded for. Every fact is
// passed in rather than probed, because the rules differ per deployment and a
// wrong guess is only discovered when an envelope becomes unreadable.
type ReplayKeyConfig struct {
	// Keys is the raw JOB_REPLAY_KEY value. It is the only source of an
	// explicitly configured key, and an explicit key always wins.
	Keys string
	// Dialect is the database dialect the deployment runs. PostgreSQL may have
	// several processes and hosts writing one database, so it can never
	// generate a key of its own.
	Dialect string
	// Ephemeral reports an in-memory database: its Jobs and their envelopes die
	// with the process, so a per-boot key loses nothing that outlives it.
	Ephemeral bool
	// KeyFilePath is where a persistent single-process deployment keeps its
	// private key file. An empty path means the deployment has no data root that
	// could hold one, which is not a deployment this module will seal into.
	KeyFilePath string
}

// LoadReplayKeyring resolves the deployment's replay keyring, or refuses to let
// the deployment start.
//
// The refusal matters more than the loading. A deployment that could accept
// durable secret work without a key it will still hold after a restart does not
// fail loudly — it accepts Jobs whose input becomes permanently unreadable the
// moment that process exits, and the first symptom is a Retry button that cannot
// work. So PostgreSQL needs an explicit key (it may have other processes), a
// persistent single-process deployment gets a private 0600 key file beside its
// data, and only an in-memory database may generate a key that is thrown away.
func LoadReplayKeyring(cfg ReplayKeyConfig) (*Keyring, error) {
	if strings.TrimSpace(cfg.Keys) != "" {
		keys, err := ParseReplayKeys(cfg.Keys)
		if err != nil {
			return nil, err
		}
		return NewKeyring(keys...)
	}

	if cfg.Ephemeral {
		key, err := GenerateReplayKey()
		if err != nil {
			return nil, err
		}
		return NewKeyring(key)
	}

	if strings.EqualFold(cfg.Dialect, constants.DbTypePosgres) {
		return nil, fmt.Errorf("%w: this deployment runs PostgreSQL, which several processes or hosts may "+
			"write, so set JOB_REPLAY_KEY to one or more comma-separated base64-encoded 32-byte keys (the "+
			"first is active, the rest are kept for reading older envelopes)", ErrReplayKeyRequired)
	}

	if cfg.KeyFilePath == "" {
		return nil, fmt.Errorf("%w: this deployment keeps durable Job input but has no data root to hold a "+
			"private key file; set JOB_REPLAY_KEY", ErrReplayKeyRequired)
	}

	key, err := loadOrCreateReplayKeyFile(cfg.KeyFilePath)
	if err != nil {
		return nil, err
	}
	return NewKeyring(key)
}

// replayKeyFilePerm is the only mode a replay key file is ever created or left
// in. Anything wider is a key readable by every account on the host.
const replayKeyFilePerm fs.FileMode = 0o600

// loadOrCreateReplayKeyFile reads the deployment's key file, creating it on
// first use.
//
// Creation is create-exclusive plus an atomic publish rather than a rename: two
// processes starting against one database must end up with the *same* key, and a
// rename would let the second silently overwrite the first's key — which is
// exactly the split-brain this file exists to prevent. The linker publishes only
// into an absent name; the loser of that race reads the winner's key. A file
// whose permissions are wider than 0600 is tightened rather than trusted.
func loadOrCreateReplayKeyFile(path string) (ReplayKey, error) {
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		if chmodErr := os.Chmod(path, replayKeyFilePerm); chmodErr != nil {
			return ReplayKey{}, fmt.Errorf("jobs: tighten replay key file %s: %w", path, chmodErr)
		}
		key, parseErr := parseReplayKeyFile(path, existing)
		if parseErr != nil {
			return ReplayKey{}, parseErr
		}
		return key, nil
	case errors.Is(err, fs.ErrNotExist):
	default:
		return ReplayKey{}, fmt.Errorf("jobs: read replay key file %s: %w", path, err)
	}

	generated, err := GenerateReplayKey()
	if err != nil {
		return ReplayKey{}, err
	}
	return publishReplayKeyFile(path, generated)
}

// parseReplayKeyFile reads the file's single base64 key. A file holding several
// is refused rather than read as its first: this file is the one key a
// single-process deployment keeps, and taking the first of a list would silently
// ignore the rest.
func parseReplayKeyFile(path string, contents []byte) (ReplayKey, error) {
	keys, err := ParseReplayKeys(strings.TrimSpace(string(contents)))
	if err != nil {
		return ReplayKey{}, fmt.Errorf("jobs: replay key file %s: %w", path, err)
	}
	if len(keys) != 1 {
		return ReplayKey{}, fmt.Errorf("%w: replay key file %s holds %d keys, want exactly 1",
			ErrInvalidReplayKey, path, len(keys))
	}
	return keys[0], nil
}

// publishReplayKeyFile writes a fresh key to its final name, atomically and only
// if that name is still free, and returns the key that name now holds.
//
// The return value is the point. When the name was taken, the process that lost
// the race must adopt the winner's key — returning its own would give one
// database two keys, and every envelope written by the other process would be
// unreadable here.
func publishReplayKeyFile(path string, key ReplayKey) (ReplayKey, error) {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, "."+JobReplayKeyFileName+"-*")
	if err != nil {
		return ReplayKey{}, fmt.Errorf("jobs: create replay key file in %s: %w", dir, err)
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if err := temp.Chmod(replayKeyFilePerm); err != nil {
		temp.Close()
		return ReplayKey{}, fmt.Errorf("jobs: set replay key file mode: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(key.Key) + "\n"
	if _, err := temp.WriteString(encoded); err != nil {
		temp.Close()
		return ReplayKey{}, fmt.Errorf("jobs: write replay key file: %w", err)
	}
	// Synced before the name exists: a key file that is published and then lost
	// to a power cut would leave envelopes nothing can open.
	if err := temp.Sync(); err != nil {
		temp.Close()
		return ReplayKey{}, fmt.Errorf("jobs: sync replay key file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return ReplayKey{}, fmt.Errorf("jobs: close replay key file: %w", err)
	}

	// Link is the exclusive publish: it fails when the name is taken, where a
	// rename would silently replace whatever another process just wrote.
	if err := os.Link(tempName, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			// Another process won the race. Its key is the deployment's key.
			existing, readErr := os.ReadFile(path)
			if readErr != nil {
				return ReplayKey{}, fmt.Errorf("jobs: read the replay key another process published: %w", readErr)
			}
			winner, parseErr := parseReplayKeyFile(path, existing)
			if parseErr != nil {
				return ReplayKey{}, parseErr
			}
			return winner, nil
		}
		return ReplayKey{}, fmt.Errorf("jobs: publish replay key file %s: %w", path, err)
	}
	return key, nil
}

// ReplayCodec is one Kind version's declaration of how its input becomes
// searchable text and how it becomes sealed bytes, and how it comes back.
//
// The hooks exist because only the Kind knows what in its input is a secret, a
// pointer to a mutable domain row, or a derived field. Two rules follow from
// that: Sanitize is the only thing that may turn input into text a reader sees,
// and a Kind version with no registered codec cannot have its input sealed,
// decoded or migrated — so it is refusable at acceptance rather than a Job whose
// Retry silently cannot work.
type ReplayCodec struct {
	// Sanitize derives the bounded, searchable summary for one input. It is
	// where a Kind states what about its input is safe to read.
	Sanitize func(input json.RawMessage) (json.RawMessage, error)
	// Encode returns the bytes to seal: the input normalized into what must
	// actually be stored for a later execution.
	Encode func(input json.RawMessage) (json.RawMessage, error)
	// Decode returns the input an executor runs with, from an envelope written
	// at kindVersion.
	Decode func(payload json.RawMessage, kindVersion uint) (json.RawMessage, error)
	// Migrate upgrades an envelope written at fromVersion to toVersion. It is
	// only consulted when the envelope's own version has no registered codec —
	// a version that was retired after its Jobs drained.
	Migrate func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error)
}

// replayCodecKey names one registered codec: a Kind and one version of its input
// semantics.
type replayCodecKey struct {
	kind    string
	version uint
}

// RegisterReplayCodec teaches the Service how one Kind version's input is
// summarized and sealed.
//
// Registration is per (Kind, version) and a repeated pair is refused rather than
// replaced: two codecs for one version means two answers to "what is this input",
// and which one wins would depend on initialization order.
func (s *Service) RegisterReplayCodec(kind string, version uint, codec ReplayCodec) error {
	if strings.TrimSpace(kind) == "" {
		return fmt.Errorf("%w: kind is required", ErrInvalidReplayCodec)
	}
	if len(kind) > MaxKindBytes {
		return fmt.Errorf("%w: kind is %d bytes, over the %d-byte ceiling", ErrInvalidReplayCodec, len(kind), MaxKindBytes)
	}
	if version == 0 {
		return fmt.Errorf("%w: kind version must be at least 1", ErrInvalidReplayCodec)
	}
	switch {
	case codec.Sanitize == nil:
		return fmt.Errorf("%w: %s v%d has no Sanitize hook", ErrInvalidReplayCodec, kind, version)
	case codec.Encode == nil:
		return fmt.Errorf("%w: %s v%d has no Encode hook", ErrInvalidReplayCodec, kind, version)
	case codec.Decode == nil:
		return fmt.Errorf("%w: %s v%d has no Decode hook", ErrInvalidReplayCodec, kind, version)
	case codec.Migrate == nil:
		return fmt.Errorf("%w: %s v%d has no Migrate hook", ErrInvalidReplayCodec, kind, version)
	}

	s.replayMu.Lock()
	defer s.replayMu.Unlock()
	if s.replayCodecs == nil {
		s.replayCodecs = map[replayCodecKey]ReplayCodec{}
	}
	key := replayCodecKey{kind: kind, version: version}
	if _, exists := s.replayCodecs[key]; exists {
		return fmt.Errorf("%w: %s v%d is already registered", ErrInvalidReplayCodec, kind, version)
	}
	s.replayCodecs[key] = codec
	return nil
}

// replayCodecFor returns the codec to decode an envelope written at version
// with, and the version it decodes *to*.
//
// An envelope whose own version is registered is decoded at that version: it is
// the semantics the Job was accepted under, and migrating it forward would make
// a dispatch depend on a later release's reading of it. Only when that version
// has no codec does the newest registered version for the Kind take over, and
// then Migrate is what carries the payload forward.
func (s *Service) replayCodecFor(kind string, version uint) (ReplayCodec, uint, error) {
	s.replayMu.Lock()
	defer s.replayMu.Unlock()

	if codec, ok := s.replayCodecs[replayCodecKey{kind: kind, version: version}]; ok {
		return codec, version, nil
	}
	latest := uint(0)
	for key := range s.replayCodecs {
		if key.kind == kind && key.version > latest {
			latest = key.version
		}
	}
	if latest == 0 {
		return ReplayCodec{}, 0, fmt.Errorf("%w: %s v%d", ErrReplayCodecUnregistered, kind, version)
	}
	return s.replayCodecs[replayCodecKey{kind: kind, version: latest}], latest, nil
}

// hasReplayCodec reports whether any registered codec could decode an envelope
// written at this Kind version. It is the codec half of a snapshot's
// availability answer, and it never runs a hook.
func (s *Service) hasReplayCodec(kind string, version uint) bool {
	_, _, err := s.replayCodecFor(kind, version)
	return err == nil
}

// sealReplay encodes and encrypts one Kind's input for one Job.
//
// The returned row is the envelope; the summary is the only text derived from
// the input, and it comes from the Kind's own sanitizer rather than from the
// adapter, so an adapter cannot put a secret in the one place readers look.
func (s *Service) sealReplay(deps Deps, job models.Job, input ReplayInput, now time.Time) (models.JobReplayEnvelope, json.RawMessage, error) {
	// The key is checked first: a process with no keyring at all cannot store
	// this whatever the Kind declares, and "no replay key" is the answer an
	// operator needs rather than "no codec registered".
	if deps.Replay == nil || deps.Replay.Keys == nil {
		return models.JobReplayEnvelope{}, nil, fmt.Errorf("%w: no replay key is configured, so this input "+
			"cannot be stored and must not be stored in the clear", ErrReplayKeyRequired)
	}

	codec, err := s.requireReplayCodec(job.Kind, job.KindVersion)
	if err != nil {
		return models.JobReplayEnvelope{}, nil, err
	}
	summary, err := codec.Sanitize(input.Input)
	if err != nil {
		return models.JobReplayEnvelope{}, nil, fmt.Errorf("%w: %s v%d sanitize: %v",
			ErrInvalidReplay, job.Kind, job.KindVersion, err)
	}
	if err := validateReplayJSON("summary", summary, MaxSummaryBytes); err != nil {
		return models.JobReplayEnvelope{}, nil, err
	}

	payload, err := codec.Encode(input.Input)
	if err != nil {
		return models.JobReplayEnvelope{}, nil, fmt.Errorf("%w: %s v%d encode: %v",
			ErrInvalidReplay, job.Kind, job.KindVersion, err)
	}
	if err := validateReplayJSON("input", payload, MaxReplayPayloadBytes); err != nil {
		return models.JobReplayEnvelope{}, nil, err
	}

	nonce := make([]byte, replayNonceBytes)
	if _, err := rand.Read(nonce); err != nil {
		return models.JobReplayEnvelope{}, nil, fmt.Errorf("jobs: generate replay nonce: %w", err)
	}
	ciphertext, err := sealReplayPayload(deps.Replay.Keys, nonce, payload, replayAdditionalData(job))
	if err != nil {
		return models.JobReplayEnvelope{}, nil, err
	}

	return models.JobReplayEnvelope{
		JobID:         job.ID,
		Kind:          job.Kind,
		KindVersion:   job.KindVersion,
		SchemaVersion: ReplayEnvelopeSchemaVersion,
		KeyID:         deps.Replay.Keys.ActiveKeyID(),
		Nonce:         nonce,
		Ciphertext:    ciphertext,
		CreatedAt:     now,
		UpdatedAt:     now,
	}, summary, nil
}

// requireReplayCodec resolves the codec registered for a Kind version or refuses
// with the reason no codec is available.
func (s *Service) requireReplayCodec(kind string, version uint) (ReplayCodec, error) {
	codec, _, err := s.replayCodecFor(kind, version)
	return codec, err
}

// validateReplayJSON checks a codec's output before it is stored: it must be
// JSON, and within its bound. A hook returning nonsense is a Kind bug, and the
// boundary is where it is refused.
func validateReplayJSON(what string, payload json.RawMessage, limit int) error {
	if len(payload) == 0 {
		return fmt.Errorf("%w: the %s is empty", ErrInvalidReplay, what)
	}
	if !json.Valid(payload) {
		return fmt.Errorf("%w: the %s is not valid JSON", ErrInvalidReplay, what)
	}
	if len(payload) > limit {
		return fmt.Errorf("%w: the %s is %d bytes, over the %d-byte ceiling", ErrInvalidReplay, what, len(payload), limit)
	}
	return nil
}

// replayNonceBytes is the GCM standard nonce length.
const replayNonceBytes = 12

// replayAdditionalData binds a ciphertext to exactly one Job, Kind and Kind
// version. Moving a row to another Job, or reading it under another Kind's
// decoder, therefore fails authentication rather than decrypting into the wrong
// execution.
//
// The envelope *schema* version is deliberately not part of it. It is a column
// that says how to read the row, and binding it here would mean a later release
// that bumps it — or that reads a row written under an older schema — has to
// reproduce this release's encoding exactly to decrypt anything at all.
func replayAdditionalData(job models.Job) []byte {
	return []byte(strings.Join([]string{
		"mahresources/job-replay",
		job.ID,
		job.Kind,
		strconv.FormatUint(uint64(job.KindVersion), 10),
	}, "\n"))
}

// sealReplayPayload encrypts one payload with the keyring's active key.
func sealReplayPayload(ring *Keyring, nonce, payload, additionalData []byte) ([]byte, error) {
	aead, err := newReplayAEAD(ring.active.Key)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nonce, payload, additionalData), nil
}

// openReplayPayload decrypts one envelope with the key it names. A key this
// process does not hold is reported as exactly that, because "we cannot read
// this because the key is elsewhere" and "these bytes are not what they claim"
// are different operational problems.
func openReplayPayload(ring *Keyring, envelope models.JobReplayEnvelope, additionalData []byte) ([]byte, error) {
	if ring == nil || !ring.HasKey(envelope.KeyID) {
		return nil, fmt.Errorf("%w: envelope %s was sealed with key %s",
			ErrReplayKeyUnavailable, envelope.JobID, envelope.KeyID)
	}
	aead, err := newReplayAEAD(ring.keys[envelope.KeyID].Key)
	if err != nil {
		return nil, err
	}
	plaintext, err := aead.Open(nil, envelope.Nonce, envelope.Ciphertext, additionalData)
	if err != nil {
		return nil, fmt.Errorf("%w: envelope %s", ErrReplayCorrupt, envelope.JobID)
	}
	return plaintext, nil
}

// newReplayAEAD builds the AES-256-GCM cipher over one key.
func newReplayAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidReplayKey, err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("jobs: replay cipher: %w", err)
	}
	return aead, nil
}

// stampReplayExpiry gives a Job that just reached an end state its replay
// deadline: a fixed window measured from terminal completion.
//
// It is called from the terminal transition that produced finished_at, so the
// deadline exists the moment the outcome does, and it is anchored on the
// finished instant rather than on acceptance — a Job that ran for a week must
// not lose its input while it was still running. A Job with no envelope, a
// purged one, or a retention of zero ("not configured", never "expire now")
// is left exactly as it is.
func stampReplayExpiry(tx *gorm.DB, job models.Job, replay *ReplayConfig, now time.Time) error {
	if replay == nil || replay.Retention <= 0 || job.FinishedAt == nil {
		return nil
	}
	expires := job.FinishedAt.Add(replay.Retention).UTC()
	result := tx.Model(&models.JobReplayEnvelope{}).
		Where("job_id = ? AND purged_at IS NULL", job.ID).
		Updates(map[string]any{"expires_at": expires, "updated_at": now.UTC()})
	if result.Error != nil {
		return fmt.Errorf("jobs: stamp replay expiry: %w", result.Error)
	}
	return nil
}

// replayEnvelope loads one Job's envelope row.
func replayEnvelope(db *gorm.DB, jobID string) (models.JobReplayEnvelope, error) {
	var envelope models.JobReplayEnvelope
	err := db.Where("job_id = ?", jobID).First(&envelope).Error
	if err != nil {
		if isNotFound(err) {
			return models.JobReplayEnvelope{}, fmt.Errorf("%w: job %s", ErrReplayAbsent, jobID)
		}
		return models.JobReplayEnvelope{}, fmt.Errorf("jobs: load replay envelope: %w", err)
	}
	return envelope, nil
}

// replayAvailabilityOf reports what a viewer can do with one Job's replay input.
//
// It never decrypts, and it never runs a codec: an ordinary read must not spend
// a key operation per row, and the answer — can this be opened here — is already
// implied by the key ID and the registered Kind versions. Corruption is not
// visible here on purpose: a snapshot says Retry is unavailable, and the host's
// own read is where the two causes are told apart.
func (s *Service) replayAvailabilityOf(db *gorm.DB, keys *Keyring, job models.Job, now time.Time) ReplayAvailability {
	if ReplayClass(job.ReplayClass) == ReplayClassNonReplayable {
		return ReplayAvailabilityNone
	}
	envelope, err := replayEnvelope(db, job.ID)
	if err != nil {
		return ReplayUnreadable
	}
	switch {
	case envelope.PurgedAt != nil && envelope.PurgeReason == models.JobReplayPurgeForgotten:
		return ReplayForgotten
	case envelope.PurgedAt != nil:
		return ReplayExpired
	case envelope.ExpiresAt != nil && !envelope.ExpiresAt.After(now):
		// The deadline was stamped by the terminal transition and the sweep has
		// not caught up yet. A viewer is told the input is gone now, because it
		// is: retention is a decision the lifecycle already recorded, and the
		// sweep only removes bytes.
		return ReplayExpired
	case keys == nil || !keys.HasKey(envelope.KeyID):
		return ReplayUnreadable
	case !s.hasReplayCodec(envelope.Kind, envelope.KindVersion):
		return ReplayUnreadable
	}
	return ReplayAvailable
}

// OpenReplay reads one Job's replay input: the decrypted, migrated value an
// executor runs with, or a refusal that says why there is none.
//
// It applies the shared visibility predicate rather than an execution token. A
// Retry is a fresh submission by a person, so the first question is "may this
// principal see this Job at all"; the host runtime, which owns work nobody
// asked about in this instant, opens with Access{Administrator: true} — not a
// claim about a user, but the fact that the process is not one.
//
// The failure families are kept apart on purpose. A key this process does not
// hold, a Kind version nothing can decode, and ciphertext that does not
// authenticate are all "no replay here" to a viewer, and each means something
// different to an operator.
func (s *Service) OpenReplay(deps Deps, access Access, jobID string) (OpenedReplay, error) {
	if strings.TrimSpace(jobID) == "" {
		return OpenedReplay{}, fmt.Errorf("%w: empty job id", ErrNotFound)
	}
	var job models.Job
	err := visibleTo(deps.DB, access).Where("id = ?", jobID).First(&job).Error
	if err != nil {
		if isNotFound(err) {
			return OpenedReplay{}, fmt.Errorf("%w: %s", ErrNotFound, jobID)
		}
		return OpenedReplay{}, fmt.Errorf("jobs: load job: %w", err)
	}
	if ReplayClass(job.ReplayClass) == ReplayClassNonReplayable {
		return OpenedReplay{}, fmt.Errorf("%w: job %s declares non-replayable input", ErrReplayAbsent, job.ID)
	}

	envelope, err := replayEnvelope(deps.DB, job.ID)
	if err != nil {
		return OpenedReplay{}, err
	}
	if envelope.PurgedAt != nil {
		return OpenedReplay{}, replayPurgedError(envelope)
	}
	if envelope.ExpiresAt != nil && !envelope.ExpiresAt.After(deps.now()) {
		return OpenedReplay{}, fmt.Errorf("%w: job %s", ErrReplayExpired, job.ID)
	}
	if envelope.SchemaVersion != ReplayEnvelopeSchemaVersion {
		return OpenedReplay{}, fmt.Errorf("%w: envelope %s was written with schema version %d, and this "+
			"release reads %d", ErrReplayDecodeFailed, job.ID, envelope.SchemaVersion, ReplayEnvelopeSchemaVersion)
	}
	if envelope.Kind != job.Kind || envelope.KindVersion != job.KindVersion {
		// The row disagrees with its Job about what it holds. The ciphertext is
		// bound to the Job's Kind and version, so it would fail authentication
		// anyway; naming the real reason is better than reporting a data error
		// for a bookkeeping one.
		return OpenedReplay{}, fmt.Errorf("%w: envelope %s names %s v%d, its job is %s v%d",
			ErrReplayCorrupt, job.ID, envelope.Kind, envelope.KindVersion, job.Kind, job.KindVersion)
	}

	codec, targetVersion, err := s.replayCodecFor(envelope.Kind, envelope.KindVersion)
	if err != nil {
		return OpenedReplay{}, err
	}

	plaintext, err := openReplayPayload(replayKeys(deps), envelope, replayAdditionalData(job))
	if err != nil {
		return OpenedReplay{}, err
	}

	var migratedFrom *uint
	if targetVersion != envelope.KindVersion {
		migrated, migrateErr := codec.Migrate(plaintext, envelope.KindVersion, targetVersion)
		if migrateErr != nil {
			return OpenedReplay{}, fmt.Errorf("%w: %s v%d -> v%d: %v",
				ErrReplayDecodeFailed, envelope.Kind, envelope.KindVersion, targetVersion, migrateErr)
		}
		plaintext = migrated
		from := envelope.KindVersion
		migratedFrom = &from
	}

	decoded, err := codec.Decode(plaintext, targetVersion)
	if err != nil {
		return OpenedReplay{}, fmt.Errorf("%w: %s v%d: %v", ErrReplayDecodeFailed, envelope.Kind, targetVersion, err)
	}
	if err := validateReplayJSON("input", decoded, MaxReplayPayloadBytes); err != nil {
		return OpenedReplay{}, err
	}

	return OpenedReplay{
		JobID:         job.ID,
		Kind:          job.Kind,
		KindVersion:   job.KindVersion,
		SchemaVersion: envelope.SchemaVersion,
		Input:         decoded,
		MigratedFrom:  migratedFrom,
	}, nil
}

// ReplayBlocked reports whether an unreadable replay input blocks the Job it
// belongs to, which is the rule that keeps a decode failure from being decided
// per Kind.
//
// Nonterminal work whose required input cannot be decoded must become blocked:
// it cannot run with incomplete input, and discarding it would erase a Job that
// a person is still waiting for. Terminal work has nothing left to do — the
// failure merely means Retry and Repeat are not advertised, and its recorded
// outcome stands.
func ReplayBlocked(state State, err error) bool {
	if state.Terminal() {
		return false
	}
	switch {
	case errors.Is(err, ErrReplayKeyUnavailable),
		errors.Is(err, ErrReplayCodecUnregistered),
		errors.Is(err, ErrReplayCorrupt),
		errors.Is(err, ErrReplayDecodeFailed):
		return true
	default:
		return false
	}
}

// replayPurgedError reports why a purged envelope has no input, in the terms the
// purge was recorded in.
func replayPurgedError(envelope models.JobReplayEnvelope) error {
	if envelope.PurgeReason == models.JobReplayPurgeForgotten {
		return fmt.Errorf("%w: job %s", ErrReplayForgotten, envelope.JobID)
	}
	return fmt.Errorf("%w: job %s", ErrReplayExpired, envelope.JobID)
}

// DefaultReplayPurgeBatch bounds one purge transaction's work, so a sweep over a
// large backlog is a series of small writes rather than one long one.
const DefaultReplayPurgeBatch = 200

// PurgeExpiredReplay removes the input of finished Jobs whose replay window has
// passed, and reports how many envelopes it emptied.
//
// Terminal state is the exemption, not the timestamp: a Job that is not
// finished keeps execution-required input however old it is, because input that
// is still owed to an execution is not history. The deadline itself is stamped
// by the terminal transition, so this sweep only ever reads back a decision the
// lifecycle already made — it never decides that something is finished.
//
// The purge is one statement, and it is the transaction's first statement: on
// SQLite the writer lock is taken before anything is read, which keeps the sweep
// from promoting a read snapshot after another connection committed. The batch
// is selected by a bounded subquery and claimed by the update itself, so two
// sweeps racing each other purge one envelope once.
func (s *Service) PurgeExpiredReplay(deps Deps, limit int) (int, error) {
	if limit <= 0 {
		limit = DefaultReplayPurgeBatch
	}
	now := deps.now()
	purge := map[string]any{
		"ciphertext":   gorm.Expr("NULL"),
		"nonce":        gorm.Expr("NULL"),
		"purged_at":    now,
		"purge_reason": models.JobReplayPurgeExpired,
		"updated_at":   now,
	}

	purged := int64(0)
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		expired := tx.Model(&models.JobReplayEnvelope{}).
			Select("job_replay_envelopes.job_id").
			Where("job_replay_envelopes.purged_at IS NULL").
			Where("job_replay_envelopes.expires_at IS NOT NULL AND job_replay_envelopes.expires_at <= ?", now).
			Where("EXISTS (SELECT 1 FROM jobs WHERE jobs.id = job_replay_envelopes.job_id AND jobs.state IN ?)",
				terminalStates()).
			Order("job_replay_envelopes.expires_at, job_replay_envelopes.job_id").
			Limit(limit)

		result := tx.Model(&models.JobReplayEnvelope{}).
			Where("job_id IN (?)", expired).
			Updates(purge)
		if result.Error != nil {
			return fmt.Errorf("jobs: purge expired replay input: %w", result.Error)
		}
		purged = result.RowsAffected
		return nil
	})
	if err != nil {
		return 0, err
	}
	return int(purged), nil
}

// terminalStates lists the states an ordinary retention sweep may touch.
func terminalStates() []string {
	states := make([]string, 0, len(AllStates))
	for _, state := range AllStates {
		if state.Terminal() {
			states = append(states, string(state))
		}
	}
	return states
}

// ForgetReplay purges one finished Job's replay input and reports the Job's
// snapshot with its availability now reading forgotten.
//
// It is refused while the input is execution-required — the Job must first be
// cancelled or reconciled to a terminal state — because purging input a pending
// execution still needs leaves work that can neither run nor be recovered, and
// because "Forget" is not a second way to cancel something.
//
// The purge is one guarded update on the one row that is both the envelope and
// the marker: the ciphertext and nonce become NULL and the purge instant and
// reason are written in the same statement, so there is no instant at which the
// bytes are gone and nothing records why. The row itself survives, which is what
// makes the purge durable evidence rather than an absence — a later backfill can
// see that this Job's input was deliberately removed and must not be
// reconstructed from a legacy copy.
//
// Repeating it is not an error: an already-purged envelope is the outcome that
// was asked for, and the reason it was purged for is not rewritten.
func (s *Service) ForgetReplay(deps Deps, access Access, jobID string) (Snapshot, error) {
	if strings.TrimSpace(jobID) == "" {
		return Snapshot{}, fmt.Errorf("%w: empty job id", ErrNotFound)
	}
	var job models.Job
	err := visibleTo(deps.DB, access).Where("id = ?", jobID).First(&job).Error
	if err != nil {
		if isNotFound(err) {
			return Snapshot{}, fmt.Errorf("%w: %s", ErrNotFound, jobID)
		}
		return Snapshot{}, fmt.Errorf("jobs: load job: %w", err)
	}
	if !State(job.State).Terminal() {
		return Snapshot{}, fmt.Errorf("%w: job %s is %s",
			ErrReplayExecutionRequired, job.ID, job.State)
	}
	// Read outside the transaction to tell "nothing was ever stored" apart from
	// "somebody purged it first"; the write itself is guarded, so a purge racing
	// this one still ends with the input gone.
	if _, err := replayEnvelope(deps.DB, job.ID); err != nil {
		return Snapshot{}, err
	}

	now := deps.now()
	err = deps.DB.Transaction(func(tx *gorm.DB) error {
		// The transaction's first statement is the write, for the same reason
		// every other write in this module is: on SQLite the writer lock must be
		// taken before anything is read.
		return tx.Model(&models.JobReplayEnvelope{}).
			Where("job_id = ? AND purged_at IS NULL", job.ID).
			Updates(map[string]any{
				"ciphertext":   gorm.Expr("NULL"),
				"nonce":        gorm.Expr("NULL"),
				"purged_at":    now,
				"purge_reason": models.JobReplayPurgeForgotten,
				"updated_at":   now,
			}).Error
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("jobs: forget replay input: %w", err)
	}
	return s.snapshotFor(deps, job), nil
}
