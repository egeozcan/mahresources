# Template generation as a multi-Turn Generation run

Status: planned 2026-11-07 from a grilling session (questions Q1 to Q34). Terms are defined in `GLOSSARY.md` under "Template generation". The consent decision is `docs/adr/0009-generation-runs-approve-an-envelope.md`.

## Success criteria

1. Every template generation request (slot cluster, MetaSchema, whole template) is a Generation run: the model may call read-only Tools over up to `ai_template_max_turns` Turns (default 10, hard maximum 20), then submits a Draft. The Draft reaches the editors only through the existing Apply step.
2. The run works with all four providers (DeepSeek, OpenRouter, OpenAI, Anthropic) through native tool calling. A model that cannot call Tools is refused with a clear message.
3. Before a run, the Disclosure states the destination, the Seed messages, the Tools and what they can reach, the caps, and two opt-ins (previews run `[mrql]`; previews run plugin shortcodes), both off by default. The `groundingHash` covers all of that.
4. The browser shows each Turn as it happens, offers Finish now and Cancel, and afterwards shows the full transcript, each Tool result expandable to the exact text sent.
5. A run never reads beyond the account's subtree scope. A group-scoped principal gets the same refusals from every Tool that it gets from the matching HTTP route.
6. The classic one-shot path (draft, repair, fan-out) stays selectable through a runtime setting until an eval report compares the two. Removing it is the user's decision.
7. Go unit, Postgres, E2E (browser and CLI) suites pass. Docs describe the feature exactly as built.

## Decisions (from the grilling, condensed)

| # | Decision |
|---|---|
| Q1, Q17, Q18′ | The Disclosure approves an Envelope: everything the account can read, plus that run's opt-ins. Values are read live. ADR 0009. |
| Q2, Q31 | The harness replaces repair and fan-out. The classic path stays behind `ai_template_generation_mode` until the eval report; the user decides removal. |
| Q3 | Native tool calling through a provider-neutral conversation type, one adapter per wire format. |
| Q4, Q27 | Server-Sent Events on the generate request. Finish now forces a final submit Turn. Cancel aborts. Leaving the editor cancels. A new run in the same editor supersedes the old one. |
| Q5, Q11′, Q11″ | `preview` returns rendered HTML as text. `[mrql]` executes only with the MRQL opt-in, plugin shortcodes only with the plugin opt-in; otherwise each renders an inert placeholder. |
| Q6, Q15, Q23 | Turn cap 10 (max 20), run deadline 5 minutes, token budget 400k with cached prompt tokens counted at a quarter. One model per run, chosen by target through the existing per-kind settings. |
| Q7, Q24 | `submit` may be called several times with some fields. Each submit returns its lint result. The last submission of each field wins. The run ends on a submit marked done, or at the cap. |
| Q8 | Every Tool is read-only. No Tool writes through any app API. |
| Q9, Q19 | `list_related` walks up (owners), down (owned) and across (links). No hop cap; turns and budget bound the walk. |
| Q10 | Entity Tools return the sample allow-list projection, extended with the full Meta. |
| Q12, Q20 | Tools: `get_entity`, `list_related`, `get_template`, `list_carriers`, `list_revisions`, `get_revision`, `diff_revisions`, `lint`, `check_mrql`, `preview`, `get_partial`, `get_shortcode_docs`, `submit`. Revision Tools are gated on edit rights to that carrier. |
| Q13 | The reference docs stay in the cached shared prefix. Tool definitions go in the prefix too. |
| Q14 | `submit` accepts only the requested fields. `CustomCSS` stays append-only. `lint` and `preview` accept any field. |
| Q16, Q32 | The transcript is shown, not stored. Each result expands to the exact text sent. |
| Q22 | Results are byte-capped and say when they were cut. Results older than 3 Turns are replaced by a one-line stub when the conversation is resent. |
| Q25 | Up to 6 Tool calls per Turn, run in parallel; still one Turn. |
| Q26 | The Seed is today's whole per-request prompt tail. |
| Q28 | A model without tool support is refused at the dry run where detectable, and on the first Turn otherwise. No fallback to the classic path. |
| Q29 | One generation limiter slot per run. The provider semaphore is held only during a provider call. At most 2 concurrent runs per account. |
| Q30 | No `mr` CLI command. |
| Q33 | One shared progress-panel partial for the slot and whole-template hosts. A polite live region announces Turn starts and the end only. |

