---
title: mr resource version-restore
description: Restore a resource to a previous version
sidebar_label: version-restore
---

# mr resource version-restore

Restore a previous version to be the current version of a Resource.
Creates a new version that is a copy of the target (the original target
version is preserved). Both `--resource-id` and `--version-id` are
required. The optional `--comment` annotates the restore for the audit
trail.

## Usage

    mr resource version-restore

## Examples

**Restore with a comment**

    mr resource version-restore --resource-id 42 --version-id 17 --comment "revert bad edit"

**Silent restore**

    mr resource version-restore --resource-id 42 --version-id 17

**upload**

    GRP=$(mr group create --name "doctest-vrestore-$$-$RANDOM" --json | jq -r '.ID')
    ID=$(mr resource upload ./testdata/sample.jpg --owner-id=$GRP --name "restore-test-$$" --json | jq -r '.[0].ID')
    V1=$(mr resource versions $ID --json | jq -r '.[0].id')
    mr resource version-upload $ID ./testdata/sample.png
    BEFORE=$(mr resource versions $ID --json | jq -r 'length')
    mr resource version-restore --resource-id $ID --version-id $V1
    AFTER=$(mr resource versions $ID --json | jq -r 'length')
    test "$AFTER" -gt "$BEFORE"


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--resource-id` | uint | `0` | Resource ID (required) **(required)** |
| `--version-id` | uint | `0` | Version ID (required) **(required)** |
| `--comment` | string | `` | Restore comment |
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
- [`mr resource version-upload`](./version-upload.md)
