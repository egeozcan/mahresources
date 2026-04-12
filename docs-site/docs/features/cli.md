---
sidebar_position: 21
title: CLI (mr)
---

# CLI (mr)

`mr` is a command-line client for the mahresources API. It covers all entity types with CRUD operations, bulk actions, file upload/download, version management, search, and plugin control.

## Installation

Build from source (requires Go):

```bash
npm run build-cli
```

This produces the `mr` binary in the project root. Move it to a directory in your `PATH` to use it globally.

## Server Connection

By default, `mr` connects to `http://localhost:8181`. Override with:

```bash
# Flag
mr --server http://myserver:9090 resources list

# Environment variable
export MAHRESOURCES_URL=http://myserver:9090
mr resources list
```

Flag takes precedence over the environment variable.

## Global Flags

| Flag | Description |
|------|-------------|
| `--server` | Server URL (default `http://localhost:8181`, env: `MAHRESOURCES_URL`) |
| `--json` | Output raw JSON instead of formatted tables |
| `--no-header` | Omit table column headers |
| `--quiet` | Only output IDs |
| `--page` | Page number for list commands (default page size: 50) |

## Output Modes

By default, list commands display aligned tables. Single-entity commands display key-value pairs.

```bash
# Table output
mr tags list

# Raw JSON
mr tags list --json

# IDs only (useful for scripting)
mr tags list --quiet

# No column headers
mr tags list --no-header

# Pagination
mr resources list --page 3
```

## Commands

Commands follow a consistent pattern: singular commands (`tag`, `resource`, `note`) operate on individual entities, plural commands (`tags`, `resources`, `notes`) handle lists and bulk operations.

### tag / tags

```bash
# Get a tag
mr tag get <id>

# Create
mr tag create --name "Photography" --description "Photo-related"

# Edit
mr tag edit-name <id> "New Name"
mr tag edit-description <id> "New description"

# Delete
mr tag delete <id>

# List (with filters)
mr tags list --name "Photo" --description "keyword"

# Merge tags (move all associations from losers to winner, delete losers)
mr tags merge --winner 1 --losers 2,3,4

# Bulk delete
mr tags delete --ids 5,6,7
```

### category / categories

Group categories. Same CRUD pattern as tags, plus custom template fields.

```bash
mr category get <id>
mr category create --name "People" --description "Person groups"
mr category create --name "Projects" --custom-header "<h2>Projects</h2>" --meta-schema '{"type":"object"}'
mr category edit-name <id> "New Name"
mr category edit-description <id> "New description"
mr category delete <id>
mr categories list --name "filter"
```

### resource-category / resource-categories

Resource categories. Same CRUD pattern as group categories.

```bash
mr resource-category get <id>
mr resource-category create --name "Screenshots" --description "Screen captures"
mr resource-category edit-name <id> "New Name"
mr resource-category delete <id>
mr resource-categories list
```

### note-type / note-types

```bash
mr note-type get <id>
mr note-type create --name "Meeting Notes" --custom-header "<h2>Meetings</h2>"
mr note-type edit --id 1 --name "Updated Name" --custom-sidebar "<p>sidebar</p>"
mr note-type edit-name <id> "New Name"
mr note-type edit-description <id> "New description"
mr note-type delete <id>
mr note-types list --name "filter"
```

### resource / resources

```bash
# Get a resource
mr resource get <id>

# Upload a file
mr resource upload photo.jpg --name "Vacation Photo" --owner-id 1 --meta '{"location":"beach"}'

# Download
mr resource download <id> -o photo.jpg

# Download a scaled thumbnail
mr resource preview <id> --width 200 --height 200 -o thumb.jpg

# Create from URL (server fetches the file)
mr resource from-url --url "https://example.com/image.png" --name "Remote Image" --tags 1,2

# Create from a path on the server filesystem
mr resource from-local --path /data/files/document.pdf --name "Local Doc"

# Edit metadata
mr resource edit <id> --name "New Name" --tags 1,2,3 --groups 4,5 --meta '{"key":"value"}'

# Inline edits
mr resource edit-name <id> "New Name"
mr resource edit-description <id> "New description"

# Rotate an image
mr resource rotate <id> --degrees 90

# Recalculate dimensions
mr resource recalculate-dimensions <id>

# Delete
mr resource delete <id>
```

#### Resource Versions

```bash
# List versions
mr resource versions <resource-id>

# Get a specific version
mr resource version <version-id>

# Upload a new version
mr resource version-upload <resource-id> updated-file.jpg --comment "Higher resolution"

# Download a specific version
mr resource version-download <version-id> -o old-version.jpg

# Restore a previous version
mr resource version-restore --resource-id 1 --version-id 3 --comment "Reverting"

# Delete a version
mr resource version-delete --resource-id 1 --version-id 2

# Compare two versions
mr resource versions-compare <resource-id> --v1 1 --v2 2

# Clean up old versions
mr resource versions-cleanup <resource-id> --keep 5 --dry-run
mr resource versions-cleanup <resource-id> --older-than-days 90
```

#### Bulk Resource Operations

