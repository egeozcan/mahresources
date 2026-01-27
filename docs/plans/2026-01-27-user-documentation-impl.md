# User Documentation Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Create a comprehensive Docusaurus documentation site for Mahresources and deploy it to GitHub Pages.

**Architecture:** Docusaurus 3 static site in `docs-site/` folder with ~30 markdown pages covering installation, usage, configuration, API, and deployment. GitHub Actions workflow auto-deploys to GitHub Pages on push to master.

**Tech Stack:** Docusaurus 3, React, Node.js 20, GitHub Actions, GitHub Pages

---

## Phase 1: Docusaurus Setup

### Task 1: Initialize Docusaurus Project

**Files:**
- Create: `docs-site/` (entire directory structure)

**Step 1: Create Docusaurus site**

Run from project root:
```bash
cd /Users/egecan/Code/mahresources/.worktrees/feature/user-documentation
npx create-docusaurus@latest docs-site classic --typescript
```

When prompted, accept defaults.

**Step 2: Verify installation**

```bash
cd docs-site && npm run build
```

Expected: Build completes successfully.

**Step 3: Clean up default content**

Delete default docs that we'll replace:
```bash
rm -rf docs-site/docs/*
rm -rf docs-site/blog
```

**Step 4: Commit**

```bash
git add docs-site
git commit -m "chore: initialize Docusaurus documentation site"
```

---

### Task 2: Configure Docusaurus for GitHub Pages

**Files:**
- Modify: `docs-site/docusaurus.config.ts`

**Step 1: Update configuration**

Replace `docs-site/docusaurus.config.ts` with:

```typescript
import {themes as prismThemes} from 'prism-react-renderer';
import type {Config} from '@docusaurus/types';
import type * as Preset from '@docusaurus/preset-classic';

const config: Config = {
  title: 'Mahresources Documentation',
  tagline: 'Personal information management for files, notes, and relationships',
  favicon: 'img/favicon.ico',

  url: 'https://egeozcan.github.io',
  baseUrl: '/mahresources/',

  organizationName: 'egeozcan',
  projectName: 'mahresources',
  trailingSlash: false,

  onBrokenLinks: 'throw',
  onBrokenMarkdownLinks: 'warn',

  i18n: {
    defaultLocale: 'en',
    locales: ['en'],
  },

  presets: [
    [
      'classic',
      {
        docs: {
          routeBasePath: '/',
          sidebarPath: './sidebars.ts',
          editUrl: 'https://github.com/egeozcan/mahresources/tree/master/docs-site/',
        },
        blog: false,
        theme: {
          customCss: './src/css/custom.css',
        },
      } satisfies Preset.Options,
    ],
  ],

  themeConfig: {
    announcementBar: {
      id: 'security_warning',
      content: '⚠️ <strong>Security Notice:</strong> Mahresources has no authentication. Only run on trusted private networks.',
      backgroundColor: '#dc2626',
      textColor: '#ffffff',
      isCloseable: false,
    },
    navbar: {
      title: 'Mahresources',
      items: [
        {
          type: 'docSidebar',
          sidebarId: 'docs',
          position: 'left',
          label: 'Documentation',
        },
        {
          href: 'https://github.com/egeozcan/mahresources',
          label: 'GitHub',
          position: 'right',
        },
      ],
    },
    footer: {
      style: 'dark',
      links: [
        {
          title: 'Docs',
          items: [
            {label: 'Getting Started', to: '/getting-started/installation'},
            {label: 'User Guide', to: '/user-guide/navigation'},
            {label: 'API Reference', to: '/api/overview'},
          ],
        },
        {
          title: 'More',
          items: [
            {label: 'GitHub', href: 'https://github.com/egeozcan/mahresources'},
            {label: 'Issues', href: 'https://github.com/egeozcan/mahresources/issues'},
          ],
        },
      ],
      copyright: `Copyright © ${new Date().getFullYear()} Mahresources. Built with Docusaurus.`,
    },
    prism: {
      theme: prismThemes.github,
      darkTheme: prismThemes.dracula,
      additionalLanguages: ['bash', 'json', 'yaml', 'go'],
    },
  } satisfies Preset.ThemeConfig,
};

export default config;
```

**Step 2: Verify build still works**

```bash
cd docs-site && npm run build
```

Expected: Build completes (may warn about missing docs).

**Step 3: Commit**

```bash
git add docs-site/docusaurus.config.ts
git commit -m "chore: configure Docusaurus for GitHub Pages deployment"
```

---

### Task 3: Configure Sidebar Navigation

**Files:**
- Modify: `docs-site/sidebars.ts`

**Step 1: Update sidebars**

Replace `docs-site/sidebars.ts` with:

```typescript
import type {SidebarsConfig} from '@docusaurus/plugin-content-docs';

const sidebars: SidebarsConfig = {
  docs: [
    'intro',
    {
      type: 'category',
      label: 'Getting Started',
      collapsed: false,
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

export default sidebars;
```

**Step 2: Commit**

```bash
git add docs-site/sidebars.ts
git commit -m "chore: configure documentation sidebar navigation"
```

---

### Task 4: Create GitHub Actions Workflow

**Files:**
- Create: `.github/workflows/docs.yml`

**Step 1: Create workflow file**

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

**Step 2: Commit**

```bash
git add .github/workflows/docs.yml
git commit -m "ci: add GitHub Actions workflow for docs deployment"
```

---

### Task 5: Update Root .gitignore

**Files:**
- Modify: `.gitignore`

**Step 1: Add docs-site ignores**

Append to `.gitignore`:

```
# Documentation site
docs-site/node_modules/
docs-site/build/
docs-site/.docusaurus/
```

**Step 2: Commit**

```bash
git add .gitignore
git commit -m "chore: add docs-site to gitignore"
```

---

## Phase 2: Core Documentation Pages

### Task 6: Create Introduction Page

**Files:**
- Create: `docs-site/docs/intro.md`

**Step 1: Write intro.md**

```markdown
---
slug: /
sidebar_position: 1
---

# Introduction

Mahresources is a personal information management system for organizing files, notes, and their relationships. It provides a web-based interface for managing your digital assets on your own infrastructure.

## Key Features

- **Resource Management** - Upload, organize, and search files of any type with automatic thumbnail generation for images, videos, PDFs, and office documents
- **Note Taking** - Create rich text notes with Markdown support, attach them to resources and groups
- **Hierarchical Organization** - Build flexible hierarchies using Groups and Categories
- **Tagging System** - Apply tags across all entity types for cross-cutting organization
- **Full-Text Search** - Find anything instantly with global search (Cmd/Ctrl+K)
- **Image Similarity** - Automatically detect similar or duplicate images using perceptual hashing
- **Version Control** - Track changes to resources with full version history and comparison tools
- **Saved Queries** - Store complex searches for quick access
- **Flexible Relationships** - Create typed connections between groups for graph-like navigation
- **API Access** - Full REST API for automation and integrations

## Security Model

:::danger No Authentication

Mahresources has **no built-in authentication or authorization**. Anyone with network access to the application has full read, write, and delete access to all data.

**This means:**
- Never expose Mahresources directly to the public internet
- Only run on trusted private networks or behind a VPN
- If you need remote access, use a reverse proxy with authentication (see [Reverse Proxy Guide](/deployment/reverse-proxy))

:::

## Who Is This For?

Mahresources is designed for individuals or small teams who want to:

- Organize personal files, photos, and documents
- Build a knowledge base with interconnected notes
- Manage research materials and references
- Create structured collections with custom metadata

## Getting Started

Ready to begin? Head to the [Installation Guide](/getting-started/installation) to set up Mahresources on your system.
```

**Step 2: Verify build**

```bash
cd docs-site && npm run build
```

Expected: Build may fail due to missing linked pages - that's OK for now.

**Step 3: Commit**

```bash
git add docs-site/docs/intro.md
git commit -m "docs: add introduction page with security warning"
```

---

### Task 7: Create Getting Started - Installation

**Files:**
- Create: `docs-site/docs/getting-started/installation.md`

**Step 1: Write installation.md**

```markdown
---
sidebar_position: 1
---

# Installation

This guide covers different ways to install Mahresources on your system.

:::danger Security Warning

Before proceeding, understand that Mahresources has **no authentication**. Only install it on trusted private networks. See the [Introduction](/) for details.

:::

## Prerequisites

- **For binary/source installation:** Go 1.21+ and Node.js 18+
- **For Docker:** Docker Engine 20+
- **Optional:** ffmpeg (video thumbnails), LibreOffice (office document thumbnails)

## Option 1: Pre-built Binaries

Download the latest release from [GitHub Releases](https://github.com/egeozcan/mahresources/releases):

```bash
# Download and extract (adjust for your platform)
wget https://github.com/egeozcan/mahresources/releases/latest/download/mahresources-linux-amd64.tar.gz
tar -xzf mahresources-linux-amd64.tar.gz
chmod +x mahresources
```

## Option 2: Build from Source

Clone the repository and build:

```bash
# Clone
git clone https://github.com/egeozcan/mahresources.git
cd mahresources

# Install dependencies and build
npm install
npm run build

# Build Go binary with required tags
go build --tags 'json1 fts5'
```

The `json1` tag enables SQLite JSON functions, and `fts5` enables full-text search.

## Option 3: Docker

Pull and run the Docker image:

```bash
docker run -d \
  --name mahresources \
  -p 8080:8080 \
  -v mahresources-data:/data \
  -v mahresources-files:/files \
  -e DB_TYPE=SQLITE \
  -e DB_DSN=/data/mahresources.db \
  -e FILE_SAVE_PATH=/files \
  ghcr.io/egeozcan/mahresources:latest
```

See [Docker Deployment](/deployment/docker) for more options.

## Optional Dependencies

### ffmpeg (Video Thumbnails)

Install ffmpeg for video thumbnail generation:

```bash
# Ubuntu/Debian
sudo apt install ffmpeg

# macOS
brew install ffmpeg

# Windows
# Download from https://ffmpeg.org/download.html
```

Set the path if not in system PATH:

```bash
./mahresources -ffmpeg-path=/usr/local/bin/ffmpeg
```

### LibreOffice (Office Document Thumbnails)

Install LibreOffice for Word, Excel, PowerPoint thumbnail generation:

```bash
# Ubuntu/Debian
sudo apt install libreoffice

# macOS
brew install --cask libreoffice
```

Mahresources auto-detects `soffice` or `libreoffice` in PATH. Override with:

```bash
./mahresources -libreoffice-path=/path/to/soffice
```

## Next Steps

Continue to [Quick Start](/getting-started/quick-start) to run Mahresources for the first time.
```

