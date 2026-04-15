---
title: mr resource version-download
description: Download a specific version file
sidebar_label: version-download
---

# mr resource version-download

Stream a specific version's bytes to a local file. Use `resource
download` to fetch the current version; this command exists to retrieve
older versions by their version ID. Output path defaults to
`version_<id>` if `-o` is not given.

## Usage

```bash
mr resource version-download <version-id>
```

Positional arguments:

- `<version-id>`


## Examples

**Download a version to an explicit path**

```bash
mr resource version-download 17 -o old.jpg
```

**Default output path**

```bash
mr resource version-download 17
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output` | string | `` | Output file path (default: version_&lt;id&gt;) |
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

- [`mr resource download`](./download.md)
- [`mr resource versions`](./versions.md)
- [`mr resource version`](./version.md)
