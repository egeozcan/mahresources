package commands

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// lintAllowlist names the top-level command groups whose subtrees are
// subject to strict lint rules. Each migration PR adds its group.
var lintAllowlist = map[string]bool{
	"docs":                true,
	"resource":            true,
	"resources":           true,
	"group":               true,
	"groups":              true,
	"note":                true,
	"notes":               true,
	"query":               true,
	"queries":             true,
	"tag":                 true,
	"tags":                true,
	"mrql":                true,
	"note-type":           true,
	"note-types":          true,
	"category":            true,
	"categories":          true,
	"resource-category":   true,
	"resource-categories": true,
	"relation":            true,
	"relation-type":       true,
	"relation-types":      true,
	"note-block":          true,
	"note-blocks":         true,
	"series":              true,
	"job":                 true,
	"jobs":                true,
}

// SetLintAllowlistForTest temporarily replaces the production allowlist
// with the given map. It returns a restore function that the caller must
// defer. Intended for use from tests only.
func SetLintAllowlistForTest(m map[string]bool) func() {
	prev := lintAllowlist
	lintAllowlist = m
	return func() { lintAllowlist = prev }
}

// RunLintForTest is a public wrapper around lintCommandTreeTo so external
// test packages (e.g., commands_test) can invoke lint without depending
// on the unexported entry point.
func RunLintForTest(root *cobra.Command, stdout, stderr io.Writer) error {
	return lintCommandTreeTo(root, stdout, stderr)
}

func lintCommandTree(root *cobra.Command) error {
	return lintCommandTreeTo(root, os.Stdout, os.Stderr)
}

func lintCommandTreeTo(root *cobra.Command, stdout, stderr io.Writer) error {
	var failures []string
	var warnings []string
	for _, c := range walkSkippingBuiltins(root) {
		if c == root {
			continue
		}
		// top = first path segment after root name, e.g., "resource" in "mr resource get".
		segments := strings.SplitN(c.CommandPath(), " ", 3)
		if len(segments) < 2 {
			continue
		}
		top := segments[1]
		if !lintAllowlist[top] {
			continue
		}
		f, w := lintCommand(c)
		failures = append(failures, f...)
		warnings = append(warnings, w...)
	}
	sort.Strings(failures)
	sort.Strings(warnings)
	for _, w := range warnings {
		fmt.Fprintln(stderr, "warning:", w)
	}
	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintln(stderr, "error:", f)
		}
		return fmt.Errorf("%d lint failures", len(failures))
	}
	fmt.Fprintln(stdout, "OK:", len(warnings), "warnings")
	return nil
}

func lintCommand(c *cobra.Command) (failures, warnings []string) {
	path := c.CommandPath()
	if len(c.Short) == 0 {
		failures = append(failures, fmt.Sprintf("%s: missing Short", path))
	} else if len(c.Short) > 60 {
		failures = append(failures, fmt.Sprintf("%s: Short > 60 chars (%d)", path, len(c.Short)))
	}
	if strings.TrimSpace(c.Long) == "" {
		failures = append(failures, fmt.Sprintf("%s: missing Long", path))
	} else if sentenceCount(c.Long) < 2 {
		failures = append(failures, fmt.Sprintf("%s: Long has fewer than 2 sentences", path))
	}
	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if strings.TrimSpace(f.Usage) == "" {
			failures = append(failures, fmt.Sprintf("%s: flag --%s missing description", path, f.Name))
		}
	})
	// exitCodes annotation is required on every command (spec: "all commands").
	if c.Annotations["exitCodes"] == "" {
		failures = append(failures, fmt.Sprintf("%s: missing exitCodes annotation", path))
	}

	if !c.HasSubCommands() {
		exs := parseExamples(c.Example)
		if len(exs) < 2 {
			failures = append(failures, fmt.Sprintf("%s: fewer than 2 examples (%d)", path, len(exs)))
		}
		hasDoctest := false
		for _, ex := range exs {
			if ex.Doctest {
				hasDoctest = true
				break
			}
		}
		if !hasDoctest {
			warnings = append(warnings, fmt.Sprintf("%s: no # mr-doctest: examples", path))
		}
	}
	return failures, warnings
}

func sentenceCount(s string) int {
	// Conservative: count period-space and trailing period.
	n := strings.Count(s, ". ")
	trimmed := strings.TrimSpace(s)
	if strings.HasSuffix(trimmed, ".") {
		n++
	}
	return n
}
