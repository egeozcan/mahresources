# Audit: Configuration, Deployment, Intro, Getting Started, Troubleshooting

Audited 14 files against inventories and style guide.

---

## intro.md

**Verdict:** PATCH
**Reason:** Accurate content, but contains a banned phrase from the style guide and the plugin system description is vague per the style guide example.

### Missing Content
- No mention of note blocks (a major feature)
- No mention of Download Cockpit / download queue
- No mention of series
- No mention of activity logging
- No mention of note sharing
- No mention of custom templates (CustomHeader/CustomSidebar/CustomSummary)
- No mention of meta schemas

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- Line ~25: "Extend Mahresources with plugins that hook into CRUD events and run background actions." -- Vague, per style guide example 3. Should name the language (Lua) and list concrete capabilities.

---

## getting-started/installation.md

**Verdict:** PATCH
**Reason:** Accurate and well-structured, but the Dockerfile Go version caution may be stale and should be verified.

### Missing Content
- No mention of ImageMagick as an optional dependency for HEIC/AVIF image thumbnails (code falls back to ImageMagick via `decodeImageWithFallback`)

### Wrong Content
- None found

### Stale Content
- Line ~39: "The Dockerfile currently uses `golang:1.21-alpine`, but the module requires Go 1.22+." -- Needs verification against the current Dockerfile. If the Dockerfile has been updated, this caution is stale.

### Style Issues
- None

---

## getting-started/quick-start.md

**Verdict:** PATCH
**Reason:** Good structure, but uses port 8080 in examples while the style guide requires using the default port 8181.

### Missing Content
- None significant for a quick-start page

### Wrong Content
- Lines ~14-15, ~48-49, ~62-63: Examples use `-bind-address=:8080` and `BIND_ADDRESS=:8080`. The style guide specifies port 8181 as the default for examples, and the application's default port is 8181. Using 8080 is inconsistent with all other doc pages.

### Stale Content
- None

### Style Issues
- Port inconsistency as noted above

---

## getting-started/first-steps.md

**Verdict:** OK
**Reason:** Well-structured tutorial walkthrough. Content is accurate and follows the how-to template.

### Missing Content
- None for a first-steps page. The "What's Next?" section appropriately links to advanced features.

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## configuration/overview.md

**Verdict:** PATCH
**Reason:** Comprehensive flag table is accurate and complete. Contains a banned phrase from the style guide.

### Missing Content
- None. The quick reference table includes all flags from the inventory including `cleanup-logs-days`, `video-thumb-*`, `thumb-worker-*`, `share-port`, `share-bind-address`, `plugin-path`, and `plugins-disabled`.

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- Line ~35: "This allows you to override `.env` settings for specific runs." -- Banned phrase per style guide. Replace with: "Command-line flags override `.env` settings."

---

## configuration/database.md

**Verdict:** OK
**Reason:** Accurate and thorough. Covers SQLite, PostgreSQL, in-memory, seeding, connection pools, logging, and startup optimizations.

### Missing Content
- None

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## configuration/storage.md

**Verdict:** OK
**Reason:** Accurate coverage of primary storage, alt filesystems, seed/copy-on-write, and storage layout. Well-structured with good examples.

### Missing Content
- None

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## configuration/advanced.md

**Verdict:** OK
**Reason:** Covers all advanced config flags accurately including share server, log cleanup, plugin configuration, network timeouts, hash worker, thumbnail worker, video thumbnail settings, external tools, and startup optimizations. All values match the inventory.

### Missing Content
- None. All flags from the inventory that are not covered by database.md or storage.md are documented here.

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## deployment/docker.md

**Verdict:** PATCH
**Reason:** Good coverage with SQLite and PostgreSQL compose files. The template Dockerfile may have a Go version that needs updating.

### Missing Content
- No mention of share server port exposure in Docker examples (covered in public-sharing.md, so this is acceptable as a cross-reference gap)
- No health check for the Mahresources container itself in compose files

### Wrong Content
- None

### Stale Content
- Line ~180: Template Dockerfile uses `golang:1.22-alpine`. Should be verified this matches the module's go.mod requirement. If go.mod requires a newer version, this is stale.

### Style Issues
- None

---

## deployment/systemd.md

**Verdict:** OK
**Reason:** Complete and accurate systemd deployment guide. Security hardening directives, update procedure, PostgreSQL variant, and troubleshooting all included.

### Missing Content
- None

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## deployment/reverse-proxy.md

**Verdict:** PATCH
**Reason:** Good coverage of Nginx, Caddy, Traefik, and alternative auth methods. Minor style issue.

### Missing Content
- No mention of share server reverse proxy config (this is covered in public-sharing.md, which is appropriate)
- No SSE-specific proxy configuration guidance (for `/v1/jobs/events` and `/v1/download/events`). SSE requires disabling response buffering -- only partially addressed in public-sharing.md's Nginx config but not here.

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- Line ~89: "Caddy provides automatic HTTPS with simpler configuration." -- Minor: "simpler" is comparative without a reference. Consider: "Caddy handles HTTPS certificates automatically."

---

## deployment/public-sharing.md

**Verdict:** OK
**Reason:** Thorough guide covering architecture, configuration, Docker deployment, reverse proxy configs for all three proxy types, security hardening, testing, and troubleshooting.

### Missing Content
- The share server also serves `POST /s/{token}/block/{blockId}/state` and `GET /s/{token}/block/{blockId}/calendar/events` endpoints, which are not mentioned in the testing section's curl examples. This is minor -- the feature is for interactive shared notes.

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- None

---

## deployment/backups.md

**Verdict:** PATCH
**Reason:** Comprehensive backup guide. One factual issue about thumbnail storage.

### Missing Content
- No mention of backing up the plugin directory (`plugins/`) if custom plugins are in use
- No mention of backing up alternative filesystem paths (configured via `-alt-fs`)

### Wrong Content
- Line ~25: "thumbnails are stored in the database, not on disk" -- This is correct. Preview/thumbnail binary data is stored in the `previews` table (as `Data []byte`). However, the statement could confuse users into thinking thumbnails are not part of the database backup. The phrasing is technically correct but worth clarifying.

### Stale Content
- None

### Style Issues
- None

---

## troubleshooting.md

**Verdict:** PATCH
**Reason:** Good coverage of common issues. Contains a placeholder value and could cover more scenarios.

### Missing Content
- No section on share server troubleshooting (partially covered in public-sharing.md)
- No mention of thumbnail worker configuration (`-thumb-worker-disabled`, `-thumb-backfill`) as a debugging step for thumbnail issues
- No mention of the video thumbnail timeout flags (`-video-thumb-timeout`, `-video-thumb-concurrency`) for video thumbnail failures
- No mention of log cleanup (`-cleanup-logs-days`) for database size management

### Wrong Content
- None

### Stale Content
- None

### Style Issues
- Line ~17: `lsof | grep your-database.db` -- Uses placeholder `your-database.db`. Style guide bans placeholder values. Replace with: `lsof | grep mahresources.db`
- Line ~115: "Images: JPEG, PNG, GIF, WebP, BMP" -- BMP is listed but not in the `IsHashable` content type list. Need to verify if BMP gets automatic thumbnails (it likely does via standard Go image decoders, but not perceptual hashing).
