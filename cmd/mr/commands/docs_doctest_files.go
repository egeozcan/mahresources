package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Markdown outside the CLI's own help text also carries runnable examples: the
// installable agent skill under skills/ is nothing but examples, and an example
// that no longer works teaches an agent the wrong command. `--files` runs those
// blocks through the same doctest harness the help-text examples use.
//
// The default is inverted from the help-text case on purpose. There, a `# Example`
// section mixes illustration with doctests, so a block opts in with `# mr-doctest:`.
// Here every fenced bash block is meant to be a working command, so every block
// runs and a block that cannot (it needs fixtures, writes files, or is destructive)
// opts out with `# mr-doctest: skip, <reason>`.

const doctestDirective = "# mr-doctest:"

// markdownExamples extracts one doctest per fenced shell block, and reports how
// many blocks opted out with `# mr-doctest: skip`. The count is returned rather
// than dropped so the run's summary cannot imply blocks were verified when they
// were deliberately not run.
func markdownExamples(path string) ([]dumpExample, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")

	var out []dumpExample
	optedOut := 0
	var open *fence
	blockStart := 0
	var body []string

	for i, line := range lines {
		if open == nil {
			if f, ok := openingFence(line); ok {
				open, blockStart, body = &f, i+1, nil
			}
			continue
		}
		if open.closedBy(line) {
			switch ex, ok := buildMarkdownExample(path, blockStart, body); {
			case ok:
				out = append(out, ex)
			case ex.Label != "":
				optedOut++ // a skip directive; a block with no command has no label
			}
			open = nil
			continue
		}
		// Strip the fence's own indentation so an example nested in a list item
		// still runs as written: left in place, a heredoc terminator indented
		// with its block would no longer terminate anything.
		body = append(body, trimIndent(line, open.indent))
	}
	if open != nil {
		return nil, 0, fmt.Errorf("%s: unterminated code fence opened at line %d", path, blockStart)
	}
	return out, optedOut, nil
}

// fence is an open code fence, tracked per CommonMark: a closing fence must use
// the same character and be at least as long as the opening one, so a block
// that quotes the other fence style does not end early.
type fence struct {
	char   byte
	length int
	indent int
}

// openingFence reports whether the line opens a *shell* block. Blocks in
// another language, or with no language, are prose rather than commands.
func openingFence(line string) (fence, bool) {
	indent := len(line) - len(strings.TrimLeft(line, " \t"))
	rest := strings.TrimLeft(line, " \t")
	if len(rest) == 0 || (rest[0] != '`' && rest[0] != '~') {
		return fence{}, false
	}
	char := rest[0]
	n := 0
	for n < len(rest) && rest[n] == char {
		n++
	}
	if n < 3 {
		return fence{}, false
	}
	switch strings.TrimSpace(rest[n:]) {
	case "bash", "sh", "shell":
		return fence{char: char, length: n, indent: indent}, true
	}
	return fence{}, false
}

func (f fence) closedBy(line string) bool {
	rest := strings.TrimLeft(line, " \t")
	n := 0
	for n < len(rest) && rest[n] == f.char {
		n++
	}
	// A closing fence carries no info string, though trailing spaces are fine.
	return n >= f.length && strings.TrimSpace(rest[n:]) == ""
}

func trimIndent(line string, indent int) string {
	for i := 0; i < indent; i++ {
		if len(line) == 0 || (line[0] != ' ' && line[0] != '\t') {
			break
		}
		line = line[1:]
	}
	return line
}

