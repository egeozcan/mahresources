package plugin_system

import (
	"net/http"
	"testing"
)

// TestAPluginNeverSeesTheCallersCredentials.
//
// Without this, a plugin serving a mah.api endpoint or a mah.page is a confused
// deputy: it receives whatever the browser or CLI sent, and the Authorization
// header or session cookie is enough to call this server's own API as that
// user. POST /v1/download/submit or /v1/resource/remote with an internal URL,
// and the app — which carries no plugin egress policy on those paths — fetches
// the cloud metadata endpoint and persists the answer as a resource the plugin
// can read back. That defeats the manifest's network allowlist entirely, and
// needs no db:write either, because the app does the writing.
func TestAPluginNeverSeesTheCallersCredentials(t *testing.T) {
	header := http.Header{}
	header.Set("Authorization", "Bearer sk-live-do-not-leak")
	header.Set("Cookie", "mahresources_session=abcdef123456")
	header.Set("X-CSRF-Token", "csrf-abcdef")
	header.Set("Proxy-Authorization", "Basic cHJveHk6c2VjcmV0")
	header.Set("Content-Type", "application/json")
	header.Set("User-Agent", "mr/1.0")
	header.Add("X-Plugin-Custom", "one")
	header.Add("X-Plugin-Custom", "two")

	safe := SafeRequestHeaders(header)

	for _, withheld := range []string{"authorization", "cookie", "x-csrf-token", "proxy-authorization"} {
		if v, present := safe[withheld]; present {
			t.Errorf("%s reached the plugin: %v", withheld, v)
		}
	}

	// Nothing else may be lost: plugins read their own callers' headers, and a
	// redaction that took the rest would break every one of them.
	if got := safe["content-type"]; got != "application/json" {
		t.Errorf("content-type = %v, want application/json", got)
	}
	if got := safe["user-agent"]; got != "mr/1.0" {
		t.Errorf("user-agent = %v, want mr/1.0", got)
	}
	multi, ok := safe["x-plugin-custom"].([]any)
	if !ok || len(multi) != 2 {
		t.Fatalf("a repeated custom header was not preserved as a list: %#v", safe["x-plugin-custom"])
	}
}

// TestCredentialRedactionIsCaseInsensitive: http.Header canonicalises on Set,
// but a header can also arrive on the wire in any case, and Go preserves what
// it was given when the key is set directly on the map.
func TestCredentialRedactionIsCaseInsensitive(t *testing.T) {
	header := http.Header{
		"AUTHORIZATION": []string{"Bearer leak"},
		"CoOkIe":        []string{"session=leak"},
	}
	safe := SafeRequestHeaders(header)
	if len(safe) != 0 {
		t.Fatalf("credential headers survived a case difference: %#v", safe)
	}
}

// TestRedactionCoversThisAppsCredentialSurface pins the list against what this
// application actually authenticates with, so a new credential header cannot be
// added to the app without someone deciding whether a plugin may see it.
func TestRedactionCoversThisAppsCredentialSurface(t *testing.T) {
	// Bearer token (the mr CLI), session cookie (the browser), and the
	// per-session synchronizer token. See CLAUDE.md, "Authentication & roles".
	for _, name := range []string{"authorization", "cookie", "x-csrf-token"} {
		if !credentialHeaders[name] {
			t.Errorf("%s is one of this app's credentials but is not withheld from plugins", name)
		}
	}
}

// TestTheCSRFTokenIsWithheldInEverySpellingItIsAccepted.
//
// server/csrf.go accepts the token three ways: the X-CSRF-Token header, a
// csrf_token query parameter (native multipart upload forms cannot set a
// header), and a csrf_token urlencoded form field. Withholding only the header
// leaves the stated invariant — a plugin never sees the caller's credentials —
// false for the other two, while the query string is handed over in full.
func TestTheCSRFTokenIsWithheldInEverySpellingItIsAccepted(t *testing.T) {
	query := map[string]any{
		"csrf_token": "tok-do-not-leak",
		"page":       "2",
	}
	if got := StripCredentialParams(query); len(got) != 1 || got["page"] != "2" {
		t.Fatalf("expected only the credential removed, got %#v", got)
	}

	form := map[string]any{"CSRF_TOKEN": "tok", "title": "hello"}
	stripped := StripCredentialParams(form)
	if _, present := stripped["CSRF_TOKEN"]; present {
		t.Error("the form field survived a case difference")
	}
	if stripped["title"] != "hello" {
		t.Error("an ordinary form field was removed")
	}
}

// TestAPluginNeverSeesAnEntitysShareToken.
//
// entityToMap walks every exported field reflectively, so a credential added to
// a model reaches plugins by default. Note.ShareToken is a bearer token
// granting anonymous read of that note and its resources — and a shortcode
// needs only `render`, the least privileged capability that sees an entity at
// all, to be handed one.
func TestAPluginNeverSeesAnEntitysShareToken(t *testing.T) {
	token := "sharetoken-do-not-leak"
	entity := struct {
		ID           uint
		Name         string
		ShareToken   *string
		TokenHash    string
		CsrfToken    string
		PasswordHash string
	}{ID: 7, Name: "visible", ShareToken: &token,
		TokenHash: "hash", CsrfToken: "csrf", PasswordHash: "pw"}

	got := entityToMap(&entity)

	if got["Name"] != "visible" {
		t.Fatalf("ordinary fields were lost: %#v", got)
	}
	if got["ID"] == nil {
		t.Error("ID was lost")
	}
	for _, withheld := range []string{"ShareToken", "TokenHash", "CsrfToken", "PasswordHash"} {
		if v, present := got[withheld]; present {
			t.Errorf("%s reached the plugin: %v", withheld, v)
		}
	}
}
