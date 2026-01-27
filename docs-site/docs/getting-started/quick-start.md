---
sidebar_position: 2
---

# Quick Start

Get Mahresources running in under a minute.

## Ephemeral Mode (Try It Out)

The fastest way to explore Mahresources is ephemeral mode, which uses in-memory storage. No files are written to disk, and all data is lost when you stop the server.

```bash
./mahresources -ephemeral -bind-address=:8080
```

Open your browser to [http://localhost:8080](http://localhost:8080) and start exploring.

## Your First Upload

1. Navigate to **Resources** in the sidebar
2. Click **Create Resource**
3. Drag and drop a file or click to select one
4. Add a name and optional description
5. Click **Save**

Your file is now stored and indexed. If it's an image, a thumbnail is automatically generated.

## Your First Note

1. Navigate to **Notes** in the sidebar
2. Click **Create Note**
3. Enter a title and content
4. Select a Note Type (or create one first under **Note Types**)
5. Click **Save**

## Persistent Setup

For real use, you'll want data to persist between restarts.

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

Instead of command-line flags, you can use environment variables or a `.env` file:

```bash
# .env file
DB_TYPE=SQLITE
DB_DSN=./mahresources.db
FILE_SAVE_PATH=./files
BIND_ADDRESS=:8080
```

Then simply run:
```bash
./mahresources
```

## Next Steps

Now that Mahresources is running, follow the [First Steps](./first-steps) guide to learn the basics of organizing your content.
