---
title: mr mcp
description: Serve the server's MCP endpoint over stdio
sidebar_label: mcp
---

# mr mcp

Run a local Model Context Protocol server on stdin and stdout that forwards every request to the mahresources server's `/mcp` endpoint, for MCP clients that only launch local servers. The server must be started with `-mcp`. Credentials come from the same place as every other command: `MR_TOKEN`, or the stored credentials from `mr auth login`.

The bridge adds no tools of its own. It lists the server's tools when it starts and forwards each call. Destructive operations still need a confirmation: the bridge passes the server's confirmation request to the MCP client, which shows it to its user, and passes the answer back. Because the stdio session keeps state, this also works for MCP clients on protocol versions older than 2026-07-28, which the stateless HTTP endpoint cannot ask directly. A client that cannot show a confirmation at all gets a refusal, and nothing runs.

To register the bridge with an MCP client, give the client `mr mcp` as the command and `--server` (or `MAHRESOURCES_URL`) for the server address.

## Usage

```bash
mr mcp
```

## Examples

**Serve the default server (http://localhost:8181) over stdio**

```bash
mr mcp
```

**Serve a remote server**

```bash
MR_TOKEN=... mr mcp --server https://mahresources.example.com
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
## Output

MCP JSON-RPC messages on stdout, one per line; nothing else is written to stdout

## Exit Codes

0 when the MCP client closes stdin; 1 when the server cannot be reached or does not serve /mcp

## See Also

- [`mr auth login`](./auth/login.md)
- [`mr token create`](./token/create.md)
