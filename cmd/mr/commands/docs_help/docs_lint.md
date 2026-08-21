---
exitCodes: 0 if all commands pass; 1 if any fail
relatedCmds: docs dump, docs check-examples
---

# Long

Validate every user-facing command's help against the template rules defined
in the spec: Short, Long, ≥2 Examples per leaf, rich flag descriptions,
required Annotations (outputShape where applicable, exitCodes), and sensible
Short length. Missing `# mr-doctest:` examples emit warnings, not errors. A
doctest whose body is *empty* is an error rather than a warning: the runner
hands the body to `bash -c`, an empty one exits 0, and it reports PASS having
executed nothing. The usual cause is a stray `#` line inside a block, since
every line beginning with `#` starts a new example and so splits the block.

Example *order* is checked for the same reason: a non-doctest example that
follows a doctest is an error. When the stray `#` is not the first line of a
block, the doctest keeps a non-empty body and everything below the comment,
usually the assertions, becomes a separate example that no pass ever runs, so
the block silently stops half way. Doctests therefore come last in every
command's Example, and an illustrative example that is deliberately not
runnable has to be moved above the doctest rather than left at the end.

Lint is allowlist-gated during migration: only command groups explicitly added
to the allowlist are subject to the strict rules, so partial migrations do not
block CI.

# Example

  # Lint the full command tree
  mr docs lint

  # Use in CI (non-zero exit fails the build)
  mr docs lint || exit 1

  # mr-doctest: the committed help text passes its own lint
  mr docs lint
