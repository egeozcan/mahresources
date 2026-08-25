---
title: mr docs lint
description: Validate every command's help against the template
sidebar_label: lint
---

# mr docs lint

Validate every user-facing command's help against the template rules defined
in the spec: Short, Long, ≥2 Examples per leaf, a description on every flag,
the `exitCodes` annotation, and sensible Short length. Missing
`# mr-doctest:` examples emit warnings, not errors. A
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

Every command in the tree is linted; the allowlist that gated this during the
help-text migration is empty and survives only as a test hook.

## Usage

```bash
mr docs lint
```

## Examples

**Lint the full command tree**

```bash
mr docs lint
```

**Use in CI (non-zero exit fails the build)**

```bash
mr docs lint || exit 1
```


## Flags

This command has no local flags.
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

0 if all commands pass; 1 if any fail

## See Also

- [`mr docs dump`](./dump.md)
- [`mr docs check-examples`](./check-examples.md)
