# MRQL Natural-Language Generation

**Date:** 2026-06-27
**Status:** Design approved, ready for implementation planning
**Scope:** Web-only `/mrql` editor feature

## 1. Summary

Add a web-only "Describe results" feature to the existing MRQL editor. The user describes the result set they want, the server calls DeepSeek with MRQL syntax instructions only, validates the generated MRQL locally, and returns the query plus a short explanation. A valid, non-stale generated query is inserted into the existing CodeMirror editor, but it is not executed automatically. Invalid or stale generated output remains in the generation panel until the user explicitly applies it. The user reviews the explanation and presses Run through the existing MRQL execution path.

## 2. Product Decisions

- First surface is only the `/mrql` web page.
- Generation endpoint only generates and validates. It does not execute MRQL.
- The UI shows a short explanation before the user runs the query.
- DeepSeek credentials are configured by server-side environment variables.
- No local app vocabulary is sent to DeepSeek. The prompt includes only MRQL syntax/reference material and the user's natural-language request.
- The generated query is trusted only after passing the existing local MRQL parser, validator, and generator-specific safety lint.
- The endpoint is not a read-via-POST route in v1. It spends external provider quota, so it must remain CSRF-protected and require normal write capability when auth is enabled.

## 3. Current Context

The repo already has a complete MRQL path:

- Web editor: `templates/mrql.tpl` and `src/components/mrqlEditor.js`
- Execution: `POST /v1/mrql`
- Validation: `POST /v1/mrql/validate`
- Completion: `POST /v1/mrql/complete`
- Saved query endpoints under `/v1/mrql/saved`
- Backend execution and result shaping in `application_context/mrql_context.go`
- Parser/validator/translator in `mrql/`

Auth already treats MRQL execution, validation, completion, and saved-query running as read-via-POST operations for read-only principals. That read-via-POST list is also used for CSRF exemption, so the generation endpoint must not be added there. MRQL execution itself applies group-scope RBAC at query time.

DeepSeek's current official docs describe an OpenAI-compatible chat completions API at `https://api.deepseek.com/chat/completions`, models including `deepseek-v4-flash` and `deepseek-v4-pro`, and JSON output through `response_format: {"type":"json_object"}` when the prompt explicitly asks for JSON. Their docs also note that `deepseek-chat` and `deepseek-reasoner` are scheduled to be deprecated on 2026-07-24 15:59 UTC.

References checked on 2026-06-27:

- https://api-docs.deepseek.com/
- https://api-docs.deepseek.com/api/create-chat-completion
- https://api-docs.deepseek.com/api/list-models
- https://api-docs.deepseek.com/news/news260424

## 4. Goals

- Let users draft MRQL from natural language without learning the full syntax upfront.
- Keep execution explicit and reviewable.
- Protect privacy by not sending tag/category/note type/resource category names unless the user wrote them in the prompt.
- Keep provider details isolated behind a backend service so the browser never sees API keys.
- Validate generated MRQL using existing local code before showing it as runnable.
- Prevent generated drafts from causing surprising external calls, quota spend, saved-query overwrites, or stale-result confusion.

## 5. Non-Goals

- No CLI command in v1.
- No saved-query NLP creation flow in v1.
- No automatic query execution.
- No schema-aware generation using local metadata.
- No generic LLM provider marketplace or admin UI for secrets.
- No training, caching, or fine-tuning on user prompts.
- No guest/read-only generation in v1. Read-only access can be reconsidered later with separate CSRF-protected auth handling and rate limits.

## 6. Architecture

Add a new endpoint:

`POST /v1/mrql/generate`

Request:

```json
{
  "prompt": "photos from the last 30 days tagged invoice"
}
```

Response:

```json
{
  "query": "type = resource AND contentType ~ \"image/*\" AND tags = \"invoice\" AND created > -30d",
  "explanation": "Finds image resources tagged invoice from the last 30 days.",
  "valid": true,
  "errors": []
}
```

Backend pieces:

- A small generation service, preferably near `application_context` or a focused `mrql/generator` package.
- A DeepSeek client with injectable HTTP transport for tests.
- A `MRQLGenerator` interface hung off `MahresourcesContext` or passed into the handler constructor, with a test setter/fake so API tests never hit the real network.
- A request/response handler in `server/api_handlers/mrql_api_handlers.go`.
- A route in `server/routes.go`.
- OpenAPI registration in `server/routes_openapi.go`.
- Auth policy coverage that keeps `/v1/mrql/generate` out of `isReadViaPost`; the route should require normal CSRF protection and write capability in v1.
- Boot/env config fields on `MahresourcesConfig`.
- Generated-query safety lint that runs after `mrql.Parse` and `mrql.Validate`.

