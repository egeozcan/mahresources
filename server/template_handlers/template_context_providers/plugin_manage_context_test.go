package template_context_providers

import (
	"os"
	"strings"
	"testing"

	"mahresources/plugin_system"
)

func TestCommandConfirmationSelectsOnlyADiscoveredCommandPlugin(t *testing.T) {
	plugins := []pluginDisplay{
		{Name: "plain"},
		{Name: "command", Commands: []plugin_system.CommandDisplay{{Name: "run", DisplayArgv: "tool", TimeoutSeconds: 3600}}},
	}
	if got := commandConfirmationPlugin(plugins, "missing"); got != nil {
		t.Fatalf("unknown query selected %+v", got)
	}
	if got := commandConfirmationPlugin(plugins, "plain"); got != nil {
		t.Fatalf("commandless plugin selected %+v", got)
	}
	got := commandConfirmationPlugin(plugins, " command ")
	if got == nil || got.Name != "command" || got.Commands[0].DisplayArgv != "tool" {
		t.Fatalf("valid command plugin was not selected exactly: %+v", got)
	}
}

func TestPluginCommandWarningAndConfirmationStayExplicit(t *testing.T) {
	source, err := os.ReadFile("../../../templates/managePlugins.tpl")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, phrase := range []string{
		"server service account",
		"operator-configured plugin-command path",
		"not sandboxed",
		"unrestricted networking including private and",
		"loopback addresses",
		"anything the OS account can read",
		"sibling plugin exchange folders",
		"separate",
		"db:write",
		"Confirm and enable",
	} {
		if !strings.Contains(text, phrase) {
			t.Errorf("managePlugins.tpl command warning does not contain %q", phrase)
		}
	}
	if got := strings.Count(text, `name="confirm_commands"`); got != 1 {
		t.Fatalf("confirm_commands appears %d times; only the separate confirmation form may carry it", got)
	}
	if strings.Contains(text, "command.DisplayArgv|safe") {
		t.Fatal("command argv bypasses Pongo2 escaping")
	}
}

func TestScheduledDownloadStatusLabelOnlyStopsOwnerlessPending(t *testing.T) {
	cases := []struct {
		name   string
		status string
		owned  bool
		want   string
	}{
		{name: "owned pending", status: "pending", owned: true, want: "pending"},
		{name: "ownerless pending", status: "pending", owned: false, want: "stopped"},
		{name: "ownerless submitted", status: "submitted", owned: false, want: "submitted"},
		{name: "ownerless failed", status: "failed", owned: false, want: "failed"},
		{name: "ownerless cancelled", status: "cancelled", owned: false, want: "cancelled"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := scheduledDownloadStatusLabel(tc.status, tc.owned); got != tc.want {
				t.Fatalf("template status label = %q, want %q", got, tc.want)
			}
		})
	}
}
