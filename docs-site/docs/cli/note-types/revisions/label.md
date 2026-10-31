---
title: mr note-types revisions label
description: Set, change or clear a revision's label
sidebar_label: label
---

# mr note-types revisions label

Set, change or clear the label of a revision of a note type. Pass the new
text as the second argument, at most 200 characters, or pass `--clear` to
remove the label; giving both or neither is an error. The label is the
only part of a revision that can change after it is recorded.

## Usage

```bash
mr note-types revisions label <number> [text]
```

Positional arguments:

- `<number>`
- `<text>` (optional)


## Examples

**Label revision 2**

```bash
mr note-types revisions label 2 "before redesign" --id 3
```

**Remove the label again**

```bash
mr note-types revisions label 2 --clear --id 3
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--id` | uint | `0` | ID of the note type **(required)** |
| `--clear` | bool | `false` | Remove the label instead of setting one |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Output

Revision summary with the new label under --json; confirmation otherwise

## Exit Codes

0 on success; 1 on any error

## See Also

- [`mr note-types revisions list`](./list.md)
- [`mr note-types revisions show`](./show.md)
- [`mr note-types revisions restore`](./restore.md)
