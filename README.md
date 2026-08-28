# Mahresources

Mahresources is a self-hosted system for storing files, writing notes, and linking them together. It runs as a single Go binary with SQLite or PostgreSQL, and serves a web UI for browsing, searching, and editing everything. It is designed to work with libraries of millions of resources.

**[Read the full documentation](https://egeozcan.github.io/mahresources/)**

## Screenshots

<table>
  <tr>
    <td align="center">
      <img src="docs-site/static/img/dashboard.png" width="400" alt="Dashboard"><br>
      <em>Dashboard</em>
    </td>
    <td align="center">
      <img src="docs-site/static/img/grid-view.png" width="400" alt="Resource grid with filters"><br>
      <em>Resource Grid</em>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs-site/static/img/note-blocks.png" width="400" alt="Note system with structured blocks"><br>
      <em>Note Blocks</em>
    </td>
    <td align="center">
      <img src="docs-site/static/img/group-tree.png" width="400" alt="Hierarchical group organization"><br>
      <em>Group Tree</em>
    </td>
  </tr>
  <tr>
    <td align="center">
      <img src="docs-site/static/img/global-search.png" width="400" alt="Global search"><br>
      <em>Global Search (Cmd/Ctrl+K)</em>
    </td>
    <td align="center">
      <img src="docs-site/static/img/timeline-view.png" width="400" alt="Timeline view of recent activity"><br>
      <em>Timeline View</em>
    </td>
  </tr>
</table>

## What It Does

Files (called **Resources**), **Notes**, and **Groups** live in a database with tracked relationships and full-text search. Groups nest inside each other and hold any mix of Resources and Notes. **Tags** and **Categories** classify items across the hierarchy.

### Content

**Resources** are files of any type. Thumbnails are generated automatically: images natively, video through FFmpeg, and Office documents through LibreOffice. Perceptual hashing detects similar files, and every Resource keeps a full version history.

**Notes** hold text assembled from typed **note blocks**: text, heading, divider, gallery, references, todos, table, and calendar. Plugins can register more block types.

**Groups** form a nested hierarchy. A Group owns Resources, Notes, and other Groups, and typed **relations** connect one Group to another ("works at", "parent of"). A **series** ties together Resources that share metadata, such as the pages of one scanned document.

**Tags** are flat labels. **Categories**, **Resource Categories**, and **Note Types** are types, and each gives its entity its own presentation.

### Finding Things

**Full-text search** covers all content, via FTS5 on SQLite or tsvector on PostgreSQL, and is reachable from anywhere with Cmd/Ctrl+K.

**MRQL** is a query language for Resources, Notes, and Groups. It has its own page at `/mrql`, a filter bar on every list page, saved queries, and an `[mrql]` shortcode that embeds results in templates.

**Saved queries** store raw SQL alongside a custom result template. Point `DB_READONLY_DSN` at a read-only connection to enforce that at the database level.

Two more ways to move through a library are the **timeline view** and **image similarity** browsing.

### Working With Data

**Bulk operations** tag, merge, delete, or edit metadata across many items at once. **Resource Reduction** is a workspace for collapsing repeated Resources: clusters of repeats are proposed for review, and nothing is deleted until you approve it. **Group export / import** moves a Group subtree, with its resources, notes, tags, and categories, through a self-contained tar archive. The **download queue** takes remote URLs, shows them in the Download Cockpit, and retries them from persisted history.

Each Category, Resource Category, and Note Type carries a **meta schema**, a JSON Schema that validates metadata and generates a form for it. The same three carry **custom templates**: HTML fragments with server-side shortcodes, rendered into slots on detail pages, cards, hover cards, and list pages. **Note sharing** publishes an individual note on a separate read-only server behind an unguessable token, and an **activity log** records create, update, delete, and plugin operations across every entity.

### Extending and Integrating

**Plugins** are sandboxed Lua. They hook writes, add pages and JSON endpoints, run background and scheduled jobs, serve their own static assets, and keep per-plugin data in a key-value store.

Every route serves both HTML and **JSON**, so appending `.json` or sending `Accept: application/json` turns any page into an API response. The **`mr` CLI** is a command-line client covering that same API. **Accounts and RBAC** are optional and off by default, with four roles and group-subtree scoping.

## Requirements

Building Mahresources needs **Go 1.26+**, **Node.js 20.19+ or 22.12+**, and **a C compiler**. The Node version range is the one Vite 8 supports; older 20.x and 22.x releases fail `npm install` with `EBADENGINE`. The SQLite driver is cgo-based, so do not set `CGO_ENABLED=0`, or the binary builds without working SQLite.

FFmpeg (video thumbnails), LibreOffice (document thumbnails), and ImageMagick (HEIC and AVIF) are optional and detected on `PATH`.

## Quick Start

```bash
git clone https://github.com/egeozcan/mahresources.git
cd mahresources
npm install
npm run build
```

`npm run build` compiles Tailwind CSS, bundles JavaScript with Vite, and builds the Go binary with the `json1` and `fts5` tags.

```bash
# Ephemeral mode: in-memory database and filesystem, nothing persists
./mahresources -ephemeral

# Persistent storage
./mahresources -db-type=SQLITE -db-dsn=mydb.db -file-save-path=./files
```

The server listens on `:8181` by default. Start it from the repository root: the binary is not self-contained and loads Pongo2 templates from `./templates` and static assets from `./public`, both relative to the working directory.

No Docker image is published, but the repository builds one. See the [installation guide](https://egeozcan.github.io/mahresources/getting-started/installation) and the [Docker deployment guide](https://egeozcan.github.io/mahresources/deployment/docker).

## Configuration

Settings come from environment variables (a `.env` file works) or command-line flags. Flags take precedence.

| Flag | Description |
|------|-------------|
| `-file-save-path` | Main file storage directory |
| `-db-type` | Database type: `SQLITE` or `POSTGRES` |
| `-db-dsn` | Database connection string |
| `-bind-address` | Server address:port (default `:8181`) |
| `-ephemeral` | Run fully in-memory, with no persistence |
| `-auth` | Enable user accounts and RBAC (off by default) |
| `-plugin-path` | Directory scanned for plugins (default `./plugins`) |
| `-share-port` | Port for the public share server (empty disables note sharing) |

Roughly sixty more flags cover thumbnails, hashing, timeouts, alternative filesystems, seeded ephemeral modes, and job limits. Some are editable at runtime from `/admin/settings` without a restart. See the [configuration reference](https://egeozcan.github.io/mahresources/configuration/overview).

## Security

Authentication is **off by default**. With `-auth` unset there is no login page and no permission checks: every request runs as an implicit administrator. That is the historical behavior, and it assumes a private, trusted network.

> **Do not expose an unprotected instance to the internet.** For remote access, put it behind a reverse proxy that enforces authentication (nginx with basic auth, OAuth2 Proxy, Authelia). See the [reverse proxy guide](https://egeozcan.github.io/mahresources/deployment/reverse-proxy).

When you need accounts, turn them on with `-auth`. Requests then authenticate with a session cookie or a per-user API token, and one of four roles applies. An **admin** has full access, including settings, plugins, and user administration. An **editor** has CRUD on entities, but no categories and no system settings. A **user** has CRUD on resources and notes, optionally confined to one Group's subtree. A **guest** is read-only, and always confined to one Group's subtree.

Scoped users and guests are held to their subtree across lists, reads, search, MRQL, file serving, and writes. Bootstrap the first account with `-create-admin-user` and `-create-admin-password`. Built-in auth can be combined with a reverse proxy. See [Authentication & RBAC](https://egeozcan.github.io/mahresources/features/authentication).

## The `mr` CLI

`mr` is a command-line client for the HTTP API, covering every entity type with CRUD, bulk actions, file upload and download, versions, jobs, plugins, MRQL, and administration.

```bash
npm run build-cli     # produces ./mr
make install-cli      # optional: install it onto your PATH
```

```bash
mr resources list --content-type image/jpeg
mr resource upload ./photo.jpg --owner-id 3 --meta '{"camera":"Pixel"}'
mr mrql 'type = resource AND tags = "photo" AND created > -7d'
```

Against a server running with `-auth`, authenticate once with `mr auth login`, or set `MR_TOKEN` to an API token. See the [CLI reference](https://egeozcan.github.io/mahresources/cli/).

## Development

```bash
make help          # list every target
make build         # CSS + JS bundle + Go binary
make dev           # watch and rebuild (requires CompileDaemon)
make test          # Go tests + frontend unit tests
make test-e2e-all  # browser and CLI e2e suites in parallel
make ci            # the main local CI checks
```

E2E tests run against an ephemeral server that the scripts start and stop themselves, so they never touch real data. The Playwright suite lives in `e2e/`, and includes accessibility tests (`make test-a11y`), CLI tests, and a Postgres run. `npm run build-js` must be re-run after editing anything under `src/`, and its output in `public/dist/` is committed.

## Documentation

[Getting Started](https://egeozcan.github.io/mahresources/getting-started/installation) covers installation, the quick start, and first steps. [Core Concepts](https://egeozcan.github.io/mahresources/concepts/overview) explains resources, notes, groups, tags, relationships, and series, and the [User Guide](https://egeozcan.github.io/mahresources/user-guide/navigation) covers navigation, search, and bulk operations.

[Configuration](https://egeozcan.github.io/mahresources/configuration/overview) documents every flag and runtime setting. [Advanced Features](https://egeozcan.github.io/mahresources/features/mrql) covers MRQL, plugins, templates, versioning, and export/import. [Deployment](https://egeozcan.github.io/mahresources/deployment/docker) covers Docker, systemd, reverse proxies, and backups. Every `mr` command is documented in the [CLI reference](https://egeozcan.github.io/mahresources/cli/), and the REST endpoints in the [API reference](https://egeozcan.github.io/mahresources/api/overview).

The OpenAPI spec is generated from the routes themselves with `make openapi`.

## Scripting and Import

Every page has a JSON equivalent, so bulk imports can be written as HTTP scripts. `cmd/importExisting/main.go` is an example of driving the application context directly as a library instead.

## License

[GNU Affero General Public License v3.0](LICENSE).
