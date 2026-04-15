package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// buildTestTree constructs a minimal dumpRoot directly (no Cobra) for testing writeMarkdown.
func buildTestTree() dumpRoot {
	return dumpRoot{
		Name:  "mr",
		Short: "mr CLI",
		PersistentFlags: []dumpFlag{
			{Name: "server", Type: "string", Default: "", Description: "server URL"},
		},
		Commands: []dumpCommand{
			{
				Path:    "foo",
				Short:   "Foo group",
				Long:    "Foo is a group",
				Use:     "foo",
				IsGroup: true,
				Args:    dumpArgs{Constraint: "none"},
				RelatedCmds: []string{"foo bar", "baz help"},
			},
			{
				Path:  "foo bar",
				Short: "Bar does stuff",
				Long:  "Bar does stuff",
				Use:   "bar --flag=x",
				Args:  dumpArgs{Constraint: "none"},
				Examples: []dumpExample{
					{Label: "basic usage", Command: "mr foo bar --flag=x"},
				},
				LocalFlags: []dumpFlag{
					{Name: "flag", Type: "string", Default: "", Description: "a local flag"},
				},
				InheritedFlags: []string{"server"},
				OutputShape:    "Thing object",
				ExitCodes:      "0 on success; 1 on any error",
				RelatedCmds:    []string{"foo", "baz help"},
			},
			{
				Path:  "baz help",
				Short: "Help for baz",
				Long:  "Help for baz",
				Use:   "help",
				Args:  dumpArgs{Constraint: "none"},
			},
		},
	}
}

