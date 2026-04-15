---
title: mr category create
description: Create a new category
sidebar_label: create
---

# mr category create



## Usage

    mr category create

## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Category name (required) **(required)** |
| `--description` | string | `` | Category description |
| `--custom-header` | string | `` | Custom header HTML |
| `--custom-sidebar` | string | `` | Custom sidebar HTML |
| `--custom-summary` | string | `` | Custom summary HTML |
| `--custom-avatar` | string | `` | Custom avatar HTML |
| `--meta-schema` | string | `` | Meta schema JSON |
| `--section-config` | string | `` | JSON controlling which sections are visible on group detail pages for this category |
| `--custom-mrql-result` | string | `` | Pongo2 template for rendering groups of this category in MRQL results |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