**Step 2: Commit**

```bash
mkdir -p docs-site/docs/getting-started
git add docs-site/docs/getting-started/installation.md
git commit -m "docs: add installation guide"
```

---

### Task 8: Create Getting Started - Quick Start

**Files:**
- Create: `docs-site/docs/getting-started/quick-start.md`

**Step 1: Write quick-start.md**

```markdown
---
sidebar_position: 2
---

# Quick Start

Get Mahresources running in under a minute with ephemeral mode.

## Ephemeral Mode

Ephemeral mode runs entirely in memory - perfect for testing. No data persists after shutdown.

```bash
./mahresources -ephemeral -bind-address=:8080
```

Open http://localhost:8080 in your browser.

## Your First Upload

1. Click **Resources** in the navigation bar
2. Click **New Resource**
3. Drag and drop a file or click to select one
4. Click **Create**

Your file is now stored with automatic thumbnail generation (for supported types).

## Your First Note

1. Click **Notes** in the navigation bar
2. Click **New Note**
3. Enter a name and description (Markdown supported)
4. Click **Create**

## Persistent Setup

When you're ready to keep your data, switch to persistent storage:

```bash
# Create directories
mkdir -p ./data ./files

# Run with persistent storage
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./data/mahresources.db \
  -file-save-path=./files \
  -bind-address=:8080
```

Your data now persists between restarts.

## Configuration Options

Common flags:

| Flag | Description | Example |
|------|-------------|---------|
| `-bind-address` | Server address:port | `:8080` |
| `-db-type` | Database type | `SQLITE` or `POSTGRES` |
| `-db-dsn` | Database connection string | `./data/app.db` |
| `-file-save-path` | File storage directory | `./files` |

See [Configuration Overview](/configuration/overview) for all options.

## Next Steps

Continue to [First Steps](/getting-started/first-steps) for a guided tour of the main features.
```

**Step 2: Commit**

```bash
git add docs-site/docs/getting-started/quick-start.md
git commit -m "docs: add quick start guide"
```

---

### Task 9: Create Getting Started - First Steps

**Files:**
- Create: `docs-site/docs/getting-started/first-steps.md`

**Step 1: Write first-steps.md**

```markdown
---
sidebar_position: 3
---

# First Steps

This guide walks you through the core workflow of organizing content in Mahresources.

## Understanding the Hierarchy

Before diving in, understand how content is organized:

- **Categories** define types of groups (e.g., "Projects", "People", "Locations")
- **Groups** are containers within categories that can own resources, notes, and sub-groups
- **Resources** are files with metadata
- **Notes** are text content with optional date ranges
- **Tags** are flat labels applied across all entity types

## Step 1: Create a Category

Categories are the foundation. Let's create one for projects.

1. Click **Categories** in the navigation
2. Click **New Category**
3. Enter name: "Projects"
4. Click **Create**

## Step 2: Create a Group

Groups live within categories and contain your actual content.

1. Click **Groups** in the navigation
2. Click **New Group**
3. Enter name: "Website Redesign"
4. Select Category: "Projects"
5. Click **Create**

## Step 3: Upload Resources

Add files to your group.

1. Open your new group by clicking its name
2. In the "Own Entities" section, find "Resources" and click **Add**
3. Upload one or more files
4. The files are now owned by this group

## Step 4: Create a Note

Add documentation or notes to your group.

1. Still in your group, find "Notes" in "Own Entities" and click **Add**
2. Enter a name like "Project Requirements"
3. Write your content using Markdown
4. Click **Create**

## Step 5: Add Tags

Tags help you find content across groups.

1. Open any resource or note
2. In the sidebar, find the Tags section
3. Start typing a tag name
4. Select an existing tag or create a new one

## Step 6: Use Global Search

Find anything instantly with the global search.

1. Press **Cmd+K** (Mac) or **Ctrl+K** (Windows/Linux)
2. Start typing
3. Results show resources, notes, groups, and tags
4. Use arrow keys to navigate, Enter to select

## What's Next?

- Learn about [Core Concepts](/concepts/overview) to understand the data model
- Explore the [User Guide](/user-guide/navigation) for detailed feature documentation
- Set up [proper deployment](/deployment/docker) for long-term use
```

**Step 2: Commit**

```bash
git add docs-site/docs/getting-started/first-steps.md
git commit -m "docs: add first steps guide"
```

---

### Task 10: Create Core Concepts - Overview

**Files:**
- Create: `docs-site/docs/concepts/overview.md`

**Step 1: Write overview.md**

```markdown
---
sidebar_position: 1
---

# Core Concepts Overview

Mahresources organizes information through interconnected entities. Understanding these relationships is key to using the system effectively.

## Entity Types

### Resources

Files stored in the system. Any file type is supported, with automatic thumbnail generation for images, videos, PDFs, and office documents.

- Have metadata (custom key-value pairs)
- Can be tagged
- Support versioning
- Can detect similar images automatically

### Notes

Text content with optional date ranges. Supports Markdown formatting.

- Have a NoteType for categorization
- Can have start and end dates
- Can be attached to resources and groups
- Can be tagged

### Groups

Hierarchical containers that organize resources and notes.

- Belong to exactly one Category
- Can own resources, notes, and sub-groups
- Can also relate to (not own) other groups, resources, and notes
- Support cloning and merging

### Categories

Types of groups. Every group must belong to a category.

- Examples: "Projects", "People", "Locations", "Events"
- Can have custom HTML headers and sidebars
- Help structure your content organization

### Tags

Flat labels applied across all entity types.

- Cross-cutting organization
- Applied to resources, notes, and groups
- Searchable and filterable

### Relations

Typed connections between groups.

- RelationTypes define the kind of connection (e.g., "works at", "parent of")
- Relations connect two groups with optional name and description
- Enable graph-like navigation

## Ownership vs Relationships

A key concept is the difference between **ownership** and **relationships**:

**Ownership (hierarchical):**
- A group *owns* its resources, notes, and sub-groups
- Owned entities appear in the "Own Entities" section
- Deleting a group affects owned entities

**Relationships (many-to-many):**
- Groups can *relate to* other groups, resources, and notes
- Related entities appear in the "Related Entities" section
- Relationships are bidirectional references, not ownership

## Common Features

All entities share some capabilities:

- **Metadata** - Custom JSON key-value pairs for additional data
- **Full-Text Search** - Names and descriptions are indexed
- **Timestamps** - Created and updated times tracked automatically
- **Inline Editing** - Edit names and descriptions directly in the UI
```

**Step 2: Commit**

```bash
mkdir -p docs-site/docs/concepts
git add docs-site/docs/concepts/overview.md
git commit -m "docs: add core concepts overview"
```

---

### Task 11: Create Remaining Concept Pages

**Files:**
- Create: `docs-site/docs/concepts/resources.md`
- Create: `docs-site/docs/concepts/notes.md`
- Create: `docs-site/docs/concepts/groups.md`
- Create: `docs-site/docs/concepts/tags-categories.md`
- Create: `docs-site/docs/concepts/relationships.md`

**Step 1: Write resources.md**

```markdown
---
sidebar_position: 2
---

# Resources

Resources are files stored in Mahresources. Any file type is supported.

## File Storage

When you upload a file, Mahresources:

1. Calculates a hash (SHA-256) for deduplication
2. Stores the file in the configured storage location
3. Generates a thumbnail (for supported types)
4. Extracts metadata where possible (dimensions, duration)

Files are stored using a hash-based directory structure to prevent filename conflicts and enable deduplication.

## Thumbnails

Automatic thumbnail generation for:

- **Images** - JPEG, PNG, GIF, WebP, BMP, TIFF
- **Videos** - MP4, WebM, MOV, AVI (requires ffmpeg)
- **PDFs** - First page rendered as image
- **Office Documents** - Word, Excel, PowerPoint (requires LibreOffice)

## Versioning

Resources support version control:

- Upload a new file to an existing resource to create a version
- All previous versions are preserved
- Compare versions side-by-side (images, text, binary)
- Revert to any previous version

See [Versioning](/features/versioning) for details.

## Image Similarity

For images, Mahresources calculates perceptual hashes to find similar or duplicate images:

- Similarity detection runs in the background
- Similar images appear on the resource detail page
- One-click merge to deduplicate

See [Image Similarity](/features/image-similarity) for details.

## Metadata

Resources can have custom metadata as key-value pairs:

- Add any data relevant to your use case
- Metadata is searchable
- Bulk operations can add metadata to multiple resources

## Relationships

Resources can be connected to:

- **Groups** - Owned by or related to groups
- **Notes** - Attached to notes
- **Tags** - Labeled with tags
```

**Step 2: Write notes.md**

```markdown
---
sidebar_position: 3
---

# Notes

Notes are text content with optional date ranges, supporting Markdown formatting.

## Creating Notes

When creating a note, you can specify:

- **Name** - A title for the note
- **Description** - The main content (Markdown supported)
- **NoteType** - Categorization (e.g., "Meeting Notes", "Ideas")
- **Start Date** - Optional start time
- **End Date** - Optional end time
- **Owner** - Group that owns this note

## Markdown Support

The description field supports Markdown:

- Headers, lists, links
- Code blocks with syntax highlighting
- Tables
- Blockquotes

## NoteTypes

NoteTypes categorize notes and can customize their display:

- Create types like "Meeting Notes", "Ideas", "References"
- NoteTypes can have custom HTML in headers and sidebars
- Filter notes by type in list views

## Date Ranges

Optional start and end dates enable time-based organization:

- Track when something happened
- Filter notes by date range
- Useful for events, meetings, projects

## Wide Display Mode

For long notes, use the "Wide display" link on the note detail page for a distraction-free reading view.

## Relationships

Notes can be connected to:

- **Owner (Group)** - The group that owns this note
- **Resources** - Files attached to this note
- **Groups** - Groups this note is related to
- **Tags** - Labels applied to this note
```

**Step 3: Write groups.md**

