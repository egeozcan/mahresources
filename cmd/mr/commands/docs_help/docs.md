---
exitCodes: 0 on success; 1 on any error
relatedCmds: docs dump, docs lint, docs check-examples
---

# Long

Introspect and validate the mr CLI's own documentation. The `docs` subcommands
walk the command tree to emit machine-readable JSON, generate docs-site
Markdown pages, validate help text against the template rules, and execute
runnable examples.

Use `mr docs` during CLI development to keep help text consistent, and in CI
to guarantee that published documentation stays in sync with the
implementation.
