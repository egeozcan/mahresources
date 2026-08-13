// Command skills-gen renders the installable agent skill's generated reference
// from the repository's own sources of truth.
//
// The MRQL language reference is not authored twice. docs-site/docs/features/
// mrql-reference.md is the canonical cheatsheet, and mrql/reference_docs_test.go
// checks it against mrql/fields.go and mrql/limits.go. This command transforms
// that page into skills/mahresources-mrql/references/language.md and appends a
// CLI section built from the live Cobra tree (via `mr docs dump --format json`),
// so a flag added to `mr mrql` shows up in the skill without anyone retyping it.
//
// CI regenerates and diffs, exactly like the docs-site CLI pages.
//
//	go run ./cmd/skills-gen
//	go run ./cmd/skills-gen -mr ./mr -o skills/mahresources-mrql/references/language.md
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	defaultReference = "docs-site/docs/features/mrql-reference.md"
	defaultOutput    = "skills/mahresources-mrql/references/language.md"
	defaultMrBinary  = "./mr"
	// docsBaseURL matches the -docs-site-base-url default. Relative Docusaurus
	// links must become absolute: the skill is installed outside this repo, so
	// "./mrql.md" would resolve against the reader's own filesystem.
	docsBaseURL = "https://egeozcan.github.io/mahresources"
)

// cliCommandPaths are the command subtrees whose flags the skill documents.
var cliCommandPaths = []string{
	"mrql",
	"mrql save",
	"mrql list",
	"mrql run",
	"mrql explain",
	"mrql export",
	"mrql delete",
	"search",
}

type dumpRoot struct {
	PersistentFlags []dumpFlag    `json:"persistentFlags"`
	Commands        []dumpCommand `json:"commands"`
}

type dumpCommand struct {
	Path          string     `json:"path"`
	Short         string     `json:"short"`
	Use           string     `json:"use"`
	LocalFlags    []dumpFlag `json:"localFlags"`
	RequiredFlags []string   `json:"requiredFlags"`
	OutputShape   string     `json:"outputShape,omitempty"`
	ExitCodes     string     `json:"exitCodes,omitempty"`
}

type dumpFlag struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
	EnvVar      string `json:"envVar,omitempty"`
}

func main() {
	var (
		reference = flag.String("reference", defaultReference, "path to the canonical MRQL reference page")
		output    = flag.String("o", defaultOutput, "path to the generated skill reference")
		mrBinary  = flag.String("mr", defaultMrBinary, "path to the built mr binary used for `docs dump`")
		cliJSON   = flag.String("cli-json", "", "read the CLI tree from this file instead of invoking -mr")
	)
	flag.Parse()

	if err := run(*reference, *output, *mrBinary, *cliJSON); err != nil {
		fmt.Fprintln(os.Stderr, "skills-gen:", err)
		os.Exit(1)
	}
}

func run(reference, output, mrBinary, cliJSON string) error {
	page, err := os.ReadFile(reference)
	if err != nil {
		return fmt.Errorf("reading reference: %w", err)
	}

	tree, err := loadCLITree(mrBinary, cliJSON)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	buf.WriteString(header(reference))
	buf.WriteString(transformReference(string(page)))
	buf.WriteString("\n")
	buf.WriteString(renderCLISection(tree))

	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	return writeFileAtomic(output, buf.Bytes())
}

// writeFileAtomic avoids leaving a half-written reference behind when the write
// fails partway: os.WriteFile truncates first, and CI would then diff a
// truncated file against the committed one and report it as drift.
func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".skills-gen-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func header(reference string) string {
	return fmt.Sprintf(`<!--
GENERATED FILE. DO NOT EDIT.

Language sections come from %s.
The CLI section is built from the live Cobra command tree.

Regenerate with:  npm run skills-gen
-->

`, reference)
}

var (
	frontmatterRE = regexp.MustCompile(`(?s)\A---\r?\n.*?\r?\n---\r?\n`)
	// mdLinkRE matches an inline Markdown link whose target is a relative path,
	// which is how Docusaurus cross-references other doc pages.
	mdLinkRE = regexp.MustCompile(`\]\((\.{1,2}/[^)]+)\)`)
)

// transformReference strips the Docusaurus frontmatter and rewrites relative
// doc links to absolute docs-site URLs.
func transformReference(page string) string {
	page = frontmatterRE.ReplaceAllString(page, "")
	page = mdLinkRE.ReplaceAllStringFunc(page, func(m string) string {
		target := mdLinkRE.FindStringSubmatch(m)[1]
		return "](" + absoluteDocsURL(target) + ")"
	})
	return strings.TrimLeft(page, "\n")
}

