---
sidebar_position: 1
---

# Installation

This guide covers the different ways to install and run Mahresources.

:::danger Security Warning

Mahresources has **no built-in authentication**. Never expose it directly to the public internet. Always run it on a private network or behind a reverse proxy with proper authentication.

:::

## Prerequisites

Depending on your installation method, you'll need:

### For Building from Source
- **Go 1.21+** - [Download Go](https://go.dev/dl/)
- **Node.js 18+** - [Download Node.js](https://nodejs.org/)

### For Docker
- **Docker 20+** - [Install Docker](https://docs.docker.com/get-docker/)

## Option 1: Pre-built Binaries

Download the latest release for your platform from the [GitHub Releases](https://github.com/egeozcan/mahresources/releases) page.

```bash
# Extract the archive
tar -xzf mahresources-linux-amd64.tar.gz

# Make it executable (Linux/macOS)
chmod +x mahresources

# Run it
./mahresources -ephemeral -bind-address=:8080
```

## Option 2: Build from Source

Clone the repository and build:

```bash
# Clone the repository
git clone https://github.com/egeozcan/mahresources.git
cd mahresources

# Install dependencies and build everything (CSS + JS + Go binary)
npm install
npm run build

# Build the Go binary with required SQLite extensions
go build --tags 'json1 fts5'
```

The `json1` tag enables SQLite JSON functions, and `fts5` enables full-text search.

## Option 3: Docker

Run Mahresources with Docker:

```bash
# Pull and run (ephemeral mode)
docker run -p 8080:8080 ghcr.io/egeozcan/mahresources:latest -ephemeral

# Run with persistent storage
docker run -p 8080:8080 \
  -v $(pwd)/data:/data \
  -v $(pwd)/files:/files \
  ghcr.io/egeozcan/mahresources:latest \
  -db-type=SQLITE \
  -db-dsn=/data/mahresources.db \
  -file-save-path=/files
```

## Optional Dependencies

For enhanced functionality, install these optional tools:

### FFmpeg (Video Thumbnails)

FFmpeg enables automatic thumbnail generation for video files.

```bash
# macOS
brew install ffmpeg

# Ubuntu/Debian
sudo apt install ffmpeg

# Windows (via Chocolatey)
choco install ffmpeg
```

Then specify the path (if not in PATH):
```bash
./mahresources -ffmpeg-path=/usr/bin/ffmpeg
```

### LibreOffice (Document Thumbnails)

LibreOffice enables thumbnail generation for Office documents (Word, Excel, PowerPoint, etc.).

```bash
# macOS
brew install --cask libreoffice

# Ubuntu/Debian
sudo apt install libreoffice

# Windows
# Download from https://www.libreoffice.org/download/
```

Mahresources auto-detects `soffice` or `libreoffice` in your PATH. To specify a custom path:
```bash
./mahresources -libreoffice-path=/usr/bin/libreoffice
```

## Next Steps

Once installed, proceed to the [Quick Start](./quick-start) guide to run Mahresources for the first time.