```markdown
---
sidebar_position: 4
---

# Groups

Groups are hierarchical containers that organize your content.

## Categories

Every group belongs to exactly one Category. Categories define types of groups:

- "Projects" - for project-related groups
- "People" - for person profiles
- "Locations" - for places
- "Events" - for events or meetings

Create categories that match your organizational needs.

## Ownership Hierarchy

Groups can own:

- **Resources** - Files belonging to this group
- **Notes** - Text content belonging to this group
- **Sub-Groups** - Child groups for nested organization

Owned entities appear in the "Own Entities" section on the group detail page.

## Related Entities

Groups can also relate to entities without owning them:

- **Related Groups** - Other groups connected to this one
- **Related Resources** - Files referenced but not owned
- **Related Notes** - Notes referenced but not owned

Related entities appear in the "Related Entities" section.

## Cloning

Clone a group to copy its structure:

- Creates a new group with the same name (suffixed)
- Copies category and metadata
- Does not copy owned entities

Useful for creating templates.

## Merging

Merge multiple groups into one:

- Select a "winner" group
- Other groups' entities are transferred to the winner
- Other groups are deleted

Useful for consolidating duplicates.

## Custom Display

Categories can have custom HTML for headers and sidebars, enabling:

- Custom layouts for specific group types
- Embedded widgets or visualizations
- Calculated fields using Alpine.js

See [Custom Templates](/features/custom-templates) for details.
```

**Step 4: Write tags-categories.md**

```markdown
---
sidebar_position: 5
---

# Tags and Categories

Tags and Categories serve different organizational purposes.

## Tags

Tags are flat labels applied across all entity types.

### Characteristics

- **Cross-cutting** - Same tag can apply to resources, notes, and groups
- **Flat structure** - No hierarchy, just labels
- **Searchable** - Filter lists by tags
- **Bulk operations** - Add/remove tags from multiple entities

### Use Cases

- Status labels: "To Review", "Archived", "Important"
- Topic labels: "Finance", "Marketing", "Personal"
- Any cross-cutting concern that spans categories

### Adding Tags

1. Open any entity (resource, note, or group)
2. Find the Tags section in the sidebar
3. Start typing to search existing tags or create new ones
4. Click a tag to add it

### Filtering by Tags

In any list view:
1. Use the filter panel
2. Select one or more tags
3. List shows only entities with those tags

## Categories

Categories are types of groups. Every group must belong to exactly one category.

### Characteristics

- **Required** - Groups must have a category
- **Exclusive** - Groups belong to only one category
- **Customizable** - Custom HTML headers and sidebars
- **Structural** - Define the shape of your organization

### Use Cases

- Entity types: "People", "Projects", "Locations"
- Domains: "Work", "Personal", "Archive"
- Any top-level organizational structure

### Custom HTML

Categories can have custom HTML in:

- **Header** - Displayed at the top of group pages in this category
- **Sidebar** - Displayed in the sidebar of group pages

This enables specialized displays for different group types. See [Custom Templates](/features/custom-templates).
```

**Step 5: Write relationships.md**

```markdown
---
sidebar_position: 6
---

# Relationships

Relationships create typed connections between groups, enabling graph-like navigation.

## Relation Types

Before creating relationships, define the types of connections you need:

- **"works at"** - Connect a person to an organization
- **"parent of"** - Connect a parent to a child
- **"located in"** - Connect an entity to a location
- **"related to"** - Generic connection

### Creating Relation Types

1. Click **Relation Types** in the navigation
2. Click **New Relation Type**
3. Enter a name (e.g., "works at")
4. Optionally add a description
5. Click **Create**

## Creating Relationships

Once you have relation types, create relationships between groups:

1. Open a group detail page
2. Find the "Relations" section
3. Click **Add** next to "Relations"
4. Select the relation type
5. Search for and select the target group
6. Optionally add a name and description for this specific relationship
7. Click **Create**

## Navigating Relationships

On a group detail page:

- **Relations** - Connections from this group to others
- **Reverse Relations** - Connections from other groups to this one

Click any related group to navigate to it.

## Use Cases

### Organization Chart

- Create relation type "reports to"
- Connect employee groups to manager groups
- Navigate the hierarchy via relationships

### Project Network

- Create relation type "depends on"
- Connect project groups to their dependencies
- Visualize project relationships

### Knowledge Graph

- Create multiple relation types
- Build a network of connected concepts
- Navigate by following relationships
```

**Step 6: Commit all concept pages**

```bash
git add docs-site/docs/concepts/
git commit -m "docs: add core concepts documentation (resources, notes, groups, tags, relationships)"
```

---

## Phase 3: User Guide Pages

### Task 12: Create User Guide - Navigation

**Files:**
- Create: `docs-site/docs/user-guide/navigation.md`

**Step 1: Write navigation.md**

```markdown
---
sidebar_position: 1
---

# Navigation

Learn how to navigate the Mahresources interface.

## Top Navigation Bar

The navigation bar at the top provides access to all main sections:

- **Resources** - File management
- **Notes** - Text content
- **Groups** - Hierarchical organization
- **Tags** - Label management
- **Categories** - Group types
- **Queries** - Saved searches
- **Relation Types** - Connection types

On mobile, the navigation collapses into a hamburger menu.

## Global Search

Access global search with **Cmd+K** (Mac) or **Ctrl+K** (Windows/Linux).

Global search finds:
- Resources by name
- Notes by name and content
- Groups by name
- Tags by name

Use arrow keys to navigate results, Enter to select, Escape to close.

## List Views

All entity types have list views with common features:

### Pagination

- Navigate pages at the bottom
- Adjust items per page

### Sorting

- Click column headers to sort
- Some lists support multi-column sorting

### Filtering

- Use the filter panel to narrow results
- Filter by tags, categories, dates, and more
- Combine multiple filters

## Detail Views

Clicking an entity opens its detail view:

### Main Content

- Description and core information
- Related entities in collapsible sections

### Sidebar

- Owner information
- Tags with add/remove
- Metadata display
- Action buttons

### Inline Editing

- Click names or descriptions to edit inline
- Changes save automatically
```

**Step 2: Commit**

```bash
mkdir -p docs-site/docs/user-guide
git add docs-site/docs/user-guide/navigation.md
git commit -m "docs: add navigation user guide"
```

---

### Task 13: Create Remaining User Guide Pages

**Files:**
- Create: `docs-site/docs/user-guide/managing-resources.md`
- Create: `docs-site/docs/user-guide/managing-notes.md`
- Create: `docs-site/docs/user-guide/organizing-with-groups.md`
- Create: `docs-site/docs/user-guide/search.md`
- Create: `docs-site/docs/user-guide/bulk-operations.md`

**Step 1: Write managing-resources.md**

```markdown
---
sidebar_position: 2
---

# Managing Resources

Resources are files stored in Mahresources. This guide covers all resource operations.

## Uploading Files

### From Your Computer

1. Navigate to **Resources** > **New Resource**
2. Drag and drop files or click to select
3. Multiple files can be uploaded at once
4. Fill in optional fields (owner, groups, tags)
5. Click **Create**

### From URL

1. Navigate to **Resources** > **New Resource**
2. Enter a URL in the "Remote URL" field
3. Mahresources downloads and stores the file
4. Fill in optional fields
5. Click **Create**

### From Server Path

If the file already exists on the server:

1. Use the API endpoint `POST /v1/resource/local`
2. Provide the local file path
3. File is copied or moved into Mahresources storage

## Viewing Resources

### Preview

The resource detail page shows a preview (thumbnail) in the sidebar. Click to view full size.

### Full View

Click the preview image or use the "View" link to see the original file in your browser.

### Download

Right-click the preview and "Save As", or use the download link.

## Editing Resources

### Basic Information

- **Name** - Click to edit inline
- **Description** - Click to edit inline, supports Markdown

### Metadata

Add custom key-value pairs in the sidebar:

1. Find the "Meta Data" section
2. Add, edit, or remove key-value pairs
3. Changes save automatically

### Tags

Add or remove tags in the sidebar:

1. Find the "Tags" section
2. Type to search or create tags
3. Click to add, X to remove

## Image Operations

For image resources, additional operations are available:

### Rotate

1. Find "Rotate 90 Degrees" in the sidebar
2. Click **Rotate**
3. Image is rotated clockwise 90 degrees

### Recalculate Dimensions

If dimensions are incorrect:

1. Find "Update Dimensions" in the sidebar
2. Click **Recalculate Dimensions**

## Deleting Resources

1. Open the resource detail page
2. Click **Delete** in the sidebar
3. Confirm the deletion

:::warning
Deleting a resource removes the file permanently. There is no recycle bin.
:::
```

**Step 2: Write managing-notes.md**

```markdown
---
sidebar_position: 3
---

# Managing Notes

Notes are text content with optional date ranges.

## Creating Notes

1. Navigate to **Notes** > **New Note**
2. Fill in the fields:
   - **Name** - Required title
   - **Description** - Content (Markdown supported)
   - **Note Type** - Optional categorization
   - **Start Date** - Optional
   - **End Date** - Optional
   - **Owner** - Group that owns this note
3. Click **Create**

## Note Types

Note types categorize notes and can customize their display.

### Creating Note Types

1. Navigate to **Note Types** > **New Note Type**
2. Enter a name (e.g., "Meeting Notes")
3. Optionally add description and custom HTML
4. Click **Create**

### Using Note Types

When creating or editing a note, select a note type from the dropdown.

## Editing Notes

### Inline Editing

- Click the name to edit inline
- Click the description to edit inline

### Full Edit

1. Open the note detail page
2. Click **Edit** to access the full form
3. Modify any field
4. Click **Update**

## Attaching Resources

Connect files to your notes:

1. Open the note detail page
2. Find the "Resources" section
3. Click **Add**
4. Search for and select resources
5. Resources are now attached

## Linking to Groups

Connect notes to groups:

1. Open the note detail page
2. Find the "Groups" section
3. Click **Add**
4. Search for and select groups

## Wide Display Mode

For reading long notes:

1. Open the note detail page
2. Click "Wide display" at the top
3. Note content displays in a full-width, distraction-free view

## Deleting Notes

1. Open the note detail page
2. Click **Delete**
3. Confirm the deletion
```

**Step 3: Write organizing-with-groups.md**

