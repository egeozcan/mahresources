---
sidebar_position: 100
---

# Troubleshooting

Common issues and how to resolve them.

## Common Issues

### "Database is locked" (SQLite)

This error occurs when multiple processes or connections attempt to write to the SQLite database simultaneously.

**Solutions:**
- Reduce the number of database connections using `-max-db-connections=2`
- Check for hung processes that may be holding database locks: `lsof | grep your-database.db`
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

**General checks:**
- Ensure the file storage directory has write permissions
- Check application logs for specific error messages

### Slow Startup

Large databases slow startup due to full-text search initialization and version migration.

**Solutions:**
- Skip full-text search initialization: `-skip-fts` or `SKIP_FTS=1` (disables full-text search)
- Skip version migration: `-skip-version-migration` or `SKIP_VERSION_MIGRATION=1` (for databases with millions of resources)
- Use PostgreSQL instead of SQLite for better performance with large datasets

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
- **Threshold too strict:** Adjust `-hash-similarity-threshold` (default: 10, higher = more matches)

**Check hash worker status:**
- Look for hash worker log messages during startup
- The worker processes batches at intervals configured by `-hash-poll-interval` (default: 1 minute)

## Frequently Asked Questions

### Can multiple users access the same instance?

Yes, multiple users can connect simultaneously. However, there is **no user isolation** -- all users see and can modify the same data. The application is designed for personal use or trusted environments, not multi-tenant deployments.

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

- **Images:** JPEG, PNG, GIF, WebP, BMP - thumbnails generated automatically
- **Videos:** MP4, WebM, MOV, AVI, MKV - thumbnails via FFmpeg
- **Documents:** PDF, DOCX, XLSX, PPTX, ODT, ODS, ODP - thumbnails via LibreOffice

Files without special handling are stored and served without processing.

### How much disk space do versions use?

Resource versioning uses content-addressable storage with deduplication. Files with identical SHA1 hashes are stored once, regardless of how many versions reference them. Restoring a previous version creates a new version record but does not duplicate the file on disk.

Disk usage depends on the number of *unique* file contents across all versions. To manage storage:
- Use version cleanup to remove old versions (per-Resource or bulk)
- Run cleanup in dry-run mode first to preview what would be deleted

### Can I run multiple instances?

**With PostgreSQL:** Yes, multiple Mahresources instances can connect to the same PostgreSQL database.

**With SQLite:** Only one instance should write to a SQLite database at a time. Concurrent writes cause "database is locked" errors. You can run a read-only instance with `-db-readonly-dsn` for queries while another instance handles writes.

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
rm /path/to/your/database.db
rm -rf /path/to/your/files/*
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
