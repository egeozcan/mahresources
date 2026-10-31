# Runtime AI providers, credentials and models

Status: implemented and verified following user approval.

## Success criterion

An administrator can configure DeepSeek, OpenRouter, OpenAI and Anthropic API
keys, choose the provider and model for MRQL and template generation separately,
and change those choices on a running server. The next generation request uses
the saved configuration, including on a server that started with no API keys.
Overrides survive a restart. Saved keys never appear in API responses, page
source, CLI output, disclosures, audit records or application/SQL diagnostics.
Existing DeepSeek environment configuration continues to work without changes.

## Scope and assumptions

- Configuration is deployment-wide and admin-only, following `/admin/settings`.
  The roles allowed to use each generation feature stay the same. This does not
  introduce per-user credentials or change subtree permissions.
- Both existing AI features are in scope: MRQL drafting and generation of
  category, resource-category and note-type templates, including MetaSchema,
  planner calls, HTML/CSS pairs and repair.
- Keep one key per provider. MRQL and templates each select one provider and a
  default model. Templates retain model overrides for the existing `slot`,
  `pair`, `plan`, `bundle` and `metaschema` call kinds; one template operation
  uses its selected provider for all those kinds. Persist model selections by
  provider, so switching back restores that provider's choices and a single-key
  API/CLI change cannot silently borrow models from the previously selected one.
- Model IDs are editable text, so using a newly released model requires no
  application release. No embedded model catalogue or automatic model choice.
  Saving configuration does not make a paid test call or prove model access.
- Use the four providers' fixed official endpoints. Custom endpoints, automatic
  failover, per-request model pickers and changes to the eval harness are outside
  this feature.
- Reuse runtime-settings persistence and its precedence/reset behavior, adding
  actual secret support before registering credentials. Stored keys are readable
  by the server and database administrators; this does not introduce encryption
  at rest or a new master-key lifecycle. Document this storage property.

## Configuration behavior

1. Add runtime secret entries for the four providers. `DEEPSEEK_API_KEY` remains
   the DeepSeek boot fallback; `OPENROUTER_API_KEY`, `OPENAI_API_KEY` and
   `ANTHROPIC_API_KEY` supply optional boot fallbacks for the other keys. No key
   is accepted through a new command-line flag.
2. Add runtime provider selections for MRQL and templates, provider-specific
   default-model entries for each, and provider-specific model overrides for the
   five template call kinds. Defaults select DeepSeek and resolve its models
   through the existing `DEEPSEEK_MODEL` and
   `DEEPSEEK_TEMPLATE_*_MODEL` values. A non-DeepSeek provider requires an
   explicit default model; it never silently inherits a DeepSeek boot model.
3. Preserve DeepSeek's existing timeout, token-budget, repair and thinking
   behavior. Its thinking overrides apply only to DeepSeek. Other providers use
   model-default reasoning, disclosed as `provider_default` rather than falsely
   reporting thinking disabled; do not forward DeepSeek's `thinking` object to
   another API. Because reasoning can consume tokens before visible JSON,
   non-DeepSeek defaults allow 8192 output/completion tokens for MRQL, planner,
   slot, pair and MetaSchema calls, and 16384 for a bundle. Their default call
   timeouts are 60s (120s for a bundle), with a 120s whole-template fan-out
   deadline. These are bounded defaults, not a promise every model fits them.
   Provide optional advanced runtime output-token and timeout overrides with
   the provider-specific profiles (positive bounded values; at most 65536 tokens
   and 10 minutes), so operators can tune a chosen model without a restart.
   Explicit timeouts/token overrides take precedence over provider defaults.
4. A template kind's explicit runtime model overrides its template default model.
   Legacy DeepSeek per-kind model environment values apply only while the
   template provider is DeepSeek. The admin form shows the effective profiles.
   Changing provider in the form loads that provider's saved model selections,
   or requires a new default model when none is configured. Optional kind
   models may inherit its default. Server resolution enforces this provider
   association for UI, batch API, single-key API/CLI changes and resets alike.
5. Secret reads expose only `secret`, `configured`, boot-configured and override
   status, never the key, a prefix/suffix or a mask that could be saved as a key.
   Keep password inputs empty after load/save. Leaving a key field untouched
   preserves it; explicit Replace, Clear and Reset operations are distinct.
   Clear saves an empty override, disabling that credential even when an env
   fallback exists. Reset deletes the override and restores the boot fallback.
6. Extend the settings service/API with an atomic batch save for a profile form.
   Validate all entries before writing. Hold the same mutation lock used by
   single-key Set/Reset across the database transaction and cache publication:
   commit the transaction, publish all cache changes, then release the lock.
   A failed write leaves both the database and cache unchanged. Existing
   single-key commands use the same validation/redaction rules.
7. Saving configuration validates provider names and bounded model/key syntax
   without echoing secret values. Missing credentials/models leave generation
   unavailable with the existing unconfigured response; no outbound call occurs.
   Unknown or inaccessible model IDs produce a sanitized provider error when
   used, rather than an automatic fallback to another provider.
