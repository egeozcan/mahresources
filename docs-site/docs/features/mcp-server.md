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
| `template_guide`, `template_lint`, `template_preview`, `template_edit`, ... | Edit category, resource category and note type templates; see [Editing Templates](#editing-templates) |
| `search_operations` | List API operations, filtered by words, tag or safety class |
| `describe_operation` | Show an operation's arguments as a JSON Schema |
| `call_operation` | Run any operation by its `operationId` |

The operations behind `search_operations` are the ones in the [OpenAPI spec](../api/overview.md), so a new API route becomes reachable without a new tool. Operations that return files (resource views, version files, export downloads, job outputs) answer with a link to the file instead of its content; fetch it with the same Authorization header. Thumbnails up to 512 KiB come back as images. A JSON response over 8 MiB is refused with a request to narrow the query.

Long-running work, such as downloads, exports, imports and plugin actions, returns a job ID. Poll it with `get_job`.

## Editing Templates

An agent editing category, resource category or note type templates gets the same help a [Generation run](./custom-templates.md#generation-runs)'s model gets:

| Tool | What it does |
|------|--------------|
| `template_guide` | The run's reference for one carrier: shortcode, plugin and MRQL references, template partials, the carrier's name, description and detail-page layout, its Meta JSON Schema, sample members and, with `slot`, that slot's current content. Call it first. |
| `template_lint` | Lint a field. Flags shortcode errors, MRQL the renderer would reject, missing partials, bracketed words that are not shortcodes, `[meta]` paths that are not in the schema, and unbalanced HTML. Omit `content` to lint the saved value. |
| `template_preview` | Render a field against a real entity. Returns the HTML, the CSS and the text a reader sees, with `[meta]` values written in. Renders against `editing.entityId`, else one of the carrier's members. `editing.previewMRQL` runs `[mrql]` queries and `editing.previewPlugins` runs plugin shortcodes, read-only. |
| `template_check_mrql` | Check an MRQL query without running it. |
| `template_get_entity`, `template_list_related`, `template_list_members` | Read sample entities and their relations. |
| `template_get_template`, `template_list_carriers` | Read saved templates of any carrier. |
| `template_list_revisions`, `template_get_revision`, `template_diff_revisions` | Read the template history. |
| `template_get_partial`, `template_get_shortcode_docs` | Read a partial or one shortcode's full documentation. |
| `template_edit` | Save fields (`{"CustomHeader": "...", "CustomHeaderCSS": "..."}`); other fields keep their values. It lints first and refuses an unknown field before saving anything, then answers with each field's lint result and a preview of the first HTML field. |

The `template_*` read tools take an `editing` object naming the carrier being edited (`carrierType`, `carrierId`). They are the run's own Tools, served by the run's executor through `templateTool`, so a result matches what a run would see. Every save is a template revision, so a bad edit can be compared and restored. Editing a note type asks for a confirmation, like any note type edit. Like Generation runs, the template tools are not available to group-limited accounts.

## Permissions

Each tool call runs as an ordinary API request from the signed-in user, through the same authentication, role checks, group scope, plugin permissions and rate limits as the REST API. A guest can read but not write; a user limited to a group sees only that group's subtree. The MCP endpoint itself admits every signed-in role.

Read-only operations stay read-only when they run plugin code. Template and description previews render plugin shortcodes, and during a read-only call (and while a confirmation is being prepared) a plugin may only call its read functions; a shortcode that tries to write fails instead of changing anything.

A few operations are never available over MCP:

- signing in and out;
- anything that issues a credential: creating API tokens, changing a password, creating users and editing users. A token or password made here would work against the REST API directly, outside the confirmations below;
- event streams, HTML fragments rendered for the browser, and the entity picker;
- the legacy `/v1/download/*` aliases (their `/v1/jobs/*` equivalents are available).

## Confirmations

These operations are **destructive**:

- every delete, including version cleanup, clearing finished jobs, deleting download history, clearing a resource's thumbnails and revoking an API token. Deleting one of your own UI settings is the one exception;
- merges and mass edits, adding meta in bulk (it overwrites the keys it names) and replacing resource tags in bulk;
- operations that delete stored previews, including an uploaded custom thumbnail: rotating, cropping or trimming a resource, uploading or restoring a version, and setting a custom thumbnail;
- removing a resource from its series (`removeResourceFromSeries`), because the series is deleted when its last member leaves. `editResource` with `SeriesId` 0 is refused over MCP, since the JSON request it sends cannot remove series membership;
- updating an existing group (`createOrUpdateGroup` with an `ID`) and editing a relation type, because both delete the relations that no longer fit the categories;
- applying a resource reduction or a group import;
- running a plugin action or plugin schedule, and running a job command (one of them, `forget`, erases a job's stored inputs);
- sharing and unsharing notes, revoking shares in bulk, creating, editing or restoring a note type (which can publish its templates on shared notes), and writing a template partial (share pages render the stored content);
- changes to plugins and runtime settings.

`search_operations` with `safety: destructive` lists them all. A destructive call never runs on its first request. The server answers with a confirmation request that names what the call would affect, and the agent's client shows it to the user. The operation runs only after the user accepts.

Applying a resource reduction states how many clusters are checked and how many resources would be removed. Applying an import states what it creates and how many existing entities its GUID collision policy affects. For resources, notes, groups, saved queries and series, the confirmation names each target the user can see, such as `#12 "Invoice 2026"`, for up to 25 targets. Targets that do not exist or are outside the user's scope are listed as "not found or not visible", without saying which. Other targets, such as tags, are listed by ID. Mass edits also show the result of a dry run.

The confirmation also shows the call's arguments in full. A destructive call whose arguments exceed 16 KiB is refused rather than shown in part; split it into smaller calls.

A confirmation is bound to the exact call it was issued for: the same user and token, the same tool, the same operation and the same arguments. It expires after five minutes, can be used once, and does not survive a server restart.

**Clients that cannot show a confirmation.** The client must speak MCP 2026-07-28 and support form elicitation. Otherwise a destructive call is refused, the refusal includes what the call would have affected, and nothing changes. Older clients that support elicitation can confirm through `mr mcp`, which asks them over its stdio session. Automation that must delete without a person present should use the REST API or the `mr` CLI.

Argument names are matched to the operation's declared fields case-insensitively, with the same Unicode case folding the API's JSON decoder uses, and an argument given twice under different spellings (`Id` and `id`) is refused.

Operations that take a request body the API spec does not describe accept it under a `body` argument, sent as JSON. Use it for a field that shares its name with a parameter, such as a saved query's bind value named `name`: `{"id": 7, "body": {"name": "Alice"}}`. A destructive call refuses this: a field inside `body` may not repeat the name of another argument, because the handler could act on the body copy instead of the one the confirmation named. Plugin settings (`updatePluginSettings`) are the exception, since the handler reads them as a separate map that never selects the plugin.

**Mass edits** over MCP must list their targets in `ID`. A `Target` other than `ids`, and selecting targets with `Filter`, `MRQLQuery`, `MRQLParams` or `MRQLSnapshot`, are refused, because the matching set could change between the confirmation and the edit. Resolve the IDs first with a list tool or MRQL.

**Trust.** The server cannot see whether a person answered. MCP requires clients to show confirmation requests to their user, and the server relies on the client to do so. What the server guarantees is that a destructive operation never runs without a confirmation round trip, and that the confirmation covers only what it named.

## Browser Requests

`/mcp` refuses any request carrying an `Origin` or `Sec-Fetch-Site` header, unless its origin is listed in `-mcp-allowed-origins`. Agents and desktop clients send neither header. This applies with authentication disabled as well, so a web page cannot call the endpoint from a visitor's browser, including through DNS rebinding.
