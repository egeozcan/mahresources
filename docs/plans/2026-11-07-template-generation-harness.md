# Template generation as a multi-Turn Generation run

Status: planned 2026-11-07 from a grilling session (questions Q1 to Q34), revised after pi review round 1 and the provider research in Appendix A. Terms are defined in `GLOSSARY.md` under "Template generation". The consent decision is `docs/adr/0009-generation-runs-approve-an-envelope.md`.

## Success criteria

1. In harness mode, every template generation request (slot cluster, MetaSchema, whole template) is a Generation run: the model may call read-only Tools over up to `ai_template_max_turns` Turns (default 10, hard maximum 20), then submits a Draft. The Draft reaches the editors only through the existing Apply step.
2. The run works with all four providers through native tool calling. A model that cannot call Tools is refused with a clear message.
3. Before a run, the Disclosure states the destination, the Seed messages, the Tools and what they can reach, the caps, and two opt-ins (previews run `[mrql]`; previews run plugin shortcodes), both off by default. The `groundingHash` covers all of that, and the mode.
4. The browser shows each Turn as it happens, offers Finish now and Cancel, and afterwards shows the full transcript, each message and Tool result expandable to the exact text sent.
5. A run never reads beyond the account's current access. Access is re-checked before every Turn; a revoked, demoted or re-scoped account ends the run with no Draft.
6. A model-written preview cannot exhaust memory or run past its deadline.
7. Classic mode (draft, repair, fan-out) is unchanged in behaviour and Disclosure, and stays selectable until an eval report compares the two. Removing it is the user's decision.
8. Go unit, Postgres, E2E (browser and CLI) suites pass. Docs describe the feature exactly as built.

## Decisions (from the grilling, condensed)

| # | Decision |
|---|---|
| Q1, Q17, Q18′ | The Disclosure approves an Envelope: everything the account can read, plus that run's opt-ins. Values are read live. ADR 0009. |
| Q2, Q31 | The harness replaces repair and fan-out. The classic path stays behind `ai_template_generation_mode` until the eval report; the user decides removal. |
| Q3 | Native tool calling through a provider-neutral conversation type. |
| Q4, Q27 | Server-Sent Events on the generate request. Finish now makes the next Turn the final one. Cancel aborts. Leaving the editor cancels. A new run in the same editor supersedes the old one. |
| Q5, Q11′, Q11″ | `preview` returns rendered HTML as text. `[mrql]` executes only with the MRQL opt-in, plugin shortcodes only with the plugin opt-in; otherwise each renders an inert placeholder. |
| Q6, Q15, Q23 | Turn cap 10 (max 20), run deadline 5 minutes, token budget 400k with cached prompt tokens counted at a quarter. One model per run, chosen by target through the existing per-kind settings. |
| Q7, Q24 | `submit` may be called several times with some fields. Each submit returns its lint result. The last submission of each field wins. |
| Q8 | Every Tool is read-only. No Tool writes through any app API. |
| Q9, Q19 | `list_related` walks up (owners), down (owned) and across (links). No hop cap; Turns and budget bound the walk. |
| Q10 | Entity Tools return the sample allow-list projection, extended with the full Meta. |
| Q12, Q20 | Tools: `get_entity`, `list_related`, `get_template`, `list_carriers`, `list_revisions`, `get_revision`, `diff_revisions`, `lint`, `check_mrql`, `preview`, `get_partial`, `get_shortcode_docs`, `submit`, plus `list_members` (added during planning, section 2). Revision Tools are gated on edit rights to that carrier. |
| Q13 | The reference docs stay in the cached shared prefix. Tool definitions are part of the prefix and never change during a run. |
| Q14 | `submit` accepts only the requested fields. `CustomCSS` in a whole-template run stays append-only. `lint` and `preview` accept any field. |
| Q16, Q32 | The transcript is shown, not stored. Every message and result expands to the exact text sent. |
| Q22 | Results are byte-capped and say when they were cut. Results older than 3 Turns are replaced by a one-line stub when the conversation is resent. |
| Q25 | Up to 6 Tool calls per Turn, run in parallel; still one Turn. |
| Q26 | The Seed carries today's grounding data. |
| Q28 | A model without tool support is refused at the dry run where detectable, and on the first Turn otherwise. No fallback to the classic path. |
| Q29 | One generation limiter slot per run. The provider semaphore is held only during a provider call. At most 2 concurrent runs per account. |
| Q30 | No `mr` CLI command. |
| Q33 | One shared progress-panel partial for the slot and whole-template hosts. A polite live region announces Turn starts and the end only. |

Decisions made while planning (recorded here so they are reviewable):

- **No forced Tool choice, anywhere.** The newest Anthropic models reject `tool_choice` `any` and `tool`; DeepSeek rejects `required` and named choices in thinking mode; OpenRouter support varies per endpoint (Appendix A). Narrowing the `tools` list on the last Turn breaks the prompt cache and, on Anthropic, invalidates earlier thinking blocks. So every Turn sends the same `tools` with `auto`, and the final Turn is enforced by the server (section 3), not the provider.
- **OpenAI runs use the Responses API** (`POST https://api.openai.com/v1/responses`). Chat Completions supports tools on current OpenAI reasoning models only with `reasoning_effort: "none"`, and not at all on the newest ones (Appendix A). Classic mode keeps Chat Completions.
- **Thinking stays constant for the whole run.** DeepSeek requires `reasoning_content` replay on every request with `tools`, and toggling thinking mid-loop is undocumented or invalidates the cache.

## Design

### 1. Provider conversation layer (`application_context/ai_conversation*.go`)

