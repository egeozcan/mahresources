---
slug: /
sidebar_position: 1
---

# Introduction

Mahresources is a self-hosted system for storing files, writing notes, and linking them together. It runs as a single Go binary with SQLite or PostgreSQL, and serves a web UI for browsing, searching, and editing everything.

## What is Mahresources?

Mahresources stores files (called Resources), Notes, and Groups in a database, tracks the relationships between them, and makes everything searchable. Groups can nest inside each other and contain any mix of Resources and Notes. Tags and Categories provide additional ways to classify items across Groups.

## Key Features

- **File storage with thumbnails** - Store any file type. Images, videos, and Office documents get automatic thumbnails (videos require FFmpeg, documents require LibreOffice).
- **Notes** - Create text notes and link them to specific Resources and Groups.
- **Nested Groups** - Groups contain Resources, Notes, and other Groups, forming a hierarchy.
- **Tags and Categories** - Tag any entity. Categories define types for Groups (e.g., "Person", "Project").
- **Full-text search** - SQLite FTS5-backed search across all content, accessible via Cmd/Ctrl+K.
- **Image similarity** - Perceptual hashing finds visually similar images automatically.
- **Resource versioning** - Track versions of a Resource over time.
- **Saved queries** - Store and re-run search filters.
- **Group relations** - Define typed relationships between Groups (e.g., "works at", "belongs to").
- **JSON API** - Every page has a JSON equivalent (`Accept: application/json` or `.json` suffix) for scripting and integration.

:::danger No Authentication

Mahresources has **no** authentication or authorization. Run it on a private network only.

**Do not expose it to the internet.** For remote access, put it behind a reverse proxy with authentication (nginx + basic auth, OAuth2 Proxy, Authelia, etc.).

:::

## Who is This For?

Anyone who wants to store files and notes locally, link them together, and search across them without relying on a cloud service. Common uses include research material management, personal knowledge bases, and file archiving with metadata.

## Getting Started

[Install Mahresources](./getting-started/installation) to get started.