```markdown
---
sidebar_position: 4
---

# Organizing with Groups

Groups are hierarchical containers for organizing your content.

## Creating Groups

1. Navigate to **Groups** > **New Group**
2. Fill in the fields:
   - **Name** - Required
   - **Category** - Required, select from existing categories
   - **Description** - Optional, supports Markdown
   - **Owner** - Optional parent group for hierarchy
3. Click **Create**

## Group Hierarchy

### Setting an Owner

To create a hierarchy, set a group's owner to another group:

1. Edit the group
2. Select an owner group
3. The group becomes a child of the owner

### Navigating Hierarchy

- On a group page, owned sub-groups appear in "Own Entities"
- Click a sub-group to navigate down
- Use the owner link to navigate up

## Adding Content

### Owned Entities

Entities owned by a group:

1. Open the group detail page
2. Expand "Own Entities"
3. Click **Add** next to Resources, Notes, or Sub-Groups
4. Create or select entities to add

### Related Entities

Entities related but not owned:

1. Open the group detail page
2. Expand "Related Entities"
3. Click **Add** next to the entity type
4. Search for and select existing entities

## Creating Relations

Connect groups to other groups with typed relationships:

1. Open the group detail page
2. Expand "Relations"
3. Click **Add**
4. Select a relation type
5. Select the target group
6. Click **Create**

See [Relationships](/concepts/relationships) for more.

## Cloning Groups

Create a copy of a group's structure:

1. Open the group detail page
2. Find "Clone group?" in the sidebar
3. Click **Clone**

The new group has the same name (with suffix), category, and metadata, but no owned entities.

## Merging Groups

Combine multiple groups into one:

1. Open the target ("winner") group
2. Find "Merge others with this group?" in the sidebar
3. Search for groups to merge
4. Click **Merge**

All entities from merged groups transfer to the winner. Merged groups are deleted.
```

**Step 4: Write search.md**

```markdown
---
sidebar_position: 5
---

# Search

Mahresources provides multiple ways to find your content.

## Global Search

The fastest way to find anything.

### Opening Global Search

- Press **Cmd+K** (Mac) or **Ctrl+K** (Windows/Linux)
- Or click the search button in the navigation bar

### Using Global Search

1. Start typing your search term
2. Results appear instantly
3. Results include resources, notes, groups, and tags
4. Use arrow keys to navigate
5. Press Enter to open the selected result
6. Press Escape to close

### Result Types

Results are labeled by type:
- Resources show file type indicators
- Notes show note icon
- Groups show their category
- Tags show tag icon

## List View Filtering

Each list view has filtering capabilities.

### Basic Filters

1. Open any list view (Resources, Notes, Groups, etc.)
2. Use the filter panel above the list
3. Apply filters:
   - **Tags** - Select one or more tags
   - **Categories** - Filter groups by category
   - **Date Range** - Filter by creation or update date
   - **Owner** - Filter by owning group

### Combining Filters

Multiple filters are combined with AND logic - results must match all filters.

## Full-Text Search

Names and descriptions are indexed for full-text search.

### In List Views

1. Use the search/filter panel
2. Enter text to search
3. Matches in names and descriptions are found

### Search Operators

Basic text matching is supported. Enter words to find entities containing those words.

## Saved Queries

Save complex filter combinations for reuse.

### Creating a Saved Query

1. Apply filters in any list view
2. Navigate to **Queries** > **New Query**
3. Copy the current URL parameters
4. Give the query a name
5. Click **Create**

### Using Saved Queries

1. Navigate to **Queries**
2. Click a saved query
3. The list view opens with saved filters applied

See [Saved Queries](/features/saved-queries) for more.
```

**Step 5: Write bulk-operations.md**

```markdown
---
sidebar_position: 6
---

# Bulk Operations

Perform actions on multiple entities at once.

## Selecting Multiple Items

In list views, select multiple items:

1. Click the checkbox on each item you want to select
2. Or use "Select All" to select all visible items
3. Selected count appears at the top

## Available Bulk Operations

### Add Tags

Add tags to all selected items:

1. Select items
2. Click **Add Tags**
3. Search for or create tags
4. Click **Apply**

Tags are added to all selected items.

### Remove Tags

Remove tags from all selected items:

1. Select items
2. Click **Remove Tags**
3. Select tags to remove
4. Click **Apply**

### Add Metadata

Add key-value pairs to all selected items:

1. Select items
2. Click **Add Meta**
3. Enter key and value
4. Click **Apply**

Metadata is merged with existing metadata.

### Delete

Delete all selected items:

1. Select items
2. Click **Delete**
3. Confirm the deletion

:::warning
Bulk delete is permanent. There is no recycle bin.
:::

## Merging Duplicates

### Merge Resources

When similar resources are detected:

1. Open a resource detail page
2. Similar resources appear in "Similar Resources" section
3. Click **Merge Others To This**
4. Other resources are deleted, their relationships transferred

### Merge Groups

Combine multiple groups:

1. Open the target group
2. In sidebar, find "Merge others with this group?"
3. Select groups to merge
4. Click **Merge**

All content from merged groups transfers to the target.

## Best Practices

- **Review before deleting** - Bulk delete cannot be undone
- **Use tags for staging** - Tag items as "To Delete" before bulk deleting
- **Test with small batches** - Verify operations work as expected before large batches
```

**Step 6: Commit all user guide pages**

```bash
git add docs-site/docs/user-guide/
git commit -m "docs: add user guide (resources, notes, groups, search, bulk operations)"
```

---

## Phase 4: Configuration Pages

### Task 14: Create Configuration Pages

**Files:**
- Create: `docs-site/docs/configuration/overview.md`
- Create: `docs-site/docs/configuration/database.md`
- Create: `docs-site/docs/configuration/storage.md`
- Create: `docs-site/docs/configuration/advanced.md`

**Step 1: Write overview.md**

```markdown
---
sidebar_position: 1
---

# Configuration Overview

Mahresources is configured through environment variables or command-line flags.

:::danger Security Reminder

Mahresources has no authentication. Ensure it runs only on trusted networks. See [Reverse Proxy](/deployment/reverse-proxy) for adding authentication.

:::

## Configuration Methods

### Environment Variables

Set in your shell or a `.env` file:

```bash
export DB_TYPE=SQLITE
export DB_DSN=./data/app.db
export FILE_SAVE_PATH=./files
export BIND_ADDRESS=:8080
```

### Command-Line Flags

Pass when starting the application:

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./data/app.db \
  -file-save-path=./files \
  -bind-address=:8080
```

### Precedence

Command-line flags take precedence over environment variables.

## Quick Reference

| Flag | Environment Variable | Description | Default |
|------|---------------------|-------------|---------|
| `-bind-address` | `BIND_ADDRESS` | Server address:port | `:8181` |
| `-db-type` | `DB_TYPE` | Database type (SQLITE, POSTGRES) | `SQLITE` |
| `-db-dsn` | `DB_DSN` | Database connection string | Required |
| `-file-save-path` | `FILE_SAVE_PATH` | File storage directory | Required* |
| `-ephemeral` | `EPHEMERAL=1` | Memory-only mode | `false` |
| `-ffmpeg-path` | `FFMPEG_PATH` | Path to ffmpeg | Auto-detect |
| `-libreoffice-path` | `LIBREOFFICE_PATH` | Path to LibreOffice | Auto-detect |

*Not required in ephemeral mode.

## Common Configurations

### Development/Testing

```bash
./mahresources -ephemeral -bind-address=:8080
```

### Simple Persistent Setup

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./data/mahresources.db \
  -file-save-path=./files \
  -bind-address=:8080
```

### Production with PostgreSQL

```bash
./mahresources \
  -db-type=POSTGRES \
  -db-dsn="host=localhost user=mah password=secret dbname=mahresources" \
  -file-save-path=/var/lib/mahresources/files \
  -bind-address=127.0.0.1:8080
```

See the following pages for detailed configuration options:
- [Database Configuration](/configuration/database)
- [Storage Configuration](/configuration/storage)
- [Advanced Options](/configuration/advanced)
```

**Step 2: Write database.md**

```markdown
---
sidebar_position: 2
---

# Database Configuration

Mahresources supports SQLite and PostgreSQL databases.

## SQLite (Default)

SQLite is the simplest option, storing everything in a single file.

### Basic Setup

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./data/mahresources.db
```

### Build Requirements

SQLite requires specific build tags:

```bash
go build --tags 'json1 fts5'
```

- `json1` - Enables JSON query functions
- `fts5` - Enables full-text search

### Connection Limits

For concurrent access (e.g., during tests):

```bash
./mahresources -max-db-connections=2
```

Reduces SQLite lock contention.

## PostgreSQL

PostgreSQL is recommended for larger deployments or multi-user scenarios.

### Basic Setup

```bash
./mahresources \
  -db-type=POSTGRES \
  -db-dsn="host=localhost port=5432 user=mahresources password=secret dbname=mahresources sslmode=disable"
```

### Connection String Format

Standard PostgreSQL connection string:

```
host=HOST port=PORT user=USER password=PASS dbname=DB sslmode=MODE
```

### Read Replica

For read-heavy workloads, configure a read-only connection:

```bash
./mahresources \
  -db-type=POSTGRES \
  -db-dsn="host=primary ..." \
  -db-readonly-dsn="host=replica ..."
```

Read operations use the replica when available.

## Database Logging

Log database queries for debugging:

```bash
# Log to stdout
./mahresources -db-log-file=STDOUT

# Log to file
./mahresources -db-log-file=/var/log/mahresources/db.log

# Disable (default)
./mahresources -db-log-file=
```

## Full-Text Search

FTS is enabled by default. For large existing databases, skip initialization:

```bash
./mahresources -skip-fts
```

This speeds up startup but disables text search.

## Large Database Options

For databases with many resources:

```bash
./mahresources \
  -skip-version-migration \
  -skip-fts
```

- `-skip-version-migration` - Skip version migration at startup
- `-skip-fts` - Skip FTS initialization

These can be re-enabled after initial load.
```

**Step 3: Write storage.md**

```markdown
---
sidebar_position: 3
---

# Storage Configuration

Configure where and how Mahresources stores files.

## Primary Storage

Set the main file storage directory:

```bash
./mahresources -file-save-path=/var/lib/mahresources/files
```

This directory must:
- Exist before starting
- Be writable by the Mahresources process
- Have sufficient disk space

## File Organization

Files are stored using hash-based paths:

```
/files/ab/cd/abcdef1234567890...
```

This prevents filename conflicts and enables deduplication.

## Alternative Filesystems

Spread storage across multiple locations:

### Using Flags

```bash
./mahresources \
  -file-save-path=/primary \
  -alt-fs=archive:/mnt/archive \
  -alt-fs=fast:/ssd/cache
```

### Using Environment Variables

```bash
export FILE_SAVE_PATH=/primary
export FILE_ALT_COUNT=2
export FILE_ALT_NAME_1=archive
export FILE_ALT_PATH_1=/mnt/archive
export FILE_ALT_NAME_2=fast
export FILE_ALT_PATH_2=/ssd/cache
```

## Memory Filesystem

For testing, use in-memory storage:

```bash
./mahresources -memory-fs
```

