---
sidebar_position: 2
---

# Quick Start

Get up and running in under a minute.

## Ephemeral Mode (Try It Out)

Ephemeral mode uses transient storage. Uploaded files stay in memory and are lost when the server stops, and the SQLite database is a scratch file at `/tmp/mahresources_ephemeral_<pid>.db` that is never read again after the process exits (it is deleted at the start of the next run that reuses that PID, not at shutdown).

```bash
./mahresources -ephemeral -bind-address=:8181
```

`-bind-address` (or `BIND_ADDRESS`) has no default: without it the server listens on port 80, which normally fails for a non-root user.

Open [http://localhost:8181](http://localhost:8181) in your browser.

## Your First Upload

1. Navigate to **Resources** in the top navigation bar
2. Click the **Create** button
3. Select a file to upload
4. Add a name and optional description
5. Click **Save**

The file is stored and indexed. Images get automatic thumbnails.

## Your First Note

1. Navigate to **Notes** in the top navigation bar
2. Click the **Create** button
3. Enter a title and text
4. Optionally select a Note Type (you can create one under **Note Types** in the Admin menu)
5. Click **Save**

## Persistent Setup

To keep data between restarts, configure a database and file path.

### SQLite (Recommended for Getting Started)

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./mahresources.db \
  -file-save-path=./files \
  -bind-address=:8181
```

This creates:
- `mahresources.db` - SQLite database for all metadata
- `files/` - Directory where uploaded files are stored

### PostgreSQL (For Larger Deployments)

```bash
./mahresources \
  -db-type=POSTGRES \
  -db-dsn="host=localhost user=mah password=secret dbname=mahresources" \
  -file-save-path=/var/lib/mahresources/files \
  -bind-address=:8181
```

## Environment Variables

All flags can also be set as environment variables in a `.env` file:

```bash
# .env file
DB_TYPE=SQLITE
DB_DSN=./mahresources.db
FILE_SAVE_PATH=./files
BIND_ADDRESS=:8181
```

Then run:
```bash
./mahresources
```

## Next Steps

Next: [First Steps](./first-steps) to learn how to organize content.
