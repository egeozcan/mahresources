---
title: mr note-type create
description: Create a new note type
sidebar_label: create
---

# mr note-type create



## Usage

    mr note-type create

## Examples


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `` | Note type name (required) **(required)** |
| `--description` | string | `` | Note type description |
| `--custom-header` | string | `` | Custom header HTML |
| `--custom-sidebar` | string | `` | Custom sidebar HTML |
| `--custom-summary` | string | `` | Custom summary HTML |
| `--custom-avatar` | string | `` | Custom avatar HTML |
| `--meta-schema` | string | `` | JSON Schema defining the metadata structure for notes of this type |
| `--section-config` | string | `` | JSON controlling which sections are visible on note detail pages |
| `--custom-mrql-result` | string | `` | Pongo2 template for rendering notes of this type in MRQL results |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

