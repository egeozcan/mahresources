---
sidebar_position: 100
---

# Troubleshooting

Common issues and how to resolve them.

## Common Issues

The server prints to stdout and also records warnings and errors in the application log at `/logs`, where entries can be filtered by level and entity type. A single entry is at `/log?id=N`, and `GET /v1/logs` returns the same data as JSON. Retention there is controlled by `-cleanup-logs-days`.

### Server Won't Start

Three startup failures abort the process before it binds a port.

**`panic: stat <working-directory>/templates: no such file or directory`**
- The server loads its templates from `./templates`, resolved against the working directory. Start it from the directory that holds `templates/` and `public/`, not from an arbitrary path.

**`File save path is empty (use -memory-fs for ephemeral mode)`**
- Pass `-file-save-path=/path/to/files`, or run without persistence using `-ephemeral` or `-memory-fs`.

**`please set the DB_TYPE env var to SQLITE or POSTGRES`**
- `-db-type` (or `DB_TYPE`) is unset or not one of the two values. The comparison is case-sensitive, so `sqlite` fails where `SQLITE` works.

### "Database is locked" (SQLite)

This error occurs when multiple processes or connections attempt to write to the SQLite database simultaneously.

**Fix:**
- Set `-max-db-connections=2` to limit concurrent writes
- Check for hung processes that may be holding database locks: `lsof | grep mahresources.db`
- Ensure only one instance is running against the SQLite database
- If running E2E tests, use ephemeral mode to avoid conflicts with a production database

### Thumbnails Not Generating

Video and office document thumbnails require external tools to be configured.

**For video files (FFmpeg):**
- Verify FFmpeg is installed: `ffmpeg -version`
- Set the path explicitly: `-ffmpeg-path=/usr/bin/ffmpeg` or `FFMPEG_PATH=/usr/bin/ffmpeg`
- Check file permissions on the FFmpeg binary

**For office documents (LibreOffice):**
- Verify LibreOffice is installed: `libreoffice --version` or `soffice --version`
- Set the path explicitly: `-libreoffice-path=/usr/bin/libreoffice` or `LIBREOFFICE_PATH=/usr/bin/libreoffice`
- `soffice` or `libreoffice` in PATH is auto-detected; use explicit paths when auto-detection fails

**Video thumbnail timeouts:**
- If video thumbnails fail silently, increase the FFmpeg timeout: `-video-thumb-timeout=60s` (default: `30s`)
- Reduce concurrent video thumbnail generation if the server is resource-constrained: `-video-thumb-concurrency=2` (default: `4`)
- Check the lock timeout for queued video thumbnails: `-video-thumb-lock-timeout=120s` (default: `60s`)

**Thumbnail worker:**
- Verify the thumbnail worker is not disabled: remove `-thumb-worker-disabled` or `THUMB_WORKER_DISABLED=1` if set
- To backfill thumbnails for existing videos that were uploaded before FFmpeg was configured, set `-thumb-backfill` or `THUMB_BACKFILL=1`

**General checks:**
- Ensure the file storage directory has write permissions
- Check application logs for specific error messages

### Slow Startup

Large databases slow startup due to full-text search initialization and version migration.

**Solutions:**
- Skip full-text search initialization: `-skip-fts` or `SKIP_FTS=1` (disables full-text search)
- Skip version migration: `-skip-version-migration` or `SKIP_VERSION_MIGRATION=1` (for databases with millions of resources)
- Use PostgreSQL instead of SQLite for better performance with large datasets

### Database Growing Too Large

Activity log entries accumulate over time and can increase database size.

**Fix:**
- Set `-cleanup-logs-days=90` or `CLEANUP_LOGS_DAYS=90` to delete log entries older than 90 days on startup
- Adjust the value based on your retention needs (0 disables cleanup)

### Upload Failures

Common causes of upload failure:

**Disk space:**
- Check available disk space on the storage volume
- Monitor the file save path directory for capacity

**Reverse proxy limits:**
- If behind nginx, increase `client_max_body_size`:
  ```nginx
  client_max_body_size 100M;
  ```
- If behind Apache, adjust `LimitRequestBody`
- Check proxy timeout settings for large file uploads

