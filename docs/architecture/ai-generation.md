# Runtime AI generation

MRQL drafting and category-template generation select their provider separately.
Both support `deepseek`, `openrouter`, `openai` and `anthropic`. An administrator
can save credentials, models and call budgets at `/admin/settings` without
restarting, including on a server started with no credentials. The permissions,
subtree checks and manual review required to use generation are unchanged.

## Settings and credentials

`application_context/ai_generation.go` registers the AI settings in the existing
runtime-settings service. The names are:

| Key | Meaning |
| --- | --- |
| `ai_<provider>_api_key` | One write-only credential per provider |
| `ai_mrql_provider`, `ai_template_provider` | Selected provider for each feature |
| `ai_<feature>_<provider>_model` | Default model for that feature and provider |
| `ai_<feature>_<provider>_max_tokens` | Output/completion-token cap; `0` inherits |
| `ai_<feature>_<provider>_timeout` | Per-call Go duration; `0s` inherits |
| `ai_template_<provider>_<kind>_model` | Optional per-kind model; empty inherits |
| `ai_template_<provider>_<kind>_max_tokens` | Per-kind cap; `0` inherits |
| `ai_template_<provider>_<kind>_timeout` | Per-kind duration; `0s` inherits |

`<feature>` is `mrql` or `template`; `<kind>` is `slot`, `pair`, `plan`,
`bundle` or `metaschema`. Model IDs are editable strings, up to 256 bytes,
without whitespace or control characters. They are not checked against a local
catalogue or tested through a paid provider call when saved. Token caps allow
1–65536 and timeouts allow positive durations up to 10 minutes, with zero as the
inherit sentinel.

Model settings belong to a provider even through single-key API/CLI changes:
switching OpenAI on cannot reuse a saved DeepSeek model. Switching back restores
that provider's saved profile. Both feature selections default to DeepSeek.
Existing `DEEPSEEK_MODEL`, timeout and per-kind environment values remain its
boot defaults. Other providers need an explicit model. The four `*_API_KEY`
environment variables supply credential boot defaults; runtime overrides win.

Credential reads return status only: `secret: true`, `current: null`,
`bootDefault: null`, `configured`, `bootConfigured` and override metadata. The
UI never fills a password field from a stored value. Omitting a credential from
a save preserves it; setting an empty override clears it even if an environment
key exists; reset removes the override and restores the environment fallback.

Runtime credential overrides are plaintext values in `runtime_settings`, readable
by the server and database administrators, including in backups. This feature
does not encrypt that table or introduce a master key. Centralized secret-aware
views, auditing and diagnostics keep the values out of settings responses, page
source, CLI output, audit old/new values and SQL/application logging. Validation
and database failures return sanitized errors for secret writes.

`PUT /v1/admin/settings` accepts an atomic batch
`{ "values": { "key": "value" }, "reset": ["key"], "reason": "..." }`.
All entries validate before mutation. Single-key Set/Reset and batches hold the
same lock across the database transaction and cache publication; failures publish
nothing. Audit writes happen after the transaction, avoiding a second SQLite
connection trying to take its own transaction's writer lock. Feature reset
deletes the feature's selection and provider-specific profile overrides
together; credentials have independent resets.

## Effective call profiles

Explicit per-kind values take precedence over feature/provider defaults, then
the shipped profile or DeepSeek boot settings. The admin UI and
`GET /v1/admin/settings/ai-profiles` expose resolved, non-secret profiles.

| Provider | MRQL | Template calls | Whole-template deadline |
| --- | --- | --- | --- |
| DeepSeek | 800 tokens, `DEEPSEEK_TIMEOUT` (20s) | Existing per-kind profiles: 4000 tokens for slot/pair/MetaSchema, 1200 for plan, 8000 for bundle; thinking raises these to 8192/16384 | 55s |
| OpenRouter, OpenAI, Anthropic | 8192 tokens, 60s | 8192 tokens and 60s; bundle 16384 tokens and 120s | 120s |

