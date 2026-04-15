---
title: mr docs lint
description: Validate every command's help against the template
sidebar_label: lint
---

# mr docs lint

Validate every user-facing command's help against the template rules defined
in the spec: Short, Long, ≥2 Examples per leaf, rich flag descriptions,
required Annotations (outputShape where applicable, exitCodes), and sensible
Short length. Missing `# mr-doctest:` examples emit warnings, not errors.

Lint is allowlist-gated during migration: only command groups explicitly added
to the allowlist are subject to the strict rules, so partial migrations do not
block CI.

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