A new seam beside `TemplateDraftProvider`, which stays for MRQL and classic mode:

```go
type ConversationRequest struct {
    System    string                // shared prefix + harness instructions; byte-stable per run
    Messages  []ConversationMessage
    Tools     []ToolSpec            // fixed order, identical on every Turn of a run
    Model     string
    Thinking  bool
    MaxTokens int
}
type ConversationMessage struct {
    Role    string         // user | assistant | tool
    Text    string
    Calls   []ToolCall     // assistant only
    Results []ToolResult   // tool only: one message holds every result of a Turn, in call order
    Replay  json.RawMessage // the provider's own assistant payload, resent verbatim
}
type ToolSpec struct{ Name, Description string; Parameters json.RawMessage }
type ToolCall struct {
    ID, Name  string
    Arguments json.RawMessage
    // Incomplete marks a call cut off by the output limit (Finish "length"):
    // its arguments did not arrive whole. Malformed marks complete arguments
    // that are not a JSON object. Neither runs; each gets an error result,
    // so every call ID is answered and the conversation stays valid.
    Incomplete, Malformed bool
}
type ToolResult struct{ CallID, Text string; IsError bool }
type ConversationResponse struct {
    Text   string
    Calls  []ToolCall
    Finish string          // stop | length | tool_calls
    Replay json.RawMessage
    Usage  TemplateTokenUsage
}
type TemplateConversationProvider interface {
    Converse(ctx context.Context, req ConversationRequest) (ConversationResponse, error)
    Endpoint() string
    ProviderName() string
    Transport() TemplateTransport
    // SupportsTools answers without a paid call where the provider can tell
    // (OpenRouter model metadata); otherwise true, and the first Turn's error
    // says so. An error means "unknown" and does not refuse the run.
    SupportsTools(ctx context.Context, model string) (bool, error)
    // HistoryRewritable reports whether earlier messages may be edited
    // (stubbed) between Turns for this model. True only for DeepSeek, and for
    // OpenRouter models under the "deepseek/" prefix. False for Anthropic,
    // whose thinking signatures are bound to every preceding message, for
    // OpenAI Responses, whose encrypted reasoning items are not documented
    // either way, and for every other OpenRouter model, which may be a Claude
    // model replaying signed reasoning through reasoning_details.
    HistoryRewritable(model string) bool
}
// ConversationUsage is the provider's accounting for one Turn. Each field is
// nil when the provider did not report it, so a missing value is never read
// as zero. Classic mode keeps TemplateTokenUsage.
type ConversationUsage struct {
    Prompt, Completion, Reasoning, CacheHit, CacheMiss *int
}
```

`ConversationResponse.Usage` is a `ConversationUsage`. Missing `Prompt` falls back to the Turn's prompt estimate; missing `Completion` to the Turn's full `max_tokens`; missing cache fields count the whole prompt as uncached.

Three wire adapters on the existing `generationProviderClient` (redirect blocking, the 4 MiB response cap and sanitized errors are kept; `post` takes the endpoint per call so one client serves both OpenAI endpoints):

- **Chat Completions with tools** (DeepSeek, OpenRouter): `tools[].function`, `tool_choice: "auto"`, assistant `tool_calls`, one `role: tool` message per result with `tool_call_id`. No `response_format`. `Replay` is the whole assistant message object, which carries DeepSeek's `reasoning_content` and OpenRouter's `reasoning_details`. DeepSeek's `thinking` field keeps its current meaning. OpenRouter requests add `provider: {"require_parameters": true}` so they route only to endpoints that accept `tools`.
- **OpenAI Responses**: `instructions`, `input` items, `tools[]` with `type: "function"` and an explicit `strict: false` (Responses otherwise normalizes schemas to strict mode, which would make every optional argument required and break the omitted-`content` rule and the `submit.fields` map), `store: false`, `include: ["reasoning.encrypted_content"]`, `max_output_tokens`. `Replay` is the response's `output` array, resent as input items; results are `function_call_output` items. `status: "incomplete"` with reason `max_output_tokens` maps to `length`.
- **Anthropic Messages**: `tools[].input_schema`, `tool_choice: {"type": "auto"}`, `tool_use` blocks, one user message holding every `tool_result` block, a top-level `cache_control: {"type": "ephemeral"}` (automatic caching). `Replay` is the raw `content` array, including thinking blocks and their signatures. `stop_reason: tool_use` maps to `tool_calls`. Headers stay as they are (Bearer and `2026-10-21` are current; Appendix A).

A provider error, a refusal, an unknown finish value, or a structurally invalid reply (a tool call with no ID or name) is an error, as today. A call whose arguments are not a JSON object is returned with `Malformed`. On `length`, a call whose arguments did not arrive whole is returned with `Incomplete`. Neither is an adapter error. Missing usage fields are reported as missing (pointer fields), not as zero, so the loop can fall back to its own estimate (section 3).

`SupportsTools` for OpenRouter reads `GET https://openrouter.ai/api/v1/models?supported_parameters=tools` under the dry run's request context, with a 5 s timeout and a 10-minute process cache. DeepSeek, OpenAI Responses and Anthropic answer true.

### 2. Tools

The generator stays DB-free. `application_context/template_run_tools.go` owns the catalogue: names, descriptions, JSON Schema parameters, argument validation, the byte caps, the truncation notice and the `submit` contract. It calls a seam the handler builds per request:

