package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"
)

// The help text promises that --scope-group 0 clears the scope. Assert the wire
// contract that makes it true, not the help string: an "omit zero values"
// cleanup of the presence check would silently turn the documented clear into a
// no-op while leaving the help text, and any test of it, untouched. Help-text
// freshness is already covered by `mr docs lint` in CI.
func TestUserUpdateScopeGroupZeroRequestsAClear(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ID":4,"username":"kept","role":"editor","scopeGroupId":null,"disabled":false}`))
	}))
	defer server.Close()

	cmd := newUserUpdateCmd(client.New(server.URL), &output.Options{Quiet: true})
	cmd.SetArgs([]string{"4", "--scope-group", "0"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute user update: %v", err)
	}
	scope, present := gotBody["scopeGroupId"]
	if !present {
		t.Fatalf("--scope-group 0 must send scopeGroupId so the server clears it, got body %v", gotBody)
	}
	if scope != float64(0) {
		t.Fatalf("scopeGroupId=%v, want 0 (the server maps zero to a cleared scope)", scope)
	}
}

func TestUserUpdateSendsOnlyExplicitFields(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if r.Method == http.MethodGet {
			t.Error("user update must not GET-merge a stale user snapshot")
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ID":4,"username":"kept","displayName":"new name","role":"editor","disabled":true}`))
	}))
	defer server.Close()

	cmd := newUserUpdateCmd(client.New(server.URL), &output.Options{Quiet: true})
	cmd.SetArgs([]string{"4", "--display-name", "new name"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute user update: %v", err)
	}
	if gotMethod != http.MethodPost || gotPath != "/v1/user" {
		t.Fatalf("request=%s %s, want POST /v1/user", gotMethod, gotPath)
	}
	if len(gotBody) != 2 || gotBody["id"] != float64(4) || gotBody["displayName"] != "new name" {
		t.Fatalf("body=%v, want only id plus explicitly changed displayName", gotBody)
	}
	for _, unrelated := range []string{"username", "role", "scopeGroupId", "disabled", "password"} {
		if _, present := gotBody[unrelated]; present {
			t.Errorf("unrelated field %q must be absent from partial update body: %v", unrelated, gotBody)
		}
	}
}
