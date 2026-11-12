package server

import (
	"net/http"
	"strings"
	"testing"
)

// The shares dashboard lists every shared note in the caller's scope with its
// public /s/<token> URL, so it is an editor surface for reads as well as writes
// (#120). It used to sit in isEditorPath, whose safe-method downgrade turned
// every GET into capRead, so the entry gated nothing: the page is GET-only.
func TestAdminSharesPageRequiresEditor(t *testing.T) {
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost} {
		for _, path := range []string{"/admin/shares", "/admin/shares.json", "/admin/shares.body"} {
			if got := requiredCapability(method, path); got != capEditor {
				t.Errorf("requiredCapability(%s %s) = %v, want capEditor", method, path, got)
			}
		}
	}
	if got := requiredCapability(http.MethodPost, "/v1/admin/shares/bulk-revoke"); got != capEditor {
		t.Errorf("requiredCapability(POST /v1/admin/shares/bulk-revoke) = %v, want capEditor", got)
	}
}

// No /admin page may resolve to capRead. A page path matched by none of the
// gated branches falls through to the default, where a GET is safe and capRead
// lets every authenticated role (guests included) read it, and nothing fails
// for the admin who built it. This sweeps the template registry so the next
// /admin page fails here instead of shipping open.
func TestNoAdminTemplatePageIsCapRead(t *testing.T) {
	checked := 0
	for path := range templates {
		if !strings.HasPrefix(path, "/admin/") {
			continue
		}
		checked++
		if got := requiredCapability(http.MethodGet, path); got == capRead {
			t.Errorf("GET %s resolves to capRead, so every role can read it — list it in isSystemPath or give it an explicit case in requiredCapability (server/authz_policy.go)", path)
		}
	}
	if checked == 0 {
		t.Fatal("no /admin/ template routes found; this test measured nothing")
	}
}
