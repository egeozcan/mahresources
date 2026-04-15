---
exitCodes: 0 if all commands pass; 1 if any fail
relatedCmds: docs dump, docs check-examples
---

# Long

Validate every user-facing command's help against the template rules defined
in the spec: Short, Long, ≥2 Examples per leaf, rich flag descriptions,
required Annotations (outputShape where applicable, exitCodes), and sensible
Short length. Missing `# mr-doctest:` examples emit warnings, not errors.

Lint is allowlist-gated during migration: only command groups explicitly added
to the allowlist are subject to the strict rules, so partial migrations do not
block CI.

# Example

  # Lint the full command tree
  mr docs lint

  # Use in CI (non-zero exit fails the build)
  mr docs lint || exit 1