```bash
# List with filters
mr resources list --content-type image/png --owner-id 1 --tags 1,2 --groups 3
mr resources list --created-after 2025-01-01 --min-width 1920 --sort-by "created_at desc"

# Bulk tag operations
mr resources add-tags --ids 1,2,3 --tags 10,11
mr resources remove-tags --ids 1,2,3 --tags 10
mr resources replace-tags --ids 1,2,3 --tags 20,21

# Add groups
mr resources add-groups --ids 1,2,3 --groups 5,6

# Add metadata
mr resources add-meta --ids 1,2,3 --meta '{"reviewed":true}'

# Set dimensions
mr resources set-dimensions --ids 1,2 --width 1920 --height 1080

# Merge (move associations from losers to winner, delete losers)
mr resources merge --winner 1 --losers 2,3

# Bulk delete
mr resources delete --ids 4,5,6

# List unique metadata keys
mr resources meta-keys

# Bulk version cleanup
mr resources versions-cleanup --keep 3 --owner-id 1 --dry-run
```

### note / notes

```bash
# Get a note
mr note get <id>

# Create
mr note create --name "Meeting Notes" --description "Q1 review" \
  --owner-id 1 --note-type-id 2 --tags 1,3 --groups 5 --resources 10,11

# Edit
mr note edit-name <id> "Updated Title"
mr note edit-description <id> "Updated content"

# Share / unshare
mr note share <id>
mr note unshare <id>

# Delete
mr note delete <id>

# List with filters
mr notes list --name "meeting" --tags 1,2 --owner-id 1 --note-type-id 2
mr notes list --created-before 2025-06-01

# Bulk operations
mr notes add-tags --ids 1,2 --tags 5,6
mr notes remove-tags --ids 1,2 --tags 5
mr notes add-groups --ids 1,2 --groups 3,4
mr notes add-meta --ids 1,2 --meta '{"status":"reviewed"}'
mr notes delete --ids 3,4,5
mr notes meta-keys
```

### note-block / note-blocks

```bash
# Get a block
mr note-block get <id>

# Create a block
mr note-block create --note-id 1 --type text --content '{"text":"Hello"}' --position "a"

# Update content
mr note-block update <id> --content '{"text":"Updated"}'

# Update state
mr note-block update-state <id> --state '{"collapsed":true}'

# List available block types
mr note-block types

# Delete
mr note-block delete <id>

# List blocks for a note
mr note-blocks list --note-id 1

# Reorder blocks
mr note-blocks reorder --note-id 1 --positions '{"1":"a","2":"b"}'

# Rebalance positions
mr note-blocks rebalance --note-id 1
```

### group / groups

```bash
# Get a group
mr group get <id>

# Create
mr group create --name "Acme Corp" --category-id 1 --owner-id 2 \
  --tags 1,3 --groups 5 --meta '{"industry":"tech"}' --url "https://acme.com"

# Edit
mr group edit-name <id> "New Name"
mr group edit-description <id> "New description"

# Navigate hierarchy
mr group parents <id>
mr group children <id>

# Clone
mr group clone <id>

# Delete
mr group delete <id>

# List with filters
mr groups list --category-id 1 --owner-id 2 --tags 1,2 --url "acme"
mr groups list --created-after 2025-01-01

# Bulk operations
mr groups add-tags --ids 1,2 --tags 5,6
mr groups remove-tags --ids 1,2 --tags 5
mr groups add-meta --ids 1,2 --meta '{"region":"EU"}'
mr groups merge --winner 1 --losers 2,3
mr groups delete --ids 4,5
mr groups meta-keys

# Export a group subtree to a tar archive
mr group export <id> [<id>...] -o output.tar
mr group export <id> --include-versions --include-previews -o full.tar
mr group export <id> --no-blobs -o metadata-only.tar
mr group export <id> --gzip -o output.tar.gz

# Import a tar archive
mr group import <tarfile>
mr group import <tarfile> --dry-run --plan-output plan.json
mr group import <tarfile> --decisions decisions.json
mr group import <tarfile> --parent-group 42
mr group import <tarfile> --on-resource-conflict duplicate
mr group import <tarfile> --acknowledge-missing-hashes
```

### relation / relation-type / relation-types

```bash
# Create a relation between groups
mr relation create --from-group-id 1 --to-group-id 2 --relation-type-id 1 \
  --name "Partnership" --description "Since 2024"

# Edit
mr relation edit-name <id> "New Name"
mr relation edit-description <id> "New description"

# Delete
mr relation delete <id>

# Relation types
mr relation-type create --name "Subsidiary" --reverse-name "Parent Company" \
  --from-category 1 --to-category 2
mr relation-type edit --id 1 --name "Updated Name"
mr relation-type delete <id>
mr relation-types list --name "filter"
```

### series

```bash
mr series get <id>
mr series create --name "Photo Series 2025"
mr series edit <id> --name "Updated Name" --meta '{"season":2}'
mr series delete <id>
mr series remove-resource <resource-id>
mr series list --name "filter" --slug "photo"
```

### query / queries

