---
title: mr resource-category create
description: Create a new resource category
sidebar_label: create
---

# mr resource-category create



## Usage

    mr resource-category create

## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Resource category name (required) **(required)** |
| `--description` | string | `` | Resource category description |
| `--custom-header` | string | `` | Custom header HTML |
| `--custom-sidebar` | string | `` | Custom sidebar HTML |
| `--custom-summary` | string | `` | Custom summary HTML |
| `--custom-avatar` | string | `` | Custom avatar HTML |
| `--meta-schema` | string | `` | Meta schema JSON |
| `--section-config` | string | `` | JSON controlling which sections are visible on resource detail pages for this category |
| `--custom-mrql-result` | string | `` | Pongo2 template for rendering resources of this category in MRQL results |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

