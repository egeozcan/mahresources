---
title: mr resource version-upload
description: Upload a new version of a resource
sidebar_label: version-upload
---

# mr resource version-upload

Push a new version of an existing Resource. The new bytes replace the
current version pointer; previous versions remain accessible via their
version IDs. The `--comment` flag attaches a free-form note (useful for
"rotated 90°" or "rescanned" audit trails).

## Usage

    mr resource version-upload <resource-id> <file>

Positional arguments:

- `<resource-id>`
- `<file>`


## Examples

**Upload a new version**

    mr resource version-upload 42 ./photo_v2.jpg

**With a comment**

    mr resource version-upload 42 ./photo_v2.jpg --comment "color corrected"

**upload**

    GRP=$(mr group create --name "doctest-vupload-$$-$RANDOM" --json | jq -r '.ID')
    ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "vup-test-$$" --json | jq -r '.[0].ID')
    BEFORE=$(mr resource versions $ID --json | jq -r 'length')
    mr resource version-upload $ID ./testdata/sample.png
    AFTER=$(mr resource versions $ID --json | jq -r 'length')
    test "$AFTER" -gt "$BEFORE"


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--comment` | string | `` | Version comment |
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
- [`mr resource version`](./version.md)
- [`mr resource version-restore`](./version-restore.md)
