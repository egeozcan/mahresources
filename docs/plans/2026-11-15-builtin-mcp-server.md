# Plan: built-in MCP server

## Context

Agents should be able to drive every mahresources function. The `mr` CLI
reaches about 150 of the 256 `/v1` routes. The OpenAPI registry
(`server.RegisterAPIRoutesWithOpenAPI`) already describes all of them: 274
paths and 298 operations, with an entry for every registered `/v1` route.

Decided with the user:

- **Protocol:** MCP 2026-07-28 (stateless), served over Streamable HTTP inside
  the main binary.
- **Tool shape:** hybrid. A few named tools for common work, plus generic tools
  that reach every other operation.
- **Confirmations are mandatory** for destructive operations, and the server
  enforces them.

Revision 2 follows pi's review of revision 1. The ten findings and how each is
resolved are listed at the end.

## SDK facts (verified in source, go-sdk v1.8.0)

- `StreamableHTTPOptions{Stateless: true}` accepts 2026-07-28 and every older
  protocol version (`mcp/streamable.go:871-876`).
- On a stateless server, an old-protocol request gets a synthesized
  `InitializeParams` with no capabilities (`streamable.go:504-511`). The SDK
  rejects server-to-client requests there (`streamable.go:129-146`), so the
  legacy elicitation fallback in `serverMultiRoundTripMiddleware`
  (`mcp/mrtr.go:124-160`) fails for those clients.
- A new-protocol request carries its capabilities in `_meta`, which
  `ServerRequest.ClientCapabilities()` reads (`mcp/shared.go:712-724`). Multi
  Round-Trip Requests (MRTR) are a returned result plus an echoed
  `requestState`, not a request from server to client, so they work on a
  stateless server (`mrtr.go:53-70`).
- The SDK says `requestState` must be encrypted, signed and verified
  (`mcp/protocol.go:313-318`).
- The SDK's own Host check covers loopback listeners only, and
  `CrossOriginProtection` is nil by default (`streamable.go:175-195, 316-333`).
- We set no `MCPGODEBUG` flags, because they are removed in v1.9.0. The module
  needs Go 1.25; the repo is on 1.26.4.

## Architecture

The code lives in package `server/mcpserver`. It sits inside `server/` because
`TestServerIsALeaf` (`internal/arch/layering_test.go:122`) forbids packages
outside it from importing `server/...`. It imports `server/openapi` and the
SDK, but not `server` itself: `CreateServer` constructs the handler and hands
it the dispatch target, so there is no import cycle.

1. **Turning it on.** `-mcp` / `MCP_ENABLED`, default off. Same flag-plus-env
   pattern as the rest of `main.go`. Also `-mcp-allowed-origins` /
   `MCP_ALLOWED_ORIGINS`, empty by default.
2. **Endpoint.** `CreateServer` builds the middleware chain (security headers,
   authentication, CSRF, authorization, body limit, then the router) as one
   handler `h`. When MCP is on, it registers `POST /mcp` on the router with
   `mcpserver.New(Options{Dispatch: h, Spec: registry.GenerateSpec(), ...})`.
   `GET` and `DELETE` on `/mcp` return 405, because there are no sessions.
3. **Admission.** All of this runs in a wrapper *outside* the CSRF middleware,
   so a cookie-only request gets 401 rather than CSRF's 403.
   - `requiredCapability` gets an explicit `/mcp` case returning `capRead`, so
     any signed-in role (guest included) is let in to the endpoint. Each
     dispatched operation is then authorized on its own. `/mcp` is **not**
     added to `isReadViaPost`, because `csrfExempt` reuses that list.
   - When auth is enabled, the handler requires `Authorization: Bearer`. A
     cookie-only request gets 401, which also keeps `/mcp` out of CSRF reach.
   - **Origin policy**, applied whether auth is on or off: a request carrying
     an `Origin` or `Sec-Fetch-Site` header is a browser request and is refused
     with 403, unless its `Origin` is listed in `-mcp-allowed-origins`.
     Non-browser MCP clients send neither header. This also covers DNS
     rebinding, where `Origin` and `Host` match each other and a same-origin
     check would pass.
