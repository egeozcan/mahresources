---
outputShape: CommandTree JSON (when --format json) or directory of Markdown files (when --format markdown)
exitCodes: 0 on success; 1 on any error
relatedCmds: docs lint, docs check-examples
---

# Long

Emit the full mr command tree with rich metadata: persistent flags, per-command
local and inherited flags, required-flag lists, positional-argument contracts,
parsed examples, and Annotations (outputShape, exitCodes, relatedCmds). JSON
output is intended for agents and tooling; Markdown output is intended for the
docs-site (`docs-site/docs/cli/`).

Cobra's built-in `help` and `completion` subcommands are skipped: they are not
user-facing and are excluded from the documented contract.

# Example

  # Emit JSON to stdout (agent-friendly)
  mr docs dump --format json

  # Emit JSON to a file
  mr docs dump --format json --output /tmp/mr-tree.json

  # Regenerate docs-site pages
  mr docs dump --format markdown --output docs-site/docs/cli/