// buildMarkdownExample turns one block into a doctest, consuming any leading
// `# mr-doctest:` directive lines. It returns false for a block that opted out
// or carries no command.
func buildMarkdownExample(path string, startLine int, body []string) (dumpExample, bool) {
	ex := dumpExample{Doctest: true, Label: fmt.Sprintf("%s:%d", filepath.ToSlash(path), startLine)}

	for len(body) > 0 {
		trimmed := strings.TrimSpace(body[0])
		if !strings.HasPrefix(trimmed, doctestDirective) {
			break
		}
		meta := strings.TrimSpace(strings.TrimPrefix(trimmed, doctestDirective))
		body = body[1:]
		if isSkipDirective(meta) {
			// Keep the label so the caller can tell "opted out" from "no command".
			return dumpExample{Label: ex.Label}, false
		}
		origin := ex.Label
		if i := strings.Index(origin, " "); i >= 0 {
			origin = origin[:i] // the file:line prefix, never a previous directive's label
		}
		_, ex = applyExampleMetadata(meta, ex)
		// applyExampleMetadata takes the label from the directive text; keep the
		// file:line prefix so a failure names the block it came from.
		ex.Label = origin + " " + ex.Label
		ex.Doctest = true
	}

	command := strings.TrimSpace(strings.Join(body, "\n"))
	if command == "" {
		return dumpExample{}, false
	}
	ex.Command = command
	return ex, true
}

func isSkipDirective(meta string) bool {
	head, _, _ := strings.Cut(meta, ",")
	return strings.EqualFold(strings.TrimSpace(head), "skip")
}

// checkMarkdownExamples runs every extracted block against the server, using the
// same runner, cwd and environment as the help-text doctests.
func checkMarkdownExamples(paths []string, serverURL, environment string) error {
	serverURL = resolveServerURL(serverURL)

	// Some of these blocks write files (`mrql export -o out.csv`), so they run in
	// a scratch directory: CI diffs the working tree right afterwards. Fixtures
	// still resolve against cmd/mr, so `stdin=<name>` keeps working here too.
	fixtureRoot, err := doctestCwd()
	if err != nil {
		return err
	}
	cwd, err := os.MkdirTemp("", "mr-doctest-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(cwd)

	mrPath, err := resolveMrBinary()
	if err != nil {
		return err
	}
	env := append(os.Environ(),
		"MAHRESOURCES_URL="+serverURL,
		"PATH="+prependPath(os.Getenv("PATH"), filepath.Dir(mrPath)),
	)

	files, err := expandMarkdownPaths(paths)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("--files matched no markdown files")
	}

	var failures []string
	ran, skipped, optedOut := 0, 0, 0
	for _, file := range files {
		examples, fileOptedOut, err := markdownExamples(file)
		if err != nil {
			return err
		}
		optedOut += fileOptedOut
		if len(examples) == 0 && fileOptedOut == 0 {
			// A markdown file named explicitly but yielding nothing is nearly
			// always a mistake (a fence that is not tagged bash/sh/shell), and
			// silently running zero examples is the one outcome that looks like
			// success while gating nothing.
			return fmt.Errorf("%s: no runnable shell blocks found; tag the fence ```bash, or drop the file from --files", file)
		}
		for _, ex := range examples {
			if ex.SkipOn != "" && ex.SkipOn == environment {
				skipped++
				fmt.Printf("SKIP  %s (skip-on=%s)\n", ex.Label, ex.SkipOn)
				continue
			}
			ran++
			if err := runDoctest(ex, cwd, fixtureRoot, env); err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", ex.Label, err))
				fmt.Printf("FAIL  %s\n", ex.Label)
			} else {
				fmt.Printf("PASS  %s\n", ex.Label)
			}
		}
	}
	fmt.Printf("%d markdown example(s) run, %d opted out, %d skipped for this environment, in %d file(s)\n",
		ran, optedOut, skipped, len(files))

	if len(failures) > 0 {
		for _, f := range failures {
			fmt.Fprintln(os.Stderr, f)
		}
		return fmt.Errorf("%d doctest failures", len(failures))
	}
	return nil
}

// expandMarkdownPaths resolves globs and directories to a de-duplicated list of
// markdown files, in the order the patterns were given (filepath.Glob and
// WalkDir are both lexical, so the result is stable). Order is preserved rather
// than sorted because examples can depend on earlier ones having run.
// A directory means "every .md underneath it", so `--files skills/` keeps
// working as the skill grows.
func expandMarkdownPaths(patterns []string) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if strings.EqualFold(filepath.Ext(p), ".md") && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}

	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("bad --files pattern %q: %w", pattern, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("--files pattern %q matched nothing", pattern)
		}
		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				return nil, err
			}
			if !info.IsDir() {
				add(m)
				continue
			}
			if err := filepath.WalkDir(m, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() {
					add(p)
				}
				return nil
			}); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}