```go
// TemplateRunEnv is what a harness run needs from the request. The handler
// builds it; TemplateGenerationInput carries it as Run (nil for classic).
type TemplateRunEnv struct {
    Executor TemplateToolExecutor
    Sink     TemplateRunSink        // nil: no progress events
    Finish   func() bool            // Finish now requested
    Account  TemplateRunAccount     // label shown, identity hashed
    OptIns   TemplateRunOptIns      // previewMRQL, previewPlugins
}
type TemplateToolExecutor interface {
    // CheckAccess re-resolves the original credential under ctx and returns
    // ErrTemplateRunAccessChanged when it is no longer valid or the account's
    // role or scope changed since the run started. It rebinds the scoped
    // context (and so the subtree allow-list) when access is unchanged.
    CheckAccess(ctx context.Context) error
    Execute(ctx context.Context, call TemplateToolInvocation) (TemplateToolOutput, error)
}
```

The credential check needs `resolvePrincipal` and `sameAccess`, which are private to package `server`. Phase 0 adds `api_handlers.TemplateRunAccessFactory` (`func(r *http.Request) func(ctx context.Context) (*application_context.MahresourcesContext, error)`). `server/routes.go` builds it from the same code `currentJobEventsContext` uses (`server/request_scope.go:22-107`) and passes it to `GetGenerateTemplateHandler`.

`server/api_handlers/template_run_tools.go` implements the executor:

- **Live access.** `CheckAccess` re-resolves the credential when auth is enabled, compares `sameAccess`, and rebinds the principal (and so the subtree allow-list). It also compares a fingerprint (sha256 of the sorted allowed group IDs) with the one taken when the run started, because `sameAccess` compares only user, role, superuser and scope root (`server/request_scope.go:112-127`) and so misses a group moved out of the subtree. Any change to either, a shrink or a growth, ends the run: earlier results already in the conversation may hold data the account can no longer read. A changed identity is never adopted. The loop calls it before each Turn's provider call, again after the provider returns and before any Tool runs, and again before a Draft or transcript is released on any exit path (section 3).
- **Cancellation reaches the DB.** A new method `WithParentContext(ctx) *MahresourcesContext` returns a copy whose `db` handle is rebased onto `ctx`, keeping the scope filter and acting user, as `WithRequest`'s rebase branch does (`application_context/context.go:1368-1390`). Credential resolution and the principal rebinding in `CheckAccess` run on such a copy under the context they are given. The rebinding uses a new exported `WithPrincipalContext(ctx, p) (*MahresourcesContext, error)`, implemented through the private `withPrincipalWithin` (`application_context/scoping.go:120-131`), not `WithPrincipal`'s `context.Background()`. The check before releasing a result runs under the request context with a 5 s timeout, not the run context, which may already have reached its deadline; client cancellation still cancels it. Each Tool call runs on a copy rebased onto its own context. Cancel, the run deadline and the per-Tool timeout therefore cancel every query.
- **Panics are contained.** Each Tool call runs under a `recover` that turns a panic into a sanitized error result ("the Tool failed") and logs the stack. A panic never reaches the run or the process.
- **Scope.** Entity reads use the same `Find`/`Pluck`/`Count` readers as the HTTP routes, never `Scan`. Carriers and partials are global tables readable by every role, as their GET routes are. Revisions on a carrier other than the one being edited check edit rights with `requireTemplateCarrierEdit` (`application_context/template_revision_context.go`).
- **Refusals are results, not failures.** A missing or out-of-scope entity is one error text for both cases, so scope cannot be probed. "Not allowed" for a revision read. Neither ends the run.
- **Fencing.** Every result is fenced with the existing `<<<BEGIN/END label>>>` markers and defanged, as samples are today.
- **Caps.** Each result has a byte cap. A result over its cap is cut at a UTF-8 boundary, before fencing, and ends with `[cut: N of M bytes shown]`. List Tools also bound the query (`LIMIT entries+1`) before serializing.

| Tool | Arguments | Returns | Cap |
|---|---|---|---|
| `get_entity` | `type` (group, resource, note), `id` | the sample projection (`ProjectTemplateSample`) with full Meta, and its carrier's ID and name | 8 KiB |
| `list_related` | `type`, `id`, `direction` (up, down, across), `kind?`, `offset?` | `{type, id, name, carrier}` entries | 50 entries, 16 KiB |
| `get_template` | `carrierType`, `id`, `fields?` | saved slots, MetaSchema, SectionConfig | 32 KiB |
| `list_carriers` | `carrierType`, `nameQuery?`, `offset?` | `{id, name, description}` entries | 50 entries, 16 KiB |
| `list_members` | `carrierType`, `id`, `offset?` | the carrier's member entities, newest first, as `{type, id, name}` | 50 entries, 16 KiB |
| `list_revisions` | `carrierType?`, `id?` (default: the carrier being edited) | revision summaries | 50 entries, 16 KiB |
| `get_revision` | as above plus `number`, `field?` | revision content | 32 KiB |
| `diff_revisions` | as above plus `from`, `to?` | slot-grouped diff | 32 KiB |
| `lint` | `field`, `content?` | lint issues (with `PartialExists`), or MetaSchema issues | 8 KiB |
| `check_mrql` | `query`, `params?`, `limit?`, `buckets?` | `CheckTemplateMRQL` verdict with those options | 4 KiB |
| `preview` | `field`, `content?`, `css?`, `entityType?`, `entityId?` | rendered HTML text, and CSS text | 16 KiB |
| `get_partial` | `name` | description and source | 32 KiB |
| `get_shortcode_docs` | `name` | the full doc entry of a built-in or plugin shortcode | 16 KiB |
| `submit` | `fields` (map), `explanation?`, `done?` | lint result per field | 8 KiB |