## Design

### 1. Provider conversation layer (`application_context/ai_conversation*.go`)

A new seam beside `TemplateDraftProvider`, which stays for MRQL and the classic path:

```go
type ConversationRequest struct {
    System     string            // the shared prefix plus the Seed system text
    Messages   []ConversationMessage
    Tools      []ToolSpec        // stable order; part of the cached prefix
    ToolChoice ToolChoice        // Auto, or Force(name)
    Model      string
    Thinking   bool
    MaxTokens  int
}
type ConversationMessage struct {
    Role       string            // user | assistant | tool
    Text       string
    ToolCalls  []ToolCall        // assistant only
    Results    []ToolResult      // tool only: one message carries every result of a Turn
    Replay     json.RawMessage   // provider-specific assistant content to send back verbatim (thinking blocks, reasoning_content)
}
type ToolSpec struct{ Name, Description string; Parameters json.RawMessage }
type ToolCall struct{ ID, Name string; Arguments json.RawMessage }
type ToolResult struct{ CallID, Text string; IsError bool }
type ConversationResponse struct {
    Text      string
    ToolCalls []ToolCall
    Finish    string             // stop | length | tool_calls
    Replay    json.RawMessage
    Usage     TemplateTokenUsage
}
type TemplateConversationProvider interface {
    Converse(ctx context.Context, req ConversationRequest) (ConversationResponse, error)
    Endpoint() string
    ProviderName() string
    // SupportsTools reports, as far as can be known without a paid call,
    // whether the model accepts tools. OpenRouter asks its model metadata;
    // the others answer true and rely on the first Turn's error.
    SupportsTools(ctx context.Context, model string) (bool, error)
}
```

Two adapters on the existing `generationProviderClient` transport (redirect blocking, 4 MiB cap, sanitized errors are kept):

- **OpenAI-compatible** (DeepSeek, OpenRouter, OpenAI): `tools`, `tool_choice`, `parallel_tool_calls`, assistant `tool_calls`, `role: tool` results with `tool_call_id`. No `response_format` on a conversation call. `max_completion_tokens` for OpenAI. DeepSeek `thinking` keeps its current meaning; any `reasoning_content` the provider requires back inside a tool loop is carried in `Replay`.
- **Anthropic Messages**: `tools`, `tool_choice`, `tool_use` and `tool_result` blocks, thinking blocks returned unchanged through `Replay`, `cache_control` breakpoints on the system prefix and the tools, and on the last message.

The exact field names, forced-choice rules (with and without thinking) and cache placement come from `scratchpad/provider-tool-calling.md`, the provider research done for this plan. They are summarized in an appendix of this file during phase 1, with links to the primary docs. The Anthropic client's `Authorization: Bearer` header and its `Anthropic-Version` value are checked against that research and fixed in phase 1 if wrong, as a separate commit, since they affect the classic path too.

A provider error, a refusal, or an unknown finish value is an error, as today. `length` is not an error: the loop handles it (section 3).

### 2. Tools (`application_context/template_run_tools.go`, `server/api_handlers/template_run_tools.go`)

The generator stays DB-free. `application_context` owns the Tool catalogue (names, descriptions, JSON Schema parameters, argument validation, byte caps, truncation notices, the `submit` contract), and calls a seam the handler builds per request:

```go
type TemplateToolExecutor interface {
    Execute(ctx context.Context, call TemplateToolInvocation) (TemplateToolOutput, error)
}
```

