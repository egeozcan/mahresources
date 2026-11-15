---
title: MCP Server
description: Let AI agents use mahresources through the Model Context Protocol
---

# MCP Server

Mahresources can serve the [Model Context Protocol](https://modelcontextprotocol.io) (MCP) at `/mcp`, so an AI agent can search, read and change your data through the same API the web UI and the `mr` CLI use. Every REST operation the signed-in user may call is reachable, and destructive ones only run after the user confirms them in the agent's client.

The endpoint speaks MCP 2026-07-28 over stateless Streamable HTTP. Clients on older protocol versions are served by the same endpoint.

## Turning It On

Start the server with `-mcp` (or `MCP_ENABLED=1`). It is off by default.

```bash
./mahresources -mcp ...
```

| Flag | Env Variable | Default | Description |
|------|--------------|---------|-------------|
| `-mcp` | `MCP_ENABLED=1` | `false` | Serve the MCP endpoint at `/mcp` |
| `-mcp-allowed-origins` | `MCP_ALLOWED_ORIGINS` | (empty) | Comma-separated browser origins `/mcp` accepts. Empty refuses every browser request |

## Connecting a Client

**HTTP clients.** Point the client at `http://<host>:<port>/mcp`. With [authentication](./authentication.md) enabled, the client must send an API token as `Authorization: Bearer <token>`. Create one with `mr token create` or on the account page. A browser session cookie is not accepted.

**Clients that only launch local servers.** Use [`mr mcp`](../cli/mcp.md) as the command. It serves the endpoint over stdin and stdout, with the credentials `mr` already uses (`MR_TOKEN`, or the stored login from `mr auth login`):

```json
{
  "mcpServers": {
    "mahresources": {
      "command": "mr",
      "args": ["mcp", "--server", "http://localhost:8181"]
    }
  }
}
```

## Tools

The server offers a few named tools for common work and three generic tools that reach everything else.

| Tool | What it does |
|------|--------------|
| `search` | Full-text search across every entity type |
| `mrql_query` | Run an [MRQL](./mrql.md) query |
| `list_resources`, `get_resource` | List and read resources |
| `list_notes`, `get_note` | List and read notes |
| `list_groups`, `get_group` | List and read groups |
| `list_tags` | List tags |
| `create_note`, `create_group` | Create, or update when `ID` is set |
| `edit_resource` | Edit a resource's fields |
| `upload_resource` | Upload files, each given as `{name, contentBase64}` |
| `add_resource_from_url` | Fetch a URL on the server and store it as a resource |
| `add_tags`, `remove_tags` | Tag or untag resources, notes or groups |
| `delete` | Delete one resource, note, group or tag |
| `merge` | Merge resources, groups or tags into one winner |
| `get_job`, `list_jobs` | Follow background jobs |
| `search_operations` | List API operations, filtered by words, tag or safety class |
| `describe_operation` | Show an operation's arguments as a JSON Schema |
| `call_operation` | Run any operation by its `operationId` |

The operations behind `search_operations` are the ones in the [OpenAPI spec](../api/overview.md), so a new API route becomes reachable without a new tool. Operations that return files (resource views, version files, export downloads, job outputs) answer with a link to the file instead of its content; fetch it with the same Authorization header. Thumbnails up to 512 KiB come back as images. A JSON response over 8 MiB is refused with a request to narrow the query.

Long-running work, such as downloads, exports, imports and plugin actions, returns a job ID. Poll it with `get_job`.

## Permissions

Each tool call runs as an ordinary API request from the signed-in user, through the same authentication, role checks, group scope, plugin permissions and rate limits as the REST API. A guest can read but not write; a user limited to a group sees only that group's subtree. The MCP endpoint itself admits every signed-in role.

A few operations are never available over MCP:

- signing in and out;
- anything that issues a credential: creating API tokens, changing a password, creating users and editing users. A token or password made here would work against the REST API directly, outside the confirmations below;
- event streams, HTML fragments rendered for the browser, and the entity picker;
- the legacy `/v1/download/*` aliases (their `/v1/jobs/*` equivalents are available).

## Confirmations

These operations are **destructive**: every delete (including version cleanup, clearing finished jobs, deleting download history, clearing a resource's thumbnails, and revoking an API token), merges, mass edits, adding meta in bulk (it overwrites the keys it names), replacing resource tags in bulk, applying a resource reduction or a group import, running a job command (one of them, `forget`, erases a job's stored inputs), sharing and unsharing notes, revoking shares in bulk, and changes to plugins or runtime settings. `search_operations` with `safety: destructive` lists them all. A destructive call never runs on its first request. The server answers with a confirmation request that names what the call would affect, and the agent's client shows it to the user. The operation runs only after the user accepts.

Applying a resource reduction states how many clusters are checked and how many resources would be removed. Applying an import states what it creates and how many existing entities its GUID collision policy affects. For resources, notes, groups, saved queries and series, the confirmation names each target the user can see, such as `#12 "Invoice 2026"`, for up to 25 targets. Targets that do not exist or are outside the user's scope are listed as "not found or not visible", without saying which. Other targets, such as tags, are listed by ID. Mass edits also show the result of a dry run.

A confirmation is bound to the exact call it was issued for: the same user and token, the same tool, the same operation and the same arguments. It expires after five minutes, can be used once, and does not survive a server restart.

**Clients that cannot show a confirmation.** The client must speak MCP 2026-07-28 and support form elicitation. Otherwise a destructive call is refused, the refusal includes what the call would have affected, and nothing changes. Older clients that support elicitation can confirm through `mr mcp`, which asks them over its stdio session. Automation that must delete without a person present should use the REST API or the `mr` CLI.

Argument names are matched to the operation's declared fields case-insensitively, the way the API decodes them, and an argument given twice under different spellings (`Id` and `id`) is refused.

**Mass edits** over MCP must list their targets in `ID`. A `Target` other than `ids`, and selecting targets with `Filter`, `MRQLQuery`, `MRQLParams` or `MRQLSnapshot`, are refused, because the matching set could change between the confirmation and the edit. Resolve the IDs first with a list tool or MRQL.

**Trust.** The server cannot see whether a person answered. MCP requires clients to show confirmation requests to their user, and the server relies on the client to do so. What the server guarantees is that a destructive operation never runs without a confirmation round trip, and that the confirmation covers only what it named.

## Browser Requests

`/mcp` refuses any request carrying an `Origin` or `Sec-Fetch-Site` header, unless its origin is listed in `-mcp-allowed-origins`. Agents and desktop clients send neither header. This applies with authentication disabled as well, so a web page cannot call the endpoint from a visitor's browser, including through DNS rebinding.
