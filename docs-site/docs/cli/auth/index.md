---
title: mr auth
description: Log in, log out, and inspect the current identity
sidebar_label: auth
---

# mr auth

Manage CLI authentication against a mahresources server. Logging in mints an API token and stores it so subsequent commands are authenticated automatically.

## Usage

```bash
mr auth
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

0 success; 1 error (login failed, network error, or not authenticated)