4. **Dispatch.** Each tool call is turned into an `http.Request` and sent
   through `h`. The request:
   - takes its context from the incoming `/mcp` request, so a disconnect
     cancels the handler. `PropagateRequestCancellation` is also set; for
     legacy clients, cancellation is only through disconnect;
   - copies `Authorization`, `User-Agent`, `X-Forwarded-For`,
     `X-Forwarded-Proto`, `X-Real-Ip` and `RemoteAddr`, so client-IP trust
     and rate limits behave as they do for the REST call.
   - Roles, group scope, plugin permissions and body limits all run unchanged.
   - The response is captured by a **bounded recorder**: 8 MiB for JSON. A
     larger response is cut off, the dispatched context is cancelled, and the
     tool returns `isError` asking for a narrower query.
   - Operations whose only success content type is not JSON (file views,
     version files, export downloads, job outputs) are not dispatched. They
     return a `resource_link` to the guarded URL. The exceptions are
     thumbnails (`getResourcePreview`, `getVersionThumbnail`): they are
     dispatched with a 512 KiB cap and inlined as image content.
5. **Catalog,** built from `registry.GenerateSpec()`, so the registry's own
   conversion rules apply. That covers ID query params, path params, header
   params, pagination and multipart. The input schema is merged from:
   - the OpenAPI parameters (`in: query/path/header`);
   - the request body schema, with `$ref`s resolved, preferring JSON, then
     form, then multipart.
   - Multipart file fields take `{name, contentBase64}` objects and are capped
     by the existing upload body limit.
   - Some registry entries leave out inputs their handlers read; `editNoteName`,
     for example, declares `id` but not `Name`. The input schema therefore
     allows additional properties and passes them through as form or query
     fields. Registry gaps found along the way are fixed.
   - **Named tools:**
     - `search`, `mrql_query`;
     - `get_resource`, `list_resources`, `get_note`, `list_notes`, `get_group`,
       `list_groups`, `list_tags`;
     - `create_note`, `create_group`, `edit_resource`, `upload_resource`
       (base64), `add_resource_from_url`;
     - `add_tags` / `remove_tags` (resources, notes, groups);
     - `get_job`, `list_jobs`;
     - `delete` (resource, note, group, tag);
     - `merge` (resources, groups, tags).

     Each named tool maps to one `operationId` plus fixed arguments and runs
     through the same code path as `call_operation`.
   - **Generic tools:**
     - `search_operations(query?, tag?)` returns operation IDs, summaries and
       safety classes.
     - `describe_operation(operationId)` returns the input schema.
     - `call_operation(operationId, arguments)` calls the operation.
6. **Safety classes.** These live in a table in `mcpserver`, keyed by
   `operationId`. The OpenAPI registration calls stay as they are.
   - Every non-GET operation must be explicitly classified as `ReadOnly` (a
     read over POST), `Mutating`, `Destructive`, or `Excluded` (with a
     reason).
   - GET operations default to `ReadOnly` and may be marked `Excluded`.
   - A drift test fails when an operation is unclassified, or when the table
     names an operation that no longer exists.
   - Annotations follow from the class: `readOnlyHint`, `destructiveHint`,
     `idempotentHint` (GET only). `openWorldHint` is set on operations that
     fetch URLs or call AI providers.
7. **Excluded operations:**
   - login, logout;
   - credential issuance: `createOwnToken`, `changeOwnPassword`, `createUser`,
     `updateUser`. A minted token or password would be a credential usable
     outside MCP, so the model never gets one through MCP;
   - the SSE streams;
   - the render fragments (plugin block/display render, deferred shortcodes);
   - the entity picker.
8. **Results.**
   - JSON responses go into `structuredContent`, with a text copy for older
     clients.
   - An HTTP error becomes `isError` with the status code and message.
   - Long-running operations return their job ID, which the model polls with
     `get_job`.

## Confirmations

This applies to every `Destructive` operation, whether reached through a named
tool or through `call_operation`.

