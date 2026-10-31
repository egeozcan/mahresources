---
title: mr categories revisions restore
description: Restore a revision, or one slot of it
sidebar_label: restore
---

# mr categories revisions restore

Restore a revision of a category. Without `--slot` the whole revision becomes
the category's current templates; with `--slot` only that slot is replaced and
every other field stays as it is. `--slot` takes the primary field name,
such as `CustomHeader`, never the `...CSS` sibling. The slot's CSS comes
along unless you pass `--no-css`, which requires `--slot`.

Restoring moves forward: it records a new revision with source `restore`
and never removes later ones, even when the content already matches the
current state. A slot the old revision did not have is restored as empty.
A revision whose content no longer passes the ordinary save validation is
refused with an error naming the field, and nothing is recorded.

## Usage

```bash
mr categories revisions restore <number>
```

Positional arguments:

- `<number>`


## Examples

**Restore the whole of revision 1**

```bash
mr categories revisions restore 1 --id 3
```

**Restore only the header and keep the current header CSS**

```bash
mr categories revisions restore 2 --id 3 --slot CustomHeader --no-css
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--id` | uint | `0` | ID of the category **(required)** |
| `--slot` | string | `` | Restore only this slot, for example CustomHeader (default: the whole revision) |
| `--no-css` | bool | `false` | With --slot, leave the slot's CSS as it is now |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Summary of the new restore revision with number, source, restoredFrom and changedSlots under --json; confirmation otherwise

## Exit Codes

0 on success; 1 on any error