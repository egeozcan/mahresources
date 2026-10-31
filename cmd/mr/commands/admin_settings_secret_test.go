package commands

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"mahresources/cmd/mr/client"
	"mahresources/cmd/mr/output"

	"github.com/spf13/cobra"
)

func TestAdminSettingsGetPreservesSecretStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"key":"ai_openai_api_key","secret":true,"configured":false,"bootConfigured":true,"current":null,"bootDefault":null}]`))
	}))
	defer server.Close()
	cmd := NewAdminSettingsGetCmd(client.New(server.URL), &output.Options{JSON: true})
	cmd.SetArgs([]string{"ai_openai_api_key"})
	var err error
	stdout := captureStdout(t, func() { err = cmd.Execute() })
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("decode CLI output: %v", err)
	}
	for key, want := range map[string]bool{"secret": true, "configured": false, "bootConfigured": true} {
		if value, present := got[key]; !present || value != want {
			t.Errorf("%s=%v (present=%v), want %v", key, value, present, want)
		}
	}
}

func TestAdminSettingsCommandsDescribeSecretStatus(t *testing.T) {
	for _, command := range []string{"list", "get", "set", "reset"} {
		t.Run(command, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				view := map[string]any{
					"key": "ai_openai_api_key", "label": "OpenAI API key", "group": "ai", "type": "string", "secret": true,
					"configured": true, "bootConfigured": false, "current": nil, "bootDefault": nil,
				}
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodGet {
					_ = json.NewEncoder(w).Encode([]any{view})
				} else {
					_ = json.NewEncoder(w).Encode(view)
				}
			}))
			defer server.Close()
			c := client.New(server.URL)
			opts := &output.Options{}
			var cmd *cobra.Command
			switch command {
			case "list":
				cmd = NewAdminSettingsListCmd(c, opts)
			case "get":
				cmd = NewAdminSettingsGetCmd(c, opts)
				cmd.SetArgs([]string{"ai_openai_api_key"})
			case "set":
				cmd = NewAdminSettingsSetCmd(c, opts)
				cmd.SetArgs([]string{"ai_openai_api_key", "submitted-secret"})
			case "reset":
				cmd = NewAdminSettingsResetCmd(c, opts)
				cmd.SetArgs([]string{"ai_openai_api_key"})
			}
			var err error
			stdout := captureStdout(t, func() { err = cmd.Execute() })
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(stdout, "configured") || !strings.Contains(stdout, "not configured") {
				t.Fatalf("secret status missing from CLI output: %s", stdout)
			}
			if strings.Contains(stdout, "<nil>") || strings.Contains(stdout, "submitted-secret") {
				t.Fatalf("secret output should describe status only: %s", stdout)
			}
		})
	}
}