**Server size limit:**
- `-max-upload-size` / `MAX_UPLOAD_SIZE` (default 2 GB) bounds one request, and exceeding it is answered with HTTP 400. A native multi-file form post is capped as a whole, while the browser-side bulk upload widget sends one file per request and is therefore capped per file. Raise the limit, or send fewer files per request.

**Duplicate content:**
- HTTP 409 with `a resource with identical content already exists (#N)` means the file's hash is already stored. Open resource `#N` instead of re-uploading.

**Permission issues:**
- Verify the application has write access to the file save path
- Check directory ownership and permissions

### Search Not Working

If search returns no results or behaves unexpectedly:

- **Full-text search disabled:** Check if the application was started with `-skip-fts`.
- **SQLite build flags:** Ensure the binary was built with `--tags 'json1 fts5'` for full search support
- **Index not populated:** For new databases, the full-text search index builds automatically. Large imports take time to index.

### Similar Images Not Appearing

The image similarity feature uses perceptual hashing to find visually similar images.

**Possible causes:**
- **Hash worker disabled:** Check if `-hash-worker-disabled` flag or `HASH_WORKER_DISABLED=1` is set
- **Still processing:** The hash worker processes images in batches. New uploads may take time to be indexed.
- **Threshold too strict:** Adjust `-hash-similarity-threshold` (default: 10, maximum 11; higher values match more, up to that ceiling)

**Check hash worker status:**
- Look for hash worker log messages during startup
- The worker processes batches at intervals configured by `-hash-poll-interval` (default: 1 minute)

## Frequently Asked Questions

### Can multiple users access the same instance?

Yes. With auth off, which is the default, every request runs as an implicit administrator and there is **no user isolation**: all users see and can modify the same data.

With `-auth` enabled, each user logs in and holds one of four roles, and group-limited users and guests are confined to a single Group's subtree across lists, reads, search, MRQL and file serving. See [Authentication & RBAC](./features/authentication.md).

### How do I migrate from SQLite to PostgreSQL?

There is no built-in migration tool. To migrate:

1. Export your data from SQLite (you may need to write custom scripts)
2. Set up a PostgreSQL database
3. Configure Mahresources to use PostgreSQL:
   ```bash
   -db-type=POSTGRES -db-dsn="host=localhost user=mahresources password=secret dbname=mahresources sslmode=disable"
   ```
4. Import your data into PostgreSQL
5. Copy your file storage directory to the new server if needed

Consider using third-party tools like `pgloader` for the data migration.

### What file types are supported?

Any file type can be stored. Special handling is provided for:

- **Images:** JPEG, PNG, GIF, WebP, BMP, TIFF, SVG - thumbnails generated automatically. HEIC and AVIF also work, but require ImageMagick.
- **Videos:** MP4, WebM, MOV, AVI, MKV - thumbnails via FFmpeg
- **Documents:** DOCX, XLSX, PPTX, DOC, XLS, PPT, ODT, ODS, ODP - thumbnails via LibreOffice

Files without special handling are stored and served without processing.

### How much disk space do versions use?

Resource versioning uses content-addressable storage with deduplication. Files with identical SHA1 hashes are stored once, regardless of how many versions reference them. Restoring a previous version creates a new version record but does not duplicate the file on disk.

Disk usage depends on the number of *unique* file contents across all versions. To manage storage:
- Use version cleanup to remove old versions (per-Resource or bulk)
- Run cleanup in dry-run mode first to preview what would be deleted

### Can I run multiple instances?

**With PostgreSQL:** Yes, multiple Mahresources instances can connect to the same PostgreSQL database. Two things need attention across processes. Set `TEMPLATE_SIGNING_KEY` to the same value on every process, or deferred `[lazy]`, `[details]` and `[reload]` blocks fail to open when a reveal lands on a different process than the page render (see [Shortcodes](./features/shortcodes.md)). Upload deduplication is enforced per process, so two processes uploading byte-identical, not-yet-stored content at the same moment can each create a resource.

**With SQLite:** Only one instance should write to a SQLite database at a time. Concurrent writes cause "database is locked" errors. `-db-readonly-dsn` does not change this: it supplies a separate connection used only to run saved raw-SQL queries, so a query cannot write to the database. It does not turn an instance read-only, and it does not make a second SQLite writer safe.

