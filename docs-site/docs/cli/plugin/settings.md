---
title: mr plugin settings
description: Update plugin settings (pass JSON via --data)
sidebar_label: settings
---

# mr plugin settings



## Usage

    mr plugin settings <name>

Positional arguments:

- `<name>`


## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--data` | string | `{}` | Plugin settings as JSON (required) **(required)** |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

