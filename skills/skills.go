// Package skills embeds the installable agent skills' reference files, so the
// server can hand the same MRQL reference to a model over MCP that `npx skills
// add` installs. The files are generated (cmd/skills-gen) or hand-written as
// described in README.md; this package only carries them into the binary.
package skills

import _ "embed"

// MRQLLanguage is the generated MRQL language reference
// (mahresources-mrql/references/language.md).
//
//go:embed mahresources-mrql/references/language.md
var MRQLLanguage string

// MRQLRecipes is the task-shaped MRQL recipe list
// (mahresources-mrql/references/recipes.md). Its examples are `mr` CLI
// commands.
//
//go:embed mahresources-mrql/references/recipes.md
var MRQLRecipes string
