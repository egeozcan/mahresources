---
title: mr series remove-resource
description: Remove a resource from its series
sidebar_label: remove-resource
---

# mr series remove-resource

Remove a resource from its series. Takes the resource ID as a single
positional argument and clears the resource's `SeriesId`; the series
itself and the resource's bytes are preserved. To move a resource to a
different series instead of detaching it, use `resource edit
--series-id` on the resource.

## Usage

```bash
mr series remove-resource <resource-id>
```

Positional arguments:

- `<resource-id>`


## Examples

**Detach resource 123 from whatever series it belongs to**

```bash
mr series remove-resource 123
```

**Detach and confirm by inspecting the resource's seriesId**

```bash
mr series remove-resource 123 && mr resource get 123 --json | jq .seriesId
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

0 on success; 1 on any error

## See Also

- [`mr resource edit`](../resource/edit.md)
- [`mr series get`](./get.md)
- [`mr resources list`](../resources/list.md)