Files are lost on restart.

## Ephemeral Mode

Combine memory database and filesystem:

```bash
./mahresources -ephemeral
```

Equivalent to `-memory-db -memory-fs`.

## Seed Filesystem

Start with existing files in read-only mode:

```bash
./mahresources \
  -seed-fs=/original-files \
  -file-save-path=/overlay
```

- Reads from `/original-files`
- Writes go to `/overlay`
- Original files are never modified

Useful for:
- Testing with production data
- Creating demo environments
- Staging migrations

## Copy-on-Write Patterns

### Fully Ephemeral with Seed

```bash
./mahresources \
  -ephemeral \
  -seed-db=./production.db \
  -seed-fs=./production-files
```

Test with real data, changes lost on exit.

### Persistent Overlay

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./changes.db \
  -seed-fs=./original \
  -file-save-path=./changes
```

Original files unchanged, new files in overlay.
```

**Step 4: Write advanced.md**

```markdown
---
sidebar_position: 4
---

# Advanced Configuration

Additional configuration options for specialized setups.

## Thumbnail Generation

### ffmpeg (Videos)

For video thumbnail generation:

```bash
# Auto-detect from PATH
./mahresources

# Explicit path
./mahresources -ffmpeg-path=/usr/local/bin/ffmpeg
```

### LibreOffice (Office Documents)

For Word, Excel, PowerPoint thumbnails:

```bash
# Auto-detect soffice or libreoffice in PATH
./mahresources

# Explicit path
./mahresources -libreoffice-path=/usr/bin/soffice
```

## Hash Worker

The hash worker processes images in the background for similarity detection.

### Configuration

```bash
./mahresources \
  -hash-worker-count=4 \
  -hash-batch-size=500 \
  -hash-poll-interval=1m \
  -hash-similarity-threshold=10
```

| Option | Description | Default |
|--------|-------------|---------|
| `-hash-worker-count` | Concurrent workers | 4 |
| `-hash-batch-size` | Resources per batch | 500 |
| `-hash-poll-interval` | Time between batches | 1m |
| `-hash-similarity-threshold` | Max Hamming distance | 10 |

### Disabling

For resource-constrained environments:

```bash
./mahresources -hash-worker-disabled
```

## Remote Download Timeouts

When downloading resources from URLs:

```bash
./mahresources \
  -remote-connect-timeout=30s \
  -remote-idle-timeout=60s \
  -remote-overall-timeout=30m
```

| Option | Description | Default |
|--------|-------------|---------|
| `-remote-connect-timeout` | Connection timeout | 30s |
| `-remote-idle-timeout` | Idle transfer timeout | 60s |
| `-remote-overall-timeout` | Total download timeout | 30m |

## Bind Address

### All Interfaces

```bash
./mahresources -bind-address=:8080
```

:::danger
Binding to all interfaces exposes the application to the network. Only do this on trusted networks.
:::

### Localhost Only

```bash
./mahresources -bind-address=127.0.0.1:8080
```

Safest option - only accessible locally. Use with a reverse proxy for remote access.

### Specific Interface

```bash
./mahresources -bind-address=192.168.1.100:8080
```

## Startup Optimizations

For large databases:

```bash
./mahresources \
  -skip-fts \
  -skip-version-migration
```

These skip potentially slow initialization steps. Features may be limited until indexes are built.
```

**Step 5: Commit configuration pages**

```bash
mkdir -p docs-site/docs/configuration
git add docs-site/docs/configuration/
git commit -m "docs: add configuration documentation"
```

---

## Phase 5: Advanced Features Pages

### Task 15: Create Advanced Features Pages

**Files:**
- Create: `docs-site/docs/features/versioning.md`
- Create: `docs-site/docs/features/image-similarity.md`
- Create: `docs-site/docs/features/saved-queries.md`
- Create: `docs-site/docs/features/custom-templates.md`

**Step 1: Write versioning.md**

```markdown
---
sidebar_position: 1
---

# Resource Versioning

Mahresources tracks versions of resources, preserving history when files are updated.

## How Versioning Works

When you upload a new file to an existing resource:

1. The current file becomes a previous version
2. The new file becomes the current version
3. All versions are preserved on disk
4. Version history is tracked in the database

## Viewing Version History

1. Open a resource detail page
2. Find the "Versions" panel
3. All versions are listed with timestamps

## Comparing Versions

Compare two versions side-by-side:

1. In the version panel, select two versions
2. Click **Compare**
3. A comparison view opens

### Comparison Modes

Different content types have different comparison views:

- **Images** - Side-by-side visual comparison, slider overlay
- **Text** - Line-by-line diff with additions and deletions highlighted
- **Binary** - Hex diff for binary files
- **PDFs** - Page-by-page comparison

## Reverting to a Previous Version

1. Open the version history
2. Find the version you want to restore
3. Click **Revert**
4. The selected version becomes current (previous current becomes a version)

## Storage Implications

All versions are stored on disk:

- Each version is a complete file copy
- Storage grows with each version
- There is no automatic cleanup

Plan storage capacity accordingly for frequently-updated resources.

## Version Migration

At startup, Mahresources may run version migration for existing resources. For large databases, skip this:

```bash
./mahresources -skip-version-migration
```

Run migration later when convenient.
```

**Step 2: Write image-similarity.md**

```markdown
---
sidebar_position: 2
---

# Image Similarity Detection

Mahresources can automatically detect similar or duplicate images using perceptual hashing.

## How It Works

1. When images are uploaded, they're queued for hash calculation
2. A background worker calculates perceptual hashes (pHash)
3. Hashes are compared to find similar images
4. Similar pairs are stored for quick lookup

## Viewing Similar Images

1. Open an image resource detail page
2. Find the "Similar Resources" section
3. Similar images are listed with similarity scores

If no similar images are shown, either:
- The image hasn't been processed yet (check back later)
- No similar images exist in your library

## Merging Duplicates

When duplicates are found:

1. Open one of the duplicate resources
2. In "Similar Resources", all matches are shown
3. Click **Merge Others To This**
4. Confirm the merge

The other resources are deleted, and their relationships (tags, groups, notes) are transferred to the remaining resource.

## Configuration

### Similarity Threshold

Control how similar images must be to match:

```bash
./mahresources -hash-similarity-threshold=10
```

Lower values = stricter matching (fewer false positives)
Higher values = looser matching (more matches, may include non-duplicates)

Default is 10, which catches most duplicates while avoiding false positives.

### Worker Settings

```bash
./mahresources \
  -hash-worker-count=4 \
  -hash-batch-size=500 \
  -hash-poll-interval=1m
```

Increase worker count for faster processing on multi-core systems.

### Disabling

To disable similarity detection entirely:

```bash
./mahresources -hash-worker-disabled
```

## Supported Formats

Image similarity works with:

- JPEG
- PNG
- GIF
- WebP
- BMP
- TIFF

Other image formats may not be processed.

## Performance Considerations

For large image libraries:

- Initial processing takes time (batched in background)
- CPU usage increases during hash calculation
- Similarity queries are fast (uses pre-computed pairs)

Monitor system resources and adjust worker settings as needed.
```

**Step 3: Write saved-queries.md**

```markdown
---
sidebar_position: 3
---

# Saved Queries

Save complex filter combinations for quick access.

## What Are Saved Queries?

Saved queries store:

- Filter parameters (tags, categories, date ranges)
- Sort order
- Any URL query parameters from a list view

Think of them as bookmarks for filtered views.

## Creating a Saved Query

### Method 1: From a Filtered View

1. Go to any list view (Resources, Notes, Groups)
2. Apply filters until you see the results you want
3. Copy the URL (it contains your filter parameters)
4. Navigate to **Queries** > **New Query**
5. Paste the URL parameters into the query field
6. Give it a name
7. Click **Create**

### Method 2: Direct Creation

1. Navigate to **Queries** > **New Query**
2. Enter a name (e.g., "Untagged Resources")
3. Enter query parameters (e.g., `hasTags=false`)
4. Click **Create**

## Using Saved Queries

1. Navigate to **Queries**
2. Click any saved query
3. You're taken to the list view with filters applied

## Query Parameter Examples

Common filter parameters:

```
# Resources without tags
hasTags=false

# Resources in a specific group
groups=123

# Notes from this year
createdAfter=2024-01-01

# Combined filters
hasTags=false&contentType=image/jpeg
```

## Editing Queries

1. Navigate to **Queries**
2. Click the query name to open it
3. Click **Edit**
4. Modify name or parameters
5. Click **Update**

## Deleting Queries

1. Open the query detail page
2. Click **Delete**
3. Confirm deletion

## Use Cases

### Maintenance Queries

- "Untagged Resources" - Find resources needing categorization
- "Large Files" - Find resources above a size threshold
- "Old Uploads" - Find resources uploaded long ago

### Workflow Queries

- "To Review" - Items tagged for review
- "This Week's Uploads" - Recent additions
- "Project X Materials" - Items in a specific group

### Reporting Queries

- "All PDFs" - Resources of a specific type
- "Meeting Notes 2024" - Notes with specific type and date range
```

**Step 4: Write custom-templates.md**

```markdown
---
sidebar_position: 4
---

# Custom Templates

Categories and NoteTypes can include custom HTML for specialized displays.

:::warning Trusted Content Only

Custom HTML is rendered without sanitization. Only add custom templates on trusted networks where all users are trusted. Malicious HTML could access or modify data.

:::

## Category Custom HTML

Categories can have custom HTML in two locations:

### Custom Header

Displayed at the top of group pages in this category.

```html
<div class="bg-blue-100 p-4 rounded">
  <h2 x-text="entity.Name"></h2>
  <p>Custom header content for this category</p>
</div>
```

### Custom Sidebar

Displayed in the sidebar of group pages in this category.

```html
<div class="mt-4">
  <h3 class="font-bold">Quick Stats</h3>
  <p>Resources: <span x-text="entity.OwnResources?.length || 0"></span></p>
</div>
```

## NoteType Custom HTML

NoteTypes can also have custom headers and sidebars, displayed on notes of that type.

## Alpine.js Integration

Custom HTML has access to entity data via Alpine.js:

```html
<div x-data>
  <!-- entity is available in the scope -->
  <span x-text="entity.Name"></span>
  <span x-text="entity.Description"></span>
  <span x-text="JSON.stringify(entity.Meta)"></span>