`server/api_handlers` implements it on the request's scoped context, so every read goes through the same `Find`/`Pluck`/`Count` paths and subtree callbacks as the HTTP routes. No Tool uses `Scan`. Each Tool:

| Tool | Arguments | Returns | Cap |
|---|---|---|---|
| `get_entity` | `type` (group, resource, note), `id` | the sample projection (`ProjectTemplateSample`) with full Meta, plus its Category / Resource Category / Note Type ID and name | 8 KiB |
| `list_related` | `type`, `id`, `direction` (up, down, across), `kind?`, `offset?` | entries `{type, id, name, carrier}` | 50 entries |
| `get_template` | `carrierType`, `id`, `fields?` | saved slots, MetaSchema, SectionConfig | 32 KiB |
| `list_carriers` | `carrierType`, `nameQuery?`, `offset?` | `{id, name, description}` | 50 entries |
| `list_revisions` | `carrierType?`, `id?` (default: the carrier being edited) | revision summaries | 50 entries |
| `get_revision` | as above plus `number`, `field?` | revision content | 32 KiB |
| `diff_revisions` | as above plus `from`, `to?` | slot-grouped diff | 32 KiB |
| `lint` | `field`, `content?` | lint issues (with `PartialExists`), or MetaSchema issues | 8 KiB |
| `check_mrql` | `query` | `CheckTemplateMRQL` verdict | 2 KiB |
| `preview` | `field`, `content?`, `css?`, `entityType?`, `entityId?` | rendered HTML text, plus CSS text | 16 KiB |
| `get_partial` | `name` | the partial's description and source | 32 KiB |
| `get_shortcode_docs` | `name` | the full doc entry for a built-in or plugin shortcode | 16 KiB |
| `submit` | `fields` (map), `explanation?`, `done?` | lint result per field | n/a |

Rules shared by all Tools:

- `content` omitted means the field's latest submission in this run, else the editor's current value from the Seed.
- Revision Tools on a carrier other than the one being edited check edit rights through the same predicate as the revision routes (`requireTemplateCarrierEdit`). A refusal is a Tool error result ("not allowed"), never a run failure.
- A missing or out-of-scope entity is a Tool error result with the same wording for both cases, so scope cannot be probed.
- Results are fenced with the existing `<<<BEGIN/END label>>>` markers and defanged, as samples are today.
- A result over its cap is cut at a UTF-8 boundary and ends with `[cut: N of M bytes shown]`.
- `preview` builds the meta context with `BuildMetaContextForEntity` like the preview route. The MRQL executor is the capped executor only when the MRQL opt-in is set; otherwise it is an executor returning a new `shortcodes.ErrQueryUnavailable`, rendered by `RenderMRQLShortcode` as an inert comment that carries the static check verdict. The plugin renderer is `previewPluginRenderer` only when the plugin opt-in is set; otherwise nil, which renders the existing "plugin unavailable" comment. `ListHeader`/`ListFooter` fields preview against the carrier, as carrier-mode preview does.
- `submit` rejects fields outside the request (`TemplateGenerationInput.BundleSlots`, the cluster, or MetaSchema) with an error result. A cluster submission must carry both halves across the run's submissions before the Draft can be applied, using the existing half-pair rule.

### 3. The run loop (`application_context/template_run.go`)

`GenerateTemplate` dispatches on the mode setting: classic, or `runTemplate`. `runTemplate`:

