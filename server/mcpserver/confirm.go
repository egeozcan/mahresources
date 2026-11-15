package mcpserver

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

// confirmationTTL is how long a confirmation challenge stays valid.
const confirmationTTL = 5 * time.Minute

// challenge is what a confirmation's requestState carries. It is sealed with
// AES-256-GCM, so the client can neither read nor alter it.
type challenge struct {
	UserID   uint   `json:"u"`
	AuthHash string `json:"a"` // SHA-256 of the Authorization header
	Tool     string `json:"t"`
	OpID     string `json:"o"`
	ArgsHash string `json:"h"`
	Expires  int64  `json:"e"` // unix seconds
	Nonce    string `json:"n"`
}

// confirmer issues and redeems confirmation challenges.
//
// The key mixes in a nonce drawn at boot, so a restart invalidates every
// outstanding challenge. That is what lets the used-nonce set live in memory:
// a challenge can only be replayed within the process that issued it, and
// that process remembers it was used.
type confirmer struct {
	aead cipher.AEAD
	now  func() time.Time

	mu   sync.Mutex
	used map[string]int64 // nonce -> expiry (unix seconds)
}

func newConfirmer(signingKey []byte) (*confirmer, error) {
	boot := make([]byte, 32)
	if _, err := rand.Read(boot); err != nil {
		return nil, err
	}
	return newConfirmerWithBoot(signingKey, boot)
}

func newConfirmerWithBoot(signingKey, boot []byte) (*confirmer, error) {
	h := sha256.New()
	h.Write([]byte("mcp-confirm-v1"))
	h.Write(signingKey)
	h.Write(boot)
	block, err := aes.NewCipher(h.Sum(nil))
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &confirmer{aead: aead, now: time.Now, used: map[string]int64{}}, nil
}

// issue seals a new challenge for the given binding.
func (c *confirmer) issue(b challenge) (string, error) {
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	b.Nonce = hex.EncodeToString(nonce)
	b.Expires = c.now().Add(confirmationTTL).Unix()
	plain, err := json.Marshal(b)
	if err != nil {
		return "", err
	}
	iv := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}
	sealed := c.aead.Seal(iv, iv, plain, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

var (
	errChallengeInvalid  = errors.New("the confirmation is invalid or was issued before the server restarted")
	errChallengeExpired  = errors.New("the confirmation expired; call the tool again to get a new one")
	errChallengeMismatch = errors.New("the confirmation was issued for a different caller, operation or arguments")
	errChallengeUsed     = errors.New("the confirmation was already used")
)

// redeem checks state against want and, when everything matches, consumes the
// nonce. It is the only way a destructive call gets past confirmation, and it
// consumes before returning, so two concurrent retries cannot both succeed.
func (c *confirmer) redeem(state string, want challenge) error {
	raw, err := base64.RawURLEncoding.DecodeString(state)
	if err != nil || len(raw) < c.aead.NonceSize() {
		return errChallengeInvalid
	}
	iv, sealed := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]
	plain, err := c.aead.Open(nil, iv, sealed, nil)
	if err != nil {
		return errChallengeInvalid
	}
	var got challenge
	if err := json.Unmarshal(plain, &got); err != nil || got.Nonce == "" {
		return errChallengeInvalid
	}
	now := c.now().Unix()
	if now > got.Expires {
		return errChallengeExpired
	}
	if got.UserID != want.UserID || got.AuthHash != want.AuthHash || got.Tool != want.Tool ||
		got.OpID != want.OpID || got.ArgsHash != want.ArgsHash {
		return errChallengeMismatch
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for n, exp := range c.used {
		if now > exp {
			delete(c.used, n)
		}
	}
	if _, seen := c.used[got.Nonce]; seen {
		return errChallengeUsed
	}
	c.used[got.Nonce] = got.Expires
	return nil
}

// hashArgs returns a stable digest of the dispatched arguments. json.Marshal
// sorts map keys, and numbers decoded with UseNumber keep their exact text,
// so equal arguments always hash equally.
func hashArgs(args map[string]any) (string, error) {
	b, err := json.Marshal(args)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}
