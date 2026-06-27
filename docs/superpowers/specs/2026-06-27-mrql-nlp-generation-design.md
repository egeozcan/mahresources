# MRQL Natural-Language Generation

**Date:** 2026-06-27
**Status:** Design approved, ready for implementation planning
**Scope:** Web-only `/mrql` editor feature

## 1. Summary

Add a web-only "Describe results" feature to the existing MRQL editor. The user describes the result set they want, the server calls DeepSeek with MRQL syntax instructions only, validates the generated MRQL locally, and returns the query plus a short explanation. The generated query is inserted into the existing CodeMirror editor, but it is not executed automatically. The user reviews the explanation and presses Run through the existing MRQL execution path.

## 2. Product Decisions

- First surface is only the `/mrql` web page.
- Generation endpoint only generates and validates. It does not execute MRQL.
- The UI shows a short explanation before the user runs the query.
- DeepSeek credentials are configured by server-side environment variables.
- No local app vocabulary is sent to DeepSeek. The prompt includes only MRQL syntax/reference material and the user's natural-language request.
- The generated query is trusted only after passing the existing local MRQL parser and validator.

## 3. Current Context

The repo already has a complete MRQL path:

- Web editor: `templates/mrql.tpl` and `src/components/mrqlEditor.js`
- Execution: `POST /v1/mrql`
- Validation: `POST /v1/mrql/validate`
- Completion: `POST /v1/mrql/complete`
- Saved query endpoints under `/v1/mrql/saved`
- Backend execution and result shaping in `application_context/mrql_context.go`
- Parser/validator/translator in `mrql/`

Auth already treats MRQL execution, validation, completion, and saved-query running as read-via-POST operations for read-only principals. MRQL execution itself applies group-scope RBAC at query time.

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

## 5. Non-Goals

- No CLI command in v1.
- No saved-query NLP creation flow in v1.
- No automatic query execution.
- No schema-aware generation using local metadata.
- No generic LLM provider marketplace or admin UI for secrets.
- No training, caching, or fine-tuning on user prompts.

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
- A request/response handler in `server/api_handlers/mrql_api_handlers.go`.
- A route in `server/routes.go`.
- OpenAPI registration in `server/routes_openapi.go`.
- Auth policy entry in `isReadViaPost`.
- Boot/env config fields on `MahresourcesConfig`.

Frontend pieces:

- Extend `mrqlEditor()` with natural-language prompt state, generation loading/error state, generated explanation, and generated validity.
- Add compact UI to `templates/mrql.tpl` near the query editor.
- Generate inserts text into the existing CodeMirror editor through `setQuery()`.
- Existing validation and execution paths remain unchanged.

## 7. Configuration

Environment variables:

- `DEEPSEEK_API_KEY`: required to enable generation.
- `DEEPSEEK_MODEL`: optional, default `deepseek-v4-pro`.
- `DEEPSEEK_TIMEOUT`: optional, default around 20 seconds.

The runtime settings table should not store the API key in v1 because settings are listable through the admin API/UI and currently return current values. If runtime secret editing is added later, it should use a masked secret-specific storage and view model.

## 8. Prompt Contract

The server prompt should include:

- A concise MRQL syntax reference.
- A small set of examples covering entity type, text search, tags, dates, ordering, limit, and group-by.
- Explicit privacy rule: use only names, IDs, tag values, categories, or metadata keys that appear in the user's request.
- Explicit output rule: return strict JSON only.

Expected provider JSON:

```json
{
  "query": "type = resource AND contentType ~ \"image/*\" LIMIT 50",
  "explanation": "Finds up to 50 image resources."
}
```

The service should ignore any extra fields, reject missing/empty fields, and cap field lengths.

## 9. Data Flow

1. User enters natural language in the `/mrql` page.
2. User clicks Generate.
3. Browser sends `{ "prompt": "..." }` to `/v1/mrql/generate`.
4. Backend validates prompt length and configuration.
5. Backend calls DeepSeek with syntax-only context and JSON output.
6. Backend parses the provider JSON.
7. Backend validates the returned MRQL with `mrql.Parse` and `mrql.Validate`.
8. Backend returns `query`, `explanation`, `valid`, and validation errors.
9. Browser inserts `query` into CodeMirror and shows the explanation.
10. User presses Run if satisfied.

## 10. Error Handling

Backend response policy:

- Missing prompt: `400`.
- Prompt too long: `400`.
- Missing `DEEPSEEK_API_KEY`: `503` with "MRQL generation is not configured."
- Provider network error: `502`.
- Provider timeout: `504`.
- Provider non-2xx response: `502` with a plain, non-secret message.
- Provider malformed JSON: `502`.
- Empty or oversized generated query/explanation: `502`.
- Generated MRQL fails local validation: `200` with `valid: false`, `query`, `explanation`, and `errors`.

Frontend behavior:

- Generation has its own loading state separate from query execution.
- Backend/provider errors show a concise alert and do not modify the current query.
- Invalid generated MRQL is inserted into the editor for inspection, but the explanation panel clearly marks it invalid.
- The feature never auto-runs the generated query.

## 11. Security And Privacy

- The browser never receives or submits the DeepSeek API key.
- Do not log the API key.
- Do not log provider responses at normal log levels.
- Do not include local database vocabulary in the prompt.
- Cap prompt, query, and explanation sizes.
- Use the local parser/validator as the trust boundary.
- Reuse existing MRQL RBAC by leaving execution on `/v1/mrql`; scoped users remain scoped at execution time.
- Add `/v1/mrql/generate` to read-via-POST auth so read-only users can generate drafts in the same broad capability family as validation/completion/execution.

## 12. UX Details

Add a compact generation panel near the existing Query heading:

- Label: "Describe results"
- Multiline input or textarea with placeholder examples.
- Generate button.
- Loading state: "Generating..."
- Explanation panel after success.
- Validation status in the generated panel.

The UI should remain secondary to the MRQL editor. It should help draft a query, not replace the editor. The generated query should use the same validation feedback and Run button the page already has.

## 13. Testing

Go API tests:

- Missing API key returns `503`.
- Empty prompt returns `400`.
- Overlong prompt returns `400`.
- Successful provider JSON returns a valid query and explanation.
- Generated invalid MRQL returns `200` with `valid: false` and validation errors.
- Malformed provider JSON returns `502`.
- Provider timeout returns `504`.

Provider client tests:

- Use an injected fake HTTP client or `httptest.Server`.
- Confirm Authorization header is sent but never exposed in returned errors.
- Confirm request uses non-stream chat completions and JSON response format.
- Confirm model defaults and env override are honored.

Frontend/E2E tests:

- Generate button posts prompt to the backend.
- Generated query appears in CodeMirror.
- Explanation appears.
- Invalid generation shows validation feedback and does not execute.
- Existing Run behavior still works after generation.

Docs/config:

- Update config documentation for `DEEPSEEK_API_KEY`, `DEEPSEEK_MODEL`, and `DEEPSEEK_TIMEOUT`.
- Update MRQL feature docs to say generation is syntax-only and sends the user's prompt to DeepSeek.

## 14. Acceptance Criteria

- `/mrql` users can generate an MRQL draft from a natural-language prompt.
- Generated query is validated locally before the API reports it as valid.
- The query is inserted into the editor but not run.
- A short explanation is shown before the user runs it.
- The feature is disabled gracefully when `DEEPSEEK_API_KEY` is absent.
- No local metadata vocabulary is sent to DeepSeek.
- API key remains server-only.
- Tests cover provider success, validation failure, malformed provider responses, and UI insertion.
