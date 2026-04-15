---
title: mr resource rotate
description: Rotate a resource image
sidebar_label: rotate
---

# mr resource rotate

Rotate an image Resource by the given number of degrees. Only image
Resources are supported; the rotation creates a new version on success
so the original is preserved. The `--degrees` flag is required and
typically takes 90, 180, or 270 (negative values rotate counter-
clockwise).

## Usage

    mr resource rotate <id>

Positional arguments:

- `<id>`


## Examples

**Rotate 90 degrees clockwise**

    mr resource rotate 42 --degrees 90

**Rotate 180 degrees**

    mr resource rotate 42 --degrees 180

**small fixtures may fail to decode; tolerate known errors**

    GRP=$(mr group create --name "doctest-rotate-$$-$RANDOM" --json | jq -r '.ID')
    ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "rotate-test-$$" --json | jq -r '.[0].ID')
    mr resource rotate $ID --degrees 90


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--degrees` | int | `0` | Rotation degrees (required) **(required)** |
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

- [`mr resource preview`](./preview.md)
- [`mr resource edit`](./edit.md)
- [`mr resource versions`](./versions.md)