`content` omitted means the field's value in the Turn-start snapshot (section 3): its latest committed submission, else the editor value from the Seed.

`list_members` (added during planning) exists because `list_carriers` finds carriers but not their members, and the samples are only five of them.

**One validation contract.** `lint`, `submit` and the final Draft all use one harness validator: `validateTemplateContent`'s options plus a partial-existence check. The handler supplies it through a new `TemplateGenerationInput.PartialExists func(ctx context.Context, name string) bool` seam, built on `WithParentContext(ctx)` per call, so a blocked partial lookup is cancelled by the Tool timeout or the run deadline. Each validation wraps it in the `func(string) bool` the linter takes, closing over that validation's context. Classic mode keeps its current validator unchanged.

**Preview and lint work is bounded.**
- `preview` builds the meta context with `BuildMetaContextForEntity` like the preview route, and previews `ListHeader`/`ListFooter` fields against the carrier.
- Source limits: the `content` and `css` of `lint`, `preview` and `submit` are each at most `MaxTemplateGeneratedContentLength` bytes and 2,000 shortcode tokens; over either is an error result before parsing.
- Rendering runs with a new render budget on the request context (`shortcodes.WithRenderBudget(ctx, 256 KiB output, 512 KiB parsed source)`). Parsed source counts every partial and plugin output parsed during the render. `processWithDepth`, `[each]` and every other loop that appends output check the budget before appending, and check `ctx.Err()` on every iteration. The parser's token loop checks `ctx.Err()` too. Exhausting any of these ends the render with `shortcodes.ErrRenderBudgetExceeded`, which the Tool reports as an error result.
- Pages and the preview route set no budget, so they behave as today apart from the cancellation checks.
- Schema expansion is bounded. `[meta]` resolves the MetaSchema through `inlineRefsDeep` (`shortcodes/meta_handler.go:345-396`), which follows `$ref` with no cycle detection: a schema whose `$defs.node.child` refers to `node` passes `ValidateMetaSchema` and then overflows the stack, a fatal error no `recover` can catch (reproduced in review round 3). The fix tracks the refs on the current expansion path, stops at a repeated ref, caps expansion at 32 levels and 10,000 nodes, and checks `ctx.Err()`. This is an existing crash on every page that renders `[meta]`, fixed at the root.
- `[property]` refuses unexported struct fields (`shortcodes/property_handler.go:83-87` reaches them through `FieldByName`, and `formatFieldValue` then panics in `Interface()`; reproduced with `[property path="CreatedAt.wall"]`). This is an existing bug in the preview route and pages, fixed at the root.
- With the MRQL opt-in off, the executor returns a new `shortcodes.ErrQueryUnavailable`. `RenderMRQLShortcode` renders that as an inert comment carrying the static check verdict. This also applies to `[mrql]` reached through partials, conditionals, `[using]`, result templates and CSS, since all of them go through the one executor.
- With the plugin opt-in off, the plugin renderer is nil, which renders the existing "plugin unavailable" comment. With it on, it is `previewPluginRenderer`, including its scoped-principal check.

**Submit.**
- `submit` rejects fields outside the request with an error result: the cluster, MetaSchema, or `BundleSlots`.
- In a whole-template run, a `CustomCSS` submission is *additions*. The Draft's `CustomCSS` is the Seed's `CustomCSS`, unchanged, plus the latest additions, joined once, as `appendSharedCSS` does today. The additions are linted; the combined size gets the oversize check.
- Companion CSS fields (`CustomHeaderCSS` and so on) and a single-slot `CustomCSS` request are whole values, as today.
- The half-pair rule applies to the Draft as a whole: a cluster with one half submitted is reported, and the client refuses to apply it, as today.

### 3. The run loop (`application_context/template_run.go`)

**Mode.** `runtimeAIGeneration` resolves the mode with the rest of the settings snapshot at request start. `DiscloseTemplate` and `GenerateTemplate` both dispatch on that snapshot. Classic mode is today's code, untouched, and its Disclosure is unchanged except that the hash also covers the mode. A mode change between approval and the real request is a 409. `DiscloseTemplate` gains a `context.Context` parameter (for the OpenRouter lookup); every caller and fake is updated in Phase 0. A harness-mode request whose input has no `Run` is refused as a bad request, so a caller that did not build a run environment cannot start one by accident.

**Seed.** A dedicated builder, `buildTemplateRunSections`, reuses today's grounding sections:
- identity, section layout, sibling styles, MetaSchema, samples summary, partial and plugin references, current content and the request.

It leaves out classic orchestration:
- the output-format line, the planner instructions ("a separate call fills each section…", `template_generation_fanout.go:260`), the pair templates and their placeholders, and the repair sections.

It adds a harness-only **starting points** section: the edited carrier as `{carrierType, id, name}` (none on the create form), and each sample as `{type, id, name}`. The handler keeps the IDs it loaded the samples by. Classic projections stay unchanged.

It adds the harness instructions:
- the Tools, the caps, "submit early, in parts; your last submission of each field is kept", and the `CustomCSS` additions rule for whole-template runs.

Within harness mode, the system prompt, the Tool list and the Seed stay byte-stable across Turns of a run and across runs with unchanged data, so they are cached. The two modes lay their messages out differently and do not share a cache.

**Turns.** Each Turn:

