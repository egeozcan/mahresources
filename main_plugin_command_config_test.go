package main

import (
	"os"
	"strings"
	"testing"
)

func TestPluginCommandLifecycleMainOrdering(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	settings := strings.Index(text, "context.SetSettings(settings)")
	commands := strings.Index(text, "context.StartPluginCommands(")
	plugins := strings.Index(text, "context.ActivateEnabledPlugins()")
	if settings < 0 || commands < 0 || plugins < 0 || !(settings < commands && commands < plugins) {
		t.Fatalf("startup order must be settings -> command recovery -> plugin activation: %d %d %d", settings, commands, plugins)
	}
	downloadStop := strings.Index(text, "defer context.DownloadManager().Shutdown()")
	commandStop := strings.Index(text, "defer context.StopPluginCommands()")
	if downloadStop < 0 || commandStop < 0 || downloadStop > commandStop {
		t.Fatalf("defer registration must be download then commands for LIFO shutdown: %d %d", downloadStop, commandStop)
	}
}

func TestPluginCommandConfigPathPrecedence(t *testing.T) {
	cases := []struct {
		name         string
		flagValue    string
		flagSet      bool
		envValue     string
		envSet       bool
		inherited    string
		want         string
		wantExplicit bool
	}{
		{name: "inherited", inherited: "/inherited", want: "/inherited"},
		{name: "environment", envValue: "/environment", envSet: true, inherited: "/inherited", want: "/environment", wantExplicit: true},
		{name: "flag", flagValue: "/flag", flagSet: true, envValue: "/environment", envSet: true, inherited: "/inherited", want: "/flag", wantExplicit: true},
		{name: "explicit empty flag", flagValue: "", flagSet: true, envValue: "/environment", envSet: true, inherited: "/inherited", want: "", wantExplicit: true},
		{name: "explicit empty environment", envValue: "", envSet: true, inherited: "/inherited", want: "", wantExplicit: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, explicit := pluginCommandPathValue(tc.flagValue, tc.flagSet, tc.envValue, tc.envSet, tc.inherited)
			if got != tc.want || explicit != tc.wantExplicit {
				t.Fatalf("got (%q,%v), want (%q,%v)", got, explicit, tc.want, tc.wantExplicit)
			}
		})
	}
}
