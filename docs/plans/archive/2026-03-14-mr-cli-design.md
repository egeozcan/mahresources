# `mr` CLI for mahresources

## Overview

A Go CLI using Cobra that wraps the mahresources JSON API. Ships as a single binary (`mr`) from `cmd/mr/`. Talks to a running mahresources server over HTTP.

## Decisions

- **Language**: Go (same as server, reuse model types)
- **Framework**: Cobra
- **Communication**: HTTP client against the JSON API
- **Binary name**: `mr`
- **Output**: Human-readable tables by default, `--json` for machine-readable
- **Location**: `cmd/mr/` in this repo
- **File operations**: Supported in v1 (upload, download, versions)
- **Server config**: `--server` flag + `MAHRESOURCES_URL` env var, default `http://localhost:8181`

## Command Structure

Entity-first pattern: `mr <entity> <action> [args] [flags]`

Singular for single-item ops, plural for list/bulk ops.

### Standard Actions Per Entity

| Action | Maps to |
|--------|---------|
| `list` | `GET /v1/{entities}` |
| `get <id>` | `GET /v1/{entity}?id=` |
| `create` | `POST /v1/{entity}` |
| `edit <id>` | `POST /v1/{entity}/edit` or `POST /v1/{entity}` |
| `delete <id>` | `POST /v1/{entity}/delete?Id=` |
| `edit-name <id>` | `POST /v1/{entity}/editName?id=` |
| `edit-description <id>` | `POST /v1/{entity}/editDescription?id=` |

Bulk actions on plural forms: `add-tags`, `remove-tags`, `add-groups`, `add-meta`, `delete`, `merge`.

### Entities

- `resource` / `resources`
- `note` / `notes`
- `group` / `groups`
- `tag` / `tags`
- `category` / `categories`
- `resource-category` / `resource-categories`
- `query` / `queries`
- `relation` / `relations`
- `relation-type` / `relation-types`
- `note-type` / `note-types`
- `note-block` / `note-blocks`
- `series`
- `log` / `logs`
- `job` / `jobs`
- `search`
- `plugin` / `plugins`

### Entity-Specific Actions

#### Resources

```
mr resource upload <file>                         # POST /v1/resource (multipart)
mr resource download <id> [-o file]               # GET /v1/resource/view
mr resource preview <id> [-w 200 -h 200]          # GET /v1/resource/preview
mr resource from-url --url "..."                   # POST /v1/resource/remote
mr resource from-local --path "..."                # POST /v1/resource/local
mr resource rotate <id> --degrees 90
mr resources replace-tags --ids ... --tags ...
mr resources set-dimensions --ids ... --w ... --h ...
```

#### Resource Versions

```
mr resource versions <id>
mr resource version <id>
mr resource version-upload <resource-id> <file>
mr resource version-download <version-id> [-o file]
mr resource version-restore --resource-id X --version-id Y
mr resource version-delete --resource-id X --version-id Y
mr resource versions-cleanup <resource-id> [--keep N]
mr resources versions-cleanup --ids ... [--keep N]
mr resource versions-compare <resource-id> --v1 X --v2 Y
```

#### Notes

```
mr note share <id>
mr note unshare <id>
```

#### Note Blocks

```
mr note-blocks list --note-id <id>
mr note-block get <id>
mr note-block create --note-id <id> --type "..." --content "..."
mr note-block update <id> --content "..."
mr note-block delete <id>
mr note-blocks reorder --note-id <id> --block-ids 1,2,3
mr note-block types
```

#### Groups

```
mr group parents <id>
mr group children <id>
mr group clone <id>
```

#### Queries

```
mr query run <id>
mr query run --name "query-name"
mr query schema
```

#### Search

```
mr search "query text" [--types resources,notes --limit 20]
```

#### Jobs

```
mr job submit --urls "..." [--tags ... --groups ...]
mr jobs list
mr job cancel <id>
mr job pause <id>
mr job resume <id>
mr job retry <id>
```

#### Plugins

```
mr plugins list
mr plugin enable <name>
mr plugin disable <name>
mr plugin settings <name> --data '{...}'
mr plugin purge-data <name>
```

#### Meta Keys

```
mr resources meta-keys
mr notes meta-keys
mr groups meta-keys
```

### Global Flags

```
--server URL        Server URL (env: MAHRESOURCES_URL, default: http://localhost:8181)
--json              Output as JSON instead of table
--no-header         Omit table header
--page N            Page number for list commands (default: 1)
--quiet             Only output IDs (useful for scripting)
```

## Project Layout

```
cmd/mr/
├── main.go              # Cobra root command, global flags
├── client/
│   └── client.go        # HTTP client wrapper
├── output/
│   └── output.go        # Table/JSON/quiet output formatting
└── commands/
    ├── resources.go
    ├── notes.go
    ├── groups.go
    ├── tags.go
    ├── categories.go
    ├── resource_categories.go
    ├── queries.go
    ├── relations.go
    ├── relation_types.go
    ├── note_types.go
    ├── note_blocks.go
    ├── series.go
    ├── logs.go
    ├── jobs.go
    ├── search.go
    └── plugins.go
```

## Key Implementation Details

- **Reuse types**: Import `models` and `query_models` for request/response serialization
- **File uploads**: `multipart/form-data` via Go's `mime/multipart`
- **File downloads**: Stream response body to file with progress
- **Error handling**: Parse server error responses, display cleanly
- **Pagination**: `--page` flag on list commands; display page info in table footer