### How do I perform a factory reset?

:::warning Data Loss Warning
A factory reset permanently deletes all data. This action cannot be undone. Always backup your database and files before proceeding.
:::

**To reset completely:**

1. Stop the Mahresources server
2. Delete the database file (SQLite) or drop the database (PostgreSQL)
3. Delete the file storage directory contents
4. Restart Mahresources - it will create a fresh database

**For SQLite:**
```bash
rm /opt/mahresources/data/mahresources.db
rm -rf /opt/mahresources/files/*
```

**For PostgreSQL:**
```sql
DROP DATABASE mahresources;
CREATE DATABASE mahresources;
```

### Plugin Not Loading

If a plugin does not appear in the management UI:

- Verify the plugin directory path: `-plugin-path=./plugins` (default)
- Check that the plugin subdirectory contains a `plugin.lua` file
- Check the application logs for Lua parse errors during discovery
- Confirm plugins are not disabled: remove `-plugins-disabled` flag or `PLUGINS_DISABLED=1`

### Plugin Errors

If an enabled plugin fails to run:

- Check application logs for Lua runtime errors
- Verify plugin settings are configured (required settings block enabling)
- For HTTP-related errors, check that target URLs are reachable from the server
- Each Lua VM is single-threaded; long-running operations in hooks (over 5 seconds) will time out

### Plugin requests are refused with "blocked request"

Plugins may only reach the hosts their manifest declares. A refusal names the host that was asked for; the application log carries the full reason.

- **The host is not in the plugin's `network` list.** Add it, then re-enable the plugin so the new list is consented to. A plugin with no `network` list may reach any public host.
- **The host resolves to a private address.** Private, loopback, link-local, carrier-grade NAT, reserved and multicast addresses are refused for every plugin, including plugins with no manifest at all. To allow one, the plugin must name the **address** -- an IP literal or a CIDR block -- in `network` and declare `allow_private_hosts = true`. A hostname does not work here, however it resolves; see [Plugin Permissions](./features/plugin-permissions.md).
- **The deployment uses an outbound proxy.** Plugin requests deliberately ignore `HTTP_PROXY`/`HTTPS_PROXY`, because through a proxy the address filter would inspect the proxy rather than the request's real destination. Such requests are blocked at the firewall instead.

### A plugin that downloads files stopped working after upgrade

`mah.db.create_resource_from_url` and `mah.db.add_resource_version_from_url` now require the `http` capability in addition to `db:write`, because they open a socket. Add `"http"` to the plugin's `capabilities`, add the host it downloads from to its `network` list, and re-enable it.

The host to add is the one the **files** come from, which is often not the API host. A plugin that calls an API at `api.example` and then downloads results from `cdn.example` needs both.

### A plugin refuses to load: "declares more than was consented to"

The plugin's manifest asks for more than was agreed to when it was enabled -- a new capability, a new host, `allow_private_hosts` turned on, or a `network` list removed (which widens it to any public host). The message names the difference.

Re-enable the plugin from the management UI to consent to the new set. Nothing loads until you do; this is the mechanism that stops an edit to `plugin.lua` from silently widening a grant.

Plugins enabled before this feature existed are granted whatever they declare on their first load after upgrade, once, and prompt normally after that.

### Download Queue Issues

**Stuck downloads:**
- Check network connectivity to the target URL
- Review timeout settings: `-remote-connect-timeout`, `-remote-idle-timeout`, `-remote-overall-timeout`
- Cancel and retry the stuck job

**Queue full (100 jobs):**
- Completed and failed jobs are evicted automatically (oldest first)
- Active and paused jobs are never evicted
- Cancel or remove paused jobs to free queue slots

## Getting Help

If you encounter issues not covered here:

- **GitHub Issues:** Report bugs and request features at [https://github.com/egeozcan/mahresources/issues](https://github.com/egeozcan/mahresources/issues)
- **Search existing issues** before creating a new one -- your problem may already have a solution
- When reporting issues, include:
  - Application version
  - Database type (SQLite/PostgreSQL)
  - Operating system
  - Relevant log output
  - Steps to reproduce the problem
