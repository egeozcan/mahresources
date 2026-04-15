---
title: mr resource
description: Upload, download, edit, or version a resource
sidebar_label: resource
---

# mr resource

Resources are files stored in mahresources. A Resource has a name,
content bytes, MIME type, optional dimensions, perceptual hash, and
free-form meta JSON. Resources relate many-to-many to Tags, Notes, and
Groups, and support versioned edits (see `versions`, `version-upload`).

Use the `resource` subcommands to operate on a single resource by ID:
fetch metadata, upload a file, rotate an image, or manage its version
history. Use `resources list` to discover resources matching filters.

## Usage

```bash
mr resource
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

- [`mr resources list`](../resources/list.md)
- [`mr groups list`](../groups/list.md)
- [`mr tags list`](../tags/list.md)