Frontend pieces:

- Extend `mrqlEditor()` with natural-language prompt state, generation loading/error state, generated explanation, and generated validity.
- Add compact UI to `templates/mrql.tpl` near the query editor.
- Valid, non-stale generation inserts text into the existing CodeMirror editor through `setQuery()`.
- Existing validation and execution paths remain unchanged.
- Successful generation clears any loaded saved-query update context and stale execution results.
- Invalid generation is shown in the generation panel and is not inserted into the editor by default.

## 7. Configuration

Environment variables:

- `DEEPSEEK_API_KEY`: required to enable generation.
- `DEEPSEEK_MODEL`: optional, default `deepseek-v4-pro`.
- `DEEPSEEK_TIMEOUT`: optional Go duration string, default `20s`; invalid values fail startup with a clear error.

Boot config should add `DeepSeekAPIKey`, `DeepSeekModel`, and `DeepSeekTimeout` fields. The API key intentionally has no CLI flag in v1, to reduce accidental process-list exposure.

The runtime settings table should not store the API key in v1 because settings are listable through the admin API/UI and currently return current values. The API key must also not appear in admin boot-only settings, OpenAPI examples, client JSON, templates, logs, or error strings. If runtime secret editing is added later, it should use a masked secret-specific storage and view model. Non-secret status may expose only a boolean such as `mrqlGenerationConfigured`, plus model and timeout.

Configured size limits:

- Natural-language prompt: trim whitespace, reject empty or whitespace-only, max 2,000 characters.
- Generated MRQL query: trim whitespace, reject empty, max 2,000 characters.
- Generated explanation: trim whitespace, reject empty, max 1,000 characters.

## 8. Prompt Contract

The server prompt should include:

- A concise MRQL syntax reference.
- A small set of examples covering entity type, text search, tags, dates, ordering, limit, and group-by.
- Explicit privacy rule: use only names, IDs, tag values, categories, or metadata keys that appear in the user's request.
- Explicit output rule: return strict JSON only.
- Clause order: filter expression, optional `SCOPE`, optional `GROUP BY`, optional `ORDER BY`, optional `LIMIT`, optional `OFFSET`.
- Strings must be double-quoted.
- MIME patterns use `contentType ~ "image/*"` or similar quoted strings.
- Relative dates use MRQL relative values such as `-30d` or supported date functions. Do not emit natural-language dates.
- `TEXT` search must use `TEXT ~ "plain words"` only. Do not use wildcards, empty punctuation-only values, or operators other than `~` with `TEXT`.
- `category`, `resourceCategory`, and `noteType` should use numeric IDs only when the user explicitly supplied an ID. Without local vocabulary, prefer `tags`, `name`, or `TEXT` rather than inventing category or type names.
- `GROUP BY` always requires an explicit `type`. Use `COUNT()` for aggregate counts. In bucketed group-by, explain that `LIMIT` applies per bucket.
- Add a modest `LIMIT` such as `LIMIT 50` for broad generated queries unless the user asks for a different limit.

Expected provider JSON:

```json
{
  "query": "type = resource AND contentType ~ \"image/*\" LIMIT 50",
  "explanation": "Finds up to 50 image resources."
}
```

The service should ignore any extra fields, reject missing/empty fields, and cap field lengths.

## 9. Provider Contract

DeepSeek requests should use non-stream chat completions:

- URL: `https://api.deepseek.com/chat/completions`
- `stream: false`
- `model`: configured `DEEPSEEK_MODEL`
- `response_format: {"type":"json_object"}`
- `max_tokens`: small bounded value, around 800
- Thinking disabled if the API supports a stable request option for the selected model

The client parses `choices[0].message.content` as JSON. It rejects:

- Empty `choices`
- Missing/null message content
- Malformed JSON
- Missing, empty, or oversized `query` / `explanation`
- `finish_reason` values that indicate truncation, refusal, content filtering, or incomplete output, including `length`
- Non-2xx provider responses

Errors returned to the browser must not include provider response bodies, provider request bodies, the user prompt, generated query, or generated explanation.

