---
title: mr docs dump
description: Emit the mr command tree as JSON or Markdown
sidebar_label: dump
---

# mr docs dump

Emit the full mr command tree with rich metadata: persistent flags, per-command
local and inherited flags, required-flag lists, positional-argument contracts,
parsed examples, and Annotations (outputShape, exitCodes, relatedCmds). JSON
output is intended for agents and tooling; Markdown output is intended for the
docs-site (`docs-site/docs/cli/`).

Cobra's built-in `help` and `completion` subcommands are skipped: they are not
user-facing and are excluded from the documented contract.

## Usage

```bash
mr docs dump
```

## Examples

**Emit JSON to stdout (agent-friendly)**

```bash
mr docs dump --format json
```

**Emit JSON to a file**

```bash
mr docs dump --format json --output /tmp/mr-tree.json
```

**Regenerate docs-site pages**

```bash
mr docs dump --format markdown --output docs-site/docs/cli/
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | `` | Output format: json (stdout by default) or markdown (requires --output). Required. **(required)** |
| `--output` | string | `` | Output `path`. Required for markdown; optional for json (stdout when omitted). |
| `--help` | bool | `false` | help for dump |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

CommandTree JSON (when --format json) or directory of Markdown files (when --format markdown)

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr docs lint`](./lint.md)
- [`mr docs check-examples`](./check-examples.md)
