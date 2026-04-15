---
title: mr series
description: Manage resource series (list, create, edit, delete)
sidebar_label: series
---

# mr series

A Series is an ordered collection of Resources, typically used for content
that has an intrinsic sequence: a volume of a manga, a photo shoot, the
chapters of a scanned document. A Resource may belong to at most one
Series via its `SeriesId` reference, and removing that reference detaches
the Resource from the Series without deleting either.

Use the `series` subcommands to manage a series by ID: fetch it, create
a new one, rename or fully edit it, delete it, remove a resource from
its series, or list series matching filters. Series membership is
assigned on the Resource side (see `resource edit --series-id`), so to
attach a resource to a series edit the resource.

## Usage

    mr series

## Examples


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

- [`mr resource`](../resource/index.md)
- [`mr resources list`](../resources/list.md)
- [`mr groups list`](../groups/list.md)
