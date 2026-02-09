---
sidebar_position: 1
---

# Installation

:::danger Security Warning

Mahresources has **no built-in authentication**. Never expose it directly to the public internet. Always run it on a private network or behind a reverse proxy with proper authentication.

:::

## Prerequisites

### Building from Source
- **Go 1.22+** - [Download Go](https://go.dev/dl/)
- **Node.js 18+** - [Download Node.js](https://nodejs.org/)

### Docker
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

Clone and build:

```bash
# Clone the repository
git clone https://github.com/egeozcan/mahresources.git
cd mahresources

# Install dependencies and build everything (CSS + JS + Go binary)
npm install
npm run build
```

The `npm run build` command builds the Tailwind CSS, bundles the JavaScript with Vite, and compiles the Go binary with the `json1` (SQLite JSON functions) and `fts5` (full-text search) build tags.

## Option 3: Docker

No pre-built image is published. Build it locally from the repository:

```bash
git clone https://github.com/egeozcan/mahresources.git
cd mahresources
docker build -t mahresources .

# Run in ephemeral mode (data lost on exit)
docker run -p 8181:8181 mahresources -ephemeral

# Run with persistent storage
docker run -p 8181:8181 \
  -v $(pwd)/data:/app/data \
  -v $(pwd)/files:/app/files \
  mahresources
```

See the [Docker deployment guide](../deployment/docker) for compose files and production setup.

## Optional Dependencies

### FFmpeg (Video Thumbnails)

Install FFmpeg to generate thumbnails for video files.

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

Install LibreOffice to generate thumbnails for Office documents (Word, Excel, PowerPoint, etc.).

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

Next: [Quick Start](./quick-start) -- run Mahresources for the first time.