func TestDumpMarkdown_RootIndex(t *testing.T) {
	dir := t.TempDir()
	tree := buildTestTree()
	if err := writeMarkdown(tree, dir); err != nil {
		t.Fatalf("writeMarkdown error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil {
		t.Fatalf("index.md not found: %v", err)
	}
	text := string(content)

	for _, want := range []string{"foo", "foo bar", "baz help"} {
		if !strings.Contains(text, want) {
			t.Errorf("index.md missing row for %q\n---\n%s", want, text)
		}
	}
}

func TestDumpMarkdown_FooGroupIndex(t *testing.T) {
	dir := t.TempDir()
	tree := buildTestTree()
	if err := writeMarkdown(tree, dir); err != nil {
		t.Fatalf("writeMarkdown error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "foo", "index.md"))
	if err != nil {
		t.Fatalf("foo/index.md not found: %v", err)
	}
	text := string(content)

	if !strings.Contains(text, "Foo is a group") {
		t.Errorf("foo/index.md missing long description\n---\n%s", text)
	}
}

func TestDumpMarkdown_FooBarLeaf(t *testing.T) {
	dir := t.TempDir()
	tree := buildTestTree()
	if err := writeMarkdown(tree, dir); err != nil {
		t.Fatalf("writeMarkdown error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "foo", "bar.md"))
	if err != nil {
		t.Fatalf("foo/bar.md not found: %v", err)
	}
	text := string(content)

	checks := []struct {
		desc string
		want string
	}{
		{"h1 title", "# mr foo bar"},
		{"long description", "Bar does stuff"},
		{"examples section header", "## Examples"},
		{"example label bold", "**basic usage**"},
		{"example command", "mr foo bar --flag=x"},
		{"flags section header", "## Flags"},
		{"local flag name", "--flag"},
		{"inherited flags section", "### Inherited global flags"},
		{"inherited flag name", "--server"},
		{"output section", "## Output"},
		{"output shape content", "Thing object"},
		{"exit codes section", "## Exit Codes"},
		{"exit codes content", "0 on success; 1 on any error"},
		{"see also section", "## See Also"},
	}
	for _, c := range checks {
		if !strings.Contains(text, c.want) {
			t.Errorf("foo/bar.md: %s — missing %q\n---\n%s", c.desc, c.want, text)
		}
	}
}

func TestDumpMarkdown_FooBarSeeAlsoLinks(t *testing.T) {
	dir := t.TempDir()
	tree := buildTestTree()
	if err := writeMarkdown(tree, dir); err != nil {
		t.Fatalf("writeMarkdown error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "foo", "bar.md"))
	if err != nil {
		t.Fatalf("foo/bar.md not found: %v", err)
	}
	text := string(content)

	// From foo/bar.md:
	//   - "foo" (parent group) → foo/index.md, same dir → ./index.md
	//   - "baz help" (cross-group) → baz/help.md, from foo/ → ../baz/help.md
	seeAlsoChecks := []struct {
		desc string
		want string
	}{
		{"parent link label", "`mr foo`"},
		{"parent link path", "./index.md"},
		{"cross-group link label", "`mr baz help`"},
		{"cross-group link path", "../baz/help.md"},
	}
	for _, c := range seeAlsoChecks {
		if !strings.Contains(text, c.want) {
			t.Errorf("foo/bar.md See Also: %s — missing %q\n---\n%s", c.desc, c.want, text)
		}
	}
}

func TestDumpMarkdown_BazHelpExists(t *testing.T) {
	dir := t.TempDir()
	tree := buildTestTree()
	if err := writeMarkdown(tree, dir); err != nil {
		t.Fatalf("writeMarkdown error: %v", err)
	}

	if _, err := os.ReadFile(filepath.Join(dir, "baz", "help.md")); err != nil {
		t.Fatalf("baz/help.md not found: %v", err)
	}
}

func TestDumpMarkdown_UsageLine(t *testing.T) {
	dir := t.TempDir()
	tree := buildTestTree()
	if err := writeMarkdown(tree, dir); err != nil {
		t.Fatalf("writeMarkdown error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "foo", "bar.md"))
	if err != nil {
		t.Fatalf("foo/bar.md not found: %v", err)
	}
	text := string(content)

	// The Use on foo bar is "bar --flag=x"; the usage line should be "mr foo bar --flag=x"
	if !strings.Contains(text, "mr foo bar --flag=x") {
		t.Errorf("foo/bar.md: usage line should contain 'mr foo bar --flag=x'\n---\n%s", text)
	}
}

func TestDumpMarkdown_PositionalArgs_Exact(t *testing.T) {
	dir := t.TempDir()
	tree := dumpRoot{
		Name:  "mr",
		Short: "mr CLI",
		Commands: []dumpCommand{
			{
				Path:  "res get",
				Short: "Get a resource",
				Long:  "Get a resource by ID",
				Use:   "get <id>",
				Args:  dumpArgs{Constraint: "exact", N: 1, Names: []string{"id"}},
			},
		},
	}
	if err := writeMarkdown(tree, dir); err != nil {
		t.Fatalf("writeMarkdown error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "res", "get.md"))
	if err != nil {
		t.Fatalf("res/get.md not found: %v", err)
	}
	text := string(content)

	if !strings.Contains(text, "Positional arguments:") {
		t.Errorf("res/get.md: missing 'Positional arguments:' section\n---\n%s", text)
	}
	if !strings.Contains(text, "<id>") {
		t.Errorf("res/get.md: missing positional arg '<id>'\n---\n%s", text)
	}
}

func TestDumpMarkdown_PositionalArgs_None(t *testing.T) {
	dir := t.TempDir()
	tree := dumpRoot{
		Name:  "mr",
		Short: "mr CLI",
		Commands: []dumpCommand{
			{
				Path:  "res list",
				Short: "List resources",
				Long:  "List all resources",
				Use:   "list",
				Args:  dumpArgs{Constraint: "none"},
			},
		},
	}
	if err := writeMarkdown(tree, dir); err != nil {
		t.Fatalf("writeMarkdown error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "res", "list.md"))
	if err != nil {
		t.Fatalf("res/list.md not found: %v", err)
	}
	text := string(content)

	if strings.Contains(text, "Positional arguments:") {
		t.Errorf("res/list.md: should NOT have 'Positional arguments:' section for 'none' constraint\n---\n%s", text)
	}
}

func TestDumpMarkdown_FooBarSeeAlsoFullLink(t *testing.T) {
	dir := t.TempDir()
	tree := buildTestTree()
	if err := writeMarkdown(tree, dir); err != nil {
		t.Fatalf("writeMarkdown error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(dir, "foo", "bar.md"))
	if err != nil {
		t.Fatalf("foo/bar.md not found: %v", err)
	}
	text := string(content)

	// Full Markdown link syntax check
	if !strings.Contains(text, "[`mr foo`](./index.md)") {
		t.Errorf("foo/bar.md: missing parent link [`mr foo`](./index.md)\n---\n%s", text)
	}
	if !strings.Contains(text, "[`mr baz help`](../baz/help.md)") {
		t.Errorf("foo/bar.md: missing cross-group link [`mr baz help`](../baz/help.md)\n---\n%s", text)
	}
}
