# Template generation runs

Template generation has two modes, chosen by the `ai_template_generation_mode`
setting. **Classic** mode makes one provider call per section (plus a planning
call, a repair call and fan-out for whole templates) and is described in
`ai-generation.md`. **Harness** mode, the default, runs the request as a
**Generation run**: a loop of **Turns** in which the model reads the library
through read-only **Tools** and submits a **Draft**. This file describes the
harness mode as built. The terms (Generation run, Turn, Tool, Disclosure,
Envelope, Seed, Related entity, Draft) are defined in `GLOSSARY.md`; the
decision to approve an Envelope instead of every message is
`docs/adr/0009-generation-runs-approve-an-envelope.md`.

## Where the code is

| Concern | Code |
| --- | --- |
| Mode and limits, one settings snapshot per request | `application_context/ai_generation.go`: `templateGenerator`, `addAIHarnessSettingSpecs` |
| Loop, stop reasons, budget, stubs | `application_context/template_run.go`: `runHarness`, `loop`, `admit`, `conclude` |
| Seed and instructions | `application_context/template_run_seed.go`: `buildTemplateRunSections` |
| Tool catalogue | `application_context/template_run_tools.go`: `templateRunToolSpecs` |
| Tool argument checks, caps, fencing | `application_context/template_run_tools_exec.go` |
| Tool execution against the database | `server/api_handlers/template_run_tools.go`: `templateToolExecutor` |
| Submissions and the Draft | `application_context/template_run_submit.go`; `template_run.go`: `buildResult` |
| Disclosure | `application_context/template_generation_disclosure.go`: `newHarnessDisclosure`, `harnessEnvelope` |
| Provider seam and three wire adapters | `application_context/ai_conversation*.go` |
| Run registry | `application_context/template_run_registry.go` |
| Handler, delivery order, finish route | `server/api_handlers/template_run_handlers.go` |
| Event stream | `server/api_handlers/template_run_stream.go` |
| Access re-check | `server/template_run_access.go`: `templateRunAccessFactory` |
| Privacy mode | `application_context/template_privacy*.go`: the masker, the guard, `Entity` and `QueryResult`; `server/api_handlers/template_run_tools.go` masks Tool results and previews |
| Browser | `src/components/templateRun.js`, `templates/partials/form/templateRunProgress.tpl` |

## One request, start to finish

1. **Dry run.** The editor posts the request with `dryRun`. `GetGenerateTemplateHandler`
   calls `PrepareTemplateGeneration` once, so the mode, the limits and the
   provider settings of the whole request come from one snapshot
   (`runtimeAIGeneration.templateGenerator`). A group-limited account (`IsScoped()`
   or `RequiresScope()`) is refused with 403 before any provider call, a dry run
   included. The reason is that a run reads live data over many Turns and the
   Envelope would have to be confined to the subtree.
2. **Disclosure.** `discloseHarness` validates the request, assembles the Seed,
   refuses a model known not to support Tools (`SupportsTools`, only where it can
   be answered without a paid call), refuses a prompt whose Seed plus the final
   Turn's reserve exceeds the token budget, and returns the Disclosure with its
   `groundingHash`. No provider call is made.
3. **Approval.** The user reads the Disclosure, may tick the two opt-ins (each
   tick re-runs the dry run, `generationDisclosure.js`: `setOptIn`) and presses
   Send.
4. **Run.** The real request repeats the fields plus the hash. `generateHarness`
   rebuilds the Disclosure and answers 409 (`ErrTemplateGenerationGroundingChanged`)
   when the hash differs, 400 when it is missing. Then `runHarness` starts the
   loop under a context with the run deadline.

## Seed

The Seed is the system prompt and the first user message
(`buildTemplateRunSections`). It carries the instructions ("How this run works":
Turn cap, deadline, token budget, calls per Turn, the Tool list and the submit
contract), the carrier runtime, the Category identity, the detail-page layout,
the Meta schema, bounded sample values, the starting points (the edited carrier
and the loaded samples by type, ID and name, so a Tool can open any of them),
the user's request, the task (requested fields and whether each must be
submitted), sibling styles, the current content and the partials already
included. Data parts are fenced between `<<<BEGIN label>>>` and `<<<END label>>>`
lines (`referenceBlock`). The Seed is a start, not a limit: any Tool may read
outside it, inside the Envelope.