8. Runtime overrides take precedence over boot values and persist in the existing
   settings table. Grouped feature reset atomically deletes its provider
   selection and all its provider-specific model/budget/timeout overrides under
   that same lock, then publishes boot fallbacks together. Credentials have
   separate resets. Single-key API/CLI resets keep their documented meaning.

## Provider integration and request lifetime

- Keep the existing MRQL and template draft-provider seams, local validation,
  truncation rules, timeouts, rate limits and manual review before execution/save.
  Share transport code where the wire contract is actually shared.
- DeepSeek and OpenRouter use chat-completions adapters; OpenAI gets its own
  request options, including its completion-token limit field. Do not assume
  that compatibility means every provider accepts every request parameter.
- Anthropic uses its native Messages API. Adapt system/user messages,
  authentication/version headers, text content blocks, stop reasons and usage
  into the existing generator result types. Request JSON in the existing
  generation prompts and preserve strict local parsing/validation. Handle
  refusal, missing text and token exhaustion explicitly.
- Keep raw provider errors/bodies and credentials out of user-visible errors and
  host logs. Provider diagnostics may identify only the provider, model, status
  and safe error classification. Disable credential-bearing redirects.
- Install runtime-aware generator entry points even when no boot key is present.
  Capture one immutable effective settings snapshot at the start of each MRQL
  operation and each template disclosure/generation operation. Template planning,
  fan-out and repair keep that same snapshot even if an admin saves settings
  while a call is running.
- Settings and admission state are process-lifetime shared pointers across
  `WithPrincipal`/transaction context copies. A runtime update must not replace
  or reset rate limiters or the eight-slot template provider semaphore. An
  in-flight request completes with its original configuration; new operations
  use the saved one. Existing single-process settings-cache semantics remain.
- Template disclosures show the actual provider, endpoint, model by call kind
  and transport format/authentication scheme without credential values. Hash
  the public routing/options as well as the prompt data, so a provider/model
  change between disclosure and generation requires another review. Credential
  rotation alone does not add secret material to a client-visible hash.
- Replace DeepSeek-specific user-facing descriptions, including MRQL's privacy
  text, with the configured provider or accurate provider-neutral language.

## Implementation checklist

### 1. Baseline and secret-aware settings

- [x] Read changed files in full; record the clean baseline and run the existing
      Go, frontend and ephemeral E2E suites before implementation.
- [x] Add regression coverage in `runtime_settings_test.go` and
      `runtime_settings_boot_test.go` for secret list/default/status, load,
      validation failures, set, clear, reset, audit and failed SQL writes.
- [x] Add secret metadata and centralized safe views/logging in
      `runtime_setting_spec.go` and `runtime_settings.go`. Prevent GORM/slow-query
      logging and wrapped database errors from exposing credential parameters.
- [x] Add transactional batch mutation and a coherent snapshot reader with tests
      for rollback and deterministic concurrent reads/updates. Keep audit writes
      outside the SQLite settings transaction.

### 2. Provider transports

- [x] Add fake-server protocol tests for all four providers: destination,
      authentication, request options, JSON content, usage, truncation/refusal,
      malformed responses, non-2xx, cancellation, timeout and redirect refusal.
      Include reasoning-only/empty responses when the output budget is exhausted
      and assert truthful reasoning mode and effective budgets in disclosures.
- [x] Implement provider adapters with the standard library and existing
      dependencies. Preserve the existing DeepSeek constructors for current
      callers and benchmark compatibility.
- [x] Prove MRQL and template draft generation work through each adapter using
      realistic mocked provider responses, not live credentials or paid calls.

### 3. Runtime wiring and disclosure

- [x] Register provider keys and generation settings, wire the optional env-key
      fallbacks and install runtime-aware generators after settings load.
- [x] Test enabling generation after a keyless startup, rotation/clear/reset,
      provider/model switches, restart/reload and context-copy visibility.
      Exercise single-key API/CLI selection/reset too: a different provider's
      saved model must never become the effective model by accident.
- [x] Test that a concurrent update cannot mix a provider, model and key or
      change a template's planner/pair/repair configuration partway through.
- [x] Keep one process-wide semaphore across changes and prove old/new requests
      together cannot exceed the existing provider-slot limit.
- [x] Adapt template disclosures and their hash tests for each provider, stale
      review after a routing change, and credential non-disclosure. Update MRQL
      generation status/privacy text as needed.

### 4. Admin API, UI and CLI

- [x] Add the atomic save endpoint under the existing admin-settings policy;
      update OpenAPI definitions and ensure errors have correct HTTP status.
- [x] Add a focused AI section to `/admin/settings`: provider dropdowns, model
      inputs, optional template kind overrides and write-only credential controls.
      Put optional output budgets/timeouts in an advanced section and show their
      effective inherited values.
      Follow existing Alpine form, focus, status/error and keyboard patterns.
      Exclude these entries from the generic settings rows so duplicate controls
      cannot bypass the grouped profile and explicit credential actions.