1. Rebuilds the Disclosure and checks `groundingHash`, as today.
2. Builds the first request: system = the shared prefix + harness instructions; messages = one user message holding the Seed (today's tail with the output-format line replaced by Tool instructions); tools = the catalogue in a fixed order.
3. Loops per Turn while `turn < maxTurns`:
   - Forces `submit` when this is the last Turn, when Finish now was requested, when the remaining deadline is under one Turn timeout, or when the remaining budget is under one Turn's `max_tokens` plus the last prompt size. Where a provider cannot force a named Tool (for example Anthropic with thinking enabled), the Turn uses `auto` plus a final instruction, and any non-submit call on that Turn is answered with an error result and ends the run.
   - Calls the provider under the existing per-call timeout and the process semaphore, released before Tools run.
   - Counts usage against the budget: completion + uncached prompt + a quarter of cached prompt.
   - On `length`: if the reply has complete Tool calls, runs them; otherwise appends "your reply was cut off; submit smaller parts" and continues. Two consecutive `length` replies end the run.
   - On Tool calls: runs up to 6 in parallel with a per-Tool timeout (10 s, 20 s for `preview`); a 7th and later get "too many calls in one Turn". Results go into one tool message in call order.
   - On plain text with no Tool call: once, appends "call submit to return your draft"; a second time ends the run.
   - Ends when a `submit` with `done: true` succeeds, or the loop runs out.
4. Builds the result from the last submission of each field, linted again with the same rules as `validateTemplateContent`, and the same half-pair and oversize checks. Nothing submitted is an error (`ErrTemplateGenerationProvider`, "the model returned no draft"), reported with the transcript.
5. Before each Turn, Tool results from Turns older than the last 3 are replaced by their stub in the messages sent. The transcript keeps the full text.

`TemplateGenerationResult` gains `Turns []TemplateRunTurn` (Tool name, arguments, result text, sizes, duration, error flag; usage per Turn) and `StopReason` (done, turn_cap, deadline, budget, finish_now, no_draft). The classic path leaves both empty.

Events: the loop takes an optional `TemplateRunSink` (`TurnStarted`, `ToolFinished`, `Submitted`, `Usage`). The SSE handler implements it. Tests use a recording sink.

Finish now: the handler registers the run in a process-wide `templateRunRegistry` keyed by a random run ID, and records the account. `POST /v1/{carrier}/generateTemplate/finish` with `{runId}` sets the run's flag when the account matches. The registry also enforces at most 2 live runs per account (key: user ID; when auth is disabled, the client IP).

### 4. Disclosure v3 (`template_generation_disclosure.go`)

- Continuation rule version 3, new text: Tool results may carry any data the account can read, limited to the Tools and opt-ins shown; they are listed in the transcript after the run.
- New disclosure fields, all hashed: `Tools` (name, description, parameter schema), `Caps` (max Turns, deadline, budget, per-Turn call limit, result caps), `OptIns` (`previewMRQL`, `previewPlugins`), `Envelope` (a fixed sentence per opt-in state plus the account label: username and scope group name, or "all data" for an unscoped account) and `ScopeIdentity` (user ID, role, scope group ID, superuser), which is hashed and not shown.
- The profile list has one entry, the run's kind, with `MaxCalls` = max Turns.
- The request DTO gains `previewMRQL` and `previewPlugins` booleans. Changing either in the dialog re-runs the dry run and shows the new hash's Disclosure.

### 5. HTTP and SSE (`server/api_handlers/template_generation_api_handlers.go`)

- The real request answers `text/event-stream` when the client sends `Accept: text/event-stream`, otherwise one JSON result at the end, which keeps API callers and tests simple.
- Events: `run` (run ID, caps), `turn` (number, forced), `tool` (name, arguments summary, bytes, error flag, duration), `submission` (field, error and warning counts), `usage` (budget used), `result` (the full `TemplateGenerationResult`, transcript included), `error` (message, and the transcript so far). A comment heartbeat every 15 s.
- Errors before the stream starts keep today's status codes. After it starts, they arrive as an `error` event.
- Request cancellation (client abort) cancels the run context, which cancels the in-flight provider call and Tool calls.
- OpenAPI entries for the event stream and the finish route; `go run ./cmd/openapi-gen` regenerated.

### 6. Frontend

- `templates/partials/form/templateRunProgress.tpl` plus `src/components/templateRun.js`: Turn list, Turn and budget meter, Finish now, Cancel (AbortController), collapse into the transcript at the end. Used by `templateGeneration.js` and `templateBundle.js`, which keep their Apply, snapshot and overwrite checks unchanged.
- The Disclosure dialog shows Tools, caps and the Envelope sentence, with the two opt-in checkboxes; toggling one re-requests the dry run.
- The SSE reader is `fetch` with a stream reader (the request is a POST with a CSRF token), not `EventSource`.
- A new run in the same editor aborts the previous fetch.
- Live region: "Turn N started" and the end state only.

### 7. Settings (`application_context/ai_generation.go`)

New runtime entries in the "ai" group: `ai_template_generation_mode` (`harness` or `classic`, default `harness`), `ai_template_max_turns` (default 10, range 1 to 20), `ai_template_run_deadline` (default 5m, range 30s to 15m), `ai_template_run_token_budget` (default 400000, range 20000 to 2000000). `pair` and `plan` overrides keep working for the classic path.

### 8. Eval (`internal/templateeval`)

The recorder records Turns and Tool calls; the scorer is unchanged (it scores the final Draft). A `compare` run of both modes on one model over the corpus produces the report the user decides removal on: valid-draft rate, lint error count, Turns used, tokens and cost per run. Without a provider key in the environment the run is skipped and reported as not run.

## Phases and lanes

Phase 0 (one commit, before any lane): the interfaces and types of sections 1 to 3, with stubs, so lanes compile against them.

| Lane | Scope | Depends on |
|---|---|---|
| A | Section 1: conversation types, both adapters, `SupportsTools`, Anthropic header check, httptest round-trip tests per provider including parallel calls, forced choice, `length`, thinking replay | Phase 0 |
| B | Section 2: Tool catalogue, argument validation, caps, the executor in `api_handlers`, `ErrQueryUnavailable` placeholder, scope tests (SQLite and Postgres) | Phase 0 |
| C | Sections 3, 4, 7: run loop with a scripted fake provider and fake executor, Disclosure v3, settings, registry | Phase 0 |
| D | Sections 5, 6: SSE, finish route, OpenAPI, frontend, E2E and a11y tests | B, C |
| E | Section 8 and docs: eval recorder, docs-site pages, `docs/architecture/template-generation-runs.md`, runtime settings docs | C (eval), D (docs) |

A, B and C run in parallel in this worktree with disjoint files, each staging explicit paths. Each lane is test-first: a failing test for each behaviour, then the code. Each lane's batch gets a pi review before the next phase starts.

## Tests (beyond each lane's unit tests)

- Scope: a group-scoped principal's run cannot read an entity, related entity, carrier or revision outside its subtree through any Tool (`server/api_tests`, SQLite and Postgres).
- Hash: the hash changes with each opt-in, the Turn cap, the Tool list and the account; it is stable across two dry runs with unchanged data.
- Placeholder: with opt-ins off, a previewed `[mrql]` executes no query (asserted through a counting executor) and a plugin shortcode reaches no plugin.
- Budget, deadline, cap, Finish now and the per-Turn call limit each force the final submit Turn and set `StopReason`.
- Stubbing: a result older than 3 Turns is a stub in the next request and complete in the transcript.
- E2E (stubbed provider through `page.route` with an event-stream body): progress panel, Finish now, Cancel, transcript expansion, opt-in re-disclosure, Apply unchanged, accessibility of the panel.

## Risks

- **Prompt size growth.** Each Turn resends the conversation. The stubbing rule and the budget bound it; the eval reports tokens per run.
- **Provider differences in forced choice and thinking.** Handled per adapter. Where forcing is impossible, the final Turn degrades as section 3 describes.
- **Injected entity text now reaches the model through Tools at the model's own request.** The boundary stays the explicit Apply step (decision 10 of the 2026-10-24 plan); the review panel's highlighting of code-running constructs is unchanged.
- **Plugin Lua during previews** can have side effects; this is why it is a separate opt-in, off by default.