The system prompt, the Tool list and the Seed are byte-stable across the Turns of
a run, so the prefix is cache-eligible (a provider may or may not cache it).
History is append-only: a user instruction, an assistant reply, and a tool
message holding every result of the Turn in call order. The one exception is
stubbing of old Tool results for providers that allow it (see "Replay and
stubs").

## Tools

`templateRunToolSpecs` fixes the catalogue and its order; it never changes during
a run. Fourteen Tools, all read-only: `get_entity`, `list_related`,
`get_template`, `list_carriers`, `list_members`, `list_revisions`, `get_revision`,
`diff_revisions`, `lint`, `check_mrql`, `preview`, `get_partial`,
`get_shortcode_docs` and `submit`.

- Per-call result caps are constants in `template_run_tools.go` (`ToolCap*Bytes`);
  list Tools return at most `ToolListEntryCap` (50) entries per page. A result
  over its cap is cut with a notice. The caps are part of the Disclosure
  (`harnessResultCaps`).
- A Turn runs at most `TemplateRunMaxCallsPerTurn` (6) calls. Calls beyond that get
  an error result. The calls of one Turn run in parallel (`handleReply`) and their
  results are returned together, in call order.
- A successful Tool result body is fenced and untrusted. A failure is plain text,
  not fenced, except submit validation errors (below): a Tool that fails, times out or panics yields a fixed text
  (`runTool`), and a refusal is `ToolErrorResult`, which is capped and has fence
  markers defanged because it can echo a model-chosen argument. Argument errors
  of `submit` are a third form: `submitErrorText` clips model-chosen names, caps the
  text and fences it, unlike the plain-text execution failures and `ToolErrorResult`
  refusals of the other Tools. No database error
  string is passed on. A plugin shortcode's error can still reach the model inside
  a `preview` result, because the shortcode processor puts an error marker in the
  rendered HTML (`shortcodes/processor.go`) and the preview returns that HTML.
- A Tool has `defaultTemplateRunToolTimeout` (10 s); `preview` has
  `defaultTemplateRunPreviewTimout` (20 s). Previews and diffs are limited to 2 and
  1 at a time inside a run (`templateRunPreviewConcurrency`,
  `templateRunDiffConcurrency`) and render under an output budget of 256 KiB and a
  parsed-source budget of 512 KiB (`templateRunRenderOutputBytes`,
  `templateRunRenderParsedBytes`, set with `shortcodes.WithRenderBudget`), each
  applied separately to the HTML render and to the CSS render, since every
  `ProcessChecked` call starts its own meter. The
  preview renders with `shortcodes.ProcessChecked`, so a render past either budget,
  or a `matches` pattern past the regular expression allowance, is an error result
  rather than a cut-off render. No other render sets a budget: ordinary pages and
  the editor's live preview keep rendering, and there a pattern past the allowance
  only makes that `matches` comparison false (`renderMeter.regexp`); with
  `combine="any"` another comparison can still show the block.
- Lint, inside and outside a run, reads at most `shortcodes.MaxLintSourceBytes`
  (1 MiB); a larger source gets one "too large to check" error. Lint, rendering
  and MetaSchema compilation share `shortcodes.RegexAllowance`: per pattern
  `MaxRegexpSource` (32 KiB), `MaxRegexpProgramSize` (20,000 instructions) and
  `MaxRegexpRunes` (100,000), and per operation 200,000 instructions, 2,000,000
  runes and 4 MiB of pattern text. A MetaSchema past it does not compile, so it is
  refused on save and in a generated draft (`meterRegexpEngine`).
- `diff_revisions` uses `DiffTemplateRevisionsContext`, which diffs under
  `models.MaxBoundedDiffCells` (1,000,000 cells per field; a larger field takes
  the coarse delete-then-insert form) and stops with the run's context.
- `ValidateTemplateRunContent` passes the run's context to lint
  (`LintOptions.Context`), MetaSchema lint and `ValidateMetaSchemaContext`, so a
  cancelled run gets an error issue, never a clean verdict on content it did not
  finish checking.
- `lint`, `preview` and `submit` refuse a source over
  `MaxTemplateGeneratedContentLength` (20000 bytes) or with more than
  `MaxTemplateToolShortcodeTokens` (2000) `[` tokens before parsing
  (`CheckTemplateToolSource`).
- `lint`, `submit` and the final Draft share one validator,
  `ValidateTemplateRunField` over `ValidateTemplateRunContent`, so one input gives
  one verdict.
- With `content` omitted, `lint` and `preview` use the field's latest committed
  submission as of the start of the Turn, and the editor's value when nothing was
  submitted yet (`contentFor`). For a MetaSchema run the editor's value is the
  schema currently in the editor (`harnessPreparation.editorValues`).
- `list_revisions`, `get_revision` and `diff_revisions` need edit rights to the
  carrier, as the revision routes do.

### Opt-ins

`preview` renders the model's own template against a real entity. Two things in
that render read beyond the entities the other Tools reach, so each is off unless
the user ticks it for that run (`TemplateRunOptIns`, request fields `previewMRQL`
and `previewPlugins`):

- **`previewMRQL` off:** `[mrql]` renders an inert placeholder
  (`UnavailableQueryExecutor`). **On:** queries run through
  `BuildModelPreviewQueryExecutor`, capped to `previewMRQLLimitCap` (5) rows and
  the MRQL preview limits.
- **`previewPlugins` off:** plugin shortcodes render a placeholder. **On:** they
  run read-only (`plugin_system.WithModelPreview`). The plugin's own code and
  resource use apply; the preview limits do not apply inside it.

Both flags are in the Disclosure and its hash (`TemplateDisclosedOptIns`), and
classic mode ignores them. Plugin previews are refused while Privacy mode is on
(see "Privacy mode" below), so ticking `previewPlugins` needs `privacyOff`.

## Envelope and Disclosure

The Disclosure (`TemplateDisclosure`) in harness mode adds to the classic fields:
the Tools (name, description, parameter schema), the caps (Turns, deadline, token
budget, calls per Turn, Tool and preview timeouts, result caps, the budget note),
the opt-ins and the Envelope sentence (`harnessEnvelope`). The sentence states
that Tools read live data as the account label and can reach everything that
account can read, no more, that values are read when a Tool runs and not when the
user approves, and what each opt-in adds. The `groundingHash` covers all of it and
the account identity (user ID, role, scope, superuser;
`scopeIdentityOf`), which is hashed and never shown. It does not cover the values
Tools will return. The continuation rule
(`templateContinuationRuleHarness`) says every message sent is listed in the
transcript after the run.

The Envelope's boundary is the account's subtree scope enforced by the scoped
request context, plus the two opt-ins. Because a group-limited account is refused
earlier, the label is always the whole library (`harnessPreparation.env`).

## The loop

`harnessRun.loop` repeats, for Turn 1, 2, ...:

1. Stop with `deadline` if the run context expired.
2. Re-check access (`checkAccess`, below).
3. Decide whether this is the **final Turn** (`finalReason`), in this order:
   `turn_cap` (the Turn number reached `ai_template_max_turns`), `finish_now` (the
   user pressed Finish now), `deadline` (less time left than one provider timeout
   plus one Tool timeout), `budget` (less than the final reserve left). A final
   Turn appends the instruction `templateRunFinalInstruction` and only `submit`
   runs in it; other calls get `not run: final Turn`.
4. **Admit** (`admit`): estimate the outgoing request at 2.5 bytes per token (a
   deliberate over-count), set `max_tokens` to `min(profile max tokens,
   remaining budget - estimate)`. If that is below the final reserve (4000 tokens,
   or the profile's `max_tokens` when smaller) the Turn becomes final with reason
   `budget`. If it is below 1000 tokens, nothing is called and the run stops with
   `budget`.
5. Call the provider (`callProvider`) under the call timeout, bounded by the run
   deadline, and the process-wide provider slot semaphore (eight slots, shared
   with classic mode; the wait is inside the timeout). The access check runs again
   after the slot is acquired, immediately before the request is sent.
6. Charge the budget (`ChargeableTokens`): prompt tokens not served from cache,
   plus completion tokens, plus a quarter of the cached prompt tokens. A missing
   count falls back to the estimate or to `max_tokens`.
7. Re-check access, record the assistant message, and act on the reply
   (`handleReply`): run the Tools, validate and commit any submit calls, append
   the tool message.

The run ends when a `submit` call sets `done` (`done`), when the final Turn ends,
or on an override.

### Stop reasons

| Stop reason | When |
| --- | --- |
| `done` | A submit call committed with `done: true` |
| `turn_cap` | The Turn cap was reached; the last Turn was final |
| `finish_now` | Finish now was requested; the next Turn was final |
| `deadline` | The run deadline expired, or too little time was left for another Turn |
| `budget` | Not enough token budget for another Turn |
| `length_repeated` | Two consecutive replies were cut at the output limit with no complete call |
| `no_tool_call` | The model answered with plain text twice, or on the final Turn |
| `provider_error` | The provider call failed |
| `privacy_guard` | The request about to be sent carried an Entity value. It was not sent; the run has no Draft |

A reply cut at the output limit (`finish: length`) gets an error result on the cut
call, `cut off at the output limit; send it again in smaller parts`, and the run
continues. Plain text with no call gets one nudge (`templateRunNudgeText`).

### Replay and stubs

A reply's `Replay` (the provider's own assistant payload, including signed
reasoning) is stored and resent byte for byte on later Turns. Only for providers
that allow rewriting history (`HistoryRewritable`: DeepSeek, and OpenRouter models
under `deepseek/`) are Tool results older than three Turns replaced by a stub
that is fixed once written (`outgoing`, `templateRunStubRetention`). Other
providers are never rewritten. Anthropic requires thinking signatures to stay with
the messages they were issued for; whether OpenAI's encrypted reasoning items
tolerate edits to earlier messages is undocumented, and an OpenRouter model
outside `deepseek/` may be a Claude model, so history is preserved conservatively
for all of them (`HistoryRewritable`).

## Submissions and the Draft

`submit` takes a map of field name to content, an optional explanation and
`done`. The arguments are validated against the Turn-start state
(`parseSubmitArgs`), each field is linted, and the lint result is returned to the
model. Submits of one Turn commit in call order after all calls ended, so two
submits of one field in a Turn resolve to the later call. The last submission of
each field is kept.

The Draft is built when the run ends (`buildResult`) from the last committed
value of each field, linted again with the same validator:

- A pair (an HTML slot and its CSS) is applied whole or not at all: submitting one
  half adds an error to it.
- For a whole template with sections, `CustomCSS` is additions appended once to
  the current stylesheet (`templateRunCombinedCSS`), never a rewrite.
- A field that was requested and required but not submitted gets a warning.
- A run that ended on `deadline` or `provider_error` with at least one submission
  returns the last Draft with a warning that the run ended early. A run with no
  submission returns an error carrying the stop reason and the transcript
  (`TemplateRunError`).

Nothing reaches an editor until the user presses Apply.

## Provider layer

`TemplateConversationProvider` (`ai_conversation.go`) is the harness seam beside
the classic `TemplateDraftProvider`. Three wire adapters serve four providers:
Chat Completions with tools (DeepSeek and OpenRouter), OpenAI Responses, and
Anthropic Messages. The OpenAI endpoint differs from classic mode
(`generationConversationProvider.Endpoint`): tool calling on current OpenAI
reasoning models needs Responses. All adapters share the classic client's
redirect refusal, 4 MiB response cap and sanitized errors
(`GenerationProviderError`: status and a class of `http`, `transport`, `decode`,
`size` or `refusal`; never a body or a header). Usage is normalized so that
`Prompt - CacheHit` is the uncached part for every provider
(`ai_conversation_usage.go`); a field the provider did not report stays nil.

A call whose arguments are cut or are not an object is marked `Incomplete` or
`Malformed` (`classifyArguments`), never run, and answered with an error result so
every call ID is answered.

## Transport: event stream and finish route

A real request sent with `Accept: text/event-stream` is answered as server-sent
events once the run reaches its first Turn; any error found earlier keeps its
ordinary status code (`serveHarnessRun`, `templateRunStream`). Without that
header the answer is one JSON result at the end.

| Event | Payload |
| --- | --- |
| `run` | `runId`, and the caps `maxTurns`, `deadline`, `tokenBudget` |
| `turn` | `turn`, `maxTurns`, `finalReason` |
| `tool` | `turn`, `name`, `args` (fixed set), `bytes`, `isError`, `durationMs` |
| `submission` | `turn`, `field`, `errors`, `warnings` |
| `usage` | `turn`, `budgetUsed`, `budgetMax` |
| `result` | The Draft response plus `stopReason`, `submitted`, `transcript`. Last |
| `error` | `error`, `status`, `stopReason`, `transcript`. Last |

Progress events (`turn`, `tool`, `submission`, `usage`) carry no free text: only Tool
names from the catalogue (else `unknown`), values from fixed sets, IDs and counts
(`progressArgs`, `sanitizeProgressArgs`). One goroutine owns every write. Progress
passes through a channel of 64 and is dropped when it is full; `run`, `result` and
`error` are never dropped. A `: heartbeat` comment is sent every 15 seconds, each
frame has a 10 second write deadline, and a failed write cancels the run, as does
leaving the stream.

**Finish route.** `POST /v1/{category|resourceCategory|noteType}/generateTemplate/finish`
with `{"runId": ...}` (`GetFinishTemplateRunHandler`) sets the run's flag; the
next Turn becomes the final Turn (`finish_now`). An unknown run and another
account's run both answer 404, so run IDs cannot be probed. The route is mounted
under the carrier's prefix and inherits the generate route's role gate.

**Delivery order.** After the generator returns, the handler stops accepting
progress, drains what is queued, re-checks access under a fresh request-parent
context (`stillHasAccess`, 5 s), and only then writes the terminal frame or
JSON. If the client left, nothing is written. If access changed, the Draft and the
transcript are dropped. Before streaming starts, or in JSON delivery, the response
is HTTP 403. After the event stream has started the HTTP status is already 200, so
the terminal frame is an `error` event whose payload carries `"status": 403`
(`terminalFrame`).

## Access

Access is re-checked, not assumed: around every Turn (before the Turn, after the
provider call, before the provider call itself, and before releasing anything) and
by the handler before delivery. `templateRunAccessFactory` re-resolves the
request's original credential under the given context; with auth on, a disabled,
demoted or newly scoped account yields `ErrTemplateRunAccessChanged`, and a
changed identity is never adopted. With auth off the implicit administrator
stands. A failure to verify access fails closed as a change. A cancelled lookup is
cancellation, not revocation. A run whose access changed returns a
`TemplateRunError` with no transcript and releases nothing.

## Run registry and limits

`StartTemplateRun` registers a run before it starts and a deferred `Release`
removes it on every exit path. One account may have `MaxLiveTemplateRunsPerAccount`
(2) live runs; a third is a 429 (`ErrTemplateRunLimit`). The account key is
`TemplateRunAccountKey`: `user:<id>` when accounts are enabled, and `ip:<client
address>` when auth is off. With auth off every request carries the root
administrator principal, so the user ID cannot tell clients apart; the limit and
the ownership check of the finish route are therefore keyed by the client address
(`templateRunAccountKey` in `template_run_handlers.go`). Behind a proxy that
address is `X-Forwarded-For` (leftmost), then `X-Real-IP`, then the peer address
(`ClientIP`).

## Settings

Five runtime settings in the `ai` group (`addAIHarnessSettingSpecs`); a request
reads them once (`templateGenerator`), so changing one never alters a run in
progress:

| Key | Default | Bounds |
| --- | --- | --- |
| `ai_template_generation_mode` | `harness` | `harness` or `classic` |
| `ai_template_max_turns` | 10 | 1 to 20 (`MaxTemplateRunTurns`) |
| `ai_template_run_deadline` | 5 minutes | 30 seconds to 15 minutes |
| `ai_template_run_token_budget` | 400000 | 100000 to 2000000 |
| `ai_template_privacy_mode` | `per-run` | `per-run` or `always` |

Model, thinking, `max_tokens` and per-call timeout per Turn come from the same
call profile classic mode uses for the request's kind (`slot`, `pair`, `bundle` or
`metaschema`).

