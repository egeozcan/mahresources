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
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mahresources/constants"
	"mahresources/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

// KeyIDs returns the stable, public identifiers of the keys this process can
// open. SQL read paths use the same set to answer replay availability without
// probing or decrypting one envelope per Job.
func (k *Keyring) KeyIDs() []string {
	if k == nil {
		return nil
	}
	ids := make([]string, 0, len(k.keys))
	for id := range k.keys {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
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
	return publishReplayKeyFile(path, generated, syncReplayKeyDirectory)
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

// replayKeyDirSync makes the directory entry that names a published file durable:
// it flushes the directory itself. It is the step file-content syncing does not
// take, and the one this file cannot do without.
//
// It is a parameter of publishReplayKeyFile rather than a direct call because the
// property worth testing is the publication's *response* to a directory that
// cannot be flushed, and no real filesystem can be made to fail at that instant.
type replayKeyDirSync func(dir string) error

// syncReplayKeyDirectory flushes one directory, so the entry naming a file linked
// into it survives a power failure.
//
// The platform errors that mean "this filesystem does not flush directories" are
// tolerated: the guarantee is weaker there for every file on it, and refusing to
// start a deployment over a capability the filesystem does not have would be a
// worse answer than the one it already gives. Everything else is a refusal, because
// an entry that could not be flushed is a key that might not be there next boot.
func syncReplayKeyDirectory(dir string) error {
	handle, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("jobs: open %s to flush the replay key file's name: %w", dir, err)
	}
	defer handle.Close()

	if err := handle.Sync(); err != nil {
		if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.ENOSYS) {
			return nil
		}
		return fmt.Errorf("jobs: flush %s after publishing the replay key file: %w", dir, err)
	}
	return nil
}

