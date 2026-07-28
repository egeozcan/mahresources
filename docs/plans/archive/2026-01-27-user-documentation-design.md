# User Documentation Design

## Overview

Comprehensive user documentation for Mahresources using Docusaurus, deployed to GitHub Pages. Covers installation, usage, configuration, API reference, and deployment for all audience levels.

## Audience

- **Non-technical end users** - People organizing files and notes
- **Technical self-hosters** - Developers/sysadmins installing and configuring
- **API consumers** - Developers building integrations

## Security Model (Critical)

Mahresources has **no authentication or authorization**. This must be prominently communicated:

1. **Global announcement bar** on every page
2. **Dedicated security section** in introduction
3. **Warning callouts** in installation, configuration, deployment, and API sections
4. **Docusaurus `:::danger` admonitions** for maximum visibility

## Directory Structure

```
docs-site/
├── docusaurus.config.js
├── package.json
├── sidebars.js
├── docs/
│   ├── intro.md
│   ├── getting-started/
│   │   ├── installation.md
│   │   ├── quick-start.md
│   │   └── first-steps.md
│   ├── concepts/
│   │   ├── overview.md
│   │   ├── resources.md
│   │   ├── notes.md
│   │   ├── groups.md
│   │   ├── tags-categories.md
│   │   └── relationships.md
│   ├── user-guide/
│   │   ├── navigation.md
│   │   ├── managing-resources.md
│   │   ├── managing-notes.md
│   │   ├── organizing-with-groups.md
│   │   ├── search.md
│   │   └── bulk-operations.md
│   ├── configuration/
│   │   ├── overview.md
│   │   ├── database.md
│   │   ├── storage.md
│   │   └── advanced.md
│   ├── features/
│   │   ├── versioning.md
│   │   ├── image-similarity.md
│   │   ├── saved-queries.md
│   │   └── custom-templates.md
│   ├── api/
│   │   ├── overview.md
│   │   ├── resources.md
│   │   ├── notes.md
│   │   ├── groups.md
│   │   └── other-endpoints.md
│   ├── deployment/
│   │   ├── docker.md
│   │   ├── systemd.md
│   │   ├── reverse-proxy.md
│   │   └── backups.md
│   └── troubleshooting.md
└── static/
    └── img/
```

## Content Plan

### Introduction (intro.md)

- What Mahresources is: personal information management for files, notes, relationships
- Key features overview
- Security model callout (no auth, private networks only)
- Quick links to getting started

### Getting Started

**installation.md**
- Security warning
- Pre-built binaries installation
- Building from source (Go 1.21+, Node.js 18+, build commands)
- Docker installation
- Optional dependencies: ffmpeg, LibreOffice

**quick-start.md**
- Ephemeral mode for testing: `./mahresources -ephemeral -bind-address=:8080`
- Open browser, upload first file, create first note
- Transition to persistent setup

**first-steps.md**
- Create a Category
- Create a Group in that category
- Upload resources to the group
- Create notes
- Add tags
- Use global search (Cmd/Ctrl+K)

### Core Concepts

**overview.md**
- Entity hierarchy explanation
- Common features: tags, metadata, full-text search
- Ownership vs relationships

**resources.md**
- Files with metadata and thumbnails
- Hash calculation for deduplication
- Versioning support
- Relationships to groups, notes, tags

**notes.md**
- Text content with Markdown support
- NoteTypes for categorization
- Optional start/end dates
- Attachments to resources and groups

**groups.md**
- Hierarchical containers within Categories
- Owned vs related entities
- Cloning and merging

**tags-categories.md**
- Tags: flat labels across all entities
- Categories: types of groups
- Custom HTML headers/sidebars

**relationships.md**
- RelationTypes define connection kinds
- Relations connect two groups
- Graph-like navigation

### User Guide

**navigation.md**
- Top navigation bar
- Global search (Cmd/Ctrl+K)
- List views with filtering and pagination
- Detail views with sidebar
- Mobile responsive design

