//go:build windows

package plugin_commands

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

type unsupportedWindowsExecutor struct {
	deps RunnerDependencies
}

func NewExecutor(deps RunnerDependencies) Executor {
	return &unsupportedWindowsExecutor{deps: deps}
}

func (e *unsupportedWindowsExecutor) Prepare(run QueuedRun) error {
	return fmt.Errorf("plugin command execution is unsupported on Windows")
}

func (e *unsupportedWindowsExecutor) Cleanup(run QueuedRun) {
	if e.deps.Settings == nil {
		return
	}
	root := filepath.Clean(e.deps.Settings.StagingRoot())
	if ensureWindowsRunPath(root, run) {
		_ = os.RemoveAll(run.ExchangeDir)
	}
}

func (e *unsupportedWindowsExecutor) Execute(context.Context, QueuedRun) Outcome {
	return Outcome{Status: RunStatusFailed, Error: "plugin command execution is unsupported on Windows"}
}

func ensureWindowsRunPath(root string, run QueuedRun) bool {
	return run.RunID != "" && run.Request.PluginName != "" && filepath.Clean(run.ExchangeDir) == filepath.Join(root, "plugin_exchange", run.Request.PluginName, run.RunID)
}
