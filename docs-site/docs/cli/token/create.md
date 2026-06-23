---
title: mr token create
description: Mint a new API token
sidebar_label: create
---

# mr token create

Create a new API token for the authenticated account and print the secret once. Store it securely; it cannot be retrieved again.

## Usage

```bash
mr token create
```

## Examples

**Create a token labelled 'ci'**

```bash
mr token create --name ci
```

**Create a token that expires in 30 days**

```bash
mr token create --name temp --expires-in 720h
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--name` | string | `mr cli` | Label for the token |
| `--expires-in` | string | `` | Optional expiry as a Go duration (e.g. 720h); empty = never |
### Inherited global flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | bool | `false` | Output raw JSON |
| `--no-header` | bool | `false` | Omit table headers |
| `--page` | int | `1` | Page number for list commands (default page size: 50) |
| `--quiet` | bool | `false` | Only output IDs |
| `--server` | string | `http://localhost:8181` | mahresources server URL (env: MAHRESOURCES_URL) |
## Exit Codes

0 success; 1 error (not authenticated, network error, or token not found)