**managing-resources.md**
- Uploading: drag-drop, file picker, URL
- Viewing: preview, full-size, download
- Editing: rename, description, metadata
- Image operations: rotate, recalculate dimensions
- Deleting

**managing-notes.md**
- Creating with name, description, dates
- Assigning NoteType
- Attaching resources
- Linking to groups
- Wide display mode
- Managing NoteTypes

**organizing-with-groups.md**
- Creating within categories
- Setting owner for hierarchy
- Owned vs related entities
- Creating relations between groups
- Cloning and merging

**search.md**
- Global search quick access
- List view filters
- Full-text search
- Saved Queries

**bulk-operations.md**
- Multi-select in list views
- Bulk add/remove tags
- Bulk add metadata
- Bulk delete
- Merging duplicates

### Configuration

**overview.md**
- Security warning
- Environment variables vs command-line flags
- Quick reference table
- Ephemeral vs persistent modes

**database.md**
- SQLite setup (default)
- PostgreSQL setup
- Read-only replica configuration
- Logging options
- Connection pool tuning
- FTS initialization skip

**storage.md**
- Primary storage path
- Alternative filesystems
- Memory filesystem
- Seed filesystem (copy-on-write)
- File organization on disk

**advanced.md**
- Thumbnail generation (ffmpeg, LibreOffice)
- Hash worker settings
- Remote download timeouts
- Bind address configuration
- Version migration skip

### Advanced Features

**versioning.md**
- How versioning works
- Version history panel
- Comparing versions (image, text, binary)
- Reverting to previous versions
- Storage implications

**image-similarity.md**
- Perceptual hashing explanation
- Background hash worker
- Similar images display
- Similarity threshold configuration
- One-click merge
- Supported formats

**saved-queries.md**
- Creating queries
- Accessing saved queries
- Use cases
- Editing and deleting

**custom-templates.md**
- Category custom HTML
- NoteType custom HTML
- Alpine.js entity data access
- Security note (trusted network only)
- Examples

### API Reference

**overview.md**
- Security warning (no API auth)
- Base path `/v1`
- Dual response format (JSON suffix or Accept header)
- Request formats
- Pagination
- Error responses
- OpenAPI spec generation

**resources.md**
- List, get, create, edit, delete endpoints
- View and preview endpoints
- Bulk operations
- Request/response examples

**notes.md**
- Note CRUD
- NoteType CRUD
- Inline editing endpoints

**groups.md**
- Group CRUD
- Clone and merge
- Relation management
- Bulk operations

**other-endpoints.md**
- Tags and Categories
- Saved Queries
- Global search
- Logs
- Meta keys

### Deployment

**docker.md**
- Security warning
- Docker run with volumes
- Docker Compose example
- Environment configuration
- Resource limits

**systemd.md**
- Unit file creation
- User/permissions
- Environment file
- Service management
- Logging

**reverse-proxy.md**
- Why required for non-local deployment
- Nginx configuration
- Caddy configuration
- Basic authentication setup
- Large upload handling
- Network restriction

**backups.md**
- What to back up
- SQLite backup strategies
- PostgreSQL backup
- File storage backup
- Testing restores
- Seeding from backups

### Troubleshooting

**Common Issues:**
- Database locked (SQLite)
- Thumbnails not generating
- Slow startup
- Upload failures
- Search not working
- Similar images not appearing

**FAQ:**
- Multiple users
- SQLite to PostgreSQL migration
- Supported file types
- Version disk usage
- Multiple instances
- Docker image status
- Factory reset

**Getting Help:**
- GitHub issues

## Docusaurus Configuration

### docusaurus.config.js

