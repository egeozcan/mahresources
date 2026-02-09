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
- Ensure only one instance of Mahresources is running against the SQLite database
- If running E2E tests, use ephemeral mode to avoid conflicts with a production database

### Thumbnails Not Generating

Thumbnails may fail to generate for videos or office documents if external tools are not properly configured.

**For video files (ffmpeg):**
- Verify ffmpeg is installed: `ffmpeg -version`
- Set the path explicitly: `-ffmpeg-path=/usr/bin/ffmpeg` or `FFMPEG_PATH=/usr/bin/ffmpeg`
- Check file permissions on the ffmpeg binary

**For office documents (LibreOffice):**
- Verify LibreOffice is installed: `libreoffice --version` or `soffice --version`
- Set the path explicitly: `-libreoffice-path=/usr/bin/libreoffice` or `LIBREOFFICE_PATH=/usr/bin/libreoffice`
- The application auto-detects `soffice` or `libreoffice` in PATH, but explicit configuration may be needed

**General checks:**
- Ensure the file storage directory has write permissions
- Check application logs for specific error messages

### Slow Startup

Large databases may cause slow startup times due to Full-Text Search initialization and version migration.

**Solutions:**
- Skip FTS initialization: `-skip-fts` or `SKIP_FTS=1` (note: disables full-text search functionality)
- Skip version migration: `-skip-version-migration` or `SKIP_VERSION_MIGRATION=1` (for databases with millions of resources)
- Use PostgreSQL instead of SQLite for better performance with large datasets

### Upload Failures

File uploads may fail for several reasons:

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

- **FTS may be skipped:** Check if the application was started with `-skip-fts`. Full-text search requires FTS to be enabled.
- **SQLite build flags:** Ensure the binary was built with `--tags 'json1 fts5'` for full search support
- **Index not populated:** For new databases, the FTS index builds automatically. Large imports may take time to index.

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

### Can multiple users access Mahresources?

Yes, multiple users can access the same Mahresources instance simultaneously. However, there is **no user isolation** - all users see and can modify the same data. Mahresources is designed for personal use or trusted environments, not multi-tenant deployments.

### How do I migrate from SQLite to PostgreSQL?

Mahresources does not provide a built-in migration tool. To migrate:

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

Mahresources can store any file type. Special handling is provided for:

- **Images:** JPEG, PNG, GIF, WebP, BMP - thumbnails generated automatically
- **Videos:** MP4, WebM, MOV, AVI, MKV - thumbnails via ffmpeg
- **Documents:** PDF, DOCX, XLSX, PPTX, ODT, ODS, ODP - thumbnails via LibreOffice
- **Audio:** MP3, WAV, FLAC, OGG - metadata extraction

Files without special handling are stored and served without processing.

### How much disk space do versions use?

Resource versioning stores complete copies of each file version. Disk usage depends on:

- How frequently files are updated
- The size of updated files
- Number of versions retained

To manage disk usage:
- Periodically review and delete old versions
- Consider the trade-off between version history and storage costs

### Can I run multiple instances?

**With PostgreSQL:** Yes, multiple Mahresources instances can connect to the same PostgreSQL database.

**With SQLite:** Only one instance should write to a SQLite database at a time. SQLite allows only one writer, and concurrent writes will cause "database is locked" errors. You can run a read-only instance with `-db-readonly-dsn` for queries while another instance handles writes.

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

## Getting Help

If you encounter issues not covered here:

- **GitHub Issues:** Report bugs and request features at [https://github.com/egeozcan/mahresources/issues](https://github.com/egeozcan/mahresources/issues)
- **Search existing issues** before creating a new one - your problem may already have a solution
- When reporting issues, include:
  - Mahresources version
  - Database type (SQLite/PostgreSQL)
  - Operating system
  - Relevant log output
  - Steps to reproduce the problem