</div>
```

## Available Data

In category templates, `entity` contains:

- `ID`, `Name`, `Description`
- `Meta` - Custom metadata object
- `Tags` - Array of tags
- `OwnResources`, `OwnNotes`, `OwnGroups` - Owned entities
- `RelatedResources`, `RelatedNotes`, `RelatedGroups` - Related entities

In note type templates, `entity` contains:

- `ID`, `Name`, `Description`
- `Meta` - Custom metadata object
- `Tags` - Array of tags
- `StartDate`, `EndDate` - Optional dates
- `Resources`, `Groups` - Related entities

## Examples

### Display Metadata as Table

```html
<table class="w-full" x-data>
  <template x-for="[key, value] in Object.entries(entity.Meta || {})">
    <tr>
      <td class="font-bold" x-text="key"></td>
      <td x-text="value"></td>
    </tr>
  </template>
</table>
```

### Conditional Display

```html
<div x-data>
  <template x-if="entity.Meta?.status === 'active'">
    <span class="bg-green-500 text-white px-2 py-1 rounded">Active</span>
  </template>
  <template x-if="entity.Meta?.status === 'archived'">
    <span class="bg-gray-500 text-white px-2 py-1 rounded">Archived</span>
  </template>
</div>
```

### External Widget

```html
<div x-data>
  <iframe
    :src="'https://example.com/widget?id=' + entity.ID"
    class="w-full h-64"
  ></iframe>
</div>
```

## Styling

Custom HTML can use Tailwind CSS classes, which are available in the page.
```

**Step 5: Commit features pages**

```bash
mkdir -p docs-site/docs/features
git add docs-site/docs/features/
git commit -m "docs: add advanced features documentation"
```

---

## Phase 6: API Reference Pages

### Task 16: Create API Reference Pages

**Files:**
- Create: `docs-site/docs/api/overview.md`
- Create: `docs-site/docs/api/resources.md`
- Create: `docs-site/docs/api/notes.md`
- Create: `docs-site/docs/api/groups.md`
- Create: `docs-site/docs/api/other-endpoints.md`

**Step 1: Write overview.md**

```markdown
---
sidebar_position: 1
---

# API Overview

Mahresources provides a REST API for all operations.

:::danger No API Authentication

The API has no authentication. Anyone with network access can perform any operation. Only expose on trusted networks.

:::

## Base URL

All API endpoints are prefixed with `/v1`:

```
http://localhost:8080/v1/resources
http://localhost:8080/v1/notes
http://localhost:8080/v1/groups
```

## Response Formats

### JSON Response

Request JSON with either:

1. **URL suffix**: Add `.json` to the path
   ```
   GET /v1/resources.json
   ```

2. **Accept header**: Set `Accept: application/json`
   ```
   GET /v1/resources
   Accept: application/json
   ```

### HTML Response (Default)

Without JSON indicators, endpoints return HTML pages.

## Request Formats

POST endpoints accept:

- `application/json` - JSON body
- `application/x-www-form-urlencoded` - Form data
- `multipart/form-data` - File uploads

## Pagination

List endpoints support pagination:

```
GET /v1/resources?page=2&perPage=50
```

Response includes:

```json
{
  "data": [...],
  "total": 150,
  "page": 2,
  "perPage": 50
}
```

## Error Responses

Errors return appropriate HTTP status codes:

- `400` - Bad request (invalid parameters)
- `404` - Not found
- `500` - Server error

Error body:

```json
{
  "error": "Description of the error"
}
```

## OpenAPI Specification

Generate the full OpenAPI spec:

```bash
go run ./cmd/openapi-gen
```

Output options:

```bash
# YAML (default)
go run ./cmd/openapi-gen -output openapi.yaml

# JSON
go run ./cmd/openapi-gen -output openapi.json -format json
```

Use the spec with tools like Swagger UI or to generate client libraries.

## Common Patterns

### List with Filters

```bash
curl "http://localhost:8080/v1/resources.json?tags=123&contentType=image/jpeg"
```

### Get Single Item

```bash
curl "http://localhost:8080/v1/resource.json?id=456"
```

### Create

```bash
curl -X POST "http://localhost:8080/v1/note" \
  -H "Content-Type: application/json" \
  -d '{"name": "New Note", "description": "Content"}'
```

### Update

```bash
curl -X POST "http://localhost:8080/v1/note" \
  -H "Content-Type: application/json" \
  -d '{"id": 123, "name": "Updated Name"}'
```

### Delete

```bash
curl -X POST "http://localhost:8080/v1/note/delete" \
  -d "Id=123"
```
```

**Step 2: Write resources.md**

```markdown
---
sidebar_position: 2
---

# Resources API

API endpoints for managing resources (files).

## List Resources

```
GET /v1/resources
```

### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | number | Page number (default: 1) |
| `perPage` | number | Items per page (default: 20, max: 100) |
| `tags` | number[] | Filter by tag IDs |
| `groups` | number[] | Filter by group IDs |
| `contentType` | string | Filter by MIME type |
| `name` | string | Search by name |
| `hasTags` | boolean | Filter by tag presence |

### Example

```bash
curl "http://localhost:8080/v1/resources.json?contentType=image/jpeg&perPage=10"
```

## Get Resource

```
GET /v1/resource?id={id}
```

### Example

```bash
curl "http://localhost:8080/v1/resource.json?id=123"
```

## Create Resource (Upload)

```
POST /v1/resource
Content-Type: multipart/form-data
```

### Form Fields

| Field | Type | Description |
|-------|------|-------------|
| `resource` | file | File to upload (multiple allowed) |
| `name` | string | Resource name (defaults to filename) |
| `description` | string | Description (Markdown) |
| `ownerId` | number | Owner group ID |
| `groups` | number[] | Related group IDs |
| `tags` | number[] | Tag IDs |

### Example

```bash
curl -X POST "http://localhost:8080/v1/resource" \
  -F "resource=@photo.jpg" \
  -F "name=My Photo" \
  -F "ownerId=5"
```

## Create Resource (From URL)

```
POST /v1/resource/remote
```

### Body

```json
{
  "url": "https://example.com/image.jpg",
  "name": "Downloaded Image",
  "ownerId": 5
}
```

## Create Resource (From Server Path)

```
POST /v1/resource/local
```

### Body

```json
{
  "path": "/path/on/server/file.pdf",
  "name": "Server File",
  "ownerId": 5
}
```

## Edit Resource

```
POST /v1/resource/edit
```

### Body

```json
{
  "id": 123,
  "name": "New Name",
  "description": "New description"
}
```

## Delete Resource

```
POST /v1/resource/delete
```

### Body

```
Id=123
```

## View Resource (Original File)

```
GET /v1/resource/view?id={id}
```

Returns the original file with appropriate content type.

## Preview Resource (Thumbnail)

```
GET /v1/resource/preview?id={id}&height={height}
```

Returns a thumbnail image.

## Bulk Operations

### Add Tags

```
POST /v1/resources/addTags
```

```json
{
  "ids": [1, 2, 3],
  "tags": [10, 20]
}
```

### Remove Tags

```
POST /v1/resources/removeTags
```

### Add Metadata

```
POST /v1/resources/addMeta
```

```json
{
  "ids": [1, 2, 3],
  "meta": {"key": "value"}
}
```

### Bulk Delete

```
POST /v1/resources/delete
```

```json
{
  "ids": [1, 2, 3]
}
```

### Merge Resources

```
POST /v1/resources/merge
```

```json
{
  "winner": 1,
  "losers": [2, 3]
}
```

### Rotate Image

```
POST /v1/resources/rotate
```

```json
{
  "id": 123,
  "degrees": 90
}
```
```

**Step 3: Write notes.md**

```markdown
---
sidebar_position: 3
---

# Notes API

API endpoints for managing notes and note types.

## Notes

### List Notes

```
GET /v1/notes
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | number | Page number |
| `perPage` | number | Items per page |
| `tags` | number[] | Filter by tag IDs |
| `groups` | number[] | Filter by group IDs |
| `noteTypeId` | number | Filter by note type |
| `ownerId` | number | Filter by owner group |
| `name` | string | Search by name |

### Get Note

```
GET /v1/note?id={id}
```

### Create/Update Note

```
POST /v1/note
```

#### Body

```json
{
  "id": null,
  "name": "Meeting Notes",
  "description": "Discussion about...",
  "noteTypeId": 1,
  "ownerId": 5,
  "startDate": "2024-01-15T10:00:00Z",
  "endDate": "2024-01-15T11:00:00Z",
  "tags": [1, 2],
  "groups": [10],
  "resources": [100, 101]
}
```

Omit `id` to create, include `id` to update.

### Delete Note

```
POST /v1/note/delete
```

```
Id=123
```

### Edit Name (Inline)

```
POST /v1/note/editName?id={id}
```

Body: new name as plain text

### Edit Description (Inline)

```
POST /v1/note/editDescription?id={id}
```

Body: new description as plain text

## Note Types

### List Note Types

```
GET /v1/note/noteTypes
```

### Create Note Type

```
POST /v1/note/noteType
```

```json
{
  "name": "Meeting Notes",
  "description": "Notes from meetings",
  "customHeader": "<div>...</div>",
  "customSidebar": "<div>...</div>"
}
```

### Edit Note Type

```
POST /v1/note/noteType/edit
```

```json
{
  "id": 1,
  "name": "Updated Name"
}
```

### Delete Note Type

```
POST /v1/note/noteType/delete
```

```
Id=1
```
```

**Step 4: Write groups.md**

```markdown
---
sidebar_position: 4
---

# Groups API

API endpoints for managing groups and relations.

## Groups

### List Groups

```
GET /v1/groups
```

#### Query Parameters

| Parameter | Type | Description |
|-----------|------|-------------|
| `page` | number | Page number |
| `perPage` | number | Items per page |
| `tags` | number[] | Filter by tag IDs |
| `categoryId` | number | Filter by category |
| `ownerId` | number | Filter by owner group |
| `name` | string | Search by name |

### Get Group

```
GET /v1/group?id={id}
```

### Get Group Parents

```
GET /v1/group/parents?id={id}
```

Returns the hierarchy of parent groups.

### Create/Update Group

```
POST /v1/group
```

```json
{
  "id": null,
  "name": "New Project",
  "description": "Project description",
  "categoryId": 1,
  "ownerId": 5,
  "tags": [1, 2],
  "groups": [10],
  "resources": [100],
  "notes": [200]
}
```

### Clone Group

```
POST /v1/group/clone
```

```json
{
  "Id": 123
}
```

### Delete Group

```
POST /v1/group/delete
```

### Bulk Add Tags

```
POST /v1/groups/addTags
```

