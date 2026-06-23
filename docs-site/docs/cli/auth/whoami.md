---
title: mr auth whoami
description: Show the authenticated principal
sidebar_label: whoami
---

# mr auth whoami

Print the identity and capabilities the server associates with the current credentials. Useful to confirm a token works and which role it has.

## Usage

```bash
mr auth whoami
```

## Examples

**Show the current identity**

```bash
mr auth whoami
```

**As raw JSON**

```bash
mr auth whoami --json
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