**Classic mode.** Setting `ai_template_generation_mode` to `classic` keeps the
single-call flow, repair and fan-out unchanged, and the opt-ins, Tools, run panel
and finish route do not apply. The mode is read per request, and the Disclosure
reports it (`Mode`).

## Privacy mode

Terms are in `GLOSSARY.md`; the decision is
`docs/adr/0010-privacy-mode-masks-entity-values-at-the-source.md`; the plan with
every design decision (P1 to P16) is `docs/plans/2026-11-08-template-privacy-mode.md`.
Privacy mode works in both generation modes. With it on, no Entity value read from
the database leaves the server in a request to a provider. It is on unless the
request sends `privacyOff` (a negative name, so an omitted field means on), and an
administrator can require it with `ai_template_privacy_mode=always`: the handler then
answers 400 "Privacy mode is required on this server" to `privacyOff`, a dry run
included, and the Disclosure reports `privacy.locked`. Privacy mode with
`previewPlugins` is a 400 too, because plugin Lua returns arbitrary text that cannot
be masked at the source. Both refusals come before any library read. The Disclosure
carries `privacy: {on, locked}` in both modes and hashes it, so approving with it on
and sending with it off is a 409. Its `legend` is shown and never hashed.

**Masking at the source.** `TemplatePrivacy` (one per request, nil means off) replaces
a value with a Placeholder `⟦tag·kind n · lines · ~len chars⟧`; a date is
`⟦tag·date n⟧`. The tag is an HMAC of the account, entity type and carrier under a
per-process key, so a dry run and the request after it build the same Seed bytes.
Equal values share a Placeholder. Numbers, booleans and IDs pass through. Masked
places: the Seed (identity description, starting-point names, sample projection,
partial descriptions), every Tool result (`get_entity`, lists, revision labels and
dates, partial descriptions), and the entity a `preview` renders against (a deep
copy, `TemplatePrivacy.Entity`, whose dates become the stand-in
`2001-02-03T04:05:06Z`, whose `[]byte` fields are dropped and whose URL becomes
`https://placeholder.invalid/<tag>-url-<n>`). `[mrql]` results are masked into new
containers (`QueryResult` never mutates the cached result); saved queries render the
fixed text "saved queries are not available in Privacy mode" and an inline query's
error becomes fixed text. Readable vocabulary is an allow-list
(`privacyFieldKept`): taxonomy names and `Custom*`, MetaSchema and SectionConfig of
the three carriers, relation type names, note block types, Meta keys, partial names,
and a Resource ContentType that is plain `type/subtype`. A Meta string equal to an
`enum` or `const` of its own field stays readable. The sample projection is
project, then mask, then clip.

