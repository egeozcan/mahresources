---
sidebar_position: 5
---

# Runtime Settings

Most configuration flags bind once at startup. A curated subset can be
overridden at runtime via the `/admin/settings` page, the `mr admin settings`
CLI, or the `/v1/admin/settings` HTTP API -- no restart required.

## How precedence works

1. Boot flag / env var supplies the initial value.
2. If the `runtime_settings` table has a row for the key, that override wins.
3. When an override differs from the boot value (flag, env var, or built-in
   default), one WARN line is logged at startup. Secret settings report status
   without printing either value.

Reset via the UI (Reset button), CLI (`mr admin settings reset <key>`), or API
(`DELETE /v1/admin/settings/<key>`) removes the override and returns to the
boot value.

## Runtime-editable settings

| Key | Type | Bounds | Boot flag | Takes effect |
| --- | --- | --- | --- | --- |
| `max_upload_size` | int64 (bytes) | 1 KiB–1 TiB; 0 = unlimited | `-max-upload-size` | next upload request |
| `upload_concurrency` | int | 1–16 | (runtime only) | next page render |
| `upload_widget_file_threshold` | int | 1–10000 | (runtime only) | next page render |
| `upload_widget_size_threshold` | int64 (bytes) | 1 MiB–1 TiB | (runtime only) | next page render |
| `max_import_size` | int64 (bytes) | 1 MiB–1 TiB | `-max-import-size` | next import parse |
| `mrql_default_limit` | int | 1–100000 | `-mrql-default-limit` | next MRQL query |
| `mrql_page_query_budget` | int | 0–100000; 0 disables | `-mrql-page-query-budget` | next page render |
| `mrql_query_timeout` | duration | 100ms–5m | `-mrql-query-timeout` | next MRQL query |
| `export_retention` | duration | 1m–30d | `-export-retention` | next sweep + UI disclosure |
| `remote_connect_timeout` | duration | 1s–10m | `-remote-connect-timeout` | next remote download |
| `remote_idle_timeout` | duration | 1s–1h | `-remote-idle-timeout` | next remote download |
| `remote_overall_timeout` | duration | 10s–24h | `-remote-overall-timeout` | next remote download |
| `remote_user_agent` | string | at most 8192 bytes, no control characters; empty = browser-like default | `-remote-user-agent` | next remote download |
| `download_failed_retention` | duration | 1h–365d | `-download-failed-retention` | next history sweep |
| `download_history_retention` | duration | 1h–365d | `-download-history-retention` | next history sweep |
| `download_cockpit_limit` | int | 1–200 | `-download-cockpit-limit` | next page load (Jobs drawer finished rows) |
| `share_public_url` | string (http/https URL) | absolute http/https URL; empty = relative `/s/<token>` only | `-share-public-url` | next share link render |
| `docs_site_base_url` | string (http/https URL) | absolute; non-empty host | `-docs-site-base-url` | next page render |
| `docs_links_disabled` | int | 0–1; 1 hides links | `-docs-links-disabled` | next page render |
| `hash_similarity_threshold` | int | 0–11 (v2 pairs stored up to distance 11) | `-hash-similarity-threshold` | next hash comparison |
| `hash_ahash_threshold` | uint64 | 0–64; 0 disables | `-hash-ahash-threshold` | next hash comparison |
| `hash_backfill_paused` | int | 0–1; 1 pauses the v2 backfill | (runtime only) | next backfill cycle |
| `job_replay_retention` | duration | 1h–365d | (runtime only) | next Job replay sweep |
| `job_history_retention` | duration | 1h–365d | `-job-history-retention` | next Job history sweep |
| `job_attention_retention` | duration | 1h–365d | `-job-attention-retention` | next Job history sweep |
| `job_pin_limit` | int | 1–10000 | `-job-pin-limit` | next pin |

## AI providers, models and credentials

The **AI generation** section at `/admin/settings` configures DeepSeek,
OpenRouter, OpenAI and Anthropic. Save one credential per provider, then select
the provider and model separately for **MRQL generation** and **Template
generation**. Changes apply to the next generation operation without a restart,
even if the server started without a key, and survive a restart. Configuration
is shared by the deployment and requires an administrator; generation keeps its
existing feature permissions.

Enter the provider's model ID directly. OpenRouter IDs include their namespace
(for example, `openai/<model>`). Saving does not contact the provider or verify
model access. Each provider keeps its own saved models, so switching providers
cannot accidentally inherit another provider's model. A provider needs both a
key and a default model; otherwise generation returns "not configured".

Credential fields are write-only and stay empty after loading or saving.
**Replace** stores a new key; leaving the field untouched preserves it.
**Clear** saves an empty override, disabling the key even when an environment
fallback exists. **Reset** removes the override and restores that fallback.
The status reports whether a key is configured, whether a boot key exists and
whether an override is present. Keys and key fragments are never returned.

