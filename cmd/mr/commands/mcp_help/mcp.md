---
outputShape: MCP JSON-RPC messages on stdout, one per line; nothing else is written to stdout
exitCodes: 0 when the MCP client closes stdin; 1 when the server cannot be reached or does not serve /mcp
relatedCmds: auth login, token create
---

# Long

Run a local Model Context Protocol server on stdin and stdout that forwards every request to the mahresources server's `/mcp` endpoint, for MCP clients that only launch local servers. The server must be started with `-mcp`. Credentials come from the same place as every other command: `MR_TOKEN`, or the stored credentials from `mr auth login`.

The bridge adds no tools of its own. It lists the server's tools when it starts and forwards each call. Destructive operations still need a confirmation: the bridge passes the server's confirmation request to the MCP client, which shows it to its user, and passes the answer back. Because the stdio session keeps state, this also works for MCP clients on protocol versions older than 2026-07-28, which the stateless HTTP endpoint cannot ask directly. A client that cannot show a confirmation at all gets a refusal, and nothing runs.

To register the bridge with an MCP client, give the client `mr mcp` as the command and `--server` (or `MAHRESOURCES_URL`) for the server address.

# Example

  # Serve the default server (http://localhost:8181) over stdio
  mr mcp

  # Serve a remote server, authenticated with a token
  MR_TOKEN=... mr mcp --server https://mahresources.example.com

  # mr-doctest: list the bridged tools with a minimal MCP handshake
  { printf '%s\n' \
      '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"doctest","version":"1"}}}' \
      '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
      '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'; sleep 3; } \
    | mr mcp | jq -s -e 'map(select(.id == 2))[0].result.tools | map(.name) | (index("search_operations") != null and index("delete") != null)'