**The guard.** `generationProviderClient.postPayload` hands every body to
`CheckOutbound(ctx, payload)` when the request context carries a masker
(`WithTemplatePrivacy`, put there by `runHarness` and by classic
`GenerateTemplate`). It walks every string leaf and looks, with an in-package
Aho-Corasick automaton, for each masked value of four or more characters in its raw,
JSON-escaped and HTML-escaped spelling, case-insensitively. Only the first 64 bytes
of a value are matched, so a truncated copy of a long value is still found. A value
that also occurs in the run's readable corpus is exempt, permanently. The corpus is
filled by `Keep`: readable Seed sections, the Tool catalogue and the model
identifier, each adapter's protocol strings (`SkeletonStrings`), the model's own
output, server-authored loop strings, readable Tool results, partial source a
preview resolves, preserved numbers and the formatted text made from them
(`shortcodes.WithScalarFormatObserver`). A source nobody registered can only cause a
false positive, never a missed leak. All reconciliation happens inside
`CheckOutbound`, under the call's context. The masker stops at 20,000 values, 4 MiB
of legend bytes or one value over 256 KiB: past a limit it returns the fixed
`⟦tag·withheld⟧`, keeps a sticky error, every publisher discards its partial output,
and the guard refuses outright.

**A refusal.** Every wrap between the transport and the handler uses `%w`. Harness:
the run stops with `privacy_guard`, returns no Draft even when it has submissions,
and marks the refused Turn's transcript records `sent:false` and reverts a stub
first written that Turn (`rollbackCommit`). Classic: the error is fatal, repair never
absorbs it and a fan-out cancels its sibling sections. The answer is HTTP 500 with
the fixed text "Privacy mode stopped a request that contained an entity value.
Nothing was sent".

