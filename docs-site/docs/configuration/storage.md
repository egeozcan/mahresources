---
sidebar_position: 3
---

# Storage Configuration

Mahresources stores uploaded files on the filesystem. This page covers primary storage, alternative filesystems, and advanced features like copy-on-write.

## Primary Storage

The main storage location is configured with `-file-save-path`:

```bash
./mahresources -file-save-path=/var/lib/mahresources/files -db-type=SQLITE -db-dsn=./db.sqlite
```

Or with environment variables:

```bash
FILE_SAVE_PATH=/var/lib/mahresources/files
```

:::warning Required Setting
`-file-save-path` is required unless using `-memory-fs` or `-ephemeral` mode.
:::

## In-Memory Filesystem

For testing or ephemeral usage, store files in memory:

```bash
./mahresources -memory-fs -db-type=SQLITE -db-dsn=./test.db
```

All files are lost when the server stops.

### Ephemeral Mode

Combine in-memory database and filesystem:

```bash
./mahresources -ephemeral
```

This is equivalent to `-memory-db -memory-fs` and is useful for:
- Running E2E tests
- Quick demos
- Development without leaving artifacts

## Alternative Filesystems

Mahresources supports multiple storage locations. This is useful for:
- Spreading storage across multiple drives
- Accessing legacy file locations
- Organizing files by type or date

### Using Command-Line Flags

Use the `-alt-fs` flag with `key:path` format:

```bash
./mahresources \
  -file-save-path=/data/primary \
  -alt-fs=archive:/mnt/archive \
  -alt-fs=media:/mnt/media \
  -db-type=SQLITE -db-dsn=./db.sqlite
```

### Using Environment Variables

```bash
FILE_SAVE_PATH=/data/primary
FILE_ALT_COUNT=2
FILE_ALT_NAME_1=archive
FILE_ALT_PATH_1=/mnt/archive
FILE_ALT_NAME_2=media
FILE_ALT_PATH_2=/mnt/media
```

Resources can reference files in any configured filesystem by their key.

## Seed Filesystem (Copy-on-Write)

The seed filesystem feature creates a copy-on-write overlay, where:
- Reads come from the seed directory (base layer)
- Writes go to the primary storage (overlay layer)

This is useful for:
- Testing with production data without modifying it
- Creating demo environments
- Sharing a read-only base across multiple instances

### Ephemeral with Seed

All changes stay in memory:

```bash
./mahresources \
  -ephemeral \
  -seed-db=./production.db \
  -seed-fs=/mnt/production-files
```

### Persistent Overlay

Changes are written to disk but the seed remains untouched:

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./overlay.db \
  -seed-fs=/mnt/shared-files \
  -file-save-path=/data/changes
```

In this setup:
- Existing files are read from `/mnt/shared-files`
- New or modified files are written to `/data/changes`
- The seed directory is never modified

### Memory Overlay

Writes go to memory, seed stays on disk:

```bash
./mahresources \
  -memory-fs \
  -seed-fs=/mnt/production-files \
  -db-type=SQLITE \
  -db-dsn=./test.db
```

## Configuration Reference

| Flag | Env Variable | Description |
|------|--------------|-------------|
| `-file-save-path` | `FILE_SAVE_PATH` | Main file storage directory |
| `-memory-fs` | `MEMORY_FS=1` | Use in-memory filesystem |
| `-ephemeral` | `EPHEMERAL=1` | Memory DB + memory FS |
| `-seed-fs` | `SEED_FS` | Read-only base directory for copy-on-write |
| `-alt-fs` | `FILE_ALT_*` | Alternative filesystems |

### Alternative Filesystem Environment Variables

| Variable | Description |
|----------|-------------|
| `FILE_ALT_COUNT` | Number of alternative filesystems |
| `FILE_ALT_NAME_N` | Name/key for filesystem N |
| `FILE_ALT_PATH_N` | Path for filesystem N |

## Storage Layout

Mahresources organizes files by their hash to prevent duplicates and enable content-addressable storage:

```
files/
  ab/
    cd/
      abcd1234...  # file stored by content hash
  12/
    34/
      12345678...
```

This structure:
- Prevents duplicate storage of identical files
- Distributes files across directories for filesystem efficiency
- Enables fast lookup by content hash
