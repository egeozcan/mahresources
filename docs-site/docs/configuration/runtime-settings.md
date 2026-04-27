---
sidebar_position: 5
---

# Runtime Settings

Most configuration flags bind once at startup. A curated subset can be
overridden at runtime via the `/admin/settings` page, the `mr admin settings`
CLI, or the `/v1/admin/settings` HTTP API — no restart required.

## How precedence works

1. Boot flag / env var supplies the initial value.
2. If the `runtime_settings` table has a row for the key, that override wins.
3. When a flag is set *and* an override differs from it, one WARN line is
   logged at startup so operators are not silently surprised.

Reset via the UI (Reset button), CLI (`mr admin settings reset <key>`), or API
(`DELETE /v1/admin/settings/<key>`) removes the override and returns to the
boot value.

## Runtime-editable settings

| Key | Type | Bounds | Boot flag | Takes effect |
| --- | --- | --- | --- | --- |
| `max_upload_size` | int64 (bytes) | 1 KiB–1 TiB; 0 = unlimited | `-max-upload-size` | next upload request |
| `max_import_size` | int64 (bytes) | 1 MiB–1 TiB | `-max-import-size` | next import parse |
| `mrql_default_limit` | int | 1–100000 | `-mrql-default-limit` | next MRQL query |
| `mrql_query_timeout` | duration | 100ms–5m | `-mrql-query-timeout` | next MRQL query |
| `export_retention` | duration | 1m–30d | `-export-retention` | next sweep + UI disclosure |
| `remote_connect_timeout` | duration | 1s–10m | `-remote-connect-timeout` | next remote download |
| `remote_idle_timeout` | duration | 1s–1h | `-remote-idle-timeout` | next remote download |
| `remote_overall_timeout` | duration | 10s–24h | `-remote-overall-timeout` | next remote download |
| `share_public_url` | string (http/https URL) | absolute; non-empty host | `-share-public-url` | next share link render |
| `hash_similarity_threshold` | int | 0–64 | `-hash-similarity-threshold` | next hash comparison |
| `hash_ahash_threshold` | uint64 | 0–64; 0 disables | `-hash-ahash-threshold` | next hash comparison |

## Audit trail

Every change writes a row to `log_entries` with `entity_type=runtime_setting`,
the key as `entity_name`, old→new values in `message`, and the request IP in
`ip_address`. Visible in the admin log view at `/admin/overview`.

## CLI reference

See [`mr admin settings`](../cli/admin/settings/index.md).

<!-- SCREENSHOT: /admin/settings — regenerate via retake-screenshots skill -->
