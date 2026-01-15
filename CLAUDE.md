# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Development Commands

```bash
# Build the application (compiles CSS + Go binary)
npm run build

# Development mode with hot reload
npm run watch

# Build CSS only
npm run build-css

# Run tests
go test ./...

# Run specific test file
go test ./server/api_tests/...

# Build Go binary directly (requires json1 tag for SQLite JSON support)
go build --tags json1
```

## Architecture Overview

Mahresources is a CRUD application for personal information management written in Go. It manages Resources (files), Notes, Groups, Tags, Categories, Queries, and their relationships.

### Core Layers

**application_context/** - Business logic and data access layer. Each entity has a dedicated context file (e.g., `resource_context.go`, `note_context.go`) that implements CRUD operations. The main `context.go` initializes DB, filesystem, and configuration.

**models/** - GORM models and database layer. Entity models are in `*_model.go` files. Query DTOs are in `query_models/`. GORM query scopes are in `database_scopes/`.

**server/** - HTTP layer with Gorilla Mux routing.
- `api_handlers/` - JSON API endpoints
- `template_handlers/` - HTML template rendering
- `interfaces/` - Interface definitions for dependency injection (Reader, Writer, Deleter patterns)

**templates/** - Pongo2 templates (Django-like syntax). Each entity has create, display, and list templates.

### Key Design Patterns

**Dual Response Format**: Routes support both HTML and JSON responses. Add `.json` suffix or use `Accept: application/json` header to get JSON.

**Generic Entity Writers**: `EntityWriter[T]` generic type handles common CRUD operations across entities.

**Interface-based DI**: Handlers receive specific interfaces (e.g., `ResourceReader`, `GroupWriter`) rather than concrete implementations.

### Entity Relationships

- **Resource**: Files with metadata, thumbnails, perceptual hashes. Many-to-many with Tags, Notes, Groups.
- **Note**: Text content with NoteType. Many-to-many with Resources, Tags, Groups.
- **Group**: Hierarchical collections. Can own other Groups, Resources, Notes.
- **GroupRelation**: Custom typed relationships between groups.
- **Tag/Category**: Labels for organization.
- **Query**: Saved searches.

### Configuration

Environment variables in `.env` (see `.env.template`):
- `DB_TYPE`: SQLITE or POSTGRES
- `DB_DSN`: Database connection string
- `FILE_SAVE_PATH`: Resource storage directory
- `FFMPEG_PATH`: Required for video thumbnails

### API Structure

Base path: `/v1`

Endpoints follow pattern: `GET/POST/DELETE /v1/{entities}` for lists, `/v1/{entity}` for single items.

Bulk operations available: `addTags`, `removeTags`, `addMeta`, `delete`, `merge`.

## Important Notes

- No authentication/authorization - designed for private networks only
- SQLite requires `--tags json1` build flag for JSON query support
- Image processing uses bild and nfnt/resize libraries
- File system abstraction via Afero supports multiple storage locations