**Trust boundary.** The server cannot prove a human answered. MCP requires
clients to show elicitations to the user, and the server relies on that. What
the server does guarantee:

- a destructive operation never runs without a confirmation round trip;
- the confirmation covers exactly the operation and targets that were shown;
- it can be used once, and only by the caller it was issued to.

Credentials are excluded from the catalog (see above), so a model cannot step
outside MCP to avoid this.

1. **Preview.** It is built by dispatching reads **as the caller** through
   `h`, so it can only show what the caller can already see.
   - Default: ID arguments (`Id`, `ID`, `Ids`, `Winner`, `Losers`, `id`) are
     resolved through the list endpoint of the operation's entity family
     (resource, note, group, tag, and others) with an `Ids` filter. IDs that
     don't come back are reported as "not found or not visible", with no
     distinction between the two.
   - Lookups use each entity's single-item GET (resource, note, group, query,
     series and so on), capped at 25 lookups. Beyond the cap, the preview gives
     the count and the first 25 names. Entities with no single-item GET (tags,
     for example) are listed by ID only.
   - Mass edit over MCP must name its targets with explicit `ID` lists.
     `Filter`, `MRQLQuery` and `MRQLSnapshot` selection is refused, so the
     confirmed set is exactly the sealed IDs. Resolving a filter at confirm
     time and again at execution would let membership change in between
     (round-2 finding). The preview adds the `DryRun=true` result.
   - Reduction apply: the reduction is fetched, and the sealed `version`
     already pins its contents.
2. **Challenge.** The client must have sent a 2026-07-28 request with an
   elicitation capability that includes form mode. The server then returns
   `InputRequests{"confirm": ElicitParams{message: preview, schema: {confirm:
   boolean}}}` together with `requestState`.
   - `requestState` is AES-256-GCM sealed (encrypted and authenticated).
   - The key is `SHA-256("mcp-confirm-v1" || DeferredSigningKey ||
     bootNonce)`, where `bootNonce` is 32 random bytes generated at startup. A
     restart therefore invalidates every outstanding challenge, so a nonce set
     kept in memory is enough to make challenges single-use.
   - The sealed fields are: user ID (0 when auth is off), SHA-256 of the
     Authorization header, tool name, resolved `operationId`, SHA-256 of the
     canonical dispatched arguments, expiry (now + 5 min), and a random nonce.
3. **Retry.** All of these must hold, or the call is refused:
   - the state decrypts and has not expired;
   - the user and the Authorization hash match the current request;
   - the tool name and the resolved `operationId` match;
   - the argument hash matches;
   - the elicitation answered `accept` with `confirm: true`.

   The nonce is then consumed atomically (a mutex-guarded map, swept as entries
   expire) **before** dispatch. Two concurrent retries cannot both pass.
4. **Clients that cannot be asked:** older protocol versions, or no form
   elicitation. The server **fails closed**: the tool returns `isError` with
   the preview and the message "This action needs a confirmation this client
   cannot show". There is no opt-out. Automation that needs unconfirmed
   deletes uses the REST API or `mr`.
5. `destructiveHint: true` is always set, so clients that prompt on it also
   prompt the user. This is in addition to the server-side check.

## `mr mcp` (stdio bridge)

`mr mcp` is a local stdio MCP server built with the SDK. At startup it connects
to the remote `/mcp` as a 2026-07-28 client, using the CLI's saved
credentials.

- It mirrors `tools/list` and forwards each `tools/call`.
- The client-side MRTR middleware is disabled. When the remote server returns
  `input_required`, the bridge passes the same `InputRequests` and
  `requestState` downstream, and forwards the retry's `InputResponses` and
  `requestState` back to the server.
- A 2026-07-28 stdio client handles MRTR itself. For an older stdio client, the
  SDK's server-side MRTR middleware runs `Elicit` over the stateful stdio
  session; the SDK forbids server-initiated `Elicit` on 2026-07-28
  (`mcp/server.go:1613-1627`), so that path is only for older clients.
