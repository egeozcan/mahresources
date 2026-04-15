---
title: mr resources set-dimensions
description: Set dimensions on multiple resources
sidebar_label: set-dimensions
---

# mr resources set-dimensions

Force the stored `width` and `height` on every Resource listed in
`--ids`. Useful when `recalculate-dimensions` cannot decode the file
format (e.g., proprietary formats) or when the stored dimensions are
known to be stale. Does not transform the file bytes; only updates the
database record. All three flags (`--ids`, `--width`, `--height`) are
required.

## Usage

    mr resources set-dimensions

## Examples

**Set dimensions on a single resource**

    mr resources set-dimensions --ids 7 --width 1920 --height 1080

**Batch update from a tag filter**

    IDS=$(mr resources list --tags 5 --json | jq -r 'map(.id) | join(",")')
    mr resources set-dimensions --ids $IDS --width 800 --height 600

**upload**

    GRP=$(mr group create --name "doctest-setdim-$$-$RANDOM" --json | jq -r '.ID')
    ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "setdim-$$" --json | jq -r '.[0].ID')
    mr resources set-dimensions --ids $ID --width 1024 --height 768
    mr resource get $ID --json | jq -e '.Width == 1024'


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--ids` | string | `` | Comma-separated resource IDs (required) **(required)** |
| `--width` | uint | `0` | Width in pixels (required) **(required)** |
| `--height` | uint | `0` | Height in pixels (required) **(required)** |
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

- [`mr resource rotate`](../resource/rotate.md)
- [`mr resource recalculate-dimensions`](../resource/recalculate-dimensions.md)
