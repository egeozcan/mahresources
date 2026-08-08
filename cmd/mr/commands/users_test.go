package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"
)

func TestUserUpdateDocumentsScopeClearing(t *testing.T) {
	cmd := newUserUpdateCmd(client.New("http://example.invalid"), &output.Options{})
	var help strings.Builder
	cmd.SetOut(&help)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("render help: %v", err)
	}
	if !strings.Contains(help.String(), "use 0 to clear") {
		t.Fatalf("scope clear contract missing from help:\n%s", help.String())
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