// absoluteDocsURL maps a path relative to docs-site/docs/features/ onto the
// published site. Anchors are preserved; the .md extension is dropped, which is
// how Docusaurus serves the page.
func absoluteDocsURL(target string) string {
	anchor := ""
	if i := strings.IndexByte(target, '#'); i >= 0 {
		anchor, target = target[i:], target[:i]
	}
	clean := filepath.Clean(filepath.Join("features", target))
	clean = strings.TrimSuffix(clean, ".md")
	clean = strings.TrimSuffix(clean, "/index")
	return docsBaseURL + "/" + filepath.ToSlash(clean) + anchor
}

func loadCLITree(mrBinary, cliJSON string) (dumpRoot, error) {
	var raw []byte
	var err error
	if cliJSON != "" {
		raw, err = os.ReadFile(cliJSON)
		if err != nil {
			return dumpRoot{}, fmt.Errorf("reading -cli-json: %w", err)
		}
	} else {
		raw, err = exec.Command(mrBinary, "docs", "dump", "--format", "json").Output()
		if err != nil {
			// Output() puts the command's stderr on the ExitError, and that is
			// where the actionable message lives; %w alone yields "exit status 1".
			var ee *exec.ExitError
			if errors.As(err, &ee) && len(ee.Stderr) > 0 {
				return dumpRoot{}, fmt.Errorf("running %s docs dump: %w: %s", mrBinary, err, strings.TrimSpace(string(ee.Stderr)))
			}
			return dumpRoot{}, fmt.Errorf("running %s docs dump (build it with 'go build -o mr ./cmd/mr'): %w", mrBinary, err)
		}
	}
	var tree dumpRoot
	if err := json.Unmarshal(raw, &tree); err != nil {
		return dumpRoot{}, fmt.Errorf("parsing CLI tree: %w", err)
	}
	return tree, nil
}

func renderCLISection(tree dumpRoot) string {
	byPath := make(map[string]dumpCommand, len(tree.Commands))
	for _, c := range tree.Commands {
		byPath[c.Path] = c
	}

	var b strings.Builder
	b.WriteString("## CLI Reference\n\n")
	b.WriteString("Generated from the `mr` command tree, so it tracks the binary rather than this page.\n\n")

	b.WriteString("### Global flags\n\n")
	b.WriteString("Accepted by every command below.\n\n")
	writeFlagTable(&b, tree.PersistentFlags)

	for _, path := range cliCommandPaths {
		cmd, ok := byPath[path]
		if !ok {
			continue
		}
		fmt.Fprintf(&b, "\n### `%s`\n\n", cmd.usage(path))
		fmt.Fprintf(&b, "%s.\n\n", strings.TrimSuffix(cmd.Short, "."))
		if cmd.OutputShape != "" {
			fmt.Fprintf(&b, "Output: %s.\n\n", strings.TrimSuffix(cmd.OutputShape, "."))
		}
		if len(cmd.LocalFlags) == 0 {
			b.WriteString("No command-specific flags.\n")
			continue
		}
		writeFlagTable(&b, cmd.LocalFlags)
	}
	return b.String()
}

// usage renders the command's argument spec under its full path, so the
// `save <name> <query>` usage line reads as `mr mrql save <name> <query>`.
func (c dumpCommand) usage(path string) string {
	full := "mr " + path
	fields := strings.Fields(c.Use)
	if len(fields) <= 1 {
		return full
	}
	return full + " " + strings.Join(fields[1:], " ")
}

func writeFlagTable(b *strings.Builder, flags []dumpFlag) {
	sorted := append([]dumpFlag(nil), flags...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })

	b.WriteString("| Flag | Type | Default | Description |\n|---|---|---|---|\n")
	for _, f := range sorted {
		def := f.Default
		// Blank only the zero values that carry no information, and only for the
		// type that makes them zero: a string flag defaulting to the literal
		// "false" is a real default and must survive.
		if (f.Type == "bool" && def == "false") || (strings.HasSuffix(f.Type, "Array") || strings.HasSuffix(f.Type, "Slice")) && def == "[]" {
			def = ""
		}
		// EnvVar is parsed back out of the usage string, so it is already in
		// the description. Appending it again would print it twice.
		fmt.Fprintf(b, "| `--%s` | %s | %s | %s |\n", f.Name, f.Type, code(def), escapePipes(f.Description))
	}
}

func code(s string) string {
	if s == "" {
		return ""
	}
	return "`" + s + "`"
}

// escapePipes keeps a description inside its table cell: a literal pipe would
// start a new column, and a newline would end the row.
func escapePipes(s string) string {
	s = strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(s)
	return strings.ReplaceAll(s, "|", `\|`)
}
