---
slug: /
sidebar_position: 1
---

# Introduction

Mahresources is a personal information management system designed to help you organize, connect, and retrieve your files, notes, and knowledge. Built with Go and designed for self-hosting, it provides a powerful yet intuitive way to manage your digital resources.

## What is Mahresources?

At its core, Mahresources is a CRUD application that manages the relationships between your digital assets. Whether you're organizing research materials, managing a personal knowledge base, or archiving files with rich metadata, Mahresources provides the tools to keep everything connected and searchable.

## Key Features

- **Resource Management** - Upload, organize, and manage files of any type with automatic thumbnail generation for images, videos, and documents
- **Note Taking** - Create rich text notes and link them to your resources and groups
- **Hierarchical Organization** - Organize content into nested groups with parent-child relationships
- **Tagging System** - Flexible tagging with categories for multi-dimensional organization
- **Full-Text Search** - Fast search across all your content with SQLite FTS5
- **Image Similarity** - Find visually similar images using perceptual hashing
- **Version Control** - Track different versions of your resources over time
- **Saved Queries** - Save and reuse complex search queries
- **Relationships** - Define custom typed relationships between groups
- **REST API** - Full JSON API for integration with other tools and automation

:::danger No Authentication

Mahresources does **not** include authentication or authorization. It is designed for deployment on private networks only.

**Do not expose Mahresources directly to the internet.** If you need remote access, use a reverse proxy with authentication (such as nginx with basic auth, OAuth2 Proxy, or Authelia).

:::

## Who is This For?

Mahresources is ideal for:

- **Researchers** managing papers, notes, and source materials
- **Knowledge workers** building personal knowledge bases
- **Digital archivists** organizing large file collections with metadata
- **Hobbyists** cataloging collections (photos, media, documents)
- **Anyone** who wants more control over their personal data than cloud services provide

## Getting Started

Ready to dive in? Head to the [Installation Guide](./getting-started/installation) to get Mahresources running on your system.