1. `executor.CheckAccess`. On `ErrTemplateRunAccessChanged`, the run ends with an error and no Draft. Access is checked again after the provider returns, before any Tool of the Turn runs, and once more before a Draft, a recovered Draft or a transcript is released, on every exit path. A run whose access changed during its last provider call therefore runs no Tools and releases nothing.
2. Decide whether this is the **final Turn**. It is when any of these hold:
   - it is Turn `maxTurns`
   - Finish now was requested
   - the remaining deadline is under one Turn timeout plus the Tool timeout
   - the remaining budget is under one final reserve (below)

   The reason is recorded. A final Turn sends the same tools, with an instruction appended to the last message: "This is your final Turn. Call submit now with every field you want kept; other Tool calls will not run."
3. **Budget admission.** The budget is a stopping rule, not a hard ceiling, because no provider bills a prompt before receiving it. The Disclosure says so: "No new Turn starts when it could exceed the budget; a Turn's actual use can exceed its estimate."
   - The estimated prompt is the whole outgoing request (system, Tool schemas, every message and replay payload) in bytes divided by 2.5, counted uncached. The divisor is deliberately conservative.
   - `max_tokens = min(profile.MaxTokens, remaining − estimatedPrompt)`. If that is under the final reserve (`min(profile.MaxTokens, 4000)`), the Turn becomes final with what is left. If what is left is under 1000, the run stops with reason `budget` without calling.
   - After each Turn, the provider's usage replaces the estimate. If usage is missing, the estimate and the full `max_tokens` are charged.
   - The dry run refuses a request whose Seed estimate plus one final reserve exceeds the budget. The budget setting's minimum is 100k, above the shared prefix.
4. Call the provider. The provider semaphore wait is inside the call timeout, as today; the call timeout is `min(profile.Timeout, remaining deadline)`. The whole run runs under one context with the run deadline.
5. Count usage: completion + uncached prompt + a quarter of cached prompt.
6. Handle the reply:
   - **Tool calls.**
     - The first 6 run in parallel, each with a per-Tool timeout (10 s, 20 s for `preview`). The 7th and later get "too many calls in one Turn" and do not run.
     - Every call, `submit` included, reads the **Turn-start snapshot** of submissions. Submissions are validated in parallel and committed in call order after every call of the Turn has finished, so two submits of one field in a Turn resolve to the later call.
     - `done: true` takes effect at the end of the Turn.
     - Results go into one tool message, in call order.
     - On a final Turn, non-`submit` calls get "not run: final Turn" and only submissions are processed.
   - **`length`:** the complete calls run as above. Each `Incomplete` call gets the error result "cut off at the output limit; send it again in smaller parts", and each `Malformed` call gets "arguments were not a JSON object". With no complete call at all, a second consecutive such Turn ends the run (`length_repeated`). A `length` reply with no call at all gets the same message as a user message.
   - **Plain text, no tool call:** "call submit to return your draft" is appended once. A second time, or on a final Turn, ends the run (`no_tool_call`).
7. The run ends after a Turn in which a `done: true` submit committed (`done`), and after every final Turn whatever it returned (`turn_cap`, `finish_now`, `deadline` or `budget`).

**Stubbing.** When `HistoryRewritable(model)` is true (DeepSeek, and OpenRouter `deepseek/` models), Tool results from Turns older than the last 3 are replaced, before each request, by their stub ("preview of CustomHeader against Group #12, 14 KiB, superseded"). A stub is fixed once written, so the history changes once per result and the cache misses only from the oldest newly stubbed message. For every other provider and model, history is never rewritten: Anthropic binds each thinking signature to every preceding message, and an edited prefix fails with HTTP 400. Their runs are bounded by the budget alone. The transcript records the full text and, where it applies, the Turn from which it was stubbed.

**Result.** It is built from the last committed submission of each field, re-linted with `validateTemplateContent`, with the oversize, half-pair and `CustomCSS` merge rules. If nothing was submitted, the run is an error (`ErrTemplateGenerationProvider`, "the model returned no draft"). A provider error or timeout after a submission returns the Draft with a warning issue and reason `provider_error` or `deadline`. With none, it is an error, as today.

`TemplateGenerationResult` gains:
- `StopReason`: `done`, `turn_cap`, `finish_now`, `deadline`, `budget`, `length_repeated`, `no_tool_call` or `provider_error`.
- `Transcript`: ordered records `{turn, kind, text, calls, callId, isError, bytes, stubbedFrom, durationMs, usage}`.
  - `kind` is one of `seed`, `assistant`, `tool_result`, `instruction`.
  - `text` is the exact transmitted form (after cap and fence) of each message sent and each provider reply.
  - Provider `Replay` payloads are not shown. They are model output (reasoning and signatures) and carry no server data.

Classic mode leaves both empty. A run that fails after starting returns a `*TemplateRunError{Err, StopReason, Transcript}`, which wraps the usual sentinel error so the handler's status mapping is unchanged and carries the transcript so far. An access change returns a `TemplateRunError` with no transcript, and cancellation returns a plain context error.

**Events.** The loop takes an optional `TemplateRunSink` with `TurnStarted`, `ToolFinished`, `Submitted` and `Usage`. Calls from parallel Tools are concurrent, so the sink must be safe for concurrent use; the SSE sink only enqueues (section 5).

**Registry.** A process-wide `templateRunRegistry` maps a random run ID to its account (user ID; the client IP when auth is disabled), its Finish now flag and its cancel function.
- It admits at most 2 live runs per account.
- `POST /v1/{carrier}/generateTemplate/finish` with `{runId}` sets the flag when the account matches.
- A deferred removal on every exit path frees the run's entry.

### 4. Disclosure, harness mode (`template_generation_disclosure.go`)

