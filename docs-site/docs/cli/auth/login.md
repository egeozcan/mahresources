---
title: mr auth login
description: Authenticate and store an API token
sidebar_label: login
---

# mr auth login

Authenticate with a username and password, mint a personal API token, and store it in the credentials file. Subsequent mr commands read that token automatically; override it any time with the MR_TOKEN environment variable.

## Usage

```bash
mr auth login
```

## Examples

**Log in to the default server**

```bash
mr auth login --username alice --password s3cret
```

**Log in to a specific server and name the token**

```bash
mr --server https://mr.example.com auth login --username alice --password s3cret --name laptop
```


## Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--username` | string | `` | Account username |
| `--password` | string | `` | Account password |
| `--name` | string | `mr cli` | Label for the minted API token |
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