Stored credential overrides are plaintext in the database and its backups,
readable by database administrators and the server. The settings API/UI, CLI,
audit records and SQL/application diagnostics redact them. Environment fallbacks
are `DEEPSEEK_API_KEY`, `OPENROUTER_API_KEY`, `OPENAI_API_KEY` and
`ANTHROPIC_API_KEY`.

These keys also work with the existing settings API and CLI. Replace
`<provider>` with `deepseek`, `openrouter`, `openai` or `anthropic`, `<feature>`
with `mrql` or `template`, and `<kind>` with `slot`, `pair`, `plan`, `bundle` or
`metaschema`:

| Key | Value |
| --- | --- |
| `ai_<provider>_api_key` | Secret string, at most 8192 bytes, no whitespace/control characters; empty clears |
| `ai_mrql_provider`, `ai_template_provider` | One of the four provider IDs; default `deepseek` |
| `ai_<feature>_<provider>_model` | Model ID, at most 256 bytes, no whitespace/control characters |
| `ai_<feature>_<provider>_max_tokens` | 1–65536 output/completion tokens; `0` inherits |
| `ai_<feature>_<provider>_timeout` | Positive Go duration up to `10m`; `0s` inherits |
| `ai_template_<provider>_<kind>_model` | Optional kind-specific model; empty inherits the template default |
| `ai_template_<provider>_<kind>_max_tokens` | Optional kind-specific token cap; `0` inherits |
| `ai_template_<provider>_<kind>_timeout` | Optional kind-specific timeout; `0s` inherits |

The advanced controls show the effective inherited profiles. An explicit kind
setting wins over a feature/provider setting, which wins over shipped defaults
or DeepSeek boot configuration. OpenRouter, OpenAI and Anthropic default to
8192 tokens and 60s per MRQL/template call, with 16384 tokens and 120s for a
single-call bundle. Their whole-template planning/fan-out deadline is 120s.
Reasoning uses the model's defaults and counts against the completion budget.
DeepSeek keeps its existing models, thinking, budgets and timeout defaults;
see [Template generation](./advanced.md#template-generation).

Each operation keeps one settings snapshot through its calls and repairs.
Changes do not alter a running generation. A template provider, model or public
option change after the disclosure dialog requires reviewing again before Send.
Rotating a key alone does not change the public disclosure hash. The same eight
template provider slots and existing rate limits apply across configuration
changes.

**Reset** for a generation feature atomically removes its provider selection and
all its provider-specific model/budget/timeout overrides, restoring boot defaults.
It preserves the credentials. A single-key CLI/API reset only resets that key.

`PUT /v1/admin/settings` saves an atomic batch of string values and resets:

```json
{
  "values": {
    "ai_mrql_provider": "openai",
    "ai_mrql_openai_model": "<your-model-id>"
  },
  "reset": [],
  "reason": "Use the selected OpenAI model for MRQL"
}
```

Omitted keys are preserved. The response is the full sanitized settings list.
Invalid entries or database failures save nothing. A key cannot appear in both
`values` and `reset`. `GET /v1/admin/settings/ai-profiles` returns the selected
providers and effective non-secret call profiles. Settings are cached per server
process; a write does not automatically refresh another running process.

## Metadata index build status

Indexed keys are configured in each **resource category**, **group category**, or
**note type** editor, alongside its metadata schema. See
[indexed metadata keys](../features/meta-schemas.md#indexed-metadata-keys).

The **Metadata index builds** section on this page shows background build status
and errors. `GET /v1/admin/settings/metadata-index-status` exposes the same status
for administrators. Saved category changes are normally picked up within five
seconds. Failed builds retry after a minute; each reconciliation has a 30-minute
budget. PostgreSQL concurrent builds interrupted by shutdown are repaired on
retry or restart.

## Bulk resource uploads

`max_upload_size` bounds one **request** body. That distinction matters on the
create-resource page, which has two ways of submitting:

- **Native post** - the browser sends every selected file in one multipart body,
  so `max_upload_size` caps the batch as a whole.
- **Client-side widget** - above `upload_widget_file_threshold` files *or*
  `upload_widget_size_threshold` bytes, the page sends one request per file,
  `upload_concurrency` at a time, with a progress bar and per-file errors. Each
  file is then capped individually, and the widget refuses an oversized file in
  the browser rather than spending the transfer to be rejected.

All three are read when the create-resource page renders and embedded in the
form, so a change applies to the next page load rather than to an upload already
in flight.

Set `upload_widget_file_threshold` to `1` to put every multi-file selection
through the widget. SQLite has exactly one writer, so `upload_concurrency` above
about 3 buys little there; Postgres tolerates more.

## Audit trail

Every change writes a row to `log_entries` with `entity_type=runtime_setting`,
the key as `entity_name`, old→new values in `message`, and the request IP in
`ip_address`. Secret entries record redacted values. Visible at `/logs` (filter entity type `runtime_setting`), or via
`GET /v1/logs`.

## CLI reference

See [`mr admin settings`](../cli/admin/settings/index.md).

<!-- SCREENSHOT: /admin/settings -- regenerate via retake-screenshots skill -->