- Continuation rule version 3, harness text: "Tool results may carry any data your account can read through the Tools and opt-ins shown. Every message sent is listed in the transcript after the run." Classic keeps version 2 and its text.
- New fields, all hashed:
  - `Mode`
  - `Tools`: name, description and parameter schema
  - `Caps`: max Turns, deadline, budget, per-Turn call limit, result caps
  - `OptIns`: `previewMRQL` and `previewPlugins`
  - `Envelope`: a fixed sentence per opt-in state, plus the account label (username and scope group name, or "everything" for an unscoped account)
  - `ScopeIdentity`: user ID, role, scope group ID and superuser. It is hashed and not shown.
- One profile, the run's kind, with `MaxCalls` = max Turns. `Destination.Endpoint` is the harness endpoint, which for OpenAI is the Responses URL.
- `Transport.ResponseFormat` is `tools`.
- The request DTO gains `previewMRQL` and `previewPlugins`.

### 5. HTTP and SSE (`server/api_handlers/template_generation_api_handlers.go`)

- The real request answers `text/event-stream` when the client sends `Accept: text/event-stream`, and one JSON result at the end otherwise.
- Errors found before the stream starts keep today's status codes. After the first byte, they arrive as an `error` event.
- **Events:**
  - `run`: run ID and caps
  - `turn`: number, and final reason if any
  - `tool`: name, argument summary, bytes, error flag, duration
  - `submission`: field, error and warning counts
  - `usage`: budget used
  - `result`: the full `TemplateGenerationResult`
  - `error`: message, plus the transcript so far unless the access changed
- **One writer.** A single goroutine owns every write and flush, reading a bounded channel (64 events) and a 15 s heartbeat ticker, as `job_event_handlers.go:180-296` does. Progress events are droppable: the sink's enqueue gives up when the run context ends or the channel is full. Before every frame and every flush (heartbeats and draining included), the writer sets a 10 s write deadline through `http.ResponseController`, because the server's `WriteTimeout` is 45 minutes (`server/server.go:97-102`). A write or flush error, a timeout included, cancels the run and stops the writer.
- **Terminal delivery is separate.** The run's result or error is not sent through the progress channel. When the generator returns, the handler stops accepting progress events, drains what is queued, and writes exactly one `result` or `error` event under the request's own lifetime, with the same per-frame write deadline. The `error` event takes its transcript from `TemplateRunError`, never from progress events. A Draft recovered after the run deadline is therefore delivered while the client is still connected.
- **Cancel.** A client abort cancels the request context, which is the parent of the run context, so the in-flight provider call and every Tool's DB read are cancelled (section 2).
- OpenAPI entries for the event stream and the finish route; `go run ./cmd/openapi-gen` regenerated.

### 6. Frontend

- **Approval tuple.** `generationDisclosure.js`'s review resolves to an immutable `{groundingHash, previewMRQL, previewPlugins}` or null. It no longer resolves a boolean.
  - Toggling an opt-in in the dialog re-requests the dry run with the frozen request body. Send is disabled while that is in flight, and after it fails.
  - A response older than the latest toggle is ignored.
  - Both hosts (`templateGeneration.js`, `templateBundle.js`) send the frozen body plus that tuple.
- **Progress panel.** `templates/partials/form/templateRunProgress.tpl` plus `src/components/templateRun.js` show:
  - the Turn list, and a Turn and budget meter
  - Finish now (POSTs the finish route) and Cancel (AbortController)
  - at the end, a collapse into the transcript, rendered as text only, never as HTML

  The hosts keep their Apply, snapshot and overwrite checks unchanged.
- The SSE reader is `fetch` with a stream reader, not `EventSource`: the request is a POST with a CSRF token.
- **Teardown.** A `stopGeneration()` helper aborts the run and any re-disclosure, and invalidates late callbacks. It is called from the real teardown paths:
  - `codeEditor.destroy()`, which overrides methods spread in from `templateGeneration` (`codeEditor.js:24-25,334-340`)
  - the cluster host
  - `templateBundle`'s lifecycle (`templateBundle.js:181-189`)

  A new run in the same editor calls it first.
- **Live region:** "Turn N started" and the end state only.

### 7. Settings (`application_context/ai_generation.go`, `src/components/aiSettings.js`, `templates/partials/aiSettings.tpl`)

New runtime entries in the "ai" group, each with a control in the AI settings panel. The generic settings page excludes the group (`templates/adminSettings.tpl:17-20`).

| Entry | Default | Range |
|---|---|---|
| `ai_template_generation_mode` | `harness` | `harness` or `classic` |
| `ai_template_max_turns` | 10 | 1 to 20 |
| `ai_template_run_deadline` | 5m | 30s to 15m |
| `ai_template_run_token_budget` | 400000 | 100000 to 2000000 |

"Reset feature" for templates resets them too, as it resets every `ai_template_` key today; the panel says so. `pair` and `plan` overrides keep working for classic mode.

### 8. Eval (`internal/templateeval`, `server/api_tests/template_generation_eval_test.go`)

- **Recorder.** It records at the provider-neutral seams, not the wire: `ConversationRequest`/`ConversationResponse` pairs (Turns, Tool calls, usage, stop reason) in harness mode, and `TemplateDraftRequest`/`TemplateDraftResponse` pairs in classic mode, keeping the classic report fields. Today's recorder decodes DeepSeek-shaped wire bytes (`internal/templateeval/recorder.go:91-166`) and loses evidence for OpenAI, OpenRouter and Anthropic in either mode. Normal-suite fixtures check captured content, finish reasons, and token and cache totals for every adapter in both modes.
- **Scorer.** `ScorerVersion` is bumped and the scorer becomes mode-aware:
  - Draft quality checks score the final Draft.
  - A `length` reply on an intermediate Turn followed by a complete submission is "recovered", not truncated.
  - Truncation of the retained Draft is still a failure.
  - Failure diagnostics read tool submissions in harness mode and classic JSON in classic mode.