**A Placeholder in a Draft.** Nothing maps a Placeholder back. In harness mode
`submit` refuses a field that contains one, in either spelling (an error result, the
call's `done` ignored, other fields commit), `buildResult` re-checks, and classic
raises a validator error so that repair runs. A final pass over every result
(`withholdPlaceholders`, after fan-out assembly) empties a field that still has one,
lists it in `withheldFields` and withholds an HTML/CSS pair whole.

**The legend.** `legend` (`[{placeholder, value}]`, one entry per spelling) is on the
dry-run Disclosure, the result, and any error payload that carries the transcript, and
it is dropped exactly when the transcript is dropped on an access change. It is never
stored, logged or sent to a provider.

## Eval

`internal/templateeval` scores both modes. The recorder wraps the provider seams
(`Recorder.WrapDraftProvider`, `WrapConversationProvider`), takes error class and
status from `GenerationProviderError` and keeps no body or header. Diagnostic
copies of tool arguments and `Replay` are decoded, redacted at every string leaf
and re-encoded; text that is not complete JSON becomes
`[unparsed arguments: N bytes]`. The live `Replay` is never altered
(`TestRecorderHarnessRecordsTurnsToolCallsAndUsage`). The scorer is mode-aware: it
scores the final Draft, treats a cut Turn followed by a complete submission as
recovered, and counts truncation of the retained Draft as a failure
(`score_harness_test.go`). Reports record mode, settings, Turns, stop reasons and
tokens per run; cost is `not computed` because the repository has no pricing
source.

## Tests that pin this

- `application_context/template_run_test.go`, `template_run_budget_test.go`,
  `template_run_review_test.go`: the loop, stop reasons, admission, stubs.
- `application_context/template_run_submit_test.go`, `template_run_seed_test.go`,
  `template_run_tools_exec_test.go`: submissions, the Seed, argument checks.
- `application_context/template_run_registry_test.go`,
  `template_run_access_test.go`: the account limit and access re-checks.
- `server/api_tests/template_run_*_test.go`: the executor against the database,
  refusals, the event stream, the finish route and terminal delivery.
- `server/api_handlers/template_run_stream_test.go`: frame shapes and the writer.
- `internal/templateeval/*_test.go`: the recorder, the adapter fixtures and the
  scorer.
- `application_context/template_privacy*_test.go`, `server/api_tests/template_privacy_test.go`:
  the masker, the guard (thresholds, case, escapes, corpus exemption, paths,
  benchmarks), `Entity` field coverage, the Seed in both modes, the loop's
  `privacy_guard` and rollback, and a scripted run through every Tool behind the
  real wire adapters that asserts no protected marker reaches the provider.
