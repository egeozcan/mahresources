---
title: mr token
description: Manage your API tokens
sidebar_label: token
---

# mr token

List, create, and revoke the API tokens for the authenticated account. Tokens are bearer credentials used by the CLI and other non-browser clients. They require a server running with -auth: with authentication disabled there is no account to hold a token, and every one of them answers HTTP 400.

## Usage

```bash
mr token
```

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

0 success; 1 error (not authenticated, network error, or token not found)