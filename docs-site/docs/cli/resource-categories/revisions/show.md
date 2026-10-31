---
title: mr resource-categories revisions show
description: Show one revision with its stored content
sidebar_label: show
---

# mr resource-categories revisions show

Show one revision of a resource category. `<number>` is the per-resource category revision
number from `list`, and `--id` is required. The output is the revision's
metadata followed by each non-empty field exactly as it was stored, which
for an old resource category can include a MetaSchema that is not valid JSON.
Under `--json` the `snapshot` object holds every recorded field, and the
`editor` object holds the same fields as the edit form loads them.

## Usage

```bash
mr resource-categories revisions show <number>
```

Positional arguments:

- `<number>`


## Examples

**Show revision 2 of a resource category**

```bash
mr resource-categories revisions show 2 --id 3
```

**Print one stored field**

```bash
mr resource-categories revisions show 2 --id 3 --json | jq -r '.snapshot.CustomHeader'
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--id` | uint | `0` | ID of the resource category **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Revision summary plus snapshot, the stored value of every tracked field the revision recorded

## Exit Codes

0 on success; 1 on any error