## 10. Generated-Query Safety Lint

After `mrql.Parse` and `mrql.Validate`, run generator-specific lint before returning `valid: true`.

Lint rules:

- Explicit `LIMIT` must be between 1 and 500.
- Explicit `OFFSET` must be between 0 and 10,000.
- `TEXT ~` must contain at least one alphanumeric term after MRQL wildcard/FTS sanitization. `TEXT ~ "*"`, punctuation-only strings, and empty strings are invalid generated drafts.
- `TEXT` must not use any operator except `~`.
- `GROUP BY` must include explicit `type`.
- `category`, `resourceCategory`, and `noteType` values should be numeric when generated from a prompt that did not explicitly provide a local name. If the provider emits a non-numeric value for those fields, return `valid: false` with an explanatory lint error.

`valid: true` means parser-valid, validator-valid, and generator-lint-valid. It does not guarantee execution will succeed, that any rows exist, or that DB-backed names/scopes resolve at execution time.

## 11. Data Flow

1. User enters natural language in the `/mrql` page.
2. User clicks Generate.
3. Browser sends `{ "prompt": "..." }` to `/v1/mrql/generate`.
4. Backend validates prompt length and configuration.
5. Backend calls DeepSeek with syntax-only context and JSON output.
6. Backend parses the provider JSON.
7. Backend validates the returned MRQL with `mrql.Parse`, `mrql.Validate`, and generated-query safety lint.
8. Backend returns `query`, `explanation`, `valid`, and validation errors.
9. Browser applies only the latest non-stale generation response. For a valid generated query, it inserts `query` into CodeMirror, clears loaded saved-query update state, clears stale execution results, and shows the explanation.
10. User presses Run if satisfied.

If the editor content changed while generation was in flight, the UI should not replace it automatically. It should show the generated result in the generation panel and offer an explicit "Use generated query" action.

## 12. Error Handling

Backend response policy:

- Missing prompt: `400`.
- Prompt too long: `400`.
- Missing `DEEPSEEK_API_KEY`: `503` with "MRQL generation is not configured."
- Provider network error: `502`.
- Provider timeout: `504`.
- Provider non-2xx response: `502` with a plain, non-secret message.
- Provider malformed JSON: `502`.
- Empty or oversized generated query/explanation: `502`.
- Generated MRQL fails local parse, validation, or safety lint: `200` with `valid: false`, `query`, `explanation`, and `errors`.

Validation/lint errors should use the existing MRQL validation error shape where possible: `message`, `pos`, and `length`. Lint errors without a precise source span may omit `pos`/`length`.

Frontend behavior:

- Generation has its own loading state separate from query execution.
- Backend/provider errors show a concise alert and do not modify the current query.
- Invalid generated MRQL is shown in the generation panel for inspection, but is not inserted into the editor by default.
- Valid generated MRQL is inserted only if the response is current and the editor did not change after submission. Otherwise the UI offers an explicit "Use generated query" action.
- Applying a generated query clears `result`, execution `error`, `defaultLimitApplied`, and `appliedLimit`.
- Applying a generated query clears `loadedSavedQueryId` and `loadedSavedQueryName`, so users cannot accidentally update a saved query with a generated draft.
- Generation does not update browser URL state or query history; Run keeps owning URL/history updates.
- The feature never auto-runs the generated query.

## 13. Security And Privacy

- The browser never receives or submits the DeepSeek API key.
- Do not log the API key, user prompt, provider request body, provider response body, generated query, generated explanation, or provider error body.
- Logs may include actor identifier, prompt length, model, latency, HTTP status code, error class, and correlation ID.
- Do not include local database vocabulary in the prompt.
- Cap prompt, query, and explanation sizes.
- Use the local parser, validator, and generated-query safety lint as the trust boundary.
- Reuse existing MRQL RBAC by leaving execution on `/v1/mrql`; scoped users remain scoped at execution time.
- Keep `/v1/mrql/generate` out of read-via-POST auth/CSRF exemption. The route must require normal CSRF protection for cookie-authenticated requests and write capability in v1.
- Add simple per-user or per-IP rate limiting before calling DeepSeek, so a single browser session cannot rapidly burn provider quota.

## 14. UX Details

Add a compact generation panel near the existing Query heading:

