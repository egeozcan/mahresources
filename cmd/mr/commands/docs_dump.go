package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"

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

// writeMarkdown renders the command tree as Markdown pages under outputDir.
// Only commands whose top-level group is in lintAllowlist are published — the
// generator refuses to emit half-migrated pages that would ship empty Long /
// Example sections to users. As migration PRs extend the lint allowlist they
// automatically extend the published docs.
//
// One page per published command plus a root index.md. Parent-group commands
// are written to <group>/index.md; leaves to <group>/<leaf>.md. See Also
// links are computed as relative paths between pages.
func writeMarkdown(tree dumpRoot, outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}

	published := make([]dumpCommand, 0, len(tree.Commands))
	for _, c := range tree.Commands {
		if isPublished(c.Path) {
			published = append(published, c)
		}
	}
	publishedTree := tree
	publishedTree.Commands = published

	// Build a path-to-output-file map so See Also can generate correct
	// relative links regardless of which page is being written.
	outputPath := map[string]string{}
	for _, c := range published {
		outputPath[c.Path] = commandOutputPath(c, outputDir)
	}

	if err := writeRootIndex(publishedTree, outputDir, outputPath); err != nil {
		return err
	}

	for _, c := range published {
		target := outputPath[c.Path]
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := writeCommandPage(c, publishedTree, target, outputPath); err != nil {
			return err
		}
	}
	return nil
}

// isPublished reports whether a command at `path` (e.g., "resource get" or
// "resources") should be emitted by the Markdown generator. A command is
// published when its top-level group appears in lintAllowlist — the same
// signal that says its help content is ready for public review.
func isPublished(path string) bool {
	top := path
	if idx := strings.IndexByte(path, ' '); idx >= 0 {
		top = path[:idx]
	}
	return lintAllowlist[top]
}

// commandOutputPath returns the on-disk path for a dumped command.
// Parent/group commands are written to <group>/index.md;
// single-word leaves are written to <word>.md at the outputDir root;
// multi-word leaves are written to <group>/<rest-joined-by-slash>.md.
func commandOutputPath(c dumpCommand, outputDir string) string {
	parts := strings.Split(c.Path, " ")
	if c.IsGroup {
		all := append([]string{outputDir}, parts...)
		all = append(all, "index.md")
		return filepath.Join(all...)
	}
	if len(parts) == 1 {
		return filepath.Join(outputDir, parts[0]+".md")
	}
	top := parts[0]
	rest := strings.Join(parts[1:], "/") + ".md"
	return filepath.Join(outputDir, top, rest)
}

type rootIndexData struct {
	Commands []rootIndexEntry
}

type rootIndexEntry struct {
	Path  string
	Short string
	Link  string
}

const rootIndexTmpl = `---
title: mr CLI
description: Command-line reference for the mr tool
sidebar_label: CLI
---

# mr CLI reference

| Command | Short | |
|---------|-------|--|
{{- range .Commands}}
| ` + "`" + `mr {{.Path}}` + "`" + ` | {{.Short}} | [Details]({{.Link}}) |
{{- end}}
`

