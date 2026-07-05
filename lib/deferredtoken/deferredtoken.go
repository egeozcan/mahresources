// Package deferredtoken mints and verifies the signed tokens that back the
// [lazy] and [details] category-template shortcodes.
//
// A token carries the raw inner-template body of a deferred block together with
// the entity it must render against, authenticated by an HMAC-SHA256 signature
// over the whole payload. The signature proves that the server itself produced
// this exact (entityType, entityID, body) triple during a legitimate render, so
// the on-demand render endpoint can rebuild and render the body without trusting
// any client-supplied template text — a deferred render is provably identical to
// what would have rendered inline for the same principal on the same entity.
//
// The signing key lives on the running MahresourcesContext; see
// application_context for how it is generated (per-boot random by default, or a
// configured static key for multi-process deployments).
package deferredtoken

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"strings"
)

// payload is the signed body of a deferred-render token. Field names are kept
// short because the marshaled JSON travels inside every placeholder on the page.
type payload struct {
	T string `json:"t"` // entity type: "group", "resource", or "note"
	I uint   `json:"i"` // entity ID
	B string `json:"b"` // raw inner-template body to render on demand
}

// b64 is URL-safe base64 without padding, so tokens are safe in HTML attributes
// and request bodies without escaping.
var b64 = base64.RawURLEncoding

// Sign returns a token that encodes (entityType, entityID, body) and is
// authenticated with key. The returned string is "<b64(payload)>.<b64(hmac)>".
func Sign(key []byte, entityType string, entityID uint, body string) string {
	raw, err := json.Marshal(payload{T: entityType, I: entityID, B: body})
	if err != nil {
		// payload is a plain struct of marshalable fields — marshaling cannot
		// realistically fail, but never emit a half-formed token.
		return ""
	}
	return b64.EncodeToString(raw) + "." + b64.EncodeToString(sign(key, raw))
}

// Verify checks token against key and returns the encoded (entityType, entityID,
// body). ok is false when the token is malformed or its signature does not match,
// in which case the string/uint returns are zero values and must not be used.
func Verify(key []byte, token string) (entityType string, entityID uint, body string, ok bool) {
	dot := strings.IndexByte(token, '.')
	if dot <= 0 || dot == len(token)-1 {
		return "", 0, "", false
	}

	raw, err := b64.DecodeString(token[:dot])
	if err != nil {
		return "", 0, "", false
	}
	gotSig, err := b64.DecodeString(token[dot+1:])
	if err != nil {
		return "", 0, "", false
	}

	wantSig := sign(key, raw)
	if subtle.ConstantTimeCompare(gotSig, wantSig) != 1 {
		return "", 0, "", false
	}

	var p payload
	if err := json.Unmarshal(raw, &p); err != nil {
		return "", 0, "", false
	}
	return p.T, p.I, p.B, true
}

func sign(key, raw []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(raw)
	return mac.Sum(nil)
}
