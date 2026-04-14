// Package helptext parses Markdown help files used by the mr CLI's
// Cobra commands. Each file has YAML-ish front matter (key: value lines
// between `---` fences) plus named sections (# Long, # Example).
package helptext

import (
	"bufio"
	"embed"
	"fmt"
	"io/fs"
	"strings"
)

// Help holds the parsed contents of a help Markdown file.
type Help struct {
	Long        string
	Example     string
	Annotations map[string]string
}

// Load reads a help Markdown file from the given embedded filesystem
// and returns its parsed Help. Load panics on any error: help files
// are validated at program startup, so errors are developer mistakes
// that should halt the binary immediately.
func Load(fsys embed.FS, path string) Help {
	data, err := fs.ReadFile(fsys, path)
	if err != nil {
		panic(fmt.Errorf("helptext: reading %s: %w", path, err))
	}
	h, err := parse(string(data))
	if err != nil {
		panic(fmt.Errorf("helptext: parsing %s: %w", path, err))
	}
	return h
}

func parse(s string) (Help, error) {
	annotations := map[string]string{}
	var long, example strings.Builder
	section := ""

	scanner := bufio.NewScanner(strings.NewReader(s))
	// Increase buffer to handle long Example blocks.
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)

	inFrontMatter := false
	sawFrontMatter := false
	lineNum := 0
	for scanner.Scan() {
		line := scanner.Text()
		lineNum++

		switch {
		case lineNum == 1 && line == "---":
			inFrontMatter = true
			sawFrontMatter = true
			continue
		case inFrontMatter && line == "---":
			inFrontMatter = false
			continue
		case inFrontMatter:
			if strings.TrimSpace(line) == "" {
				continue
			}
			idx := strings.Index(line, ":")
			if idx < 0 {
				return Help{}, fmt.Errorf("front matter line %d missing colon: %q", lineNum, line)
			}
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			annotations[key] = val
			continue
		}

		if strings.HasPrefix(line, "# ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "# "))
			continue
		}

		switch section {
		case "Long":
			long.WriteString(line)
			long.WriteByte('\n')
		case "Example":
			example.WriteString(line)
			example.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return Help{}, err
	}
	if !sawFrontMatter {
		return Help{}, fmt.Errorf("missing front matter (file must start with `---`)")
	}

	longStr := strings.TrimSpace(long.String())
	exampleStr := strings.TrimSpace(example.String())
	if longStr == "" {
		return Help{}, fmt.Errorf("missing `# Long` section")
	}

	return Help{
		Long:        longStr,
		Example:     exampleStr,
		Annotations: annotations,
	}, nil
}