- [x] Test real persistence through the UI, native Enter submission, saved key
      status, clear/reset, errors and provider changes. Cover keyboard and axe
      accessibility checks.
- [x] Cover API admin authorization and cookie-CSRF protection, including denials
      for editor/user/guest; keep existing generation permissions/scope tests.
- [x] Update the CLI settings view/JSON representation to preserve secret status
      without printing keys. Keep existing single-key commands working and
      update their help/docs where behavior changes.

### 5. Documentation and final verification

- [x] Update `docs/architecture/configuration.md`, runtime-settings and generation
      docs, OpenAPI descriptions and relevant privacy text. Add an architecture
      note for runtime AI configuration, snapshots and credential handling.
- [x] Rebuild committed JS/CSS assets as needed, generate/validate affected
      OpenAPI and CLI documentation, and inspect generator diffs for unrelated
      changes.
- [x] Run `go test --tags 'json1 fts5' ./...`, `npm run test:unit`, appropriate
      race tests, `./scripts/css-scan-test.sh` and the frontend build.
- [x] Run `cd e2e && npm run test:with-server:all`, the affected auth/a11y coverage
      and the Postgres Go/E2E suites from `AGENTS.md` when Docker is available.
      Record commands, results and any unavailable checks explicitly.
- [x] Review the final diff against the success criterion and document the
      observed behavior, test evidence and remaining limits below.

## Reference contracts checked during planning

- [OpenAI Chat Completions](https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create):
  request fields and model-dependent parameter support.
- [OpenAI reasoning](https://developers.openai.com/api/docs/guides/reasoning?api-mode=chat):
  reasoning consumes completion-token budget before visible output.
- [OpenRouter Chat Completions](https://openrouter.ai/docs/api/api-reference/chat/create-a-chat-completion):
  endpoint, authentication and compatible request/response shape.
- [Anthropic Messages](https://platform.claude.com/docs/en/api/messages/create):
  native endpoint, headers, content blocks and completion outcomes.

## Review

The user approved the plan before implementation. Baseline checks passed:
tagged Go suite, 2390 frontend unit tests, and 2625 browser/CLI E2E tests (5
skipped). Regression tests were observed failing before the corresponding
runtime generation, provider adapters, secret settings and disclosure fixes.


Implementation review confirms provider-bound model defaults, write-only
credential views, atomic settings persistence/cache publication, coherent
operation snapshots and one shared template provider semaphore. Independent
reviews found no remaining implementation findings.

Verification completed so far:

- `go test --tags 'json1 fts5' ./...`: passed after regenerating committed
  OpenAPI and completing QA-artifact cleanup before the repository scan.
- `go test --tags 'json1 fts5 postgres' ./mrql/... ./server/api_tests/... -count=1`:
  passed, including the PostgreSQL secret-batch persistence/rollback test.
- Frontend unit suite: 2410 tests passed; targeted generation/settings race
  suite passed.
- Focused admin/AI/accessibility browser checks: 9 passed. PostgreSQL AI checks:
  9 passed across three repetitions with retries disabled. Desktop and mobile
  visual inspection found no overflow, clipping or console errors.
- Frontend/server build, CSS source scan, OpenAPI validation/freshness, CLI
  documentation generation/lint and Docusaurus production build passed.

The first broad browser runs overlapped frontend development/rebuilds. The
PostgreSQL settings focus failures matched a defect observed red and then fixed;
its current no-retry repetitions pass. An unrelated original SQLite promotion
retry and PostgreSQL note-rendering failure did not reproduce in controlled
repetitions (96 promotions and 65 note CRUD checks passed). Their original
attempts have no network trace, so no root cause is claimed and no unrelated
product change was made. Final full SQLite and PostgreSQL sweeps passed with assets frozen and separate
artifact directories: SQLite 2630 passed / 5 skipped (7.1 minutes), PostgreSQL
2631 passed / 4 skipped (7.6 minutes), both with no retries or failures.

Provider integration tests use fake transports against the fixed official URL
contracts; no live credentials or paid provider calls were used. Runtime keys
remain unencrypted database values, and the settings cache retains its existing
single-process update semantics, as approved above.


Final commands for the frozen sweeps were `npx playwright test
--output=/tmp/mahresources-ai-sqlite-final-results --reporter=line` from `e2e/`
(using the same fixture-managed ephemeral instances as the all-tests script),
and `npm run test:with-server:postgres --
--output=/tmp/mahresources-ai-postgres-final-results --reporter=line`. The
standard `npm run test:with-server:all` had also run successfully earlier;
separate output paths prevented concurrent verification from overwriting traces.

Final diff review: source, regression tests, generated frontend assets, generated
OpenAPI, CLI help/generated docs and configuration/architecture documentation
are included. No dependencies were added, no unrelated product code was changed,
and no real provider credentials were read or sent. QA screenshots and diagnostic
reports live outside the repository.