```bash
# Get a saved query
mr query get <id>

# Create
mr query create --name "Untagged Resources" \
  --text "SELECT id, name FROM resources r LEFT JOIN resource_tags rt ON r.id = rt.resource_id WHERE rt.resource_id IS NULL"

# Run by ID
mr query run <id>

# Run by name
mr query run-by-name --name "Untagged Resources"

# Edit
mr query edit-name <id> "New Name"
mr query edit-description <id> "Description"

# Show database schema (for building queries)
mr query schema

# Delete
mr query delete <id>

# List
mr queries list --name "filter"
```

### mrql

Execute MRQL (Mahresources Query Language) queries and manage saved queries. See the [MRQL documentation](./mrql) for the full query language reference.

#### Execute a query

```bash
# Inline query
mr mrql 'type = resource AND tags = "photo" ORDER BY created DESC'

# Read from a file
mr mrql -f query.mrql

# Read from stdin
echo 'tags = "urgent" AND updated > -7d' | mr mrql -

# Limit results
mr mrql --limit 20 'type = note AND TEXT ~ "meeting"'

# Paginate
mr mrql --page 2 --limit 50 'type = resource AND contentType ~ "image/*"'
```

| Flag | Description |
|------|-------------|
| `-f`, `--file` | Read query from a file |
| `--limit` | Items per bucket for GROUP BY, or total items for regular queries |
| `--buckets` | Groups per page for bucketed GROUP BY queries |
| `--offset` | Bucket offset for cursor-based GROUP BY pagination |
| `--render` | Request server-side template rendering via CustomMRQLResult |

#### Subcommands

```bash
# Save a query for later reuse
mr mrql save "Untagged Resources" 'type = resource AND tags IS EMPTY'
mr mrql save "Recent Notes" 'type = note AND created > -7d ORDER BY created DESC' \
  --description "Notes from the past week"

# List all saved queries
mr mrql list

# Run a saved query by name or numeric ID
mr mrql run "Untagged Resources"
mr mrql run 3

# Delete a saved query by ID
mr mrql delete 3
```

#### Output modes

Like all `mr` commands, `mrql` supports the standard output flags:

```bash
# Formatted table (default)
mr mrql 'type = resource AND tags = "photo"'

# Raw JSON
mr mrql --json 'type = resource AND fileSize > 10mb'

# IDs only (useful for piping)
mr mrql --quiet 'type = resource AND tags IS EMPTY'

# No column headers
mr mrql --no-header 'type = note AND updated > -30d'
```

#### Piping MRQL results

```bash
# Delete all untagged resources (use with caution)
mr mrql --quiet 'type = resource AND tags IS EMPTY' | while read id; do
  mr resource delete "$id"
done

# Add a tag to resources matching a query
mr mrql --quiet 'type = resource AND contentType ~ "image/*" AND created > -7d' | while read id; do
  mr resources add-tags --ids "$id" --tags 10
done

# Export MRQL results as JSON
mr mrql --json 'type = note AND tags = "important"' > important-notes.json
```

### search

Global search across all entity types.

```bash
mr search "vacation photos"
mr search "meeting" --types resources,notes --limit 50
```

| Flag | Description |
|------|-------------|
| `--types` | Comma-separated entity types to search (e.g. `resources,notes,groups`) |
| `--limit` | Maximum results (default 20) |

### log / logs

```bash
# Get a log entry
mr log get <id>

# Get history for a specific entity
mr log entity --entity-type resource --entity-id 42

# List with filters
mr logs list --level error --action delete --entity-type resource
mr logs list --created-after 2025-03-01 --message "upload"
```

### job / jobs

Download queue management.

```bash
# Submit URLs for download
mr job submit --urls "https://example.com/a.jpg,https://example.com/b.jpg" \
  --tags 1,2 --groups 3 --owner-id 1

# Control jobs
mr job cancel <id>
mr job pause <id>
mr job resume <id>
mr job retry <id>

# View the queue
mr jobs list
```

### plugin / plugins

```bash
# Enable / disable
mr plugin enable <name>
mr plugin disable <name>

# Update settings
mr plugin settings <name> --data '{"apiKey":"abc123"}'

# Purge all plugin data
mr plugin purge-data <name>

# List installed plugins
mr plugins list
```

### admin

Show server health, data statistics, and diagnostic information.

```bash
# Show all statistics
mr admin

# Server health only
mr admin --server-only

# Data statistics only
mr admin --data-only

# Raw JSON output
mr admin --json
```

| Flag | Description |
|------|-------------|
| `--server-only` | Show server health metrics only (uptime, memory, DB connections) |
| `--data-only` | Show data statistics only (entity counts, storage, growth) |

The default output (no flags) shows all sections: server health, entity counts, storage breakdown, growth trends, configuration summary, content type distribution, orphan statistics, similarity stats, and log stats.

## Scripting Examples

### Tag all PNGs in a group

```bash
TAG_ID=5
mr resources list --groups 1 --content-type image/png --quiet | while read id; do
  mr resources add-tags --ids "$id" --tags "$TAG_ID"
done
```

### Export resource list as JSON

```bash
mr resources list --json > resources.json
```

### Bulk download all resources matching a filter

```bash
mr resources list --tags 10 --quiet | while read id; do
  mr resource download "$id" -o "resource_${id}"
done
```
