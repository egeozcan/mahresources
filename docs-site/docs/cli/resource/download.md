---
title: mr resource download
description: Download a resource file
sidebar_label: download
---

# mr resource download

Stream a Resource's bytes to a local file. Writes to the path given by
`-o, --output`, defaulting to `resource_<id>` in the current directory.
The file content is streamed as-is from the server; no conversion is
performed.

## Usage

    mr resource download <id>

Positional arguments:

- `<id>`


## Examples

**Download to an explicit path**

    mr resource download 42 -o ./out.jpg

**Download to the default path (resource_42)**

    mr resource download 42

**upload**

    GRP=$(mr group create --name "doctest-download-$$-$RANDOM" --json | jq -r '.ID')
    ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "dl-test-$$" --json | jq -r '.[0].ID')
    OUT=$(mktemp)
    mr resource download $ID -o $OUT
    test -s $OUT
    rm -f $OUT


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--output` | string | `` | Output file path (default: resource_<id>) |
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

- [`mr resource get`](./get.md)
- [`mr resource preview`](./preview.md)
- [`mr resource version-download`](./version-download.md)