```json
{
  "ids": [1, 2, 3],
  "tags": [10, 20]
}
```

### Bulk Remove Tags

```
POST /v1/groups/removeTags
```

### Bulk Add Metadata

```
POST /v1/groups/addMeta
```

### Bulk Delete

```
POST /v1/groups/delete
```

### Merge Groups

```
POST /v1/groups/merge
```

```json
{
  "winner": 1,
  "losers": [2, 3]
}
```

## Relations

### Create/Update Relation

```
POST /v1/relation
```

```json
{
  "id": null,
  "name": "Works at",
  "description": "Employment relationship",
  "fromGroupId": 10,
  "toGroupId": 20,
  "relationTypeId": 1
}
```

### Delete Relation

```
POST /v1/relation/delete
```

## Relation Types

### List Relation Types

```
GET /v1/relationTypes
```

### Create Relation Type

```
POST /v1/relationType
```

```json
{
  "name": "works at",
  "description": "Employment relationship"
}
```

### Edit Relation Type

```
POST /v1/relationType/edit
```

### Delete Relation Type

```
POST /v1/relationType/delete
```
```

**Step 5: Write other-endpoints.md**

```markdown
---
sidebar_position: 5
---

# Other API Endpoints

Additional API endpoints for tags, categories, queries, search, and logs.

## Tags

### List Tags

```
GET /v1/tags
```

### Create Tag

```
POST /v1/tag
```

```json
{
  "name": "Important"
}
```

### Delete Tag

```
POST /v1/tag/delete
```

## Categories

### List Categories

```
GET /v1/categories
```

### Create Category

```
POST /v1/category
```

```json
{
  "name": "Projects",
  "description": "Project groups",
  "customHeader": "<div>...</div>",
  "customSidebar": "<div>...</div>"
}
```

### Delete Category

```
POST /v1/category/delete
```

## Saved Queries

### List Queries

```
GET /v1/queries
```

### Create Query

```
POST /v1/query
```

```json
{
  "name": "Untagged Resources",
  "query": "hasTags=false"
}
```

### Delete Query

```
POST /v1/query/delete
```

## Global Search

```
GET /v1/search?q={query}
```

Returns results across all entity types:

```json
{
  "resources": [...],
  "notes": [...],
  "groups": [...],
  "tags": [...]
}
```

## Meta Keys

Get unique metadata keys used across entities:

### Resource Meta Keys

```
GET /v1/resources/meta/keys
```

### Note Meta Keys

```
GET /v1/notes/meta/keys
```

### Group Meta Keys

```
GET /v1/groups/meta/keys
```

Returns an array of unique key names used in metadata.

## Logs

Activity logs (if enabled):

### List Logs

```
GET /v1/logs
```

### Get Log Entry

```
GET /v1/log?id={id}
```
```

**Step 6: Commit API pages**

```bash
mkdir -p docs-site/docs/api
git add docs-site/docs/api/
git commit -m "docs: add API reference documentation"
```

---

## Phase 7: Deployment Pages

### Task 17: Create Deployment Pages

**Files:**
- Create: `docs-site/docs/deployment/docker.md`
- Create: `docs-site/docs/deployment/systemd.md`
- Create: `docs-site/docs/deployment/reverse-proxy.md`
- Create: `docs-site/docs/deployment/backups.md`

**Step 1: Write docker.md**

```markdown
---
sidebar_position: 1
---

# Docker Deployment

Run Mahresources in a Docker container.

:::danger Security Warning

Never expose Docker ports directly to the internet. Use a reverse proxy with authentication. See [Reverse Proxy](/deployment/reverse-proxy).

:::

## Quick Start

```bash
docker run -d \
  --name mahresources \
  -p 127.0.0.1:8080:8080 \
  -v mahresources-data:/data \
  -v mahresources-files:/files \
  -e DB_TYPE=SQLITE \
  -e DB_DSN=/data/mahresources.db \
  -e FILE_SAVE_PATH=/files \
  ghcr.io/egeozcan/mahresources:latest
```

Note: Binding to `127.0.0.1` restricts access to localhost only.

## Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  mahresources:
    image: ghcr.io/egeozcan/mahresources:latest
    container_name: mahresources
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - mahresources-data:/data
      - mahresources-files:/files
    environment:
      - DB_TYPE=SQLITE
      - DB_DSN=/data/mahresources.db
      - FILE_SAVE_PATH=/files
      - BIND_ADDRESS=:8080

volumes:
  mahresources-data:
  mahresources-files:
```

Start with:

```bash
docker compose up -d
```

## With PostgreSQL

```yaml
version: '3.8'

services:
  mahresources:
    image: ghcr.io/egeozcan/mahresources:latest
    restart: unless-stopped
    ports:
      - "127.0.0.1:8080:8080"
    volumes:
      - mahresources-files:/files
    environment:
      - DB_TYPE=POSTGRES
      - DB_DSN=host=db user=mahresources password=secret dbname=mahresources sslmode=disable
      - FILE_SAVE_PATH=/files
    depends_on:
      - db

  db:
    image: postgres:15
    restart: unless-stopped
    volumes:
      - postgres-data:/var/lib/postgresql/data
    environment:
      - POSTGRES_USER=mahresources
      - POSTGRES_PASSWORD=secret
      - POSTGRES_DB=mahresources

volumes:
  mahresources-files:
  postgres-data:
```

## Environment Variables

All configuration options work as environment variables:

```yaml
environment:
  - DB_TYPE=SQLITE
  - DB_DSN=/data/mahresources.db
  - FILE_SAVE_PATH=/files
  - BIND_ADDRESS=:8080
  - FFMPEG_PATH=/usr/bin/ffmpeg
  - HASH_WORKER_COUNT=2
```

## Resource Limits

For production, set resource limits:

```yaml
services:
  mahresources:
    # ...
    deploy:
      resources:
        limits:
          cpus: '2'
          memory: 2G
        reservations:
          cpus: '0.5'
          memory: 512M
```

## Health Check

Add a health check:

```yaml
services:
  mahresources:
    # ...
    healthcheck:
      test: ["CMD", "wget", "-q", "--spider", "http://localhost:8080/"]
      interval: 30s
      timeout: 10s
      retries: 3
```

## Updating

```bash
docker compose pull
docker compose up -d
```

## Viewing Logs

```bash
docker compose logs -f mahresources
```
```

**Step 2: Write systemd.md**

```markdown
---
sidebar_position: 2
---

# Systemd Service

Run Mahresources as a systemd service on Linux.

## Create Service User

```bash
sudo useradd -r -s /bin/false mahresources
sudo mkdir -p /var/lib/mahresources/{data,files}
sudo chown -R mahresources:mahresources /var/lib/mahresources
```

## Install Binary

```bash
sudo cp mahresources /usr/local/bin/
sudo chmod +x /usr/local/bin/mahresources
```

## Create Environment File

Create `/etc/mahresources.env`:

```bash
DB_TYPE=SQLITE
DB_DSN=/var/lib/mahresources/data/mahresources.db
FILE_SAVE_PATH=/var/lib/mahresources/files
BIND_ADDRESS=127.0.0.1:8080
```

Set permissions:

```bash
sudo chmod 600 /etc/mahresources.env
sudo chown mahresources:mahresources /etc/mahresources.env
```

## Create Service File

Create `/etc/systemd/system/mahresources.service`:

```ini
[Unit]
Description=Mahresources Personal Information Manager
After=network.target

[Service]
Type=simple
User=mahresources
Group=mahresources
EnvironmentFile=/etc/mahresources.env
ExecStart=/usr/local/bin/mahresources
Restart=on-failure
RestartSec=5

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/mahresources

[Install]
WantedBy=multi-user.target
```

## Enable and Start

```bash
sudo systemctl daemon-reload
sudo systemctl enable mahresources
sudo systemctl start mahresources
```

## Service Management

```bash
# Check status
sudo systemctl status mahresources

# View logs
sudo journalctl -u mahresources -f

# Restart
sudo systemctl restart mahresources

# Stop
sudo systemctl stop mahresources
```

## Updating

```bash
sudo systemctl stop mahresources
sudo cp new-mahresources /usr/local/bin/mahresources
sudo systemctl start mahresources
```

## Troubleshooting

### Check Logs

```bash
sudo journalctl -u mahresources --no-pager -n 100
```

### Verify Permissions

```bash
ls -la /var/lib/mahresources/
```

### Test Manually

```bash
sudo -u mahresources /usr/local/bin/mahresources -ephemeral
```
```

**Step 3: Write reverse-proxy.md**

```markdown
---
sidebar_position: 3
---

# Reverse Proxy Setup

A reverse proxy is **required** for any non-localhost deployment to add authentication and HTTPS.

:::danger Required for Remote Access

Mahresources has no built-in authentication. You MUST use a reverse proxy with authentication if accessing from outside localhost.

:::

## Nginx

### Basic Configuration

```nginx
server {
    listen 443 ssl http2;
    server_name mahresources.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    # Basic authentication
    auth_basic "Mahresources";
    auth_basic_user_file /etc/nginx/.htpasswd;

    # Increase for large file uploads
    client_max_body_size 1G;

    location / {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Timeouts for large uploads
        proxy_read_timeout 600s;
        proxy_send_timeout 600s;
    }
}
```

### Create Password File

```bash
sudo htpasswd -c /etc/nginx/.htpasswd username
```

## Caddy

Caddy automatically handles HTTPS certificates.

### Caddyfile

```
mahresources.example.com {
    basicauth * {
        username $2a$14$hash...
    }

    reverse_proxy 127.0.0.1:8080

    request_body {
        max_size 1GB
    }
}
```

### Generate Password Hash

```bash
caddy hash-password
```

## Traefik

### docker-compose.yml

```yaml
services:
  traefik:
    image: traefik:v2
    command:
      - "--providers.docker=true"
      - "--entrypoints.websecure.address=:443"
      - "--certificatesresolvers.le.acme.httpchallenge.entrypoint=web"
    ports:
      - "443:443"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock

  mahresources:
    image: ghcr.io/egeozcan/mahresources:latest
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.mah.rule=Host(`mahresources.example.com`)"
      - "traefik.http.routers.mah.tls.certresolver=le"
      - "traefik.http.routers.mah.middlewares=auth"
      - "traefik.http.middlewares.auth.basicauth.users=user:$$hash"
