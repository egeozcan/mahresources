package commands

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

func checkExamples(root *cobra.Command, serverURL, environment string) error {
	serverURL = resolveServerURL(serverURL)
	cwd, err := doctestCwd()
	if err != nil {
		return err
	}
	mrPath, err := resolveMrBinary()
	if err != nil {
		return err
	}
	env := os.Environ()
	env = append(env,
		"MAHRESOURCES_URL="+serverURL,
		"PATH="+prependPath(os.Getenv("PATH"), filepath.Dir(mrPath)),
	)

	var failures []string
	for _, c := range walkSkippingBuiltins(root) {
		for _, ex := range parseExamples(c.Example) {
			if !ex.Doctest {
				continue
			}
			if ex.SkipOn != "" && ex.SkipOn == environment {
				fmt.Printf("SKIP  %s: %s (skip-on=%s)\n", c.CommandPath(), ex.Label, ex.SkipOn)
				continue
			}
			if err := runDoctest(ex, cwd, env); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %s: %v", c.CommandPath(), ex.Label, err))
				fmt.Printf("FAIL  %s: %s\n", c.CommandPath(), ex.Label)
			} else {
				fmt.Printf("PASS  %s: %s\n", c.CommandPath(), ex.Label)
			}
		}
	}
	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintln(os.Stderr, f)
		}
		return fmt.Errorf("%d doctest failures", len(failures))
	}
	return nil
}

func runDoctest(ex dumpExample, cwd string, env []string) error {
	timeout := 30 * time.Second
	if ex.TimeoutSec > 0 {
		timeout = time.Duration(ex.TimeoutSec) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-eo", "pipefail", "-c", ex.Command)
	cmd.Dir = cwd
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if ex.Stdin != "" {
		data, err := os.ReadFile(filepath.Join(cwd, "testdata", ex.Stdin))
		if err != nil {
			return fmt.Errorf("reading stdin fixture: %w", err)
		}
		cmd.Stdin = bytes.NewReader(data)
	}

	err := cmd.Run()

	// Check for timeout before inspecting exit codes — a killed process also
	// returns an ExitError (with code -1), so we must check the context first.
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("timed out after %s", timeout)
	}

	exitCode := 0
	if ee, ok := err.(*exec.ExitError); ok {
		exitCode = ee.ExitCode()
	} else if err != nil {
		return err
	}

	expected := ex.ExpectedExit
	if exitCode == expected {
		return nil
	}
	if ex.Tolerate != "" {
		re, err := regexp.Compile(ex.Tolerate)
		if err != nil {
			return fmt.Errorf("invalid tolerate regex %q: %w", ex.Tolerate, err)
		}
		if re.Match(stderr.Bytes()) {
			return nil
		}
	}
	return fmt.Errorf("exit %d (want %d); stderr: %s", exitCode, expected, truncate(stderr.String(), 400))
}

func resolveServerURL(flag string) string {
	if flag != "" {
		return flag
	}
	if env := os.Getenv("MAHRESOURCES_URL"); env != "" {
		return env
	}
	return "http://localhost:8181"
}

// doctestCwd walks up from the current working directory looking for
// cmd/mr/testdata. Supports running from repo root, e2e/, or cmd/mr itself.
func doctestCwd() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for _, rel := range []string{".", "..", "../..", "../../.."} {
		root := filepath.Clean(filepath.Join(wd, rel))
		candidate := filepath.Join(root, "cmd", "mr")
		if _, err := os.Stat(filepath.Join(candidate, "testdata")); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not locate cmd/mr/testdata from %s", wd)
}

func resolveMrBinary() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err != nil {
		return exe, nil
	}
	return resolved, nil
}

func prependPath(existing, dir string) string {
	sep := string(os.PathListSeparator)
	if existing == "" {
		return dir
	}
	parts := strings.Split(existing, sep)
	if len(parts) > 0 && parts[0] == dir {
		return existing
	}
	return dir + sep + existing
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
