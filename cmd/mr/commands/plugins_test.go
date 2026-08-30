package commands

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

// runPluginCmd executes `mr plugin <args...>` against server and returns what
// the CLI printed plus the error it would exit on. Output is captured so a
// cobra usage dump does not land in the test log.
func runPluginCmd(t *testing.T, serverURL string, args ...string) (string, error) {
	t.Helper()
	cmd := NewPluginCmd(client.New(serverURL), &output.Options{Quiet: true})
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	return out.String(), cmd.Execute()
}

// requirePluginSubcommand fails unless `mr plugin` carries the named child.
// Without it a test of the command's argument handling would pass on cobra's
// "unknown command" error and prove nothing.
func requirePluginSubcommand(t *testing.T, name string) *cobra.Command {
	t.Helper()
	parent := NewPluginCmd(client.New("http://localhost:0"), &output.Options{})
	for _, sub := range parent.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	t.Fatalf("mr plugin has no %q subcommand", name)
	return nil
}

// serverReadsAsAllowed mirrors api_handlers.isTruthyFormValue, the rule
// /v1/plugin/scopedAccess applies to whatever this command sends. Asserting the
// value's meaning rather than its spelling leaves the command free to send
// "true", "1" or "on", and still catches a command that sends a grant when the
// operator asked for a revocation.
func serverReadsAsAllowed(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

// The per-plugin scoped-access setting is the only plugin control with no
// command-line surface. The decision has to reach the endpoint that stores it,
// in the shape that endpoint reads.
func TestPluginScopedAccessSendsTheOperatorDecision(t *testing.T) {
	for _, tc := range []struct {
		flag        string
		wantAllowed bool
	}{
		{"--allowed=true", true},
		{"--allowed=false", false},
	} {
		t.Run(tc.flag, func(t *testing.T) {
			var gotMethod, gotPath string
			var gotForm url.Values
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotMethod, gotPath = r.Method, r.URL.Path
				if err := r.ParseForm(); err != nil {
					t.Errorf("parse form: %v", err)
				}
				// r.Form merges query and body, which is what the handler's
				// FormValue reads, so either carrier passes.
				gotForm = r.Form
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"ok":true,"name":"my-plugin","allow_scoped_principals":true}`))
			}))
			defer server.Close()

			out, err := runPluginCmd(t, server.URL, "scoped-access", "my-plugin", tc.flag)
			if err != nil {
				t.Fatalf("execute: %v (output: %s)", err, out)
			}
			if gotMethod != http.MethodPost || gotPath != "/v1/plugin/scopedAccess" {
				t.Fatalf("request = %s %s, want POST /v1/plugin/scopedAccess", gotMethod, gotPath)
			}
			if gotForm.Get("name") != "my-plugin" {
				t.Errorf("name = %q, want my-plugin (sent: %v)", gotForm.Get("name"), gotForm)
			}
			if got := serverReadsAsAllowed(gotForm.Get("allowed")); got != tc.wantAllowed {
				t.Errorf("allowed = %q, which the server reads as %v; want %v",
					gotForm.Get("allowed"), got, tc.wantAllowed)
			}
		})
	}
}

// A bool flag that defaults to false would make `mr plugin scoped-access
// my-plugin` a silent revocation, so the decision is required rather than
// defaulted — the shape `plugin settings --data` already uses in this file. The
// plugin name is required for the same reason enable and disable require one.
func TestPluginScopedAccessRequiresANameAndADecision(t *testing.T) {
	requirePluginSubcommand(t, "scoped-access")

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	for _, args := range [][]string{
		{"scoped-access"},
		{"scoped-access", "my-plugin"},
	} {
		out, err := runPluginCmd(t, server.URL, args...)
		if err == nil {
			t.Errorf("args %v: expected an error, got none (output: %s)", args, out)
		}
	}
	if called {
		t.Error("an incomplete command must not reach the server")
	}
}

// Error handling matches enable and disable: the server's refusal is the
// command's exit status, not a success message printed over a failed write.
func TestPluginScopedAccessReportsServerErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"plugin \"ghost\" not found"}`))
	}))
	defer server.Close()

	out, err := runPluginCmd(t, server.URL, "scoped-access", "ghost", "--allowed=true")
	if err == nil {
		t.Fatalf("expected an error for a 400 response, got none (output: %s)", out)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error %q should carry the server's message", err)
	}
}

// CI runs `mr docs lint` and `mr docs check-examples` over the embedded help
// pages. Lint reads Long and Example off the command, which an author could
// inline — passing lint while the documented page CI runs the examples from
// does not exist.
func TestPluginScopedAccessShipsAHelpPage(t *testing.T) {
	if _, err := pluginsHelpFS.ReadFile("plugins_help/plugin_scoped_access.md"); err != nil {
		t.Fatalf("plugins_help/plugin_scoped_access.md must exist: %v", err)
	}
}

func TestPluginScheduledDownloadsOnlyOwnerlessPendingIsStopped(t *testing.T) {
	cases := []struct {
		name   string
		status string
		owned  bool
		want   string
	}{
		{name: "owned pending", status: "pending", owned: true, want: "pending"},
		{name: "ownerless pending", status: "pending", owned: false, want: "stopped (no owner)"},
		{name: "ownerless submitted", status: "submitted", owned: false, want: "submitted"},
		{name: "ownerless failed", status: "failed", owned: false, want: "failed"},
		{name: "ownerless cancelled", status: "cancelled", owned: false, want: "cancelled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pluginScheduledDownloadDisplayState(tc.status, tc.owned); got != tc.want {
				t.Fatalf("display state = %q, want %q", got, tc.want)
			}
		})
	}
}
