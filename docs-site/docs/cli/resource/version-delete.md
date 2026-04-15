---
title: mr resource version-delete
description: Delete a specific version
sidebar_label: version-delete
---

# mr resource version-delete

Delete a specific version by ID. The parent Resource is untouched. Both
`--resource-id` and `--version-id` are required. Fails if deleting would
leave the Resource with zero versions.

## Usage

```bash
mr resource version-delete
```

## Examples

**Delete an old version**

```bash
mr resource version-delete --resource-id 42 --version-id 17
```

**Pipe a list of old version IDs**

```bash
mr resource versions 42 --json | jq -r '.[1:][].id' | xargs -I {} mr resource version-delete --resource-id 42 --version-id {}
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--resource-id` | uint | `0` | Resource ID (required) **(required)** |
| `--version-id` | uint | `0` | Version ID (required) **(required)** |
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

- [`mr resource versions`](./versions.md)
- [`mr resource versions-cleanup`](./versions-cleanup.md)
