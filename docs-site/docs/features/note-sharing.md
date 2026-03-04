---
sidebar_position: 5
title: Note Sharing
description: Share notes publicly via secure, unguessable URLs
---

# Note Sharing

Individual notes can be published to a separate, public-facing server. Shared notes are accessible via cryptographically secure URLs without authentication, letting you share content publicly while keeping the main instance private.

## How It Works

When you share a note:

1. A 128-bit cryptographically random token is generated
2. The token is stored with the note in your database
3. A share URL is created: `/s/{token}`
4. The shared note is accessible on the share server (separate port)

The share URL is unguessable - knowing one token doesn't help discover others. Tokens persist until you explicitly unshare the note.

## What Gets Shared

When a note is shared, visitors can see:

- **Note content** - The note's name, description, and text content
- **Block content** - Interactive blocks (like todo lists) with shared state
- **Embedded resources** - Images and files attached to the note

What remains private:

- Tags and categories
- Group associations
- Metadata
- Other notes and resources

## Enabling Note Sharing

Note sharing requires configuring the share server. See [Public Sharing Deployment](../deployment/public-sharing.md) for detailed setup instructions.

Quick start with command-line flags:

```bash
./mahresources \
  -db-type=SQLITE \
  -db-dsn=./data/mahresources.db \
  -file-save-path=./data/files \
  -bind-address=:8181 \
  -share-port=8383
```

## Sharing a Note

### From the Note Display Page

1. Navigate to the note you want to share
2. In the sidebar, find the **Sharing** section
3. Click **Share Note**

When shared:
- The URL is automatically copied to your clipboard
- A "Shared" badge appears
- The share URL is displayed with a copy button
- An **Unshare** button becomes available

### Using the API

Share a note programmatically:

```bash
# Share a note
curl -X POST "http://localhost:8181/v1/note/share?noteId=123"

# Response:
# { "shareToken": "a1b2c3d4...", "shareUrl": "/s/a1b2c3d4..." }
```

Unshare a note:

```bash
curl -X DELETE "http://localhost:8181/v1/note/share?noteId=123"
```

## Accessing Shared Notes

Shared notes are accessed on the share server:

```
http://your-share-server:8383/s/{token}
```

### Share Server Routes

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/s/{token}` | View the shared Note |
| `POST` | `/s/{token}/block/{blockId}/state` | Update block state (toggle operations only) |
| `GET` | `/s/{token}/block/{blockId}/calendar/events` | Get calendar events for a calendar block |
| `GET` | `/s/{token}/resource/{hash}` | Access a Resource file by its hash |

The share server runs on a separate port and only serves these routes. Resource access is validated -- the server checks that the requested Resource belongs to the shared Note (either through direct associations or gallery block references).

## Interactive Blocks on Shared Notes

Interactive blocks (like todo lists) work differently on shared notes:

- **State is global** - Changes are visible to all viewers
- **Limited functionality** - Only toggle operations work (e.g., checking items)
- **No add/remove** - Creating or deleting items is not allowed

Viewers can collaborate on shared content but cannot add or remove items, which prevents spam and vandalism.

## Finding Shared Notes

Filter your notes list to show only shared notes:

1. Go to **Notes** in the navigation menu
2. In the filter panel, check **Shared Only**
3. Click **Search**

This shows all notes that currently have a share token.

## Unsharing Notes

To stop sharing a note:

1. Navigate to the shared note
2. In the sidebar **Sharing** section, click **Unshare**

When unshared:
- The share token is deleted
- The share URL immediately stops working
- If you share again later, a new token is generated

## Security Considerations

### Token Security

- Tokens are 128-bit cryptographically random values
- Generated using Go's `crypto/rand` package
- Represented as 32-character hex strings
- Cannot be predicted or enumerated

### Network Architecture

For public sharing, we recommend:

1. Keep your main Mahresources instance on a private network
2. Expose only the share server port through a reverse proxy
3. Use HTTPS for the public share server
4. Consider rate limiting on the share server

See [Public Sharing Deployment](../deployment/public-sharing.md) for detailed security guidance.

### Data Exposure

Before sharing a note, review its content carefully:
- The note's full text will be publicly visible
- Any embedded resources (images, files) will be accessible
- Block content (including todo items) will be visible

## API Reference

### Share Note

```
POST /v1/note/share?noteId={id}
```

Response:
```json
{
  "shareToken": "a1b2c3d4e5f6...",
  "shareUrl": "/s/a1b2c3d4e5f6..."
}
```

If the note is already shared, returns the existing token.

### Unshare Note

```
DELETE /v1/note/share?noteId={id}
```

Response:
```json
{
  "success": true
}
```

### List Shared Notes

```
GET /v1/notes?Shared=1
```

Returns all notes that have a share token.