```javascript
module.exports = {
  title: 'Mahresources Documentation',
  tagline: 'Personal information management for files, notes, and relationships',
  url: 'https://egeozcan.github.io',
  baseUrl: '/mahresources/',
  organizationName: 'egeozcan',
  projectName: 'mahresources',
  trailingSlash: false,

  themeConfig: {
    announcementBar: {
      id: 'security_warning',
      content: '⚠️ Mahresources has no authentication. Only run on trusted private networks.',
      backgroundColor: '#ff4444',
      textColor: '#ffffff',
      isCloseable: false,
    },
    navbar: {
      title: 'Mahresources',
      items: [
        { type: 'doc', docId: 'intro', position: 'left', label: 'Docs' },
        { href: 'https://github.com/egeozcan/mahresources', label: 'GitHub', position: 'right' },
      ],
    },
  },

  presets: [
    ['classic', {
      docs: {
        routeBasePath: '/',
        sidebarPath: require.resolve('./sidebars.js'),
      },
    }],
  ],

  themes: ['@docusaurus/theme-search-algolia'], // or local search plugin
};
```

### sidebars.js

```javascript
module.exports = {
  docs: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      items: [
        'getting-started/installation',
        'getting-started/quick-start',
        'getting-started/first-steps',
      ],
    },
    {
      type: 'category',
      label: 'Core Concepts',
      items: [
        'concepts/overview',
        'concepts/resources',
        'concepts/notes',
        'concepts/groups',
        'concepts/tags-categories',
        'concepts/relationships',
      ],
    },
    {
      type: 'category',
      label: 'User Guide',
      items: [
        'user-guide/navigation',
        'user-guide/managing-resources',
        'user-guide/managing-notes',
        'user-guide/organizing-with-groups',
        'user-guide/search',
        'user-guide/bulk-operations',
      ],
    },
    {
      type: 'category',
      label: 'Configuration',
      items: [
        'configuration/overview',
        'configuration/database',
        'configuration/storage',
        'configuration/advanced',
      ],
    },
    {
      type: 'category',
      label: 'Advanced Features',
      items: [
        'features/versioning',
        'features/image-similarity',
        'features/saved-queries',
        'features/custom-templates',
      ],
    },
    {
      type: 'category',
      label: 'API Reference',
      items: [
        'api/overview',
        'api/resources',
        'api/notes',
        'api/groups',
        'api/other-endpoints',
      ],
    },
    {
      type: 'category',
      label: 'Deployment',
      items: [
        'deployment/docker',
        'deployment/systemd',
        'deployment/reverse-proxy',
        'deployment/backups',
      ],
    },
    'troubleshooting',
  ],
};
```

## GitHub Pages Deployment

### Workflow (.github/workflows/docs.yml)

```yaml
name: Deploy Documentation

on:
  push:
    branches: [master]
    paths:
      - 'docs-site/**'
      - '.github/workflows/docs.yml'
  workflow_dispatch:

permissions:
  contents: read
  pages: write
  id-token: write

concurrency:
  group: pages
  cancel-in-progress: true

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: 20
          cache: npm
          cache-dependency-path: docs-site/package-lock.json

      - name: Install dependencies
        working-directory: docs-site
        run: npm ci

      - name: Build
        working-directory: docs-site
        run: npm run build

      - uses: actions/upload-pages-artifact@v3
        with:
          path: docs-site/build

  deploy:
    environment:
      name: github-pages
      url: ${{ steps.deployment.outputs.page_url }}
    runs-on: ubuntu-latest
    needs: build
    steps:
      - uses: actions/deploy-pages@v4
        id: deployment
```

### Repository Settings (Manual)

1. Go to repository Settings > Pages
2. Source: GitHub Actions

## Implementation Notes

- Documentation site is in `docs-site/` folder, separate from `docs/plans/`
- Update root `.gitignore` for `docs-site/node_modules` and `docs-site/build`
- Text-only initially; `static/img/` ready for future screenshots
- Use Docusaurus `:::danger` admonitions for security warnings
- Local search plugin recommended over Algolia for simplicity

## Summary

- ~30 documentation pages across 9 sections
- Docusaurus 3 with classic theme
- Automatic GitHub Pages deployment
- Prominent security warnings throughout
- No authentication model clearly communicated