DeepSeek thinking and per-kind timeout boot overrides remain DeepSeek-specific.
Other providers use model-default reasoning, disclosed as `provider_default`.
Hidden reasoning can consume the output budget before visible JSON; an empty
token-exhausted template response remains a truncation result, while MRQL
requires a complete response. Operator overrides can tune budgets and call
timeouts but do not remove the whole-template deadline.

## Request snapshots and admission

`InstallAIGeneration` installs one runtime-aware generator shared by request and
transaction context copies after settings load, even when no key is present.
Each MRQL operation and each template disclosure/generation operation reads one
coherent settings snapshot. A generation keeps its provider, key, models and
options through planning, fan-out and repair; a concurrent settings change only
affects later operations.

The controller owns the template semaphore. Rebuilding a per-operation generator
reuses that semaphore, so old and new settings together admit at most eight
template provider calls. Existing process-wide rate limiters are never reset by
a settings save. Waiting for a provider slot counts against the call timeout.
The settings cache retains its existing single-process semantics: another
running process does not receive automatic invalidation after a write.

## Provider transport and disclosure

`ai_provider_client.go` adapts the existing MRQL/template draft-provider seams
with the standard library. Runtime endpoints are fixed:

| Provider | Endpoint and request options |
| --- | --- |
| DeepSeek | `https://api.deepseek.com/chat/completions`; JSON-object chat completions, `max_tokens`, explicit DeepSeek thinking |
| OpenRouter | `https://openrouter.ai/api/v1/chat/completions`; JSON-object chat completions, `max_tokens` |
| OpenAI | `https://api.openai.com/v1/chat/completions`; JSON-object chat completions, `max_completion_tokens` |
| Anthropic | `https://api.anthropic.com/v1/messages`; native system/user messages, `max_tokens`, API version `2026-10-21`; JSON requested by the prompt |

All requests are non-streaming and use Bearer authorization. The Anthropic
version and authentication follow its [current Messages contract](https://platform.claude.com/docs/en/api/messages/create)
and [version migration](https://platform.claude.com/docs/en/api/migration-2026-10-21).
OpenAI's [completion-token cap](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create)
includes reasoning tokens. DeepSeek-only fields never go to another provider.
The legacy DeepSeek constructors retain their explicit endpoint argument for
tests and the eval harness; runtime settings cannot change endpoints.

Clients refuse redirects, including when an injected client would allow them,
and response reads stop at 4 MiB. Provider error bodies and arbitrary transport
errors never become host diagnostics. Cancellation and deadline sentinels remain
available for the existing HTTP error classification. Native text blocks,
refusal/stop reasons and cache/reasoning usage normalize into existing results.
MRQL parses one strict JSON object; templates retain their local shape, lint
and truncation checks. An unknown/inaccessible model returns a sanitized error
without falling back to another provider.

Template dry runs make no provider call. Their disclosure takes endpoint and
transport metadata from the adapter that sends the request and hashes public
routing/options, models, reasoning mode, budgets, timeouts and prompt sections.
Anthropic reports `prompt_json` rather than claiming forced JSON-object mode.
A provider/model/options change between review and generation requires another
review; credential rotation adds no secret material to the hash. A single
generation's later calls keep the reviewed snapshot.

## Verification

`ai_provider_client_test.go` pins destinations, auth, options, text blocks,
usage, truncation/refusal, sanitization, read bounds, cancellation and redirects
for all four providers, and exercises locally validated MRQL/template drafts.
Existing DeepSeek tests pin compatibility. `ai_generation_test.go` covers live
settings, provider-specific profiles, snapshots and shared admission.
Runtime-settings tests cover masked views, load/set/clear/reset, audit, batch
rollback and SQL diagnostics; API and browser tests cover authorization, CSRF,
persistence and accessible controls. These tests use mocks or ephemeral servers,
without live credentials or paid calls.