- **Runner.** The live runner constructs both generators from one provider configuration. Each report records mode, model, settings, Turns used, tokens per run and stop reasons.
  - Cost is reported as "not computed": the repo has no pricing source.
  - Without a provider key the live run is skipped with a message, not failed.
  - The fake-transport wiring tests run in the normal suite.
- **Comparison.** The two-mode report is produced by running both modes and then the existing `Compare`.

## Phases and lanes

Phase 0 (one commit, before any lane) settles every shared interface, with stubs that compile:
- the section 1 types, `ConversationUsage` and `TemplateConversationProvider`
- `TemplateRunEnv`, `TemplateToolExecutor`, `TemplateToolInvocation`/`Output`, `TemplateRunSink`, the transcript record, stop reasons, `ErrTemplateRunAccessChanged`, `TemplateRunError`
- `MahresourcesContext.WithParentContext` and `WithPrincipalContext`, implemented, with the real server-built access factory compiled against them
- the Tool catalogue's specs (names, descriptions, parameter schemas), which lanes B and C both read
- the new `TemplateGenerationInput` fields (`Run`, `PartialExists`, starting points) and result fields
- the `DiscloseTemplate(ctx, …)` signature change with every caller and fake updated
- `api_handlers.TemplateRunAccessFactory` and its parameter on `GetGenerateTemplateHandler`, wired in `server/routes.go` with a stub
- Appendix A as the adapter reference

| Lane | Scope | Depends on |
|---|---|---|
| A | Section 1: three adapters, `SupportsTools`, `HistoryRewritable`, httptest round trips per provider (parallel calls, replay of thinking and reasoning payloads, `length` with incomplete and mixed calls, malformed calls, missing usage, cache fields) | Phase 0 |
| B | Section 2: argument validation, caps, the executor (live access through the factory, DB rebasing, panic containment), source limits, render budget and cancellation checks in `shortcodes`, `ErrQueryUnavailable`, the `[property]` fix | Phase 0 |
| C | Sections 3, 4, 7 (backend): loop with a scripted fake provider and fake executor, Seed builder, Disclosure v3 and mode dispatch, settings, registry | Phase 0 |
| D | Sections 5, 6, 7 (UI): SSE with terminal delivery, finish route, OpenAPI, frontend, settings panel, E2E and a11y tests | B, C |
| E1 | Section 8 scorer and report schema; docs-site pages, `docs/architecture/template-generation-runs.md`, runtime settings docs | C (scorer), D (docs) |
| E2 | Section 8 live runner wiring through the real handler, and the two-mode report if a key is available | A, B, D |

A, B and C run in parallel in this worktree with disjoint files, each staging explicit paths. Each lane is test-first: a failing test for each behaviour, then the code. Each lane's batch gets a pi review before the next phase starts.

## Tests (beyond each lane's unit tests)

- **Entity scope** (`server/api_tests`, SQLite and Postgres): through the real router, a run by a group-scoped principal cannot read an entity or related entity outside its subtree through `get_entity`, `list_related`, `list_members` or `preview`. The run uses the synthetic scoped editor setup of `template_generation_scope_test.go`, since ordinary scoped roles cannot reach the route.
- **Live access:** with the scripted provider paused between Turns, and separately with it blocked inside the final Turn's call, the run ends with no Draft, no transcript and no Tool executed after the change, when any of these happens:
  - the account is disabled
  - the account is demoted
  - the account's scope group changes
  - a child group is moved out of the subtree; that child's entities become unreadable on the next Turn