func writeRootIndex(tree dumpRoot, outputDir string, outputPath map[string]string) error {
	entries := make([]rootIndexEntry, 0, len(tree.Commands))
	indexPath := filepath.Join(outputDir, "index.md")
	indexDir := filepath.Dir(indexPath)
	for _, c := range tree.Commands {
		entries = append(entries, rootIndexEntry{
			Path:  c.Path,
			Short: c.Short,
			Link:  relPath(indexDir, outputPath[c.Path]),
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })

	f, err := os.Create(indexPath)
	if err != nil {
		return err
	}
	defer f.Close()
	t, err := template.New("root").Parse(rootIndexTmpl)
	if err != nil {
		return err
	}
	return t.Execute(f, rootIndexData{Commands: entries})
}

type commandPageData struct {
	Title             string
	Description       string
	SidebarLabel      string
	Path              string
	Long              string
	UsageLine         string
	PositionalArgs    []positionalArg
	Examples          []commandPageExample
	LocalFlags        []commandPageFlag
	InheritedFlags    []commandPageFlag
	HasLocalFlags     bool
	HasInheritedFlags bool
	OutputShape       string
	ExitCodes         string
	SeeAlsoLinks      []seeAlsoLink
}

type positionalArg struct {
	Name string
	Note string
}

type commandPageExample struct {
	Label   string
	Command string
}

type commandPageFlag struct {
	Name        string
	Type        string
	Default     string
	Description string
	Required    bool
}

type seeAlsoLink struct {
	Name string
	Link string
}

const commandPageTmpl = `---
title: mr {{.Path}}
description: {{.Description}}
sidebar_label: {{.SidebarLabel}}
---

# mr {{.Path}}

{{.Long}}

## Usage

    {{.UsageLine}}
{{- if .PositionalArgs}}

Positional arguments:

{{range .PositionalArgs}}- ` + "`" + `<{{.Name}}>` + "`" + `{{if .Note}} {{.Note}}{{end}}
{{end}}{{- end}}

## Examples

{{range .Examples}}**{{.Label}}**

    {{.Command}}

{{end}}
## Flags

{{if .HasLocalFlags}}| Flag | Type | Default | Description |
|------|------|---------|-------------|
{{range .LocalFlags}}| ` + "`" + `--{{.Name}}` + "`" + ` | {{.Type}} | ` + "`" + `{{.Default}}` + "`" + ` | {{.Description}}{{if .Required}} **(required)**{{end}} |
{{end}}{{else}}This command has no local flags.
{{end}}
{{- if .HasInheritedFlags}}### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
{{range .InheritedFlags}}| ` + "`" + `--{{.Name}}` + "`" + ` | {{.Type}} | ` + "`" + `{{.Default}}` + "`" + ` | {{.Description}} |
{{end}}{{end}}
{{- if .OutputShape}}## Output

{{.OutputShape}}

{{end -}}
## Exit Codes

{{.ExitCodes}}
{{- if .SeeAlsoLinks}}

## See Also

{{range .SeeAlsoLinks}}- [` + "`" + `mr {{.Name}}` + "`" + `]({{.Link}})
{{end}}{{- end}}`

func writeCommandPage(c dumpCommand, tree dumpRoot, targetPath string, outputPath map[string]string) error {
	pageDir := filepath.Dir(targetPath)
	parts := strings.Split(c.Path, " ")
	sidebarLabel := parts[len(parts)-1]

	// Build Usage line. If c.Use starts with the leaf name, strip it and prepend
	// the full command path. Otherwise just use "mr <path>".
	usageLine := "mr " + c.Path
	leafName := parts[len(parts)-1]
	if c.Use != "" {
		if strings.HasPrefix(c.Use, leafName) {
			after := strings.TrimSpace(strings.TrimPrefix(c.Use, leafName))
			if after != "" {
				usageLine = "mr " + c.Path + " " + after
			}
		} else {
			usageLine = "mr " + c.Path + " " + c.Use
		}
	}

	// Positional args section.
	var posArgs []positionalArg
	switch c.Args.Constraint {
	case "none":
		// none — no section
	case "exact":
		for _, n := range c.Args.Names {
			posArgs = append(posArgs, positionalArg{Name: n})
		}
	case "minimum":
		for _, n := range c.Args.Names {
			posArgs = append(posArgs, positionalArg{Name: n, Note: "(variadic; one or more)"})
		}
	case "maximum":
		for _, n := range c.Args.Names {
			posArgs = append(posArgs, positionalArg{Name: n, Note: "(optional)"})
		}
	case "range":
		for i, n := range c.Args.Names {
			note := ""
			if i >= c.Args.Min {
				note = "(optional)"
			}
			posArgs = append(posArgs, positionalArg{Name: n, Note: note})
		}
	}

	// Examples. Doctest blocks are CI-only (they create groups, reference
	// repo-local ./testdata fixtures, and use shell tricks like `$$`/$RANDOM
	// that aren't meaningful to a human reading the docs). Filter them out
	// of the published page so users only see reference examples written
	// for them.
	var examples []commandPageExample
	for _, e := range c.Examples {
		if e.Doctest {
			continue
		}
		examples = append(examples, commandPageExample{
			Label:   e.Label,
			Command: strings.ReplaceAll(e.Command, "\n", "\n    "),
		})
	}

	// Local flags.
	var localFlags []commandPageFlag
	for _, fl := range c.LocalFlags {
		localFlags = append(localFlags, commandPageFlag{
			Name:        fl.Name,
			Type:        fl.Type,
			Default:     fl.Default,
			Description: fl.Description,
			Required:    fl.Required,
		})
	}

	// Inherited flags: look up in tree.PersistentFlags by name.
	pfByName := map[string]dumpFlag{}
	for _, fl := range tree.PersistentFlags {
		pfByName[fl.Name] = fl
	}
	var inheritedFlags []commandPageFlag
	for _, name := range c.InheritedFlags {
		fl, ok := pfByName[name]
		if !ok {
			continue
		}
		inheritedFlags = append(inheritedFlags, commandPageFlag{
			Name:        fl.Name,
			Type:        fl.Type,
			Default:     fl.Default,
			Description: fl.Description,
		})
	}

	// See Also links.
	var seeAlso []seeAlsoLink
	for _, related := range c.RelatedCmds {
		dest, ok := outputPath[related]
		if !ok {
			fmt.Fprintf(os.Stderr, "warning: %s references unknown command %q in relatedCmds\n", c.Path, related)
			continue
		}
		seeAlso = append(seeAlso, seeAlsoLink{
			Name: related,
			Link: relPath(pageDir, dest),
		})
	}

	data := commandPageData{
		Title:             "mr " + c.Path,
		Description:       c.Short,
		SidebarLabel:      sidebarLabel,
		Path:              c.Path,
		Long:              c.Long,
		UsageLine:         usageLine,
		PositionalArgs:    posArgs,
		Examples:          examples,
		LocalFlags:        localFlags,
		InheritedFlags:    inheritedFlags,
		HasLocalFlags:     len(localFlags) > 0,
		HasInheritedFlags: len(inheritedFlags) > 0,
		OutputShape:       c.OutputShape,
		ExitCodes:         c.ExitCodes,
		SeeAlsoLinks:      seeAlso,
	}

	f, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	defer f.Close()
	t, err := template.New("page").Parse(commandPageTmpl)
	if err != nil {
		return err
	}
	return t.Execute(f, data)
}

// relPath wraps filepath.Rel with slash normalisation so Markdown links
// produced on Windows still use forward slashes. It also ensures that
// same-directory links start with "./" (e.g. "./index.md" not "index.md")
// so that Docusaurus resolves them correctly as relative paths.
func relPath(fromDir, toFile string) string {
	rel, err := filepath.Rel(fromDir, toFile)
	if err != nil {
		return toFile
	}
	s := filepath.ToSlash(rel)
	if !strings.HasPrefix(s, ".") {
		s = "./" + s
	}
	return s
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
