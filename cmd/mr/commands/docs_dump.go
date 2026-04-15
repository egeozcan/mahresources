package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type dumpRoot struct {
	Name            string        `json:"name"`
	Short           string        `json:"short"`
	Long            string        `json:"long"`
	PersistentFlags []dumpFlag    `json:"persistentFlags"`
	Commands        []dumpCommand `json:"commands"`
}

type dumpCommand struct {
	Path           string        `json:"path"`
	Short          string        `json:"short"`
	Long           string        `json:"long,omitempty"`
	Use            string        `json:"use"`
	IsGroup        bool          `json:"isGroup"`
	Args           dumpArgs      `json:"args"`
	Examples       []dumpExample `json:"examples"`
	LocalFlags     []dumpFlag    `json:"localFlags"`
	InheritedFlags []string      `json:"inheritedFlags"`
	RequiredFlags  []string      `json:"requiredFlags"`
	OutputShape    string        `json:"outputShape,omitempty"`
	ExitCodes      string        `json:"exitCodes,omitempty"`
	RelatedCmds    []string      `json:"relatedCmds,omitempty"`
}

type dumpArgs struct {
	Constraint string   `json:"constraint"`
	N          int      `json:"n,omitempty"`
	Min        int      `json:"min,omitempty"`
	Max        int      `json:"max,omitempty"`
	Names      []string `json:"names"`
}

type dumpFlag struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
	EnvVar      string `json:"envVar,omitempty"`
}

type dumpExample struct {
	Label        string `json:"label"`
	Command      string `json:"command"`
	Doctest      bool   `json:"doctest"`
	ExpectedExit int    `json:"expectedExit,omitempty"`
	SkipOn       string `json:"skipOn,omitempty"`
	Tolerate     string `json:"tolerate,omitempty"`
	TimeoutSec   int    `json:"timeoutSec,omitempty"`
	Stdin        string `json:"stdin,omitempty"`
}

// dumpCommandTree walks the root Cobra tree and emits JSON or Markdown.
func dumpCommandTree(root *cobra.Command, format, output string) error {
	tree := buildDump(root)
	switch format {
	case "json":
		return writeJSON(tree, output)
	case "markdown":
		if output == "" {
			return fmt.Errorf("--output is required for markdown format")
		}
		return writeMarkdown(tree, output) // implemented in Task 6
	default:
		return fmt.Errorf("invalid --format: %q (want json or markdown)", format)
	}
}

// writeMarkdown is a placeholder until Task 6 implements it.
func writeMarkdown(tree dumpRoot, output string) error {
	return fmt.Errorf("markdown format not implemented yet (Task 6)")
}

func buildDump(root *cobra.Command) dumpRoot {
	r := dumpRoot{
		Name:            root.Name(),
		Short:           root.Short,
		Long:            root.Long,
		PersistentFlags: collectFlags(root.PersistentFlags(), nil),
	}
	persistentNames := map[string]bool{}
	for _, f := range r.PersistentFlags {
		persistentNames[f.Name] = true
	}
	for _, c := range walkSkippingBuiltins(root) {
		if c == root {
			continue
		}
		r.Commands = append(r.Commands, buildDumpCommand(c, persistentNames))
	}
	return r
}

func walkSkippingBuiltins(root *cobra.Command) []*cobra.Command {
	var out []*cobra.Command
	var rec func(*cobra.Command)
	rec = func(c *cobra.Command) {
		name := c.Name()
		if name == "help" || name == "completion" {
			return
		}
		out = append(out, c)
		for _, ch := range c.Commands() {
			rec(ch)
		}
	}
	rec(root)
	return out
}

var argNameRE = regexp.MustCompile(`[<\[]([a-z0-9-]+)[>\]]`)

// variadicRE matches variadic argument patterns like [<id>...] or <id>...
var variadicRE = regexp.MustCompile(`\[?<[a-z0-9-]+>\.\.\.]\s*`)

func parseArgsFromUse(use string, annotation string) dumpArgs {
	if annotation != "" {
		if a, ok := parseArgsAnnotation(annotation); ok {
			a.Names = collectNames(use)
			return a
		}
	}

	names := collectNames(use)
	hasVariadic := strings.Contains(use, "...")

	// Strip variadic patterns before counting required/optional positional args,
	// so that [<id>...] doesn't contribute to the optional count.
	stripped := variadicRE.ReplaceAllString(use, "")

	required := 0
	optional := 0
	for _, m := range argNameRE.FindAllStringSubmatchIndex(stripped, -1) {
		openCh := stripped[m[0]]
		if openCh == '<' {
			required++
		} else {
			optional++
		}
	}

	switch {
	case hasVariadic:
		return dumpArgs{Constraint: "minimum", Min: required, Names: names}
	case required == 0 && optional == 0:
		return dumpArgs{Constraint: "none", Names: nil}
	case optional == 0:
		return dumpArgs{Constraint: "exact", N: required, Names: names}
	case required == 0:
		return dumpArgs{Constraint: "maximum", Max: optional, Names: names}
	default:
		return dumpArgs{Constraint: "range", Min: required, Max: required + optional, Names: names}
	}
}

func collectNames(use string) []string {
	matches := argNameRE.FindAllStringSubmatch(use, -1)
	var names []string
	seen := map[string]bool{}
	for _, m := range matches {
		if seen[m[1]] {
			continue
		}
		seen[m[1]] = true
		names = append(names, m[1])
	}
	return names
}

