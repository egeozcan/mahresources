---
title: mr resource preview
description: Download a scaled thumbnail of a resource
sidebar_label: preview
---

# mr resource preview

Download a server-rendered thumbnail preview of a Resource. Width and
height can be capped via `-w, --width` and `--height`; without caps the
server returns its default preview size. Not every content type supports
previews (e.g., some binary formats or failed decodes).

## Usage

```bash
mr resource preview <id>
```

Positional arguments:

- `<id>`


## Examples

**Default preview**

```bash
mr resource preview 42 -o preview.jpg
```

**Constrained to 256x256 max**

```bash
mr resource preview 42 -o preview.jpg -w 256 --height 256
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output` | string | `` | Output file path (default: preview_&lt;id&gt;) |
| `--width` | uint | `0` | Preview width |
| `--height` | uint | `0` | Preview height |
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
- [`mr resource recalculate-dimensions`](./recalculate-dimensions.md)