// publishReplayKeyFile writes a fresh key to its final name, atomically and only
// if that name is still free, makes that name durable, and returns the key the
// name now holds.
//
// The return value is the point. When the name was taken, the process that lost
// the race must adopt the winner's key — returning its own would give one database
// two keys, and every envelope written by the other process would be unreadable
// here — and it flushes the directory it adopted the key from too, because that
// entry is now what its own envelopes depend on.
func publishReplayKeyFile(path string, key ReplayKey, flush replayKeyDirSync) (ReplayKey, error) {
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
	// to a power cut would leave envelopes nothing can open. The name itself is
	// flushed below — this makes the bytes durable, that makes the entry durable,
	// and neither is enough alone.
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
			// Another process won the race. Its key is the deployment's key, and its
			// entry is the one to flush: this process is about to seal envelopes that
			// only that file can open.
			if err := flush(dir); err != nil {
				return ReplayKey{}, err
			}
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

	// The name exists; now it has to survive. The file's contents were synced
	// before the link, and the directory is flushed after it, so a power failure
	// leaves either no key file or the whole of it — never bytes and no entry.
	if err := flush(dir); err != nil {
		return ReplayKey{}, err
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
		// As for Decode at the other end of the envelope: the codec's own text is
		// deliberately not carried. Sanitize runs on the input the caller just
		// *submitted*, in the clear, so its error can quote a URL query string, a
		// Cookie header or a plugin value — and this error reaches Accept's caller
		// and the runtime log. The classification is what a reader acts on, and the
		// Kind and version are what an operator needs to find it.
		return models.JobReplayEnvelope{}, nil, fmt.Errorf("%w: %s v%d could not be sanitized",
			ErrInvalidReplay, job.Kind, job.KindVersion)
	}
	if err := validateReplayJSON("summary", summary, MaxSummaryBytes); err != nil {
		return models.JobReplayEnvelope{}, nil, err
	}

	payload, err := codec.Encode(input.Input)
	if err != nil {
		// The same, for the hook that produces the bytes about to be encrypted.
		return models.JobReplayEnvelope{}, nil, fmt.Errorf("%w: %s v%d could not be encoded",
			ErrInvalidReplay, job.Kind, job.KindVersion)
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

// requireReplayCodec resolves the codec registered for exactly this Kind
// version, or refuses with the reason no codec is available.
//
// It deliberately does not fall back to the newest registered version, which is
// what replayCodecFor does for *reading*. Sealing under a borrowed encoder would
// store one version's bytes under another version's label, and every later read
// trusts that label: OpenReplay would migrate bytes that are already the newer
// shape as though they were the older one. Migration is for envelopes that
// already exist, never for input being accepted now.
func (s *Service) requireReplayCodec(kind string, version uint) (ReplayCodec, error) {
	s.replayMu.Lock()
	codec, ok := s.replayCodecs[replayCodecKey{kind: kind, version: version}]
	s.replayMu.Unlock()
	if !ok {
		return ReplayCodec{}, fmt.Errorf("%w: %s v%d", ErrReplayCodecUnregistered, kind, version)
	}
	return codec, nil
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
	// The nonce length is checked here because the AEAD does not return an
	// error for a wrong one: crypto/cipher panics. A truncated, NULL or
	// oversized nonce in the row is a corrupt envelope like any other, and it
	// must be that refusal rather than a panic that takes the runtime goroutine
	// with it.
	if len(envelope.Nonce) != aead.NonceSize() {
		return nil, fmt.Errorf("%w: envelope %s carries a %d-byte nonce, want %d",
			ErrReplayCorrupt, envelope.JobID, len(envelope.Nonce), aead.NonceSize())
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
// not lose its input while it was still running. The window is the one in effect
// at that instant, not the one the caller's handle was built with, for the same
// reason the Job's own deadline is: an execution outlives the setting it started
// under. A Job with no envelope, a purged one, or a retention of zero ("not
// configured", never "expire now") is left exactly as it is.
func stampReplayExpiry(tx *gorm.DB, job models.Job, retention time.Duration, now time.Time) error {
	if retention <= 0 || job.FinishedAt == nil {
		return nil
	}
	expires := job.FinishedAt.Add(retention).UTC()
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
	return s.replayAvailabilityFrom(keys, envelope, now)
}

// replayAvailabilityFrom answers the same question from an already-loaded
// envelope row, so a listing can answer a whole page with one query instead of
// one per Job. The two answers cannot drift because there is one implementation
// and the loader is the only difference.
func (s *Service) replayAvailabilityFrom(keys *Keyring, envelope models.JobReplayEnvelope, now time.Time) ReplayAvailability {
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
		if errors.Is(err, ErrReplayAbsent) {
			// The Job's durable class says its input is replayable, so the
			// absence is not "nothing to run with" but "the input this Job
			// requires is gone". The two are different answers, and only the
			// second one blocks dispatch.
			return OpenedReplay{}, fmt.Errorf("%w: job %s", ErrReplayEnvelopeMissing, job.ID)
		}
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
			// As for Decode: the migration runs on decrypted bytes, so its error
			// text may quote them.
			return OpenedReplay{}, fmt.Errorf("%w: %s v%d could not be migrated to v%d",
				ErrReplayDecodeFailed, envelope.Kind, envelope.KindVersion, targetVersion)
		}
		plaintext = migrated
		from := envelope.KindVersion
		migratedFrom = &from
	}

	decoded, err := codec.Decode(plaintext, targetVersion)
	if err != nil {
		// The codec's own text is deliberately not carried: Decode runs on the
		// *decrypted* input, so its error can quote a URL, a Cookie header or a
		// plugin value — and this error reaches runtime logs and the caller that
		// asked to run the Job. The classification is what a reader acts on, and
		// the Kind and version are what an operator needs to find it.
		return OpenedReplay{}, fmt.Errorf("%w: %s v%d could not be decoded",
			ErrReplayDecodeFailed, envelope.Kind, targetVersion)
	}
	if err := validateReplayJSON("input", decoded, MaxReplayPayloadBytes); err != nil {
		// A codec that succeeds and still produces something no executor may run
		// with — nothing at all, something that is not JSON, something over the
		// ceiling — is a decode failure in the sense §5 means, so it is classified
		// as one rather than left as a validation error the dispatch seam does not
		// recognise and would therefore let run on with no input.
		return OpenedReplay{}, fmt.Errorf("%w: %s v%d produced input no executor may run with: %v",
			ErrReplayDecodeFailed, envelope.Kind, targetVersion, err)
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
		errors.Is(err, ErrReplayDecodeFailed),
		errors.Is(err, ErrReplayEnvelopeMissing):
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
// Each batch is selected before the transaction, then its guarded update is the
// transaction's first statement: on SQLite the writer lock is taken before
// anything is read, which keeps the sweep from promoting a read snapshot after
// another connection committed. The candidate IDs are bounded, and all
// eligibility predicates are checked again by the update so stale choices are
// harmless.
func (s *Service) PurgeExpiredReplay(deps Deps, limit int) (int, error) {
	if limit <= 0 {
		limit = DefaultReplayPurgeBatch
	}
	now := deps.now()
<<<<<<< HEAD
	// Candidate discovery may happen before the transaction. The guarded UPDATE
	// below rechecks every expiry and terminal-state predicate after it has taken
	// SQLite's writer lock, so this snapshot never authorizes a purge on its own.
	var candidateIDs []string
	if err := deps.DB.Model(&models.JobReplayEnvelope{}).
		Select("job_replay_envelopes.job_id").
		Where("job_replay_envelopes.purged_at IS NULL").
		Where("job_replay_envelopes.expires_at IS NOT NULL AND job_replay_envelopes.expires_at <= ?", now).
		Where("EXISTS (SELECT 1 FROM jobs WHERE jobs.id = job_replay_envelopes.job_id AND jobs.state IN ?)", terminalStates()).
		Order("job_replay_envelopes.expires_at, job_replay_envelopes.job_id").
		Limit(limit).Pluck("job_replay_envelopes.job_id", &candidateIDs).Error; err != nil {
		return 0, fmt.Errorf("jobs: select expired replay input: %w", err)
	}
	if len(candidateIDs) == 0 {
		return 0, nil
	}
	purge := map[string]any{
		"ciphertext":   gorm.Expr("NULL"),
		"nonce":        gorm.Expr("NULL"),
		"purged_at":    now,
		"purge_reason": models.JobReplayPurgeExpired,
		"updated_at":   now,
	}

	purged := int64(0)
	err := deps.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&models.JobReplayEnvelope{}).
			Where("job_replay_envelopes.job_id IN ?", candidateIDs).
			Where("job_replay_envelopes.purged_at IS NULL").
			Where("job_replay_envelopes.expires_at IS NOT NULL AND job_replay_envelopes.expires_at <= ?", now).
			Where("EXISTS (SELECT 1 FROM jobs WHERE jobs.id = job_replay_envelopes.job_id AND jobs.state IN ?)", terminalStates()).
			Updates(purge)
		if result.Error != nil {
			return fmt.Errorf("jobs: purge expired replay input: %w", result.Error)
		}
		purged = result.RowsAffected
		var purgedIDs []string
		if err := tx.Model(&models.JobReplayEnvelope{}).
			Where("job_id IN ?", candidateIDs).
			Where("purged_at = ? AND purge_reason = ?", now, models.JobReplayPurgeExpired).
			Pluck("job_id", &purgedIDs).Error; err != nil {
			return fmt.Errorf("jobs: read purged replay input: %w", err)
		}
		for _, jobID := range purgedIDs {
			if err := purgeLegacyReplaySourcesTx(tx, jobID, models.JobReplayPurgeExpired, now); err != nil {
				return fmt.Errorf("jobs: purge legacy replay source: %w", err)
			}
		}
		if deps.PurgeReplayDerivedFacts != nil && len(purgedIDs) != 0 {
			if err := deps.PurgeReplayDerivedFacts(tx, purgedIDs); err != nil {
				return fmt.Errorf("jobs: purge expired replay-derived facts: %w", err)
			}
		}
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
		if err := tx.Model(&models.JobReplayEnvelope{}).
			Where("job_id = ? AND purged_at IS NULL", job.ID).
			Updates(map[string]any{
				"ciphertext":   gorm.Expr("NULL"),
				"nonce":        gorm.Expr("NULL"),
				"purged_at":    now,
				"purge_reason": models.JobReplayPurgeForgotten,
				"updated_at":   now,
			}).Error; err != nil {
			return err
		}
		var envelope models.JobReplayEnvelope
		if err := tx.Where("job_id = ?", job.ID).First(&envelope).Error; err != nil {
			return err
		}
		if err := purgeLegacyReplaySourcesTx(tx, job.ID, envelope.PurgeReason, now); err != nil {
			return err
		}
		if deps.PurgeReplayDerivedFacts != nil {
			return deps.PurgeReplayDerivedFacts(tx, []string{job.ID})
		}
		return nil
	})
	if err != nil {
		return Snapshot{}, fmt.Errorf("jobs: forget replay input: %w", err)
	}
	return s.snapshotFor(deps, access, job), nil
}

// purgeLegacyReplaySourcesTx removes every mapped or resolvable unmapped
// plaintext replay copy in the same transaction that marks the canonical
// envelope purged. Source mappings are the normal bridge between pre-Job rows
// and a canonical Job, but a mixed writer may have committed a legacy source and
// its canonical handle/JobID before it committed the mapping. Those rows are
// resolved and given a durable purged mapping here so a later migration cannot
// reconstruct input that Forget or expiry already removed.
func purgeLegacyReplaySourcesTx(tx *gorm.DB, jobID, reason string, now time.Time) error {
	if !tx.Migrator().HasTable(&models.JobSourceMapping{}) {
		return errors.New("legacy replay source mapping schema is unavailable")
	}
	if !tx.Migrator().HasTable(&models.JobLegacyHandle{}) {
		return errors.New("legacy Job handle schema is unavailable")
	}
	var mappings []models.JobSourceMapping
	if err := tx.Where("job_id = ?", jobID).Limit(maxLegacyReplaySourcesPerJob + 1).Find(&mappings).Error; err != nil {
		return err
	}
	if len(mappings) > maxLegacyReplaySourcesPerJob {
		return errors.New("too many mapped legacy replay sources to purge safely")
	}
	mapped := make(map[string]struct{}, len(mappings))
	for _, mapping := range mappings {
		mapped[legacyReplaySourceKey(mapping.SourceKind, mapping.SourceID)] = struct{}{}
		updates := map[string]any{"updated_at": now}
		mappingReason := reason
		if err := clearLegacyReplaySourceTx(tx, mapping.SourceKind, mapping.SourceID); err != nil {
			return err
		}
		if mapping.Status == models.JobSourceMappingPurged && mapping.PurgeReason != "" {
			mappingReason = mapping.PurgeReason
		}
		purgedAt := now
		updates["status"] = models.JobSourceMappingPurged
		updates["purged_at"] = purgedAt
		updates["purge_reason"] = mappingReason
		updates["scrubbed_at"] = purgedAt
		updates["post_scrub_hash"] = ""
		updates["blocker_code"] = ""
		if err := tx.Model(&models.JobSourceMapping{}).
			Where("source_kind = ? AND source_id = ?", mapping.SourceKind, mapping.SourceID).
			Updates(updates).Error; err != nil {
			return err
		}
	}

	unmapped, err := findUnmappedLegacyReplaySourcesTx(tx, jobID)
	if err != nil {
		return err
	}
	for _, source := range unmapped {
		key := legacyReplaySourceKey(source.kind, source.id)
		if _, ok := mapped[key]; ok {
			continue
		}
		var existing models.JobSourceMapping
		err := tx.Where("source_kind = ? AND source_id = ?", source.kind, source.id).First(&existing).Error
		if err == nil {
			if existing.JobID != jobID {
				return errors.New("legacy replay source is mapped to a different Job")
			}
			// A mapping committed after the initial scan. Its source is still
			// cleared below, and the existing marker is refreshed after the update.
			if err := clearLegacyReplaySourceTx(tx, source.kind, source.id); err != nil {
				return err
			}
			at := now
			if err := tx.Model(&models.JobSourceMapping{}).
				Where("source_kind = ? AND source_id = ? AND job_id = ?", source.kind, source.id, jobID).
				Updates(map[string]any{
					"status": models.JobSourceMappingPurged, "purged_at": at, "purge_reason": reason,
					"scrubbed_at": at, "post_scrub_hash": "", "blocker_code": "", "updated_at": at,
				}).Error; err != nil {
				return err
			}
			mapped[key] = struct{}{}
			continue
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := clearLegacyReplaySourceTx(tx, source.kind, source.id); err != nil {
			return err
		}
		at := now
		marker := models.JobSourceMapping{
			SourceKind: source.kind, SourceID: source.id, JobID: jobID, SourceRevision: 1,
			Status: models.JobSourceMappingPurged, Origin: models.JobSourceOriginBackfilled,
			CopiedAt: at, ScrubbedAt: &at, PurgedAt: &at, PurgeReason: reason,
			CreatedAt: at, UpdatedAt: at,
		}
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&marker).Error; err != nil {
			return err
		}
		var saved models.JobSourceMapping
		if err := tx.Where("source_kind = ? AND source_id = ?", source.kind, source.id).First(&saved).Error; err != nil {
			return err
		}
		if saved.JobID != jobID || saved.Status != models.JobSourceMappingPurged || saved.PurgedAt == nil {
			return errors.New("legacy replay purge marker conflicts with another Job")
		}
		mapped[key] = struct{}{}
	}
	return nil
}

const maxLegacyReplaySourcesPerJob = 10000

type legacyReplaySourceRef struct {
	kind string
	id   string
}

func legacyReplaySourceKey(kind, id string) string { return kind + "\x00" + id }

func clearLegacyReplaySourceTx(tx *gorm.DB, kind, id string) error {
	switch kind {
	case "download-history":
		return tx.Model(&models.DownloadHistoryEntry{}).Where("id = ?", id).
			Updates(map[string]any{"payload": nil, "url": ""}).Error
	case "scheduled-download":
		return tx.Model(&models.ScheduledDownload{}).Where("id = ?", id).
			Updates(map[string]any{"payload": nil, "url": ""}).Error
	case "plugin-command-run":
		return tx.Model(&models.PluginCommandRun{}).Where("id = ?", id).
			Updates(map[string]any{"params_json": "", "inputs_json": ""}).Error
	case "plugin-command-import":
		return tx.Model(&models.PluginCommandImport{}).Where("id = ?", id).
			Updates(map[string]any{"fields_json": ""}).Error
	case "resource-reduction":
		return nil
	default:
		return errors.New("unknown legacy replay source kind")
	}
}

func findUnmappedLegacyReplaySourcesTx(tx *gorm.DB, jobID string) ([]legacyReplaySourceRef, error) {
	var handles []models.JobLegacyHandle
	if err := tx.Where("job_id = ?", jobID).Order("namespace ASC, handle ASC").
		Limit(maxLegacyReplaySourcesPerJob + 1).Find(&handles).Error; err != nil {
		return nil, err
	}
	if len(handles) > maxLegacyReplaySourcesPerJob {
		return nil, errors.New("too many legacy handles to prove replay purge safely")
	}
	byNamespace := make(map[string][]string)
	for _, handle := range handles {
		byNamespace[handle.Namespace] = append(byNamespace[handle.Namespace], handle.Handle)
	}
	var result []legacyReplaySourceRef
	add := func(kind string, ids []string) error {
		if len(ids) > maxLegacyReplaySourcesPerJob {
			return errors.New("too many unmapped legacy replay sources to purge safely")
		}
		for _, id := range ids {
			result = append(result, legacyReplaySourceRef{kind: kind, id: id})
		}
		return nil
	}

	if tx.Migrator().HasTable(&models.DownloadHistoryEntry{}) {
		var ids []uint
		query := tx.Model(&models.DownloadHistoryEntry{}).Select("id").Where("job_id = ?", jobID)
		if values := byNamespace["download"]; len(values) > 0 {
			query = tx.Model(&models.DownloadHistoryEntry{}).Select("id").Where("job_id = ? OR job_id IN ?", jobID, values)
		}
		if err := query.Order("id ASC").Limit(maxLegacyReplaySourcesPerJob+1).Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		sourceIDs := make([]string, len(ids))
		for i, id := range ids {
			sourceIDs[i] = strconv.FormatUint(uint64(id), 10)
		}
		if err := add("download-history", sourceIDs); err != nil {
			return nil, err
		}
	}
	if tx.Migrator().HasTable(&models.ScheduledDownload{}) {
		var handlesAsIDs []uint
		for _, handle := range byNamespace["scheduled-download"] {
			id, err := strconv.ParseUint(handle, 10, 64)
			if err != nil || id == 0 {
				return nil, errors.New("scheduled download handle is invalid")
			}
			handlesAsIDs = append(handlesAsIDs, uint(id))
		}
		var candidates []struct {
			ID    uint
			JobID string
		}
		query := tx.Model(&models.ScheduledDownload{}).Select("id", "job_id")
		downloadHandles := byNamespace["download"]
		if len(handlesAsIDs) > 0 && len(downloadHandles) > 0 {
			query = query.Where("id IN ? OR job_id IN ?", handlesAsIDs, downloadHandles)
		} else if len(handlesAsIDs) > 0 {
			query = query.Where("id IN ?", handlesAsIDs)
		} else if len(downloadHandles) > 0 {
			query = query.Where("job_id IN ?", downloadHandles)
		}
		if len(handlesAsIDs) > 0 || len(downloadHandles) > 0 {
			if err := query.Order("id ASC").Limit(maxLegacyReplaySourcesPerJob + 1).Scan(&candidates).Error; err != nil {
				return nil, err
			}
			if len(candidates) > maxLegacyReplaySourcesPerJob {
				return nil, errors.New("too many scheduled downloads to prove replay purge safely")
			}
		}
		candidateHandles := make([]string, len(candidates))
		for i, candidate := range candidates {
			candidateHandles[i] = strconv.FormatUint(uint64(candidate.ID), 10)
		}
		var scheduledOwners []models.JobLegacyHandle
		if len(candidateHandles) > 0 {
			if err := tx.Where("namespace = ? AND handle IN ?", "scheduled-download", candidateHandles).
				Limit(maxLegacyReplaySourcesPerJob + 1).Find(&scheduledOwners).Error; err != nil {
				return nil, err
			}
			if len(scheduledOwners) > maxLegacyReplaySourcesPerJob {
				return nil, errors.New("too many scheduled download handles to prove replay purge safely")
			}
		}
		ownerByHandle := make(map[string]string, len(scheduledOwners))
		for _, owner := range scheduledOwners {
			ownerByHandle[owner.Handle] = owner.JobID
		}
		downloadHandleSet := make(map[string]struct{}, len(downloadHandles))
		for _, handle := range downloadHandles {
			downloadHandleSet[handle] = struct{}{}
		}
		sourceIDs := make([]string, 0, len(candidates))
		for _, candidate := range candidates {
			handle := strconv.FormatUint(uint64(candidate.ID), 10)
			if ownerJobID, hasScheduledOwner := ownerByHandle[handle]; hasScheduledOwner {
				if ownerJobID == jobID {
					sourceIDs = append(sourceIDs, handle)
				}
				continue
			}
			if _, resolvesThroughDownload := downloadHandleSet[candidate.JobID]; !resolvesThroughDownload {
				continue
			}
			// Migration gives a row's scheduled-download handle precedence over
			// its JobID download-handle fallback. Only adopt the fallback when
			// the row has no scheduled handle at all.
			sourceIDs = append(sourceIDs, handle)
		}
		if err := add("scheduled-download", sourceIDs); err != nil {
			return nil, err
		}
	}
	if tx.Migrator().HasTable(&models.PluginCommandRun{}) {
		var ids []string
		query := tx.Model(&models.PluginCommandRun{}).Select("id").Where("job_id = ?", jobID)
		if values := byNamespace["plugin-command-run"]; len(values) > 0 {
			query = tx.Model(&models.PluginCommandRun{}).Select("id").Where("job_id = ? OR id IN ?", jobID, values)
		}
		if err := query.Order("id ASC").Limit(maxLegacyReplaySourcesPerJob+1).Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		if err := add("plugin-command-run", ids); err != nil {
			return nil, err
		}
	}
	if tx.Migrator().HasTable(&models.PluginCommandImport{}) {
		var ids []string
		query := tx.Model(&models.PluginCommandImport{}).Select("id").Where("job_id = ?", jobID)
		if values := byNamespace["plugin-command-import"]; len(values) > 0 {
			query = tx.Model(&models.PluginCommandImport{}).Select("id").Where("job_id = ? OR id IN ?", jobID, values)
		}
		if err := query.Order("id ASC").Limit(maxLegacyReplaySourcesPerJob+1).Pluck("id", &ids).Error; err != nil {
			return nil, err
		}
		if err := add("plugin-command-import", ids); err != nil {
			return nil, err
		}
	}
	return result, nil
}

// HasReplayCodec reports whether any registered codec could decode an envelope
// written at one Kind version.
//
// It is exported because registration is a wiring step that may legitimately run
// twice — a context registering its Kinds against a service it was handed — and
// the second caller has to be able to ask whether the pair is already there
// instead of provoking the refusal that guards against two codecs for one
// version.
func HasReplayCodec(s *Service, kind string, version uint) bool {
	if s == nil {
		return false
	}
	return s.hasReplayCodec(kind, version)
}