- **Carriers and revisions:** every role that can start a run reads every carrier through `get_template` and `list_carriers`. Revision Tools on another carrier follow `requireTemplateCarrierEdit`; for example, an editor's NoteType run cannot read a Category's revisions.
- **Generation roles:** the real router refuses the run route to roles without the carrier's edit capability, as today.
- **Hash:** the hash changes with the mode, each opt-in, the Turn cap, the Tool list and the account. It is stable across two dry runs with unchanged data.
- **Placeholder:** with the opt-ins off, a previewed `[mrql]` executes no query, asserted through a counting executor. That holds whether the `[mrql]` is direct, inside a partial, a conditional, `[using]`, an `[mrql]` result template or CSS. A plugin shortcode reaches no plugin.
- **Render budget:** five nested `[each]` over a 100-element array returns `ErrRenderBudgetExceeded` within the bound, in bounded memory, with both opt-ins off. A cancelled context stops a long `[each]` and a long parse. An adversarial run of opening and closing tags hits the token limit before parsing.
- **Panics:** `[property path="CreatedAt.wall"]` renders nothing through the preview route and pages, and a deliberately panicking Tool returns an error result through the parallel executor.
- **Starting points:** a run with automatically chosen samples and no IDs in the request text can `get_entity` each sample by the IDs in the Seed.
- **One validator:** a missing `[partial]` gives the same issue from `lint`, from `submit` and in the final Draft.
- **Adapters to loop:** a `length` reply with only an incomplete call, and one with a complete call followed by an incomplete one; a missing usage block charges the estimate.
- **Anthropic history:** a run long enough to pass 3 Turns sends every earlier message byte-identical (no stubbing), natively and through OpenRouter with a non-DeepSeek model and `reasoning_details`.
- **Schema expansion:** a `[meta]` preview over a self-referencing `$ref` schema, and over a large acyclic one, returns within the bounds with both opt-ins off; the same schema on a group page no longer crashes the process.
- **Access lifetimes:** a run with auth enabled that recovers a Draft after its deadline passes the final access check under the request context; with the account revoked just before that check, nothing is released.
- **Slow client:** a client that stops reading makes the writer time out within the write deadline, which cancels the run and frees the registry entry.
- **Failure transcript:** a provider failure after Tool Turns but before any submission delivers the transcript in the `error` event with progress events deliberately dropped.
- **Partial lookups:** a blocked partial lookup during `lint`, `submit` and final validation is cancelled by the Tool timeout and the run deadline.
- **Responses schemas:** wire fixtures with omitted `content`, omitted revision carrier arguments and a partial `submit.fields` map carry `strict: false`.
- **Terminal delivery:** over a real stream, a submission followed by run-deadline expiry delivers the recovered Draft as the `result` event.
- **Dry-run lookup:** a stalled OpenRouter metadata lookup times out to "unknown", a cancelled dry run cancels it, and a second dry run uses the cache.
- **Loop:**
  - Each stop reason, including after earlier successful submissions.
  - Finish now well before the cap.
  - A final Turn with `done: false`.
  - Budget admission (a Turn's `max_tokens` clamped; refusal at the dry run).
  - Same-Turn submits completing in reverse order.
  - A 7th call in one Turn.
  - Stubbing: a result older than 3 Turns is a stub in the next request and complete in the transcript.
- **CSS:** repeated `CustomCSS` additions, a partial bundle, and byte-for-byte preservation of the Seed's `CustomCSS`.
- **SSE integration** (real HTTP, chunked): with a blocking fake provider, the first `turn` event arrives before the provider is released. Finish now makes the next Turn final. A client disconnect cancels the provider call and a waiting Tool read, and frees the registry entry. Simultaneous Tool completions and a heartbeat interleave without corrupting frames (race detector on).
- **Frontend unit:** the approval tuple, with re-disclosure responses arriving in reversed order; the exact final POST body.
- **E2E** (stubbed through `page.route` with event-stream bodies): the progress panel, Finish now and Cancel requests, transcript expansion, opt-in re-disclosure, Apply unchanged, editor teardown aborting the request, and accessibility of the panel and dialog.
- **Classic mode:** the existing generation tests pass unchanged with the mode set to classic.

## Risks

- **Prompt size growth.** Each Turn resends the conversation. Stubbing and the budget bound it; the eval reports tokens per run.
- **The final Turn is a soft request.** No provider can be forced portably. Submissions from earlier Turns are kept, and the instruction asks the model to submit early.
- **Injected entity text reaches the model through Tools.** The boundary stays the explicit Apply step (decision 10 of the 2026-10-24 plan). The review panel's highlighting of code-running constructs is unchanged.
- **Plugin Lua during previews** can have side effects. That is why it is a separate opt-in, off by default.
- **Anthropic keys not scoped to a workspace** need an `anthropic-workspace-id` header that the client does not send. This affects classic mode too. It is a follow-up (a setting), not part of this plan.

## Appendix A: provider facts (researched 2026-11-07 from primary docs; no live calls)

| | DeepSeek | OpenRouter | OpenAI (Responses) | Anthropic |
|---|---|---|---|---|
| Endpoint | `api.deepseek.com/chat/completions` | `openrouter.ai/api/v1/chat/completions` | `api.openai.com/v1/responses` | `api.anthropic.com/v1/messages` |
| Tool spec | `tools[].function{name,description,parameters}` | same | `tools[]{type:function,name,description,parameters}` | `tools[]{name,description,input_schema}` |
| Choice used | `auto` | `auto` | `auto` | `{type:auto}` |
| Forced choice | rejected with thinking on | per endpoint (`supports_tool_choice`) | supported | `any`/`tool` rejected on Opus 5.5, Sonnet 5.5, Fable 5.1, Mythos 5.1 |
| Replay | whole assistant message incl. `reasoning_content` (400 if omitted while `tools` present) | whole assistant message incl. `reasoning_details` | `output` items incl. encrypted reasoning (`store:false`) | raw `content` incl. thinking blocks with signatures, unmodified |
| Results | one `role:tool` message per call | same | `function_call_output` items | one user message of `tool_result` blocks |
| Tool stop | `finish_reason: tool_calls` | same | `function_call` items in `output` | `stop_reason: tool_use` |
| Caching | automatic prefix; `prompt_cache_hit/miss_tokens` | passthrough; `prompt_tokens_details.cached_tokens` | automatic; `input_tokens_details.cached_tokens` | top-level `cache_control` (automatic); `cache_read/creation_input_tokens` |
| Tool support check | assume yes | `GET /api/v1/models?supported_parameters=tools` | assume yes | assume yes |

Sources:
- https://api-docs.deepseek.com/guides/thinking_mode
- https://api-docs.deepseek.com/api/create-chat-completion
- https://openrouter.ai/docs/guides/best-practices/reasoning-tokens
- https://openrouter.ai/api/v1/models
- https://developers.openai.com/api/docs/guides/migrate-to-responses
- https://developers.openai.com/api/docs/guides/reasoning
- https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use ("Forcing tool use")
- https://platform.claude.com/docs/en/build-with-claude/thinking
- https://platform.claude.com/docs/en/build-with-claude/prompt-caching
- https://platform.claude.com/docs/en/api/overview (headers)

Lane A re-verifies each row against these pages before writing its fixtures.

Not verified: DeepSeek parallel-call suppression; OpenRouter `reasoning_details` round trip for DeepSeek models; exact cache-write price multipliers.
