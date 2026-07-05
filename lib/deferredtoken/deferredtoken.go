// Package deferredtoken mints and opens the tokens that back the [lazy] and
// [details] category-template shortcodes.
//
// A token carries the raw inner-template body of a deferred block together with
// the entity it must render against. The token is produced with authenticated
// encryption (AES-256-GCM), which gives two properties the deferred-render
// endpoint relies on:
//
//   - Confidentiality: the token is emitted into a data-token attribute on the
//     rendered page, so it must not leak the template source (conditional
//     branches, MRQL text, plugin attrs). The body is encrypted, so a reader of
//     the page cannot recover it.
//   - Integrity/authenticity: Open fails on any tampering or a wrong key, so the
//     endpoint only ever renders (entityType, entityID, body) triples the server
//     itself produced — no client-supplied template text is trusted.
//
// The key lives on the running MahresourcesContext; see application_context for
// how it is derived (per-boot random by default, or a configured static key for
// multi-process deployments). Any key length is accepted — it is hashed to a
// fixed 32-byte AES-256 key internally.
package deferredtoken

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
)

// payload is the sealed body of a deferred-render token. Field names are kept
// short because the encrypted payload travels inside every placeholder on the page.
type payload struct {
	T string `json:"t"` // entity type: "group", "resource", or "note"
	I uint   `json:"i"` // entity ID
	B string `json:"b"` // raw inner-template body to render on demand
}

// b64 is URL-safe base64 without padding, so tokens are safe in HTML attributes
// and request bodies without escaping.
var b64 = base64.RawURLEncoding

// aeadFor builds an AES-256-GCM AEAD from key. The key is hashed to a fixed 32
// bytes so any key length is accepted and always yields a valid AES-256 key.
func aeadFor(key []byte) (cipher.AEAD, error) {
	sum := sha256.Sum256(key)
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Seal encrypts (entityType, entityID, body) into an opaque, authenticated token.
// Returns "" only if encryption cannot be performed, which does not happen with a
// valid AES-GCM AEAD and a working CSPRNG.
func Seal(key []byte, entityType string, entityID uint, body string) string {
	raw, err := json.Marshal(payload{T: entityType, I: entityID, B: body})
	if err != nil {
		return ""
	}
	aead, err := aeadFor(key)
	if err != nil {
		return ""
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return ""
	}
	// Seal prepends the nonce to the ciphertext+tag so Open can recover it.
	sealed := aead.Seal(nonce, nonce, raw, nil)
	return b64.EncodeToString(sealed)
}

// Open decrypts and authenticates a token produced by Seal, returning the encoded
// (entityType, entityID, body). ok is false when the token is malformed, tampered
// with, or was sealed under a different key; the other returns are then zero
// values and must not be used.
func Open(key []byte, token string) (entityType string, entityID uint, body string, ok bool) {
	sealed, err := b64.DecodeString(token)
	if err != nil {
		return "", 0, "", false
	}
	aead, err := aeadFor(key)
	if err != nil {
		return "", 0, "", false
	}
	ns := aead.NonceSize()
	if len(sealed) < ns {
		return "", 0, "", false
	}
	nonce, ciphertext := sealed[:ns], sealed[ns:]
	raw, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", 0, "", false
	}
	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", 0, "", false
	}
	return p.T, p.I, p.B, true
}