- Label: "Describe results"
- Multiline input or textarea with placeholder examples.
- Generate button.
- Loading state: "Generating..."
- Explanation panel after success.
- Validation status in the generated panel.
- Stable test IDs for prompt, Generate button, status, error, generated query preview, explanation, and "Use generated query".
- Dedicated `generationError` and `generationStatus` state. Do not reuse `validationError` or execution `error` for generation failures.
- `role="status" aria-live="polite"` for loading, success, and invalid-generation summaries.
- `role="alert"` only for hard provider/config/request errors.
- A real `<label for>` for the prompt textarea, `aria-describedby` for help/error text, `aria-invalid` for prompt validation errors, disabled and `aria-busy` loading state.
- Prompt validation errors focus the textarea. Success, invalid generation, and provider errors should be announced through the generation message container.
- Textarea Enter inserts a newline. Existing CodeMirror Run shortcuts remain scoped to the editor.

The UI should remain secondary to the MRQL editor. It should help draft a query, not replace the editor. The generated query should use the same validation feedback and Run button the page already has.

## 15. Testing

Go API tests:

- Missing API key returns `503`.
- Empty prompt returns `400`.
- Overlong prompt returns `400`.
- Successful provider JSON returns a valid query and explanation.
- Generated invalid MRQL returns `200` with `valid: false` and parse/validation/lint errors.
- Malformed provider JSON returns `502`.
- Provider timeout returns `504`.
- Provider non-2xx errors do not leak provider response body or prompt text.
- Authz test confirms `/v1/mrql/generate` is not read-via-POST and requires write capability in auth-enabled mode.
- CSRF test confirms cookie-authenticated generation requests require a valid CSRF token.
- Fake-provider request-capture test seeds local tags/categories/note types/resource categories/meta keys and asserts none appear in the outbound prompt unless the user typed them.
- Table-driven generated-query lint tests cover huge `LIMIT`/`OFFSET`, `TEXT ~ "*"`, punctuation-only `TEXT`, `TEXT = "x"`, category/noteType/resourceCategory names without numeric IDs, `GROUP BY` without type, and broad valid queries with modest limits.

Provider client tests:

- Use an injected fake HTTP client or `httptest.Server`.
- Confirm Authorization header is sent but never exposed in returned errors.
- Confirm request uses non-stream chat completions, JSON response format, bounded `max_tokens`, and configured model.
- Confirm model defaults and env override are honored.
- Reject empty choices, null content, malformed nested JSON, schema mismatch, and truncating/filtering `finish_reason`.

Frontend/E2E tests:

- Generate button posts prompt to the backend.
- Valid generated query appears in CodeMirror.
- Explanation appears.
- Invalid generation shows validation feedback and does not execute.
- Existing Run behavior still works after generation.
- Stale generation responses do not overwrite newer editor content.
- Loading a saved query, then applying a generated query, hides the Update button and treats the draft as unsaved.
- Applying a generated query clears stale execution results and does not update URL/history until Run.
- Provider/config hard errors leave the current editor content unchanged.
- MRQLPage helper updates: `enterGenerationPrompt()`, `generateMRQL()`, `useGeneratedQuery()`, `getGenerationExplanation()`, and `getGenerationError()`.

Docs/config:

- Update config documentation for `DEEPSEEK_API_KEY`, `DEEPSEEK_MODEL`, and `DEEPSEEK_TIMEOUT`.
- Update MRQL feature docs to say generation is syntax-only and sends the user's prompt to DeepSeek.
- Add an admin/privacy note covering what is sent to DeepSeek, what is never sent, who can use the feature, how to disable it, provider/quota/rate-limit behavior, logging guarantees, and that generated MRQL is validated but still user-reviewed before execution.

## 16. Acceptance Criteria

- `/mrql` users can generate an MRQL draft from a natural-language prompt.
- Generated query is parsed, validated, and linted locally before the API reports it as valid.
- A valid, non-stale generated query is inserted into the editor but not run.
- A short explanation is shown before the user runs it.
- The feature is disabled gracefully when `DEEPSEEK_API_KEY` is absent.
- No local metadata vocabulary is sent to DeepSeek.
- API key remains server-only.
- Generation remains CSRF-protected and is not classified as read-via-POST.
- Generated drafts cannot accidentally update an already-loaded saved query.
- Stale generation responses cannot overwrite newer editor edits.
- Generated drafts do not leave old execution results visible as if they belonged to the new query.
- Tests cover provider success, validation failure, malformed provider responses, and UI insertion.
