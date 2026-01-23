# mahresources

Just a simple CRUD app written in golang to serve my personal information management needs. 
It's surely an overkill for any imagination of my requirements but the thing is I like developing software more than
using software.

This thing has support for notes, resources (files), tags, groups (generic entity). Everything is taggable. 
Notes, resources and groups can have json metadata that's editable and queryable via the web GUI.
See the models folder for exact relation definitions.

It generates previews for videos and images, ffmpeg is needed for videos. Thumbnails are cached on the database. I
currently have 1.5 Million resources saved through it without problems.

It also has an API. The frontend routes also dump a JSON object will nearly all the data used to render it, when
accepts header is json, or you type ".json" at the end of the path (like /groups.json instead of /groups).

Supports postgres and sqlite only. Mysql support should be fairly easy to add, I just personally don't need it.

Frontend uses Vite for bundling, Alpine.js for reactivity, and Tailwind CSS. Most things work without JS,
but auto-completers and some forms need it. Global search is accessible via `Cmd/Ctrl+K`.

## Build

```bash
# Full build (CSS + JS bundle + Go binary)
npm run build

# Or just the Go binary (requires json1 for SQLite JSON, fts5 for full-text search)
go build --tags 'json1 fts5'
```

For development with hot reload:
```bash
npm run watch
```

## Configuration

All settings can be configured via environment variables (in `.env`) or command-line flags. Flags take precedence.

```bash
# Using environment variables
cp .env.template .env
# Edit .env with your values
./mahresources

# Using command-line flags
./mahresources -db-type=SQLITE -db-dsn=mydb.db -file-save-path=./files -bind-address=:8080
```

Key flags:
| Flag | Description |
|------|-------------|
| `-file-save-path` | Main file storage directory |
| `-db-type` | Database type: SQLITE or POSTGRES |
| `-db-dsn` | Database connection string |
| `-bind-address` | Server address:port (default :8181) |
| `-ffmpeg-path` | Path to ffmpeg for video thumbnails |
| `-ephemeral` | Run fully in-memory (no persistence) |
| `-memory-db` | Use in-memory SQLite database |
| `-seed-db` | SQLite file to seed memory-db (for testing/demos) |
| `-seed-fs` | Directory as read-only base for copy-on-write |

### Ephemeral Mode

Run without any persistence (useful for demos or testing):
```bash
./mahresources -ephemeral -bind-address=:8080
```

Test against a copy of your data without modifying the original:
```bash
./mahresources -memory-db -seed-db=./production.db -file-save-path=./files
```

Fully seeded ephemeral mode (both database and files):
```bash
./mahresources -ephemeral -seed-db=./production.db -seed-fs=./files
```

Copy-on-write with persistent overlay (writes saved to disk):
```bash
./mahresources -db-type=SQLITE -db-dsn=./mydb.db -seed-fs=./original-files -file-save-path=./changes
```
The `-seed-fs` option uses copy-on-write: reads come from the seed directory, writes go to the overlay (memory with `-memory-fs`, or disk with `-file-save-path`).

### Frontend Assets

To build CSS/JS, install node and run `npm ci` first. The generated assets are committed, so you only need this if
you're modifying frontend code.

### Scripting

You probably need to import your own data, and you can do it via the HTTP API, or you can directly use the library
functions. For an example, see /cmd/importExisting/main.go, which can be run like
`go run ./cmd/importExisting/main.go -target "/some/folder" -ownerId 1234`.

The structure is very modular. I'll make it even more so
as I continue to develop.

## Testing

### Go Unit Tests
```bash
go test ./...
```

### E2E Tests (Playwright)

The project includes a comprehensive Playwright test suite covering CRUD operations, bulk operations, global search, edge cases, and accessibility (WCAG compliance via axe-core).

**Recommended: Use the automatic server management scripts** which handle starting an ephemeral server, running tests, and cleanup:

```bash
cd e2e
npm run test:with-server         # Run all tests
npm run test:with-server:headed  # Run with browser visible
npm run test:with-server:debug   # Run in debug mode
npm run test:with-server:a11y    # Run accessibility tests only
```

These scripts automatically find an available port, start an ephemeral server, run tests in parallel, and clean up.

**Manual server management** (if you need more control):

```bash
# 1. Build the application
npm run build

# 2. Start server in ephemeral mode (separate terminal)
./mahresources -ephemeral -bind-address=:8181 -max-db-connections=2

# 3. Run tests
cd e2e && npm test
```

Other test commands:
```bash
cd e2e
npm run test:headed    # Run with browser visible
npm run test:ui        # Playwright UI mode
npm run test:a11y      # Accessibility tests only
npm run report         # View HTML test report
```

## Security

There is zero security. No authorization or authentication, or even user accounts, really. This is thought to be run
on private networks or behind some sort of security layer like a firewall. 

# Help me

If you have any experience on tagging photos automatically (tensorflow stuff), any help is appreciated. I'll also come 
around to it eventually (probably), but I'm very open to help.

## Random Screenshot

![Screenshot of the app](img.png "I admit that I'm too lazy to add enough fake data for a better screenshot")

See how it is possible to query via the Meta field (JSON). Currently, there are more fields, and you can actually sort
and bulk edit the results. I'd rather keep developing than adding up-to-date screenshots though. Trust me, it looks OK.

## maybe do

- [ ] Make note categories work
- [ ] faster image previews with libvips?
- [x] some integration tests, perhaps behavioral test
- [ ] importers like perkeep? maybe.
- [ ] sync could be interesting