- Older stdio clients that support elicitation therefore *can* confirm through
  the bridge. If the downstream client has no elicitation capability, the
  bridge cannot fulfil the request and nothing runs.
- Headers, `_meta` and SSE/JSON framing are all handled by the SDK transports,
  not by hand.

## Tests (red first)

Go tests in `server/mcpserver` (unit) and `server/api_tests` (integration over
`httptest`, SDK client, SQLite and Postgres). Each protocol-sensitive case runs
on both 2026-07-28 and 2025-11-25.

- **Drift:**
  - every operation is classified;
  - the table has no stale IDs;
  - every named tool resolves;
  - every non-JSON operation is either link-only or a thumbnail.
- **Schema:**
  - `getNote` requires `id`;
  - a header param, a paginated list and a multipart upload all produce usable
    schemas;
  - a `$ref` body is resolved.
- **Admission:**
  - no token, a cookie-only request and an invalid token each get 401;
  - a guest can read (`list_resources`) and is refused a write (403 →
    `isError`);
  - each of these is refused with 403: a hostile `Origin`, `Origin: null`,
    `Sec-Fetch-Site: cross-site`, and a rebinding-style request with
    `Origin == Host`;
  - an allowlisted `Origin` passes;
  - `GET` and `DELETE` on `/mcp` return 405.
- **Scope:** a group-scoped principal sees only its subtree through `search`,
  `call_operation` and the preview. A mixed visible/invisible bulk delete
  shows only the visible names, and the invisible IDs are never named.
- **Dispatch:**
  - the result matches the direct REST response;
  - an oversized JSON response gives `isError`;
  - a binary operation returns a link;
  - a thumbnail is inlined;
  - a cancelled parent request cancels the handler.
- **Confirmations:**
  - an older client is refused, and nothing is deleted;
  - a new client without elicitation is refused;
  - accept runs the operation; decline and cancel do not;
  - each of these is refused: changed arguments, a different operation with
    the same arguments, a different tool name, an expired state, a replayed
    nonce, two simultaneous retries (exactly one runs), a state from before a
    restart (new boot nonce), another user, another token, and a tampered
    ciphertext;
  - a target set that changed between challenge and retry gets a new
    challenge;
  - the excluded credential operations are absent from `search_operations`
    and refused by `call_operation`.
- **Bridge (CLI E2E):** `mr mcp` lists tools, calls a read, and runs a delete
  through an elicitation, with both accept and decline.

## Out of scope

- OAuth / Client ID Metadata Documents: API tokens are enough for a personal
  server.
- `subscriptions/listen`, MCP prompts, MCP resources (other than
  `resource_link` results).

## Review log

Revision 1 → 2 (pi, gpt-6.1-sol:high):

1. **Confirmation could be reused for another operation** → the sealed state
   includes the tool name and the resolved `operationId`, and both are
   checked.
2. **Restart replay** → the key includes a per-boot nonce. Nonces are consumed
   before dispatch. The state is AES-GCM, as the SDK requires.
3. **Trust boundary** → stated explicitly. Credential issuance is excluded, and
   the per-token opt-out is dropped.
4. **Origin/Host with auth off** → an explicit refusal of browser requests,
   with an allowlist.
5. **Preview scope and unstable filters** → previews are built by dispatching
   as the caller (the fingerprint was later dropped, see revision 3).
6. **Guests blocked** → a `/mcp` `capRead` case; per-operation authorization
   is unchanged.
7. **Layering** → the package is `server/mcpserver`, with the dispatch target
   injected.
8. **Schema inputs** → derived from `GenerateSpec()`, not from `RouteInfo`
   fields.
9. **Cancellation and bridge** → the parent context plus
   `PropagateRequestCancellation`; the bridge is built from SDK transports.
10. **Unbounded capture** → a bounded recorder, links for binary operations,
    and capped thumbnails.

Revision 2 → 3 (pi, round 2): mass edit takes explicit IDs only, and the
fingerprint is dropped; the bridge passes MRTR through; registry gaps are
fixed and extra arguments pass through; previews use per-entity lookups; the
Bearer check runs before CSRF.