var argsAnnotationRE = regexp.MustCompile(`^(none|exact|minimum|maximum|range)(?::(\d+)(?:-(\d+))?)?$`)

func parseArgsAnnotation(s string) (dumpArgs, bool) {
	m := argsAnnotationRE.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return dumpArgs{}, false
	}
	switch m[1] {
	case "none":
		return dumpArgs{Constraint: "none"}, true
	case "exact":
		n, _ := strconv.Atoi(m[2])
		return dumpArgs{Constraint: "exact", N: n}, true
	case "minimum":
		n, _ := strconv.Atoi(m[2])
		return dumpArgs{Constraint: "minimum", Min: n}, true
	case "maximum":
		n, _ := strconv.Atoi(m[2])
		return dumpArgs{Constraint: "maximum", Max: n}, true
	case "range":
		lo, _ := strconv.Atoi(m[2])
		hi, _ := strconv.Atoi(m[3])
		return dumpArgs{Constraint: "range", Min: lo, Max: hi}, true
	}
	return dumpArgs{}, false
}

func buildDumpCommand(c *cobra.Command, persistent map[string]bool) dumpCommand {
	local := collectFlags(c.LocalFlags(), persistent)
	var required []string
	for _, f := range local {
		if f.Required {
			required = append(required, f.Name)
		}
	}
	var inherited []string
	c.InheritedFlags().VisitAll(func(f *pflag.Flag) {
		if persistent[f.Name] {
			inherited = append(inherited, f.Name)
		}
	})
	related := parseRelatedCmds(c.Annotations["relatedCmds"])
	return dumpCommand{
		Path:           c.CommandPath()[len(c.Root().Name())+1:],
		Short:          c.Short,
		Long:           c.Long,
		Use:            c.Use,
		IsGroup:        c.HasSubCommands(),
		Args:           parseArgsFromUse(c.Use, c.Annotations["argsConstraint"]),
		Examples:       parseExamples(c.Example),
		LocalFlags:     local,
		InheritedFlags: inherited,
		RequiredFlags:  required,
		OutputShape:    c.Annotations["outputShape"],
		ExitCodes:      c.Annotations["exitCodes"],
		RelatedCmds:    related,
	}
}

func parseRelatedCmds(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func collectFlags(set *pflag.FlagSet, skip map[string]bool) []dumpFlag {
	var out []dumpFlag
	set.VisitAll(func(f *pflag.Flag) {
		if skip != nil && skip[f.Name] {
			return
		}
		required := false
		for _, a := range f.Annotations["cobra_annotation_bash_completion_one_required_flag"] {
			if a == "true" {
				required = true
			}
		}
		out = append(out, dumpFlag{
			Name:        f.Name,
			Type:        f.Value.Type(),
			Default:     f.DefValue,
			Description: f.Usage,
			Required:    required,
			EnvVar:      envVarFromUsage(f.Usage),
		})
	})
	return out
}

var envVarRE = regexp.MustCompile(`env: ([A-Z_][A-Z0-9_]*)`)

func envVarFromUsage(usage string) string {
	if m := envVarRE.FindStringSubmatch(usage); m != nil {
		return m[1]
	}
	return ""
}

var (
	labelLineRE = regexp.MustCompile(`^\s*#\s+(.+)$`)
	doctestRE   = regexp.MustCompile(`^mr-doctest:\s*(.+)$`)
	metaKVRE    = regexp.MustCompile(`^\s*([a-z-]+)(?:=(.*))?\s*$`)
)

func parseExamples(s string) []dumpExample {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	var out []dumpExample
	var cur *dumpExample
	var body strings.Builder
	flush := func() {
		if cur == nil {
			return
		}
		cur.Command = strings.TrimRight(body.String(), "\n")
		out = append(out, *cur)
		cur = nil
		body.Reset()
	}
	for _, line := range lines {
		if m := labelLineRE.FindStringSubmatch(line); m != nil {
			flush()
			cur = &dumpExample{}
			label := m[1]
			if dm := doctestRE.FindStringSubmatch(label); dm != nil {
				cur.Doctest = true
				label = dm[1]
			}
			cur.Label, *cur = applyExampleMetadata(label, *cur)
			continue
		}
		if cur == nil {
			continue
		}
		body.WriteString(strings.TrimPrefix(line, "  "))
		body.WriteByte('\n')
	}
	flush()
	return out
}

func applyExampleMetadata(raw string, ex dumpExample) (string, dumpExample) {
	parts := strings.Split(raw, ",")
	label := strings.TrimSpace(parts[0])
	ex.Label = label
	for _, p := range parts[1:] {
		kv := metaKVRE.FindStringSubmatch(p)
		if kv == nil {
			continue
		}
		switch kv[1] {
		case "expect-exit":
			n, _ := strconv.Atoi(strings.TrimSpace(kv[2]))
			ex.ExpectedExit = n
		case "skip-on":
			ex.SkipOn = strings.TrimSpace(kv[2])
		case "tolerate":
			ex.Tolerate = strings.Trim(strings.TrimSpace(kv[2]), "/")
		case "timeout":
			v := strings.TrimSuffix(strings.TrimSpace(kv[2]), "s")
			n, _ := strconv.Atoi(v)
			ex.TimeoutSec = n
		case "stdin":
			ex.Stdin = strings.TrimSpace(kv[2])
		}
	}
	return label, ex
}

func writeJSON(tree dumpRoot, output string) error {
	var w io.Writer = os.Stdout
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(tree)
}
