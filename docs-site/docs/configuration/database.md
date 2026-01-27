---
sidebar_position: 2
---

# Database Configuration

Mahresources supports SQLite and PostgreSQL databases. This page covers setup and configuration for both.

## SQLite

SQLite is the simplest option and works well for most deployments, even with millions of resources.

### Basic Setup

```bash
./mahresources -db-type=SQLITE -db-dsn=./mahresources.db -file-save-path=./files
```

Or with environment variables:

```bash
DB_TYPE=SQLITE
DB_DSN=./mahresources.db
```

### Build Requirements

When building from source, SQLite requires special build tags for full functionality:

```bash
go build --tags 'json1 fts5'
```

| Tag | Purpose |
|-----|---------|
| `json1` | Enables JSON query support for metadata fields |
| `fts5` | Enables Full-Text Search for notes and resources |

:::warning
Pre-built binaries include these tags. Only relevant when building from source.
:::

### In-Memory Database

For testing or ephemeral usage:

```bash
./mahresources -memory-db -file-save-path=./files
```

Or use the combined ephemeral flag:

```bash
./mahresources -ephemeral
```

### Seeding from Existing Database

Start with a copy of an existing database (useful for testing or demos):

```bash
./mahresources -memory-db -seed-db=./production.db -file-save-path=./files
```

Changes are made to the in-memory copy and lost when the server stops.

### Connection Pool Limits

For concurrent access scenarios (like parallel E2E tests), limit connections to reduce lock contention:

```bash
./mahresources -db-type=SQLITE -db-dsn=./test.db -max-db-connections=2
```

## PostgreSQL

PostgreSQL is recommended for multi-user deployments or when you need advanced database features.

### Basic Setup

```bash
./mahresources \
  -db-type=POSTGRES \
  -db-dsn="host=localhost port=5432 user=mahresources password=secret dbname=mahresources sslmode=disable" \
  -file-save-path=./files
```

The DSN follows standard PostgreSQL connection string format.

### With SSL

```bash
DB_TYPE=POSTGRES
DB_DSN="host=db.example.com port=5432 user=app password=secret dbname=mahresources sslmode=require"
```

### Read Replica

For high-read workloads, configure a read-only connection to a replica:

```bash
./mahresources \
  -db-type=POSTGRES \
  -db-dsn="host=primary.db user=app password=secret dbname=mahresources" \
  -db-readonly-dsn="host=replica.db user=app password=secret dbname=mahresources"
```

Read operations will use the replica, reducing load on the primary.

## Database Logging

Control database query logging with `-db-log-file`:

| Value | Behavior |
|-------|----------|
| `STDOUT` | Log queries to standard output |
| *(empty)* | Disable query logging |
| `/path/to/file` | Log queries to specified file |

```bash
# Log to stdout (useful for debugging)
./mahresources -db-type=SQLITE -db-dsn=./test.db -db-log-file=STDOUT

# Log to file
./mahresources -db-type=SQLITE -db-dsn=./test.db -db-log-file=/var/log/mahresources-db.log
```

## Startup Optimizations

For large databases with millions of resources, certain startup operations can be slow.

### Skip Full-Text Search Initialization

```bash
./mahresources -skip-fts -db-type=SQLITE -db-dsn=./large.db -file-save-path=./files
```

This skips FTS index creation/update at startup. Useful if you do not need text search.

### Skip Version Migration

```bash
./mahresources -skip-version-migration -db-type=SQLITE -db-dsn=./large.db -file-save-path=./files
```

Skips the resource version migration that runs at startup. Use this for faster startup on large databases after the initial migration has completed.

## Configuration Reference

| Flag | Env Variable | Description |
|------|--------------|-------------|
| `-db-type` | `DB_TYPE` | Database type: `SQLITE` or `POSTGRES` |
| `-db-dsn` | `DB_DSN` | Database connection string |
| `-db-readonly-dsn` | `DB_READONLY_DSN` | Read-only connection (PostgreSQL) |
| `-db-log-file` | `DB_LOG_FILE` | Query log destination |
| `-memory-db` | `MEMORY_DB=1` | Use in-memory SQLite |
| `-seed-db` | `SEED_DB` | SQLite file to seed memory-db |
| `-max-db-connections` | `MAX_DB_CONNECTIONS` | Connection pool size limit |
| `-skip-fts` | `SKIP_FTS=1` | Skip FTS initialization |
| `-skip-version-migration` | `SKIP_VERSION_MIGRATION=1` | Skip version migration |
