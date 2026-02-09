---
sidebar_position: 2
---

# Quick Start

Get Mahresources running in under a minute.

## Ephemeral Mode (Try It Out)

Ephemeral mode uses in-memory storage -- nothing is written to disk, and all data is lost when the server stops.

```bash
./mahresources -ephemeral -bind-address=:8080
```

Open [http://localhost:8080](http://localhost:8080) in your browser.

## Your First Upload

1. Navigate to **Resources** in the top navigation bar
2. Click the **Create** button
3. Drag and drop a file or click to select one
4. Add a name and optional description
5. Click **Save**

Mahresources stores and indexes the file. Images get automatic thumbnails.

## Your First Note

1. Navigate to **Notes** in the top navigation bar
2. Click the **Create** button
3. Enter a title and content
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
  -bind-address=:8080
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
  -bind-address=:8080
```

## Environment Variables

All flags can also be set as environment variables in a `.env` file:

```bash
# .env file
DB_TYPE=SQLITE
DB_DSN=./mahresources.db
FILE_SAVE_PATH=./files
BIND_ADDRESS=:8080
```

Then run:
```bash
./mahresources
```

## Next Steps

Next: [First Steps](./first-steps) -- learn how to organize content.
