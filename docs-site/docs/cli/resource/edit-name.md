---
title: mr resource edit-name
description: Edit a resource's name
sidebar_label: edit-name
---

# mr resource edit-name

Update only the name of an existing resource. Shorthand for
`mr resource edit <id> --name <value>` when name is the only change.

## Usage

    mr resource edit-name <id> <new-name>

Positional arguments:

- `<id>`
- `<new-name>`


## Examples

**Rename resource 42**

    mr resource edit-name 42 "my new name"

**Rename and confirm with a follow-up get**

    mr resource edit-name 42 "renamed" && mr resource get 42 --json | jq -r .name

**upload**

    GRP=$(mr group create --name "doctest-editname-$$-$RANDOM" --json | jq -r '.ID')
    ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "before-$$" --json | jq -r '.[0].ID')
    NEWNAME="after-$$"
    mr resource edit-name $ID "$NEWNAME"
    mr resource get $ID --json | jq -e --arg n "$NEWNAME" '.Name == $n'


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

0 on success; 1 on any error

## See Also

- [`mr resource edit`](./edit.md)
- [`mr resource edit-description`](./edit-description.md)
- [`mr resource edit-meta`](./edit-meta.md)
