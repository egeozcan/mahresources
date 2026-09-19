package main

import (
	"flag"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginCommandLifecycleMainOrdering(t *testing.T) {
	source, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "log.Fatal") {
		t.Fatal("main must return through deferred plugin command and staging cleanup, not call log.Fatal/os.Exit")
	}
	settings := strings.Index(text, "context.SetSettings(settings)")
	commands := strings.Index(text, "context.StartPluginCommandsIfEnabled(")
	plugins := strings.Index(text, "context.ActivateEnabledPlugins()")
	if settings < 0 || commands < 0 || plugins < 0 || !(settings < commands && commands < plugins) {
		t.Fatalf("startup order must be settings -> command recovery -> plugin activation: %d %d %d", settings, commands, plugins)
	}
	tempCleanup := strings.Index(text, "defer os.RemoveAll(pluginCommandConfig.StagingPath)")
	downloadStop := strings.Index(text, "defer context.DownloadManager().Shutdown()")
	commandStop := strings.Index(text, "context.StopPluginCommands()")
	workers := strings.Index(text, "hw.Start()")
	if tempCleanup < 0 || downloadStop < 0 || commandStop < 0 || workers < 0 ||
		!(tempCleanup < downloadStop && downloadStop < commandStop && commandStop < workers) {
		t.Fatalf("startup-safe cleanup order must be staging -> download -> commands -> workers: %d %d %d %d", tempCleanup, downloadStop, commandStop, workers)
	}
}

func TestPluginCommandTemporaryStagingIsRemovedWhenContextCreationFails(t *testing.T) {
	const (
		helperEnv     = "MAHRESOURCES_CONTEXT_FAILURE_HELPER"
		commandDirEnv = "MAHRESOURCES_CONTEXT_FAILURE_COMMAND_DIR"
	)
	if os.Getenv(helperEnv) == "1" {
		flag.CommandLine = flag.NewFlagSet("mahresources", flag.ExitOnError)
		os.Args = []string{
			"mahresources",
			"-ephemeral",
			"-plugin-command-path", os.Getenv(commandDirEnv),
			"-seed-db", filepath.Join(os.TempDir(), "missing-seed.db"),
		}
		main()
		t.Fatal("main returned success for a missing seed database")
	}

	tmp := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestPluginCommandTemporaryStagingIsRemovedWhenContextCreationFails$")
	cmd.Env = append(environmentWithout("TMPDIR", helperEnv, commandDirEnv),
		"TMPDIR="+tmp,
		helperEnv+"=1",
		commandDirEnv+"="+t.TempDir(),
	)
	output, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("helper process succeeded, output:\n%s", output)
	}
	if !strings.Contains(string(output), "-seed-db file does not exist") || !strings.Contains(err.Error(), "exit status") {
		t.Fatalf("helper process error = %v, output:\n%s", err, output)
	}
	if _, ok := err.(*exec.ExitError); !ok {
		t.Fatalf("helper process error type = %T, want *exec.ExitError", err)
	}

	matches, globErr := filepath.Glob(filepath.Join(tmp, "mahresources-plugin-commands-*"))
	if globErr != nil {
		t.Fatal(globErr)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary plugin command staging roots leaked after context failure: %v", matches)
	}
}

func TestContextConstructionPathContainsNoFatalExit(t *testing.T) {
	source, err := os.ReadFile("application_context/context.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	if strings.Contains(text, "log.Fatal") {
		t.Fatal("context construction must return errors instead of bypassing main's deferred cleanup")
	}

	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	mainText := string(mainSource)
	if !strings.Contains(mainText, "application_context.OpenContextWithConfig(cfg)") {
		t.Fatal("main must use the error-returning context constructor")
	}
}

func environmentWithout(keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[key] = struct{}{}
	}
	out := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, skip := blocked[key]; !skip {
			out = append(out, entry)
		}
	}
	return out
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
