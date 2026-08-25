---
title: mr auth login
description: Authenticate and store an API token
sidebar_label: login
---

# mr auth login

Authenticate with a username and password, mint a personal API token, and store it in the credentials file. The server must be running with `-auth`; with authentication disabled every caller is already an implicit administrator with no account of its own, and the mint step answers HTTP 400. The token is written to `$MR_TOKEN_FILE`, or `$XDG_CONFIG_HOME/mahresources/token`, or `~/.config/mahresources/token`, mode 0600 and keyed by the server's origin, so a token stored for one server is never sent to another. Subsequent mr commands read that token automatically; override it any time with the MR_TOKEN environment variable.

## Usage

```bash
mr auth login
```

## Examples

**Log in to the default server**

```bash
mr auth login --username alice --password password1
```

**Log in to a specific server and name the token**

```bash
mr --server https://mr.example.com auth login --username alice --password password1 --name laptop
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