```

## Network Restriction

Alternatively, restrict by IP:

### Nginx

```nginx
location / {
    allow 192.168.1.0/24;
    allow 10.0.0.0/8;
    deny all;

    proxy_pass http://127.0.0.1:8080;
}
```

## VPN-Only Access

The safest approach: only allow access over VPN.

1. Set up WireGuard, OpenVPN, or Tailscale
2. Bind Mahresources to localhost: `-bind-address=127.0.0.1:8080`
3. Configure reverse proxy to only accept connections from VPN interface
```

**Step 4: Write backups.md**

```markdown
---
sidebar_position: 4
---

# Backups

Protect your data with regular backups.

## What to Back Up

1. **Database** - All metadata, relationships, and configuration
2. **File storage** - All uploaded files

## SQLite Backups

### Simple Copy (Application Stopped)

```bash
# Stop the application first
sudo systemctl stop mahresources

# Copy the database
cp /var/lib/mahresources/data/mahresources.db /backup/mahresources-$(date +%Y%m%d).db

# Start the application
sudo systemctl start mahresources
```

### Online Backup (Application Running)

Use SQLite's backup command:

```bash
sqlite3 /var/lib/mahresources/data/mahresources.db ".backup '/backup/mahresources.db'"
```

### Automated Backup Script

Create `/usr/local/bin/backup-mahresources.sh`:

```bash
#!/bin/bash
BACKUP_DIR=/backup/mahresources
DATE=$(date +%Y%m%d-%H%M%S)

mkdir -p $BACKUP_DIR

# Backup database
sqlite3 /var/lib/mahresources/data/mahresources.db ".backup '$BACKUP_DIR/db-$DATE.db'"

# Backup files (incremental)
rsync -av --delete /var/lib/mahresources/files/ $BACKUP_DIR/files/

# Remove backups older than 30 days
find $BACKUP_DIR -name "db-*.db" -mtime +30 -delete
```

Add to crontab:

```bash
0 2 * * * /usr/local/bin/backup-mahresources.sh
```

## PostgreSQL Backups

```bash
pg_dump -h localhost -U mahresources mahresources > /backup/mahresources-$(date +%Y%m%d).sql
```

Or for binary format:

```bash
pg_dump -Fc -h localhost -U mahresources mahresources > /backup/mahresources-$(date +%Y%m%d).dump
```

## File Storage Backups

### rsync (Local or Remote)

```bash
# Local backup
rsync -av --delete /var/lib/mahresources/files/ /backup/files/

# Remote backup
rsync -av --delete /var/lib/mahresources/files/ user@backup-server:/backup/files/
```

### rclone (Cloud Storage)

```bash
rclone sync /var/lib/mahresources/files/ remote:mahresources-backup/files/
```

## Testing Restores

Regularly test your backups:

```bash
# Create test environment
./mahresources \
  -ephemeral \
  -seed-db=/backup/mahresources.db \
  -seed-fs=/backup/files \
  -bind-address=:9090
```

Verify data is accessible at http://localhost:9090.

## Restore Procedure

### Full Restore

```bash
# Stop application
sudo systemctl stop mahresources

# Restore database
cp /backup/mahresources.db /var/lib/mahresources/data/mahresources.db

# Restore files
rsync -av /backup/files/ /var/lib/mahresources/files/

# Fix permissions
sudo chown -R mahresources:mahresources /var/lib/mahresources

# Start application
sudo systemctl start mahresources
```

## Backup Checklist

- [ ] Database backed up regularly
- [ ] File storage backed up regularly
- [ ] Backups stored off-site or on different storage
- [ ] Restore procedure tested quarterly
- [ ] Backup retention policy defined
- [ ] Backup monitoring/alerting configured
```

**Step 5: Commit deployment pages**

```bash
mkdir -p docs-site/docs/deployment
git add docs-site/docs/deployment/
git commit -m "docs: add deployment documentation"
```

---

## Phase 8: Troubleshooting Page

### Task 18: Create Troubleshooting Page

**Files:**
- Create: `docs-site/docs/troubleshooting.md`

**Step 1: Write troubleshooting.md**

```markdown
---
sidebar_position: 100
---

# Troubleshooting

Common issues and solutions.

## Database Issues

### "Database is locked" (SQLite)

**Symptoms:** Operations fail with "database is locked" error.

**Causes:**
- Multiple processes accessing the database
- Long-running queries blocking writes

**Solutions:**

1. Ensure only one Mahresources instance is running
2. Reduce concurrent connections:
   ```bash
   ./mahresources -max-db-connections=2
   ```
3. Check for hung processes:
   ```bash
   ps aux | grep mahresources
   ```

### Slow Startup with Large Database

**Symptoms:** Application takes long to start.

**Solutions:**

1. Skip FTS initialization:
   ```bash
   ./mahresources -skip-fts
   ```

2. Skip version migration:
   ```bash
   ./mahresources -skip-version-migration
   ```

3. Use both for fastest startup:
   ```bash
   ./mahresources -skip-fts -skip-version-migration
   ```

## Thumbnail Issues

### Thumbnails Not Generating

**Symptoms:** Preview images are missing or show placeholders.

**Causes:**
- Missing ffmpeg (videos)
- Missing LibreOffice (office documents)
- File permission issues

**Solutions:**

1. Verify ffmpeg is installed and in PATH:
   ```bash
   which ffmpeg
   ffmpeg -version
   ```

2. Verify LibreOffice is installed:
   ```bash
   which soffice || which libreoffice
   ```

3. Set explicit paths:
   ```bash
   ./mahresources -ffmpeg-path=/usr/bin/ffmpeg -libreoffice-path=/usr/bin/soffice
   ```

4. Check file permissions on storage directory

## Upload Issues

### Upload Fails

**Symptoms:** File uploads fail silently or with errors.

**Causes:**
- Disk space full
- File size limits (reverse proxy)
- Permission issues

**Solutions:**

1. Check disk space:
   ```bash
   df -h /path/to/files
   ```

2. Check reverse proxy limits (Nginx):
   ```nginx
   client_max_body_size 1G;
   ```

3. Check directory permissions:
   ```bash
   ls -la /path/to/files
   ```

## Search Issues

### Search Not Finding Content

**Symptoms:** Full-text search returns no results for known content.

**Causes:**
- FTS was skipped at startup
- Content added before FTS was enabled

**Solutions:**

1. Restart without `-skip-fts`:
   ```bash
   ./mahresources  # without -skip-fts
   ```

2. Verify FTS tables exist (SQLite):
   ```bash
   sqlite3 mahresources.db ".tables" | grep fts
   ```

## Image Similarity Issues

### Similar Images Not Appearing

**Symptoms:** Known duplicate images don't show as similar.

**Causes:**
- Hash worker disabled
- Images not yet processed
- Threshold too strict

**Solutions:**

1. Check hash worker is enabled (not using `-hash-worker-disabled`)

2. Check processing status - new images process in background

3. Adjust threshold:
   ```bash
   ./mahresources -hash-similarity-threshold=15
   ```

## Connection Issues

### Cannot Connect to Application

**Symptoms:** Browser shows connection refused.

**Solutions:**

1. Check application is running:
   ```bash
   sudo systemctl status mahresources
   ```

2. Check bind address:
   - `127.0.0.1:8080` - localhost only
   - `:8080` - all interfaces

3. Check firewall:
   ```bash
   sudo ufw status
   ```

## FAQ

### Can I use this with multiple users?

Yes, but there's no user isolation. Everyone sees everything. All users are effectively administrators.

### How do I migrate from SQLite to PostgreSQL?

1. Export data using SQLite tools or application API
2. Set up PostgreSQL database
3. Import data
4. Update configuration to use PostgreSQL

There's no built-in migration tool.

### What file types are supported?

Any file type can be uploaded. Thumbnails are generated for:
- Images: JPEG, PNG, GIF, WebP, BMP, TIFF
- Videos: MP4, WebM, MOV, AVI (requires ffmpeg)
- Documents: PDF, Word, Excel, PowerPoint (requires LibreOffice)

### How much disk space do versions use?

Each version is a complete file copy. There's no delta storage or automatic cleanup. Plan storage accordingly.

### Can I run multiple instances?

- **SQLite:** Only one writer at a time. Multiple readers OK.
- **PostgreSQL:** Multiple instances OK.

### How do I reset everything?

```bash
# Stop application
sudo systemctl stop mahresources

# Delete database and files
rm -rf /var/lib/mahresources/data/*
rm -rf /var/lib/mahresources/files/*

# Start fresh
sudo systemctl start mahresources
```

:::warning
This permanently deletes all data!
:::

## Getting Help

- **GitHub Issues:** [github.com/egeozcan/mahresources/issues](https://github.com/egeozcan/mahresources/issues)
- **Source Code:** [github.com/egeozcan/mahresources](https://github.com/egeozcan/mahresources)
```

**Step 2: Commit troubleshooting page**

```bash
git add docs-site/docs/troubleshooting.md
git commit -m "docs: add troubleshooting and FAQ"
```

---

## Phase 9: Final Verification

### Task 19: Build and Verify Documentation

**Step 1: Install dependencies**

```bash
cd /Users/egecan/Code/mahresources/.worktrees/feature/user-documentation/docs-site
npm install
```

**Step 2: Build the site**

```bash
npm run build
```

Expected: Build completes without errors.

**Step 3: Preview locally**

```bash
npm run serve
```

Open http://localhost:3000/mahresources/ and verify:
- All pages load
- Navigation works
- Security warning banner appears
- Links between pages work

**Step 4: Fix any issues**

If build fails or pages are broken, fix the issues.

**Step 5: Commit any fixes**

```bash
git add .
git commit -m "fix: documentation build issues"
```

---

### Task 20: Final Commit and Branch Ready

**Step 1: Verify all commits**

```bash
git log --oneline
```

**Step 2: Push branch**

```bash
git push -u origin feature/user-documentation
```

**Step 3: Create PR (optional)**

```bash
gh pr create --title "docs: add comprehensive user documentation site" --body "Adds Docusaurus documentation site with:
- Getting started guides
- Core concepts explanation
- User guide for all features
- Configuration reference
- API documentation
- Deployment guides
- Troubleshooting and FAQ

Includes GitHub Actions workflow for automatic deployment to GitHub Pages.

## Testing
- Run \`cd docs-site && npm run build\` to verify build
- Run \`cd docs-site && npm run serve\` to preview locally"
```

---

## Summary

This plan creates:
- **30 documentation pages** across 9 sections
- **Docusaurus 3** site with TypeScript configuration
- **GitHub Actions** workflow for automatic deployment
- **Prominent security warnings** throughout
- Comprehensive coverage from installation to API reference
